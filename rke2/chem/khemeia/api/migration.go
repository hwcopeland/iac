// Package main provides the BLOB-to-S3 migration logic for moving binary artifacts
// from MySQL BLOB columns to Garage (S3-compatible object store).
//
// The migration is idempotent: rows that already have an S3 key are skipped.
// It processes rows in batches of 100 to limit memory usage.
//
// Schema changes (ADD COLUMN s3_*_key) are applied idempotently using
// column-existence checks before ALTER TABLE.
//
// Dual-read helpers (GetReceptorPDBQT, GetDockedPDBQT, GetArtifactContent) allow
// the API to read from S3 when available, falling back to the MySQL BLOB column
// for rows not yet migrated.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"strings"
)

// migrationBatchSize controls how many rows are processed per batch.
const migrationBatchSize = 100

// ---------------------------------------------------------------------------
// Schema migration — add nullable S3 key columns
// ---------------------------------------------------------------------------

// columnExists checks whether a column exists in a table.
func columnExists(ctx context.Context, db *sql.DB, database, table, column string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME = ?`,
		database, table, column,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking column %s.%s.%s: %w", database, table, column, err)
	}
	return count > 0, nil
}

// addColumnIfNotExists adds a nullable VARCHAR(512) column to a table if it
// does not already exist. This makes the ALTER TABLE idempotent.
func addColumnIfNotExists(ctx context.Context, db *sql.DB, database, table, column string) error {
	exists, err := columnExists(ctx, db, database, table, column)
	if err != nil {
		return err
	}
	if exists {
		log.Printf("[migration] Column %s.%s already exists, skipping ALTER TABLE", table, column)
		return nil
	}
	ddl := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s VARCHAR(512) NULL", table, column)
	if _, err := db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("adding column %s to %s: %w", column, table, err)
	}
	log.Printf("[migration] Added column %s.%s", table, column)
	return nil
}

// ApplySchemaChanges adds the S3 key columns to all BLOB-containing tables.
// Safe to call multiple times (idempotent).
func ApplySchemaChanges(ctx context.Context, db *sql.DB, database string) error {
	columns := []struct {
		table  string
		column string
	}{
		{"docking_workflows", "s3_receptor_key"},
		{"docking_results", "s3_pose_key"},
		{"ligands", "s3_pdbqt_key"},
		{"job_artifacts", "s3_key"},
	}

	for _, c := range columns {
		if err := addColumnIfNotExists(ctx, db, database, c.table, c.column); err != nil {
			return fmt.Errorf("schema migration failed: %w", err)
		}
	}

	log.Printf("[migration] Schema changes complete for database %s", database)
	return nil
}

// ---------------------------------------------------------------------------
// BLOB migration — move data from MySQL to S3
// ---------------------------------------------------------------------------

// sha256Hex computes the SHA-256 hex digest of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// RunBlobMigration performs the full BLOB-to-S3 migration across all tables
// in the docking database. It:
//  1. Applies schema changes (add s3_*_key columns).
//  2. Migrates docking_workflows.receptor_pdbqt -> khemeia-receptors.
//  3. Migrates docking_results.docked_pdbqt -> khemeia-poses.
//  4. Migrates ligands.pdbqt -> khemeia-libraries.
//  5. Migrates job_artifacts.content -> khemeia-reports.
//
// Each step is idempotent: rows with a non-NULL S3 key are skipped.
// Provenance records are created for each migrated artifact.
func RunBlobMigration(ctx context.Context, db *sql.DB, s3 S3Client, database string) error {
	log.Println("[migration] Starting BLOB migration...")

	// Step 1: Apply schema changes.
	if err := ApplySchemaChanges(ctx, db, database); err != nil {
		return fmt.Errorf("schema changes failed: %w", err)
	}

	// Step 2: Migrate receptors.
	if err := migrateReceptors(ctx, db, s3); err != nil {
		return fmt.Errorf("receptor migration failed: %w", err)
	}

	// Step 3: Migrate docking results (poses).
	if err := migrateDockingResults(ctx, db, s3); err != nil {
		return fmt.Errorf("docking result migration failed: %w", err)
	}

	// Step 4: Migrate ligand PDBQTs.
	if err := migrateLigands(ctx, db, s3); err != nil {
		return fmt.Errorf("ligand migration failed: %w", err)
	}

	// Step 5: Migrate job artifacts.
	if err := migrateJobArtifacts(ctx, db, s3); err != nil {
		return fmt.Errorf("job artifact migration failed: %w", err)
	}

	log.Println("[migration] BLOB migration complete")
	return nil
}

// migrateReceptors migrates docking_workflows.receptor_pdbqt to S3.
// S3 key pattern: {pdbid}/{name}.pdbqt in khemeia-receptors bucket.
func migrateReceptors(ctx context.Context, db *sql.DB, s3 S3Client) error {
	log.Println("[migration] Migrating receptors (docking_workflows.receptor_pdbqt)...")
	migrated := 0

	for {
		rows, err := db.QueryContext(ctx, `
			SELECT name, pdbid, receptor_pdbqt
			FROM docking_workflows
			WHERE receptor_pdbqt IS NOT NULL
			  AND s3_receptor_key IS NULL
			LIMIT ?`, migrationBatchSize)
		if err != nil {
			return fmt.Errorf("querying unmigrated receptors: %w", err)
		}

		batchCount := 0
		for rows.Next() {
			var name, pdbid string
			var blobData []byte

			if err := rows.Scan(&name, &pdbid, &blobData); err != nil {
				rows.Close()
				return fmt.Errorf("scanning receptor row: %w", err)
			}

			if len(blobData) == 0 {
				continue
			}

			// Build S3 key and upload.
			s3Key := fmt.Sprintf("%s/%s.pdbqt", pdbid, name)
			checksum := sha256Hex(blobData)

			if err := s3.PutArtifact(ctx, BucketReceptors, s3Key, bytes.NewReader(blobData), "chemical/x-pdbqt"); err != nil {
				rows.Close()
				return fmt.Errorf("uploading receptor %s: %w", name, err)
			}

			// Update MySQL: set S3 key (keep BLOB for now — cleanup is a separate step).
			if _, err := db.ExecContext(ctx,
				`UPDATE docking_workflows SET s3_receptor_key = ? WHERE name = ?`,
				s3Key, name); err != nil {
				rows.Close()
				return fmt.Errorf("updating receptor S3 key for %s: %w", name, err)
			}

			// Record provenance.
			bucket := BucketReceptors
			prov := &ProvenanceRecord{
				ArtifactType:   "receptor",
				S3Bucket:       &bucket,
				S3Key:          &s3Key,
				ChecksumSHA256: &checksum,
				CreatedByJob:   "blob-migration",
				JobKind:        strPtr("BlobMigration"),
				JobNamespace:   "chem",
			}
			if err := RecordProvenance(ctx, db, prov, nil); err != nil {
				log.Printf("[migration] Warning: failed to record provenance for receptor %s: %v", name, err)
			}

			batchCount++
			migrated++
		}
		rows.Close()

		if batchCount == 0 {
			break
		}

		if migrated%migrationBatchSize == 0 || batchCount < migrationBatchSize {
			log.Printf("[migration] Receptors migrated so far: %d", migrated)
		}
	}

	log.Printf("[migration] Receptor migration complete: %d rows migrated", migrated)
	return nil
}

// migrateDockingResults migrates docking_results.docked_pdbqt to S3.
// S3 key pattern: DockJob/{workflow_name}/{compound_id}.pdbqt in khemeia-poses bucket.
func migrateDockingResults(ctx context.Context, db *sql.DB, s3 S3Client) error {
	log.Println("[migration] Migrating docking results (docking_results.docked_pdbqt)...")
	migrated := 0

	for {
		rows, err := db.QueryContext(ctx, `
			SELECT id, workflow_name, compound_id, docked_pdbqt
			FROM docking_results
			WHERE docked_pdbqt IS NOT NULL
			  AND s3_pose_key IS NULL
			LIMIT ?`, migrationBatchSize)
		if err != nil {
			return fmt.Errorf("querying unmigrated docking results: %w", err)
		}

		batchCount := 0
		for rows.Next() {
			var id int
			var workflowName, compoundID string
			var blobData []byte

			if err := rows.Scan(&id, &workflowName, &compoundID, &blobData); err != nil {
				rows.Close()
				return fmt.Errorf("scanning docking result row: %w", err)
			}

			if len(blobData) == 0 {
				continue
			}

			s3Key := fmt.Sprintf("DockJob/%s/%s.pdbqt", workflowName, compoundID)
			checksum := sha256Hex(blobData)

			if err := s3.PutArtifact(ctx, BucketPoses, s3Key, bytes.NewReader(blobData), "chemical/x-pdbqt"); err != nil {
				rows.Close()
				return fmt.Errorf("uploading pose %s/%s: %w", workflowName, compoundID, err)
			}

			if _, err := db.ExecContext(ctx,
				`UPDATE docking_results SET s3_pose_key = ? WHERE id = ?`,
				s3Key, id); err != nil {
				rows.Close()
				return fmt.Errorf("updating pose S3 key for id %d: %w", id, err)
			}

			// Record provenance.
			bucket := BucketPoses
			prov := &ProvenanceRecord{
				ArtifactType:   "docked_pose",
				S3Bucket:       &bucket,
				S3Key:          &s3Key,
				ChecksumSHA256: &checksum,
				CreatedByJob:   "blob-migration",
				JobKind:        strPtr("BlobMigration"),
				JobNamespace:   "chem",
			}
			if err := RecordProvenance(ctx, db, prov, nil); err != nil {
				log.Printf("[migration] Warning: failed to record provenance for pose %d: %v", id, err)
			}

			batchCount++
			migrated++
		}
		rows.Close()

		if batchCount == 0 {
			break
		}

		if migrated%migrationBatchSize == 0 || batchCount < migrationBatchSize {
			log.Printf("[migration] Docking results migrated so far: %d", migrated)
		}
	}

	log.Printf("[migration] Docking result migration complete: %d rows migrated", migrated)
	return nil
}

// migrateLigands migrates ligands.pdbqt to S3.
// S3 key pattern: {source_db}/{compound_id}.pdbqt in khemeia-libraries bucket.
func migrateLigands(ctx context.Context, db *sql.DB, s3 S3Client) error {
	log.Println("[migration] Migrating ligands (ligands.pdbqt)...")
	migrated := 0

	for {
		rows, err := db.QueryContext(ctx, `
			SELECT id, compound_id, source_db, pdbqt
			FROM ligands
			WHERE pdbqt IS NOT NULL
			  AND s3_pdbqt_key IS NULL
			LIMIT ?`, migrationBatchSize)
		if err != nil {
			return fmt.Errorf("querying unmigrated ligands: %w", err)
		}

		batchCount := 0
		for rows.Next() {
			var id int
			var compoundID, sourceDB string
			var blobData []byte

			if err := rows.Scan(&id, &compoundID, &sourceDB, &blobData); err != nil {
				rows.Close()
				return fmt.Errorf("scanning ligand row: %w", err)
			}

			if len(blobData) == 0 {
				continue
			}

			s3Key := fmt.Sprintf("%s/%s.pdbqt", sourceDB, compoundID)
			checksum := sha256Hex(blobData)

			if err := s3.PutArtifact(ctx, BucketLibraries, s3Key, bytes.NewReader(blobData), "chemical/x-pdbqt"); err != nil {
				rows.Close()
				return fmt.Errorf("uploading ligand %s/%s: %w", sourceDB, compoundID, err)
			}

			if _, err := db.ExecContext(ctx,
				`UPDATE ligands SET s3_pdbqt_key = ? WHERE id = ?`,
				s3Key, id); err != nil {
				rows.Close()
				return fmt.Errorf("updating ligand S3 key for id %d: %w", id, err)
			}

			// Record provenance.
			bucket := BucketLibraries
			prov := &ProvenanceRecord{
				ArtifactType:   "compound",
				S3Bucket:       &bucket,
				S3Key:          &s3Key,
				ChecksumSHA256: &checksum,
				CreatedByJob:   "blob-migration",
				JobKind:        strPtr("BlobMigration"),
				JobNamespace:   "chem",
			}
			if err := RecordProvenance(ctx, db, prov, nil); err != nil {
				log.Printf("[migration] Warning: failed to record provenance for ligand %d: %v", id, err)
			}

			batchCount++
			migrated++
		}
		rows.Close()

		if batchCount == 0 {
			break
		}

		if migrated%migrationBatchSize == 0 || batchCount < migrationBatchSize {
			log.Printf("[migration] Ligands migrated so far: %d", migrated)
		}
	}

	log.Printf("[migration] Ligand migration complete: %d rows migrated", migrated)
	return nil
}

// migrateJobArtifacts migrates job_artifacts.content to S3.
// S3 key pattern: {job_name}/{filename} in khemeia-reports bucket.
// This function migrates artifacts from ALL plugin databases (docking, qe,
// nwchem, psi4, atomic) by being called per-database.
func migrateJobArtifacts(ctx context.Context, db *sql.DB, s3 S3Client) error {
	log.Println("[migration] Migrating job artifacts (job_artifacts.content)...")
	migrated := 0

	for {
		rows, err := db.QueryContext(ctx, `
			SELECT id, job_name, filename, content_type, content
			FROM job_artifacts
			WHERE content IS NOT NULL
			  AND s3_key IS NULL
			LIMIT ?`, migrationBatchSize)
		if err != nil {
			return fmt.Errorf("querying unmigrated job artifacts: %w", err)
		}

		batchCount := 0
		for rows.Next() {
			var id int
			var jobName, filename, contentType string
			var blobData []byte

			if err := rows.Scan(&id, &jobName, &filename, &contentType, &blobData); err != nil {
				rows.Close()
				return fmt.Errorf("scanning job artifact row: %w", err)
			}

			if len(blobData) == 0 {
				continue
			}

			s3Key := fmt.Sprintf("%s/%s", jobName, filename)
			checksum := sha256Hex(blobData)

			if err := s3.PutArtifact(ctx, BucketReports, s3Key, bytes.NewReader(blobData), contentType); err != nil {
				rows.Close()
				return fmt.Errorf("uploading artifact %s/%s: %w", jobName, filename, err)
			}

			if _, err := db.ExecContext(ctx,
				`UPDATE job_artifacts SET s3_key = ? WHERE id = ?`,
				s3Key, id); err != nil {
				rows.Close()
				return fmt.Errorf("updating artifact S3 key for id %d: %w", id, err)
			}

			// Record provenance.
			bucket := BucketReports
			prov := &ProvenanceRecord{
				ArtifactType:   "report",
				S3Bucket:       &bucket,
				S3Key:          &s3Key,
				ChecksumSHA256: &checksum,
				CreatedByJob:   "blob-migration",
				JobKind:        strPtr("BlobMigration"),
				JobNamespace:   "chem",
			}
			if err := RecordProvenance(ctx, db, prov, nil); err != nil {
				log.Printf("[migration] Warning: failed to record provenance for artifact %d: %v", id, err)
			}

			batchCount++
			migrated++
		}
		rows.Close()

		if batchCount == 0 {
			break
		}

		if migrated%migrationBatchSize == 0 || batchCount < migrationBatchSize {
			log.Printf("[migration] Job artifacts migrated so far: %d", migrated)
		}
	}

	log.Printf("[migration] Job artifact migration complete: %d rows migrated", migrated)
	return nil
}

// RunBlobMigrationAllDBs runs the migration across all plugin databases.
// Schema changes and job_artifacts migration run per-database. Docking-specific
// tables (docking_workflows, docking_results, ligands) only exist in the docking DB.
func RunBlobMigrationAllDBs(ctx context.Context, pluginDBs map[string]*sql.DB, s3 S3Client) error {
	// Migrate docking-specific tables first (they only exist in the docking DB).
	dockingDB, ok := pluginDBs["docking"]
	if !ok {
		return fmt.Errorf("docking database not found in plugin databases")
	}

	log.Println("[migration] === Phase 1: Docking database ===")
	if err := RunBlobMigration(ctx, dockingDB, s3, "docking"); err != nil {
		return fmt.Errorf("docking DB migration failed: %w", err)
	}

	// Migrate job_artifacts in all other plugin databases.
	for slug, db := range pluginDBs {
		if slug == "docking" {
			continue // already handled
		}

		log.Printf("[migration] === Migrating job_artifacts in %s database ===", slug)

		// Add s3_key column to job_artifacts in this database.
		if err := addColumnIfNotExists(ctx, db, slug, "job_artifacts", "s3_key"); err != nil {
			return fmt.Errorf("schema change for %s.job_artifacts failed: %w", slug, err)
		}

		if err := migrateJobArtifacts(ctx, db, s3); err != nil {
			return fmt.Errorf("job_artifacts migration for %s failed: %w", slug, err)
		}
	}

	log.Println("[migration] === All databases migrated ===")
	return nil
}

// ---------------------------------------------------------------------------
// Dual-read helpers — S3-first with MySQL BLOB fallback
// ---------------------------------------------------------------------------

// GetReceptorPDBQT retrieves the receptor PDBQT for a docking workflow.
// It reads from S3 if s3_receptor_key is set, otherwise falls back to the
// MySQL BLOB column. This allows a seamless transition during migration.
func GetReceptorPDBQT(ctx context.Context, db *sql.DB, s3 S3Client, workflowName string) (string, error) {
	var s3Key sql.NullString
	var blobData []byte

	err := db.QueryRowContext(ctx,
		`SELECT s3_receptor_key, receptor_pdbqt FROM docking_workflows WHERE name = ?`,
		workflowName,
	).Scan(&s3Key, &blobData)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("workflow %q not found", workflowName)
	}
	if err != nil {
		return "", fmt.Errorf("querying receptor for %q: %w", workflowName, err)
	}

	// Prefer S3 if the key is set.
	if s3Key.Valid && s3Key.String != "" {
		data, err := readS3String(ctx, s3, BucketReceptors, s3Key.String)
		if err != nil {
			// Fall through to BLOB on S3 read failure — log but do not error.
			log.Printf("[migration] Warning: S3 read failed for receptor %s (key=%s), falling back to BLOB: %v",
				workflowName, s3Key.String, err)
		} else {
			return data, nil
		}
	}

	// Fall back to MySQL BLOB.
	if len(blobData) == 0 {
		return "", fmt.Errorf("no receptor data available for workflow %q", workflowName)
	}
	return string(blobData), nil
}

// GetDockedPDBQT retrieves a docked pose PDBQT for a specific compound in a workflow.
// It reads from S3 if s3_pose_key is set, otherwise falls back to the MySQL BLOB column.
func GetDockedPDBQT(ctx context.Context, db *sql.DB, s3 S3Client, workflowName, compoundID string) (string, error) {
	var s3Key sql.NullString
	var blobData []byte

	err := db.QueryRowContext(ctx,
		`SELECT s3_pose_key, docked_pdbqt FROM docking_results
		 WHERE workflow_name = ? AND compound_id = ?
		 LIMIT 1`,
		workflowName, compoundID,
	).Scan(&s3Key, &blobData)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("pose for %s/%s not found", workflowName, compoundID)
	}
	if err != nil {
		return "", fmt.Errorf("querying pose for %s/%s: %w", workflowName, compoundID, err)
	}

	// Prefer S3 if the key is set.
	if s3Key.Valid && s3Key.String != "" {
		data, err := readS3String(ctx, s3, BucketPoses, s3Key.String)
		if err != nil {
			log.Printf("[migration] Warning: S3 read failed for pose %s/%s (key=%s), falling back to BLOB: %v",
				workflowName, compoundID, s3Key.String, err)
		} else {
			return data, nil
		}
	}

	// Fall back to MySQL BLOB.
	if len(blobData) == 0 {
		return "", fmt.Errorf("no pose data available for %s/%s", workflowName, compoundID)
	}
	return string(blobData), nil
}

// GetArtifactContent retrieves a job artifact's binary content.
// It reads from S3 if s3_key is set, otherwise falls back to the MySQL BLOB column.
func GetArtifactContent(ctx context.Context, db *sql.DB, s3 S3Client, jobName, filename string) ([]byte, string, error) {
	var s3Key sql.NullString
	var contentType string
	var content []byte

	err := db.QueryRowContext(ctx,
		`SELECT s3_key, content_type, content FROM job_artifacts
		 WHERE job_name = ? AND filename = ?`,
		jobName, filename,
	).Scan(&s3Key, &contentType, &content)
	if err == sql.ErrNoRows {
		return nil, "", fmt.Errorf("artifact %s/%s not found", jobName, filename)
	}
	if err != nil {
		return nil, "", fmt.Errorf("querying artifact %s/%s: %w", jobName, filename, err)
	}

	// Prefer S3 if the key is set.
	if s3Key.Valid && s3Key.String != "" {
		data, err := readS3Bytes(ctx, s3, BucketReports, s3Key.String)
		if err != nil {
			log.Printf("[migration] Warning: S3 read failed for artifact %s/%s (key=%s), falling back to BLOB: %v",
				jobName, filename, s3Key.String, err)
		} else {
			return data, contentType, nil
		}
	}

	// Fall back to MySQL BLOB.
	if len(content) == 0 {
		return nil, "", fmt.Errorf("no content available for artifact %s/%s", jobName, filename)
	}
	return content, contentType, nil
}

// ---------------------------------------------------------------------------
// S3 read helpers
// ---------------------------------------------------------------------------

// readS3String reads an S3 object and returns its content as a string.
func readS3String(ctx context.Context, s3 S3Client, bucket, key string) (string, error) {
	data, err := readS3Bytes(ctx, s3, bucket, key)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// readS3Bytes reads an S3 object and returns its content as a byte slice.
func readS3Bytes(ctx context.Context, s3 S3Client, bucket, key string) ([]byte, error) {
	reader, err := s3.GetArtifact(ctx, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("reading S3 object %s/%s: %w", bucket, key, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("reading S3 body %s/%s: %w", bucket, key, err)
	}
	return data, nil
}

// ---------------------------------------------------------------------------
// Migration verification
// ---------------------------------------------------------------------------

// VerifyMigration checks that no BLOB rows remain without corresponding S3 keys.
// Returns a summary of unmigrated row counts per table.
func VerifyMigration(ctx context.Context, db *sql.DB) (map[string]int, error) {
	queries := map[string]string{
		"docking_workflows.receptor_pdbqt": `SELECT COUNT(*) FROM docking_workflows WHERE receptor_pdbqt IS NOT NULL AND s3_receptor_key IS NULL`,
		"docking_results.docked_pdbqt":     `SELECT COUNT(*) FROM docking_results WHERE docked_pdbqt IS NOT NULL AND s3_pose_key IS NULL`,
		"ligands.pdbqt":                    `SELECT COUNT(*) FROM ligands WHERE pdbqt IS NOT NULL AND s3_pdbqt_key IS NULL`,
		"job_artifacts.content":            `SELECT COUNT(*) FROM job_artifacts WHERE content IS NOT NULL AND s3_key IS NULL`,
	}

	result := make(map[string]int)
	for label, query := range queries {
		var count int
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			// Table might not exist in this database — skip.
			if strings.Contains(err.Error(), "doesn't exist") {
				continue
			}
			return nil, fmt.Errorf("verifying %s: %w", label, err)
		}
		result[label] = count
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

// strPtr returns a pointer to a string value.
func strPtr(s string) *string {
	return &s
}
