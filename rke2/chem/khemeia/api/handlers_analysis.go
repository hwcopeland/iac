// Package main provides docking set analysis endpoints.
// These aggregate pocket-level statistics across multiple top-scoring
// docked compounds for a given docking workflow.
package main

import (
	"context"
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
	InfluenceScore      float64           `json:"influence_score"`
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
	CompoundID    string   `json:"compound_id"`
	Smiles        string   `json:"smiles"`
	Affinity      float64  `json:"affinity"`
	MW            *float64 `json:"mw"`
	LogP          *float64 `json:"logp"`
	HBA           *int     `json:"hba"`
	HBD           *int     `json:"hbd"`
	PSA           *float64 `json:"psa"`
	RO5Violations *int     `json:"ro5_violations"`
	QED           *float64 `json:"qed"`
	ADMET         ADMETFlags `json:"admet"`
}

// ADMETFlags holds preliminary ADMET/drug-likeness predictions computed from
// molecular properties.
type ADMETFlags struct {
	Lipinski     bool `json:"lipinski"`      // Lipinski Rule of 5 (MW<500, LogP<5, HBA<10, HBD<5)
	Veber        bool `json:"veber"`         // Veber (PSA<=140, rotatable bonds not checked)
	LeadLike     bool `json:"lead_like"`     // Lead-like (MW 200-350, LogP -1 to 3.5, HBA<=7, HBD<=3)
	GoodQED      bool `json:"good_qed"`      // QED >= 0.5
	P450Risk     bool `json:"p450_risk"`     // High LogP (>3) suggests CYP metabolism liability
	HighPSA      bool `json:"high_psa"`      // PSA>140 — poor oral absorption
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
	// For influence score computation
	AffinityWeightedSum float64 // sum of normalized affinity for contacting compounds
	BeneficialCount     int     // H-bond + ionic + dipole (beneficial interactions)
	TotalInteractions   int     // all interactions (for beneficial ratio)
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

// receptorContacts implements GET /api/v1/docking/analysis/receptor-contacts/{jobName}?top=100.
// It aggregates pocket residue contacts across the top N docked compounds for a workflow.
func (h *APIHandler) receptorContacts(w http.ResponseWriter, r *http.Request, jobName string) {
	topN := 100
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
		`SELECT compound_id, affinity_kcal_mol, docked_pdbqt
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

	// First pass: collect all compound data (need affinities for normalization).
	type compoundData struct {
		CompoundID string
		Affinity   float64
		PosePDBQT  string
	}
	var compounds []compoundData
	for rows.Next() {
		var cd compoundData
		var pdbqt []byte
		if err := rows.Scan(&cd.CompoundID, &cd.Affinity, &pdbqt); err != nil {
			continue
		}
		if len(pdbqt) == 0 {
			continue
		}
		cd.PosePDBQT = string(pdbqt)
		compounds = append(compounds, cd)
	}
	if err := rows.Err(); err != nil {
		writeError(w, fmt.Sprintf("error reading docking results: %v", err), http.StatusInternalServerError)
		return
	}

	// Normalize affinities to [0, 1] where best (most negative) = 1.0.
	// Affinities are negative (e.g., -10 is better than -6).
	bestAffinity := 0.0
	worstAffinity := 0.0
	if len(compounds) > 0 {
		bestAffinity = compounds[0].Affinity  // most negative (sorted ASC)
		worstAffinity = compounds[len(compounds)-1].Affinity
	}
	affinityRange := worstAffinity - bestAffinity // positive number
	normalizeAffinity := func(aff float64) float64 {
		if affinityRange < 0.01 {
			return 1.0 // all compounds have similar affinity
		}
		return (worstAffinity - aff) / affinityRange // best → 1.0, worst → 0.0
	}

	// Compute total normalized affinity across all compounds (for affinity-weighted freq).
	totalNormAffinity := 0.0
	for _, cd := range compounds {
		totalNormAffinity += normalizeAffinity(cd.Affinity)
	}

	// Accumulate per-residue contact stats across all compounds.
	const cutoff = 5.0
	accumulators := make(map[residueKey]*residueAccumulator)
	compoundsAnalyzed := 0

	for _, cd := range compounds {
		ligandAtoms := parseModel1Atoms(cd.PosePDBQT)
		if len(ligandAtoms) == 0 {
			continue
		}

		pocketResidues, _ := classifyPocket(receptorAtoms, ligandAtoms, cutoff)
		compoundsAnalyzed++
		normAff := normalizeAffinity(cd.Affinity)

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
			acc.AffinityWeightedSum += normAff

			for _, inter := range pr.Interactions {
				acc.TotalInteractions++
				switch inter {
				case "hbond":
					acc.HBond++
					acc.BeneficialCount++ // H-bond = beneficial
				case "hydrophobic":
					acc.Hydrophobic++
					// hydrophobic = neutral (not counted as beneficial)
				case "ionic":
					acc.Ionic++
					acc.BeneficialCount++ // ionic/salt bridge = beneficial
				case "dipole":
					acc.Dipole++
					acc.BeneficialCount++ // pi-stacking/dipole = beneficial
				case "contact":
					acc.Contact++
					// van der Waals contact = neutral
				}
			}
		}
	}

	// Build the response slice from accumulators with influence scores.
	//
	// Influence Score = 0.4 * contact_freq
	//                 + 0.35 * affinity_weighted_freq
	//                 + 0.25 * beneficial_interaction_ratio
	//
	// contact_freq = compounds_contacting / total_compounds_analyzed
	// affinity_weighted_freq = sum(norm_affinity for contacting compounds) / sum(norm_affinity for all compounds)
	// beneficial_interaction_ratio = beneficial_count / total_interactions (H-bond, ionic, dipole = beneficial)
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

		// Affinity-weighted frequency
		affinityWeightedFreq := 0.0
		if totalNormAffinity > 0 {
			affinityWeightedFreq = acc.AffinityWeightedSum / totalNormAffinity
		}

		// Beneficial interaction ratio
		beneficialRatio := 0.5 // default neutral if no interactions
		if acc.TotalInteractions > 0 {
			beneficialRatio = float64(acc.BeneficialCount) / float64(acc.TotalInteractions)
		}

		influence := 0.4*freq + 0.35*affinityWeightedFreq + 0.25*beneficialRatio
		influence = math.Round(influence*1000) / 1000 // 3 decimal places

		contacts = append(contacts, AggregateResidueContact{
			ChainID:          acc.ChainID,
			ResID:            acc.ResID,
			ResName:          acc.ResName,
			ContactFrequency: math.Round(freq*100) / 100,
			InfluenceScore:   influence,
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

	// Sort by influence_score descending (most important residues first).
	sort.Slice(contacts, func(i, j int) bool {
		if contacts[i].InfluenceScore != contacts[j].InfluenceScore {
			return contacts[i].InfluenceScore > contacts[j].InfluenceScore
		}
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
// Returns SMILES + affinity + molecular properties + ADMET flags for the top N compounds.
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
	compoundIDs := make([]string, 0)
	for rows.Next() {
		var c CompoundEntry
		if err := rows.Scan(&c.CompoundID, &c.Smiles, &c.Affinity); err != nil {
			continue
		}
		compounds = append(compounds, c)
		compoundIDs = append(compoundIDs, c.CompoundID)
	}
	if err := rows.Err(); err != nil {
		writeError(w, fmt.Sprintf("error reading compounds: %v", err), http.StatusInternalServerError)
		return
	}

	// Enrich with molecular properties from ChEMBL if available.
	if h.chemblDB != nil && len(compoundIDs) > 0 {
		h.enrichWithChEMBLProperties(r.Context(), compounds, compoundIDs)
	}

	resp := FingerprintsResponse{
		JobName:   jobName,
		Compounds: compounds,
		Total:     len(compounds),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// enrichWithChEMBLProperties fetches molecular properties from ChEMBL and
// computes ADMET flags for each compound.
func (h *APIHandler) enrichWithChEMBLProperties(
	ctx context.Context,
	compounds []CompoundEntry,
	compoundIDs []string,
) {
	if len(compoundIDs) == 0 {
		return
	}

	// Build IN clause with placeholders.
	placeholders := make([]string, len(compoundIDs))
	args := make([]interface{}, len(compoundIDs))
	for i, id := range compoundIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT md.chembl_id, cp.mw_freebase, cp.alogp, cp.hba, cp.hbd,
		       cp.psa, cp.num_ro5_violations, cp.qed_weighted
		FROM molecule_dictionary md
		JOIN compound_properties cp ON md.molregno = cp.molregno
		WHERE md.chembl_id IN (%s)`,
		strings.Join(placeholders, ","),
	)

	rows, err := h.chemblDB.QueryContext(ctx, query, args...)
	if err != nil {
		return // silently degrade — properties are optional enrichment
	}
	defer rows.Close()

	// Build a lookup map.
	type props struct {
		MW            *float64
		LogP          *float64
		HBA           *int
		HBD           *int
		PSA           *float64
		RO5Violations *int
		QED           *float64
	}
	propMap := make(map[string]props)

	for rows.Next() {
		var chemblID string
		var p props
		if err := rows.Scan(&chemblID, &p.MW, &p.LogP, &p.HBA, &p.HBD,
			&p.PSA, &p.RO5Violations, &p.QED); err != nil {
			continue
		}
		propMap[chemblID] = p
	}

	// Merge properties and compute ADMET flags.
	for i := range compounds {
		p, ok := propMap[compounds[i].CompoundID]
		if !ok {
			continue
		}
		compounds[i].MW = p.MW
		compounds[i].LogP = p.LogP
		compounds[i].HBA = p.HBA
		compounds[i].HBD = p.HBD
		compounds[i].PSA = p.PSA
		compounds[i].RO5Violations = p.RO5Violations
		compounds[i].QED = p.QED
		compounds[i].ADMET = computeADMET(p.MW, p.LogP, p.HBA, p.HBD, p.PSA, p.QED)
	}
}

// computeADMET derives preliminary ADMET/drug-likeness flags from molecular
// properties.
func computeADMET(mw, logp *float64, hba, hbd *int, psa, qed *float64) ADMETFlags {
	flags := ADMETFlags{}

	// Lipinski Rule of 5: MW<500, LogP<5, HBA<10, HBD<5
	if mw != nil && logp != nil && hba != nil && hbd != nil {
		flags.Lipinski = *mw < 500 && *logp < 5 && *hba < 10 && *hbd < 5
	}

	// Veber: PSA <= 140 (rotatable bonds not available here)
	if psa != nil {
		flags.Veber = *psa <= 140
		flags.HighPSA = *psa > 140
	}

	// Lead-like: MW 200-350, LogP -1 to 3.5, HBA<=7, HBD<=3
	if mw != nil && logp != nil && hba != nil && hbd != nil {
		flags.LeadLike = *mw >= 200 && *mw <= 350 &&
			*logp >= -1 && *logp <= 3.5 &&
			*hba <= 7 && *hbd <= 3
	}

	// QED quality
	if qed != nil {
		flags.GoodQED = *qed >= 0.5
	}

	// P450 risk: high lipophilicity tends toward CYP metabolism
	if logp != nil {
		flags.P450Risk = *logp > 3.0
	}

	return flags
}
