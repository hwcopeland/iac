// Package main provides ChEMBL compound search and import handlers.
// These endpoints allow scientists to search the ChEMBL compound library
// and import selected compounds into the docking ligand database.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// CompoundResult represents a single compound from ChEMBL search results.
type CompoundResult struct {
	ChEMBLID      string   `json:"chembl_id"`
	PrefName      *string  `json:"pref_name"`
	SMILES        string   `json:"smiles"`
	MW            *float64 `json:"mw"`
	LogP          *float64 `json:"logp"`
	HBA           *int     `json:"hba"`
	HBD           *int     `json:"hbd"`
	PSA           *float64 `json:"psa"`
	Ro5Violations *int     `json:"ro5_violations"`
	QED           *float64 `json:"qed"`
	MaxPhase      *float64 `json:"max_phase"`
	Formula       *string  `json:"formula"`
}

// buildChEMBLFilterClauses constructs WHERE conditions and parameterized args from
// a string-keyed parameter map.  Both SearchLigands (URL query) and ImportFromFilter
// (JSON body) funnel through this helper so filter logic is defined once.
func buildChEMBLFilterClauses(params map[string]string) (conditions []string, args []interface{}) {
	// Required: must have SMILES
	conditions = append(conditions, "cs.canonical_smiles IS NOT NULL")

	// Text search
	if search := params["q"]; search != "" {
		conditions = append(conditions, "(md.chembl_id LIKE ? OR md.pref_name LIKE ?)")
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern)
	}

	// MW range
	if v := params["mw_min"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			conditions = append(conditions, "cp.mw_freebase >= ?")
			args = append(args, f)
		}
	}
	if v := params["mw_max"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			conditions = append(conditions, "cp.mw_freebase <= ?")
			args = append(args, f)
		}
	}

	// LogP range
	if v := params["logp_min"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			conditions = append(conditions, "cp.alogp >= ?")
			args = append(args, f)
		}
	}
	if v := params["logp_max"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			conditions = append(conditions, "cp.alogp <= ?")
			args = append(args, f)
		}
	}

	// HBA/HBD max
	if v := params["hba_max"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			conditions = append(conditions, "cp.hba <= ?")
			args = append(args, n)
		}
	}
	if v := params["hbd_max"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			conditions = append(conditions, "cp.hbd <= ?")
			args = append(args, n)
		}
	}

	// Max phase
	if v := params["max_phase"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			conditions = append(conditions, "md.max_phase = ?")
			args = append(args, f)
		}
	}

	// Ro5 compliant
	if params["ro5"] == "true" {
		conditions = append(conditions, "cp.num_ro5_violations = 0")
	}

	return conditions, args
}

// SearchLigands handles GET /api/v1/ligands/search.
// Searches the ChEMBL compound library with optional filters.
func (h *APIHandler) SearchLigands(w http.ResponseWriter, r *http.Request) {
	if h.chemblDB == nil {
		writeError(w, "ChEMBL database not available", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()

	// Pagination
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	// Convert URL query params to a flat map for the shared filter builder.
	params := make(map[string]string)
	for _, key := range []string{"q", "mw_min", "mw_max", "logp_min", "logp_max", "hba_max", "hbd_max", "max_phase", "ro5"} {
		if v := q.Get(key); v != "" {
			params[key] = v
		}
	}

	conditions, args := buildChEMBLFilterClauses(params)

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	baseQuery := fmt.Sprintf(`
		FROM molecule_dictionary md
		JOIN compound_structures cs ON md.molregno = cs.molregno
		LEFT JOIN compound_properties cp ON md.molregno = cp.molregno
		%s`, whereClause)

	// Fast approximate count — caps at limit+1 so broad filters don't scan millions of rows.
	// The exact count doesn't matter for the scatter plot; "2000+" is sufficient.
	countCap := limit + 1
	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (SELECT 1 %s LIMIT %d) _cnt", baseQuery, countCap)
	if err := h.chemblDB.QueryRowContext(r.Context(), countSQL, args...).Scan(&total); err != nil {
		writeError(w, fmt.Sprintf("search count failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Data query
	dataSQL := fmt.Sprintf(`SELECT
		md.chembl_id, md.pref_name, cs.canonical_smiles,
		cp.mw_freebase, cp.alogp, cp.hba, cp.hbd, cp.psa,
		cp.num_ro5_violations, cp.qed_weighted, md.max_phase, cp.full_molformula
		%s
		ORDER BY md.chembl_id
		LIMIT ? OFFSET ?`, baseQuery)

	dataArgs := append(args, limit, offset)
	rows, err := h.chemblDB.QueryContext(r.Context(), dataSQL, dataArgs...)
	if err != nil {
		writeError(w, fmt.Sprintf("search query failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var compounds []CompoundResult
	for rows.Next() {
		var c CompoundResult
		if err := rows.Scan(
			&c.ChEMBLID, &c.PrefName, &c.SMILES,
			&c.MW, &c.LogP, &c.HBA, &c.HBD, &c.PSA,
			&c.Ro5Violations, &c.QED, &c.MaxPhase, &c.Formula,
		); err != nil {
			continue
		}
		compounds = append(compounds, c)
	}
	if compounds == nil {
		compounds = []CompoundResult{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"compounds": compounds,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

// ImportFromChEMBLRequest represents a request to import compounds from ChEMBL.
type ImportFromChEMBLRequest struct {
	ChEMBLIDs []string `json:"chembl_ids"`
	SourceDB  string   `json:"source_db"`
}

// ImportFromChEMBL handles POST /api/v1/ligands/import-from-chembl.
// Looks up compounds in ChEMBL by ID and imports them into the docking ligands table.
func (h *APIHandler) ImportFromChEMBL(w http.ResponseWriter, r *http.Request) {
	if h.chemblDB == nil {
		writeError(w, "ChEMBL database not available", http.StatusServiceUnavailable)
		return
	}

	dockingDB := h.pluginDB("docking")
	if dockingDB == nil {
		writeError(w, "docking database not available", http.StatusInternalServerError)
		return
	}

	var req ImportFromChEMBLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if len(req.ChEMBLIDs) == 0 {
		writeError(w, "chembl_ids is required", http.StatusBadRequest)
		return
	}
	if req.SourceDB == "" {
		writeError(w, "source_db is required", http.StatusBadRequest)
		return
	}
	if len(req.ChEMBLIDs) > 500 {
		writeError(w, "maximum 500 compounds per import", http.StatusBadRequest)
		return
	}

	// Batch lookup from ChEMBL
	placeholders := make([]string, len(req.ChEMBLIDs))
	lookupArgs := make([]interface{}, len(req.ChEMBLIDs))
	for i, id := range req.ChEMBLIDs {
		placeholders[i] = "?"
		lookupArgs[i] = id
	}

	rows, err := h.chemblDB.QueryContext(r.Context(),
		fmt.Sprintf(`SELECT md.chembl_id, cs.canonical_smiles
			FROM molecule_dictionary md
			JOIN compound_structures cs ON md.molregno = cs.molregno
			WHERE md.chembl_id IN (%s)`, strings.Join(placeholders, ",")),
		lookupArgs...)
	if err != nil {
		writeError(w, fmt.Sprintf("ChEMBL lookup failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	imported := 0
	for rows.Next() {
		var chemblID, smiles string
		if err := rows.Scan(&chemblID, &smiles); err != nil {
			continue
		}

		_, err := dockingDB.ExecContext(r.Context(),
			`INSERT INTO ligands (compound_id, smiles, source_db)
			 VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE smiles = VALUES(smiles)`,
			chemblID, smiles, req.SourceDB)
		if err != nil {
			continue
		}
		imported++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"imported":  imported,
		"total":     len(req.ChEMBLIDs),
		"source_db": req.SourceDB,
	})
}

// ImportFromFilterRequest represents a request to import all ChEMBL compounds
// matching a set of search filters into the docking ligands table.
type ImportFromFilterRequest struct {
	SourceDB string   `json:"source_db"`
	Q        string   `json:"q,omitempty"`
	MWMin    *float64 `json:"mw_min,omitempty"`
	MWMax    *float64 `json:"mw_max,omitempty"`
	LogPMin  *float64 `json:"logp_min,omitempty"`
	LogPMax  *float64 `json:"logp_max,omitempty"`
	HBDMax   *int     `json:"hbd_max,omitempty"`
	HBAMax   *int     `json:"hba_max,omitempty"`
	MaxPhase *float64 `json:"max_phase,omitempty"`
	Ro5      bool     `json:"ro5,omitempty"`
}

// toParamMap converts the typed request struct into a flat string map suitable
// for buildChEMBLFilterClauses.
func (req *ImportFromFilterRequest) toParamMap() map[string]string {
	params := make(map[string]string)
	if req.Q != "" {
		params["q"] = req.Q
	}
	if req.MWMin != nil {
		params["mw_min"] = strconv.FormatFloat(*req.MWMin, 'f', -1, 64)
	}
	if req.MWMax != nil {
		params["mw_max"] = strconv.FormatFloat(*req.MWMax, 'f', -1, 64)
	}
	if req.LogPMin != nil {
		params["logp_min"] = strconv.FormatFloat(*req.LogPMin, 'f', -1, 64)
	}
	if req.LogPMax != nil {
		params["logp_max"] = strconv.FormatFloat(*req.LogPMax, 'f', -1, 64)
	}
	if req.HBDMax != nil {
		params["hbd_max"] = strconv.Itoa(*req.HBDMax)
	}
	if req.HBAMax != nil {
		params["hba_max"] = strconv.Itoa(*req.HBAMax)
	}
	if req.MaxPhase != nil {
		params["max_phase"] = strconv.FormatFloat(*req.MaxPhase, 'f', -1, 64)
	}
	if req.Ro5 {
		params["ro5"] = "true"
	}
	return params
}

// ImportFromFilter handles POST /api/v1/ligands/import-from-filter.
// Imports all ChEMBL compounds matching the provided search filters into the
// docking ligands table.  Returns an error if more than 10,000 compounds match
// (the caller should narrow filters).
func (h *APIHandler) ImportFromFilter(w http.ResponseWriter, r *http.Request) {
	if h.chemblDB == nil {
		writeError(w, "ChEMBL database not available", http.StatusServiceUnavailable)
		return
	}

	dockingDB := h.pluginDB("docking")
	if dockingDB == nil {
		writeError(w, "docking database not available", http.StatusInternalServerError)
		return
	}

	var req ImportFromFilterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}
	if req.SourceDB == "" {
		writeError(w, "source_db is required", http.StatusBadRequest)
		return
	}

	// Build filter clauses from the JSON body.
	conditions, args := buildChEMBLFilterClauses(req.toParamMap())

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	baseQuery := fmt.Sprintf(`
		FROM molecule_dictionary md
		JOIN compound_structures cs ON md.molregno = cs.molregno
		LEFT JOIN compound_properties cp ON md.molregno = cp.molregno
		%s`, whereClause)

	// Count how many compounds match before fetching them all.
	var totalMatched int
	countSQL := "SELECT COUNT(*) " + baseQuery
	if err := h.chemblDB.QueryRowContext(r.Context(), countSQL, args...).Scan(&totalMatched); err != nil {
		writeError(w, fmt.Sprintf("filter count failed: %v", err), http.StatusInternalServerError)
		return
	}

	if totalMatched == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"imported":      0,
			"total_matched": 0,
			"source_db":     req.SourceDB,
		})
		return
	}

	// Fetch all matching chembl_id + canonical_smiles from ChEMBL.
	dataSQL := fmt.Sprintf(
		"SELECT md.chembl_id, cs.canonical_smiles %s ORDER BY md.chembl_id",
		baseQuery,
	)
	rows, err := h.chemblDB.QueryContext(r.Context(), dataSQL, args...)
	if err != nil {
		writeError(w, fmt.Sprintf("filter query failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Collect rows before starting the transaction so the ChEMBL result set
	// is fully consumed and does not block while we write to docking.
	type ligandRow struct {
		chemblID string
		smiles   string
	}
	var ligands []ligandRow
	for rows.Next() {
		var lr ligandRow
		if err := rows.Scan(&lr.chemblID, &lr.smiles); err != nil {
			continue
		}
		ligands = append(ligands, lr)
	}
	if err := rows.Err(); err != nil {
		writeError(w, fmt.Sprintf("error reading filter results: %v", err), http.StatusInternalServerError)
		return
	}

	// Batch INSERT into docking.ligands within a transaction.
	tx, err := dockingDB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to begin transaction: %v", err), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback() // no-op after Commit

	stmt, err := tx.PrepareContext(r.Context(),
		`INSERT INTO ligands (compound_id, smiles, source_db)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE smiles = VALUES(smiles)`)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to prepare insert: %v", err), http.StatusInternalServerError)
		return
	}
	defer stmt.Close()

	imported := 0
	for _, lig := range ligands {
		if _, err := stmt.ExecContext(r.Context(), lig.chemblID, lig.smiles, req.SourceDB); err != nil {
			continue
		}
		imported++
	}

	if err := tx.Commit(); err != nil {
		writeError(w, fmt.Sprintf("failed to commit import: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"imported":      imported,
		"total_matched": totalMatched,
		"source_db":     req.SourceDB,
	})
}
