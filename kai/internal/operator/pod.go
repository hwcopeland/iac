package operator

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// defaultResources are the fallback CPU/memory requests and limits for agent pods.
var defaultResources = AgentSandboxResources{
	Requests: ResourceList{CPU: "500m", Memory: "1Gi"},
	Limits:   ResourceList{CPU: "2", Memory: "4Gi"},
}

// BuildAgentPod constructs the Pod that runs the agent container for sb.
// callbackURL is the fully-qualified internal URL the agent should POST events to.
// imagePullSecret is the name of the k8s Secret for private registry auth; pass ""
// to omit imagePullSecrets from the spec.
// xaiAPIKey is injected as XAI_API_KEY so the agent can call api.x.ai directly.
func BuildAgentPod(sb *AgentSandbox, callbackURL, callbackToken, imagePullSecret, xaiAPIKey string) *corev1.Pod {
	trueVal := true

	// ── Resource requirements ─────────────────────────────────────────────────
	res := defaultResources
	if sb.Spec.Resources != nil {
		if sb.Spec.Resources.Requests.CPU != "" {
			res.Requests.CPU = sb.Spec.Resources.Requests.CPU
		}
		if sb.Spec.Resources.Requests.Memory != "" {
			res.Requests.Memory = sb.Spec.Resources.Requests.Memory
		}
		if sb.Spec.Resources.Limits.CPU != "" {
			res.Limits.CPU = sb.Spec.Resources.Limits.CPU
		}
		if sb.Spec.Resources.Limits.Memory != "" {
			res.Limits.Memory = sb.Spec.Resources.Limits.Memory
		}
	}

	resourceReqs := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    parseQuantity(res.Requests.CPU, "500m"),
			corev1.ResourceMemory: parseQuantity(res.Requests.Memory, "1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    parseQuantity(res.Limits.CPU, "2"),
			corev1.ResourceMemory: parseQuantity(res.Limits.Memory, "4Gi"),
		},
	}

	// ── Standard Kai environment variables ───────────────────────────────────
	stdEnv := []corev1.EnvVar{
		{Name: "KAI_RUN_ID", Value: sb.Spec.RunID},
		{Name: "KAI_TEAM_ID", Value: sb.Spec.TeamID},
		{Name: "KAI_AGENT_ROLE", Value: sb.Spec.AgentRole},
		{Name: "KAI_SANDBOX_NAME", Value: sb.Name},
		{Name: "KAI_CALLBACK_URL", Value: callbackURL},
		{Name: "KAI_CALLBACK_TOKEN", Value: callbackToken},
		{Name: "XAI_API_KEY", Value: xaiAPIKey},
	}
	// Append spec.env after standard vars; spec cannot override standard keys
	// but can add extras. If a spec key collides with a standard key the standard
	// one (earlier in the slice) wins per typical container env resolution.
	env := append(stdEnv, sb.Spec.Env...)

	// ── Volume / mount for shared workspace ──────────────────────────────────
	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount
	if sb.Spec.WorkspaceClaimName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "workspace",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: sb.Spec.WorkspaceClaimName,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "workspace",
			MountPath: "/workspace",
		})
	}

	// ── ImagePullSecrets ──────────────────────────────────────────────────────
	var pullSecrets []corev1.LocalObjectReference
	if imagePullSecret != "" {
		pullSecrets = []corev1.LocalObjectReference{{Name: imagePullSecret}}
	}

	// ── SecurityContext ───────────────────────────────────────────────────────
	// Agents need write access to their working directory, so ReadOnlyRootFilesystem
	// is false, but we still enforce non-root execution.
	secCtx := &corev1.SecurityContext{
		RunAsNonRoot: &trueVal,
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sb.Name + "-pod",
			Namespace: sb.Namespace,
			Labels: map[string]string{
				"kai.hwcopeland.net/sandbox": sb.Name,
				"kai.hwcopeland.net/role":    sb.Spec.AgentRole,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         SchemeGroupVersion.String(),
					Kind:               "AgentSandbox",
					Name:               sb.Name,
					UID:                sb.UID,
					Controller:         &trueVal,
					BlockOwnerDeletion: &trueVal,
				},
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "kai-agent",
			ImagePullSecrets:   pullSecrets,
			RestartPolicy:      corev1.RestartPolicyNever,
			Volumes:            volumes,
			Containers: []corev1.Container{
				{
					Name:            "agent",
					Image:           sb.Spec.Image,
					Env:             env,
					EnvFrom:         sb.Spec.EnvFrom,
					Resources:       resourceReqs,
					VolumeMounts:    volumeMounts,
					SecurityContext: secCtx,
				},
			},
		},
	}

	return pod
}

// parseQuantity parses a resource quantity string, falling back to defaultVal on error.
func parseQuantity(s, defaultVal string) resource.Quantity {
	if s == "" {
		return resource.MustParse(defaultVal)
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return resource.MustParse(defaultVal)
	}
	return q
}
