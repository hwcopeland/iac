---
project: "kai"
maturity: "draft"
last_updated: "2025-07-14"
updated_by: "@staff-engineer"
scope: "AgentSandbox CRD definition, controller-runtime reconciler design, NetworkPolicy
  per-sandbox isolation, workspace PVC lifecycle, and TTL garbage collection."
owner: "@staff-engineer"
dependencies:
  - docs/prd/kai.md
  - docs/tdd/kai-backend.md
  - docs/tdd/kai-deploy.md
  - rke2/chem/flux/k8s-jobs/crd/dockingjob-crd.yaml
---

# TDD: Kai AgentSandbox CRD + Operator

## 1. Problem Statement

Each agent in a Kai team run needs an isolated, resource-bounded execution environment.
Kubernetes Jobs model single-run semantics well but lack a status sub-resource for
phase tracking, can't carry run-specific metadata cleanly, and don't support the
TTL + forced-termination semantics Kai needs.

A custom CRD (`AgentSandbox`) gives us:
- First-class phase tracking (`Pending → Running → Terminating → Terminated`)
- Status sub-resource (backend can update status without triggering full reconcile)
- Clean ownership semantics (GC via `ownerReferences` or explicit TTL)
- Extensible spec for future per-role resource tiers

This follows the proven `DockingJob` CRD pattern already in this repo
(`rke2/chem/flux/k8s-jobs/crd/dockingjob-crd.yaml`).

---

## 2. CRD Definition

```yaml
# rke2/kai/crd/agentsandbox-crd.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: agentsandboxes.kai.hwcopeland.net
spec:
  group: kai.hwcopeland.net
  names:
    kind: AgentSandbox
    listKind: AgentSandboxList
    plural: agentsandboxes
    singular: agentsandbox
    shortNames:
      - asb
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      subresources:
        status: {}           # enables status sub-resource (patch status without full update)
      additionalPrinterColumns:
        - name: Role
          type: string
          jsonPath: .spec.agentRole
        - name: Phase
          type: string
          jsonPath: .status.phase
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
      schema:
        openAPIV3Schema:
          type: object
          required: [spec]
          properties:
            spec:
              type: object
              required: [image, agentRole, runId, teamId]
              properties:
                image:
                  type: string
                  description: "Agent container image, e.g. zot.hwcopeland.net/kai/kai-agent:build-000042-abc1234"
                agentRole:
                  type: string
                  enum: [planner, researcher, coder_1, coder_2, reviewer]
                runId:
                  type: string
                  description: "UUID of the parent team_run"
                teamId:
                  type: string
                  description: "UUID of the owning team"
                timeoutSeconds:
                  type: integer
                  default: 1800
                  minimum: 60
                  maximum: 86400
                workspaceClaimName:
                  type: string
                  description: "Name of the shared workspace PVC for this run"
                resources:
                  type: object
                  properties:
                    requests:
                      type: object
                      properties:
                        cpu:    { type: string, default: "500m" }
                        memory: { type: string, default: "1Gi" }
                    limits:
                      type: object
                      properties:
                        cpu:    { type: string, default: "2" }
                        memory: { type: string, default: "4Gi" }
                env:
                  type: array
                  items:
                    type: object
                    required: [name, value]
                    properties:
                      name:  { type: string }
                      value: { type: string }
                envFrom:
                  type: array
                  description: "SecretRef sources for env vars (e.g. LiteLLM API key)"
                  items:
                    type: object
                    properties:
                      secretRef:
                        type: object
                        properties:
                          name: { type: string }
            status:
              type: object
              properties:
                phase:
                  type: string
                  enum: [Pending, Running, Terminating, Terminated]
                  default: Pending
                podRef:
                  type: string
                  description: "Name of the managed Pod"
                startTime:
                  type: string
                  format: date-time
                endTime:
                  type: string
                  format: date-time
                message:
                  type: string
                  description: "Human-readable status or error message"
                conditions:
                  type: array
                  items:
                    type: object
                    required: [type, status]
                    properties:
                      type:               { type: string }
                      status:             { type: string, enum: [True, False, Unknown] }
                      reason:             { type: string }
                      message:            { type: string }
                      lastTransitionTime: { type: string, format: date-time }
```

---

## 3. Reconciler Design

### 3.1 SetupWithManager

```go
// internal/operator/reconciler.go

func (r *AgentSandboxReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&kaiv1alpha1.AgentSandbox{}).
        Owns(&corev1.Pod{}).              // watch pods owned by sandboxes
        Owns(&networkingv1.NetworkPolicy{}).
        WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
        Complete(r)
}
```

`Owns(&corev1.Pod{})` means the controller is automatically notified when a managed
Pod changes phase — no polling required.

### 3.2 Reconcile Loop (pseudocode)

```go
func (r *AgentSandboxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var sb kaiv1alpha1.AgentSandbox
    if err := r.Get(ctx, req.NamespacedName, &sb); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Finalizer: ensure cleanup runs even if object is deleted while Running
    if !sb.DeletionTimestamp.IsZero() {
        return r.reconcileDeleting(ctx, &sb)
    }

    switch sb.Status.Phase {
    case "", kaiv1alpha1.PhasePending:
        return r.reconcilePending(ctx, &sb)
    case kaiv1alpha1.PhaseRunning:
        return r.reconcileRunning(ctx, &sb)
    case kaiv1alpha1.PhaseTerminating:
        return r.reconcileTerminating(ctx, &sb)
    case kaiv1alpha1.PhaseTerminated:
        return r.reconcileTerminated(ctx, &sb)  // TTL GC
    }
    return ctrl.Result{}, nil
}
```

### 3.3 reconcilePending

```go
func (r *AgentSandboxReconciler) reconcilePending(ctx context.Context, sb *kaiv1alpha1.AgentSandbox) (ctrl.Result, error) {
    // 1. Create NetworkPolicy
    netpol := r.buildNetworkPolicy(sb)
    if err := r.Create(ctx, netpol); err != nil && !apierrors.IsAlreadyExists(err) {
        return ctrl.Result{}, fmt.Errorf("create netpol: %w", err)
    }

    // 2. Create Pod
    pod := r.buildAgentPod(sb)
    if err := ctrl.SetControllerReference(sb, pod, r.Scheme); err != nil {
        return ctrl.Result{}, err
    }
    if err := r.Create(ctx, pod); err != nil && !apierrors.IsAlreadyExists(err) {
        return ctrl.Result{}, fmt.Errorf("create pod: %w", err)
    }

    // 3. Transition to Running
    sb.Status.Phase   = kaiv1alpha1.PhaseRunning
    sb.Status.PodRef  = pod.Name
    sb.Status.StartTime = &metav1.Time{Time: time.Now()}
    if err := r.Status().Update(ctx, sb); err != nil {
        return ctrl.Result{}, err
    }

    // 4. Requeue at timeout deadline
    timeout := time.Duration(sb.Spec.TimeoutSeconds) * time.Second
    return ctrl.Result{RequeueAfter: timeout}, nil
}
```

### 3.4 reconcileRunning

```go
func (r *AgentSandboxReconciler) reconcileRunning(ctx context.Context, sb *kaiv1alpha1.AgentSandbox) (ctrl.Result, error) {
    var pod corev1.Pod
    if err := r.Get(ctx, types.NamespacedName{Namespace: sb.Namespace, Name: sb.Status.PodRef}, &pod); err != nil {
        if apierrors.IsNotFound(err) {
            return r.markTerminated(ctx, sb, "PodDisappeared", "Pod was deleted unexpectedly")
        }
        return ctrl.Result{}, err
    }

    // Check timeout
    if sb.Status.StartTime != nil {
        elapsed := time.Since(sb.Status.StartTime.Time)
        timeout := time.Duration(sb.Spec.TimeoutSeconds) * time.Second
        if elapsed > timeout {
            slog.Warn("sandbox timed out", "sandbox", sb.Name, "elapsed", elapsed)
            // Transition to Terminating — next reconcile deletes the pod
            sb.Status.Phase   = kaiv1alpha1.PhaseTerminating
            sb.Status.Message = fmt.Sprintf("Timed out after %s", timeout)
            r.Status().Update(ctx, sb)
            return ctrl.Result{Requeue: true}, nil
        }
    }

    // Pod completed
    switch pod.Status.Phase {
    case corev1.PodSucceeded:
        return r.markTerminated(ctx, sb, "Succeeded", "Agent completed successfully")
    case corev1.PodFailed:
        reason := extractFailureReason(&pod)
        return r.markTerminated(ctx, sb, "Failed", reason)
    }

    // Still running — requeue to check timeout
    remaining := time.Duration(sb.Spec.TimeoutSeconds)*time.Second - time.Since(sb.Status.StartTime.Time)
    if remaining < 30*time.Second {
        remaining = 30 * time.Second
    }
    return ctrl.Result{RequeueAfter: remaining}, nil
}
```

### 3.5 reconcileTerminating

```go
func (r *AgentSandboxReconciler) reconcileTerminating(ctx context.Context, sb *kaiv1alpha1.AgentSandbox) (ctrl.Result, error) {
    // Delete pod with grace period
    var pod corev1.Pod
    if err := r.Get(ctx, types.NamespacedName{Namespace: sb.Namespace, Name: sb.Status.PodRef}, &pod); err == nil {
        grace := int64(30)
        r.Delete(ctx, &pod, &client.DeleteOptions{GracePeriodSeconds: &grace})
    }
    return r.markTerminated(ctx, sb, "Terminated", sb.Status.Message)
}
```

### 3.6 reconcileTerminated (TTL GC)

Matches the 300-second TTL pattern from `DockingJob` in this repo:

```go
func (r *AgentSandboxReconciler) reconcileTerminated(ctx context.Context, sb *kaiv1alpha1.AgentSandbox) (ctrl.Result, error) {
    if sb.Status.EndTime == nil {
        return ctrl.Result{}, nil
    }
    const ttl = 300 * time.Second
    age := time.Since(sb.Status.EndTime.Time)
    if age < ttl {
        return ctrl.Result{RequeueAfter: ttl - age}, nil
    }
    slog.Info("deleting TTL-expired sandbox", "name", sb.Name, "age", age)
    return ctrl.Result{}, r.Delete(ctx, sb)
}
```

---

## 4. Pod Template

```go
func (r *AgentSandboxReconciler) buildAgentPod(sb *kaiv1alpha1.AgentSandbox) *corev1.Pod {
    return &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            GenerateName: fmt.Sprintf("kai-agent-%s-%s-", sb.Spec.RunId[:8], sb.Spec.AgentRole),
            Namespace:    sb.Namespace,
            Labels: map[string]string{
                "app":        "kai-agent",
                "kai/run-id": sb.Spec.RunId,
                "kai/role":   sb.Spec.AgentRole,
                "kai/sandbox": sb.Name,
            },
        },
        Spec: corev1.PodSpec{
            RestartPolicy: corev1.RestartPolicyNever,  // not a Job — we own lifecycle
            Containers: []corev1.Container{
                {
                    Name:  "agent",
                    Image: sb.Spec.Image,
                    Env: append(
                        sb.Spec.Env,
                        corev1.EnvVar{Name: "KAI_RUN_ID",      Value: sb.Spec.RunId},
                        corev1.EnvVar{Name: "KAI_TEAM_ID",     Value: sb.Spec.TeamId},
                        corev1.EnvVar{Name: "KAI_AGENT_ROLE",  Value: sb.Spec.AgentRole},
                        corev1.EnvVar{Name: "KAI_SANDBOX_NAME", Value: sb.Name},
                        corev1.EnvVar{Name: "KAI_CALLBACK_URL",
                            Value: "http://kai-api.kai.svc.cluster.local:8081/internal/callback"},
                        corev1.EnvVar{Name: "LITELLM_BASE_URL",
                            Value: "http://openhands-litellm.openhands.svc.cluster.local:4000"},
                    ),
                    EnvFrom: sb.Spec.EnvFrom,
                    Resources: corev1.ResourceRequirements{
                        Requests: corev1.ResourceList{
                            corev1.ResourceCPU:    resource.MustParse(sb.Spec.Resources.Requests.CPU),
                            corev1.ResourceMemory: resource.MustParse(sb.Spec.Resources.Requests.Memory),
                        },
                        Limits: corev1.ResourceList{
                            corev1.ResourceCPU:    resource.MustParse(sb.Spec.Resources.Limits.CPU),
                            corev1.ResourceMemory: resource.MustParse(sb.Spec.Resources.Limits.Memory),
                        },
                    },
                    VolumeMounts: []corev1.VolumeMount{
                        {Name: "workspace", MountPath: "/workspace"},
                    },
                },
            },
            Volumes: []corev1.Volume{
                {
                    Name: "workspace",
                    VolumeSource: corev1.VolumeSource{
                        PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
                            ClaimName: sb.Spec.WorkspaceClaimName,
                        },
                    },
                },
            },
            ImagePullSecrets: []corev1.LocalObjectReference{
                {Name: "zot-pull-secret"},
            },
        },
    }
}
```

---

## 5. NetworkPolicy (Per-Sandbox)

Each AgentSandbox gets its own `NetworkPolicy` restricting egress to exactly what the
agent needs. Note: this cluster uses Cilium — `CiliumNetworkPolicy` may be preferred
over standard `NetworkPolicy` for cross-namespace egress, but standard NetworkPolicy
is used here for portability.

```go
func (r *AgentSandboxReconciler) buildNetworkPolicy(sb *kaiv1alpha1.AgentSandbox) *networkingv1.NetworkPolicy {
    return &networkingv1.NetworkPolicy{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("kai-sandbox-%s", sb.Name),
            Namespace: sb.Namespace,
        },
        Spec: networkingv1.NetworkPolicySpec{
            PodSelector: metav1.LabelSelector{
                MatchLabels: map[string]string{"kai/sandbox": sb.Name},
            },
            PolicyTypes: []networkingv1.PolicyType{
                networkingv1.PolicyTypeIngress,
                networkingv1.PolicyTypeEgress,
            },
            Ingress: []networkingv1.NetworkPolicyIngressRule{},  // no ingress
            Egress: []networkingv1.NetworkPolicyEgressRule{
                // DNS
                {
                    Ports: []networkingv1.NetworkPolicyPort{
                        {Protocol: protocolPtr(corev1.ProtocolUDP), Port: intStrPtr(53)},
                        {Protocol: protocolPtr(corev1.ProtocolTCP), Port: intStrPtr(53)},
                    },
                },
                // Kai API internal callback (port 8081)
                {
                    To: []networkingv1.NetworkPolicyPeer{{
                        NamespaceSelector: &metav1.LabelSelector{
                            MatchLabels: map[string]string{"kubernetes.io/metadata.name": "kai"},
                        },
                        PodSelector: &metav1.LabelSelector{
                            MatchLabels: map[string]string{"app": "kai-api"},
                        },
                    }},
                    Ports: []networkingv1.NetworkPolicyPort{
                        {Protocol: protocolPtr(corev1.ProtocolTCP), Port: intStrPtr(8081)},
                    },
                },
                // LiteLLM (cross-namespace to openhands)
                {
                    To: []networkingv1.NetworkPolicyPeer{{
                        NamespaceSelector: &metav1.LabelSelector{
                            MatchLabels: map[string]string{"kubernetes.io/metadata.name": "openhands"},
                        },
                        PodSelector: &metav1.LabelSelector{
                            MatchLabels: map[string]string{"app": "openhands-litellm"},
                        },
                    }},
                    Ports: []networkingv1.NetworkPolicyPort{
                        {Protocol: protocolPtr(corev1.ProtocolTCP), Port: intStrPtr(4000)},
                    },
                },
            },
        },
    }
}
```

---

## 6. Workspace PVC Lifecycle

One `PersistentVolumeClaim` is created per team run. All `AgentSandbox` pods in that
run mount it at `/workspace`. Agents read/write files there; the reviewer reads
coder output; artifacts are registered in the DB with paths relative to this PVC.

```go
// internal/operator/workspace.go

func (r *AgentSandboxReconciler) CreateWorkspacePVC(ctx context.Context, runID string) error {
    storageClass := "longhorn"
    pvc := &corev1.PersistentVolumeClaim{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("kai-workspace-%s", runID),
            Namespace: r.Namespace,
            Labels:    map[string]string{"kai/run-id": runID},
        },
        Spec: corev1.PersistentVolumeClaimSpec{
            AccessModes: []corev1.PersistentVolumeAccessMode{
                corev1.ReadWriteMany,  // multiple pods mount simultaneously
            },
            StorageClassName: &storageClass,
            Resources: corev1.VolumeResourceRequirements{
                Requests: corev1.ResourceList{
                    corev1.ResourceStorage: resource.MustParse("5Gi"),
                },
            },
        },
    }
    return r.Create(ctx, pvc)
}

func (r *AgentSandboxReconciler) DeleteWorkspacePVC(ctx context.Context, runID string) error {
    pvc := &corev1.PersistentVolumeClaim{}
    key := types.NamespacedName{
        Namespace: r.Namespace,
        Name:      fmt.Sprintf("kai-workspace-%s", runID),
    }
    if err := r.Get(ctx, key, pvc); err != nil {
        return client.IgnoreNotFound(err)
    }
    return r.Delete(ctx, pvc)
}
```

**Cleanup timing**: The run orchestrator calls `DeleteWorkspacePVC` after the reviewer
`AgentSandbox` transitions to `Terminated` and all artifact paths are recorded in the DB.

**Note**: Longhorn `ReadWriteMany` requires the Longhorn NFS provisioner. Verify it is
enabled on this cluster before Phase 1. Alternative: use `ReadWriteOnce` with a single
pod at a time (sequential agent execution only) for Phase 1.

---

## 7. RBAC

The operator runs as the `kai-api` ServiceAccount and needs a `ClusterRole` (not
namespaced `Role`) because it watches CRDs which are cluster-scoped resources, even
though the sandboxes themselves are namespaced.

```yaml
# rke2/kai/rbac.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kai-operator
rules:
  - apiGroups: [kai.hwcopeland.net]
    resources: [agentsandboxes]
    verbs: [get, list, watch, create, update, patch, delete]
  - apiGroups: [kai.hwcopeland.net]
    resources: [agentsandboxes/status]
    verbs: [get, update, patch]
  - apiGroups: [""]
    resources: [pods]
    verbs: [get, list, watch, create, delete]
  - apiGroups: [networking.k8s.io]
    resources: [networkpolicies]
    verbs: [get, list, watch, create, delete]
  - apiGroups: [""]
    resources: [persistentvolumeclaims]
    verbs: [get, list, watch, create, delete]
  - apiGroups: [""]
    resources: [events]
    verbs: [create, patch]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kai-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kai-operator
subjects:
  - kind: ServiceAccount
    name: kai-api
    namespace: kai
```

---

## 8. Open Questions

### OQ-1: Longhorn ReadWriteMany

Does this cluster have the Longhorn NFS provisioner enabled? `ReadWriteMany` is required
for multiple agent pods to mount the workspace PVC simultaneously. If not available,
Phase 1 must use sequential agent execution with `ReadWriteOnce`.

### OQ-2: CiliumNetworkPolicy vs NetworkPolicy

Cilium on this cluster may require `CiliumNetworkPolicy` for cross-namespace egress
(LiteLLM in `openhands` namespace). Standard `NetworkPolicy` cross-namespace selectors
work differently under Cilium. Test and confirm which resource type is needed.

### OQ-3: Agent Image Contents

`kai-agent` image is referenced but not yet defined. Needs its own TDD covering:
what tools are pre-installed, how it receives its task, how it calls LiteLLM, and
how it POSTs results back to `/internal/callback`.

### OQ-4: Workspace Cleanup Race

If the backend crashes after creating the PVC but before the run completes, the PVC
leaks. A reconciliation loop or a startup scan for orphaned `kai-workspace-*` PVCs
(no matching `team_runs` row) would be prudent.
