// Package main provides the docking job controller with REST API
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

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
	DefaultAutodockPvc      = "pvc-autodock"
	DefaultUserPvcPrefix    = "claim-"
	DefaultMountPath        = "/data"
	DefaultLigandsChunkSize = 10000
	DefaultPDBID            = "7jrn"
	DefaultLigandDb         = "ChEBI_complete"
	DefaultJupyterUser      = "jovyan"
	DefaultNativeLigand     = "TTT"

	DockingJobFinalizer = "docking.khemia.io/finalizer"

)

// DockingJobController handles the lifecycle of DockingJob resources
type DockingJobController struct {
	client    *kubernetes.Clientset
	namespace string
	jobClient typed.JobInterface
	stopCh    chan struct{}
	// results caches postprocessing output (parent job name → "Best energy: X").
	// Populated immediately after the postprocessing job succeeds so the result
	// is readable even after the pod's 5-minute TTL expires.
	results sync.Map
}

// DockingJob represents the custom resource
type DockingJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DockingJobSpec   `json:"spec,omitempty"`
	Status            DockingJobStatus `json:"status,omitempty"`
}

type DockingJobSpec struct {
	PDBID            string `json:"pdbid,omitempty"`
	LigandDb         string `json:"ligandDb,omitempty"`
	JupyterUser      string `json:"jupyterUser,omitempty"`
	NativeLigand     string `json:"nativeLigand,omitempty"`
	LigandsChunkSize int    `json:"ligandsChunkSize,omitempty"`
	Image            string `json:"image,omitempty"`
	AutodockPvc      string `json:"autodockPvc,omitempty"`
	UserPvcPrefix    string `json:"userPvcPrefix,omitempty"`
	MountPath        string `json:"mountPath,omitempty"`
}

type DockingJobStatus struct {
	Phase            string     `json:"phase,omitempty"`
	BatchCount       int        `json:"batchCount,omitempty"`
	CompletedBatches int        `json:"completedBatches,omitempty"`
	Message          string     `json:"message,omitempty"`
	StartTime        *time.Time `json:"startTime,omitempty"`
	CompletionTime   *time.Time `json:"completionTime,omitempty"`
}

type DockingJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DockingJob `json:"items"`
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

	return &DockingJobController{
		client:    client,
		namespace: namespace,
		jobClient: client.BatchV1().Jobs(namespace),
		stopCh:    make(chan struct{}),
	}, nil
}

func getConfig() (*rest.Config, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

// Run starts the controller
func (c *DockingJobController) Run(ctx context.Context) error {
	log.Println("Starting Docking Job Controller...")

	go func() {
		if err := c.startAPIServer(); err != nil && err != http.ErrServerClosed {
			log.Printf("API server error: %v", err)
		}
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.reconcileJobs(); err != nil {
				log.Printf("Reconciliation error: %v", err)
			}
		}
	}
}

func (c *DockingJobController) startAPIServer() error {
	handler := NewAPIHandler(c.client, c.namespace, c)

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

func (c *DockingJobController) processDockingJob(job DockingJob) {
	log.Printf("[%s] Starting pipeline: pdbid=%s ligand_db=%s image=%s chunk_size=%d",
		job.Name, job.Spec.PDBID, job.Spec.LigandDb, job.Spec.Image, job.Spec.LigandsChunkSize)

	now := time.Now()
	job.Status.Phase = "Running"
	job.Status.StartTime = &now

	log.Printf("[%s] Step 1/5: copy-ligand-db", job.Name)
	if err := c.createCopyLigandDbJob(job); err != nil {
		c.failJob(job, fmt.Sprintf("copy ligand DB failed: %v", err))
		return
	}

	log.Printf("[%s] Step 2/5: prepare-receptor (concurrent with split-sdf)", job.Name)
	if err := c.createPrepareReceptorJob(job); err != nil {
		c.failJob(job, fmt.Sprintf("prepare receptor failed: %v", err))
		return
	}

	log.Printf("[%s] Step 3/5: split-sdf", job.Name)
	batchCount, err := c.createSplitSdfJob(job)
	if err != nil {
		c.failJob(job, fmt.Sprintf("split SDF failed: %v", err))
		return
	}

	log.Printf("[%s] split-sdf complete: %d batches", job.Name, batchCount)
	// prepare-receptor was started concurrently with split-sdf (both are
	// independent of each other). Ensure it has finished before docking
	// starts, since docking needs the PDBQT receptor file on the PVC.
	receptorJobName := fmt.Sprintf("%s-prepare-receptor", job.Name)
	log.Printf("[%s] Waiting for prepare-receptor to finish...", job.Name)
	if err := c.waitForJobCompletion(receptorJobName); err != nil {
		c.failJob(job, fmt.Sprintf("prepare receptor failed: %v", err))
		return
	}

	job.Status.BatchCount = batchCount
	log.Printf("[%s] Step 4/5: processing %d batch(es)", job.Name, batchCount)

	for i := 0; i < batchCount; i++ {
		// batchLabel is the filesystem label used in script args and filenames.
		// k8sLabel is batchLabel with underscores replaced by hyphens so it is
		// valid in Kubernetes resource names (RFC 1123 subdomain).
		batchLabel := fmt.Sprintf("%s_batch%d", job.Spec.LigandDb, i)
		k8sLabel := strings.ReplaceAll(batchLabel, "_", "-")
		log.Printf("[%s] Batch %d/%d: prepare-ligands batchLabel=%s", job.Name, i+1, batchCount, batchLabel)

		if err := c.createPrepareLigandsJob(job, batchLabel, k8sLabel); err != nil {
			c.failJob(job, fmt.Sprintf("prepare ligands batch %d failed: %v", i, err))
			return
		}
		prepareName := fmt.Sprintf("%s-prepare-ligands-%s", job.Name, k8sLabel)
		if err := c.waitForJobCompletion(prepareName); err != nil {
			c.failJob(job, fmt.Sprintf("prepare ligands batch %d failed: %v", i, err))
			return
		}

		log.Printf("[%s] Batch %d/%d: docking pdbid=%s batchLabel=%s", job.Name, i+1, batchCount, job.Spec.PDBID, batchLabel)
		if err := c.createDockingJobExecution(job, batchLabel, k8sLabel); err != nil {
			c.failJob(job, fmt.Sprintf("docking batch %d failed: %v", i, err))
			return
		}
		dockingName := fmt.Sprintf("%s-docking-%s", job.Name, k8sLabel)
		if err := c.waitForJobCompletion(dockingName); err != nil {
			c.failJob(job, fmt.Sprintf("docking batch %d failed: %v", i, err))
			return
		}

		job.Status.CompletedBatches = i + 1
		log.Printf("[%s] Batch %d/%d complete", job.Name, i+1, batchCount)
	}

	log.Printf("[%s] Step 5/6: postprocessing", job.Name)
	if err := c.createPostProcessingJob(job); err != nil {
		c.failJob(job, fmt.Sprintf("postprocessing failed: %v", err))
		return
	}
	postprocessingName := fmt.Sprintf("%s-postprocessing", job.Name)
	if err := c.waitForJobCompletion(postprocessingName); err != nil {
		c.failJob(job, fmt.Sprintf("postprocessing failed: %v", err))
		return
	}

	log.Printf("[%s] Postprocessing complete — capturing result", job.Name)
	result := c.captureResult(postprocessingName)
	c.results.Store(job.Name, result)
	log.Printf("[%s] Cached result: %q", job.Name, result)

	log.Printf("[%s] Step 6/6: export results to MySQL", job.Name)
	if err := c.createMySQLExportJob(job); err != nil {
		c.failJob(job, fmt.Sprintf("mysql export failed: %v", err))
		return
	}
	mysqlExportName := fmt.Sprintf("%s-export-mysql", job.Name)
	if err := c.waitForJobCompletion(mysqlExportName); err != nil {
		c.failJob(job, fmt.Sprintf("mysql export failed: %v", err))
		return
	}
	log.Printf("[%s] MySQL export complete", job.Name)

	completionTime := time.Now()
	job.Status.Phase = "Completed"
	job.Status.CompletionTime = &completionTime
	job.Status.Message = "All steps completed successfully"

	log.Printf("Docking job %s completed successfully", job.Name)
}

func (c *DockingJobController) failJob(job DockingJob, message string) {
	job.Status.Phase = "Failed"
	job.Status.Message = message
	log.Printf("Docking job %s failed: %s", job.Name, message)
}

func (c *DockingJobController) createCopyLigandDbJob(job DockingJob) error {
	userPvcPath := fmt.Sprintf("%s%s", job.Spec.UserPvcPrefix, job.Spec.JupyterUser)

	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-copy-ligand", job.Name),
			Labels: map[string]string{
				"docking.khemia.io/workflow":   job.Name,
				"docking.khemia.io/job-type":   "copy-ligand-db",
				"docking.khemia.io/parent-job": job.Name,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(300),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:  "copy",
							Image: "alpine:latest",
							Command: []string{"/bin/sh", "-c"},
							Args: []string{
								fmt.Sprintf("cp %s/%s/%s.sdf %s/%s.sdf",
									job.Spec.MountPath, userPvcPath, job.Spec.LigandDb,
									job.Spec.MountPath, job.Spec.LigandDb),
							},
							VolumeMounts: []corev1.VolumeMount{pvcMount("autodock-pvc", job.Spec.MountPath)},
						},
					},
					Volumes: []corev1.Volume{pvcVolume("autodock-pvc", job.Spec.AutodockPvc)},
				},
			},
		},
	}

	created, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return c.waitForJobCompletion(created.Name)
}

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
					RestartPolicy: corev1.RestartPolicyOnFailure,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{
						{
							Name:            "prepare",
							Image:           job.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							WorkingDir:      job.Spec.MountPath,
							Command:         []string{"python3", "/autodock/scripts/proteinprepv2.py"},
							Args: []string{
								"--protein_id", job.Spec.PDBID,
								"--ligand_id", job.Spec.NativeLigand,
							},
							VolumeMounts: []corev1.VolumeMount{pvcMount("autodock-pvc", job.Spec.MountPath)},
						},
					},
					Volumes: []corev1.Volume{pvcVolume("autodock-pvc", job.Spec.AutodockPvc)},
				},
			},
		},
	}

	_, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	return err
}

func (c *DockingJobController) createSplitSdfJob(job DockingJob) (int, error) {
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-split-sdf", job.Name),
			Labels: map[string]string{
				"docking.khemia.io/workflow":   job.Name,
				"docking.khemia.io/job-type":   "split-sdf",
				"docking.khemia.io/parent-job": job.Name,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(300),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{
						{
							Name:            "split",
							Image:           job.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							WorkingDir:      job.Spec.MountPath,
							Command:         []string{"/bin/sh", "-c"},
							Args: []string{
								fmt.Sprintf("/autodock/scripts/split_sdf.sh %d %s",
									job.Spec.LigandsChunkSize, job.Spec.LigandDb),
							},
							Env: []corev1.EnvVar{
								{Name: "MOUNT_PATH_AUTODOCK", Value: job.Spec.MountPath},
							},
							VolumeMounts: []corev1.VolumeMount{pvcMount("autodock-pvc", job.Spec.MountPath)},
						},
					},
					Volumes: []corev1.Volume{pvcVolume("autodock-pvc", job.Spec.AutodockPvc)},
				},
			},
		},
	}

	created, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	if err != nil {
		return 0, err
	}

	if err := c.waitForJobCompletion(created.Name); err != nil {
		return 0, err
	}

	return c.parseBatchCountFromLogs(created.Name)
}

func (c *DockingJobController) createPrepareLigandsJob(job DockingJob, batchLabel, k8sLabel string) error {
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-prepare-ligands-%s", job.Name, k8sLabel),
			Labels: map[string]string{
				"docking.khemia.io/workflow":   job.Name,
				"docking.khemia.io/job-type":   "prepare-ligands",
				"docking.khemia.io/batch":      k8sLabel,
				"docking.khemia.io/parent-job": job.Name,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(300),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{
						{
							Name:            "prepare",
							Image:           job.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							WorkingDir:      job.Spec.MountPath,
							Command:         []string{"python3", "/autodock/scripts/ligandprepv2.py"},
							Args: []string{
								fmt.Sprintf("%s.sdf", batchLabel),
								batchLabel,
								"--format", "pdb",
							},
							Env: []corev1.EnvVar{
								{Name: "MOUNT_PATH_AUTODOCK", Value: job.Spec.MountPath},
							},
							VolumeMounts: []corev1.VolumeMount{pvcMount("autodock-pvc", job.Spec.MountPath)},
						},
					},
					Volumes: []corev1.Volume{pvcVolume("autodock-pvc", job.Spec.AutodockPvc)},
				},
			},
		},
	}

	_, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	return err
}

func (c *DockingJobController) createDockingJobExecution(job DockingJob, batchLabel, k8sLabel string) error {
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-docking-%s", job.Name, k8sLabel),
			Labels: map[string]string{
				"docking.khemia.io/workflow":   job.Name,
				"docking.khemia.io/job-type":   "docking",
				"docking.khemia.io/batch":      k8sLabel,
				"docking.khemia.io/parent-job": job.Name,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(300),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{
						{
							Name:            "docking",
							Image:           job.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							WorkingDir:      job.Spec.MountPath,
							Command:         []string{"python3", "/autodock/scripts/dockingv2.py"},
							Args: []string{job.Spec.PDBID, batchLabel},
							VolumeMounts: []corev1.VolumeMount{pvcMount("autodock-pvc", job.Spec.MountPath)},
						},
					},
					Volumes: []corev1.Volume{pvcVolume("autodock-pvc", job.Spec.AutodockPvc)},
				},
			},
		},
	}

	_, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	return err
}

func (c *DockingJobController) createPostProcessingJob(job DockingJob) error {
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-postprocessing", job.Name),
			Labels: map[string]string{
				"docking.khemia.io/workflow":   job.Name,
				"docking.khemia.io/job-type":   "postprocessing",
				"docking.khemia.io/parent-job": job.Name,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(300),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{
						{
							Name:            "postprocess",
							Image:           job.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							WorkingDir:      job.Spec.MountPath,
							Command:         []string{"/autodock/scripts/3_post_processing.sh"},
							Args:            []string{job.Spec.PDBID, job.Spec.LigandDb},
							VolumeMounts:    []corev1.VolumeMount{pvcMount("autodock-pvc", job.Spec.MountPath)},
						},
					},
					Volumes: []corev1.Volume{pvcVolume("autodock-pvc", job.Spec.AutodockPvc)},
				},
			},
		},
	}

	_, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	return err
}

func (c *DockingJobController) createMySQLExportJob(job DockingJob) error {
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-export-mysql", job.Name),
			Labels: map[string]string{
				"docking.khemia.io/workflow":   job.Name,
				"docking.khemia.io/job-type":   "export-mysql",
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
							Name:            "export",
							Image:           job.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							WorkingDir:      job.Spec.MountPath,
							Command:         []string{"python3", "/autodock/scripts/export_energies_mysql.py"},
							Args: []string{
								"--workflow", job.Name,
								"--pdbid", job.Spec.PDBID,
								"--db-label", job.Spec.LigandDb,
								"--base-dir", job.Spec.MountPath,
							},
							Env: []corev1.EnvVar{
								{Name: "MYSQL_HOST", Value: "docking-mysql.chem.svc.cluster.local"},
								{Name: "MYSQL_PORT", Value: "3306"},
								{Name: "MYSQL_USER", Value: "root"},
								{Name: "MYSQL_DATABASE", Value: "docking"},
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
							VolumeMounts: []corev1.VolumeMount{pvcMount("autodock-pvc", job.Spec.MountPath)},
						},
					},
					Volumes: []corev1.Volume{pvcVolume("autodock-pvc", job.Spec.AutodockPvc)},
				},
			},
		},
	}

	_, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	return err
}

// captureResult reads the postprocessing pod's logs immediately after the job
// succeeds and returns the "Best energy: ..." line (or the full log on fallback).
// Called from processDockingJob while the pod is guaranteed fresh.
func (c *DockingJobController) captureResult(postprocessingJobName string) string {
	pods, err := c.client.CoreV1().Pods(c.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", postprocessingJobName),
	})
	if err != nil || len(pods.Items) == 0 {
		log.Printf("[captureResult] no pods found for job %s: err=%v", postprocessingJobName, err)
		return ""
	}
	podName := pods.Items[0].Name
	log.Printf("[captureResult] reading logs from pod %s", podName)
	stream, err := c.client.CoreV1().Pods(c.namespace).GetLogs(podName, &corev1.PodLogOptions{}).
		Stream(context.TODO())
	if err != nil {
		log.Printf("[captureResult] GetLogs error for pod %s: %v", podName, err)
		return ""
	}
	defer stream.Close()
	buf, err := io.ReadAll(stream)
	if err != nil {
		log.Printf("[captureResult] ReadAll error for pod %s: %v", podName, err)
		return ""
	}
	log.Printf("[captureResult] pod %s logs (%d bytes):\n%s", podName, len(buf), strings.TrimSpace(string(buf)))
	for _, line := range strings.Split(strings.TrimSpace(string(buf)), "\n") {
		if strings.HasPrefix(line, "Best energy:") {
			log.Printf("[captureResult] found result: %s", line)
			return strings.TrimSpace(line)
		}
	}
	// No "Best energy:" line found — return trimmed full output for diagnostics.
	return strings.TrimSpace(string(buf))
}

// parseBatchCountFromLogs reads the completed split-sdf pod's logs and parses
// the batch count printed by split_sdf.sh as its last stdout line.
func (c *DockingJobController) parseBatchCountFromLogs(jobName string) (int, error) {
	pods, err := c.client.CoreV1().Pods(c.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil {
		return 0, fmt.Errorf("listing pods for job %s: %w", jobName, err)
	}
	if len(pods.Items) == 0 {
		return 0, fmt.Errorf("no pods found for job %s", jobName)
	}

	podName := pods.Items[0].Name
	req := c.client.CoreV1().Pods(c.namespace).GetLogs(podName, &corev1.PodLogOptions{})
	stream, err := req.Stream(context.TODO())
	if err != nil {
		return 0, fmt.Errorf("getting logs for pod %s: %w", podName, err)
	}
	defer stream.Close()

	buf, err := io.ReadAll(stream)
	if err != nil {
		return 0, fmt.Errorf("reading logs from pod %s: %w", podName, err)
	}

	lines := strings.Split(strings.TrimSpace(string(buf)), "\n")
	if len(lines) == 0 {
		return 0, fmt.Errorf("empty logs from split-sdf pod %s", podName)
	}

	lastLine := strings.TrimSpace(lines[len(lines)-1])
	count, err := strconv.Atoi(lastLine)
	if err != nil {
		return 0, fmt.Errorf("parsing batch count (last log line %q): %w", lastLine, err)
	}
	if count <= 0 {
		return 0, fmt.Errorf("invalid batch count %d from split-sdf job %s", count, jobName)
	}

	return count, nil
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

func (c *DockingJobController) reconcileJobs() error {
	return nil
}

// pvcVolume returns a corev1.Volume backed by a PVC
func pvcVolume(name, claimName string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: claimName,
			},
		},
	}
}

// pvcMount returns a corev1.VolumeMount
func pvcMount(name, mountPath string) corev1.VolumeMount {
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
