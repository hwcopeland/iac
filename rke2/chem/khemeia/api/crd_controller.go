// Package main provides the CRD-based job controller that watches Khemeia CRD
// instances (TargetPrep, DockJob, RefineJob, ADMETJob, etc.) and drives their
// lifecycle: create K8s Jobs, monitor completion, update CRD status, handle retries.
//
// This controller runs alongside the existing plugin-based job_runner.go. Both
// coexist — the plugin path handles PluginSubmit -> RunPluginJob, while this
// controller handles CRD instances created via the advance API or kubectl.
//
// Uses k8s.io/client-go/dynamic (no code-gen) for unstructured CRD access.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	typed "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/tools/cache"
	"gopkg.in/yaml.v3"
)

// crdResyncPeriod is the informer resync interval for CRD watches.
const crdResyncPeriod = 30 * time.Second

// khemeiaGroup is the API group for all Khemeia CRDs.
const khemeiaGroup = "khemeia.io"

// khemeiaVersion is the current CRD API version.
const khemeiaVersion = "v1alpha1"

// registeredCRDs maps CRD kind names to their GroupVersionResource.
// New CRD kinds are added here to register them with the controller.
var registeredCRDs = map[string]schema.GroupVersionResource{
	"TargetPrep":  {Group: khemeiaGroup, Version: khemeiaVersion, Resource: "targetpreps"},
	"LibraryPrep": {Group: khemeiaGroup, Version: khemeiaVersion, Resource: "librarypreps"},
	"DockJob":     {Group: khemeiaGroup, Version: khemeiaVersion, Resource: "dockjobs"},
	"RefineJob":   {Group: khemeiaGroup, Version: khemeiaVersion, Resource: "refinejobs"},
	"ADMETJob":    {Group: khemeiaGroup, Version: khemeiaVersion, Resource: "admetjobs"},
}

// defaultComputeClassForKind maps CRD kinds to their default compute class.
// Overridable in the CRD spec via the computeClass field.
var defaultComputeClassForKind = map[string]string{
	"TargetPrep":          "cpu",
	"LibraryPrep":         "cpu",
	"DockJob":             "gpu",
	"RefineJob":           "gpu",
	"ADMETJob":            "cpu",
	"GenerateJob":         "gpu",
	"LinkJob":             "gpu",
	"SelectivityJob":      "cpu-high-mem",
	"RBFEJob":             "gpu",
	"ABFEJob":             "gpu",
	"IngestStructureJob":  "cpu",
	"ReportJob":           "cpu",
}

// crdImageMapping maps CRD kinds to their container image.
var crdImageMapping = map[string]string{
	"TargetPrep":  "zot.hwcopeland.net/chem/target-prep:latest",
	"LibraryPrep": "zot.hwcopeland.net/chem/library-prep:latest",
	"DockJob":     "zot.hwcopeland.net/chem/vina:1.2",
	"RefineJob":   "zot.hwcopeland.net/chem/refine:latest",
	"ADMETJob":    "zot.hwcopeland.net/chem/admet:latest",
}

// ComputeClass defines the scheduling and resource configuration for a job.
// Loaded from the khemeia-compute-classes ConfigMap.
type ComputeClass struct {
	Description  string                `yaml:"description"`
	Resources    map[string]string     `yaml:"resources"`
	NodeSelector map[string]string     `yaml:"nodeSelector"`
	Tolerations  []corev1.Toleration   `yaml:"tolerations"`
	Volumes      []corev1.Volume       `yaml:"volumes"`
	VolumeMounts []corev1.VolumeMount  `yaml:"volumeMounts"`
	Env          []corev1.EnvVar       `yaml:"env"`
}

// CRDController watches Khemeia CRD instances and drives their lifecycle.
type CRDController struct {
	dynamicClient  dynamic.Interface
	kubeClient     kubernetes.Interface
	jobClient      typed.JobInterface
	s3Client       S3Client
	db             *sql.DB
	namespace      string
	computeClasses map[string]ComputeClass
	stopCh         chan struct{}
	mu             sync.Mutex
}

// NewCRDController creates a CRD controller using the given clients.
// The db parameter is used for provenance queries; s3 is for artifact storage.
func NewCRDController(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, namespace string, db *sql.DB, s3 S3Client) *CRDController {
	return &CRDController{
		dynamicClient:  dynamicClient,
		kubeClient:     kubeClient,
		jobClient:      kubeClient.BatchV1().Jobs(namespace),
		s3Client:       s3,
		db:             db,
		namespace:      namespace,
		computeClasses: make(map[string]ComputeClass),
		stopCh:         make(chan struct{}),
	}
}

// Start loads compute classes, sets up dynamic informers for each registered
// CRD kind, and begins watching for CRD events. Blocks until ctx is cancelled.
func (c *CRDController) Start(ctx context.Context) {
	// Load compute classes from ConfigMap.
	if err := c.loadComputeClasses(); err != nil {
		log.Printf("[crd-controller] Warning: failed to load compute classes: %v (using defaults)", err)
	} else {
		log.Printf("[crd-controller] Loaded %d compute classes", len(c.computeClasses))
	}

	// Create a shared informer factory scoped to our namespace.
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		c.dynamicClient, crdResyncPeriod, c.namespace, nil,
	)

	for kind, gvr := range registeredCRDs {
		informer := factory.ForResource(gvr)
		informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onAdd,
			UpdateFunc: c.onUpdate,
		})
		log.Printf("[crd-controller] Registered informer for %s (%s)", kind, gvr.Resource)
	}

	// Start all informers and block until context cancellation.
	factory.Start(ctx.Done())

	// Wait for cache sync before processing events.
	factory.WaitForCacheSync(ctx.Done())
	log.Println("[crd-controller] All informer caches synced, controller running")

	<-ctx.Done()
	log.Println("[crd-controller] Context cancelled, stopping")
}

// Stop signals the controller to shut down.
func (c *CRDController) Stop() {
	close(c.stopCh)
}

// loadComputeClasses reads the khemeia-compute-classes ConfigMap and parses it.
func (c *CRDController) loadComputeClasses() error {
	cm, err := c.kubeClient.CoreV1().ConfigMaps(c.namespace).Get(
		context.Background(), "khemeia-compute-classes", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get compute classes ConfigMap: %w", err)
	}

	data, ok := cm.Data["classes.yaml"]
	if !ok {
		return fmt.Errorf("compute classes ConfigMap missing 'classes.yaml' key")
	}

	var config struct {
		Classes map[string]ComputeClass `yaml:"classes"`
	}
	if err := yaml.Unmarshal([]byte(data), &config); err != nil {
		return fmt.Errorf("failed to parse compute classes: %w", err)
	}

	c.mu.Lock()
	c.computeClasses = config.Classes
	c.mu.Unlock()

	return nil
}

// onAdd handles new CRD instances. If the instance is Pending with gate:auto,
// it creates a K8s Job immediately.
func (c *CRDController) onAdd(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	c.reconcile(u)
}

// onUpdate handles CRD instance updates. Re-evaluates the lifecycle state.
func (c *CRDController) onUpdate(oldObj, newObj interface{}) {
	u, ok := newObj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	c.reconcile(u)
}

// reconcile is the main reconciliation logic for a CRD instance.
func (c *CRDController) reconcile(u *unstructured.Unstructured) {
	kind := u.GetKind()
	name := u.GetName()
	phase := getPhase(u)

	switch phase {
	case "Pending":
		gate := getSpecString(u, "gate")
		if gate == "auto" {
			// Check dependency readiness (parentJob must be Succeeded if present).
			if !c.areDependenciesReady(u) {
				log.Printf("[crd-controller] %s/%s: Pending (auto-gate), dependencies not ready", kind, name)
				return
			}
			log.Printf("[crd-controller] %s/%s: Pending -> creating K8s Job (auto-gate)", kind, name)
			c.createAndTrackJob(u)
		}
		// gate:manual — do nothing, wait for advance API

	case "Running":
		// Monitor the K8s Job status.
		c.monitorRunningJob(u)

	case "Failed":
		// Check if retry is possible.
		retryCount := getStatusInt(u, "retryCount")
		maxRetries := getSpecInt(u, "maxRetries", 3)
		retryPolicy := getSpecString(u, "retryPolicy")

		if retryPolicy == "never" {
			return
		}
		if retryCount < maxRetries {
			log.Printf("[crd-controller] %s/%s: Failed, retrying (%d/%d)", kind, name, retryCount+1, maxRetries)
			c.retryJob(u, retryCount)
		}

	case "Succeeded":
		// No action needed.
	}
}

// areDependenciesReady checks if the parentJob (if any) is in Succeeded phase.
func (c *CRDController) areDependenciesReady(u *unstructured.Unstructured) bool {
	spec, ok := u.Object["spec"].(map[string]interface{})
	if !ok {
		return true
	}

	parentJob, ok := spec["parentJob"].(map[string]interface{})
	if !ok {
		// No parent dependency.
		return true
	}

	parentKind, _ := parentJob["kind"].(string)
	parentName, _ := parentJob["name"].(string)
	if parentKind == "" || parentName == "" {
		return true
	}

	gvr, ok := registeredCRDs[parentKind]
	if !ok {
		log.Printf("[crd-controller] Unknown parent kind %q, treating as ready", parentKind)
		return true
	}

	parent, err := c.dynamicClient.Resource(gvr).Namespace(c.namespace).Get(
		context.Background(), parentName, metav1.GetOptions{})
	if err != nil {
		log.Printf("[crd-controller] Failed to get parent %s/%s: %v", parentKind, parentName, err)
		return false
	}

	return getPhase(parent) == "Succeeded"
}

// createAndTrackJob creates a K8s Job for the CRD instance and updates its status to Running.
func (c *CRDController) createAndTrackJob(u *unstructured.Unstructured) {
	kind := u.GetKind()
	name := u.GetName()

	job, err := c.buildJobForCRD(u)
	if err != nil {
		log.Printf("[crd-controller] %s/%s: failed to build job spec: %v", kind, name, err)
		c.updateCRDStatus(u, "Failed", fmt.Sprintf("failed to build job spec: %v", err))
		return
	}

	_, err = c.jobClient.Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		log.Printf("[crd-controller] %s/%s: failed to create K8s Job: %v", kind, name, err)
		c.updateCRDStatus(u, "Failed", fmt.Sprintf("failed to create K8s Job: %v", err))
		return
	}

	log.Printf("[crd-controller] %s/%s: K8s Job %s created", kind, name, job.Name)
	c.updateCRDStatus(u, "Running", "K8s Job created")
}

// monitorRunningJob checks the K8s Job status for a Running CRD instance.
func (c *CRDController) monitorRunningJob(u *unstructured.Unstructured) {
	kind := u.GetKind()
	name := u.GetName()
	jobName := crdJobName(kind, name)

	job, err := c.jobClient.Get(context.Background(), jobName, metav1.GetOptions{})
	if err != nil {
		// Job may not exist yet (just created), or was cleaned up.
		return
	}

	if job.Status.Succeeded > 0 {
		log.Printf("[crd-controller] %s/%s: K8s Job succeeded", kind, name)
		c.updateCRDStatus(u, "Succeeded", "K8s Job completed successfully")
	} else if job.Status.Failed > 0 {
		log.Printf("[crd-controller] %s/%s: K8s Job failed", kind, name)
		c.updateCRDStatus(u, "Failed", "K8s Job failed")
	}
	// If still active, do nothing — wait for next reconciliation.
}

// retryJob increments the retry count, deletes the failed Job, and creates a new one.
func (c *CRDController) retryJob(u *unstructured.Unstructured, currentRetryCount int32) {
	kind := u.GetKind()
	name := u.GetName()
	jobName := crdJobName(kind, name)

	// Delete the old failed Job.
	propagation := metav1.DeletePropagationBackground
	_ = c.jobClient.Delete(context.Background(), jobName, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})

	// Update retry count in status before recreating.
	c.updateCRDStatusRetry(u, currentRetryCount+1,
		fmt.Sprintf("Retry %d/%d: recreating K8s Job after failure",
			currentRetryCount+1, getSpecInt(u, "maxRetries", 3)))

	// Create a new Job.
	c.createAndTrackJob(u)
}

// buildJobForCRD constructs a batchv1.Job from the CRD instance's spec.
func (c *CRDController) buildJobForCRD(u *unstructured.Unstructured) (*batchv1.Job, error) {
	kind := u.GetKind()
	name := u.GetName()
	spec := getSpec(u)

	// Resolve container image.
	image, ok := crdImageMapping[kind]
	if !ok {
		return nil, fmt.Errorf("no image mapping for CRD kind %q", kind)
	}

	// Resolve compute class.
	className := getSpecString(u, "computeClass")
	if className == "" {
		className = defaultComputeClassForKind[kind]
	}
	if className == "" {
		className = "cpu" // ultimate fallback
	}

	c.mu.Lock()
	cc, classFound := c.computeClasses[className]
	c.mu.Unlock()

	// Build resource requirements.
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}

	// Build the base Job.
	backoffLimit := int32(0)
	ttl := int32(600)
	jobName := crdJobName(kind, name)

	// Build environment variables.
	env := c.buildCRDJobEnv(kind, name, spec)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
			Labels: map[string]string{
				"app":                   "khemeia-crd-job",
				"khemeia.io/crd-kind":   kind,
				"khemeia.io/crd-name":   name,
				"khemeia.io/managed-by": "crd-controller",
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			BackoffLimit:            &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"khemeia.io/crd-kind": kind,
						"khemeia.io/crd-name": name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{
						{
							Name:            strings.ToLower(kind),
							Image:           image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env:             env,
							Resources:       resources,
							VolumeMounts: []corev1.VolumeMount{
								emptyDirMount("scratch", "/scratch"),
							},
						},
					},
					Volumes: []corev1.Volume{
						emptyDirVolume("scratch"),
					},
				},
			},
		},
	}

	// Apply compute class overrides if available.
	if classFound {
		if err := c.applyComputeClass(&job.Spec.Template.Spec, cc); err != nil {
			return nil, fmt.Errorf("failed to apply compute class %q: %w", className, err)
		}
	}

	return job, nil
}

// applyComputeClass merges compute class configuration into the pod spec.
func (c *CRDController) applyComputeClass(podSpec *corev1.PodSpec, cc ComputeClass) error {
	if len(podSpec.Containers) == 0 {
		return fmt.Errorf("pod spec has no containers")
	}
	container := &podSpec.Containers[0]

	// Resources.
	resList := parseResourceList(cc.Resources)
	container.Resources.Requests = resList
	container.Resources.Limits = resList

	// Node selector.
	if len(cc.NodeSelector) > 0 {
		podSpec.NodeSelector = cc.NodeSelector
	}

	// Tolerations.
	podSpec.Tolerations = append(podSpec.Tolerations, cc.Tolerations...)

	// Volumes and mounts (GPU host paths, etc.).
	podSpec.Volumes = append(podSpec.Volumes, cc.Volumes...)
	container.VolumeMounts = append(container.VolumeMounts, cc.VolumeMounts...)

	// Environment (LD_LIBRARY_PATH for GPU, etc.).
	container.Env = append(container.Env, cc.Env...)

	return nil
}

// parseResourceList converts a map of resource strings to a ResourceList.
func parseResourceList(resources map[string]string) corev1.ResourceList {
	rl := corev1.ResourceList{}
	for k, v := range resources {
		rl[corev1.ResourceName(k)] = resource.MustParse(v)
	}
	return rl
}

// buildCRDJobEnv creates environment variables for CRD-spawned job containers.
// Injects MySQL and Garage credentials so job containers can write artifacts
// and provenance records.
func (c *CRDController) buildCRDJobEnv(kind, name string, spec map[string]interface{}) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{Name: "JOB_NAME", Value: name},
		{Name: "JOB_KIND", Value: kind},
		{Name: "NAMESPACE", Value: c.namespace},
		// MySQL credentials for provenance recording.
		{Name: "MYSQL_HOST", Value: os.Getenv("MYSQL_HOST")},
		{Name: "MYSQL_PORT", Value: os.Getenv("MYSQL_PORT")},
		{Name: "MYSQL_USER", Value: os.Getenv("MYSQL_USER")},
		{Name: "MYSQL_PASSWORD", Value: os.Getenv("MYSQL_PASSWORD")},
	}

	// Garage credentials for artifact storage.
	if os.Getenv("GARAGE_ENABLED") == "true" {
		envs = append(envs,
			corev1.EnvVar{Name: "GARAGE_ENABLED", Value: "true"},
			corev1.EnvVar{Name: "GARAGE_ENDPOINT", Value: os.Getenv("GARAGE_ENDPOINT")},
			corev1.EnvVar{Name: "GARAGE_ACCESS_KEY", Value: os.Getenv("GARAGE_ACCESS_KEY")},
			corev1.EnvVar{Name: "GARAGE_SECRET_KEY", Value: os.Getenv("GARAGE_SECRET_KEY")},
			corev1.EnvVar{Name: "GARAGE_REGION", Value: os.Getenv("GARAGE_REGION")},
		)
	}

	// Pass CRD spec fields as uppercase env vars for the job container.
	for k, v := range spec {
		// Skip complex fields.
		switch k {
		case "parentJob", "gate", "timeout", "retryPolicy", "maxRetries", "computeClass":
			continue
		}
		if s, ok := v.(string); ok {
			envs = append(envs, corev1.EnvVar{
				Name:  strings.ToUpper(k),
				Value: s,
			})
		} else if n, ok := v.(int64); ok {
			envs = append(envs, corev1.EnvVar{
				Name:  strings.ToUpper(k),
				Value: fmt.Sprintf("%d", n),
			})
		} else if f, ok := v.(float64); ok {
			envs = append(envs, corev1.EnvVar{
				Name:  strings.ToUpper(k),
				Value: fmt.Sprintf("%g", f),
			})
		}
	}

	return envs
}

// updateCRDStatus patches the CRD status subresource with a new phase and event.
func (c *CRDController) updateCRDStatus(u *unstructured.Unstructured, phase, message string) {
	kind := u.GetKind()
	name := u.GetName()

	gvr, ok := registeredCRDs[kind]
	if !ok {
		log.Printf("[crd-controller] Cannot update status: unknown kind %q", kind)
		return
	}

	// Get the latest version to avoid conflicts.
	latest, err := c.dynamicClient.Resource(gvr).Namespace(c.namespace).Get(
		context.Background(), name, metav1.GetOptions{})
	if err != nil {
		log.Printf("[crd-controller] %s/%s: failed to get latest for status update: %v", kind, name, err)
		return
	}

	// Build status map.
	status := getStatusMap(latest)
	status["phase"] = phase

	now := time.Now().Format(time.RFC3339)
	if phase == "Running" {
		status["startTime"] = now
	}
	if phase == "Succeeded" || phase == "Failed" {
		status["completionTime"] = now
	}

	// Append event.
	events := getEvents(latest)
	events = append(events, map[string]interface{}{
		"timestamp": now,
		"message":   message,
	})
	status["events"] = events

	latest.Object["status"] = status

	_, err = c.dynamicClient.Resource(gvr).Namespace(c.namespace).
		UpdateStatus(context.Background(), latest, metav1.UpdateOptions{})
	if err != nil {
		log.Printf("[crd-controller] %s/%s: failed to update status to %s: %v", kind, name, phase, err)
	} else {
		log.Printf("[crd-controller] %s/%s: status updated to %s", kind, name, phase)
	}
}

// updateCRDStatusRetry updates the retry count and appends a retry event.
func (c *CRDController) updateCRDStatusRetry(u *unstructured.Unstructured, retryCount int32, message string) {
	kind := u.GetKind()
	name := u.GetName()

	gvr, ok := registeredCRDs[kind]
	if !ok {
		return
	}

	latest, err := c.dynamicClient.Resource(gvr).Namespace(c.namespace).Get(
		context.Background(), name, metav1.GetOptions{})
	if err != nil {
		log.Printf("[crd-controller] %s/%s: failed to get latest for retry update: %v", kind, name, err)
		return
	}

	status := getStatusMap(latest)
	status["retryCount"] = int64(retryCount)
	status["phase"] = "Pending" // Reset to Pending for re-creation.

	now := time.Now().Format(time.RFC3339)
	events := getEvents(latest)
	events = append(events, map[string]interface{}{
		"timestamp": now,
		"message":   message,
	})
	status["events"] = events

	latest.Object["status"] = status

	_, err = c.dynamicClient.Resource(gvr).Namespace(c.namespace).
		UpdateStatus(context.Background(), latest, metav1.UpdateOptions{})
	if err != nil {
		log.Printf("[crd-controller] %s/%s: failed to update retry status: %v", kind, name, err)
	}
}

// --- Helper functions for unstructured CRD access ---

// crdJobName returns the K8s Job name for a CRD instance.
func crdJobName(kind, name string) string {
	return fmt.Sprintf("%s-%s", strings.ToLower(kind), name)
}

// getPhase reads the phase from a CRD's status subresource.
func getPhase(u *unstructured.Unstructured) string {
	status, ok := u.Object["status"].(map[string]interface{})
	if !ok {
		return "Pending" // Default for newly created CRDs.
	}
	phase, _ := status["phase"].(string)
	if phase == "" {
		return "Pending"
	}
	return phase
}

// getSpec reads the spec from a CRD instance.
func getSpec(u *unstructured.Unstructured) map[string]interface{} {
	spec, ok := u.Object["spec"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return spec
}

// getSpecString reads a string field from the CRD spec.
func getSpecString(u *unstructured.Unstructured, field string) string {
	spec := getSpec(u)
	s, _ := spec[field].(string)
	return s
}

// getSpecInt reads an integer field from the CRD spec with a default value.
func getSpecInt(u *unstructured.Unstructured, field string, defaultVal int32) int32 {
	spec := getSpec(u)
	if v, ok := spec[field].(int64); ok {
		return int32(v)
	}
	if v, ok := spec[field].(float64); ok {
		return int32(v)
	}
	return defaultVal
}

// getStatusMap returns the status map, creating it if absent.
func getStatusMap(u *unstructured.Unstructured) map[string]interface{} {
	status, ok := u.Object["status"].(map[string]interface{})
	if !ok {
		status = make(map[string]interface{})
	}
	return status
}

// getStatusInt reads an integer field from the CRD status.
func getStatusInt(u *unstructured.Unstructured, field string) int32 {
	status, ok := u.Object["status"].(map[string]interface{})
	if !ok {
		return 0
	}
	if v, ok := status[field].(int64); ok {
		return int32(v)
	}
	if v, ok := status[field].(float64); ok {
		return int32(v)
	}
	return 0
}

// getEvents returns the events slice from the CRD status.
func getEvents(u *unstructured.Unstructured) []interface{} {
	status, ok := u.Object["status"].(map[string]interface{})
	if !ok {
		return nil
	}
	events, _ := status["events"].([]interface{})
	return events
}

// gvrForKind returns the GroupVersionResource for a known CRD kind.
// Returns an empty GVR if the kind is not registered.
func gvrForKind(kind string) schema.GroupVersionResource {
	gvr, ok := registeredCRDs[kind]
	if !ok {
		return schema.GroupVersionResource{}
	}
	return gvr
}
