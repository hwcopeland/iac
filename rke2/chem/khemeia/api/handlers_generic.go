// Package main provides generic HTTP handlers parameterized by plugin definition.
// These replace the old hardcoded QE and docking handlers with a single set of
// handlers that work for any plugin.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ArtifactSummary is the metadata view of a job artifact (excludes binary content).
type ArtifactSummary struct {
	ID          int       `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int       `json:"size_bytes"`
	CreatedAt   time.Time `json:"created_at"`
}

// PluginJobSummary is the list-level view (omits large JSON fields for performance).
type PluginJobSummary struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	SubmittedBy *string    `json:"submitted_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// PluginJobDetail is the full view returned by PluginGet.
type PluginJobDetail struct {
	ID             int                    `json:"id"`
	Name           string                 `json:"name"`
	Status         string                 `json:"status"`
	SubmittedBy    *string                `json:"submitted_by,omitempty"`
	InputData      map[string]interface{} `json:"input_data,omitempty"`
	OutputData     map[string]interface{} `json:"output_data,omitempty"`
	ErrorOutput    *string                `json:"error_output,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	StartedAt      *time.Time             `json:"started_at,omitempty"`
	CompletedAt    *time.Time             `json:"completed_at,omitempty"`
	Artifacts      []ArtifactSummary      `json:"artifacts,omitempty"`
	DockingResults []DockingResult        `json:"docking_results,omitempty"`
	ReceptorPDBQT  *string               `json:"receptor_pdbqt,omitempty"`
}

// DockingResult represents a single docking result row from the docking_results table.
type DockingResult struct {
	CompoundID string  `json:"compound_id"`
	Affinity   float64 `json:"affinity_kcal_mol"`
	LigandID   int     `json:"ligand_id"`
	PosePDBQT  *string `json:"pose_pdbqt,omitempty"`
}

// PluginSubmit returns a handler that accepts job submissions for the given plugin.
// It validates input, inserts a row into the plugin's jobs table, launches a
// goroutine to run the job, and returns HTTP 202 Accepted.
func (h *APIHandler) PluginSubmit(plugin Plugin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		// Apply defaults before validation.
		plugin.ApplyDefaults(input)

		// Validate input against the plugin schema.
		if err := plugin.ValidateInput(input); err != nil {
			writeError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Serialize input to JSON for storage.
		inputJSON, err := json.Marshal(input)
		if err != nil {
			writeError(w, fmt.Sprintf("failed to serialize input: %v", err), http.StatusInternalServerError)
			return
		}

		jobName := fmt.Sprintf("%s-%d", plugin.Slug, time.Now().UnixNano())
		submittedBy := UserFromContext(r)

		db := h.pluginDB(plugin.Slug)
		if db == nil {
			writeError(w, fmt.Sprintf("database not available for plugin %s", plugin.Slug), http.StatusInternalServerError)
			return
		}

		_, err = db.ExecContext(r.Context(),
			fmt.Sprintf(`INSERT INTO %s (name, status, submitted_by, input_data) VALUES (?, 'Pending', ?, ?)`, plugin.TableName()),
			jobName, submittedBy, string(inputJSON))
		if err != nil {
			writeError(w, fmt.Sprintf("failed to create job: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":   jobName,
			"status": "Pending",
		})

		if plugin.Slug == "docking" {
			go h.controller.RunParallelDockingJob(plugin, jobName, input)
		} else {
			go h.controller.RunPluginJob(plugin, jobName, input)
		}
	}
}

// PluginList returns a handler that lists all jobs for the given plugin.
// Returns a summary view without input_data/output_data for performance.
func (h *APIHandler) PluginList(plugin Plugin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := h.pluginDB(plugin.Slug)
		if db == nil {
			writeError(w, fmt.Sprintf("database not available for plugin %s", plugin.Slug), http.StatusInternalServerError)
			return
		}

		rows, err := db.QueryContext(r.Context(),
			fmt.Sprintf(`SELECT id, name, status, submitted_by, created_at, started_at, completed_at
				FROM %s ORDER BY created_at DESC`, plugin.TableName()))
		if err != nil {
			writeError(w, fmt.Sprintf("failed to list jobs: %v", err), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var jobs []PluginJobSummary
		for rows.Next() {
			var j PluginJobSummary
			var submittedBy sql.NullString
			var startedAt, completedAt sql.NullTime

			if err := rows.Scan(&j.ID, &j.Name, &j.Status, &submittedBy,
				&j.CreatedAt, &startedAt, &completedAt); err != nil {
				writeError(w, fmt.Sprintf("failed to scan job: %v", err), http.StatusInternalServerError)
				return
			}
			if submittedBy.Valid {
				j.SubmittedBy = &submittedBy.String
			}
			if startedAt.Valid {
				j.StartedAt = &startedAt.Time
			}
			if completedAt.Valid {
				j.CompletedAt = &completedAt.Time
			}
			jobs = append(jobs, j)
		}
		if err := rows.Err(); err != nil {
			writeError(w, fmt.Sprintf("failed to iterate jobs: %v", err), http.StatusInternalServerError)
			return
		}

		if jobs == nil {
			jobs = []PluginJobSummary{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jobs":  jobs,
			"count": len(jobs),
		})
	}
}

// PluginGet returns a handler that retrieves a single job by name, including
// full input_data and output_data.
func (h *APIHandler) PluginGet(plugin Plugin) http.HandlerFunc {
	basePath := fmt.Sprintf("/api/v1/%s/jobs/", plugin.Slug)

	return func(w http.ResponseWriter, r *http.Request) {
		jobName := strings.TrimPrefix(r.URL.Path, basePath)
		if jobName == "" {
			writeError(w, "job name required", http.StatusBadRequest)
			return
		}

		db := h.pluginDB(plugin.Slug)
		if db == nil {
			writeError(w, fmt.Sprintf("database not available for plugin %s", plugin.Slug), http.StatusInternalServerError)
			return
		}

		var j PluginJobDetail
		var submittedBy, errorOutput sql.NullString
		var inputJSON, outputJSON sql.NullString
		var startedAt, completedAt sql.NullTime

		err := db.QueryRowContext(r.Context(),
			fmt.Sprintf(`SELECT id, name, status, submitted_by, input_data, output_data,
				error_output, created_at, started_at, completed_at
				FROM %s WHERE name = ?`, plugin.TableName()), jobName).Scan(
			&j.ID, &j.Name, &j.Status, &submittedBy, &inputJSON, &outputJSON,
			&errorOutput, &j.CreatedAt, &startedAt, &completedAt)
		if err == sql.ErrNoRows {
			writeError(w, "job not found", http.StatusNotFound)
			return
		}
		if err != nil {
			writeError(w, fmt.Sprintf("failed to get job: %v", err), http.StatusInternalServerError)
			return
		}

		if submittedBy.Valid {
			j.SubmittedBy = &submittedBy.String
		}
		if errorOutput.Valid {
			j.ErrorOutput = &errorOutput.String
		}
		if startedAt.Valid {
			j.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			j.CompletedAt = &completedAt.Time
		}
		if inputJSON.Valid {
			json.Unmarshal([]byte(inputJSON.String), &j.InputData)
		}
		if outputJSON.Valid {
			json.Unmarshal([]byte(outputJSON.String), &j.OutputData)
		}

		// Fetch artifact metadata for this job (excludes binary content).
		artifactRows, err := db.QueryContext(r.Context(),
			`SELECT id, filename, content_type, size_bytes, created_at FROM job_artifacts WHERE job_name = ?`, jobName)
		if err != nil {
			log.Printf("[%s] Warning: failed to query artifacts for job %s: %v", plugin.Slug, jobName, err)
		} else {
			defer artifactRows.Close()
			for artifactRows.Next() {
				var a ArtifactSummary
				if err := artifactRows.Scan(&a.ID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.CreatedAt); err != nil {
					log.Printf("[%s] Warning: failed to scan artifact row: %v", plugin.Slug, err)
					continue
				}
				j.Artifacts = append(j.Artifacts, a)
			}
		}

		// For docking jobs, fetch ranked results with pagination.
		if plugin.Slug == "docking" {
			perPage := 50
			page := 1
			if v := r.URL.Query().Get("per_page"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
					perPage = n
				}
			}
			if v := r.URL.Query().Get("page"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					page = n
				}
			}
			offset := (page - 1) * perPage

			// Total count for pagination
			var totalResults int
			_ = db.QueryRowContext(r.Context(),
				`SELECT COUNT(*) FROM docking_results WHERE workflow_name = ?`, jobName,
			).Scan(&totalResults)
			if j.OutputData == nil {
				j.OutputData = make(map[string]interface{})
			}
			j.OutputData["total_results"] = totalResults
			j.OutputData["page"] = page
			j.OutputData["per_page"] = perPage

			// If a specific compound is requested, fetch just that one.
			compoundFilter := r.URL.Query().Get("compound")
			var dockRows *sql.Rows
			if compoundFilter != "" {
				dockRows, err = db.QueryContext(r.Context(),
					`SELECT compound_id, affinity_kcal_mol, ligand_id, docked_pdbqt
					 FROM docking_results
					 WHERE workflow_name = ? AND compound_id = ?
					 LIMIT 1`, jobName, compoundFilter)
			} else {
				dockRows, err = db.QueryContext(r.Context(),
					`SELECT compound_id, affinity_kcal_mol, ligand_id, docked_pdbqt
					 FROM docking_results
					 WHERE workflow_name = ?
					 ORDER BY affinity_kcal_mol ASC
					 LIMIT ? OFFSET ?`, jobName, perPage, offset)
			}
			if err != nil {
				log.Printf("[docking] Warning: failed to query docking results for job %s: %v", jobName, err)
			} else {
				defer dockRows.Close()
				for dockRows.Next() {
					var dr DockingResult
					if err := dockRows.Scan(&dr.CompoundID, &dr.Affinity, &dr.LigandID, &dr.PosePDBQT); err != nil {
						log.Printf("[docking] Warning: failed to scan docking result: %v", err)
						continue
					}
					j.DockingResults = append(j.DockingResults, dr)
				}
			}

			// Fetch the preprocessed receptor from the docking workflow.
			var receptorPDBQT *string
			_ = db.QueryRowContext(r.Context(),
				`SELECT receptor_pdbqt FROM docking_workflows WHERE name = ?`, jobName,
			).Scan(&receptorPDBQT)
			j.ReceptorPDBQT = receptorPDBQT
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(j)
	}
}

// PluginDelete returns a handler that deletes a job by name, including cleanup
// of the associated K8s Job and ConfigMap.
func (h *APIHandler) PluginDelete(plugin Plugin) http.HandlerFunc {
	basePath := fmt.Sprintf("/api/v1/%s/jobs/", plugin.Slug)

	return func(w http.ResponseWriter, r *http.Request) {
		jobName := strings.TrimPrefix(r.URL.Path, basePath)
		if jobName == "" {
			writeError(w, "job name required", http.StatusBadRequest)
			return
		}

		db := h.pluginDB(plugin.Slug)
		if db == nil {
			writeError(w, fmt.Sprintf("database not available for plugin %s", plugin.Slug), http.StatusInternalServerError)
			return
		}

		// Delete the K8s Job if it still exists.
		propagation := metav1.DeletePropagationBackground
		if err := h.client.BatchV1().Jobs(h.namespace).Delete(r.Context(), jobName, metav1.DeleteOptions{
			PropagationPolicy: &propagation,
		}); err != nil && !k8serrors.IsNotFound(err) {
			log.Printf("[%s] Failed to delete K8s job %s: %v", plugin.Slug, jobName, err)
		}

		// Delete the input ConfigMap if it still exists.
		cmName := fmt.Sprintf("%s-input-%s", plugin.Slug, jobName)
		if err := h.client.CoreV1().ConfigMaps(h.namespace).Delete(r.Context(), cmName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
			log.Printf("[%s] Failed to delete ConfigMap %s: %v", plugin.Slug, cmName, err)
		}

		// Delete from MySQL.
		if _, err := db.ExecContext(r.Context(),
			fmt.Sprintf(`DELETE FROM %s WHERE name = ?`, plugin.TableName()), jobName); err != nil {
			log.Printf("[%s] Failed to delete job %s from DB: %v", plugin.Slug, jobName, err)
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// PluginListArtifacts returns a handler that lists artifact metadata for a specific job.
// GET /api/v1/{slug}/artifacts/{jobName}
func (h *APIHandler) PluginListArtifacts(plugin Plugin) http.HandlerFunc {
	basePath := fmt.Sprintf("/api/v1/%s/artifacts/", plugin.Slug)

	return func(w http.ResponseWriter, r *http.Request) {
		// Parse: /api/v1/{slug}/artifacts/{jobName}
		remainder := strings.TrimPrefix(r.URL.Path, basePath)
		remainder = strings.TrimRight(remainder, "/")
		if remainder == "" {
			writeError(w, "job name required", http.StatusBadRequest)
			return
		}

		// If there is a slash, the first segment is the job name, the rest is a filename.
		// This handler only handles the list case (no filename segment).
		if strings.Contains(remainder, "/") {
			writeError(w, "use the download endpoint for individual artifacts", http.StatusBadRequest)
			return
		}
		jobName := remainder

		db := h.pluginDB(plugin.Slug)
		if db == nil {
			writeError(w, fmt.Sprintf("database not available for plugin %s", plugin.Slug), http.StatusInternalServerError)
			return
		}

		rows, err := db.QueryContext(r.Context(),
			`SELECT id, filename, content_type, size_bytes, created_at FROM job_artifacts WHERE job_name = ?`, jobName)
		if err != nil {
			writeError(w, fmt.Sprintf("failed to query artifacts: %v", err), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var artifacts []ArtifactSummary
		for rows.Next() {
			var a ArtifactSummary
			if err := rows.Scan(&a.ID, &a.Filename, &a.ContentType, &a.SizeBytes, &a.CreatedAt); err != nil {
				writeError(w, fmt.Sprintf("failed to scan artifact: %v", err), http.StatusInternalServerError)
				return
			}
			artifacts = append(artifacts, a)
		}
		if err := rows.Err(); err != nil {
			writeError(w, fmt.Sprintf("failed to iterate artifacts: %v", err), http.StatusInternalServerError)
			return
		}

		if artifacts == nil {
			artifacts = []ArtifactSummary{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"artifacts": artifacts,
			"count":     len(artifacts),
		})
	}
}

// PluginDownloadArtifact returns a handler that serves a single artifact's binary content.
// GET /api/v1/{slug}/artifacts/{jobName}/{filename}
func (h *APIHandler) PluginDownloadArtifact(plugin Plugin) http.HandlerFunc {
	basePath := fmt.Sprintf("/api/v1/%s/artifacts/", plugin.Slug)

	return func(w http.ResponseWriter, r *http.Request) {
		// Parse: /api/v1/{slug}/artifacts/{jobName}/{filename}
		remainder := strings.TrimPrefix(r.URL.Path, basePath)
		slashIdx := strings.Index(remainder, "/")
		if slashIdx < 0 || slashIdx == len(remainder)-1 {
			writeError(w, "job name and filename required", http.StatusBadRequest)
			return
		}
		jobName := remainder[:slashIdx]
		filename := remainder[slashIdx+1:]

		db := h.pluginDB(plugin.Slug)
		if db == nil {
			writeError(w, fmt.Sprintf("database not available for plugin %s", plugin.Slug), http.StatusInternalServerError)
			return
		}

		var contentType string
		var content []byte
		err := db.QueryRowContext(r.Context(),
			`SELECT content_type, content FROM job_artifacts WHERE job_name = ? AND filename = ?`,
			jobName, filename).Scan(&contentType, &content)
		if err == sql.ErrNoRows {
			writeError(w, "artifact not found", http.StatusNotFound)
			return
		}
		if err != nil {
			writeError(w, fmt.Sprintf("failed to get artifact: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}
}

// pluginDB returns the database connection for the given plugin slug.
func (h *APIHandler) pluginDB(slug string) *sql.DB {
	if h.pluginDBs == nil {
		return nil
	}
	return h.pluginDBs[slug]
}

// PluginInfo represents a plugin's metadata and schema for the registry endpoint.
type PluginInfo struct {
	Name      string         `json:"name"`
	Slug      string         `json:"slug"`
	Version   string         `json:"version"`
	Type      string         `json:"type"`
	Input     []PluginInput  `json:"input"`
	Output    []PluginOutput `json:"output"`
}

// ListPlugins handles GET /api/v1/plugins — returns visible plugins with their
// input/output schemas for dynamic frontend form generation.
// Plugins with visible: false in their YAML are hidden from the UI but their
// API routes remain active for direct access and testing.
func (h *APIHandler) ListPlugins(w http.ResponseWriter, r *http.Request) {
	var infos []PluginInfo
	for _, p := range h.controller.plugins {
		if !p.IsVisible() {
			continue
		}
		infos = append(infos, PluginInfo{
			Name:    p.Name,
			Slug:    p.Slug,
			Version: p.Version,
			Type:    p.Type,
			Input:   p.Input,
			Output:  p.Output,
		})
	}

	if infos == nil {
		infos = []PluginInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"plugins": infos,
		"count":   len(infos),
	})
}
