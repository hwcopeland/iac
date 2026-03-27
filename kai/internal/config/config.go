package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all runtime configuration for kai-api.
// Fields are populated from environment variables via envconfig.
// required:"true" fields cause Load() to return an error if unset.
type Config struct {
	// HTTP
	ListenAddr         string `envconfig:"LISTEN_ADDR"          default:":8080"`
	InternalListenAddr string `envconfig:"INTERNAL_LISTEN_ADDR" default:":8081"`

	// Development mode — enables text logging and relaxes some guards.
	Dev bool `envconfig:"DEV" default:"false"`

	// Database
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`

	// Auth — Authentik OIDC provider
	AuthIssuerURL    string `envconfig:"AUTH_ISSUER_URL"    required:"true"` // e.g. https://auth.hwcopeland.net/application/o/kai/
	AuthClientID     string `envconfig:"AUTH_CLIENT_ID"     required:"true"`
	AuthClientSecret string `envconfig:"AUTH_CLIENT_SECRET" required:"true"`
	AuthRedirectURL  string `envconfig:"AUTH_REDIRECT_URL"  required:"true"`

	// JWKS cache refresh interval (used by internal/auth/oidc.go)
	JWKSRefresh time.Duration `envconfig:"JWKS_REFRESH" default:"5m"`

	// Session
	SessionSecret   string        `envconfig:"SESSION_SECRET"   required:"true"`
	SessionDuration time.Duration `envconfig:"SESSION_DURATION" default:"168h"`

	// Agent callback — required for Phase 2 (agent operator); set to empty to defer startup until Phase 2.
	CallbackToken   string `envconfig:"SECRET_CALLBACK_TOKEN" default:""`
	CallbackBaseURL string `envconfig:"CALLBACK_BASE_URL"     default:"http://kai.kai.svc.cluster.local:8081"`

	// Kubernetes operator
	KubeNamespace  string `envconfig:"KUBE_NAMESPACE"  default:"kai"`
	KubeInCluster  bool   `envconfig:"KUBE_IN_CLUSTER" default:"true"`
	KubeConfigPath string `envconfig:"KUBECONFIG"      default:""`

	// Agent workload — required for Phase 2; placeholder until agent image is built.
	AgentImage string `envconfig:"AGENT_IMAGE" default:""`

	// LiteLLM proxy
	LiteLLMBaseURL string `envconfig:"LITELLM_BASE_URL" default:"http://openhands-litellm.openhands.svc.cluster.local:4000"`
	LiteLLMAPIKey  string `envconfig:"LITELLM_API_KEY"  default:""`

	// xAI (Grok) — direct API key injected into agent pods
	XAIAPIKey string `envconfig:"XAI_API_KEY" default:""`

	// Observability
	OTLPEndpoint string `envconfig:"OTLP_ENDPOINT" default:""`
	LogLevel     string `envconfig:"LOG_LEVEL"     default:"info"`

	// Concurrency limits
	MaxConcurrentRuns int           `envconfig:"MAX_CONCURRENT_RUNS" default:"20"`
	RunTimeout        time.Duration `envconfig:"RUN_TIMEOUT"         default:"30m"`
}

// Load reads configuration from environment variables.
// Returns an error if any required field is missing.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := envconfig.Process("", cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
