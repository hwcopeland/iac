package operator

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// defaultTimeoutSeconds is used when spec.timeoutSeconds is zero / unset.
const defaultTimeoutSeconds = 1800

// AgentSandboxReconciler reconciles AgentSandbox objects.
type AgentSandboxReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Namespace       string
	AgentImage      string
	CallbackBaseURL string
	CallbackToken   string
	// ImagePullSecret is the name of the k8s secret used for private registry auth.
	// Leave empty to skip imagePullSecrets on the Pod.
	ImagePullSecret string
	// XAIAPIKey is injected into agent pods as XAI_API_KEY for direct Grok API access.
	XAIAPIKey string
}

// Reconcile is the main reconcile loop called for every AgentSandbox event.
func (r *AgentSandboxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	var sb AgentSandbox
	if err := r.Get(ctx, req.NamespacedName, &sb); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// ── Deletion path ─────────────────────────────────────────────────────────
	if !sb.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &sb)
	}

	// ── Ensure finalizer is present before any work ───────────────────────────
	if !controllerutil.ContainsFinalizer(&sb, FinalizerName) {
		controllerutil.AddFinalizer(&sb, FinalizerName)
		if err := r.Update(ctx, &sb); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding finalizer: %w", err)
		}
		// Return and let the update trigger a fresh reconcile.
		return ctrl.Result{}, nil
	}

	log.Info("reconciling sandbox", "phase", sb.Status.Phase)

	// ── Phase state machine ───────────────────────────────────────────────────
	switch sb.Status.Phase {
	case "", PhasePending:
		return r.reconcilePending(ctx, &sb)
	case PhaseRunning:
		return r.reconcileRunning(ctx, &sb)
	case PhaseTerminating:
		return r.reconcileTerminating(ctx, &sb)
	case PhaseTerminated:
		return r.reconcileTerminated(ctx, &sb)
	default:
		log.Info("unknown phase, ignoring", "phase", sb.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// ─── Phase handlers ───────────────────────────────────────────────────────────

// reconcilePending creates the NetworkPolicy and Pod, then advances to Running.
func (r *AgentSandboxReconciler) reconcilePending(ctx context.Context, sb *AgentSandbox) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	// Ensure NetworkPolicy exists (idempotent).
	netpol := BuildNetworkPolicy(sb)
	if err := r.Create(ctx, netpol); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, fmt.Errorf("creating NetworkPolicy: %w", err)
	}

	// Ensure Pod exists (idempotent).
	pod := BuildAgentPod(sb, r.CallbackBaseURL, r.CallbackToken, r.ImagePullSecret, r.XAIAPIKey)
	if err := r.Create(ctx, pod); err != nil && !apierrors.IsAlreadyExists(err) {
		return ctrl.Result{}, fmt.Errorf("creating Pod: %w", err)
	}

	// Advance phase to Running.
	now := metav1.Now()
	sb.Status.Phase = PhaseRunning
	sb.Status.PodRef = pod.Name
	sb.Status.StartTime = &now
	sb.Status.Message = "agent pod scheduled"
	if err := r.Status().Update(ctx, sb); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status to Running: %w", err)
	}

	timeout := r.sandboxTimeout(sb)
	log.Info("sandbox → Running", "pod", pod.Name, "timeout", timeout)
	return ctrl.Result{RequeueAfter: timeout}, nil
}

// reconcileRunning polls the managed Pod and detects completion or timeout.
func (r *AgentSandboxReconciler) reconcileRunning(ctx context.Context, sb *AgentSandbox) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	var pod corev1.Pod
	podKey := types.NamespacedName{Namespace: sb.Namespace, Name: sb.Name + "-pod"}
	if err := r.Get(ctx, podKey, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("pod not found, marking terminated", "pod", podKey.Name)
			return r.markTerminated(ctx, sb, "pod_lost", "agent pod was not found")
		}
		return ctrl.Result{}, fmt.Errorf("getting pod: %w", err)
	}

	// Terminal pod phases → advance sandbox to Terminated.
	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		log.Info("pod succeeded, marking terminated")
		return r.markTerminated(ctx, sb, "completed", "agent completed successfully")
	case corev1.PodFailed:
		log.Info("pod failed, marking terminated")
		return r.markTerminated(ctx, sb, "failed", "agent pod failed")
	}

	// Timeout check.
	if sb.Status.StartTime != nil {
		timeout := r.sandboxTimeout(sb)
		if time.Since(sb.Status.StartTime.Time) > timeout {
			log.Info("sandbox timed out, deleting pod and marking terminated")
			if err := r.Delete(ctx, &pod); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, fmt.Errorf("deleting timed-out pod: %w", err)
			}
			return r.markTerminated(ctx, sb, "timed_out", "agent exceeded configured timeout")
		}
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// reconcileTerminating deletes the Pod and NetworkPolicy, then sets Terminated.
func (r *AgentSandboxReconciler) reconcileTerminating(ctx context.Context, sb *AgentSandbox) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	if err := r.deletePod(ctx, sb); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.deleteNetpol(ctx, sb); err != nil {
		return ctrl.Result{}, err
	}

	now := metav1.Now()
	sb.Status.Phase = PhaseTerminated
	sb.Status.EndTime = &now
	sb.Status.Message = "terminated by request"
	if err := r.Status().Update(ctx, sb); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting status Terminated: %w", err)
	}

	log.Info("sandbox → Terminated (from Terminating)")
	return ctrl.Result{}, nil
}

// reconcileTerminated enforces a 300-second TTL by deleting the sandbox object.
func (r *AgentSandboxReconciler) reconcileTerminated(ctx context.Context, sb *AgentSandbox) (ctrl.Result, error) {
	if sb.Status.EndTime == nil {
		// EndTime not set — nothing to do; wait for a status update to trigger requeue.
		return ctrl.Result{}, nil
	}

	const ttl = 300 * time.Second
	elapsed := time.Since(sb.Status.EndTime.Time)
	if elapsed < ttl {
		remaining := ttl - elapsed
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// TTL expired — delete the sandbox (triggers finalizer cleanup via reconcileDelete).
	if err := r.Delete(ctx, sb); client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, fmt.Errorf("deleting expired sandbox: %w", err)
	}
	return ctrl.Result{}, nil
}

// reconcileDelete runs on DeletionTimestamp; cleans up Pod + NetworkPolicy, removes finalizer.
func (r *AgentSandboxReconciler) reconcileDelete(ctx context.Context, sb *AgentSandbox) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)
	log.Info("sandbox deletion in progress, cleaning up")

	if err := r.deletePod(ctx, sb); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.deleteNetpol(ctx, sb); err != nil {
		return ctrl.Result{}, err
	}

	controllerutil.RemoveFinalizer(sb, FinalizerName)
	if err := r.Update(ctx, sb); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing finalizer: %w", err)
	}

	log.Info("finalizer removed, sandbox deletion complete")
	return ctrl.Result{}, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// markTerminated sets phase=Terminated with a reason and end timestamp.
func (r *AgentSandboxReconciler) markTerminated(ctx context.Context, sb *AgentSandbox, reason, message string) (ctrl.Result, error) {
	now := metav1.Now()
	sb.Status.Phase = PhaseTerminated
	sb.Status.EndTime = &now
	sb.Status.Message = fmt.Sprintf("%s: %s", reason, message)
	if err := r.Status().Update(ctx, sb); err != nil {
		return ctrl.Result{}, fmt.Errorf("marking terminated (%s): %w", reason, err)
	}
	return ctrl.Result{}, nil
}

// deletePod removes the sandbox's managed Pod if it exists.
func (r *AgentSandboxReconciler) deletePod(ctx context.Context, sb *AgentSandbox) error {
	var pod corev1.Pod
	key := types.NamespacedName{Namespace: sb.Namespace, Name: sb.Name + "-pod"}
	if err := r.Get(ctx, key, &pod); err != nil {
		return client.IgnoreNotFound(err)
	}
	if err := r.Delete(ctx, &pod); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("deleting pod %s: %w", key.Name, err)
	}
	return nil
}

// deleteNetpol removes the sandbox's managed NetworkPolicy if it exists.
func (r *AgentSandboxReconciler) deleteNetpol(ctx context.Context, sb *AgentSandbox) error {
	var np networkingv1.NetworkPolicy
	key := types.NamespacedName{Namespace: sb.Namespace, Name: sb.Name + "-netpol"}
	if err := r.Get(ctx, key, &np); err != nil {
		return client.IgnoreNotFound(err)
	}
	if err := r.Delete(ctx, &np); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("deleting NetworkPolicy %s: %w", key.Name, err)
	}
	return nil
}

// sandboxTimeout returns the effective timeout duration for a sandbox.
func (r *AgentSandboxReconciler) sandboxTimeout(sb *AgentSandbox) time.Duration {
	secs := sb.Spec.TimeoutSeconds
	if secs <= 0 {
		secs = defaultTimeoutSeconds
	}
	return time.Duration(secs) * time.Second
}

// ─── Controller registration ─────────────────────────────────────────────────

// SetupWithManager registers the reconciler with the controller-runtime manager.
func (r *AgentSandboxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&AgentSandbox{}).
		Owns(&corev1.Pod{}).
		Owns(&networkingv1.NetworkPolicy{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}
