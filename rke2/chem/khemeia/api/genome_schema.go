package main

import "fmt"

// EnsureGenomeSchema creates the four PostgreSQL tables backing the genomics
// structural-biophysics tooling layer (core TDD §5.3):
//
//   - variant_jobs        : one parent batch row per u4u submit.
//   - variant_resolutions : content-addressed ResolvedVariant cache (index once, reuse forever).
//   - variant_calc_jobs   : one row per (variant × calculation) within a group.
//   - variant_results     : per-calc results — typed headline columns + JSONB payload.
//
// It mirrors EnsureDockingV2Schema exactly: CREATE TABLE IF NOT EXISTS, SERIAL
// PRIMARY KEY, status CHECK constraints, and "?" placeholders (none needed here,
// pure DDL). The genome tables deliberately upgrade from JSON to Postgres-16
// native JSONB + GIN for the flexible per-calc payloads and indexed querying.
//
// It is idempotent and safe to run unconditionally at startup even when
// GENOME_ENABLED is unset (core §7.2).
func EnsureGenomeSchema(db *DB) error {
	// Parent batch row: one per u4u submit.
	jobsDDL := `CREATE TABLE IF NOT EXISTS variant_jobs (
		id            SERIAL PRIMARY KEY,
		group_name    VARCHAR(255) NOT NULL UNIQUE,
		status        TEXT NOT NULL DEFAULT 'Pending'
		                CHECK (status IN ('Pending', 'Running', 'Completed', 'Failed')),
		calculations  JSONB NOT NULL,
		variant_count INT  NOT NULL DEFAULT 0,
		calc_count    INT  NOT NULL DEFAULT 0,
		submitted_by  VARCHAR(255) NULL,
		callback_url  TEXT NULL,
		input_data    JSONB NULL,
		error_output  TEXT NULL,
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at    TIMESTAMP NULL,
		completed_at  TIMESTAMP NULL
	)`
	if _, err := db.Exec(jobsDDL); err != nil {
		return fmt.Errorf("creating variant_jobs table: %w", err)
	}
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_variant_jobs_status     ON variant_jobs (status)`,
		`CREATE INDEX IF NOT EXISTS idx_variant_jobs_created_at ON variant_jobs (created_at)`,
	} {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("creating variant_jobs index: %w", err)
		}
	}

	// Cached resolutions: index a variant once, reuse forever.
	resolutionsDDL := `CREATE TABLE IF NOT EXISTS variant_resolutions (
		id               SERIAL PRIMARY KEY,
		resolution_id    VARCHAR(40)  NOT NULL UNIQUE,
		resolution_key   VARCHAR(255) NOT NULL UNIQUE,
		uniprot_acc      VARCHAR(32)  NOT NULL,
		uniprot_isoform  VARCHAR(40)  NOT NULL,
		residue_index    INT          NOT NULL,
		wild_type_aa     CHAR(1)      NOT NULL,
		mutant_aa        CHAR(1)      NOT NULL,
		sequence_length  INT          NOT NULL,
		structure_source TEXT NOT NULL CHECK (structure_source IN ('alphafold', 'esmfold', 'pdb')),
		structure_bucket VARCHAR(64)  NOT NULL,
		structure_key    VARCHAR(512) NOT NULL,
		plddt_global     FLOAT        NULL,
		resolved         JSONB        NOT NULL,
		created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`
	if _, err := db.Exec(resolutionsDDL); err != nil {
		return fmt.Errorf("creating variant_resolutions table: %w", err)
	}
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_variant_res_uniprot ON variant_resolutions (uniprot_acc, residue_index)`,
		`CREATE INDEX IF NOT EXISTS idx_variant_res_gin     ON variant_resolutions USING GIN (resolved)`,
	} {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("creating variant_resolutions index: %w", err)
		}
	}

	// One row per (variant × calculation) within a group.
	calcJobsDDL := `CREATE TABLE IF NOT EXISTS variant_calc_jobs (
		id            SERIAL PRIMARY KEY,
		group_name    VARCHAR(255) NOT NULL,
		variant_key   VARCHAR(255) NOT NULL,
		calculation   VARCHAR(32)  NOT NULL
		                CHECK (calculation IN ('esmfold', 'ddg_stability', 'pocket_proximity', 'pgx_docking')),
		resolution_id VARCHAR(40)  NULL,
		cr_name       VARCHAR(255) NULL,
		status        TEXT NOT NULL DEFAULT 'Pending'
		                CHECK (status IN ('Pending', 'Resolving', 'Running', 'Completed', 'Failed', 'Skipped')),
		params        JSONB NULL,
		error_output  TEXT NULL,
		started_at    TIMESTAMP NULL,
		completed_at  TIMESTAMP NULL,
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`
	if _, err := db.Exec(calcJobsDDL); err != nil {
		return fmt.Errorf("creating variant_calc_jobs table: %w", err)
	}
	for _, idx := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_calc_job ON variant_calc_jobs (group_name, variant_key, calculation)`,
		`CREATE INDEX IF NOT EXISTS idx_calc_jobs_group  ON variant_calc_jobs (group_name)`,
		`CREATE INDEX IF NOT EXISTS idx_calc_jobs_status ON variant_calc_jobs (status)`,
	} {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("creating variant_calc_jobs index: %w", err)
		}
	}

	// Per-calc results. Typed columns for the headline numbers u4u consumes;
	// JSONB payload for the full per-calc detail + GIN for flexible querying.
	resultsDDL := `CREATE TABLE IF NOT EXISTS variant_results (
		id               SERIAL PRIMARY KEY,
		result_id        VARCHAR(40)  NOT NULL UNIQUE,
		group_name       VARCHAR(255) NOT NULL,
		variant_key      VARCHAR(255) NOT NULL,
		calculation      VARCHAR(32)  NOT NULL,
		resolution_id    VARCHAR(40)  NOT NULL,
		structure_source TEXT         NULL,
		ddg_fold_kcal_mol     FLOAT   NULL,
		ddg_bind_kcal_mol     FLOAT   NULL,
		fp_delta_tanimoto     FLOAT   NULL,
		esmfold_plddt         FLOAT   NULL,
		esmfold_rmsd_ang      FLOAT   NULL,
		pocket_proximity_flag BOOLEAN NULL,
		pocket_distance_ang   FLOAT   NULL,
		confidence            FLOAT   NULL,
		payload          JSONB NOT NULL,
		artifact_keys    JSONB NULL,
		created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`
	if _, err := db.Exec(resultsDDL); err != nil {
		return fmt.Errorf("creating variant_results table: %w", err)
	}
	for _, idx := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_variant_result ON variant_results (group_name, variant_key, calculation)`,
		`CREATE INDEX IF NOT EXISTS idx_variant_results_group ON variant_results (group_name)`,
		`CREATE INDEX IF NOT EXISTS idx_variant_results_calc  ON variant_results (calculation)`,
		`CREATE INDEX IF NOT EXISTS idx_variant_results_gin   ON variant_results USING GIN (payload)`,
	} {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("creating variant_results index: %w", err)
		}
	}

	return nil
}
