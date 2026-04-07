// Package main provides the Khemeia API server with a YAML-driven plugin system.
// Plugins define compute backends (Quantum ESPRESSO, AutoDock Vina, etc.) as
// declarative YAML files that are loaded at startup to generate API routes,
// database tables, and K8s job specifications.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typed "k8s.io/client-go/kubernetes/typed/batch/v1"
	rest "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Controller handles the lifecycle of plugin-driven compute jobs.
// State is persisted in MySQL; no in-memory maps.
type Controller struct {
	client    *kubernetes.Clientset
	namespace string
	jobClient typed.JobInterface
	plugins   []Plugin
	pluginDBs map[string]*sql.DB
	stopCh    chan struct{}

	// MySQL connection parameters (reused when creating per-plugin databases).
	mysqlHost     string
	mysqlPort     string
	mysqlUser     string
	mysqlPassword string

	// sharedDB is a stable reference to a single database used for shared tables
	// (basis_sets, api_tokens). Set once during init to avoid Go map iteration randomness.
	sharedDB *sql.DB
}

// NewController creates a new Controller, initializing K8s client, MySQL connections,
// and loading plugins from the plugins directory.
func NewController() (*Controller, error) {
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

	// Read MySQL connection parameters from environment.
	mysqlHost := os.Getenv("MYSQL_HOST")
	mysqlPort := os.Getenv("MYSQL_PORT")
	mysqlUser := os.Getenv("MYSQL_USER")
	mysqlPassword := os.Getenv("MYSQL_PASSWORD")
	if mysqlPort == "" {
		mysqlPort = "3306"
	}

	// Load plugins from the plugins directory.
	pluginsDir := os.Getenv("PLUGINS_DIR")
	if pluginsDir == "" {
		pluginsDir = "./plugins"
	}

	plugins, err := LoadPlugins(pluginsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugins: %v", err)
	}
	log.Printf("Loaded %d plugin(s)", len(plugins))
	for _, p := range plugins {
		log.Printf("  - %s (slug=%s, type=%s, database=%s)", p.Name, p.Slug, p.Type, p.Database)
	}

	controller := &Controller{
		client:        client,
		namespace:     namespace,
		jobClient:     client.BatchV1().Jobs(namespace),
		plugins:       plugins,
		pluginDBs:     make(map[string]*sql.DB),
		stopCh:        make(chan struct{}),
		mysqlHost:     mysqlHost,
		mysqlPort:     mysqlPort,
		mysqlUser:     mysqlUser,
		mysqlPassword: mysqlPassword,
	}

	// Create databases and tables for each plugin.
	for _, p := range plugins {
		if err := controller.initPluginDB(p); err != nil {
			controller.closeAllDBs()
			return nil, fmt.Errorf("failed to initialize database for plugin %s: %v", p.Name, err)
		}
	}
	log.Println("All plugin databases initialized")

	return controller, nil
}

// validDBName matches only safe SQL identifiers (alphanumeric + underscore).
var validDBName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// initPluginDB creates the database and tables for a single plugin.
func (c *Controller) initPluginDB(p Plugin) error {
	// Validate the database name to prevent SQL injection — it is interpolated
	// into a CREATE DATABASE statement that cannot use parameterized queries.
	if !validDBName.MatchString(p.Database) {
		return fmt.Errorf("invalid database name %q: must match [a-zA-Z0-9_]+", p.Database)
	}

	// Connect to MySQL without a specific database to create the plugin's database.
	adminDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/?parseTime=true",
		c.mysqlUser, c.mysqlPassword, c.mysqlHost, c.mysqlPort)
	adminDB, err := sql.Open("mysql", adminDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to MySQL: %w", err)
	}
	defer adminDB.Close()

	if _, err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", p.Database)); err != nil {
		return fmt.Errorf("failed to create database %s: %w", p.Database, err)
	}

	// Connect to the plugin's specific database.
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		c.mysqlUser, c.mysqlPassword, c.mysqlHost, c.mysqlPort, p.Database)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to open %s database: %w", p.Database, err)
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping %s database: %w", p.Database, err)
	}
	log.Printf("MySQL %s connection established", p.Database)

	// Create the jobs table.
	if _, err := db.Exec(p.GenerateTableDDL()); err != nil {
		db.Close()
		return fmt.Errorf("failed to create table %s: %w", p.TableName(), err)
	}

	// For QE plugin, also create the pseudopotentials table.
	if p.Slug == "qe" {
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS pseudopotentials (
			id          INT AUTO_INCREMENT PRIMARY KEY,
			filename    VARCHAR(255) NOT NULL UNIQUE,
			content     MEDIUMBLOB NOT NULL,
			element     VARCHAR(4) NOT NULL,
			functional  VARCHAR(32) NULL,
			source_url  TEXT NULL,
			created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_element (element)
		)`); err != nil {
			db.Close()
			return fmt.Errorf("failed to create pseudopotentials table: %w", err)
		}
	}

	// For docking plugin, create infrastructure tables (ligands, results, staging).
	if p.Slug == "docking" {
		dockingTables := []string{
			`CREATE TABLE IF NOT EXISTS ligands (
				id            INT AUTO_INCREMENT PRIMARY KEY,
				compound_id   VARCHAR(255) NOT NULL,
				smiles        TEXT         NOT NULL,
				pdbqt         MEDIUMBLOB   NULL,
				source_db     VARCHAR(255) NOT NULL,
				created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
				INDEX idx_source_db (source_db),
				UNIQUE INDEX idx_compound_source (compound_id, source_db)
			)`,
			`CREATE TABLE IF NOT EXISTS docking_results (
				id                INT AUTO_INCREMENT PRIMARY KEY,
				workflow_name     VARCHAR(255) NOT NULL,
				pdb_id            VARCHAR(10)  NOT NULL,
				ligand_id         INT          NOT NULL,
				compound_id       VARCHAR(255) NOT NULL,
				affinity_kcal_mol FLOAT        NOT NULL,
				docked_pdbqt      MEDIUMBLOB   NULL,
				created_at        TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
				INDEX idx_workflow (workflow_name),
				INDEX idx_pdbid    (pdb_id),
				INDEX idx_affinity (affinity_kcal_mol),
				INDEX idx_ligand   (ligand_id)
			)`,
			`CREATE TABLE IF NOT EXISTS staging (
				id         INT AUTO_INCREMENT PRIMARY KEY,
				job_type   ENUM('prep', 'dock') NOT NULL,
				payload    JSON NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
		}
		for _, ddl := range dockingTables {
			if _, err := db.Exec(ddl); err != nil {
				db.Close()
				return fmt.Errorf("failed to create docking infrastructure table: %w", err)
			}
		}
	}

	c.pluginDBs[p.Slug] = db

	// Set the shared DB to the first successfully connected database.
	// This is used for shared tables (basis_sets, api_tokens) to avoid
	// Go map iteration randomness in firstDB().
	if c.sharedDB == nil {
		c.sharedDB = db
		if err := EnsureAPITokenSchema(db); err != nil {
			log.Printf("Warning: failed to create api_tokens table in %s: %v", p.Database, err)
		}
		if err := EnsureBasisSetSchema(db); err != nil {
			log.Printf("Warning: failed to create basis_sets table in %s: %v", p.Database, err)
		}
		log.Printf("Shared tables (api_tokens, basis_sets) created in %s database", p.Database)
	}

	return nil
}

// pluginDB returns the database for a specific plugin slug.
func (c *Controller) pluginDB(slug string) *sql.DB {
	return c.pluginDBs[slug]
}

// closeAllDBs closes all plugin database connections.
func (c *Controller) closeAllDBs() {
	for slug, db := range c.pluginDBs {
		if err := db.Close(); err != nil {
			log.Printf("Warning: failed to close %s database: %v", slug, err)
		}
	}
}

// firstDB returns the shared database used for cross-plugin tables (basis_sets, api_tokens).
func (c *Controller) firstDB() *sql.DB {
	return c.sharedDB
}

func getConfig() (*rest.Config, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

// Run starts the controller.
func (c *Controller) Run(ctx context.Context) error {
	log.Println("Starting Khemeia API Controller...")

	go func() {
		if err := c.startAPIServer(); err != nil && err != http.ErrServerClosed {
			log.Printf("API server error: %v", err)
		}
	}()

	<-ctx.Done()
	c.closeAllDBs()
	return ctx.Err()
}

func (c *Controller) startAPIServer() error {
	handler := NewAPIHandler(c.client, c.namespace, c, c.pluginDBs)

	// Initialize auth middleware.
	var wrap func(http.HandlerFunc) http.HandlerFunc

	if os.Getenv("AUTH_ENABLED") == "true" {
		issuerURL := os.Getenv("OIDC_ISSUER_URL")
		if issuerURL == "" {
			return fmt.Errorf("AUTH_ENABLED=true but OIDC_ISSUER_URL is not set")
		}

		auth, err := NewAuthMiddleware(issuerURL)
		if err != nil {
			return fmt.Errorf("initializing auth middleware: %w", err)
		}
		if db := c.firstDB(); db != nil {
			auth.SetDB(db)
		}
		wrap = auth.Wrap
		log.Println("JWT authentication enabled")
	} else {
		noop := noopAuthMiddleware()
		wrap = noop.WrapNoop
		log.Println("JWT authentication disabled (AUTH_ENABLED != \"true\")")
	}

	mux := http.NewServeMux()

	// Register plugin routes dynamically.
	for _, p := range c.plugins {
		plugin := p // capture loop variable

		// POST /api/v1/{slug}/submit
		mux.HandleFunc(fmt.Sprintf("/api/v1/%s/submit", plugin.Slug), wrap(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				handler.PluginSubmit(plugin)(w, r)
			} else {
				writeError(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		}))

		// GET /api/v1/{slug}/jobs
		mux.HandleFunc(fmt.Sprintf("/api/v1/%s/jobs", plugin.Slug), wrap(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				handler.PluginList(plugin)(w, r)
			} else {
				writeError(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		}))

		// GET/DELETE /api/v1/{slug}/jobs/{name}
		mux.HandleFunc(fmt.Sprintf("/api/v1/%s/jobs/", plugin.Slug), wrap(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handler.PluginGet(plugin)(w, r)
			case http.MethodDelete:
				handler.PluginDelete(plugin)(w, r)
			default:
				writeError(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		}))

		log.Printf("Registered routes for plugin %s: /api/v1/%s/{submit,jobs,jobs/{name}}", plugin.Name, plugin.Slug)
	}

	// Plugin registry endpoint.
	mux.HandleFunc("/api/v1/plugins", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.ListPlugins(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Infrastructure endpoints (not plugin-specific).
	// Ligand import — uses the docking plugin's database.
	mux.HandleFunc("/api/v1/ligands", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.ImportLigands(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Ligand prep — uses the docking plugin's database.
	mux.HandleFunc("/api/v1/prep", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.StartPrep(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Pseudopotential management — uses the QE plugin's database.
	mux.HandleFunc("/api/v1/qe/pseudopotentials", wrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handler.UploadPseudopotential(w, r)
		case http.MethodGet:
			handler.ListPseudopotentials(w, r)
		default:
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Basis set management — shared across all plugins.
	mux.HandleFunc("/api/v1/basis-sets/search", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.SearchBasisSets(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/v1/basis-sets/import", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.ImportBasisSet(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/v1/basis-sets", wrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.ListBasisSets(w, r)
		case http.MethodPost:
			handler.UploadBasisSet(w, r)
		default:
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/v1/basis-sets/", wrap(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.GetBasisSet(w, r)
		case http.MethodDelete:
			handler.DeleteBasisSet(w, r)
		default:
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Token management — admin only (no external auth, internal IPs only).
	mux.HandleFunc("/api/v1/tokens", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handler.CreateAPIToken(w, r)
		case http.MethodGet:
			handler.ListAPITokens(w, r)
		default:
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/tokens/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			handler.RevokeAPIToken(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Health/readiness endpoints — always unauthenticated.
	mux.HandleFunc("/health", handler.HealthCheck)
	mux.HandleFunc("/readyz", handler.ReadinessCheck)

	log.Println("API server listening on :8080")
	return http.ListenAndServe(":8080", corsMiddleware(bodySizeMiddleware(mux)))
}

// allowedOrigins is the set of origins permitted by the CORS policy.
var allowedOrigins = map[string]bool{
	"https://khemeia.net":             true,
	"https://khemeia.hwcopeland.net":  true,
	"http://localhost:5174":           true,
}

// bodySizeMiddleware limits the maximum request body size to prevent abuse.
func bodySizeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB max
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers to all responses.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// readPodLogs reads the first pod's stdout for a given job.
func (c *Controller) readPodLogs(jobName string) (string, error) {
	pods, err := c.client.CoreV1().Pods(c.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil || len(pods.Items) == 0 {
		return "", fmt.Errorf("no pods found for job %s", jobName)
	}

	stream, err := c.client.CoreV1().Pods(c.namespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{}).
		Stream(context.TODO())
	if err != nil {
		return "", fmt.Errorf("getting logs: %w", err)
	}
	defer stream.Close()

	buf, err := io.ReadAll(stream)
	if err != nil {
		return "", fmt.Errorf("reading logs: %w", err)
	}
	return string(buf), nil
}

// streamJobLogs streams pod logs for a K8s Job to stdout in real-time.
// Best-effort observability — failures are logged as warnings.
func (c *Controller) streamJobLogs(ctx context.Context, jobName string) {
	var podName string
	pollTimeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for podName == "" {
		select {
		case <-ctx.Done():
			return
		case <-pollTimeout:
			log.Printf("[stream] %s: timed out waiting for pod to start", jobName)
			return
		case <-ticker.C:
			pods, err := c.client.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("job-name=%s", jobName),
			})
			if err != nil {
				log.Printf("[stream] %s: warning: failed to list pods: %v", jobName, err)
				continue
			}
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodSucceeded {
					podName = pod.Name
					break
				}
			}
		}
	}

	log.Printf("[stream] %s: streaming logs from pod %s", jobName, podName)

	stream, err := c.client.CoreV1().Pods(c.namespace).GetLogs(podName, &corev1.PodLogOptions{
		Follow: true,
	}).Stream(ctx)
	if err != nil {
		log.Printf("[stream] %s: warning: failed to open log stream: %v", jobName, err)
		return
	}
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		log.Printf("[%s] %s", jobName, scanner.Text())
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		log.Printf("[stream] %s: warning: log stream read error: %v", jobName, err)
	}
}

// emptyDirVolume returns a corev1.Volume backed by emptyDir.
func emptyDirVolume(name string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// emptyDirMount returns a corev1.VolumeMount for an emptyDir volume.
func emptyDirMount(name, mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
	}
}

// ptrInt32 returns a pointer to an int32.
func ptrInt32(i int32) *int32 {
	return &i
}

func main() {
	log.Println("Khemeia API Controller starting...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		cancel()
	}()

	controller, err := NewController()
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}

	if err := controller.Run(ctx); err != nil {
		log.Fatalf("Controller error: %v", err)
	}
}
