// Package main provides HTTP handlers for infrastructure endpoints.
// Plugin-specific handlers (submit, list, get, delete) are in handlers_generic.go.
package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// APIHandler handles HTTP requests for the Khemeia API.
type APIHandler struct {
	client     *kubernetes.Clientset
	namespace  string
	controller *Controller
	pluginDBs  map[string]*sql.DB
}

// NewAPIHandler creates a new API handler.
func NewAPIHandler(client *kubernetes.Clientset, namespace string, controller *Controller, pluginDBs map[string]*sql.DB) *APIHandler {
	return &APIHandler{
		client:     client,
		namespace:  namespace,
		controller: controller,
		pluginDBs:  pluginDBs,
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// HealthCheck handles GET /health.
func (h *APIHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// ReadinessCheck handles GET /readyz.
func (h *APIHandler) ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	// Check that at least one plugin database is reachable.
	for slug, db := range h.pluginDBs {
		if err := db.PingContext(r.Context()); err != nil {
			log.Printf("[ReadinessCheck] %s database ping failed: %v", slug, err)
			writeError(w, fmt.Sprintf("database %s not reachable", slug), http.StatusServiceUnavailable)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// --- Ligand management (docking infrastructure) ---

// LigandImportRequest represents a single ligand to import.
type LigandImportRequest struct {
	CompoundID string `json:"compound_id"`
	Smiles     string `json:"smiles"`
	PDBQTB64   string `json:"pdbqt_b64,omitempty"`
	SourceDb   string `json:"source_db"`
}

// ListLigandDatabases handles GET /api/v1/ligand-databases.
// Returns distinct source_db values from the ligands table with counts.
func (h *APIHandler) ListLigandDatabases(w http.ResponseWriter, r *http.Request) {
	db := h.pluginDB("docking")
	if db == nil {
		writeError(w, "docking database not available", http.StatusInternalServerError)
		return
	}

	rows, err := db.QueryContext(r.Context(),
		`SELECT source_db, COUNT(*) as count FROM ligands GROUP BY source_db ORDER BY source_db`)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query ligand databases: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type LigandDB struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	var dbs []LigandDB
	for rows.Next() {
		var d LigandDB
		if err := rows.Scan(&d.Name, &d.Count); err != nil {
			continue
		}
		dbs = append(dbs, d)
	}
	if dbs == nil {
		dbs = []LigandDB{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"databases": dbs,
	})
}

// ImportLigands handles POST /api/v1/ligands.
// Accepts a JSON array of ligands and upserts them into the ligands table.
func (h *APIHandler) ImportLigands(w http.ResponseWriter, r *http.Request) {
	db := h.pluginDB("docking")
	if db == nil {
		writeError(w, "docking database not available", http.StatusInternalServerError)
		return
	}

	var ligands []LigandImportRequest
	if err := json.NewDecoder(r.Body).Decode(&ligands); err != nil {
		writeError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}
	if len(ligands) == 0 {
		writeError(w, "empty ligand list", http.StatusBadRequest)
		return
	}

	imported := 0
	for _, lig := range ligands {
		if lig.CompoundID == "" || lig.Smiles == "" || lig.SourceDb == "" {
			continue
		}

		var pdbqt []byte
		if lig.PDBQTB64 != "" {
			var err error
			pdbqt, err = base64.StdEncoding.DecodeString(lig.PDBQTB64)
			if err != nil {
				log.Printf("[ImportLigands] bad base64 for %s: %v", lig.CompoundID, err)
				continue
			}
		}

		_, err := db.ExecContext(r.Context(),
			`INSERT INTO ligands (compound_id, smiles, pdbqt, source_db)
			 VALUES (?, ?, ?, ?)
			 ON DUPLICATE KEY UPDATE smiles = VALUES(smiles), pdbqt = VALUES(pdbqt)`,
			lig.CompoundID, lig.Smiles, pdbqt, lig.SourceDb)
		if err != nil {
			log.Printf("[ImportLigands] failed to upsert %s: %v", lig.CompoundID, err)
			continue
		}
		imported++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"imported": imported,
		"total":    len(ligands),
	})
}

// PrepRequest represents a request to prep ligands for docking.
type PrepRequest struct {
	SourceDb  string `json:"source_db"`
	ChunkSize int    `json:"chunk_size,omitempty"`
	Image     string `json:"image,omitempty"`
}

// StartPrep handles POST /api/v1/prep.
// Counts unprepared ligands for the given source_db, then launches batch prep jobs.
func (h *APIHandler) StartPrep(w http.ResponseWriter, r *http.Request) {
	db := h.pluginDB("docking")
	if db == nil {
		writeError(w, "docking database not available", http.StatusInternalServerError)
		return
	}

	var req PrepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.SourceDb == "" {
		writeError(w, "source_db is required", http.StatusBadRequest)
		return
	}

	var unpreparedCount int
	err := db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM ligands WHERE source_db = ? AND pdbqt IS NULL`,
		req.SourceDb).Scan(&unpreparedCount)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to count unprepared ligands: %v", err), http.StatusInternalServerError)
		return
	}

	if unpreparedCount == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "all ligands already prepped",
			"count":   0,
		})
		return
	}

	if req.ChunkSize == 0 {
		req.ChunkSize = 500
	}
	if req.Image == "" {
		req.Image = "zot.hwcopeland.net/chem/autodock-vina:latest"
	}

	batchCount := int(math.Ceil(float64(unpreparedCount) / float64(req.ChunkSize)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "prep started",
		"source_db":  req.SourceDb,
		"unprepared": unpreparedCount,
		"batches":    batchCount,
	})

	go h.controller.processLigandPrep(req)
}

// processLigandPrep runs sequential batch prep jobs to convert SMILES to PDBQT.
func (c *Controller) processLigandPrep(req PrepRequest) {
	db := c.pluginDB("docking")
	if db == nil {
		log.Printf("[prep] CRITICAL: no docking database")
		return
	}

	log.Printf("[prep] Starting ligand prep: source_db=%s chunk_size=%d image=%s",
		req.SourceDb, req.ChunkSize, req.Image)

	var unpreparedCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM ligands WHERE source_db = ? AND pdbqt IS NULL`,
		req.SourceDb).Scan(&unpreparedCount); err != nil {
		log.Printf("[prep] Failed to count unprepared ligands: %v", err)
		return
	}

	batchCount := int(math.Ceil(float64(unpreparedCount) / float64(req.ChunkSize)))
	log.Printf("[prep] %d unprepared ligands in %d batch(es)", unpreparedCount, batchCount)

	for i := 0; i < batchCount; i++ {
		offset := i * req.ChunkSize
		log.Printf("[prep] Batch %d/%d (offset=%d limit=%d)", i+1, batchCount, offset, req.ChunkSize)

		if err := c.createPrepBatchJob(req.SourceDb, req.Image, i, offset, req.ChunkSize); err != nil {
			log.Printf("[prep] Failed to create prep batch %d: %v", i, err)
			return
		}

		jobName := fmt.Sprintf("prep-%s-batch-%d", req.SourceDb, i)
		prepStreamCtx, prepStreamCancel := context.WithCancel(context.Background())
		go c.streamJobLogs(prepStreamCtx, jobName)
		if err := c.waitForPluginJobCompletion(jobName, 10*time.Minute); err != nil {
			prepStreamCancel()
			log.Printf("[prep] Prep batch %d failed: %v", i, err)
			return
		}
		prepStreamCancel()

		log.Printf("[prep] Batch %d/%d complete", i+1, batchCount)
	}

	log.Printf("[prep] All %d batches complete for source_db=%s", batchCount, req.SourceDb)
}

// createPrepBatchJob creates a K8s Job that runs prep_ligands.py for a ligand batch.
func (c *Controller) createPrepBatchJob(sourceDb, image string, batchIndex, offset, chunkSize int) error {
	workDir := "/data"
	j := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("prep-%s-batch-%d", sourceDb, batchIndex),
			Labels: map[string]string{
				"khemeia.io/job-type":   "prep-ligands",
				"khemeia.io/parent-job": fmt.Sprintf("prep-%s", sourceDb),
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptrInt32(300),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyOnFailure,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "zot-pull-secret"}},
					Containers: []corev1.Container{
						{
							Name:            "prep",
							Image:           image,
							ImagePullPolicy: corev1.PullAlways,
							WorkingDir:      workDir,
							Command:         []string{"python3", "/autodock/scripts/prep_ligands.py"},
							Env: []corev1.EnvVar{
								{Name: "SOURCE_DB", Value: sourceDb},
								{Name: "BATCH_OFFSET", Value: fmt.Sprintf("%d", offset)},
								{Name: "BATCH_LIMIT", Value: fmt.Sprintf("%d", chunkSize)},
								{Name: "MYSQL_HOST", Value: os.Getenv("MYSQL_HOST")},
								{Name: "MYSQL_PORT", Value: os.Getenv("MYSQL_PORT")},
								{Name: "MYSQL_USER", Value: os.Getenv("MYSQL_USER")},
								{Name: "MYSQL_DATABASE", Value: "docking"},
								{
									Name: "MYSQL_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: "docking-mysql-secret"},
											Key:                  "root-password",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{emptyDirMount("scratch", workDir)},
						},
					},
					Volumes: []corev1.Volume{emptyDirVolume("scratch")},
				},
			},
		},
	}

	_, err := c.jobClient.Create(context.TODO(), j, metav1.CreateOptions{})
	return err
}

// --- Pseudopotential management (QE infrastructure) ---

// UploadPseudopotential handles POST /api/v1/qe/pseudopotentials.
func (h *APIHandler) UploadPseudopotential(w http.ResponseWriter, r *http.Request) {
	db := h.pluginDB("qe")
	if db == nil {
		writeError(w, "QE database not available", http.StatusInternalServerError)
		return
	}

	var req struct {
		Filename   string `json:"filename"`
		ContentB64 string `json:"content_b64"`
		Element    string `json:"element"`
		Functional string `json:"functional,omitempty"`
		SourceURL  string `json:"source_url,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}
	if req.Filename == "" || req.ContentB64 == "" || req.Element == "" {
		writeError(w, "filename, content_b64, and element are required", http.StatusBadRequest)
		return
	}

	content, err := base64.StdEncoding.DecodeString(req.ContentB64)
	if err != nil {
		writeError(w, fmt.Sprintf("invalid base64: %v", err), http.StatusBadRequest)
		return
	}

	_, err = db.ExecContext(r.Context(),
		`INSERT INTO pseudopotentials (filename, content, element, functional, source_url)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE content = VALUES(content), functional = VALUES(functional)`,
		req.Filename, content, req.Element, req.Functional, req.SourceURL)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to store pseudopotential: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"filename": req.Filename,
		"size":     len(content),
	})
}

// ListPseudopotentials handles GET /api/v1/qe/pseudopotentials.
func (h *APIHandler) ListPseudopotentials(w http.ResponseWriter, r *http.Request) {
	db := h.pluginDB("qe")
	if db == nil {
		writeError(w, "QE database not available", http.StatusInternalServerError)
		return
	}

	rows, err := db.QueryContext(r.Context(),
		`SELECT filename, element, functional, LENGTH(content) as size, created_at FROM pseudopotentials ORDER BY element, filename`)
	if err != nil {
		writeError(w, fmt.Sprintf("query error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type ppEntry struct {
		Filename   string    `json:"filename"`
		Element    string    `json:"element"`
		Functional *string   `json:"functional,omitempty"`
		Size       int       `json:"size_bytes"`
		CreatedAt  time.Time `json:"created_at"`
	}
	var pps []ppEntry
	for rows.Next() {
		var p ppEntry
		if err := rows.Scan(&p.Filename, &p.Element, &p.Functional, &p.Size, &p.CreatedAt); err != nil {
			continue
		}
		pps = append(pps, p)
	}
	if pps == nil {
		pps = []ppEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pseudopotentials": pps,
		"count":            len(pps),
	})
}

// --- API Token management ---

// CreateAPIToken handles POST /api/v1/tokens.
func (h *APIHandler) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	var req struct {
		Username       string `json:"username"`
		ExpiresInHours int    `json:"expires_in_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Username == "" {
		writeError(w, "username is required", http.StatusBadRequest)
		return
	}
	if req.ExpiresInHours <= 0 {
		req.ExpiresInHours = 72
	}

	token, err := generateToken()
	if err != nil {
		writeError(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	expiresAt := time.Now().Add(time.Duration(req.ExpiresInHours) * time.Hour)
	_, err = db.Exec(
		"INSERT INTO api_tokens (token, username, expires_at) VALUES (?, ?, ?)",
		token, req.Username, expiresAt,
	)
	if err != nil {
		log.Printf("[CreateAPIToken] DB error: %v", err)
		writeError(w, "failed to store token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      token,
		"username":   req.Username,
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
}

// ListAPITokens handles GET /api/v1/tokens.
func (h *APIHandler) ListAPITokens(w http.ResponseWriter, r *http.Request) {
	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	rows, err := db.Query(
		"SELECT id, username, created_at, expires_at FROM api_tokens WHERE expires_at > NOW() ORDER BY created_at DESC",
	)
	if err != nil {
		writeError(w, "failed to list tokens", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type tokenEntry struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		CreatedAt string `json:"created_at"`
		ExpiresAt string `json:"expires_at"`
	}
	var tokens []tokenEntry
	for rows.Next() {
		var t tokenEntry
		var createdAt, expiresAt time.Time
		if err := rows.Scan(&t.ID, &t.Username, &createdAt, &expiresAt); err != nil {
			continue
		}
		t.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		t.ExpiresAt = expiresAt.UTC().Format(time.RFC3339)
		tokens = append(tokens, t)
	}
	if tokens == nil {
		tokens = []tokenEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"tokens": tokens})
}

// RevokeAPIToken handles DELETE /api/v1/tokens/{id}.
func (h *APIHandler) RevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	db := h.controller.firstDB()
	if db == nil {
		writeError(w, "no database available", http.StatusInternalServerError)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tokens/")
	if path == "" {
		writeError(w, "token id required", http.StatusBadRequest)
		return
	}

	result, err := db.Exec("DELETE FROM api_tokens WHERE id = ?", path)
	if err != nil {
		writeError(w, "failed to revoke token", http.StatusInternalServerError)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, "token not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})
}
