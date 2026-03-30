// Package main provides the docking job controller with REST API
package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typed "k8s.io/client-go/kubernetes/typed/batch/v1"
	rest "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	DefaultImage            = "zot.hwcopeland.net/chem/autodock-vina:latest"
	DefaultLigandsChunkSize = 10000
	DefaultPDBID            = "7jrn"
	DefaultNativeLigand     = "TTT"
	WorkDir                 = "/data"
)

// DockingJobController handles the lifecycle of docking workflows.
// State is persisted in MySQL; no in-memory maps.
type DockingJobController struct {
	client    *kubernetes.Clientset
	namespace string
	jobClient typed.JobInterface
	db        *sql.DB
	stopCh    chan struct{}
}

// DockingJob is the in-process representation passed to processDockingJob.
type DockingJob struct {
	Name   string
	Spec   DockingJobSpec
	Status DockingJobStatus
}

type DockingJobSpec struct {
	PDBID            string
	LigandDb         string // maps to source_db in MySQL
	NativeLigand     string
	LigandsChunkSize int
	Image            string
}

type DockingJobStatus struct {
	Phase            string
	BatchCount       int
	CompletedBatches int
	Message          string
	StartTime        *time.Time
	CompletionTime   *time.Time
}

// receptorData is parsed from proteinprepv2.py JSON stdout.
type receptorData struct {
	PDBQTB64   string     `json:"pdbqt_b64"`
	GridCenter gridCenter `json:"grid_center"`
}

type gridCenter struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// NewDockingJobController creates a new controller
func NewDockingJobController() (*DockingJobController, error) {
	config, err := getConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %v", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = "chem"
	}

	// Build MySQL DSN from environment variables.
	mysqlHost := os.Getenv("MYSQL_HOST")
	mysqlPort := os.Getenv("MYSQL_PORT")
	mysqlUser := os.Getenv("MYSQL_USER")
	mysqlPassword := os.Getenv("MYSQL_PASSWORD")
	mysqlDatabase := os.Getenv("MYSQL_DATABASE")
	if mysqlPort == "" {
		mysqlPort = "3306"
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		mysqlUser, mysqlPassword, mysqlHost, mysqlPort, mysqlDatabase)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mysql connection: %v", err)
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping mysql: %v", err)
	}
	log.Println("MySQL connection established")

	controller := &DockingJobController{
		client:    client,
		namespace: namespace,
		jobClient: client.BatchV1().Jobs(namespace),
		db:        db,
		stopCh:    make(chan struct{}),
	}

	if err := controller.ensureSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ensure database schema: %v", err)
	}
	log.Println("Database schema verified")

	return controller, nil
}

func getConfig() (*rest.Config, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

// ensureSchema creates the required database tables if they do not exist.
func (c *DockingJobController) ensureSchema() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS docking_workflows (
			name              VARCHAR(255) PRIMARY KEY,
			phase             ENUM('Pending', 'Running', 'Completed', 'Failed') NOT NULL DEFAULT 'Pending',
			pdbid             VARCHAR(32)  NOT NULL,
			source_db         VARCHAR(255) NOT NULL,
			native_ligand     VARCHAR(32)  NOT NULL DEFAULT 'TTT',
			chunk_size        INT          NOT NULL DEFAULT 10000,
			image             VARCHAR(512) NOT NULL,
			batch_count       INT          NOT NULL DEFAULT 0,
			completed_batches INT          NOT NULL DEFAULT 0,
			current_step      VARCHAR(64)  NULL,
			message           TEXT         NULL,
			result            TEXT         NULL,
			receptor_pdbqt    MEDIUMBLOB   NULL,
			grid_center_x     FLOAT        NULL,
			grid_center_y     FLOAT        NULL,
			grid_center_z     FLOAT        NULL,
			created_at        TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
			started_at        TIMESTAMP    NULL,
			completed_at      TIMESTAMP    NULL,
			INDEX idx_phase (phase),
			INDEX idx_created_at (created_at)
		)`,
		`CREATE TABLE IF NOT EXISTS ligands (
			id            INT AUTO_INCREMENT PRIMARY KEY,
			compound_id   VARCHAR(255) NOT NULL,
			smiles        TEXT         NOT NULL,
			pdbqt         MEDIUMBLOB   NULL,
			source_db     VARCHAR(255) NOT NULL,
			created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_source_db (source_db),
			UNIQUE INDEX idx_compound_source (compound_id, source_db)
		)`,
		`CREATE TABLE IF NOT EXISTS docking_results (
			id                INT AUTO_INCREMENT PRIMARY KEY,
			workflow_name     VARCHAR(255) NOT NULL,
			pdb_id            VARCHAR(10)  NOT NULL,
			ligand_id         INT          NOT NULL,
			compound_id       VARCHAR(255) NOT NULL,
			affinity_kcal_mol FLOAT        NOT NULL,
			created_at        TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_workflow (workflow_name),
			INDEX idx_pdbid    (pdb_id),
			INDEX idx_affinity (affinity_kcal_mol),
			INDEX idx_ligand   (ligand_id)
		)`,
		`CREATE TABLE IF NOT EXISTS staging (
			id         INT AUTO_INCREMENT PRIMARY KEY,
			job_type   ENUM('prep', 'dock') NOT NULL,
			payload    JSON NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, ddl := range tables {
		if _, err := c.db.Exec(ddl); err != nil {
			return fmt.Errorf("executing DDL: %w\n%s", err, ddl)
		}
	}
	return nil
}

// Run starts the controller
func (c *DockingJobController) Run(ctx context.Context) error {
	log.Println("Starting Docking Job Controller...")

	go func() {
		if err := c.startAPIServer(); err != nil && err != http.ErrServerClosed {
			log.Printf("API server error: %v", err)
		}
	}()

	<-ctx.Done()
	return ctx.Err()
}

func (c *DockingJobController) startAPIServer() error {
	handler := NewAPIHandler(c.client, c.namespace, c, c.db)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/dockingjobs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.ListJobs(w, r)
		case http.MethodPost:
			handler.CreateJob(w, r)
		default:
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/dockingjobs/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if hasResultsSuffix(r.URL.Path) {
				handler.GetResults(w, r)
			} else if hasLogsSuffix(r.URL.Path) {
				handler.GetLogs(w, r)
			} else {
				handler.GetJob(w, r)
			}
		case http.MethodDelete:
			handler.DeleteJob(w, r)
		default:
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/ligands", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.ImportLigands(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/health", handler.HealthCheck)
	mux.HandleFunc("/readyz", handler.ReadinessCheck)

	log.Println("API server listening on :8080")
	return http.ListenAndServe(":8080", mux)
}

func hasLogsSuffix(path string) bool {
	return len(path) > 6 && path[len(path)-5:] == "/logs"
}

func hasResultsSuffix(path string) bool {
	return len(path) > 9 && path[len(path)-8:] == "/results"
}

// processDockingJob runs the 2-step pipeline: prepare-receptor then dock-batch × N.
func (c *DockingJobController) processDockingJob(job DockingJob) {
	log.Printf("[%s] Starting pipeline: pdbid=%s source_db=%s image=%s chunk_size=%d",
		job.Name, job.Spec.PDBID, job.Spec.LigandDb, job.Spec.Image, job.Spec.LigandsChunkSize)

	c.updateWorkflow(job.Name, "current_step", "prepare-receptor")
	c.updateWorkflow(job.Name, "started_at", time.Now())

	// Step 1: Prepare receptor
	log.Printf("[%s] Step 1/2: prepare-receptor", job.Name)
	if err := c.createPrepareReceptorJob(job); err != nil {
		c.failJob(job.Name, fmt.Sprintf("prepare receptor failed: %v", err))
		return
	}
	receptorJobName := fmt.Sprintf("%s-prepare-receptor", job.Name)
	if err := c.waitForJobCompletion(receptorJobName); err != nil {
		c.failJob(job.Name, fmt.Sprintf("prepare receptor failed: %v", err))
		return
	}

	// Capture receptor data from pod stdout and store in MySQL.
	if err := c.captureReceptorData(job.Name, receptorJobName); err != nil {
		c.failJob(job.Name, fmt.Sprintf("capture receptor data failed: %v", err))
		return
	}
	log.Printf("[%s] Receptor data captured and stored", job.Name)

	// Compute batch count from ligand count.
	var ligandCount int
	if err := c.db.QueryRow(
		`SELECT COUNT(*) FROM ligands WHERE source_db = ? AND pdbqt IS NOT NULL`,
		job.Spec.LigandDb).Scan(&ligandCount); err != nil {
		c.failJob(job.Name, fmt.Sprintf("failed to count ligands: %v", err))
		return
	}
	if ligandCount == 0 {
		c.failJob(job.Name, fmt.Sprintf("no prepped ligands found for source_db '%s'", job.Spec.LigandDb))
		return
	}

	batchCount := int(math.Ceil(float64(ligandCount) / float64(job.Spec.LigandsChunkSize)))
	if _, err := c.db.Exec(`UPDATE docking_workflows SET batch_count = ?, current_step = 'dock-batch' WHERE name = ?`,
		batchCount, job.Name); err != nil {
		log.Printf("[%s] failed to update batch_count: %v", job.Name, err)
	}
	log.Printf("[%s] Step 2/2: docking %d ligands in %d batch(es)", job.Name, ligandCount, batchCount)

	// Step 2: Dock batches sequentially.
	for i := 0; i < batchCount; i++ {
		offset := i * job.Spec.LigandsChunkSize
		log.Printf("[%s] Batch %d/%d (offset=%d limit=%d)", job.Name, i+1, batchCount, offset, job.Spec.LigandsChunkSize)

		if err := c.createDockBatchJob(job, i, offset); err != nil {
			c.failJob(job.Name, fmt.Sprintf("dock batch %d failed to create: %v", i, err))
			return
		}
		dockJobName := fmt.Sprintf("%s-dock-batch-%d", job.Name, i)
		if err := c.waitForJobCompletion(dockJobName); err != nil {
			c.failJob(job.Name, fmt.Sprintf("dock batch %d failed: %v", i, err))
			return
		}

		if _, err := c.db.Exec(`UPDATE docking_workflows SET completed_batches = ? WHERE name = ?`, i+1, job.Name); err != nil {
			log.Printf("[%s] failed to update completed_batches: %v", job.Name, err)
		}
		log.Printf("[%s] Batch %d/%d complete", job.Name, i+1, batchCount)
	}

	// Wait for result-writer to drain staging, then compute best energy.
	log.Printf("[%s] Waiting for result-writer to drain staging...", job.Name)
	if err := c.waitForStagingDrain(job.Name); err != nil {
		log.Printf("[%s] Warning: staging drain wait failed: %v", job.Name, err)
	}

	var bestEnergy sql.NullFloat64
	c.db.QueryRow(`SELECT MIN(affinity_kcal_mol) FROM docking_results WHERE workflow_name = ?`,
		job.Name).Scan(&bestEnergy)

	result := "No results"
	if bestEnergy.Valid {
		result = fmt.Sprintf("Best energy: %.1f kcal/mol", bestEnergy.Float64)
	}

	if _, err := c.db.Exec(`UPDATE docking_workflows SET phase = 'Completed', completed_at = NOW(),
		current_step = NULL, result = ?, message = ? WHERE name = ?`,
		result, result, job.Name); err != nil {
		log.Printf("[%s] failed to mark workflow completed: %v", job.Name, err)
	}

	log.Printf("[%s] Pipeline complete: %s", job.Name, result)
}

// waitForStagingDrain polls the staging table until no dock rows remain for this workflow,
// or a timeout is reached.
func (c *DockingJobController) waitForStagingDrain(workflowName string) error {
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for staging drain")
		case <-ticker.C:
			var count int
			err := c.db.QueryRow(
				`SELECT COUNT(*) FROM staging WHERE job_type = 'dock' AND JSON_EXTRACT(payload, '$.workflow_name') = ?`,
				workflowName).Scan(&count)
			if err != nil {
				log.Printf("[%s] staging drain query error: %v", workflowName, err)
				continue
			}
			if count == 0 {
				return nil
			}
			log.Printf("[%s] Staging still has %d dock rows, waiting...", workflowName, count)
		}
	}
}

func (c *DockingJobController) failJob(workflowName string, message string) {
	if _, err := c.db.Exec(`UPDATE docking_workflows SET phase = 'Failed', current_step = NULL, message = ? WHERE name = ?`,
		message, workflowName); err != nil {
		log.Printf("[%s] CRITICAL: failed to mark workflow as Failed: %v", workflowName, err)
	}
	log.Printf("Docking job %s failed: %s", workflowName, message)
}

var allowedWorkflowColumns = map[string]bool{
	"current_step": true, "started_at": true, "completed_at": true,
	"batch_count": true, "completed_batches": true, "message": true, "result": true, "phase": true,
}

func (c *DockingJobController) updateWorkflow(name, column string, value interface{}) {
	if !allowedWorkflowColumns[column] {
		log.Printf("[updateWorkflow] rejected unknown column %q for workflow %s", column, name)
		return
	}
	if _, err := c.db.Exec(fmt.Sprintf(`UPDATE docking_workflows SET %s = ? WHERE name = ?`, column), value, name); err != nil {
		log.Printf("[updateWorkflow] failed to update %s for workflow %s: %v", column, name, err)
	}
}

// createPrepareReceptorJob creates a K8s Job that runs proteinprepv2.py with emptyDir.
func (c *DockingJobController) createPrepareReceptorJob(job DockingJob) error {
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-prepare-receptor", job.Name),
			Labels: map[string]string{
				"docking.khemia.io/workflow":   job.Name,
				"docking.khemia.io/job-type":   "prepare-receptor",
				"docking.khemia.io/parent-job": job.Name,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(300),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyOnFailure,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{
						{
							Name:            "prepare",
							Image:           job.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							WorkingDir:      WorkDir,
							Command:         []string{"python3", "/autodock/scripts/proteinprepv2.py"},
							Args: []string{
								"--protein_id", job.Spec.PDBID,
								"--ligand_id", job.Spec.NativeLigand,
							},
							VolumeMounts: []corev1.VolumeMount{emptyDirMount("scratch", WorkDir)},
						},
					},
					Volumes: []corev1.Volume{emptyDirVolume("scratch")},
				},
			},
		},
	}

	_, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	return err
}

// captureReceptorData reads receptor JSON from the prepare-receptor pod's stdout
// and stores the PDBQT + grid center in the docking_workflows row.
func (c *DockingJobController) captureReceptorData(workflowName, jobName string) error {
	stdout, err := c.readPodLogs(jobName)
	if err != nil {
		return fmt.Errorf("reading receptor pod logs: %w", err)
	}

	// Find the JSON line in the output (proteinprepv2.py prints other logs too).
	var rd receptorData
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") {
			if err := json.Unmarshal([]byte(line), &rd); err == nil && rd.PDBQTB64 != "" {
				break
			}
		}
	}
	if rd.PDBQTB64 == "" {
		return fmt.Errorf("no receptor data found in pod stdout")
	}

	pdbqtBytes, err := base64.StdEncoding.DecodeString(rd.PDBQTB64)
	if err != nil {
		return fmt.Errorf("decoding receptor PDBQT: %w", err)
	}

	_, err = c.db.Exec(`UPDATE docking_workflows
		SET receptor_pdbqt = ?, grid_center_x = ?, grid_center_y = ?, grid_center_z = ?
		WHERE name = ?`,
		pdbqtBytes, rd.GridCenter.X, rd.GridCenter.Y, rd.GridCenter.Z, workflowName)
	return err
}

// createDockBatchJob creates a K8s Job that runs dock_batch.py for a ligand batch.
// The dock pod reads ligands + receptor from MySQL and writes results to the staging table.
func (c *DockingJobController) createDockBatchJob(job DockingJob, batchIndex, offset int) error {
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-dock-batch-%d", job.Name, batchIndex),
			Labels: map[string]string{
				"docking.khemia.io/workflow":   job.Name,
				"docking.khemia.io/job-type":   "dock-batch",
				"docking.khemia.io/parent-job": job.Name,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(300),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyOnFailure,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{
						{
							Name:            "dock",
							Image:           job.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							WorkingDir:      WorkDir,
							Command:         []string{"python3", "/autodock/scripts/dock_batch.py"},
							Env: []corev1.EnvVar{
								{Name: "WORKFLOW_NAME", Value: job.Name},
								{Name: "PDBID", Value: job.Spec.PDBID},
								{Name: "NATIVE_LIGAND", Value: job.Spec.NativeLigand},
								{Name: "SOURCE_DB", Value: job.Spec.LigandDb},
								{Name: "BATCH_OFFSET", Value: fmt.Sprintf("%d", offset)},
								{Name: "BATCH_LIMIT", Value: fmt.Sprintf("%d", job.Spec.LigandsChunkSize)},
								{Name: "MYSQL_HOST", Value: os.Getenv("MYSQL_HOST")},
								{Name: "MYSQL_PORT", Value: os.Getenv("MYSQL_PORT")},
								{Name: "MYSQL_USER", Value: os.Getenv("MYSQL_USER")},
								{Name: "MYSQL_DATABASE", Value: os.Getenv("MYSQL_DATABASE")},
								{
									Name: "MYSQL_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: "docking-mysql-secret"},
											Key:                  "root-password",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{emptyDirMount("scratch", WorkDir)},
						},
					},
					Volumes: []corev1.Volume{emptyDirVolume("scratch")},
				},
			},
		},
	}

	_, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	return err
}

// readPodLogs reads the first pod's stdout for a given job.
func (c *DockingJobController) readPodLogs(jobName string) (string, error) {
	pods, err := c.client.CoreV1().Pods(c.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil || len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods found for job %s", jobName)
	}

	stream, err := c.client.CoreV1().Pods(c.namespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{}).
		Stream(context.TODO())
	if err != nil {
		return "", fmt.Errorf("getting logs: %w", err)
	}
	defer stream.Close()

	buf, err := io.ReadAll(stream)
	if err != nil {
		return "", fmt.Errorf("reading logs: %w", err)
	}
	return string(buf), nil
}

func (c *DockingJobController) waitForJobCompletion(jobName string) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timeout := time.After(10 * time.Minute)
	pollCount := 0

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for job %s", jobName)
		case <-ticker.C:
			pollCount++
			job, err := c.jobClient.Get(context.TODO(), jobName, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					if pollCount == 1 || pollCount%12 == 0 {
						log.Printf("[wait] %s: not found yet (poll %d)", jobName, pollCount)
					}
					continue
				}
				return err
			}
			if pollCount%12 == 0 {
				log.Printf("[wait] %s: active=%d succeeded=%d failed=%d (poll %d)",
					jobName, job.Status.Active, job.Status.Succeeded, job.Status.Failed, pollCount)
			}
			if job.Status.Succeeded > 0 {
				log.Printf("[wait] %s: succeeded after %d polls", jobName, pollCount)
				return nil
			}
			if job.Status.Failed > 0 {
				log.Printf("[wait] %s: FAILED after %d polls (active=%d)", jobName, pollCount, job.Status.Active)
				return fmt.Errorf("job %s failed", jobName)
			}
		}
	}
}

// emptyDirVolume returns a corev1.Volume backed by emptyDir.
func emptyDirVolume(name string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// emptyDirMount returns a corev1.VolumeMount for an emptyDir volume.
func emptyDirMount(name, mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
	}
}

// ptrInt32 returns a pointer to an int32
func ptrInt32(i int32) *int32 {
	return &i
}

func main() {
	log.Println("Docking Job Controller starting...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	controller, err := NewDockingJobController()
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}

	if err := controller.Run(ctx); err != nil {
		log.Fatalf("Controller error: %v", err)
	}
}
