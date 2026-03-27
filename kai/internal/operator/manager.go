package operator

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/hwcopeland/iac/kai/internal/config"
)

// NewManager creates a controller-runtime Manager scoped to cfg.KubeNamespace,
// registers the AgentSandboxReconciler, and returns the Manager ready to Start.
//
// The caller is responsible for calling mgr.Start(ctx) in a goroutine.
func NewManager(cfg *config.Config) (ctrl.Manager, error) {
	// ── Scheme: register k8s built-ins + Kai CRDs ─────────────────────────────
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(AddToScheme(s))

	// ── Kubernetes REST config ─────────────────────────────────────────────────
	restCfg, err := buildRestConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("building rest config: %w", err)
	}

	// ── Manager options ────────────────────────────────────────────────────────
	opts := ctrl.Options{
		Scheme: s,
		// Disable metrics server — observability is handled by the main server.
		Metrics: metricsserver.Options{BindAddress: "0"},
		// Scope the cache to a single namespace to reduce RBAC surface and memory.
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				cfg.KubeNamespace: {},
			},
		},
	}

	mgr, err := ctrl.NewManager(restCfg, opts)
	if err != nil {
		return nil, fmt.Errorf("creating manager: %w", err)
	}

	// ── Register reconciler ────────────────────────────────────────────────────
	reconciler := &AgentSandboxReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		Namespace:       cfg.KubeNamespace,
		AgentImage:      cfg.AgentImage,
		CallbackBaseURL: cfg.CallbackBaseURL,
		CallbackToken:   cfg.CallbackToken,
		ImagePullSecret: "zot-pull-secret",
		XAIAPIKey:       cfg.XAIAPIKey,
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		return nil, fmt.Errorf("setting up AgentSandboxReconciler: %w", err)
	}

	return mgr, nil
}

// buildRestConfig returns a *rest.Config using in-cluster credentials when
// cfg.KubeInCluster is true, falling back to cfg.KubeConfigPath otherwise.
func buildRestConfig(cfg *config.Config) (*rest.Config, error) {
	if cfg.KubeInCluster {
		rc, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("in-cluster config: %w", err)
		}
		return rc, nil
	}

	// Out-of-cluster: use the kubeconfig file at KubeConfigPath.
	// clientcmd.BuildConfigFromFlags("", "") falls back to KUBECONFIG env / ~/.kube/config.
	rc, err := clientcmd.BuildConfigFromFlags("", cfg.KubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("kubeconfig (%s): %w", cfg.KubeConfigPath, err)
	}
	return rc, nil
}
