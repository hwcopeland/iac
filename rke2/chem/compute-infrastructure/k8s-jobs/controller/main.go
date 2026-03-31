// Package main provides the docking job controller with REST API
package main

import (
	"bufio"
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
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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

	DefaultQEImage      = "costrouc/quantum-espresso:latest"
	DefaultQEExecutable = "pw.x"
	DefaultQENumCPUs    = 1
	DefaultQEMemoryMB   = 2048
	DefaultQETimeoutH   = 4 // hours
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
		`CREATE TABLE IF NOT EXISTS qe_jobs (
			id              INT AUTO_INCREMENT PRIMARY KEY,
			name            VARCHAR(255) NOT NULL UNIQUE,
			status          ENUM('Pending','Running','Completed','Failed') NOT NULL DEFAULT 'Pending',
			executable      VARCHAR(64)  NOT NULL DEFAULT 'pw.x',
			input_file      MEDIUMTEXT   NOT NULL,
			output_file     MEDIUMTEXT   NULL,
			error_output    MEDIUMTEXT   NULL,
			total_energy    DOUBLE       NULL,
			wall_time_sec   FLOAT        NULL,
			num_cpus        INT          NOT NULL DEFAULT 1,
			memory_mb       INT          NOT NULL DEFAULT 2048,
			image           VARCHAR(512) NOT NULL DEFAULT 'opensciencegrid/osgvo-quantum-espresso:latest',
			submitted_by    VARCHAR(255) NULL,
			created_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
			started_at      TIMESTAMP    NULL,
			completed_at    TIMESTAMP    NULL,
			INDEX idx_status (status),
			INDEX idx_created_at (created_at)
		)`,
		`CREATE TABLE IF NOT EXISTS pseudopotentials (
			id          INT AUTO_INCREMENT PRIMARY KEY,
			filename    VARCHAR(255) NOT NULL UNIQUE,
			content     MEDIUMBLOB NOT NULL,
			element     VARCHAR(4) NOT NULL,
			functional  VARCHAR(32) NULL,
			source_url  TEXT NULL,
			created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_element (element)
		)`,
	}

	for _, ddl := range tables {
		if _, err := c.db.Exec(ddl); err != nil {
			return fmt.Errorf("executing DDL: %w\n%s", err, ddl)
		}
	}

	// Migrate old docking_results schema if needed (add ligand_id/compound_id, drop batch_label/ligand_name).
	migrations := []string{
		`ALTER TABLE docking_results ADD COLUMN ligand_id INT NOT NULL DEFAULT 0 AFTER pdb_id`,
		`ALTER TABLE docking_results ADD COLUMN compound_id VARCHAR(255) NOT NULL DEFAULT '' AFTER ligand_id`,
		`ALTER TABLE docking_results ADD INDEX idx_ligand (ligand_id)`,
		`ALTER TABLE docking_results DROP COLUMN batch_label`,
		`ALTER TABLE docking_results DROP COLUMN ligand_name`,
		`ALTER TABLE docking_results ADD COLUMN docked_pdbqt MEDIUMBLOB NULL`,
		`ALTER TABLE docking_workflows ADD COLUMN submitted_by VARCHAR(255) NULL`,
	}
	for _, m := range migrations {
		c.db.Exec(m) // Ignore errors (column may already exist or not exist)
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

	// Initialize auth middleware. When AUTH_ENABLED is "true", JWT validation
	// is enforced for external requests; internal pod/service CIDRs are exempt.
	// When disabled (default), all requests pass through without auth.
	var wrap func(http.HandlerFunc) http.HandlerFunc

	if os.Getenv("AUTH_ENABLED") == "true" {
		issuerURL := os.Getenv("OIDC_ISSUER_URL")
		if issuerURL == "" {
			return fmt.Errorf("AUTH_ENABLED=true but OIDC_ISSUER_URL is not set")
		}

		auth, err := NewAuthMiddleware(issuerURL)
		if err != nil {
			return fmt.Errorf("initializing auth middleware: %w", err)
		}
		wrap = auth.Wrap
		log.Println("JWT authentication enabled")
	} else {
		noop := noopAuthMiddleware()
		wrap = noop.WrapNoop
		log.Println("JWT authentication disabled (AUTH_ENABLED != \"true\")")
	}

	mux := http.NewServeMux()

	// API routes — wrapped with auth middleware.
	mux.HandleFunc("/api/v1/dockingjobs", wrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.ListJobs(w, r)
		case http.MethodPost:
			handler.CreateJob(w, r)
		default:
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/v1/dockingjobs/", wrap(func(w http.ResponseWriter, r *http.Request) {
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
	}))
	mux.HandleFunc("/api/v1/ligands", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.ImportLigands(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/v1/prep", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.StartPrep(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// QE (Quantum ESPRESSO) routes — wrapped with auth middleware.
	mux.HandleFunc("/api/v1/qe/submit", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.SubmitQEJob(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/v1/qe/jobs", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.ListQEJobs(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/v1/qe/jobs/", wrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.GetQEJob(w, r)
		case http.MethodDelete:
			handler.DeleteQEJob(w, r)
		default:
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/v1/qe/pseudopotentials", wrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handler.UploadPseudopotential(w, r)
		case http.MethodGet:
			handler.ListPseudopotentials(w, r)
		default:
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Health/readiness endpoints — always unauthenticated.
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
	receptorStreamCtx, receptorStreamCancel := context.WithCancel(context.Background())
	go c.streamJobLogs(receptorStreamCtx, receptorJobName)
	if err := c.waitForJobCompletion(receptorJobName); err != nil {
		receptorStreamCancel()
		c.failJob(job.Name, fmt.Sprintf("prepare receptor failed: %v", err))
		return
	}
	receptorStreamCancel()

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
		dockStreamCtx, dockStreamCancel := context.WithCancel(context.Background())
		go c.streamJobLogs(dockStreamCtx, dockJobName)
		if err := c.waitForJobCompletion(dockJobName); err != nil {
			dockStreamCancel()
			c.failJob(job.Name, fmt.Sprintf("dock batch %d failed: %v", i, err))
			return
		}
		dockStreamCancel()

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

// processLigandPrep runs sequential batch prep jobs to convert SMILES to PDBQT.
func (c *DockingJobController) processLigandPrep(req PrepRequest) {
	log.Printf("[prep] Starting ligand prep: source_db=%s chunk_size=%d image=%s",
		req.SourceDb, req.ChunkSize, req.Image)

	// Recount unprepared ligands at processing time (may differ from handler check).
	var unpreparedCount int
	if err := c.db.QueryRow(
		`SELECT COUNT(*) FROM ligands WHERE source_db = ? AND pdbqt IS NULL`,
		req.SourceDb).Scan(&unpreparedCount); err != nil {
		log.Printf("[prep] Failed to count unprepared ligands: %v", err)
		return
	}

	batchCount := int(math.Ceil(float64(unpreparedCount) / float64(req.ChunkSize)))
	log.Printf("[prep] %d unprepared ligands in %d batch(es)", unpreparedCount, batchCount)

	for i := 0; i < batchCount; i++ {
		offset := i * req.ChunkSize
		log.Printf("[prep] Batch %d/%d (offset=%d limit=%d)", i+1, batchCount, offset, req.ChunkSize)

		if err := c.createPrepBatchJob(req.SourceDb, req.Image, i, offset, req.ChunkSize); err != nil {
			log.Printf("[prep] Failed to create prep batch %d: %v", i, err)
			return
		}

		jobName := fmt.Sprintf("prep-%s-batch-%d", req.SourceDb, i)
		prepStreamCtx, prepStreamCancel := context.WithCancel(context.Background())
		go c.streamJobLogs(prepStreamCtx, jobName)
		if err := c.waitForJobCompletion(jobName); err != nil {
			prepStreamCancel()
			log.Printf("[prep] Prep batch %d failed: %v", i, err)
			return
		}
		prepStreamCancel()

		log.Printf("[prep] Batch %d/%d complete", i+1, batchCount)
	}

	log.Printf("[prep] All %d batches complete for source_db=%s", batchCount, req.SourceDb)
}

// processQEJob runs a Quantum ESPRESSO calculation as a single K8s Job.
// Input is provided via a ConfigMap, output is captured from pod stdout.
func (c *DockingJobController) processQEJob(jobName, executable, inputFile, image string, numCPUs, memoryMB int) {
	log.Printf("[%s] Starting QE job: executable=%s cpus=%d memory=%dMi image=%s",
		jobName, executable, numCPUs, memoryMB, image)

	// 1. Update status to Running.
	if _, err := c.db.Exec(`UPDATE qe_jobs SET status='Running', started_at=NOW() WHERE name=?`, jobName); err != nil {
		log.Printf("[%s] Failed to update QE job status to Running: %v", jobName, err)
		return
	}

	ctx := context.Background()
	cmName := fmt.Sprintf("qe-input-%s", jobName)

	// 2. Create a ConfigMap with input file + pseudopotentials from DB.
	cmData := map[string]string{"input.in": inputFile}
	cmBinary := map[string][]byte{}

	// Find .UPF references in the input file and look them up in the DB.
	for _, line := range strings.Split(inputFile, "\n") {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		for _, f := range fields {
			if strings.HasSuffix(strings.ToUpper(f), ".UPF") {
				var content []byte
				err := c.db.QueryRow(`SELECT content FROM pseudopotentials WHERE filename = ?`, f).Scan(&content)
				if err == nil && len(content) > 0 {
					cmBinary[f] = content
					log.Printf("[%s] Loaded pseudopotential %s from DB (%d bytes)", jobName, f, len(content))
				} else {
					log.Printf("[%s] Pseudopotential %s not in DB, will try download at runtime", jobName, f)
				}
			}
		}
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: cmName,
			Labels: map[string]string{
				"app":                  "qe-job",
				"qe.khemia.io/job-name": jobName,
			},
		},
		Data:       cmData,
		BinaryData: cmBinary,
	}
	if _, err := c.client.CoreV1().ConfigMaps(c.namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		c.failQEJob(jobName, fmt.Sprintf("failed to create input ConfigMap: %v", err))
		return
	}

	// Cleanup ConfigMap when done (regardless of success or failure).
	defer func() {
		if err := c.client.CoreV1().ConfigMaps(c.namespace).Delete(ctx, cmName, metav1.DeleteOptions{}); err != nil {
			log.Printf("[%s] Warning: failed to delete ConfigMap %s: %v", jobName, cmName, err)
		}
	}()

	// 3. Create the K8s Job.
	backoffLimit := int32(0)
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
			Labels: map[string]string{
				"app":                   "qe-job",
				"qe.khemia.io/job-name": jobName,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(600),
			BackoffLimit:            &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            "qe",
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/bin/sh", "-c"},
							Args: []string{
								fmt.Sprintf(`cd /scratch && cp /input/input.in . && \
for pp in $(grep -i '\.UPF' input.in | awk '{print $NF}'); do \
  [ -f "$pp" ] || wget -q "https://pseudopotentials.quantum-espresso.org/upf_files/$pp" -O "$pp" 2>/dev/null || \
  wget -q "https://www.quantum-espresso.org/upf_files/$pp" -O "$pp" 2>/dev/null || \
  echo "WARNING: could not download $pp"; \
done && \
mpirun --allow-run-as-root -np %d %s -in input.in 2>&1`,
									numCPUs, executable),
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", numCPUs)),
									corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", memoryMB)),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", numCPUs)),
									corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", memoryMB)),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "input", MountPath: "/input", ReadOnly: true},
								emptyDirMount("scratch", "/scratch"),
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "input",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
								},
							},
						},
						emptyDirVolume("scratch"),
					},
				},
			},
		},
	}

	if _, err := c.jobClient.Create(ctx, j, metav1.CreateOptions{}); err != nil {
		c.failQEJob(jobName, fmt.Sprintf("failed to create K8s Job: %v", err))
		return
	}

	// 4. Stream logs for observability.
	streamCtx, streamCancel := context.WithCancel(ctx)
	go c.streamJobLogs(streamCtx, jobName)

	// 5. Wait for completion with QE-appropriate timeout.
	if err := c.waitForQEJobCompletion(jobName); err != nil {
		streamCancel()
		c.failQEJob(jobName, fmt.Sprintf("job execution failed: %v", err))
		return
	}
	streamCancel()

	// 6. Capture output from pod logs.
	output, err := c.readPodLogs(jobName)
	if err != nil {
		c.failQEJob(jobName, fmt.Sprintf("failed to read pod logs: %v", err))
		return
	}

	// 7. Parse total energy and wall time from the QE output.
	totalEnergy := parseQETotalEnergy(output)
	wallTimeSec := parseQEWallTime(output)

	// 8. Store output and parsed values.
	if _, err := c.db.Exec(
		`UPDATE qe_jobs SET status='Completed', output_file=?, total_energy=?, wall_time_sec=?, completed_at=NOW() WHERE name=?`,
		output, totalEnergy, wallTimeSec, jobName); err != nil {
		log.Printf("[%s] CRITICAL: failed to store QE results: %v", jobName, err)
		return
	}

	log.Printf("[%s] QE job completed: total_energy=%v wall_time=%v", jobName, totalEnergy, wallTimeSec)
}

// waitForQEJobCompletion polls for job completion with a 4-hour timeout (QE jobs can be long).
func (c *DockingJobController) waitForQEJobCompletion(jobName string) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	timeout := time.After(time.Duration(DefaultQETimeoutH) * time.Hour)
	pollCount := 0

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout after %d hours waiting for job %s", DefaultQETimeoutH, jobName)
		case <-ticker.C:
			pollCount++
			job, err := c.jobClient.Get(context.TODO(), jobName, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					if pollCount <= 3 || pollCount%30 == 0 {
						log.Printf("[qe-wait] %s: not found yet (poll %d)", jobName, pollCount)
					}
					continue
				}
				return err
			}
			if pollCount%30 == 0 {
				log.Printf("[qe-wait] %s: active=%d succeeded=%d failed=%d (poll %d)",
					jobName, job.Status.Active, job.Status.Succeeded, job.Status.Failed, pollCount)
			}
			if job.Status.Succeeded > 0 {
				log.Printf("[qe-wait] %s: succeeded after %d polls", jobName, pollCount)
				return nil
			}
			if job.Status.Failed > 0 {
				return fmt.Errorf("job %s failed", jobName)
			}
		}
	}
}

// failQEJob marks a QE job as Failed and stores the error output.
func (c *DockingJobController) failQEJob(jobName, message string) {
	if _, err := c.db.Exec(
		`UPDATE qe_jobs SET status='Failed', error_output=?, completed_at=NOW() WHERE name=?`,
		message, jobName); err != nil {
		log.Printf("[%s] CRITICAL: failed to mark QE job as Failed: %v", jobName, err)
	}
	log.Printf("[%s] QE job failed: %s", jobName, message)
}

// parseQETotalEnergy extracts total energy from QE output.
// Looks for lines like: "!    total   energy              =     -32.44928392 Ry"
func parseQETotalEnergy(output string) *float64 {
	re := regexp.MustCompile(`!\s+total\s+energy\s+=\s+([-\d.]+)\s+Ry`)
	matches := re.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return nil
	}
	// Use the last match (final SCF iteration).
	lastMatch := matches[len(matches)-1]
	val, err := strconv.ParseFloat(lastMatch[1], 64)
	if err != nil {
		return nil
	}
	return &val
}

// parseQEWallTime extracts wall time in seconds from QE output.
// Looks for lines like: "     PWSCF        :     12.34s CPU     13.56s WALL"
// or multi-unit formats like: "     PWSCF        :   1h23m CPU   1h24m WALL"
func parseQEWallTime(output string) *float32 {
	// Try seconds format first: "XXXs WALL"
	reSeconds := regexp.MustCompile(`([\d.]+)s\s+WALL`)
	if matches := reSeconds.FindStringSubmatch(output); len(matches) > 1 {
		val, err := strconv.ParseFloat(matches[1], 32)
		if err == nil {
			result := float32(val)
			return &result
		}
	}

	// Try h/m/s format: "1h23m45.67s WALL" or "23m45.67s WALL"
	reHMS := regexp.MustCompile(`(?:(\d+)h)?(\d+)m([\d.]+)s\s+WALL`)
	if matches := reHMS.FindStringSubmatch(output); len(matches) > 0 {
		var totalSec float64
		if matches[1] != "" {
			h, _ := strconv.ParseFloat(matches[1], 64)
			totalSec += h * 3600
		}
		m, _ := strconv.ParseFloat(matches[2], 64)
		totalSec += m * 60
		s, _ := strconv.ParseFloat(matches[3], 64)
		totalSec += s
		result := float32(totalSec)
		return &result
	}

	return nil
}

// createPrepBatchJob creates a K8s Job that runs prep_ligands.py for a ligand batch.
func (c *DockingJobController) createPrepBatchJob(sourceDb, image string, batchIndex, offset, chunkSize int) error {
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("prep-%s-batch-%d", sourceDb, batchIndex),
			Labels: map[string]string{
				"docking.khemia.io/job-type":   "prep-ligands",
				"docking.khemia.io/parent-job": fmt.Sprintf("prep-%s", sourceDb),
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
							Name:            "prep",
							Image:           image,
							ImagePullPolicy: corev1.PullAlways,
							WorkingDir:      WorkDir,
							Command:         []string{"python3", "/autodock/scripts/prep_ligands.py"},
							Env: []corev1.EnvVar{
								{Name: "SOURCE_DB", Value: sourceDb},
								{Name: "BATCH_OFFSET", Value: fmt.Sprintf("%d", offset)},
								{Name: "BATCH_LIMIT", Value: fmt.Sprintf("%d", chunkSize)},
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

// streamJobLogs streams pod logs for a K8s Job to the controller's stdout in real-time.
// It waits for the job's pod to reach Running phase, then follows the log stream until the
// pod completes or the context is cancelled. This is best-effort observability — failures
// are logged as warnings and never block the pipeline.
func (c *DockingJobController) streamJobLogs(ctx context.Context, jobName string) {
	// Wait for a Running pod (poll every 2s, give up after 2 minutes).
	var podName string
	pollTimeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for podName == "" {
		select {
		case <-ctx.Done():
			return
		case <-pollTimeout:
			log.Printf("[stream] %s: timed out waiting for pod to start, giving up on log streaming", jobName)
			return
		case <-ticker.C:
			pods, err := c.client.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("job-name=%s", jobName),
			})
			if err != nil {
				log.Printf("[stream] %s: warning: failed to list pods: %v", jobName, err)
				continue
			}
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodSucceeded {
					podName = pod.Name
					break
				}
			}
		}
	}

	log.Printf("[stream] %s: streaming logs from pod %s", jobName, podName)

	stream, err := c.client.CoreV1().Pods(c.namespace).GetLogs(podName, &corev1.PodLogOptions{
		Follow: true,
	}).Stream(ctx)
	if err != nil {
		log.Printf("[stream] %s: warning: failed to open log stream: %v", jobName, err)
		return
	}
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		log.Printf("[%s] %s", jobName, scanner.Text())
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		log.Printf("[stream] %s: warning: log stream read error: %v", jobName, err)
	}
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
