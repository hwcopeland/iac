package operator

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// BuildNetworkPolicy constructs the NetworkPolicy for an AgentSandbox.
//
// Ingress:  deny all (empty rules list).
// Egress:
//   1. DNS   — UDP+TCP port 53 to any destination.
//   2. Kai API internal — TCP port 8081 to kai-api pod in the kai namespace.
//   3. LiteLLM — TCP port 4000 to openhands-litellm in the openhands namespace.
func BuildNetworkPolicy(sb *AgentSandbox) *networkingv1.NetworkPolicy {
	trueVal := true

	protocolTCP := corev1.ProtocolTCP
	protocolUDP := corev1.ProtocolUDP

	port53 := intstr.FromInt(53)
	port8081 := intstr.FromInt(8081)
	port4000 := intstr.FromInt(4000)

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sb.Name + "-netpol",
			Namespace: sb.Namespace,
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
		Spec: networkingv1.NetworkPolicySpec{
			// Selector matches the agent pod created by BuildAgentPod.
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"kai.hwcopeland.net/sandbox": sb.Name,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},

			// Ingress: deny all — empty slice means no ingress is permitted.
			Ingress: []networkingv1.NetworkPolicyIngressRule{},

			// Egress: explicit allow-list.
			Egress: []networkingv1.NetworkPolicyEgressRule{
				// 1. DNS (UDP + TCP port 53 to any destination).
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &protocolUDP, Port: &port53},
						{Protocol: &protocolTCP, Port: &port53},
					},
					// No To peers → allow to any destination.
				},
				// 2. Kai API internal (TCP 8081, app=kai-api, namespace=kai).
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &protocolTCP, Port: &port8081},
					},
					To: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/name": "kai-api",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "kai",
								},
							},
						},
					},
				},
				// 3. LiteLLM (TCP 4000, app=openhands-litellm, namespace=openhands).
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &protocolTCP, Port: &port4000},
					},
					To: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "openhands",
								},
							},
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/name": "openhands-litellm",
								},
							},
						},
					},
				},
			},
		},
	}
}
