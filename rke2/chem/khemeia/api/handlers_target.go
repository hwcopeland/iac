// Package main provides HTTP handlers for WP-1 target preparation endpoints.
// These handlers manage the lifecycle of receptor preparation and binding-site
// definition, including native-ligand extraction, custom box specification,
// and consensus pocket detection via fpocket + P2Rank sidecar services.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// --- Service URLs for target-prep sidecar services ---

// targetPrepServiceURL is the base URL of the target-prep sidecar.
var targetPrepServiceURL = getTargetPrepURL()

// fpocketServiceURL is the base URL of the fpocket sidecar.
var fpocketServiceURL = getFpocketURL()

// p2rankServiceURL is the base URL of the P2Rank sidecar.
var p2rankServiceURL = getP2RankURL()

func getTargetPrepURL() string {
	if url := os.Getenv("TARGET_PREP_SERVICE_URL"); url != "" {
		return url
	}
	return "http://target-prep.chem.svc.cluster.local"
}

func getFpocketURL() string {
	if url := os.Getenv("FPOCKET_SERVICE_URL"); url != "" {
		return url
	}
	return "http://fpocket.chem.svc.cluster.local"
}

func getP2RankURL() string {
	if url := os.Getenv("P2RANK_SERVICE_URL"); url != "" {
		return url
	}
	return "http://p2rank.chem.svc.cluster.local"
}

// --- Request/response types ---

// TargetPrepRequest is the JSON body for POST /api/v1/targets/prepare.
type TargetPrepRequest struct {
	PDBID           string    `json:"pdb_id"`
	BindingSiteMode string    `json:"binding_site_mode"` // native-ligand, custom-box, pocket-detection
	NativeLigandID  string    `json:"native_ligand_id,omitempty"`
	Padding         float64   `json:"padding,omitempty"`
	PH              float64   `json:"pH,omitempty"`
	KeepCofactors   []string  `json:"keep_cofactors,omitempty"`
	CustomBox       *BoxSpec  `json:"custom_box,omitempty"`
}

// BoxSpec defines a 3D bounding box with center and size.
type BoxSpec struct {
	Center [3]float64 `json:"center"`
	Size   [3]float64 `json:"size"`
}

// TargetPrepResponse is the 202 Accepted response for a new target prep job.
type TargetPrepResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// TargetPrepStatus is the response for GET /api/v1/targets/{name}.
type TargetPrepStatus struct {
	Name            string            `json:"name"`
	PDBID           string            `json:"pdb_id"`
	BindingSiteMode string            `json:"binding_site_mode"`
	Phase           string            `json:"phase"`
	ReceptorS3Key   *string           `json:"receptor_s3_key,omitempty"`
	BindingSite     *BoxSpec          `json:"binding_site,omitempty"`
	Pockets         []DetectedPocket  `json:"pockets,omitempty"`
	SelectedPocket  *int              `json:"selected_pocket,omitempty"`
	StartTime       *string           `json:"start_time,omitempty"`
	CompletionTime  *string           `json:"completion_time,omitempty"`
	Error           *string           `json:"error,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

// PocketSelectRequest is the JSON body for POST /api/v1/targets/{name}/pockets/{index}/select.
type PocketSelectRequest struct {
	// Empty body — the pocket index is in the URL path.
}

// validBindingSiteModes enumerates the allowed binding_site_mode values.
var validBindingSiteModes = map[string]bool{
	"native-ligand":    true,
	"custom-box":       true,
	"pocket-detection": true,
}

// --- Target prep MySQL table ---

// EnsureTargetPrepSchema creates the target_prep_results table if it doesn't exist.
// Called during startup on the shared database following the same pattern as
// EnsureProvenanceSchema and EnsureAPITokenSchema.
func EnsureTargetPrepSchema(db *sql.DB) error {
	ddl := `CREATE TABLE IF NOT EXISTS target_prep_results (
		id                INT AUTO_INCREMENT PRIMARY KEY,
		name              VARCHAR(255) NOT NULL UNIQUE,
		pdb_id            VARCHAR(10)  NOT NULL,
		binding_site_mode ENUM('native-ligand', 'custom-box', 'pocket-detection') NOT NULL,
		phase             ENUM('Pending', 'Running', 'Succeeded', 'Failed') NOT NULL DEFAULT 'Pending',
		native_ligand_id  VARCHAR(64)  NULL,
		padding           FLOAT        NOT NULL DEFAULT 10.0,
		ph                FLOAT        NOT NULL DEFAULT 7.4,
		keep_cofactors    JSON         NULL,
		custom_box        JSON         NULL,
		receptor_s3_key   VARCHAR(512) NULL,
		binding_site      JSON         NULL,
		pockets           JSON         NULL,
		selected_pocket   INT          NULL,
		error_message     TEXT         NULL,
		start_time        TIMESTAMP    NULL,
		completion_time   TIMESTAMP    NULL,
		created_at        TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_phase (phase),
		INDEX idx_pdb_id (pdb_id),
		INDEX idx_created_at (created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("creating target_prep_results table: %w", err)
	}
	return nil
}

// --- Sidecar response types ---

// targetPrepSidecarResponse is the response from the target-prep sidecar service.
type targetPrepSidecarResponse struct {
	ReceptorPDB string     `json:"receptor_pdb"`
	BindingSite *BoxSpec   `json:"binding_site,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// fpocketSidecarResponse is the response from the fpocket sidecar service.
type fpocketSidecarResponse struct {
	Pockets []DetectedPocket `json:"pockets"`
	Error   string           `json:"error,omitempty"`
}

// p2rankSidecarResponse is the response from the P2Rank sidecar service.
type p2rankSidecarResponse struct {
	Pockets []DetectedPocket `json:"pockets"`
	Error   string           `json:"error,omitempty"`
}

// --- HTTP handlers ---

// TargetPrepareHandler handles POST /api/v1/targets/prepare.
// Validates input, creates a target prep record in MySQL, and starts an async
// goroutine to orchestrate the preparation pipeline.
func (h *APIHandler) TargetPrepareHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TargetPrepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields.
	if req.PDBID == "" {
		writeError(w, "pdb_id is required", http.StatusBadRequest)
		return
	}

	if !validBindingSiteModes[req.BindingSiteMode] {
		writeError(w, fmt.Sprintf("invalid binding_site_mode: %q (must be one of: native-ligand, custom-box, pocket-detection)", req.BindingSiteMode), http.StatusBadRequest)
		return
	}

	// Mode-specific validation.
	if req.BindingSiteMode == "native-ligand" && req.NativeLigandID == "" {
		writeError(w, "native_ligand_id is required for native-ligand mode", http.StatusBadRequest)
		return
	}

	if req.BindingSiteMode == "custom-box" {
		if req.CustomBox == nil {
			writeError(w, "custom_box is required for custom-box mode", http.StatusBadRequest)
			return
		}
		// Validate box dimensions are positive.
		for i, s := range req.CustomBox.Size {
			if s <= 0 {
				writeError(w, fmt.Sprintf("custom_box.size[%d] must be positive", i), http.StatusBadRequest)
				return
			}
		}
	}

	// Apply defaults.
	if req.Padding == 0 {
		req.Padding = 10.0
	}
	if req.PH == 0 {
		req.PH = 7.4
	}
	if len(req.KeepCofactors) == 0 {
		req.KeepCofactors = []string{"ZN", "MG", "CA", "FE"}
	}

	// Generate job name.
	jobName := fmt.Sprintf("target-prep-%s-%d", strings.ToLower(req.PDBID), time.Now().UnixNano())

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	// Serialize JSON fields.
	cofactorsJSON, _ := json.Marshal(req.KeepCofactors)
	var customBoxJSON []byte
	if req.CustomBox != nil {
		customBoxJSON, _ = json.Marshal(req.CustomBox)
	}

	// Insert the target prep record.
	_, err := db.ExecContext(r.Context(),
		`INSERT INTO target_prep_results
			(name, pdb_id, binding_site_mode, native_ligand_id, padding, ph, keep_cofactors, custom_box)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		jobName, req.PDBID, req.BindingSiteMode, nullString(req.NativeLigandID),
		req.Padding, req.PH, string(cofactorsJSON), nullBytes(customBoxJSON))
	if err != nil {
		writeError(w, fmt.Sprintf("failed to create target prep job: %v", err), http.StatusInternalServerError)
		return
	}

	// Return 202 Accepted.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(TargetPrepResponse{
		Name:   jobName,
		Status: "Pending",
	})

	// Start async pipeline.
	go h.runTargetPrepPipeline(jobName, req)
}

// TargetGetHandler handles GET /api/v1/targets/{name}.
// Returns the target prep status and results.
func (h *APIHandler) TargetGetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/v1/targets/")
	name = strings.TrimRight(name, "/")

	// Strip sub-paths (pockets/select would be handled by other handlers).
	if strings.Contains(name, "/") {
		writeError(w, "not found", http.StatusNotFound)
		return
	}

	if name == "" || name == "prepare" {
		writeError(w, "target name required", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var status TargetPrepStatus
	var nativeLigandID, receptorS3Key, errorMsg sql.NullString
	var bindingSiteJSON, pocketsJSON sql.NullString
	var selectedPocket sql.NullInt32
	var startTime, completionTime sql.NullTime

	err := db.QueryRowContext(r.Context(),
		`SELECT name, pdb_id, binding_site_mode, phase, native_ligand_id,
			receptor_s3_key, binding_site, pockets, selected_pocket,
			error_message, start_time, completion_time, created_at
		 FROM target_prep_results WHERE name = ?`, name).Scan(
		&status.Name, &status.PDBID, &status.BindingSiteMode, &status.Phase,
		&nativeLigandID, &receptorS3Key, &bindingSiteJSON, &pocketsJSON,
		&selectedPocket, &errorMsg, &startTime, &completionTime, &status.CreatedAt)

	if err == sql.ErrNoRows {
		writeError(w, "target not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to get target: %v", err), http.StatusInternalServerError)
		return
	}

	if receptorS3Key.Valid {
		status.ReceptorS3Key = &receptorS3Key.String
	}
	if errorMsg.Valid {
		status.Error = &errorMsg.String
	}
	if startTime.Valid {
		ts := startTime.Time.Format(time.RFC3339)
		status.StartTime = &ts
	}
	if completionTime.Valid {
		ts := completionTime.Time.Format(time.RFC3339)
		status.CompletionTime = &ts
	}
	if selectedPocket.Valid {
		sp := int(selectedPocket.Int32)
		status.SelectedPocket = &sp
	}

	// Unmarshal binding site.
	if bindingSiteJSON.Valid && bindingSiteJSON.String != "" {
		var bs BoxSpec
		if err := json.Unmarshal([]byte(bindingSiteJSON.String), &bs); err == nil {
			status.BindingSite = &bs
		}
	}

	// Unmarshal pockets.
	if pocketsJSON.Valid && pocketsJSON.String != "" {
		var pockets []DetectedPocket
		if err := json.Unmarshal([]byte(pocketsJSON.String), &pockets); err == nil {
			status.Pockets = pockets
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// TargetPocketsHandler handles GET /api/v1/targets/{name}/pockets.
// Returns detected pockets with consensus scores (pocket-detection mode only).
func (h *APIHandler) TargetPocketsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse: /api/v1/targets/{name}/pockets
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/targets/")
	path = strings.TrimRight(path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[0] == "" {
		writeError(w, "target name required", http.StatusBadRequest)
		return
	}
	name := parts[0]

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var mode string
	var pocketsJSON sql.NullString
	var selectedPocket sql.NullInt32

	err := db.QueryRowContext(r.Context(),
		`SELECT binding_site_mode, pockets, selected_pocket
		 FROM target_prep_results WHERE name = ?`, name).Scan(
		&mode, &pocketsJSON, &selectedPocket)

	if err == sql.ErrNoRows {
		writeError(w, "target not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to get target: %v", err), http.StatusInternalServerError)
		return
	}

	if mode != "pocket-detection" {
		writeError(w, "pockets are only available for pocket-detection mode", http.StatusBadRequest)
		return
	}

	var pockets []DetectedPocket
	if pocketsJSON.Valid && pocketsJSON.String != "" {
		if err := json.Unmarshal([]byte(pocketsJSON.String), &pockets); err != nil {
			writeError(w, "failed to parse pocket data", http.StatusInternalServerError)
			return
		}
	}
	if pockets == nil {
		pockets = []DetectedPocket{}
	}

	resp := map[string]interface{}{
		"name":    name,
		"pockets": pockets,
		"count":   len(pockets),
	}
	if selectedPocket.Valid {
		resp["selected_pocket"] = selectedPocket.Int32
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// TargetSelectPocketHandler handles POST /api/v1/targets/{name}/pockets/{index}/select.
// Selects a specific pocket for downstream use and updates the binding site.
func (h *APIHandler) TargetSelectPocketHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse: /api/v1/targets/{name}/pockets/{index}/select
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/targets/")
	path = strings.TrimRight(path, "/")
	parts := strings.Split(path, "/")

	// Expected: {name}/pockets/{index}/select -> 4 parts
	if len(parts) != 4 || parts[1] != "pockets" || parts[3] != "select" {
		writeError(w, "invalid path: expected /api/v1/targets/{name}/pockets/{index}/select", http.StatusBadRequest)
		return
	}

	name := parts[0]
	pocketIndex, err := strconv.Atoi(parts[2])
	if err != nil || pocketIndex < 0 {
		writeError(w, "pocket index must be a non-negative integer", http.StatusBadRequest)
		return
	}

	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	// Fetch the target prep record.
	var mode string
	var pocketsJSON sql.NullString

	err = db.QueryRowContext(r.Context(),
		`SELECT binding_site_mode, pockets FROM target_prep_results WHERE name = ?`, name).Scan(
		&mode, &pocketsJSON)

	if err == sql.ErrNoRows {
		writeError(w, "target not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeError(w, fmt.Sprintf("failed to get target: %v", err), http.StatusInternalServerError)
		return
	}

	if mode != "pocket-detection" {
		writeError(w, "pocket selection is only available for pocket-detection mode", http.StatusBadRequest)
		return
	}

	if !pocketsJSON.Valid || pocketsJSON.String == "" {
		writeError(w, "no pockets have been detected yet", http.StatusConflict)
		return
	}

	var pockets []DetectedPocket
	if err := json.Unmarshal([]byte(pocketsJSON.String), &pockets); err != nil {
		writeError(w, "failed to parse pocket data", http.StatusInternalServerError)
		return
	}

	if pocketIndex >= len(pockets) {
		writeError(w, fmt.Sprintf("pocket index %d out of range (0-%d)", pocketIndex, len(pockets)-1), http.StatusBadRequest)
		return
	}

	// Extract the selected pocket as the binding site.
	selectedPocket := pockets[pocketIndex]
	bindingSite := BoxSpec{
		Center: selectedPocket.Center,
		Size:   selectedPocket.Size,
	}
	bindingSiteJSON, _ := json.Marshal(bindingSite)

	// Update the record.
	_, err = db.ExecContext(r.Context(),
		`UPDATE target_prep_results
		 SET selected_pocket = ?, binding_site = ?
		 WHERE name = ?`,
		pocketIndex, string(bindingSiteJSON), name)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to update pocket selection: %v", err), http.StatusInternalServerError)
		return
	}

	// If a CRD instance exists, update its status too.
	go h.updateTargetPrepCRDBindingSite(name, bindingSite, pocketIndex)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":            name,
		"selected_pocket": pocketIndex,
		"binding_site":    bindingSite,
	})
}

// --- Target route dispatcher ---

// TargetDispatch routes /api/v1/targets/ requests based on path structure.
// It distinguishes between:
//   - POST /api/v1/targets/prepare          -> TargetPrepareHandler
//   - GET  /api/v1/targets/{name}           -> TargetGetHandler
//   - GET  /api/v1/targets/{name}/pockets   -> TargetPocketsHandler
//   - POST /api/v1/targets/{name}/pockets/{index}/select -> TargetSelectPocketHandler
func (h *APIHandler) TargetDispatch(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/targets/")
	path = strings.TrimRight(path, "/")

	// POST /api/v1/targets/prepare
	if r.Method == http.MethodPost && path == "prepare" {
		h.TargetPrepareHandler(w, r)
		return
	}

	parts := strings.Split(path, "/")

	// POST /api/v1/targets/{name}/pockets/{index}/select
	if r.Method == http.MethodPost && len(parts) == 4 && parts[1] == "pockets" && parts[3] == "select" {
		h.TargetSelectPocketHandler(w, r)
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// GET /api/v1/targets/{name}/pockets
	if len(parts) == 2 && parts[1] == "pockets" {
		h.TargetPocketsHandler(w, r)
		return
	}

	// GET /api/v1/targets/{name}
	if len(parts) == 1 && parts[0] != "" {
		h.TargetGetHandler(w, r)
		return
	}

	writeError(w, "not found", http.StatusNotFound)
}

// --- Async pipeline ---

// runTargetPrepPipeline orchestrates the full target preparation workflow:
// 1. Update status to Running
// 2. Call the target-prep sidecar to clean the receptor
// 3. Store the cleaned receptor in Garage
// 4. Based on mode, compute or detect the binding site
// 5. Record provenance
// 6. Optionally create a TargetPrep CRD instance
// 7. Update status to Succeeded or Failed
func (h *APIHandler) runTargetPrepPipeline(jobName string, req TargetPrepRequest) {
	ctx := context.Background()

	db := h.controller.firstDB()
	if db == nil {
		log.Printf("[target-prep] %s: CRITICAL: no database available", jobName)
		return
	}

	// Mark as Running.
	if _, err := db.ExecContext(ctx,
		`UPDATE target_prep_results SET phase = 'Running', start_time = NOW() WHERE name = ?`, jobName); err != nil {
		log.Printf("[target-prep] %s: failed to update status to Running: %v", jobName, err)
	}

	// Step 1: Call target-prep sidecar to clean the receptor AND compute binding site.
	// The sidecar handles everything in one call — receptor cleaning, native ligand
	// extraction, custom box validation. No need for separate binding-site call.
	sidecarResp, err := h.callTargetPrepSidecar(ctx, req)
	if err != nil {
		h.failTargetPrep(ctx, db, jobName, fmt.Sprintf("receptor preparation failed: %v", err))
		return
	}
	cleanedPDB := sidecarResp.ReceptorPDB

	// Step 2: Store cleaned receptor in Garage.
	s3Key := ArtifactKey("TargetPrep", jobName, "receptor", "pdb")
	if err := h.s3Client.PutArtifact(ctx, BucketReceptors, s3Key,
		strings.NewReader(cleanedPDB), "chemical/x-pdb"); err != nil {
		log.Printf("[target-prep] %s: warning: failed to store receptor in S3: %v", jobName, err)
	}

	// Update receptor S3 key.
	if _, err := db.ExecContext(ctx,
		`UPDATE target_prep_results SET receptor_s3_key = ? WHERE name = ?`, s3Key, jobName); err != nil {
		log.Printf("[target-prep] %s: warning: failed to update receptor_s3_key: %v", jobName, err)
	}

	// Step 3: Handle binding site from sidecar response or run pocket detection.
	switch req.BindingSiteMode {
	case "native-ligand", "custom-box":
		// Sidecar already computed the binding site — store it.
		if sidecarResp.BindingSite != nil {
			bindingSiteJSON, _ := json.Marshal(sidecarResp.BindingSite)
			if _, err := db.ExecContext(ctx,
				`UPDATE target_prep_results SET binding_site = ?, phase = 'Succeeded', completion_time = NOW() WHERE name = ?`,
				string(bindingSiteJSON), jobName); err != nil {
				h.failTargetPrep(ctx, db, jobName, fmt.Sprintf("failed to store binding site: %v", err))
				return
			}
			log.Printf("[target-prep] %s: binding site: center=%v size=%v",
				jobName, sidecarResp.BindingSite.Center, sidecarResp.BindingSite.Size)
		} else {
			h.failTargetPrep(ctx, db, jobName, "sidecar returned no binding site for "+req.BindingSiteMode+" mode")
			return
		}
		err = nil // Success — skip the old processNativeLigandMode/processCustomBoxMode
	case "pocket-detection":
		err = h.processPocketDetectionMode(ctx, db, jobName, cleanedPDB)
	default:
		err = fmt.Errorf("unknown binding site mode: %s", req.BindingSiteMode)
	}

	if err != nil {
		h.failTargetPrep(ctx, db, jobName, err.Error())
		return
	}

	// Step 4: Record provenance.
	bucket := BucketReceptors
	jobKind := "TargetPrep"
	params, _ := json.Marshal(map[string]interface{}{
		"pdb_id":            req.PDBID,
		"binding_site_mode": req.BindingSiteMode,
		"padding":           req.Padding,
		"pH":                req.PH,
		"keep_cofactors":    req.KeepCofactors,
	})

	provRecord := &ProvenanceRecord{
		ArtifactType: "receptor",
		S3Bucket:     &bucket,
		S3Key:        &s3Key,
		CreatedByJob: jobName,
		JobKind:      &jobKind,
		JobNamespace: "chem",
		Parameters:   params,
	}
	if err := RecordProvenance(ctx, db, provRecord, nil); err != nil {
		log.Printf("[target-prep] %s: warning: failed to record provenance: %v", jobName, err)
	}

	// Step 5: Optionally create a TargetPrep CRD instance.
	if os.Getenv("CRD_ENABLED") == "true" {
		h.createTargetPrepCRD(ctx, jobName, req, s3Key)
	}

	// Step 6: Mark as Succeeded.
	if _, err := db.ExecContext(ctx,
		`UPDATE target_prep_results SET phase = 'Succeeded', completion_time = NOW() WHERE name = ?`, jobName); err != nil {
		log.Printf("[target-prep] %s: failed to update status to Succeeded: %v", jobName, err)
	}

	log.Printf("[target-prep] %s: completed successfully", jobName)
}

// processNativeLigandMode extracts the binding site from the native ligand
// coordinates via the target-prep sidecar.
func (h *APIHandler) processNativeLigandMode(ctx context.Context, db *sql.DB, jobName string, req TargetPrepRequest, cleanedPDB string) error {
	// The target-prep sidecar returns binding site coordinates when
	// given the native ligand ID and padding.
	client := &http.Client{Timeout: 60 * time.Second}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"receptor_pdb":    cleanedPDB,
		"mode":            "native-ligand",
		"native_ligand_id": req.NativeLigandID,
		"padding":         req.Padding,
	})

	resp, err := client.Post(targetPrepServiceURL+"/binding-site", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to contact target-prep sidecar for binding site: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("target-prep sidecar returned %d: %s", resp.StatusCode, string(body))
	}

	var sidecarResp targetPrepSidecarResponse
	if err := json.NewDecoder(resp.Body).Decode(&sidecarResp); err != nil {
		return fmt.Errorf("failed to parse target-prep sidecar response: %w", err)
	}

	if sidecarResp.Error != "" {
		return fmt.Errorf("target-prep sidecar error: %s", sidecarResp.Error)
	}

	if sidecarResp.BindingSite == nil {
		return fmt.Errorf("target-prep sidecar returned no binding site")
	}

	// Store binding site in MySQL.
	bsJSON, _ := json.Marshal(sidecarResp.BindingSite)
	if _, err := db.ExecContext(ctx,
		`UPDATE target_prep_results SET binding_site = ? WHERE name = ?`,
		string(bsJSON), jobName); err != nil {
		return fmt.Errorf("failed to update binding site: %w", err)
	}

	log.Printf("[target-prep] %s: native-ligand binding site: center=[%.2f, %.2f, %.2f]",
		jobName, sidecarResp.BindingSite.Center[0], sidecarResp.BindingSite.Center[1], sidecarResp.BindingSite.Center[2])
	return nil
}

// processCustomBoxMode stores the user-specified custom box as the binding site.
func (h *APIHandler) processCustomBoxMode(ctx context.Context, db *sql.DB, jobName string, req TargetPrepRequest) error {
	if req.CustomBox == nil {
		return fmt.Errorf("custom_box is nil")
	}

	bsJSON, _ := json.Marshal(req.CustomBox)
	if _, err := db.ExecContext(ctx,
		`UPDATE target_prep_results SET binding_site = ? WHERE name = ?`,
		string(bsJSON), jobName); err != nil {
		return fmt.Errorf("failed to store custom box: %w", err)
	}

	log.Printf("[target-prep] %s: custom-box binding site: center=[%.2f, %.2f, %.2f]",
		jobName, req.CustomBox.Center[0], req.CustomBox.Center[1], req.CustomBox.Center[2])
	return nil
}

// processPocketDetectionMode runs fpocket and P2Rank in parallel, then
// computes consensus pocket rankings.
func (h *APIHandler) processPocketDetectionMode(ctx context.Context, db *sql.DB, jobName string, cleanedPDB string) error {
	type sidecarResult struct {
		pockets []DetectedPocket
		err     error
		tool    string
	}

	results := make(chan sidecarResult, 2)

	// Call fpocket sidecar.
	go func() {
		pockets, err := h.callFpocketSidecar(ctx, cleanedPDB)
		results <- sidecarResult{pockets: pockets, err: err, tool: "fpocket"}
	}()

	// Call P2Rank sidecar.
	go func() {
		pockets, err := h.callP2RankSidecar(ctx, cleanedPDB)
		results <- sidecarResult{pockets: pockets, err: err, tool: "p2rank"}
	}()

	// Collect results.
	var fpocketPockets, p2rankPockets []DetectedPocket
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			log.Printf("[target-prep] %s: warning: %s sidecar failed: %v", jobName, r.tool, r.err)
			// Continue — consensus ranking handles one tool returning no results.
		}
		switch r.tool {
		case "fpocket":
			fpocketPockets = r.pockets
		case "p2rank":
			p2rankPockets = r.pockets
		}
	}

	if len(fpocketPockets) == 0 && len(p2rankPockets) == 0 {
		return fmt.Errorf("both fpocket and P2Rank returned no pockets")
	}

	// Compute consensus ranking.
	rankedPockets := RankPockets(fpocketPockets, p2rankPockets)

	// Store pockets in MySQL.
	pocketsJSON, _ := json.Marshal(rankedPockets)
	if _, err := db.ExecContext(ctx,
		`UPDATE target_prep_results SET pockets = ? WHERE name = ?`,
		string(pocketsJSON), jobName); err != nil {
		return fmt.Errorf("failed to store pockets: %w", err)
	}

	// Auto-select top pocket as the binding site (can be overridden via select endpoint).
	if len(rankedPockets) > 0 {
		topPocket := rankedPockets[0]
		bs := BoxSpec{Center: topPocket.Center, Size: topPocket.Size}
		bsJSON, _ := json.Marshal(bs)
		if _, err := db.ExecContext(ctx,
			`UPDATE target_prep_results SET binding_site = ?, selected_pocket = 0 WHERE name = ?`,
			string(bsJSON), jobName); err != nil {
			log.Printf("[target-prep] %s: warning: failed to auto-select top pocket: %v", jobName, err)
		}
	}

	log.Printf("[target-prep] %s: pocket-detection found %d consensus-ranked pockets (fpocket=%d, p2rank=%d)",
		jobName, len(rankedPockets), len(fpocketPockets), len(p2rankPockets))
	return nil
}

// --- Sidecar calls ---

// callTargetPrepSidecar sends the PDB ID and parameters to the target-prep
// sidecar for receptor cleaning (PDBFixer, hydrogen addition, etc.).
// Returns the cleaned receptor PDB content.
func (h *APIHandler) callTargetPrepSidecar(ctx context.Context, req TargetPrepRequest) (*targetPrepSidecarResponse, error) {
	client := &http.Client{Timeout: 10 * time.Minute}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"pdb_id":            req.PDBID,
		"mode":              req.BindingSiteMode,
		"native_ligand_id":  req.NativeLigandID,
		"padding":           req.Padding,
		"pH":                req.PH,
		"keep_cofactors":    req.KeepCofactors,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		targetPrepServiceURL+"/prepare", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("contacting target-prep sidecar: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading target-prep sidecar response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("target-prep sidecar returned %d: %s", resp.StatusCode, string(body))
	}

	var sidecarResp targetPrepSidecarResponse
	if err := json.Unmarshal(body, &sidecarResp); err != nil {
		return nil, fmt.Errorf("parsing target-prep sidecar response: %w", err)
	}

	if sidecarResp.Error != "" {
		return nil, fmt.Errorf("target-prep sidecar: %s", sidecarResp.Error)
	}

	if sidecarResp.ReceptorPDB == "" {
		return nil, fmt.Errorf("target-prep sidecar returned empty receptor")
	}

	return &sidecarResp, nil
}

// callFpocketSidecar sends the cleaned receptor PDB to the fpocket sidecar
// and returns detected pockets with druggability scores.
func (h *APIHandler) callFpocketSidecar(ctx context.Context, receptorPDB string) ([]DetectedPocket, error) {
	client := &http.Client{Timeout: 2 * time.Minute}

	reqBody, _ := json.Marshal(map[string]string{
		"receptor_pdb": receptorPDB,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fpocketServiceURL+"/detect", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("creating fpocket request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("contacting fpocket sidecar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fpocket sidecar returned %d: %s", resp.StatusCode, string(body))
	}

	var sidecarResp fpocketSidecarResponse
	if err := json.NewDecoder(resp.Body).Decode(&sidecarResp); err != nil {
		return nil, fmt.Errorf("parsing fpocket response: %w", err)
	}

	if sidecarResp.Error != "" {
		return nil, fmt.Errorf("fpocket sidecar: %s", sidecarResp.Error)
	}

	return sidecarResp.Pockets, nil
}

// callP2RankSidecar sends the cleaned receptor PDB to the P2Rank sidecar
// and returns detected pockets with probability scores.
func (h *APIHandler) callP2RankSidecar(ctx context.Context, receptorPDB string) ([]DetectedPocket, error) {
	client := &http.Client{Timeout: 5 * time.Minute}

	reqBody, _ := json.Marshal(map[string]string{
		"receptor_pdb": receptorPDB,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p2rankServiceURL+"/predict", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("creating p2rank request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("contacting p2rank sidecar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("p2rank sidecar returned %d: %s", resp.StatusCode, string(body))
	}

	var sidecarResp p2rankSidecarResponse
	if err := json.NewDecoder(resp.Body).Decode(&sidecarResp); err != nil {
		return nil, fmt.Errorf("parsing p2rank response: %w", err)
	}

	if sidecarResp.Error != "" {
		return nil, fmt.Errorf("p2rank sidecar: %s", sidecarResp.Error)
	}

	return sidecarResp.Pockets, nil
}

// --- CRD integration ---

// createTargetPrepCRD creates a TargetPrep CRD instance in the chem namespace.
// Used when CRD_ENABLED=true. Falls back silently if the CRD is not installed.
func (h *APIHandler) createTargetPrepCRD(ctx context.Context, jobName string, req TargetPrepRequest, receptorS3Key string) {
	gvr := gvrForKind("TargetPrep")
	if gvr.Resource == "" {
		log.Printf("[target-prep] %s: TargetPrep CRD not registered, skipping CRD creation", jobName)
		return
	}

	spec := map[string]interface{}{
		"pdbId":           req.PDBID,
		"bindingSiteMode": req.BindingSiteMode,
		"padding":         req.Padding,
		"pH":              req.PH,
		"gate":            "manual", // API-created — no auto-launch needed.
	}

	if req.NativeLigandID != "" {
		spec["nativeLigandId"] = req.NativeLigandID
	}
	if req.CustomBox != nil {
		spec["customBox"] = map[string]interface{}{
			"center": req.CustomBox.Center,
			"size":   req.CustomBox.Size,
		}
	}

	crd := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "khemeia.io/v1alpha1",
			"kind":       "TargetPrep",
			"metadata": map[string]interface{}{
				"name":      jobName,
				"namespace": h.namespace,
				"labels": map[string]interface{}{
					"khemeia.io/pdb-id":     req.PDBID,
					"khemeia.io/managed-by": "khemeia-api",
				},
			},
			"spec": spec,
			"status": map[string]interface{}{
				"phase":    "Succeeded",
				"receptor": receptorS3Key,
			},
		},
	}

	_, err := h.controller.dynamicClient.Resource(gvr).Namespace(h.namespace).
		Create(ctx, crd, metav1.CreateOptions{})
	if err != nil {
		log.Printf("[target-prep] %s: warning: failed to create TargetPrep CRD: %v", jobName, err)
	} else {
		log.Printf("[target-prep] %s: TargetPrep CRD instance created", jobName)
	}
}

// updateTargetPrepCRDBindingSite updates the TargetPrep CRD status with the
// selected pocket's binding site coordinates.
func (h *APIHandler) updateTargetPrepCRDBindingSite(name string, bindingSite BoxSpec, pocketIndex int) {
	if os.Getenv("CRD_ENABLED") != "true" {
		return
	}

	gvr := gvrForKind("TargetPrep")
	if gvr.Resource == "" {
		return
	}

	ctx := context.Background()

	current, err := h.controller.dynamicClient.Resource(gvr).Namespace(h.namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		log.Printf("[target-prep] %s: warning: failed to get CRD for pocket update: %v", name, err)
		return
	}

	status := getStatusMap(current)
	status["bindingSite"] = map[string]interface{}{
		"center": bindingSite.Center,
		"size":   bindingSite.Size,
	}
	status["selectedPocket"] = int64(pocketIndex)
	current.Object["status"] = status

	_, err = h.controller.dynamicClient.Resource(gvr).Namespace(h.namespace).
		UpdateStatus(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		log.Printf("[target-prep] %s: warning: failed to update CRD binding site: %v", name, err)
	}
}

// --- Helper functions ---

// failTargetPrep marks a target prep job as Failed in MySQL with the given error message.
func (h *APIHandler) failTargetPrep(ctx context.Context, db *sql.DB, jobName string, errMsg string) {
	log.Printf("[target-prep] %s: FAILED: %s", jobName, errMsg)
	if _, err := db.ExecContext(ctx,
		`UPDATE target_prep_results SET phase = 'Failed', error_message = ?, completion_time = NOW() WHERE name = ?`,
		errMsg, jobName); err != nil {
		log.Printf("[target-prep] %s: failed to update status to Failed: %v", jobName, err)
	}
}

// nullString returns a sql.NullString for the given value (empty string = null).
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullBytes returns a sql.NullString for the given byte slice (nil/empty = null).
func nullBytes(b []byte) sql.NullString {
	if len(b) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: string(b), Valid: true}
}
