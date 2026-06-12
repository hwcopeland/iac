// handlers_genome.go implements the u4u-facing REST surface for the Khemeia
// genomics structural-biophysics tooling layer (core TDD §5.4). It mirrors
// handlers_docking_v2.go conventions exactly: a single dispatch router that
// trims the path prefix and branches on method + suffix, batch-native submit
// returning 202, async status polling, and paginated results.
//
// Endpoints (all wrapped with AuthMiddleware.Wrap in main.go, gated behind
// GENOME_ENABLED):
//
//	POST /api/v1/genome/variant/submit                       — batch submit (202)
//	GET  /api/v1/genome/jobs/{group_name}                    — group status rollup
//	GET  /api/v1/genome/jobs/{group_name}/results            — paginated §6 results
//	GET  /api/v1/genome/variant/{resolution_id}/structure    — presigned structure URL
//
// Orchestration model (core Decision B1, GEN-11 constraint): submit is the
// source of truth for the DB rows ONLY. It writes the variant_jobs parent row
// and one variant_calc_jobs child per accepted (variant × calculation). The
// CRDController (GEN-12) mints the GenomeJob CRs from those rows and reconciles
// them; this handler NEVER creates a CR. Full HTTP variant resolution
// (ResolveVariant) is likewise deferred to the GEN-12 reconcile resolve stage —
// submit only does cheap, offline per-variant validation (mode + required
// fields + per-calc params) so a single bad variant never fails the batch.
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// --- Calculation enum (mirrors variant_calc_jobs.calculation CHECK in
// genome_schema.go and the GenomeJob CRD spec.calculation enum minus the
// controller-internal "resolve" stage, which is never submitted directly). ---

// genomeCalculations is the set of u4u-submittable calculations. "resolve" is
// the adapter-only controller-internal stage (core §5.1) and is intentionally
// excluded — it is not a value a caller may request, and the schema CHECK on
// variant_calc_jobs.calculation rejects it.
var genomeCalculations = map[string]bool{
	"esmfold":          true,
	"ddg_stability":    true,
	"pocket_proximity": true,
	"pgx_docking":      true,
}

// genomeDefaultMaxBatch caps the (variants × calculations) fan-out of a single
// submit (VULN-3). 500 variant×calc units is a generous real-batch ceiling while
// still bounding the apiserver-object / GPU-pod blast radius of one request. The
// operator can override it with GENOME_MAX_BATCH.
const genomeDefaultMaxBatch = 500

// genomeMaxBatch returns the configured per-submit variant×calc cap. A non-numeric
// or non-positive GENOME_MAX_BATCH falls back to the default rather than disabling
// the guard, so a typo can never silently uncap the endpoint.
func genomeMaxBatch() int {
	if v := os.Getenv("GENOME_MAX_BATCH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return genomeDefaultMaxBatch
}

// --- Request / response types (core §5.4) ---

// GenomeSubmitRequest is the JSON body for POST /api/v1/genome/variant/submit.
// It is batch-native: the cartesian product of variants[] × calculations[]
// becomes one variant_calc_jobs row per accepted pair.
type GenomeSubmitRequest struct {
	Variants     []VariantInput             `json:"variants"`
	Calculations []string                   `json:"calculations"`
	Params       map[string]json.RawMessage `json:"params,omitempty"` // per-calc params keyed by calculation name
	CallbackURL  string                     `json:"callback_url,omitempty"`
	Priority     string                     `json:"priority,omitempty"` // "normal" | "high"
}

// GenomeSubmitResponse is the 202 Accepted body. accepted[]/rejected[] partition
// the submitted variants by cheap validation outcome (core §5.4).
type GenomeSubmitResponse struct {
	GroupName    string          `json:"group_name"`
	Status       string          `json:"status"`
	VariantCount int             `json:"variant_count"`
	CalcCount    int             `json:"calc_count"`
	Accepted     []string        `json:"accepted"`
	Rejected     []RejectedEntry `json:"rejected"`
}

// RejectedEntry is a per-variant rejection carrying a frozen E_* code
// (core Appendix A) rather than failing the whole batch.
type RejectedEntry struct {
	ID    string `json:"id"`
	Error string `json:"error"` // E_* code
	Msg   string `json:"message,omitempty"`
}

// GenomeGroupStatus is the response for GET /api/v1/genome/jobs/{group_name}.
type GenomeGroupStatus struct {
	GroupName    string            `json:"group_name"`
	Status       string            `json:"status"`
	Calculations []string          `json:"calculations"`
	Totals       GenomeTotals      `json:"totals"`
	PerCalc      []PerCalcProgress `json:"per_calc"`
	SubmittedBy  *string           `json:"submitted_by,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	StartedAt    *time.Time        `json:"started_at,omitempty"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
}

// GenomeTotals is the aggregate calc-job rollup for a group.
type GenomeTotals struct {
	CalcJobs  int `json:"calc_jobs"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

// PerCalcProgress is the per-calculation rollup within a group.
type PerCalcProgress struct {
	Calculation string `json:"calculation"`
	Completed   int    `json:"completed"`
	Total       int    `json:"total"`
	Status      string `json:"status,omitempty"`
}

// GenomeResultsResponse is the paginated results body (core §5.4). Each entry in
// Results is a §6 result envelope; Failures carries per-(variant,calc) typed errors.
type GenomeResultsResponse struct {
	GroupName    string          `json:"group_name"`
	TotalResults int             `json:"total_results"`
	Page         int             `json:"page"`
	PerPage      int             `json:"per_page"`
	Complete     bool            `json:"complete"`
	Results      []GenomeResult  `json:"results"`
	Failures     []GenomeFailure `json:"failures"`
}

// GenomeResult is the shared result envelope (core §6). payload differs per calc
// and is passed through verbatim from variant_results.payload (JSONB).
type GenomeResult struct {
	VariantKey      string          `json:"variant_key"`
	ResolutionID    string          `json:"resolution_id"`
	Calculation     string          `json:"calculation"`
	StructureSource *string         `json:"structure_source,omitempty"`
	Confidence      *float64        `json:"confidence,omitempty"`
	Status          string          `json:"status"`
	Payload         json.RawMessage `json:"payload"`
	ArtifactKeys    json.RawMessage `json:"artifact_keys,omitempty"`
}

// GenomeFailure is a per-(variant,calc) failure surfaced in results (core §5.4).
type GenomeFailure struct {
	VariantKey  string `json:"variant_key"`
	Calculation string `json:"calculation"`
	Error       string `json:"error"`
}

// GenomeStructureResponse is the JSON form of the structure endpoint when the
// caller does not request a redirect (core §5.4).
type GenomeStructureResponse struct {
	ResolutionID    string `json:"resolution_id"`
	Bucket          string `json:"bucket"`
	Key             string `json:"key"`
	StructureSource string `json:"structure_source"`
	URL             string `json:"url"`
	ExpiresInSecs   int    `json:"expires_in_secs"`
}

// --- Dispatch ---

// GenomeDispatch routes /api/v1/genome/ requests (mirrors DockingV2Dispatch).
func (h *APIHandler) GenomeDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/genome/")
	path = strings.TrimRight(path, "/")

	// POST /api/v1/genome/variant/submit
	if r.Method == http.MethodPost && path == "variant/submit" {
		h.GenomeSubmit(w, r)
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// GET /api/v1/genome/variant/{resolution_id}/structure
	if strings.HasPrefix(path, "variant/") && strings.HasSuffix(path, "/structure") {
		h.GenomeStructure(w, r)
		return
	}

	// GET /api/v1/genome/jobs/{group_name}/results
	if strings.HasPrefix(path, "jobs/") && strings.HasSuffix(path, "/results") {
		h.GenomeResults(w, r)
		return
	}

	// GET /api/v1/genome/jobs/{group_name}
	if strings.HasPrefix(path, "jobs/") {
		h.GenomeGroupStatusHandler(w, r)
		return
	}

	writeError(w, "not found", http.StatusNotFound)
}

// --- Submit ---

// GenomeSubmit handles POST /api/v1/genome/variant/submit. It performs cheap,
// offline per-variant validation (mode + required fields + per-calc params),
// partitions accepted/rejected, writes the variant_jobs parent row and one
// variant_calc_jobs child per accepted (variant × calculation), and returns 202.
// CR minting and full HTTP resolution are deferred to the GEN-12 reconcile.
func (h *APIHandler) GenomeSubmit(w http.ResponseWriter, r *http.Request) {
	var req GenomeSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if len(req.Variants) == 0 {
		writeError(w, "variants[] is required and must be non-empty", http.StatusBadRequest)
		return
	}
	if len(req.Calculations) == 0 {
		writeError(w, "calculations[] is required and must be non-empty", http.StatusBadRequest)
		return
	}

	// Validate the calculation set against the enum (whole-request gate, like
	// DockingV2Submit's engine validation). One bad calculation is a 400 — it
	// applies to every variant, so it is not a per-variant rejection.
	for _, calc := range req.Calculations {
		if !genomeCalculations[calc] {
			writeError(w, fmt.Sprintf("unknown calculation %q (valid: esmfold, ddg_stability, pocket_proximity, pgx_docking)", calc),
				http.StatusBadRequest)
			return
		}
	}

	// Validate per-calc params shape at submit (core §5.4 / R5), exactly as
	// DockingV2Submit validates engine params. Malformed params for a requested
	// calculation are a whole-request 400 (they affect every variant).
	if err := validateGenomeParams(req.Calculations, req.Params); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Priority != "" && req.Priority != "normal" && req.Priority != "high" {
		writeError(w, "priority must be 'normal' or 'high'", http.StatusBadRequest)
		return
	}

	// VULN-3 (DoS): bound the batch fan-out. Each (variant × calculation) becomes a
	// variant_calc_jobs row AND a GenomeJob CR (one apiserver object + at least one
	// reconcile + potentially a GPU pod), so an unbounded cartesian product is a cheap
	// way to exhaust the apiserver / GPU node. Reject over-cap before any DB write or CR
	// mint. The cap counts the REQUESTED product (len(variants) × len(calculations)),
	// not just accepted, so the gate cannot be bypassed by padding with invalid variants.
	maxBatch := genomeMaxBatch()
	requestedUnits := len(req.Variants) * len(req.Calculations)
	if requestedUnits > maxBatch {
		writeError(w, fmt.Sprintf(
			"batch too large: %d variant×calc units requested exceeds the server cap of %d "+
				"(reduce variants[] or calculations[], or raise GENOME_MAX_BATCH)",
			requestedUnits, maxBatch), http.StatusRequestEntityTooLarge)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	// Partition variants by cheap validation. Accepted variants get a stable
	// variant_key; rejected variants carry a typed E_* code.
	accepted := make([]VariantInput, 0, len(req.Variants))
	acceptedIDs := make([]string, 0, len(req.Variants))
	rejected := make([]RejectedEntry, 0)

	for i, v := range req.Variants {
		key := variantKey(v, i)
		if code, msg, ok := validateVariantInput(v); !ok {
			rejected = append(rejected, RejectedEntry{ID: key, Error: code, Msg: msg})
			continue
		}
		accepted = append(accepted, v)
		acceptedIDs = append(acceptedIDs, key)
	}

	// If nothing survived validation, there is no group to create — surface the
	// rejections directly with a 422 so the caller sees why (still structured,
	// never a silent drop).
	if len(accepted) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(GenomeSubmitResponse{
			Status:       "Rejected",
			VariantCount: 0,
			CalcCount:    0,
			Accepted:     []string{},
			Rejected:     rejected,
		})
		return
	}

	groupName := fmt.Sprintf("genome-%d", time.Now().UnixNano())
	submittedBy := nullableUser(UserFromContext(r))
	calcCount := len(accepted) * len(req.Calculations)

	calcsJSON, _ := json.Marshal(req.Calculations)
	inputJSON, _ := json.Marshal(req)
	callbackURL := nullableString(req.CallbackURL)

	// Write the parent batch row.
	if _, err := db.ExecContext(r.Context(),
		`INSERT INTO variant_jobs
			(group_name, status, calculations, variant_count, calc_count,
			 submitted_by, callback_url, input_data)
		 VALUES (?, 'Pending', ?, ?, ?, ?, ?, ?)`,
		groupName, string(calcsJSON), len(accepted), calcCount,
		submittedBy, callbackURL, string(inputJSON)); err != nil {
		writeError(w, fmt.Sprintf("failed to create group: %v", err), http.StatusInternalServerError)
		return
	}

	// Write one variant_calc_jobs child per accepted (variant × calculation).
	// The controller (GEN-12) picks these up and mints GenomeJob CRs.
	for i, v := range accepted {
		key := acceptedIDs[i]
		for _, calc := range req.Calculations {
			var paramsArg interface{}
			if raw, ok := req.Params[calc]; ok && len(raw) > 0 {
				paramsArg = string(raw)
			}
			if _, err := db.ExecContext(r.Context(),
				`INSERT INTO variant_calc_jobs
					(group_name, variant_key, calculation, status, params)
				 VALUES (?, ?, ?, 'Pending', ?)`,
				groupName, key, calc, paramsArg); err != nil {
				// A duplicate (group, variant_key, calc) is unexpected within a
				// single submit; log-and-continue keeps the batch resilient.
				writeError(w, fmt.Sprintf("failed to create calc job for %s/%s: %v", key, calc, err),
					http.StatusInternalServerError)
				return
			}
		}
		_ = v
	}

	// Mint a GenomeJob CR per accepted (variant × calc) row and stamp its name back
	// into variant_calc_jobs.cr_name (Decision B1). The CRDController is informer-
	// driven over existing CRs; without this synchronous dispatch the rows would sit
	// Pending forever. Mirrors the docking/advance submit-time create
	// (handlers_crd.go) rather than a poll loop. A per-row mint failure is logged and
	// leaves that row Pending/cr_name NULL for a resubmit; it never fails the batch.
	h.dispatchGenomeGroup(r.Context(), groupName, accepted, acceptedIDs, req.Calculations, req.Params)

	// Metrics: count accepted variants + the group (core §5.4 / GEN-04 helpers).
	RecordVariantSubmitted(r.Context(), len(accepted))
	RecordGenomeGroup(r.Context(), "Pending")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(GenomeSubmitResponse{
		GroupName:    groupName,
		Status:       "Pending",
		VariantCount: len(accepted),
		CalcCount:    calcCount,
		Accepted:     acceptedIDs,
		Rejected:     rejected,
	})
}

// validateVariantInput performs cheap, offline validation of a submitted variant
// (mode present + required fields for that mode). It does NOT resolve against
// UniProt/AlphaFold — that is the GEN-12 reconcile resolve stage. Returns a
// frozen E_* code (core Appendix A) on rejection.
func validateVariantInput(v VariantInput) (code, msg string, ok bool) {
	switch v.Mode {
	case VariantModeProteinChange:
		if strings.TrimSpace(v.Gene) == "" && strings.TrimSpace(v.UniProtAcc) == "" {
			return ECodeParamsInvalid, "protein_change mode requires gene or uniprot_acc", false
		}
		if strings.TrimSpace(v.ProteinChange) == "" {
			return ECodeParamsInvalid, "protein_change mode requires protein_change (e.g. p.Arg117His)", false
		}
	case VariantModeHGVS:
		if strings.TrimSpace(v.HGVS) == "" {
			return ECodeParamsInvalid, "hgvs mode requires hgvs", false
		}
	case VariantModeRSID:
		if strings.TrimSpace(v.RSID) == "" {
			return ECodeParamsInvalid, "rsid mode requires rsid", false
		}
	case "":
		return ECodeParamsInvalid, "variant.mode is required (protein_change|hgvs|rsid)", false
	default:
		return ECodeParamsInvalid, fmt.Sprintf("unknown variant.mode %q", v.Mode), false
	}
	return "", "", true
}

// validateGenomeParams shape-checks the per-calc params object at submit (core
// §5.4 / R5). It only verifies that each provided params entry is well-formed
// JSON keyed by a requested calculation; deep per-calc field validation belongs
// to the worker. Unknown calculation keys in params are an error (likely a typo).
func validateGenomeParams(calculations []string, params map[string]json.RawMessage) error {
	if len(params) == 0 {
		return nil
	}
	requested := make(map[string]bool, len(calculations))
	for _, c := range calculations {
		requested[c] = true
	}
	for calc, raw := range params {
		if !genomeCalculations[calc] {
			return fmt.Errorf("params references unknown calculation %q", calc)
		}
		if !requested[calc] {
			return fmt.Errorf("params references calculation %q not in calculations[]", calc)
		}
		// Must be a JSON object (not an array/scalar) so the worker can read fields.
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return fmt.Errorf("params.%s must be a JSON object: %v", calc, err)
		}
		// VULN-3 (DoS, second clause): clamp the one well-known unbounded numeric
		// search param. exhaustiveness scales docking compute ~linearly, so an
		// attacker-supplied 10_000_000 turns one pgx_docking unit into an effectively
		// infinite GPU job. Deep per-calc validation is the worker's job, but a cheap
		// upper clamp here is a trivial, high-value guard. The clamp REWRITES the
		// stored params (map is mutated in place) so the capped value is what reaches
		// both the DB row and the CR. Other numeric params stay the worker's concern.
		if clamped, ok := clampExhaustiveness(obj); ok {
			newRaw, err := json.Marshal(obj)
			if err != nil {
				return fmt.Errorf("params.%s re-encode after clamp: %v", calc, err)
			}
			params[calc] = newRaw
			_ = clamped
		}
	}
	return nil
}

// genomeMaxExhaustiveness is the server-side ceiling for the docking search
// `exhaustiveness` param surfaced through genome params (VULN-3). It matches the
// docking default (32) headroom while bounding worst-case GPU time.
const genomeMaxExhaustiveness = 64

// clampExhaustiveness caps params["exhaustiveness"] to genomeMaxExhaustiveness in
// place, returning the clamped value and whether a clamp occurred. A non-numeric
// or absent value is left untouched (the worker rejects malformed params).
func clampExhaustiveness(obj map[string]json.RawMessage) (int, bool) {
	raw, ok := obj["exhaustiveness"]
	if !ok {
		return 0, false
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false // not a number; leave for the worker to reject
	}
	if int(n) <= genomeMaxExhaustiveness {
		return 0, false
	}
	clamped := genomeMaxExhaustiveness
	obj["exhaustiveness"] = json.RawMessage(strconv.Itoa(clamped))
	return clamped, true
}

// variantKey derives the stable per-variant key persisted in variant_calc_jobs.
// It prefers the caller-supplied id; otherwise it falls back to a canonical
// "GENE:proteinChange" / "rsid" form, with an index suffix to guarantee
// uniqueness within a single submit (core §5.3 variant_key column).
func variantKey(v VariantInput, idx int) string {
	if id := strings.TrimSpace(v.ID); id != "" {
		return id
	}
	switch v.Mode {
	case VariantModeProteinChange:
		g := strings.TrimSpace(v.Gene)
		if g == "" {
			g = strings.TrimSpace(v.UniProtAcc)
		}
		return fmt.Sprintf("%s:%s", g, strings.TrimSpace(v.ProteinChange))
	case VariantModeHGVS:
		return strings.TrimSpace(v.HGVS)
	case VariantModeRSID:
		return strings.TrimSpace(v.RSID)
	default:
		return fmt.Sprintf("variant-%d", idx)
	}
}

// --- Group status ---

// GenomeGroupStatusHandler handles GET /api/v1/genome/jobs/{group_name}. It rolls
// up variant_calc_jobs statuses into an aggregate + per-calc breakdown.
func (h *APIHandler) GenomeGroupStatusHandler(w http.ResponseWriter, r *http.Request) {
	groupName := groupNameFromPath(r.URL.Path, "")
	if groupName == "" {
		writeError(w, "group name required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var resp GenomeGroupStatus
	var calcsJSON string
	var submittedBy sql.NullString
	var startedAt, completedAt sql.NullTime

	err := db.QueryRowContext(r.Context(),
		`SELECT group_name, status, calculations, submitted_by, created_at, started_at, completed_at
		 FROM variant_jobs WHERE group_name = ?`, groupName).Scan(
		&resp.GroupName, &resp.Status, &calcsJSON, &submittedBy,
		&resp.CreatedAt, &startedAt, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, "group not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query group: %v", err), http.StatusInternalServerError)
		return
	}

	json.Unmarshal([]byte(calcsJSON), &resp.Calculations)
	if submittedBy.Valid {
		resp.SubmittedBy = &submittedBy.String
	}
	if startedAt.Valid {
		resp.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		resp.CompletedAt = &completedAt.Time
	}

	// Per-(calculation, status) counts across the group's calc jobs.
	rows, err := db.QueryContext(r.Context(),
		`SELECT calculation, status, COUNT(*)
		 FROM variant_calc_jobs WHERE group_name = ?
		 GROUP BY calculation, status`, groupName)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query calc jobs: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// totals over the whole group; per-calc maps for the breakdown.
	perCalcTotal := map[string]int{}
	perCalcCompleted := map[string]int{}
	perCalcActive := map[string]bool{} // any Running/Resolving/Pending row
	for rows.Next() {
		var calc, status string
		var n int
		if err := rows.Scan(&calc, &status, &n); err != nil {
			continue
		}
		resp.Totals.CalcJobs += n
		perCalcTotal[calc] += n
		switch status {
		case "Completed":
			resp.Totals.Completed += n
			perCalcCompleted[calc] += n
		case "Failed":
			resp.Totals.Failed += n
		case "Skipped":
			resp.Totals.Skipped += n
		case "Running", "Resolving", "Pending":
			perCalcActive[calc] = true
		}
	}

	// Build per-calc progress in the group's declared calculation order.
	for _, calc := range resp.Calculations {
		total, ok := perCalcTotal[calc]
		if !ok {
			continue
		}
		entry := PerCalcProgress{
			Calculation: calc,
			Completed:   perCalcCompleted[calc],
			Total:       total,
		}
		if perCalcActive[calc] {
			entry.Status = "Running"
		}
		resp.PerCalc = append(resp.PerCalc, entry)
	}
	if resp.PerCalc == nil {
		resp.PerCalc = []PerCalcProgress{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// --- Results ---

// GenomeResults handles GET /api/v1/genome/jobs/{group_name}/results. It returns
// paginated §6 result envelopes plus a failures[] list, with an optional
// ?calculation= filter. Pagination mirrors DockingV2Results (page / per_page).
func (h *APIHandler) GenomeResults(w http.ResponseWriter, r *http.Request) {
	groupName := groupNameFromPath(r.URL.Path, "/results")
	if groupName == "" {
		writeError(w, "group name required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	// Verify the group exists (404 otherwise — distinct from an empty page).
	var exists int
	err := db.QueryRowContext(r.Context(),
		`SELECT 1 FROM variant_jobs WHERE group_name = ?`, groupName).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, "group not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query group: %v", err), http.StatusInternalServerError)
		return
	}

	page, perPage := paginationParams(r, 100, 200)
	calcFilter := strings.TrimSpace(r.URL.Query().Get("calculation"))
	if calcFilter != "" && !genomeCalculations[calcFilter] {
		writeError(w, fmt.Sprintf("unknown calculation filter %q", calcFilter), http.StatusBadRequest)
		return
	}

	// Count matching results for pagination + total.
	countQuery := `SELECT COUNT(*) FROM variant_results WHERE group_name = ?`
	countArgs := []interface{}{groupName}
	if calcFilter != "" {
		countQuery += ` AND calculation = ?`
		countArgs = append(countArgs, calcFilter)
	}
	var totalResults int
	if err := db.QueryRowContext(r.Context(), countQuery, countArgs...).Scan(&totalResults); err != nil {
		writeError(w, fmt.Sprintf("failed to count results: %v", err), http.StatusInternalServerError)
		return
	}

	// Page of result envelopes (§6).
	offset := (page - 1) * perPage
	resQuery := `SELECT variant_key, resolution_id, calculation, structure_source,
	                    confidence, payload, artifact_keys
	             FROM variant_results WHERE group_name = ?`
	resArgs := []interface{}{groupName}
	if calcFilter != "" {
		resQuery += ` AND calculation = ?`
		resArgs = append(resArgs, calcFilter)
	}
	resQuery += ` ORDER BY id ASC LIMIT ? OFFSET ?`
	resArgs = append(resArgs, perPage, offset)

	rows, err := db.QueryContext(r.Context(), resQuery, resArgs...)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query results: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	results := make([]GenomeResult, 0, perPage)
	for rows.Next() {
		var gr GenomeResult
		var structureSource sql.NullString
		var confidence sql.NullFloat64
		var payload, artifactKeys sql.NullString
		if err := rows.Scan(&gr.VariantKey, &gr.ResolutionID, &gr.Calculation,
			&structureSource, &confidence, &payload, &artifactKeys); err != nil {
			continue
		}
		gr.Status = "Completed" // only completed calcs land in variant_results
		if structureSource.Valid {
			gr.StructureSource = &structureSource.String
		}
		if confidence.Valid {
			gr.Confidence = &confidence.Float64
		}
		if payload.Valid {
			gr.Payload = json.RawMessage(payload.String)
		} else {
			gr.Payload = json.RawMessage("{}")
		}
		if artifactKeys.Valid && artifactKeys.String != "" {
			gr.ArtifactKeys = json.RawMessage(artifactKeys.String)
		}
		results = append(results, gr)
	}

	// Failures: terminal Failed calc jobs (carry the typed error in error_output).
	failures := make([]GenomeFailure, 0)
	failQuery := `SELECT variant_key, calculation, error_output
	              FROM variant_calc_jobs
	              WHERE group_name = ? AND status = 'Failed'`
	failArgs := []interface{}{groupName}
	if calcFilter != "" {
		failQuery += ` AND calculation = ?`
		failArgs = append(failArgs, calcFilter)
	}
	frows, ferr := db.QueryContext(r.Context(), failQuery, failArgs...)
	if ferr == nil {
		defer frows.Close()
		for frows.Next() {
			var f GenomeFailure
			var errOut sql.NullString
			if err := frows.Scan(&f.VariantKey, &f.Calculation, &errOut); err != nil {
				continue
			}
			if errOut.Valid {
				f.Error = errOut.String
			}
			failures = append(failures, f)
		}
	}

	// Complete only when no calc job in the group is still non-terminal.
	var pendingCount int
	db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM variant_calc_jobs
		 WHERE group_name = ? AND status IN ('Pending','Resolving','Running')`,
		groupName).Scan(&pendingCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GenomeResultsResponse{
		GroupName:    groupName,
		TotalResults: totalResults,
		Page:         page,
		PerPage:      perPage,
		Complete:     pendingCount == 0,
		Results:      results,
		Failures:     failures,
	})
}

// --- Structure ---

// GenomeStructure handles GET /api/v1/genome/variant/{resolution_id}/structure.
// It looks up the resolved structure's bucket/key in variant_resolutions and
// returns a presigned URL. A ?redirect=true query issues a 302 to the URL;
// otherwise the URL is returned as JSON (core §5.4).
func (h *APIHandler) GenomeStructure(w http.ResponseWriter, r *http.Request) {
	// Path: variant/{resolution_id}/structure
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/genome/variant/")
	path = strings.TrimRight(path, "/")
	resolutionID := strings.TrimSuffix(path, "/structure")
	resolutionID = strings.TrimRight(resolutionID, "/")
	if resolutionID == "" || strings.Contains(resolutionID, "/") {
		writeError(w, "resolution_id required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var bucket, key, structureSource string
	err := db.QueryRowContext(r.Context(),
		`SELECT structure_bucket, structure_key, structure_source
		 FROM variant_resolutions WHERE resolution_id = ?`, resolutionID).
		Scan(&bucket, &key, &structureSource)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, "resolution not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query resolution: %v", err), http.StatusInternalServerError)
		return
	}
	if key == "" {
		// Structure not yet folded (ESMFold fallback pending) — not an error,
		// but there is nothing to presign yet.
		writeError(w, "structure not yet available for this resolution (fold pending)", http.StatusConflict)
		return
	}

	if h.s3Client == nil {
		writeError(w, "object storage unavailable", http.StatusInternalServerError)
		return
	}

	const expiry = 15 * time.Minute
	url, err := h.s3Client.GetPresignedURL(r.Context(), bucket, key, expiry)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to presign structure: %v", err), http.StatusInternalServerError)
		return
	}

	if strings.EqualFold(r.URL.Query().Get("redirect"), "true") {
		http.Redirect(w, r, url, http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GenomeStructureResponse{
		ResolutionID:    resolutionID,
		Bucket:          bucket,
		Key:             key,
		StructureSource: structureSource,
		URL:             url,
		ExpiresInSecs:   int(expiry.Seconds()),
	})
}

// --- Helpers ---

// groupNameFromPath extracts {group_name} from /api/v1/genome/jobs/{group_name}[suffix].
func groupNameFromPath(urlPath, suffix string) string {
	p := strings.TrimPrefix(urlPath, "/api/v1/genome/jobs/")
	p = strings.TrimRight(p, "/")
	if suffix != "" {
		p = strings.TrimSuffix(p, suffix)
	}
	return strings.TrimRight(p, "/")
}

// paginationParams parses page/per_page query params with defaults and a cap,
// mirroring DockingV2Results.
func paginationParams(r *http.Request, defaultPerPage, maxPerPage int) (page, perPage int) {
	page = 1
	perPage = defaultPerPage
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= maxPerPage {
			perPage = n
		}
	}
	return page, perPage
}

// nullableString returns a driver-friendly NULL for an empty string.
func nullableString(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// nullableUser returns the token label or NULL when unauthenticated (internal).
func nullableUser(u string) interface{} {
	return nullableString(u)
}
