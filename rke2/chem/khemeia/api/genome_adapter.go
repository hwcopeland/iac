// genome_adapter.go implements the variant adapter / structure-resolution core
// for the Khemeia genomics structural-biophysics tooling layer (core TDD §5.2).
//
// This module is PURE RESOLUTION. Given a submitted variant in one of three
// modes (protein_change / hgvs / rsid), it normalizes to a canonical
// (uniprot_acc, isoform, residue_index, wt_aa, mut_aa), validates the wild-type
// amino acid against the canonical UniProt sequence, resolves a structure
// (AlphaFold DB -> ESMFold fallback marker), and read-through caches the result
// in the variant_resolutions table (content-addressed: index once, reuse
// forever). It contains NO Kubernetes / CR logic -- the controller reconcile
// (GEN-12) consumes this module's exported entrypoint and acts on the returned
// status (e.g. minting an esmfold GenomeJob when fallback is needed).
//
// Exported entrypoint (consumed by GEN-11 REST submit and GEN-12 reconcile):
//
//	func ResolveVariant(ctx context.Context, db *DB, s3 S3Client, in VariantInput) (*ResolvedVariant, error)
//
// On error, the returned error is always a *ResolveError carrying a frozen
// E_* code (core Appendix A) so callers can place it in submit `rejected[]` or
// results `failures[]` without string-matching.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// --- Frozen typed error codes (core TDD Appendix A) ---
//
// These are the complete set of resolution-scoped error codes. They are
// returned (never silently dropped) in submit `rejected[]` and results
// `failures[]`. The adapter only emits the subset reachable from pure
// resolution; the remainder (E_NO_LIGAND_STRUCTURE, E_STRUCTURE_FOLD_FAILED,
// E_PARAMS_INVALID) belong to the workers but are declared here as the single
// source of truth so every phase references the same constants.
const (
	// ECodeVariantNotMissense -- rsID (or HGVS) does not resolve to a single
	// missense protein consequence (non-coding, synonymous, or multi-gene).
	// These four calcs are missense-scoped by design.
	ECodeVariantNotMissense = "E_VARIANT_NOT_MISSENSE"

	// ECodeWTMismatch -- sequence[residue_index-1] != submitted wild_type_aa.
	// Guards against transcript / genome-build drift. Never silently proceed.
	ECodeWTMismatch = "E_WT_MISMATCH"

	// ECodeResolveUpstream -- an upstream resolver (UniProt / Ensembl /
	// AlphaFold DB) was unreachable or returned a transport/5xx error. The
	// batch continues; this variant is rejected and retryable.
	ECodeResolveUpstream = "E_RESOLVE_UPSTREAM"

	// ECodeUniProtNotFound -- no UniProt accession could be resolved for the
	// submitted gene / transcript / rsID (4xx / empty result from UniProt).
	ECodeUniProtNotFound = "E_UNIPROT_NOT_FOUND"

	// ECodeParamsInvalid -- the submitted variant block is malformed: missing
	// required fields for the declared mode, or an unparseable protein change /
	// HGVS string.
	ECodeParamsInvalid = "E_PARAMS_INVALID"

	// ECodeNoLigandStructure -- PGx worker: a drug could not be resolved to a
	// ligand structure. Declared here for a single source of truth; not emitted
	// by the adapter.
	ECodeNoLigandStructure = "E_NO_LIGAND_STRUCTURE"

	// ECodeStructureFoldFailed -- ESMFold worker: folding failed (OOM, over
	// length cap). Declared here; not emitted by the adapter.
	ECodeStructureFoldFailed = "E_STRUCTURE_FOLD_FAILED"
)

// ResolveError wraps a resolution failure with a frozen E_* code (Appendix A).
// Callers place err.Code in submit `rejected[]` / results `failures[]`.
type ResolveError struct {
	Code string // one of the ECode* constants
	Msg  string // human-readable detail (safe to log)
	Err  error  // optional wrapped cause
}

func (e *ResolveError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Msg, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Msg)
}

func (e *ResolveError) Unwrap() error { return e.Err }

// newResolveError constructs a *ResolveError.
func newResolveError(code, msg string, cause error) *ResolveError {
	return &ResolveError{Code: code, Msg: msg, Err: cause}
}

// ResolveErrorCode extracts the E_* code from any error returned by the
// adapter. If err is not a *ResolveError it returns ("", false) so callers can
// distinguish typed resolution failures from unexpected internal errors.
func ResolveErrorCode(err error) (string, bool) {
	var re *ResolveError
	if errors.As(err, &re) {
		return re.Code, true
	}
	return "", false
}

// --- Structure source constants (mirror the variant_resolutions CHECK) ---

const (
	StructureSourceAlphaFold = "alphafold"
	StructureSourceESMFold   = "esmfold"
	StructureSourcePDB       = "pdb"
)

// --- Input modes (mirror the GenomeJob CRD spec.variant.mode enum) ---

const (
	VariantModeProteinChange = "protein_change"
	VariantModeHGVS          = "hgvs"
	VariantModeRSID          = "rsid"
)

// VariantInput is the adapter's input contract. It is the in-Go projection of
// the GenomeJob CRD `spec.variant` block (core §5.1) and the per-variant object
// in the REST submit body (core §5.4). GEN-11 builds one of these per submitted
// variant; GEN-12 reconstructs it from the persisted CR/calc-job row.
//
// Exactly one mode is honored, selected by Mode. Fields not relevant to the
// chosen mode are ignored. UniProtAcc, when set, pins resolution and bypasses
// gene/rsID->accession lookup (the operator-override path in core §5.2).
type VariantInput struct {
	// ID is the caller-supplied opaque identifier (u4u's "u4u-var-001"); it is
	// echoed for traceability but does NOT participate in the resolution key.
	ID string `json:"id,omitempty"`

	// Mode selects the parsing path: protein_change | hgvs | rsid.
	Mode string `json:"mode"`

	// protein_change mode.
	Gene          string `json:"gene,omitempty"`           // HGNC gene symbol
	Transcript    string `json:"transcript,omitempty"`     // optional Ensembl/RefSeq pin
	ProteinChange string `json:"protein_change,omitempty"` // "p.Arg117His" or "p.R117H" / "R117H"

	// hgvs mode.
	HGVS string `json:"hgvs,omitempty"`

	// rsid mode.
	RSID string `json:"rsid,omitempty"`

	// UniProtAcc optionally pins the accession (operator override). When set,
	// gene/rsID -> accession resolution is skipped; the canonical sequence is
	// still fetched for WT validation.
	UniProtAcc string `json:"uniprot_acc,omitempty"`
}

// normalized is the intermediate produced by mode-specific parsing: the minimal
// substitution coordinates the rest of the pipeline needs. Internal only.
type normalized struct {
	uniprotAcc string // may be empty after parse; filled by UniProt resolution
	isoform    string // "" -> canonical (filled to "<acc>-1" after resolution)
	residue    int    // 1-based
	wtAA       byte   // single-letter
	mutAA      byte   // single-letter
	gene       string // carried for UniProt gene-name lookup
	transcript string
	warnings   []string
}

// ResolvedVariant is the canonical resolution document (core §5.2). It is both
// the JSON payload persisted in variant_resolutions.resolved and the value
// returned to callers. Field names mirror the §5.2 JSON exactly; the flat
// columns of variant_resolutions are derived from these fields at write time.
type ResolvedVariant struct {
	ResolutionID string               `json:"resolution_id"` // "rv-<sha256-16>"
	Input        ResolvedVariantInput `json:"input"`         // echo of submission
	UniProtAcc   string               `json:"uniprot_acc"`
	UniProtIso   string               `json:"uniprot_isoform"`
	Sequence     string               `json:"sequence"` // full WT protein sequence
	SequenceLen  int                  `json:"sequence_length"`
	ResidueIndex int                  `json:"residue_index"` // 1-based in Sequence
	WildTypeAA   string               `json:"wild_type_aa"`  // single-letter
	MutantAA     string               `json:"mutant_aa"`
	StructureSrc string               `json:"structure_source"` // alphafold|esmfold|pdb
	StructureS3  ResolvedStructureS3  `json:"structure_s3"`
	ResolvedAt   string               `json:"resolved_at"` // RFC3339
	Warnings     []string             `json:"warnings,omitempty"`

	// NeedsStructureFold is a non-persisted control signal for GEN-12. It is
	// true when AlphaFold DB had no model and StructureSrc was set to "esmfold"
	// but no structure has been folded yet (StructureS3.Key is empty). The
	// reconcile loop mints an esmfold GenomeJob as a parentJob dependency when
	// this is true. It is omitted from the persisted document.
	NeedsStructureFold bool `json:"-"`
}

// ResolvedVariantInput echoes what was submitted, for traceability (§5.2).
type ResolvedVariantInput struct {
	Mode          string `json:"mode"`
	Gene          string `json:"gene,omitempty"`
	Transcript    string `json:"transcript,omitempty"`
	ProteinChange string `json:"protein_change,omitempty"`
	HGVS          string `json:"hgvs,omitempty"`
	RSID          string `json:"rsid,omitempty"`
}

// ResolvedStructureS3 points at the resolved WT structure in Garage (§5.2).
type ResolvedStructureS3 struct {
	Bucket      string   `json:"bucket"`
	Key         string   `json:"key"`
	PlddtGlobal *float64 `json:"plddt_global"` // nil when source=pdb or fold pending
}

// resolutionKey computes the canonical content-addressing key.
//
//	sha256(acc:isoform:residue:wt>mut)
//
// IMPORTANT: structure_source is deliberately NOT part of the key (deviation
// from the literal core §5.2 formula, which listed it). structure_source flips
// between "alphafold" and "esmfold" as AlphaFold DB coverage changes over time,
// so including it would mint a DIFFERENT resolution_id for the SAME substitution
// depending on where the structure happened to come from -- breaking the "index
// once, reuse forever" guarantee that §5.2's own prose promises ("two different
// inputs that land on the same UniProt residue substitution share a resolution
// and structure"). Keying purely on the substitution makes a single substitution
// map to a single resolution_id regardless of structure source; structure_source
// remains a MUTABLE column on variant_resolutions, set/updated when a structure
// is actually obtained, but never part of identity.
//
// Two different inputs (rsID vs protein_change) that land on the same UniProt
// residue substitution therefore share a resolution_id. Returns (fullKey,
// resolutionID).
func resolutionKey(acc, isoform string, residue int, wtAA, mutAA byte) (string, string) {
	key := fmt.Sprintf("%s:%s:%d:%c>%c", acc, isoform, residue, wtAA, mutAA)
	sum := sha256.Sum256([]byte(key))
	short := hex.EncodeToString(sum[:])[:16]
	return key, "rv-" + short
}

// ResolveVariant is the adapter's exported entrypoint. It resolves a submitted
// variant to a canonical ResolvedVariant, validating the wild-type residue and
// read-through caching against variant_resolutions (global dedup, core Q1).
//
// Flow (core §5.2 resolution algorithm):
//  1. Parse the mode-specific input -> (acc?, residue, wt, mut).
//  2. Resolve UniProt accession + canonical sequence (gene/rsID -> acc; fetch seq).
//  3. Validate sequence[residue-1] == wt -> else E_WT_MISMATCH.
//  4. Resolve structure source: AlphaFold DB hit -> download + store to S3
//     (structure_source=alphafold); miss -> mark NeedsStructureFold + source=esmfold.
//  5. Compute resolution_key/resolution_id; SELECT cache. Hit -> return cached
//     (RecordResolutionCache "hit"). Miss -> INSERT + RecordResolutionCache "miss".
//
// On any failure the returned error is a *ResolveError with a frozen E_* code;
// resolution metrics (outcome/cache/error) are recorded via the GEN-04 helpers.
func ResolveVariant(ctx context.Context, db *DB, s3 S3Client, in VariantInput) (*ResolvedVariant, error) {
	return defaultResolver().Resolve(ctx, db, s3, in)
}

// Resolver carries the (injectable) external-service clients so unit tests can
// substitute fakes. Production callers use ResolveVariant, which builds a
// resolver backed by live UniProt / Ensembl / AlphaFold endpoints.
type Resolver struct {
	uniprot   uniProtResolver
	ensembl   ensemblResolver
	alphafold alphaFoldClient
	now       func() time.Time
}

// defaultResolver wires the live HTTP-backed clients with conservative timeouts.
func defaultResolver() *Resolver {
	return &Resolver{
		uniprot:   newHTTPUniProtResolver(),
		ensembl:   newHTTPEnsemblResolver(),
		alphafold: newHTTPAlphaFoldClient(),
		now:       time.Now,
	}
}

// Resolve is the method form of ResolveVariant; ResolveVariant delegates here so
// tests can drive a Resolver with fake clients.
func (rsv *Resolver) Resolve(ctx context.Context, db *DB, s3 S3Client, in VariantInput) (*ResolvedVariant, error) {
	// Stage 1: parse mode-specific input into substitution coordinates.
	norm, err := rsv.normalize(ctx, in)
	if err != nil {
		recordResolveFailure(ctx, err)
		return nil, err
	}

	// Stage 2: resolve UniProt accession + canonical sequence.
	acc, isoform, sequence, accWarnings, err := rsv.resolveUniProt(ctx, in, norm)
	if err != nil {
		recordResolveFailure(ctx, err)
		return nil, err
	}
	norm.warnings = append(norm.warnings, accWarnings...)

	// Stage 3: WT-AA validation against the canonical sequence (hard gate).
	if err := validateWildType(sequence, norm.residue, norm.wtAA); err != nil {
		recordResolveFailure(ctx, err)
		return nil, err
	}

	// Stage 4 + 5: cache-aware structure resolution + persistence.
	rv, err := rsv.resolveStructureAndCache(ctx, db, s3, in, norm, acc, isoform, sequence)
	if err != nil {
		recordResolveFailure(ctx, err)
		return nil, err
	}
	RecordVariantResolution(ctx, "resolved", rv.StructureSrc)
	return rv, nil
}

// recordResolveFailure emits the rejected-outcome + typed-error metrics for a
// resolution failure. Non-typed (unexpected) errors are bucketed as upstream so
// the dashboard still counts them.
func recordResolveFailure(ctx context.Context, err error) {
	code, ok := ResolveErrorCode(err)
	if !ok {
		code = ECodeResolveUpstream
	}
	RecordResolutionError(ctx, code)
	RecordVariantResolution(ctx, "rejected", "")
}

// resolveStructureAndCache performs the cache-aware structure resolution and
// persistence (stages 4-5). It first probes the cache for an already-resolved
// alphafold- or esmfold-source row (warm path, no network), and only on a miss
// performs live AlphaFold resolution + INSERT.
func (rsv *Resolver) resolveStructureAndCache(
	ctx context.Context, db *DB, s3 S3Client, in VariantInput,
	norm normalized, acc, isoform, sequence string,
) (*ResolvedVariant, error) {
	// Warm-cache probe: a single substitution maps to a single source-independent
	// resolution_id, so one lookup covers every prior resolution of this variant
	// regardless of which structure source it landed on. No more three-candidate
	// loop -- alphafold, esmfold, and pdb all converge on the same id now.
	_, resID := resolutionKey(acc, isoform, norm.residue, norm.wtAA, norm.mutAA)
	cached, err := loadResolution(ctx, db, resID)
	if err != nil {
		return nil, newResolveError(ECodeResolveUpstream, "cache lookup failed", err)
	}
	if cached != nil {
		// Convergence: if a prior resolution marked this variant for ESMFold
		// fallback but no structure has actually been stored yet (fold still
		// pending), AlphaFold DB may have gained coverage since. Re-probe and, on
		// a hit, upgrade the SAME row in place (structure_source -> alphafold +
		// real key/plddt) rather than minting a new resolution. Otherwise the
		// cached document stands.
		// NeedsStructureFold is non-persisted (json:"-"), so a reloaded row reports
		// false; the persisted signal for "esmfold marker, fold not yet complete"
		// is source==esmfold with no global pLDDT recorded (the worker sets
		// plddt_global only on fold completion).
		if cached.StructureSrc == StructureSourceESMFold && cached.StructureS3.PlddtGlobal == nil {
			upgraded, err := rsv.upgradeESMFoldIfAlphaFoldNow(ctx, db, s3, cached)
			if err != nil {
				return nil, err
			}
			RecordResolutionCache(ctx, "hit")
			return upgraded, nil
		}
		RecordResolutionCache(ctx, "hit")
		return cached, nil
	}
	RecordResolutionCache(ctx, "miss")

	// Cold path: determine the structure source via AlphaFold DB, then persist.
	rv := &ResolvedVariant{
		Input: ResolvedVariantInput{
			Mode:          in.Mode,
			Gene:          in.Gene,
			Transcript:    in.Transcript,
			ProteinChange: in.ProteinChange,
			HGVS:          in.HGVS,
			RSID:          in.RSID,
		},
		UniProtAcc:   acc,
		UniProtIso:   isoform,
		Sequence:     sequence,
		SequenceLen:  len(sequence),
		ResidueIndex: norm.residue,
		WildTypeAA:   string(norm.wtAA),
		MutantAA:     string(norm.mutAA),
		ResolvedAt:   rsv.now().UTC().Format(time.RFC3339),
		Warnings:     norm.warnings,
	}

	// The content-addressed identity is source-independent, so it is fixed BEFORE
	// structure resolution. resolveStructure uses rv.ResolutionID directly for the
	// S3 key, so an AlphaFold hit and a later esmfold fallback for the same
	// substitution share one resolution_id and one structure path.
	resKey, resID := resolutionKey(acc, isoform, norm.residue, norm.wtAA, norm.mutAA)
	rv.ResolutionID = resID

	if err := rsv.resolveStructure(ctx, s3, rv); err != nil {
		return nil, err
	}

	if err := persistResolution(ctx, db, resKey, rv); err != nil {
		return nil, newResolveError(ECodeResolveUpstream, "persisting resolution", err)
	}
	return rv, nil
}

// upgradeESMFoldIfAlphaFoldNow re-probes AlphaFold DB for a cached variant that
// was previously marked for ESMFold fallback but whose fold has not completed
// (no global pLDDT). If AlphaFold now has a model, it stores it and updates the
// SAME variant_resolutions row in place (structure_source -> alphafold, real
// structure_key + plddt) so the variant converges on one row instead of minting
// a competing resolution. On a continued miss (or upstream error treated as a
// miss for the warm path) the cached esmfold marker is returned unchanged.
func (rsv *Resolver) upgradeESMFoldIfAlphaFoldNow(
	ctx context.Context, db *DB, s3 S3Client, cached *ResolvedVariant,
) (*ResolvedVariant, error) {
	pdb, plddt, ok, err := rsv.alphafold.FetchModel(ctx, cached.UniProtAcc)
	if err != nil || !ok {
		// AlphaFold still has no usable model (or a transient upstream error);
		// leave the esmfold fallback in place. The GEN-12 reconcile continues to
		// drive the fold via NeedsStructureFold on a fresh resolve.
		return cached, nil
	}
	key := structureKey(cached.ResolutionID)
	if err := s3.PutArtifact(ctx, BucketStructures, key, bytes.NewReader(pdb), "chemical/x-pdb"); err != nil {
		return nil, newResolveError(ECodeResolveUpstream,
			fmt.Sprintf("storing upgraded AlphaFold structure for %s to S3", cached.UniProtAcc), err)
	}
	cached.StructureSrc = StructureSourceAlphaFold
	cached.StructureS3 = ResolvedStructureS3{Bucket: BucketStructures, Key: key, PlddtGlobal: plddt}
	cached.NeedsStructureFold = false
	if err := updateResolutionStructure(ctx, db, cached); err != nil {
		return nil, newResolveError(ECodeResolveUpstream, "updating converged resolution", err)
	}
	return cached, nil
}

// validateWildType enforces sequence[residue-1] == wtAA (core §5.2 step 2).
// Out-of-range residue indices and mismatches both fail closed with
// E_WT_MISMATCH -- never silently proceed against drifted coordinates.
func validateWildType(sequence string, residue int, wtAA byte) error {
	if residue < 1 || residue > len(sequence) {
		return newResolveError(ECodeWTMismatch,
			fmt.Sprintf("residue_index %d out of range for sequence length %d", residue, len(sequence)), nil)
	}
	got := sequence[residue-1]
	if got != wtAA {
		return newResolveError(ECodeWTMismatch,
			fmt.Sprintf("wild_type_aa mismatch at residue %d: submitted %c, canonical sequence has %c",
				residue, wtAA, got), nil)
	}
	return nil
}

// --- Persistence helpers (variant_resolutions cache) ---

// loadResolution reads a cached resolution by resolution_id. Returns (nil, nil)
// on cache miss. The persisted `resolved` JSONB is the canonical document.
func loadResolution(ctx context.Context, db *DB, resolutionID string) (*ResolvedVariant, error) {
	var resolvedJSON string
	err := db.QueryRowContext(ctx,
		`SELECT resolved FROM variant_resolutions WHERE resolution_id = ?`, resolutionID).
		Scan(&resolvedJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("querying variant_resolutions for %s: %w", resolutionID, err)
	}
	var rv ResolvedVariant
	if err := json.Unmarshal([]byte(resolvedJSON), &rv); err != nil {
		return nil, fmt.Errorf("unmarshaling cached resolution %s: %w", resolutionID, err)
	}
	return &rv, nil
}

// persistResolution writes a resolved variant into variant_resolutions. The
// flat columns are derived from the ResolvedVariant; `resolved` stores the full
// §5.2 document. ON CONFLICT DO NOTHING makes concurrent resolvers of the same
// content-addressed key idempotent (the unique indexes on resolution_id /
// resolution_key guarantee a single canonical row).
func persistResolution(ctx context.Context, db *DB, resolutionKeyStr string, rv *ResolvedVariant) error {
	doc, err := json.Marshal(rv)
	if err != nil {
		return fmt.Errorf("marshaling resolution document: %w", err)
	}
	var plddt interface{}
	if rv.StructureS3.PlddtGlobal != nil {
		plddt = *rv.StructureS3.PlddtGlobal
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO variant_resolutions
		   (resolution_id, resolution_key, uniprot_acc, uniprot_isoform, residue_index,
		    wild_type_aa, mutant_aa, sequence_length, structure_source,
		    structure_bucket, structure_key, plddt_global, resolved)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (resolution_id) DO NOTHING`,
		rv.ResolutionID, resolutionKeyStr, rv.UniProtAcc, rv.UniProtIso, rv.ResidueIndex,
		rv.WildTypeAA, rv.MutantAA, rv.SequenceLen, rv.StructureSrc,
		rv.StructureS3.Bucket, rv.StructureS3.Key, plddt, string(doc))
	if err != nil {
		return fmt.Errorf("inserting variant_resolutions row %s: %w", rv.ResolutionID, err)
	}
	return nil
}

// updateResolutionStructure mutates the structure-bearing columns of an existing
// variant_resolutions row in place, keyed by the (immutable) resolution_id. It is
// used when a previously esmfold-marked variant converges onto an AlphaFold model
// that became available later: structure_source flips alphafold, and
// structure_key/plddt_global/resolved are refreshed. resolution_id and
// resolution_key never change -- identity is source-independent.
func updateResolutionStructure(ctx context.Context, db *DB, rv *ResolvedVariant) error {
	doc, err := json.Marshal(rv)
	if err != nil {
		return fmt.Errorf("marshaling resolution document: %w", err)
	}
	var plddt interface{}
	if rv.StructureS3.PlddtGlobal != nil {
		plddt = *rv.StructureS3.PlddtGlobal
	}
	_, err = db.ExecContext(ctx,
		`UPDATE variant_resolutions
		    SET structure_source = ?, structure_bucket = ?, structure_key = ?,
		        plddt_global = ?, resolved = ?
		  WHERE resolution_id = ?`,
		rv.StructureSrc, rv.StructureS3.Bucket, rv.StructureS3.Key, plddt, string(doc),
		rv.ResolutionID)
	if err != nil {
		return fmt.Errorf("updating variant_resolutions row %s: %w", rv.ResolutionID, err)
	}
	return nil
}

// trimProteinPrefix trims a leading "p." and surrounding whitespace.
func trimProteinPrefix(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimPrefix(s, "p.")
}
