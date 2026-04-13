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
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	// Build WHERE clauses
	var conditions []string
	var args []interface{}

	// Required: must have SMILES
	conditions = append(conditions, "cs.canonical_smiles IS NOT NULL")

	// Text search
	if search := q.Get("q"); search != "" {
		conditions = append(conditions, "(md.chembl_id LIKE ? OR md.pref_name LIKE ?)")
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern)
	}

	// MW range
	if v := q.Get("mw_min"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			conditions = append(conditions, "cp.mw_freebase >= ?")
			args = append(args, f)
		}
	}
	if v := q.Get("mw_max"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			conditions = append(conditions, "cp.mw_freebase <= ?")
			args = append(args, f)
		}
	}

	// LogP range
	if v := q.Get("logp_min"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			conditions = append(conditions, "cp.alogp >= ?")
			args = append(args, f)
		}
	}
	if v := q.Get("logp_max"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			conditions = append(conditions, "cp.alogp <= ?")
			args = append(args, f)
		}
	}

	// HBA/HBD max
	if v := q.Get("hba_max"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			conditions = append(conditions, "cp.hba <= ?")
			args = append(args, n)
		}
	}
	if v := q.Get("hbd_max"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			conditions = append(conditions, "cp.hbd <= ?")
			args = append(args, n)
		}
	}

	// Max phase
	if v := q.Get("max_phase"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			conditions = append(conditions, "md.max_phase = ?")
			args = append(args, f)
		}
	}

	// Ro5 compliant
	if q.Get("ro5") == "true" {
		conditions = append(conditions, "cp.num_ro5_violations = 0")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	baseQuery := fmt.Sprintf(`
		FROM molecule_dictionary md
		JOIN compound_structures cs ON md.molregno = cs.molregno
		LEFT JOIN compound_properties cp ON md.molregno = cp.molregno
		%s`, whereClause)

	// Count query
	var total int
	countSQL := "SELECT COUNT(*) " + baseQuery
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
