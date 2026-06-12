// Package main provides the result-writer service that drains the staging table
// and writes results to the appropriate main tables (ligands, docking_results).
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	pollInterval  = 3 * time.Second
	statsInterval = 30 * time.Second
	batchSize     = 1000
	httpPort      = 8081
)

// stagingRow represents a row from the staging table.
type stagingRow struct {
	ID      int
	JobType string
	Payload json.RawMessage
}

// prepPayload is the JSON payload for job_type='prep'.
type prepPayload struct {
	LigandID int    `json:"ligand_id"`
	PDBQTB64 string `json:"pdbqt_b64"`
}

// dockPayload is the JSON payload for job_type='dock'.
type dockPayload struct {
	WorkflowName    string  `json:"workflow_name"`
	PDBID           string  `json:"pdb_id"`
	LigandID        int     `json:"ligand_id"`
	CompoundID      string  `json:"compound_id"`
	AffinityKcalMol float64 `json:"affinity_kcal_mol"`
	DockedPDBQT     *string `json:"docked_pdbqt,omitempty"` // all 9 Vina modes, only for top hits
}

// genomeHeadline carries the typed-column "headline" numbers a genome worker
// emits per core TDD §5.6. Every field is a pointer so an absent field maps to
// SQL NULL (distinguished from a real 0.0), preserving the schema's "any may be
// NULL depending on calculation" semantics (core §5.3).
type genomeHeadline struct {
	DdgFoldKcalMol      *float64 `json:"ddg_fold_kcal_mol,omitempty"`     // ddg_stability
	DdgBindKcalMol      *float64 `json:"ddg_bind_kcal_mol,omitempty"`     // pgx_docking (mut-wt)
	FpDeltaTanimoto     *float64 `json:"fp_delta_tanimoto,omitempty"`     // pgx_docking (min tanimoto)
	EsmfoldPlddt        *float64 `json:"esmfold_plddt,omitempty"`         // esmfold (mut plddt_mean)
	EsmfoldRmsdAng      *float64 `json:"esmfold_rmsd_ang,omitempty"`      // esmfold (CA RMSD mut vs wt)
	PocketProximityFlag *bool    `json:"pocket_proximity_flag,omitempty"` // pocket_proximity (within_cutoff)
	PocketDistanceAng   *float64 `json:"pocket_distance_ang,omitempty"`   // pocket_proximity
	Confidence          *float64 `json:"confidence,omitempty"`            // 0..1, calc-specific
}

// genomeCalcPayload is the JSON envelope for job_type='genome_calc' (core §5.6).
// A genome worker (esmfold/ddg_stability/pocket_proximity/pgx_docking) stages
// this; the result-writer maps headline.* to variant_results typed columns,
// stores Payload/ArtifactKeys as JSONB, inserts variant_results, and completes
// the owning variant_calc_jobs row.
type genomeCalcPayload struct {
	GroupName       string          `json:"group_name"`
	VariantKey      string          `json:"variant_key"`
	Calculation     string          `json:"calculation"`
	ResolutionID    string          `json:"resolution_id"`
	StructureSource string          `json:"structure_source"`
	Headline        genomeHeadline  `json:"headline"`
	Payload         json.RawMessage `json:"payload"`                 // full per-calc §6 contract → JSONB
	ArtifactKeys    json.RawMessage `json:"artifact_keys,omitempty"` // {"report":"...", ...} → JSONB
}

func main() {
	log.Println("Result-writer starting...")

	db, err := connectPostgres()
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer db.Close()
	log.Println("PostgreSQL connection established")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Wire the result-writer's own OTel→Prometheus meter for the genome
	// distribution metrics (platform TDD §5.3). Degraded mode on failure: the
	// recording helpers are nil-safe, so the drain loop runs without metrics.
	metricsShutdown, err := initMetrics(ctx)
	if err != nil {
		logMetricsInitFailure(err)
	} else {
		log.Println("Genome distribution metrics initialized")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start health server (also serves /metrics for the Prometheus scrape).
	srv := startHealthServer(db)

	// Start the poll loop.
	go pollLoop(ctx, db)

	sig := <-sigCh
	log.Printf("Received %v, shutting down...", sig)
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	if metricsShutdown != nil {
		if err := metricsShutdown(shutdownCtx); err != nil {
			log.Printf("Metrics provider shutdown error: %v", err)
		}
	}
	log.Println("Result-writer stopped")
}

// connectPostgres builds a DSN from POSTGRES_* env vars and opens a connection pool.
func connectPostgres() (*sql.DB, error) {
	host := os.Getenv("POSTGRES_HOST")
	port := os.Getenv("POSTGRES_PORT")
	user := os.Getenv("POSTGRES_USER")
	password := os.Getenv("POSTGRES_PASSWORD")
	dbname := os.Getenv("POSTGRES_DB")
	if port == "" {
		port = "5432"
	}
	if dbname == "" {
		dbname = "khemeia"
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening postgres: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}
	return db, nil
}

// pollLoop polls the staging table every pollInterval and processes rows in batches.
func pollLoop(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	statsTicker := time.NewTicker(statsInterval)
	defer statsTicker.Stop()

	var prepCount, dockCount, genomeCount int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-statsTicker.C:
			p := atomic.SwapInt64(&prepCount, 0)
			d := atomic.SwapInt64(&dockCount, 0)
			g := atomic.SwapInt64(&genomeCount, 0)
			if p > 0 || d > 0 || g > 0 {
				log.Printf("Processed %d prep rows, %d dock rows, %d genome_calc rows in last 30s", p, d, g)
			}
		case <-ticker.C:
			p, d, g, err := processBatch(ctx, db)
			if err != nil {
				log.Printf("Error processing batch: %v", err)
				continue
			}
			atomic.AddInt64(&prepCount, int64(p))
			atomic.AddInt64(&dockCount, int64(d))
			atomic.AddInt64(&genomeCount, int64(g))
		}
	}
}

// processBatch reads up to batchSize rows from staging, processes them in a
// single transaction, and deletes the processed rows.
func processBatch(ctx context.Context, db *sql.DB) (prepProcessed, dockProcessed, genomeProcessed int, err error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, job_type, payload FROM staging ORDER BY id ASC LIMIT $1`, batchSize)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("querying staging: %w", err)
	}
	defer rows.Close()

	var staged []stagingRow
	for rows.Next() {
		var r stagingRow
		if err := rows.Scan(&r.ID, &r.JobType, &r.Payload); err != nil {
			return 0, 0, 0, fmt.Errorf("scanning staging row: %w", err)
		}
		staged = append(staged, r)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, fmt.Errorf("iterating staging rows: %w", err)
	}

	if len(staged) == 0 {
		return 0, 0, 0, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	var processedIDs []int
	// metricRecorders holds post-commit metric callbacks. Distributions are
	// recorded only AFTER the tx commits so a rolled-back batch never inflates
	// the histograms (record-once-per-landed-result semantics, core §5.6).
	var metricRecorders []func()

	for _, row := range staged {
		switch row.JobType {
		case "prep":
			if err := processPrep(ctx, tx, row); err != nil {
				log.Printf("Error processing prep row %d: %v", row.ID, err)
				continue
			}
			prepProcessed++
		case "dock":
			if err := processDock(ctx, tx, row); err != nil {
				log.Printf("Error processing dock row %d: %v", row.ID, err)
				continue
			}
			dockProcessed++
		case "genome_calc":
			rec, err := processGenomeCalc(ctx, tx, row)
			if err != nil {
				log.Printf("Error processing genome_calc row %d: %v", row.ID, err)
				continue
			}
			if rec != nil {
				metricRecorders = append(metricRecorders, rec)
			}
			genomeProcessed++
		default:
			log.Printf("Unknown job_type '%s' in staging row %d, skipping", row.JobType, row.ID)
			continue
		}
		processedIDs = append(processedIDs, row.ID)
	}

	// Delete all successfully processed rows.
	if len(processedIDs) > 0 {
		if err := deleteProcessedRows(ctx, tx, processedIDs); err != nil {
			return 0, 0, 0, fmt.Errorf("deleting processed rows: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, 0, fmt.Errorf("committing transaction: %w", err)
	}

	// Commit succeeded — emit the genome distribution metrics now.
	for _, rec := range metricRecorders {
		rec()
	}

	return prepProcessed, dockProcessed, genomeProcessed, nil
}

// processPrep handles a staging row with job_type='prep'.
// Decodes the base64 pdbqt and updates the ligands table.
func processPrep(ctx context.Context, tx *sql.Tx, row stagingRow) error {
	var p prepPayload
	if err := json.Unmarshal(row.Payload, &p); err != nil {
		return fmt.Errorf("unmarshaling prep payload: %w", err)
	}

	pdbqtBytes, err := base64.StdEncoding.DecodeString(p.PDBQTB64)
	if err != nil {
		return fmt.Errorf("decoding pdbqt base64 for ligand %d: %w", p.LigandID, err)
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE ligands SET pdbqt = $1 WHERE id = $2`, pdbqtBytes, p.LigandID)
	if err != nil {
		return fmt.Errorf("updating ligand %d pdbqt: %w", p.LigandID, err)
	}
	return nil
}

// processDock handles a staging row with job_type='dock'.
// Inserts a row into the docking_results table.
func processDock(ctx context.Context, tx *sql.Tx, row stagingRow) error {
	var d dockPayload
	if err := json.Unmarshal(row.Payload, &d); err != nil {
		return fmt.Errorf("unmarshaling dock payload: %w", err)
	}

	_, err := tx.ExecContext(ctx,
		`INSERT INTO docking_results (workflow_name, pdb_id, ligand_id, compound_id, affinity_kcal_mol, docked_pdbqt)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		d.WorkflowName, d.PDBID, d.LigandID, d.CompoundID, d.AffinityKcalMol, d.DockedPDBQT)
	if err != nil {
		return fmt.Errorf("inserting docking result for ligand %d: %w", d.LigandID, err)
	}
	return nil
}

// genomeResultID derives a deterministic, idempotent result_id from the
// (group_name, variant_key, calculation) triple — the same triple that
// uq_variant_result is unique on. Re-draining the same staged result therefore
// targets the same row, complementing the ON CONFLICT idempotency below.
func genomeResultID(groupName, variantKey, calculation string) string {
	h := sha256.Sum256([]byte(groupName + ":" + variantKey + ":" + calculation))
	return "res-" + hex.EncodeToString(h[:8]) // 16 hex chars, fits VARCHAR(40)
}

// processGenomeCalc handles a staging row with job_type='genome_calc' (core §5.6).
// It maps the worker's headline.* numbers onto variant_results typed columns,
// stores the full payload + artifact_keys as JSONB, INSERTs idempotently on
// uq_variant_result, and sets the owning variant_calc_jobs row to Completed.
//
// It returns a closure that records the §5.3 distribution metrics for this
// result. The caller invokes it only after the surrounding tx commits, so a
// rolled-back batch never inflates the histograms.
func processGenomeCalc(ctx context.Context, tx *sql.Tx, row stagingRow) (func(), error) {
	var g genomeCalcPayload
	if err := json.Unmarshal(row.Payload, &g); err != nil {
		return nil, fmt.Errorf("unmarshaling genome_calc payload: %w", err)
	}

	// Guard the required identity fields — without them the row cannot be
	// mapped to a variant_calc_jobs row or satisfy the result NOT NULLs.
	if g.GroupName == "" || g.VariantKey == "" || g.Calculation == "" || g.ResolutionID == "" {
		return nil, fmt.Errorf(
			"genome_calc payload missing required field (group_name=%q variant_key=%q calculation=%q resolution_id=%q)",
			g.GroupName, g.VariantKey, g.Calculation, g.ResolutionID)
	}

	// payload is NOT NULL in the schema; default to an empty JSON object rather
	// than failing the insert if a worker omits it.
	payload := g.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	// artifact_keys is nullable; pass nil (→ SQL NULL) when absent.
	var artifactKeys interface{}
	if len(g.ArtifactKeys) > 0 {
		artifactKeys = []byte(g.ArtifactKeys)
	}

	// structure_source is nullable; empty string → NULL for cleanliness.
	var structureSource interface{}
	if g.StructureSource != "" {
		structureSource = g.StructureSource
	}

	resultID := genomeResultID(g.GroupName, g.VariantKey, g.Calculation)

	// Idempotent insert: ON CONFLICT on uq_variant_result (group_name,
	// variant_key, calculation) refreshes the row so a re-drain is a no-op-ish
	// upsert rather than a duplicate-key failure.
	_, err := tx.ExecContext(ctx,
		`INSERT INTO variant_results (
			result_id, group_name, variant_key, calculation, resolution_id, structure_source,
			ddg_fold_kcal_mol, ddg_bind_kcal_mol, fp_delta_tanimoto,
			esmfold_plddt, esmfold_rmsd_ang,
			pocket_proximity_flag, pocket_distance_ang,
			confidence, payload, artifact_keys
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (group_name, variant_key, calculation) DO UPDATE SET
			resolution_id         = EXCLUDED.resolution_id,
			structure_source      = EXCLUDED.structure_source,
			ddg_fold_kcal_mol     = EXCLUDED.ddg_fold_kcal_mol,
			ddg_bind_kcal_mol     = EXCLUDED.ddg_bind_kcal_mol,
			fp_delta_tanimoto     = EXCLUDED.fp_delta_tanimoto,
			esmfold_plddt         = EXCLUDED.esmfold_plddt,
			esmfold_rmsd_ang      = EXCLUDED.esmfold_rmsd_ang,
			pocket_proximity_flag = EXCLUDED.pocket_proximity_flag,
			pocket_distance_ang   = EXCLUDED.pocket_distance_ang,
			confidence            = EXCLUDED.confidence,
			payload               = EXCLUDED.payload,
			artifact_keys         = EXCLUDED.artifact_keys`,
		resultID, g.GroupName, g.VariantKey, g.Calculation, g.ResolutionID, structureSource,
		nullFloat(g.Headline.DdgFoldKcalMol), nullFloat(g.Headline.DdgBindKcalMol), nullFloat(g.Headline.FpDeltaTanimoto),
		nullFloat(g.Headline.EsmfoldPlddt), nullFloat(g.Headline.EsmfoldRmsdAng),
		nullBool(g.Headline.PocketProximityFlag), nullFloat(g.Headline.PocketDistanceAng),
		nullFloat(g.Headline.Confidence), []byte(payload), artifactKeys,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting variant_result for (%s,%s,%s): %w",
			g.GroupName, g.VariantKey, g.Calculation, err)
	}

	// Complete the owning per-(variant,calc) row. The controller (GEN-12), not
	// the writer, advances group-level variant_jobs.status — we only complete
	// this leaf row.
	if _, err := tx.ExecContext(ctx,
		`UPDATE variant_calc_jobs
		   SET status = 'Completed', completed_at = CURRENT_TIMESTAMP
		 WHERE group_name = $1 AND variant_key = $2 AND calculation = $3`,
		g.GroupName, g.VariantKey, g.Calculation,
	); err != nil {
		return nil, fmt.Errorf("completing variant_calc_job for (%s,%s,%s): %w",
			g.GroupName, g.VariantKey, g.Calculation, err)
	}

	// Build the post-commit metric recorder. Distributions are observed at this
	// drain choke-point (platform §5.3) rather than in the four workers. Each
	// metric fires only when its corresponding headline field is present.
	h := g.Headline
	rec := func() {
		if h.DdgFoldKcalMol != nil {
			observeDdgFold(ctx, *h.DdgFoldKcalMol)
		}
		if h.DdgBindKcalMol != nil {
			observeDdgBind(ctx, *h.DdgBindKcalMol)
		}
		if h.FpDeltaTanimoto != nil {
			observeFpDeltaTanimoto(ctx, *h.FpDeltaTanimoto)
		}
		if h.EsmfoldPlddt != nil {
			observeEsmfoldPlddt(ctx, *h.EsmfoldPlddt)
		}
		if h.PocketProximityFlag != nil {
			recordPocketProximity(ctx, *h.PocketProximityFlag)
		}
	}
	return rec, nil
}

// nullFloat converts a *float64 to a driver value: the float when present,
// otherwise nil (→ SQL NULL).
func nullFloat(v *float64) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// nullBool converts a *bool to a driver value: the bool when present, otherwise
// nil (→ SQL NULL).
func nullBool(v *bool) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// deleteProcessedRows deletes staging rows by their IDs within the transaction.
func deleteProcessedRows(ctx context.Context, tx *sql.Tx, ids []int) error {
	if len(ids) == 0 {
		return nil
	}

	// Build a parameterized IN clause with $1, $2, ... placeholders.
	var b strings.Builder
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM staging WHERE id IN (%s)", b.String())
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}

// startHealthServer starts the HTTP health/readiness server on httpPort.
func startHealthServer(db *sql.DB) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "healthy",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// Prometheus scrape endpoint. The OTel→Prometheus exporter registers into
	// the default registry; promhttp.Handler() serves it (mirrors api/main.go).
	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			log.Printf("[readyz] PostgreSQL ping failed: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"status": "not ready", "error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", httpPort),
		Handler: mux,
	}

	go func() {
		log.Printf("Health server listening on :%d", httpPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Health server error: %v", err)
		}
	}()

	return srv
}
