// Package main provides the result-writer service that drains the staging table
// and writes results to the appropriate main tables (ligands, docking_results).
package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
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
	WorkflowName   string  `json:"workflow_name"`
	PDBID          string  `json:"pdb_id"`
	LigandID       int     `json:"ligand_id"`
	CompoundID     string  `json:"compound_id"`
	AffinityKcalMol float64 `json:"affinity_kcal_mol"`
}

func main() {
	log.Println("Result-writer starting...")

	db, err := connectMySQL()
	if err != nil {
		log.Fatalf("Failed to connect to MySQL: %v", err)
	}
	defer db.Close()
	log.Println("MySQL connection established")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start health server.
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
	log.Println("Result-writer stopped")
}

// connectMySQL builds a DSN from MYSQL_* env vars and opens a connection pool.
func connectMySQL() (*sql.DB, error) {
	host := os.Getenv("MYSQL_HOST")
	port := os.Getenv("MYSQL_PORT")
	user := os.Getenv("MYSQL_USER")
	password := os.Getenv("MYSQL_PASSWORD")
	database := os.Getenv("MYSQL_DATABASE")
	if port == "" {
		port = "3306"
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		user, password, host, port, database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening mysql: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging mysql: %w", err)
	}
	return db, nil
}

// pollLoop polls the staging table every pollInterval and processes rows in batches.
func pollLoop(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	statsTicker := time.NewTicker(statsInterval)
	defer statsTicker.Stop()

	var prepCount, dockCount int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-statsTicker.C:
			p := atomic.SwapInt64(&prepCount, 0)
			d := atomic.SwapInt64(&dockCount, 0)
			if p > 0 || d > 0 {
				log.Printf("Processed %d prep rows, %d dock rows in last 30s", p, d)
			}
		case <-ticker.C:
			p, d, err := processBatch(ctx, db)
			if err != nil {
				log.Printf("Error processing batch: %v", err)
				continue
			}
			atomic.AddInt64(&prepCount, int64(p))
			atomic.AddInt64(&dockCount, int64(d))
		}
	}
}

// processBatch reads up to batchSize rows from staging, processes them in a
// single transaction, and deletes the processed rows.
func processBatch(ctx context.Context, db *sql.DB) (prepProcessed, dockProcessed int, err error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, job_type, payload FROM staging ORDER BY id ASC LIMIT ?`, batchSize)
	if err != nil {
		return 0, 0, fmt.Errorf("querying staging: %w", err)
	}
	defer rows.Close()

	var staged []stagingRow
	for rows.Next() {
		var r stagingRow
		if err := rows.Scan(&r.ID, &r.JobType, &r.Payload); err != nil {
			return 0, 0, fmt.Errorf("scanning staging row: %w", err)
		}
		staged = append(staged, r)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("iterating staging rows: %w", err)
	}

	if len(staged) == 0 {
		return 0, 0, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	var processedIDs []int

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
		default:
			log.Printf("Unknown job_type '%s' in staging row %d, skipping", row.JobType, row.ID)
			continue
		}
		processedIDs = append(processedIDs, row.ID)
	}

	// Delete all successfully processed rows.
	if len(processedIDs) > 0 {
		if err := deleteProcessedRows(ctx, tx, processedIDs); err != nil {
			return 0, 0, fmt.Errorf("deleting processed rows: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("committing transaction: %w", err)
	}

	return prepProcessed, dockProcessed, nil
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
		`UPDATE ligands SET pdbqt = ? WHERE id = ?`, pdbqtBytes, p.LigandID)
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
		`INSERT INTO docking_results (workflow_name, pdb_id, ligand_id, compound_id, affinity_kcal_mol)
		 VALUES (?, ?, ?, ?, ?)`,
		d.WorkflowName, d.PDBID, d.LigandID, d.CompoundID, d.AffinityKcalMol)
	if err != nil {
		return fmt.Errorf("inserting docking result for ligand %d: %w", d.LigandID, err)
	}
	return nil
}

// deleteProcessedRows deletes staging rows by their IDs within the transaction.
func deleteProcessedRows(ctx context.Context, tx *sql.Tx, ids []int) error {
	if len(ids) == 0 {
		return nil
	}

	// Build a parameterized IN clause.
	placeholders := make([]byte, 0, len(ids)*2)
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args[i] = id
	}

	query := fmt.Sprintf("DELETE FROM staging WHERE id IN (%s)", string(placeholders))
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

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			log.Printf("[readyz] MySQL ping failed: %v", err)
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
