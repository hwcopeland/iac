// Package main provides a proxy endpoint for the ProLIF interaction analysis sidecar.
// The Go controller fetches receptor + ligand PDBQT from the database, sends them
// to the ProLIF Flask service, and returns the generated SVG interaction map.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// prolifServiceURL is the URL of the ProLIF sidecar service.
var prolifServiceURL = getProlifURL()

func getProlifURL() string {
	url := os.Getenv("PROLIF_SERVICE_URL")
	if url == "" {
		return "http://prolif-runner.chem.svc.cluster.local"
	}
	return url
}

// InteractionMapRequest is the JSON body sent to the ProLIF sidecar.
type InteractionMapRequest struct {
	ReceptorPDBQT string `json:"receptor_pdbqt"`
	LigandPDBQT   string `json:"ligand_pdbqt"`
	CompoundID    string `json:"compound_id"`
	DarkTheme     bool   `json:"dark_theme"`
}

// InteractionMapHandler handles GET /api/v1/docking/interaction-map/{jobName}/{compoundId}
// Fetches receptor + ligand PDBQT from DB, sends to ProLIF sidecar, returns SVG.
func (h *APIHandler) InteractionMapHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/v1/docking/interaction-map/{jobName}/{compoundId}
	basePath := "/api/v1/docking/interaction-map/"
	remainder := strings.TrimPrefix(r.URL.Path, basePath)
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, "job name and compound ID required", http.StatusBadRequest)
		return
	}
	jobName := parts[0]
	compoundID := parts[1]

	db := h.pluginDB("docking")
	if db == nil {
		writeError(w, "docking database not available", http.StatusInternalServerError)
		return
	}

	// Fetch receptor PDBQT
	var receptorPDBQT []byte
	err := db.QueryRowContext(r.Context(),
		`SELECT receptor_pdbqt FROM docking_workflows WHERE name = ?`, jobName,
	).Scan(&receptorPDBQT)
	if err != nil {
		writeError(w, "receptor not found", http.StatusNotFound)
		return
	}

	// Fetch docked ligand PDBQT (search across all workflows for this compound)
	var ligandPDBQT []byte
	err = db.QueryRowContext(r.Context(),
		`SELECT docked_pdbqt FROM docking_results WHERE compound_id = ? AND docked_pdbqt IS NOT NULL LIMIT 1`,
		compoundID,
	).Scan(&ligandPDBQT)
	if err != nil {
		writeError(w, "docked pose not found", http.StatusNotFound)
		return
	}

	// Extract MODEL 1 from ligand PDBQT
	model1 := extractModel1(string(ligandPDBQT))

	// Send to ProLIF sidecar
	reqBody := InteractionMapRequest{
		ReceptorPDBQT: string(receptorPDBQT),
		LigandPDBQT:   model1,
		CompoundID:    compoundID,
		DarkTheme:     true,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(prolifServiceURL+"/interaction-map", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		log.Printf("[prolif] Failed to reach ProLIF service: %v", err)
		writeError(w, fmt.Sprintf("ProLIF service unavailable: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	// Forward the response
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// extractModel1 returns only ATOM/HETATM lines from MODEL 1 of a multi-model PDBQT.
func extractModel1(pdbqt string) string {
	lines := strings.Split(pdbqt, "\n")
	var out []string
	inFirst := false
	for _, line := range lines {
		if strings.HasPrefix(line, "MODEL") {
			if inFirst {
				break
			}
			inFirst = true
			continue
		}
		if strings.HasPrefix(line, "ENDMDL") {
			break
		}
		if inFirst && (strings.HasPrefix(line, "HETATM") || strings.HasPrefix(line, "ATOM")) {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
