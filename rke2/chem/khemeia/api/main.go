// Package main provides the Khemeia API server with a YAML-driven plugin system.
// Plugins define compute backends (Quantum ESPRESSO, AutoDock Vina, etc.) as
// declarative YAML files that are loaded at startup to generate API routes,
// database tables, and K8s job specifications.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	typed "k8s.io/client-go/kubernetes/typed/batch/v1"
	rest "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Controller handles the lifecycle of plugin-driven compute jobs.
// State is persisted in PostgreSQL; no in-memory maps.
type Controller struct {
	client        *kubernetes.Clientset
	dynamicClient dynamic.Interface
	namespace     string
	jobClient     typed.JobInterface
	plugins       []Plugin
	pluginDBs     map[string]*DB
	stopCh        chan struct{}

	// sharedDB is the single PostgreSQL connection shared by all plugins and
	// all tables. All pluginDBs entries point to the same underlying *DB.
	sharedDB *DB

	// chemblDB is an optional read-only connection to the ChEMBL compound database.
	// Uses the same PostgreSQL instance as khemeia but connects to the chembl_36 database.
	// Nil if ChEMBL is not available.
	chemblDB *DB

	// s3Client provides S3 operations against Garage for artifact storage.
	// Initialized from GARAGE_* env vars; returns a no-op client when
	// GARAGE_ENABLED != "true".
	s3Client S3Client

	// crdController manages the lifecycle of CRD-based jobs (TargetPrep, DockJob, etc.).
	// Runs alongside the plugin-based job runner; both coexist.
	crdController *CRDController
}

// NewController creates a new Controller, initializing K8s client, PostgreSQL connection,
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

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %v", err)
	}

	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = "chem"
	}

	// Read PostgreSQL connection parameters from environment.
	pgHost := os.Getenv("POSTGRES_HOST")
	pgPort := os.Getenv("POSTGRES_PORT")
	pgUser := os.Getenv("POSTGRES_USER")
	pgPassword := os.Getenv("POSTGRES_PASSWORD")
	pgDBName := os.Getenv("POSTGRES_DB")
	if pgPort == "" {
		pgPort = "5432"
	}
	if pgDBName == "" {
		pgDBName = "khemeia"
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
		log.Printf("  - %s (slug=%s, type=%s)", p.Name, p.Slug, p.Type)
	}

	// Open a single shared PostgreSQL connection used by all plugins.
	pgDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		pgHost, pgPort, pgUser, pgPassword, pgDBName)
	rawDB, err := sql.Open("postgres", pgDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL: %w", err)
	}
	rawDB.SetMaxOpenConns(20)
	rawDB.SetConnMaxLifetime(5 * time.Minute)
	if err := rawDB.Ping(); err != nil {
		rawDB.Close()
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}
	sharedDB := &DB{DB: rawDB}
	log.Printf("PostgreSQL connection established to %s/%s", pgHost, pgDBName)

	controller := &Controller{
		client:        client,
		dynamicClient: dynClient,
		namespace:     namespace,
		jobClient:     client.BatchV1().Jobs(namespace),
		plugins:       plugins,
		pluginDBs:     make(map[string]*DB),
		stopCh:        make(chan struct{}),
		sharedDB:      sharedDB,
	}

	// Point every plugin slug at the shared DB and initialize all tables.
	if err := controller.initPluginDB(plugins); err != nil {
		rawDB.Close()
		return nil, fmt.Errorf("failed to initialize plugin tables: %w", err)
	}
	log.Println("All plugin tables initialized")

	// Connect to ChEMBL database on the same PostgreSQL instance (optional).
	chemblDBName := os.Getenv("CHEMBL_POSTGRES_DB")
	if chemblDBName == "" {
		chemblDBName = "chembl_36"
	}
	chemblDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		pgHost, pgPort, pgUser, pgPassword, chemblDBName)
	cRawDB, err := sql.Open("postgres", chemblDSN)
	if err == nil {
		cRawDB.SetMaxOpenConns(5)
		cRawDB.SetConnMaxLifetime(5 * time.Minute)
		if err := cRawDB.Ping(); err != nil {
			log.Printf("Warning: ChEMBL database not reachable (%v) — ligand search disabled", err)
			cRawDB.Close()
		} else {
			controller.chemblDB = &DB{DB: cRawDB}
			log.Println("ChEMBL PostgreSQL database connection established")
		}
	} else {
		log.Printf("Warning: failed to open ChEMBL database (%v) — ligand search disabled", err)
	}

	// Initialize S3 client for Garage artifact storage.
	s3Client, err := NewS3ClientFromEnv()
	if err != nil {
		rawDB.Close()
		return nil, fmt.Errorf("failed to initialize S3 client: %w", err)
	}
	controller.s3Client = s3Client

	// Initialize CRD controller for CRD-based job lifecycle management.
	controller.crdController = NewCRDController(
		client, dynClient, namespace, controller.sharedDB, s3Client)
	log.Println("CRD controller initialized")

	return controller, nil
}

// validTableName matches only safe SQL identifiers (alphanumeric + underscore).
var validTableName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// initPluginDB creates all tables in the shared PostgreSQL database.
// Every plugin slug is mapped to the same *DB instance; there are no
// per-plugin databases in the PostgreSQL layout.
func (c *Controller) initPluginDB(plugins []Plugin) error {
	db := c.sharedDB

	// Create the per-plugin jobs table and job_artifacts table for each plugin.
	for _, p := range plugins {
		if !validTableName.MatchString(p.TableName()) {
			return fmt.Errorf("invalid table name %q: must match [a-zA-Z0-9_]+", p.TableName())
		}

		if _, err := db.Exec(p.GenerateTableDDL()); err != nil {
			return fmt.Errorf("failed to create table %s: %w", p.TableName(), err)
		}
		if err := EnsureArtifactSchema(db); err != nil {
			return fmt.Errorf("failed to create job_artifacts table: %w", err)
		}

		c.pluginDBs[p.Slug] = db
	}

	// QE plugin: pseudopotentials table.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS pseudopotentials (
		id          SERIAL PRIMARY KEY,
		filename    VARCHAR(255) NOT NULL UNIQUE,
		content     BYTEA NOT NULL,
		element     VARCHAR(4) NOT NULL,
		functional  VARCHAR(32) NULL,
		source_url  TEXT NULL,
		created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("failed to create pseudopotentials table: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_pseudopotentials_element ON pseudopotentials (element)`); err != nil {
		return fmt.Errorf("failed to create pseudopotentials index: %w", err)
	}

	// Docking plugin: infrastructure tables.
	dockingDDLs := []string{
		`CREATE TABLE IF NOT EXISTS ligands (
			id            SERIAL PRIMARY KEY,
			compound_id   VARCHAR(255) NOT NULL,
			smiles        TEXT         NOT NULL,
			pdbqt         BYTEA        NULL,
			source_db     VARCHAR(255) NOT NULL,
			s3_pdbqt_key  VARCHAR(512) NULL,
			created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ligands_source_db ON ligands (source_db)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_ligands_compound_source ON ligands (compound_id, source_db)`,
		`CREATE TABLE IF NOT EXISTS docking_results (
			id                SERIAL PRIMARY KEY,
			workflow_name     VARCHAR(255) NOT NULL,
			pdb_id            VARCHAR(10)  NOT NULL,
			ligand_id         INT          NOT NULL,
			compound_id       VARCHAR(255) NOT NULL,
			affinity_kcal_mol FLOAT        NOT NULL,
			docked_pdbqt      BYTEA        NULL,
			s3_pose_key       VARCHAR(512) NULL,
			created_at        TIMESTAMP    DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_docking_results_workflow ON docking_results (workflow_name)`,
		`CREATE INDEX IF NOT EXISTS idx_docking_results_pdbid ON docking_results (pdb_id)`,
		`CREATE INDEX IF NOT EXISTS idx_docking_results_affinity ON docking_results (affinity_kcal_mol)`,
		`CREATE INDEX IF NOT EXISTS idx_docking_results_ligand ON docking_results (ligand_id)`,
		`CREATE TABLE IF NOT EXISTS staging (
			id         SERIAL PRIMARY KEY,
			job_type   TEXT NOT NULL CHECK (job_type IN ('prep', 'dock')),
			payload    JSON NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, ddl := range dockingDDLs {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("failed to create docking infrastructure table: %w", err)
		}
	}

	// Shared tables (all plugins).
	if err := EnsureAPITokenSchema(db); err != nil {
		log.Printf("Warning: failed to create api_tokens table: %v", err)
	}
	if err := EnsureBasisSetSchema(db); err != nil {
		log.Printf("Warning: failed to create basis_sets table: %v", err)
	}
	if err := EnsureProvenanceSchema(db); err != nil {
		log.Printf("Warning: failed to create provenance tables: %v", err)
	}
	if err := EnsureTargetPrepSchema(db); err != nil {
		log.Printf("Warning: failed to create target_prep_results table: %v", err)
	}
	if err := EnsureLibraryPrepSchema(db); err != nil {
		log.Printf("Warning: failed to create library_prep tables: %v", err)
	}
	if err := EnsureDockingV2Schema(db); err != nil {
		log.Printf("Warning: failed to create docking_v2 tables: %v", err)
	}
	if err := EnsureADMETSchema(db); err != nil {
		log.Printf("Warning: failed to create admet tables: %v", err)
	}
	if err := EnsureMDSchema(db); err != nil {
		log.Printf("Warning: failed to create md tables: %v", err)
	}
	log.Println("Shared tables initialized (api_tokens, basis_sets, provenance, target_prep_results, library_prep, docking_v2, admet, md)")

	return nil
}

// pluginDB returns the shared database for the given plugin slug.
// Since all plugins share one PostgreSQL connection, this always returns sharedDB.
func (c *Controller) pluginDB(slug string) *DB {
	return c.pluginDBs[slug]
}

// closeAllDBs closes the shared PostgreSQL connection.
func (c *Controller) closeAllDBs() {
	if c.sharedDB != nil {
		if err := c.sharedDB.Close(); err != nil {
			log.Printf("Warning: failed to close PostgreSQL connection: %v", err)
		}
	}
}

// firstDB returns the shared database used for cross-plugin tables (basis_sets, api_tokens).
func (c *Controller) firstDB() *DB {
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

	if shutdown, err := initTracer(ctx); err != nil {
		log.Printf("Warning: tracer init failed: %v", err)
	} else {
		defer shutdown()
	}
	if err := initMetrics(); err != nil {
		log.Printf("Warning: metrics init failed: %v", err)
	}

	// Recover any jobs orphaned by a previous controller restart.
	c.reconcileOrphanedJobs()

	// Start the CRD controller in a goroutine. It watches CRD instances
	// and manages their lifecycle (create K8s Jobs, monitor, retry).
	if c.crdController != nil {
		go c.crdController.Start(ctx)
		log.Println("CRD controller started in background")
	}

	go func() {
		if err := c.startAPIServer(); err != nil && err != http.ErrServerClosed {
			log.Printf("API server error: %v", err)
		}
	}()

	<-ctx.Done()
	if c.crdController != nil {
		c.crdController.Stop()
	}
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

		// GET /api/v1/{slug}/artifacts/{jobName} — list artifacts
		// GET /api/v1/{slug}/artifacts/{jobName}/{filename} — download artifact
		// Must be registered before /api/v1/{slug}/jobs/ to avoid prefix conflicts.
		mux.HandleFunc(fmt.Sprintf("/api/v1/%s/artifacts/", plugin.Slug), wrap(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeError(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			// Dispatch based on path depth: job name only = list, job name + filename = download.
			basePath := fmt.Sprintf("/api/v1/%s/artifacts/", plugin.Slug)
			remainder := strings.TrimPrefix(r.URL.Path, basePath)
			remainder = strings.TrimRight(remainder, "/")
			if strings.Contains(remainder, "/") {
				handler.PluginDownloadArtifact(plugin)(w, r)
			} else {
				handler.PluginListArtifacts(plugin)(w, r)
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

		log.Printf("Registered routes for plugin %s: /api/v1/%s/{submit,jobs,jobs/{name},artifacts/{name}/{file}}", plugin.Name, plugin.Slug)
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

	// List available ligand databases (distinct source_db values).
	mux.HandleFunc("/api/v1/ligand-databases", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.ListLigandDatabases(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// ChEMBL compound search.
	mux.HandleFunc("/api/v1/ligands/search", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.SearchLigands(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Import compounds from ChEMBL into the docking ligand database.
	mux.HandleFunc("/api/v1/ligands/import-from-chembl", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.ImportFromChEMBL(w, r)
		} else {
			writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Import all ChEMBL compounds matching search filters into the docking ligand database.
	mux.HandleFunc("/api/v1/ligands/import-from-filter", wrap(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.ImportFromFilter(w, r)
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

	// Binding pocket analysis — parses receptor and docked ligand PDBQTs to
	// identify contact residues and classify interactions.
	// GET /api/v1/docking/pocket/{jobName}/{compoundId}?cutoff=5.0
	mux.HandleFunc("/api/v1/docking/pocket/", wrap(handler.PocketAnalysis))

	// Docking set analysis — aggregate statistics across top-scoring compounds.
	// GET /api/v1/docking/analysis/receptor-contacts/{jobName}?top=50
	// GET /api/v1/docking/analysis/fingerprints/{jobName}?top=100
	mux.HandleFunc("/api/v1/docking/analysis/", wrap(handler.AnalysisDispatch))

	// ProLIF interaction map — proxies to the ProLIF sidecar service.
	mux.HandleFunc("/api/v1/docking/interaction-map/", wrap(handler.InteractionMapHandler))

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

	// Provenance tracking — artifact lineage DAG.
	// POST /api/v1/provenance/record          — create a provenance entry
	// GET  /api/v1/provenance/job/{name}       — list artifacts by job
	// GET  /api/v1/provenance/{id}             — get single record
	// GET  /api/v1/provenance/{id}/ancestors   — ancestor chain (recursive CTE)
	// GET  /api/v1/provenance/{id}/descendants — descendant chain (recursive CTE)
	mux.HandleFunc("/api/v1/provenance/", wrap(handler.provenanceDispatch))

	// CRD job advance and status endpoints.
	crdHandlers := NewCRDHandlers(c.dynamicClient, c.sharedDB, c.namespace)

	// POST /api/v1/jobs/{kind}/{name}/advance — advance pipeline to next stage.
	// GET  /api/v1/jobs/{kind}/{name}/status  — get CRD job status.
	mux.HandleFunc("/api/v1/jobs/", wrap(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/advance") && r.Method == http.MethodPost {
			crdHandlers.HandleAdvance(w, r)
		} else if strings.HasSuffix(path, "/status") && r.Method == http.MethodGet {
			crdHandlers.HandleJobStatus(w, r)
		} else {
			writeError(w, "not found", http.StatusNotFound)
		}
	}))
	log.Println("Registered CRD routes: /api/v1/jobs/{kind}/{name}/{advance,status}")

	// Target preparation endpoints (WP-1).
	// POST /api/v1/targets/prepare          — submit a target prep job
	// GET  /api/v1/targets/{name}           — get target prep status
	// GET  /api/v1/targets/{name}/pockets   — list detected pockets
	// POST /api/v1/targets/{name}/pockets/{index}/select — select a pocket
	mux.HandleFunc("/api/v1/targets/", wrap(handler.TargetDispatch))
	log.Println("Registered target prep routes: /api/v1/targets/{prepare,{name},{name}/pockets,{name}/pockets/{index}/select}")

	// Library preparation endpoints (WP-2).
	// POST /api/v1/libraries/prepare                 — submit a library prep job
	// GET  /api/v1/libraries/{name}                  — get library prep status
	// GET  /api/v1/libraries/{name}/compounds        — paginated compound list
	mux.HandleFunc("/api/v1/libraries/", wrap(handler.LibraryDispatch))
	log.Println("Registered library prep routes: /api/v1/libraries/{prepare,{name},{name}/compounds}")

	// Multi-engine docking endpoints (WP-3, v2 API).
	// POST /api/v1/docking/v2/submit              — submit a multi-engine docking job
	// GET  /api/v1/docking/v2/jobs/{name}         — get job status with per-engine progress
	// GET  /api/v1/docking/v2/jobs/{name}/results — paginated consensus-ranked results
	mux.HandleFunc("/api/v1/docking/v2/", wrap(handler.DockingV2Dispatch))
	log.Println("Registered docking v2 routes: /api/v1/docking/v2/{submit,jobs/{name},jobs/{name}/results}")

	// ADMET prediction endpoints (WP-4).
	// POST /api/v1/admet/predict                        — submit an ADMET prediction job
	// GET  /api/v1/admet/jobs/{name}                    — get ADMET job status
	// GET  /api/v1/admet/jobs/{name}/results            — paginated per-compound ADMET results
	// GET  /api/v1/admet/compound/{compoundId}          — single compound ADMET profile
	// GET  /api/v1/admet/presets                        — list available MPO presets
	mux.HandleFunc("/api/v1/admet/", wrap(handler.ADMETDispatch))
	log.Println("Registered ADMET routes: /api/v1/admet/{predict,jobs/{name},jobs/{name}/results,compound/{id},presets}")

	// MD simulation endpoints (WP-5).
	// POST /api/v1/md/submit                    — submit an MD simulation job
	// GET  /api/v1/md/jobs/{name}               — get MD job status
	// GET  /api/v1/md/jobs/{name}/results       — per-compound MD results
	mux.HandleFunc("/api/v1/md/", wrap(handler.MDDispatch))
	log.Println("Registered MD routes: /api/v1/md/{submit,jobs/{name},jobs/{name}/results}")

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

	// Prometheus metrics — registered directly, excluded from OTEL tracing.
	mux.Handle("/metrics", promhttp.Handler())

	log.Println("API server listening on :8080")
	h := otelhttp.NewHandler(corsMiddleware(bodySizeMiddleware(mux)), "khemeia-api")
	return http.ListenAndServe(":8080", h)
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
	migrateBlobs := flag.Bool("migrate-blobs", false, "Run BLOB-to-S3 migration and exit")
	flag.Parse()

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

	// If --migrate-blobs is set, run the BLOB migration and exit.
	if *migrateBlobs {
		log.Println("Running BLOB-to-S3 migration...")
		if err := RunBlobMigrationAllDBs(ctx, controller.sharedDB, controller.s3Client); err != nil {
			log.Fatalf("BLOB migration failed: %v", err)
		}

		// Verify migration completeness against the shared DB.
		if controller.sharedDB != nil {
			unmigrated, err := VerifyMigration(ctx, controller.sharedDB)
			if err != nil {
				log.Printf("Warning: migration verification failed: %v", err)
			} else {
				allClear := true
				for table, count := range unmigrated {
					if count > 0 {
						log.Printf("WARNING: %d unmigrated rows in %s", count, table)
						allClear = false
					}
				}
				if allClear {
					log.Println("Migration verification: all BLOBs migrated successfully")
				}
			}
		}

		controller.closeAllDBs()
		log.Println("BLOB migration complete, exiting.")
		return
	}

	if err := controller.Run(ctx); err != nil {
		log.Fatalf("Controller error: %v", err)
	}
}
