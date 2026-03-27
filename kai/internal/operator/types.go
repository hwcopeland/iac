package operator

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// ─── API group constants ───────────────────────────────────────────────────────

const (
	// Group is the API group for all Kai CRDs.
	Group   = "kai.hwcopeland.net"
	Version = "v1alpha1"

	// FinalizerName guards sandbox cleanup even when the object is deleted mid-run.
	FinalizerName = "kai.hwcopeland.net/cleanup"

	// Phase constants mirror the CRD enum.
	PhasePending     = "Pending"
	PhaseRunning     = "Running"
	PhaseTerminating = "Terminating"
	PhaseTerminated  = "Terminated"
)

// ─── Scheme registration ──────────────────────────────────────────────────────

var (
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}
	SchemeBuilder      = &scheme.Builder{GroupVersion: SchemeGroupVersion}
	AddToScheme        = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&AgentSandbox{}, &AgentSandboxList{})
}

// ─── CRD types ────────────────────────────────────────────────────────────────

// ResourceList holds CPU and memory resource strings, e.g. "500m" / "1Gi".
// Maps to the nested requests/limits structure in the CRD.
type ResourceList struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// AgentSandboxResources mirrors the CRD spec.resources field.
type AgentSandboxResources struct {
	Requests ResourceList `json:"requests,omitempty"`
	Limits   ResourceList `json:"limits,omitempty"`
}

// AgentSandboxSpec is the desired state of an AgentSandbox.
type AgentSandboxSpec struct {
	// Image is the agent container image.
	Image string `json:"image"`
	// AgentRole is one of: planner, researcher, coder_1, coder_2, reviewer.
	AgentRole string `json:"agentRole"`
	// RunID is the UUID of the parent team_run.
	RunID string `json:"runId"`
	// TeamID is the UUID of the owning team.
	TeamID string `json:"teamId"`
	// TimeoutSeconds caps agent execution time. Defaults to 1800.
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
	// WorkspaceClaimName is the shared workspace PVC for this run.
	WorkspaceClaimName string `json:"workspaceClaimName,omitempty"`
	// Resources overrides the default CPU/memory requests and limits.
	Resources *AgentSandboxResources `json:"resources,omitempty"`
	// Env provides extra environment variables beyond the standard Kai set.
	Env []corev1.EnvVar `json:"env,omitempty"`
	// EnvFrom sources environment variables from Kubernetes Secrets or ConfigMaps.
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`
}

// SandboxCondition describes a point-in-time observation of the sandbox state.
type SandboxCondition struct {
	Type               string       `json:"type"`
	Status             string       `json:"status"`
	Reason             string       `json:"reason,omitempty"`
	Message            string       `json:"message,omitempty"`
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
}

// AgentSandboxStatus is the observed state of an AgentSandbox.
type AgentSandboxStatus struct {
	// Phase is one of: Pending, Running, Terminating, Terminated.
	Phase string `json:"phase,omitempty"`
	// PodRef is the name of the managed Pod.
	PodRef string `json:"podRef,omitempty"`
	// Message is a human-readable status summary.
	Message string `json:"message,omitempty"`
	// StartTime records when the agent pod was first scheduled.
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// EndTime records when the agent reached a terminal phase.
	EndTime    *metav1.Time       `json:"endTime,omitempty"`
	Conditions []SandboxCondition `json:"conditions,omitempty"`
}

// AgentSandbox is the Schema for the agentsandboxes API.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type AgentSandbox struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSandboxSpec   `json:"spec"`
	Status AgentSandboxStatus `json:"status,omitempty"`
}

// AgentSandboxList contains a list of AgentSandbox.
//
// +kubebuilder:object:root=true
type AgentSandboxList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentSandbox `json:"items"`
}

// ─── runtime.Object (DeepCopyObject) ─────────────────────────────────────────

func (in *AgentSandbox) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *AgentSandboxList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// ─── DeepCopy methods (hand-written; no code generator) ──────────────────────

func (in *AgentSandbox) DeepCopy() *AgentSandbox {
	if in == nil {
		return nil
	}
	out := new(AgentSandbox)
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return out
}

func (in *AgentSandbox) DeepCopyInto(out *AgentSandbox) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *AgentSandboxList) DeepCopy() *AgentSandboxList {
	if in == nil {
		return nil
	}
	out := new(AgentSandboxList)
	out.TypeMeta = in.TypeMeta
	// ListMeta: only RemainingItemCount is a pointer.
	out.ListMeta = in.ListMeta
	if in.ListMeta.RemainingItemCount != nil {
		v := *in.ListMeta.RemainingItemCount
		out.ListMeta.RemainingItemCount = &v
	}
	if in.Items != nil {
		out.Items = make([]AgentSandbox, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}

// AgentSandboxSpec

func (in *AgentSandboxSpec) DeepCopy() *AgentSandboxSpec {
	if in == nil {
		return nil
	}
	out := new(AgentSandboxSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *AgentSandboxSpec) DeepCopyInto(out *AgentSandboxSpec) {
	*out = *in // copies all scalar fields
	if in.Resources != nil {
		r := *in.Resources // AgentSandboxResources contains only strings → value copy is safe
		out.Resources = &r
	}
	if in.Env != nil {
		out.Env = make([]corev1.EnvVar, len(in.Env))
		for i := range in.Env {
			in.Env[i].DeepCopyInto(&out.Env[i])
		}
	}
	if in.EnvFrom != nil {
		out.EnvFrom = make([]corev1.EnvFromSource, len(in.EnvFrom))
		for i := range in.EnvFrom {
			in.EnvFrom[i].DeepCopyInto(&out.EnvFrom[i])
		}
	}
}

// AgentSandboxStatus

func (in *AgentSandboxStatus) DeepCopy() *AgentSandboxStatus {
	if in == nil {
		return nil
	}
	out := new(AgentSandboxStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *AgentSandboxStatus) DeepCopyInto(out *AgentSandboxStatus) {
	*out = *in
	if in.StartTime != nil {
		t := *in.StartTime
		out.StartTime = &t
	}
	if in.EndTime != nil {
		t := *in.EndTime
		out.EndTime = &t
	}
	if in.Conditions != nil {
		out.Conditions = make([]SandboxCondition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

// SandboxCondition

func (in *SandboxCondition) DeepCopy() *SandboxCondition {
	if in == nil {
		return nil
	}
	out := new(SandboxCondition)
	in.DeepCopyInto(out)
	return out
}

func (in *SandboxCondition) DeepCopyInto(out *SandboxCondition) {
	*out = *in
	if in.LastTransitionTime != nil {
		t := *in.LastTransitionTime
		out.LastTransitionTime = &t
	}
}
