// Package main provides the docking job controller with REST API
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	DefaultImage            = "hwcopeland/auto-docker:latest"
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
			if hasLogsSuffix(r.URL.Path) {
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

func (c *DockingJobController) processDockingJob(job DockingJob) {
	log.Printf("Processing docking job: %s", job.Name)

	now := time.Now()
	job.Status.Phase = "Running"
	job.Status.StartTime = &now

	if err := c.createCopyLigandDbJob(job); err != nil {
		c.failJob(job, fmt.Sprintf("copy ligand DB failed: %v", err))
		return
	}

	if err := c.createPrepareReceptorJob(job); err != nil {
		c.failJob(job, fmt.Sprintf("prepare receptor failed: %v", err))
		return
	}

	batchCount, err := c.createSplitSdfJob(job)
	if err != nil {
		c.failJob(job, fmt.Sprintf("split SDF failed: %v", err))
		return
	}

	job.Status.BatchCount = batchCount

	for i := 0; i < batchCount; i++ {
		batchLabel := fmt.Sprintf("%s_batch%d", job.Spec.LigandDb, i)

		if err := c.createPrepareLigandsJob(job, batchLabel); err != nil {
			c.failJob(job, fmt.Sprintf("prepare ligands batch %d failed: %v", i, err))
			return
		}

		if err := c.createDockingJobExecution(job, batchLabel); err != nil {
			c.failJob(job, fmt.Sprintf("docking batch %d failed: %v", i, err))
			return
		}

		job.Status.CompletedBatches = i + 1
	}

	if err := c.createPostProcessingJob(job); err != nil {
		c.failJob(job, fmt.Sprintf("postprocessing failed: %v", err))
		return
	}

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

	_, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	return err
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

	return 5, nil
}

func (c *DockingJobController) createPrepareLigandsJob(job DockingJob, batchLabel string) error {
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-prepare-ligands-%s", job.Name, batchLabel),
			Labels: map[string]string{
				"docking.khemia.io/workflow":   job.Name,
				"docking.khemia.io/job-type":   "prepare-ligands",
				"docking.khemia.io/batch":      batchLabel,
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
							Name:            "prepare",
							Image:           job.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							WorkingDir:      job.Spec.MountPath,
							Command:         []string{"python3", "/autodock/scripts/ligandprepv2.py"},
							Args: []string{
								fmt.Sprintf("%s.sdf", batchLabel),
								fmt.Sprintf("%s/output", job.Spec.MountPath),
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

func (c *DockingJobController) createDockingJobExecution(job DockingJob, batchLabel string) error {
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-docking-%s", job.Name, batchLabel),
			Labels: map[string]string{
				"docking.khemia.io/workflow":   job.Name,
				"docking.khemia.io/job-type":   "docking",
				"docking.khemia.io/batch":      batchLabel,
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
							Name:            "docking",
							Image:           job.Spec.Image,
							ImagePullPolicy: corev1.PullAlways,
							WorkingDir:      job.Spec.MountPath,
							Command:         []string{"/bin/sh", "-c"},
							Args: []string{
								fmt.Sprintf("/autodock/scripts/dockingv2.sh %s %s",
									job.Spec.PDBID, batchLabel),
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

func (c *DockingJobController) waitForJobCompletion(jobName string) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	timeout := time.After(10 * time.Minute)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for job %s", jobName)
		case <-ticker.C:
			job, err := c.jobClient.Get(context.TODO(), jobName, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					continue
				}
				return err
			}
			if job.Status.Succeeded > 0 {
				return nil
			}
			if job.Status.Failed > 0 {
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
