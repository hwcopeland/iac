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
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	ID          int                    `json:"id"`
	Name        string                 `json:"name"`
	Status      string                 `json:"status"`
	SubmittedBy *string                `json:"submitted_by,omitempty"`
	InputData   map[string]interface{} `json:"input_data,omitempty"`
	OutputData  map[string]interface{} `json:"output_data,omitempty"`
	ErrorOutput *string                `json:"error_output,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
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

		go h.controller.RunPluginJob(plugin, jobName, input)
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

// ListPlugins handles GET /api/v1/plugins — returns all loaded plugins with their
// input/output schemas for dynamic frontend form generation.
func (h *APIHandler) ListPlugins(w http.ResponseWriter, r *http.Request) {
	var infos []PluginInfo
	for _, p := range h.controller.plugins {
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
