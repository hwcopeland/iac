// Package main provides HTTP handlers for basis set storage and retrieval.
// Basis sets are shared across all plugins and stored in the first available
// plugin database. The BSE (Basis Set Exchange) REST API is supported for
// importing community basis sets.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// elementToAtomicNumber maps element symbols to their atomic numbers.
// Used to convert user-provided element symbols to the numeric format
// required by the BSE REST API.
var elementToAtomicNumber = map[string]int{
	"H": 1, "He": 2, "Li": 3, "Be": 4, "B": 5, "C": 6, "N": 7, "O": 8,
	"F": 9, "Ne": 10, "Na": 11, "Mg": 12, "Al": 13, "Si": 14, "P": 15,
	"S": 16, "Cl": 17, "Ar": 18, "K": 19, "Ca": 20, "Sc": 21, "Ti": 22,
	"V": 23, "Cr": 24, "Mn": 25, "Fe": 26, "Co": 27, "Ni": 28, "Cu": 29,
	"Zn": 30, "Ga": 31, "Ge": 32, "As": 33, "Se": 34, "Br": 35, "Kr": 36,
	"Rb": 37, "Sr": 38, "Y": 39, "Zr": 40, "Nb": 41, "Mo": 42, "Tc": 43,
	"Ru": 44, "Rh": 45, "Pd": 46, "Ag": 47, "Cd": 48, "In": 49, "Sn": 50,
	"Sb": 51, "Te": 52, "I": 53, "Xe": 54, "Cs": 55, "Ba": 56, "La": 57,
	"Ce": 58, "Pr": 59, "Nd": 60, "Pm": 61, "Sm": 62, "Eu": 63, "Gd": 64,
	"Tb": 65, "Dy": 66, "Ho": 67, "Er": 68, "Tm": 69, "Yb": 70, "Lu": 71,
	"Hf": 72, "Ta": 73, "W": 74, "Re": 75, "Os": 76, "Ir": 77, "Pt": 78,
	"Au": 79, "Hg": 80, "Tl": 81, "Pb": 82, "Bi": 83, "Po": 84, "At": 85,
	"Rn": 86, "Fr": 87, "Ra": 88, "Ac": 89, "Th": 90, "Pa": 91, "U": 92,
	"Np": 93, "Pu": 94, "Am": 95, "Cm": 96, "Bk": 97, "Cf": 98, "Es": 99,
	"Fm": 100, "Md": 101, "No": 102, "Lr": 103, "Rf": 104, "Db": 105,
	"Sg": 106, "Bh": 107, "Hs": 108, "Mt": 109, "Ds": 110, "Rg": 111,
	"Cn": 112, "Nh": 113, "Fl": 114, "Mc": 115, "Lv": 116, "Ts": 117,
	"Og": 118,
}

// BasisSetSummary is the list-level view of a basis set (omits content for performance).
type BasisSetSummary struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Elements  string    `json:"elements"`
	Format    string    `json:"format"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

// BasisSetDetail is the full view of a basis set, including content.
type BasisSetDetail struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Elements    string    `json:"elements"`
	Format      string    `json:"format"`
	Source      string    `json:"source"`
	Description *string   `json:"description,omitempty"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
}

// BasisSetUploadRequest represents a user-uploaded basis set.
type BasisSetUploadRequest struct {
	Name        string `json:"name"`
	Elements    string `json:"elements"`
	Format      string `json:"format"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content"`
}

// BasisSetImportRequest represents a request to import from the BSE API.
type BasisSetImportRequest struct {
	Name     string   `json:"name"`
	Elements []string `json:"elements"`
	Format   string   `json:"format"`
}

// EnsureBasisSetSchema creates the basis_sets table if it does not exist.
func EnsureBasisSetSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS basis_sets (
		id          INT AUTO_INCREMENT PRIMARY KEY,
		name        VARCHAR(255) NOT NULL,
		elements    VARCHAR(512) NOT NULL,
		format      VARCHAR(64) NOT NULL,
		source      VARCHAR(64) NOT NULL,
		description TEXT,
		content     LONGTEXT NOT NULL,
		created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE KEY uq_basis (name, elements(255), format)
	)`)
	return err
}

// ListBasisSets handles GET /api/v1/basis-sets.
// Returns all stored basis sets in summary form (no content field).
func (h *APIHandler) ListBasisSets(w http.ResponseWriter, r *http.Request) {
	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	rows, err := db.QueryContext(r.Context(),
		`SELECT id, name, elements, format, source, created_at
		 FROM basis_sets ORDER BY name, format`)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to list basis sets: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var sets []BasisSetSummary
	for rows.Next() {
		var s BasisSetSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Elements, &s.Format, &s.Source, &s.CreatedAt); err != nil {
			writeError(w, fmt.Sprintf("failed to scan basis set: %v", err), http.StatusInternalServerError)
			return
		}
		sets = append(sets, s)
	}
	if err := rows.Err(); err != nil {
		writeError(w, fmt.Sprintf("failed to iterate basis sets: %v", err), http.StatusInternalServerError)
		return
	}

	if sets == nil {
		sets = []BasisSetSummary{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"basis_sets": sets,
		"count":      len(sets),
	})
}

// GetBasisSet handles GET /api/v1/basis-sets/{id}.
// Returns a single basis set with full content.
func (h *APIHandler) GetBasisSet(w http.ResponseWriter, r *http.Request) {
	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/basis-sets/")
	if idStr == "" {
		writeError(w, "basis set id required", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, "invalid basis set id", http.StatusBadRequest)
		return
	}

	var s BasisSetDetail
	var description sql.NullString
	err = db.QueryRowContext(r.Context(),
		`SELECT id, name, elements, format, source, description, content, created_at
		 FROM basis_sets WHERE id = ?`, id).Scan(
		&s.ID, &s.Name, &s.Elements, &s.Format, &s.Source,
		&description, &s.Content, &s.CreatedAt)
	if err == sql.ErrNoRows {
		writeError(w, "basis set not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to get basis set: %v", err), http.StatusInternalServerError)
		return
	}

	if description.Valid {
		s.Description = &description.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

// SearchBasisSets handles GET /api/v1/basis-sets/search?name=...&element=...&format=...
// All query parameters are optional; results are ANDed.
func (h *APIHandler) SearchBasisSets(w http.ResponseWriter, r *http.Request) {
	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	query := `SELECT id, name, elements, format, source, created_at FROM basis_sets WHERE 1=1`
	var args []interface{}

	if name := r.URL.Query().Get("name"); name != "" {
		query += ` AND name LIKE ?`
		args = append(args, "%"+name+"%")
	}
	if element := r.URL.Query().Get("element"); element != "" {
		// Match element in comma-separated list or wildcard "*".
		query += ` AND (elements = '*' OR FIND_IN_SET(?, elements) > 0)`
		args = append(args, element)
	}
	if format := r.URL.Query().Get("format"); format != "" {
		query += ` AND format = ?`
		args = append(args, format)
	}

	query += ` ORDER BY name, format`

	rows, err := db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to search basis sets: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var sets []BasisSetSummary
	for rows.Next() {
		var s BasisSetSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Elements, &s.Format, &s.Source, &s.CreatedAt); err != nil {
			writeError(w, fmt.Sprintf("failed to scan basis set: %v", err), http.StatusInternalServerError)
			return
		}
		sets = append(sets, s)
	}
	if err := rows.Err(); err != nil {
		writeError(w, fmt.Sprintf("failed to iterate basis sets: %v", err), http.StatusInternalServerError)
		return
	}

	if sets == nil {
		sets = []BasisSetSummary{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"basis_sets": sets,
		"count":      len(sets),
	})
}

// UploadBasisSet handles POST /api/v1/basis-sets.
// Stores a user-provided basis set in the database.
func (h *APIHandler) UploadBasisSet(w http.ResponseWriter, r *http.Request) {
	var req BasisSetUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Elements == "" || req.Format == "" || req.Content == "" {
		writeError(w, "name, elements, format, and content are required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var description *string
	if req.Description != "" {
		description = &req.Description
	}

	result, err := db.ExecContext(r.Context(),
		`INSERT INTO basis_sets (name, elements, format, source, description, content)
		 VALUES (?, ?, ?, 'user', ?, ?)`,
		req.Name, req.Elements, req.Format, description, req.Content)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			writeError(w, fmt.Sprintf("basis set %q with elements %q in format %q already exists",
				req.Name, req.Elements, req.Format), http.StatusConflict)
			return
		}
		writeError(w, fmt.Sprintf("failed to store basis set: %v", err), http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       id,
		"name":     req.Name,
		"elements": req.Elements,
		"format":   req.Format,
		"source":   "user",
	})
}

// fetchBSE fetches content from a BSE API URL. Returns the content bytes on
// success, or an error if the request fails or returns a non-200 status.
// Returns nil content (no error) if the response body is empty, allowing the
// caller to try a fallback URL.
func fetchBSE(client *http.Client, bseURL string) ([]byte, error) {
	resp, err := client.Get(bseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to contact BSE API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("BSE API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read BSE response: %w", err)
	}

	if len(content) == 0 {
		// Empty body — signal the caller to try fallback.
		return nil, nil
	}

	return content, nil
}

// ImportBasisSet handles POST /api/v1/basis-sets/import.
// Fetches a basis set from the BSE REST API and stores it in the database.
func (h *APIHandler) ImportBasisSet(w http.ResponseWriter, r *http.Request) {
	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var req BasisSetImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.Name == "" || len(req.Elements) == 0 || req.Format == "" {
		writeError(w, "name, elements, and format are required", http.StatusBadRequest)
		return
	}

	// Convert element symbols to atomic numbers for the BSE API.
	atomicNumbers := make([]string, 0, len(req.Elements))
	for _, el := range req.Elements {
		el = strings.TrimSpace(el)
		num, ok := elementToAtomicNumber[el]
		if !ok {
			writeError(w, fmt.Sprintf("unknown element symbol: %q", el), http.StatusBadRequest)
			return
		}
		atomicNumbers = append(atomicNumbers, strconv.Itoa(num))
	}

	// Build the BSE API URL.
	// BSE expects lowercase basis name with URL encoding for special chars.
	bseName := url.PathEscape(strings.ToLower(req.Name))
	elementsParam := strings.Join(atomicNumbers, ",")

	// Use HTTPS first; the BSE site may redirect HTTP->HTTPS and lose the body.
	bseURL := fmt.Sprintf("https://www.basissetexchange.org/api/basis/%s/format/%s/?elements=%s",
		bseName, url.PathEscape(req.Format), elementsParam)

	log.Printf("[basis-import] Fetching from BSE: %s", bseURL)

	// Configure client to follow all redirects (including HTTPS->HTTP downgrades).
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil // follow all redirects
		},
	}

	content, err := fetchBSE(client, bseURL)
	if err != nil {
		// Fallback: try HTTP if HTTPS failed or returned empty content.
		httpURL := fmt.Sprintf("http://www.basissetexchange.org/api/basis/%s/format/%s/?elements=%s",
			bseName, url.PathEscape(req.Format), elementsParam)
		log.Printf("[basis-import] HTTPS attempt failed or empty, trying HTTP: %s", httpURL)
		content, err = fetchBSE(client, httpURL)
		if err != nil {
			writeError(w, fmt.Sprintf("failed to fetch from BSE API: %v", err), http.StatusBadGateway)
			return
		}
	}

	if len(content) == 0 {
		writeError(w, "BSE API returned empty content for the requested basis set", http.StatusBadGateway)
		return
	}

	// Store in the database.
	elementsCSV := strings.Join(req.Elements, ",")
	description := fmt.Sprintf("Imported from BSE (%s)", req.Name)

	result, insertErr := db.ExecContext(r.Context(),
		`INSERT INTO basis_sets (name, elements, format, source, description, content)
		 VALUES (?, ?, ?, 'bse', ?, ?)
		 ON DUPLICATE KEY UPDATE content = VALUES(content), description = VALUES(description), source = 'bse'`,
		req.Name, elementsCSV, req.Format, description, string(content))
	if insertErr != nil {
		writeError(w, fmt.Sprintf("failed to store imported basis set: %v", insertErr), http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	// If ON DUPLICATE KEY UPDATE fired, LastInsertId may be 0. Look it up.
	if id == 0 {
		_ = db.QueryRowContext(r.Context(),
			`SELECT id FROM basis_sets WHERE name = ? AND elements = ? AND format = ?`,
			req.Name, elementsCSV, req.Format).Scan(&id)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       id,
		"name":     req.Name,
		"elements": elementsCSV,
		"format":   req.Format,
		"source":   "bse",
		"size":     len(content),
	})
}

// DeleteBasisSet handles DELETE /api/v1/basis-sets/{id}.
func (h *APIHandler) DeleteBasisSet(w http.ResponseWriter, r *http.Request) {
	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/v1/basis-sets/")
	if idStr == "" {
		writeError(w, "basis set id required", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, "invalid basis set id", http.StatusBadRequest)
		return
	}

	result, err := db.ExecContext(r.Context(), `DELETE FROM basis_sets WHERE id = ?`, id)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to delete basis set: %v", err), http.StatusInternalServerError)
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, "basis set not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
