// Package main provides docking set analysis endpoints.
// These aggregate pocket-level statistics across multiple top-scoring
// docked compounds for a given docking workflow.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// --- Receptor-contacts aggregate response types ---

// InteractionCounts holds per-interaction-type totals for a residue across
// multiple compounds.
type InteractionCounts struct {
	HBond       int `json:"hbond"`
	Hydrophobic int `json:"hydrophobic"`
	Ionic       int `json:"ionic"`
	Dipole      int `json:"dipole"`
	Contact     int `json:"contact"`
}

// AggregateResidueContact describes a single receptor residue's aggregate
// contact statistics across the top N docked compounds.
type AggregateResidueContact struct {
	ChainID             string            `json:"chain_id"`
	ResID               int               `json:"res_id"`
	ResName             string            `json:"res_name"`
	ContactFrequency    float64           `json:"contact_frequency"`
	AvgDistance          float64           `json:"avg_distance"`
	InteractionCounts   InteractionCounts `json:"interaction_counts"`
	CompoundsContacting int               `json:"compounds_contacting"`
}

// ReceptorContactsResponse is the JSON envelope for the receptor-contacts
// analysis endpoint.
type ReceptorContactsResponse struct {
	JobName                string                    `json:"job_name"`
	TopN                   int                       `json:"top_n"`
	ResidueContacts        []AggregateResidueContact `json:"residue_contacts"`
	TotalCompoundsAnalyzed int                       `json:"total_compounds_analyzed"`
}

// --- Fingerprints (compound list) response types ---

// CompoundEntry holds compound-level data returned by the fingerprints endpoint.
type CompoundEntry struct {
	CompoundID string  `json:"compound_id"`
	Smiles     string  `json:"smiles"`
	Affinity   float64 `json:"affinity"`
}

// FingerprintsResponse is the JSON envelope for the fingerprints endpoint.
type FingerprintsResponse struct {
	JobName   string          `json:"job_name"`
	Compounds []CompoundEntry `json:"compounds"`
	Total     int             `json:"total"`
}

// --- Internal aggregation bookkeeping ---

// residueAccumulator accumulates contact statistics for a single residue
// across multiple compounds.
type residueAccumulator struct {
	ChainID     string
	ResID       int
	ResName     string
	TotalDist   float64 // sum of minimum distances across compounds
	CompCount   int     // number of compounds that contact this residue
	HBond       int
	Hydrophobic int
	Ionic       int
	Dipole      int
	Contact     int
}

// --- HTTP handlers ---

// AnalysisDispatch handles GET /api/v1/docking/analysis/ and dispatches to
// the appropriate sub-handler based on the next path segment.
//
//	/api/v1/docking/analysis/receptor-contacts/{jobName}?top=50
//	/api/v1/docking/analysis/fingerprints/{jobName}?top=100
func (h *APIHandler) AnalysisDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	const basePath = "/api/v1/docking/analysis/"
	remainder := strings.TrimPrefix(r.URL.Path, basePath)
	remainder = strings.TrimRight(remainder, "/")

	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, "path must be /api/v1/docking/analysis/{endpoint}/{jobName}", http.StatusBadRequest)
		return
	}

	endpoint := parts[0]
	jobName := parts[1]

	switch endpoint {
	case "receptor-contacts":
		h.receptorContacts(w, r, jobName)
	case "fingerprints":
		h.fingerprints(w, r, jobName)
	default:
		writeError(w, fmt.Sprintf("unknown analysis endpoint %q", endpoint), http.StatusNotFound)
	}
}

// receptorContacts implements GET /api/v1/docking/analysis/receptor-contacts/{jobName}?top=50.
// It aggregates pocket residue contacts across the top N docked compounds for a workflow.
func (h *APIHandler) receptorContacts(w http.ResponseWriter, r *http.Request, jobName string) {
	topN := 50
	if v := r.URL.Query().Get("top"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 500 {
			topN = parsed
		} else {
			writeError(w, "top must be an integer between 1 and 500", http.StatusBadRequest)
			return
		}
	}

	db := h.pluginDB("docking")
	if db == nil {
		writeError(w, "docking database not available", http.StatusInternalServerError)
		return
	}

	// Fetch receptor PDBQT from docking_workflows.
	var receptorPDBQT *string
	err := db.QueryRowContext(r.Context(),
		`SELECT receptor_pdbqt FROM docking_workflows WHERE name = ?`, jobName,
	).Scan(&receptorPDBQT)
	if err != nil {
		writeError(w, fmt.Sprintf("job %q not found or has no receptor data", jobName), http.StatusNotFound)
		return
	}
	if receptorPDBQT == nil || *receptorPDBQT == "" {
		writeError(w, fmt.Sprintf("no receptor PDBQT available for job %q", jobName), http.StatusNotFound)
		return
	}

	receptorAtoms := parseAllAtoms(*receptorPDBQT)
	if len(receptorAtoms) == 0 {
		writeError(w, "failed to parse any receptor atoms from PDBQT", http.StatusInternalServerError)
		return
	}

	// Fetch top N docking results (best affinity = most negative) with docked poses.
	rows, err := db.QueryContext(r.Context(),
		`SELECT compound_id, docked_pdbqt
		 FROM docking_results
		 WHERE workflow_name = ? AND docked_pdbqt IS NOT NULL
		 ORDER BY affinity_kcal_mol ASC
		 LIMIT ?`,
		jobName, topN,
	)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query docking results: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Accumulate per-residue contact stats across all compounds.
	const cutoff = 5.0
	accumulators := make(map[residueKey]*residueAccumulator)
	compoundsAnalyzed := 0

	for rows.Next() {
		var compoundID string
		var dockedPDBQT []byte
		if err := rows.Scan(&compoundID, &dockedPDBQT); err != nil {
			continue
		}
		if len(dockedPDBQT) == 0 {
			continue
		}

		ligandAtoms := parseModel1Atoms(string(dockedPDBQT))
		if len(ligandAtoms) == 0 {
			continue
		}

		// Run the same pocket classification used by the single-compound endpoint.
		pocketResidues, _ := classifyPocket(receptorAtoms, ligandAtoms, cutoff)
		compoundsAnalyzed++

		// Merge this compound's pocket residues into the accumulators.
		for _, pr := range pocketResidues {
			key := residueKey{ChainID: pr.ChainID, ResID: pr.ResID, ResName: pr.ResName}
			acc, exists := accumulators[key]
			if !exists {
				acc = &residueAccumulator{
					ChainID: pr.ChainID,
					ResID:   pr.ResID,
					ResName: pr.ResName,
				}
				accumulators[key] = acc
			}

			acc.TotalDist += pr.MinDistance
			acc.CompCount++

			for _, inter := range pr.Interactions {
				switch inter {
				case "hbond":
					acc.HBond++
				case "hydrophobic":
					acc.Hydrophobic++
				case "ionic":
					acc.Ionic++
				case "dipole":
					acc.Dipole++
				case "contact":
					acc.Contact++
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, fmt.Sprintf("error reading docking results: %v", err), http.StatusInternalServerError)
		return
	}

	// Build the response slice from accumulators.
	contacts := make([]AggregateResidueContact, 0, len(accumulators))
	for _, acc := range accumulators {
		freq := 0.0
		avgDist := 0.0
		if compoundsAnalyzed > 0 {
			freq = float64(acc.CompCount) / float64(compoundsAnalyzed)
		}
		if acc.CompCount > 0 {
			avgDist = acc.TotalDist / float64(acc.CompCount)
		}

		contacts = append(contacts, AggregateResidueContact{
			ChainID:          acc.ChainID,
			ResID:            acc.ResID,
			ResName:          acc.ResName,
			ContactFrequency: math.Round(freq*100) / 100,
			AvgDistance:       math.Round(avgDist*10) / 10,
			InteractionCounts: InteractionCounts{
				HBond:       acc.HBond,
				Hydrophobic: acc.Hydrophobic,
				Ionic:       acc.Ionic,
				Dipole:      acc.Dipole,
				Contact:     acc.Contact,
			},
			CompoundsContacting: acc.CompCount,
		})
	}

	// Sort by contact_frequency descending (most commonly contacted first).
	sort.Slice(contacts, func(i, j int) bool {
		if contacts[i].ContactFrequency != contacts[j].ContactFrequency {
			return contacts[i].ContactFrequency > contacts[j].ContactFrequency
		}
		// Tie-break: lower residue ID first for deterministic ordering.
		if contacts[i].ChainID != contacts[j].ChainID {
			return contacts[i].ChainID < contacts[j].ChainID
		}
		return contacts[i].ResID < contacts[j].ResID
	})

	resp := ReceptorContactsResponse{
		JobName:                jobName,
		TopN:                   topN,
		ResidueContacts:        contacts,
		TotalCompoundsAnalyzed: compoundsAnalyzed,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// fingerprints implements GET /api/v1/docking/analysis/fingerprints/{jobName}?top=100.
// Returns SMILES + affinity data for the top N compounds so the frontend can compute
// fingerprints via RDKit WASM.
func (h *APIHandler) fingerprints(w http.ResponseWriter, r *http.Request, jobName string) {
	topN := 100
	if v := r.URL.Query().Get("top"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 1000 {
			topN = parsed
		} else {
			writeError(w, "top must be an integer between 1 and 1000", http.StatusBadRequest)
			return
		}
	}

	db := h.pluginDB("docking")
	if db == nil {
		writeError(w, "docking database not available", http.StatusInternalServerError)
		return
	}

	// Verify the job exists.
	var exists int
	err := db.QueryRowContext(r.Context(),
		`SELECT 1 FROM docking_workflows WHERE name = ? LIMIT 1`, jobName,
	).Scan(&exists)
	if err != nil {
		writeError(w, fmt.Sprintf("job %q not found", jobName), http.StatusNotFound)
		return
	}

	// Fetch top N compounds by affinity, joining with ligands for SMILES.
	rows, err := db.QueryContext(r.Context(),
		`SELECT dr.compound_id, l.smiles, dr.affinity_kcal_mol
		 FROM docking_results dr
		 JOIN ligands l ON dr.compound_id = l.compound_id
		 WHERE dr.workflow_name = ?
		 ORDER BY dr.affinity_kcal_mol ASC
		 LIMIT ?`,
		jobName, topN,
	)
	if err != nil {
		writeError(w, fmt.Sprintf("failed to query compounds: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	compounds := make([]CompoundEntry, 0)
	for rows.Next() {
		var c CompoundEntry
		if err := rows.Scan(&c.CompoundID, &c.Smiles, &c.Affinity); err != nil {
			continue
		}
		compounds = append(compounds, c)
	}
	if err := rows.Err(); err != nil {
		writeError(w, fmt.Sprintf("error reading compounds: %v", err), http.StatusInternalServerError)
		return
	}

	resp := FingerprintsResponse{
		JobName:   jobName,
		Compounds: compounds,
		Total:     len(compounds),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
