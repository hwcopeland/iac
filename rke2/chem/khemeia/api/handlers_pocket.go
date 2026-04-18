// Package main provides the binding pocket analysis handler.
// Given a docking job name and compound ID, this endpoint parses the receptor
// and docked ligand PDBQT coordinates, identifies receptor residues within a
// distance cutoff of the ligand, and classifies protein-ligand interactions.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// --- PDBQT atom representation ---

// pdbqtAtom holds the parsed fields from a single ATOM/HETATM record in a
// PDBQT file. Fields follow the fixed-width PDB column conventions.
type pdbqtAtom struct {
	Serial  int
	Name    string
	ResName string
	ChainID string
	ResID   int
	X, Y, Z float64
	Element string // single-letter element derived from the Vina atom type
}

// residueKey uniquely identifies a residue within a protein chain.
type residueKey struct {
	ChainID string
	ResID   int
	ResName string
}

// --- Pocket analysis response types ---

// PocketResidue describes a single receptor residue in contact with the ligand.
type PocketResidue struct {
	ChainID      string   `json:"chain_id"`
	ResID        int      `json:"res_id"`
	ResName      string   `json:"res_name"`
	MinDistance   float64  `json:"min_distance"`
	Interactions []string `json:"interactions"`
	ContactAtoms int      `json:"contact_atoms"`
}

// PocketAnalysisResponse is the JSON envelope returned by the pocket endpoint.
type PocketAnalysisResponse struct {
	CompoundID     string          `json:"compound_id"`
	CutoffAngstrom float64         `json:"cutoff_angstrom"`
	PocketResidues []PocketResidue `json:"pocket_residues"`
	TotalContacts  int             `json:"total_contacts"`
	LigandAtoms    int             `json:"ligand_atoms"`
}

// --- PDBQT parsing ---

// vinaTypeToElement maps the AutoDock Vina atom type (found at column 78+)
// to a single-letter element symbol used for interaction classification.
var vinaTypeToElement = map[string]string{
	"OA": "O",
	"NA": "N",
	"SA": "S",
	"HD": "H",
	"A":  "C", // aromatic carbon
	"C":  "C",
	"N":  "N",
	"O":  "O",
	"S":  "S",
	"H":  "H",
	"F":  "F",
	"P":  "P",
	"Cl": "Cl",
	"Br": "Br",
	"I":  "I",
}

// parsePDBQTAtom parses a single ATOM or HETATM line from a PDBQT file into
// a pdbqtAtom. Returns nil if the line is too short or cannot be parsed.
// PDBQT column layout (1-indexed):
//
//	1-6   record type (ATOM / HETATM)
//	7-11  serial number
//	13-16 atom name
//	18-20 residue name
//	22    chain ID
//	23-26 residue sequence number
//	31-38 X coordinate
//	39-46 Y coordinate
//	47-54 Z coordinate
//	78+   Vina atom type
func parsePDBQTAtom(line string) *pdbqtAtom {
	if len(line) < 54 {
		return nil
	}

	record := strings.TrimSpace(line[:6])
	if record != "ATOM" && record != "HETATM" {
		return nil
	}

	serial, err := strconv.Atoi(strings.TrimSpace(line[6:11]))
	if err != nil {
		serial = 0
	}

	atomName := strings.TrimSpace(line[12:16])

	resName := ""
	if len(line) >= 20 {
		resName = strings.TrimSpace(line[17:20])
	}

	chainID := " "
	if len(line) >= 22 {
		chainID = string(line[21])
		if chainID == " " {
			chainID = "A" // default chain when blank
		}
	}

	resID := 0
	if len(line) >= 26 {
		resID, _ = strconv.Atoi(strings.TrimSpace(line[22:26]))
	}

	x, errX := strconv.ParseFloat(strings.TrimSpace(line[30:38]), 64)
	y, errY := strconv.ParseFloat(strings.TrimSpace(line[38:46]), 64)
	z, errZ := strconv.ParseFloat(strings.TrimSpace(line[46:54]), 64)
	if errX != nil || errY != nil || errZ != nil {
		return nil
	}

	// Parse Vina atom type from column 78+ for element classification.
	element := "C" // default to carbon if type field is missing
	if len(line) >= 78 {
		vinaType := strings.TrimSpace(line[77:])
		// Take the first whitespace-delimited token.
		if idx := strings.IndexByte(vinaType, ' '); idx > 0 {
			vinaType = vinaType[:idx]
		}
		if mapped, ok := vinaTypeToElement[vinaType]; ok {
			element = mapped
		} else if len(vinaType) > 0 {
			// Fallback: use first character uppercased.
			element = strings.ToUpper(vinaType[:1])
		}
	} else {
		// Fall back to atom name for element guess.
		element = guessElementFromName(atomName)
	}

	return &pdbqtAtom{
		Serial:  serial,
		Name:    atomName,
		ResName: resName,
		ChainID: chainID,
		ResID:   resID,
		X:       x,
		Y:       y,
		Z:       z,
		Element: element,
	}
}

// guessElementFromName derives a single-letter element from a PDB atom name.
// PDB atom names are left-justified for 2-char elements (e.g., "CA" = calcium)
// and right-justified for 1-char elements (e.g., " CA " = carbon-alpha).
func guessElementFromName(name string) string {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) == 0 {
		return "C"
	}
	first := trimmed[0]
	switch {
	case first == 'N':
		return "N"
	case first == 'O':
		return "O"
	case first == 'S':
		return "S"
	case first == 'H':
		return "H"
	case first == 'P':
		return "P"
	case first == 'F':
		return "F"
	default:
		return "C"
	}
}

// parseAllAtoms parses all ATOM/HETATM records from a PDBQT string.
func parseAllAtoms(pdbqt string) []pdbqtAtom {
	var atoms []pdbqtAtom
	for _, line := range strings.Split(pdbqt, "\n") {
		if a := parsePDBQTAtom(line); a != nil {
			atoms = append(atoms, *a)
		}
	}
	return atoms
}

// parseModel1Atoms parses only the ATOM/HETATM records from MODEL 1 of a
// multi-model PDBQT string (as produced by AutoDock Vina). Lines between
// "MODEL 1" and the first "ENDMDL" are considered. If there is no MODEL
// record (single-model file), all atoms are returned.
// ROOT, BRANCH, ENDBRANCH, ENDROOT, TORSDOF lines are skipped.
func parseModel1Atoms(pdbqt string) []pdbqtAtom {
	lines := strings.Split(pdbqt, "\n")

	// Check if this is a multi-model file.
	hasModels := false
	for _, line := range lines {
		if strings.HasPrefix(line, "MODEL") {
			hasModels = true
			break
		}
	}

	if !hasModels {
		return parseAllAtoms(pdbqt)
	}

	var atoms []pdbqtAtom
	inModel1 := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "MODEL") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 && fields[1] == "1" {
				inModel1 = true
				continue
			}
		}
		if inModel1 && strings.HasPrefix(trimmed, "ENDMDL") {
			break
		}
		if inModel1 {
			if a := parsePDBQTAtom(line); a != nil {
				atoms = append(atoms, *a)
			}
		}
	}
	return atoms
}

// --- Distance and interaction classification ---

// dist computes the Euclidean distance between two atoms.
func dist(a, b pdbqtAtom) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	dz := a.Z - b.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// chargedResidues is the set of residue names considered charged for ionic
// interaction classification.
var chargedResidues = map[string]bool{
	"ASP": true,
	"GLU": true,
	"LYS": true,
	"ARG": true,
	"HIS": true,
}

// isHBondElement returns true if the element can participate in hydrogen bonds.
func isHBondElement(elem string) bool {
	return elem == "N" || elem == "O"
}

// isCarbonElement returns true for non-polar carbon.
func isCarbonElement(elem string) bool {
	return elem == "C"
}

// residueInteractions holds per-residue contact statistics accumulated during
// the distance scan.
type residueInteractions struct {
	MinDist      float64
	ContactAtoms int
	HasHBond     bool
	HasHydro     bool
	HasIonic     bool
}

// classifyPocket performs the full pocket analysis: for each receptor atom,
// finds the nearest ligand atom, accumulates per-residue statistics, and
// classifies interactions.
func classifyPocket(receptorAtoms, ligandAtoms []pdbqtAtom, cutoff float64) []PocketResidue {
	if len(receptorAtoms) == 0 || len(ligandAtoms) == 0 {
		return []PocketResidue{}
	}

	// Precompute ligand coordinates for cache locality.
	type ligCoord struct {
		x, y, z float64
		element  string
	}
	ligCoords := make([]ligCoord, len(ligandAtoms))
	for i, la := range ligandAtoms {
		ligCoords[i] = ligCoord{x: la.X, y: la.Y, z: la.Z, element: la.Element}
	}

	cutoffSq := cutoff * cutoff
	hbondCutoff := 3.5
	hbondCutoffSq := hbondCutoff * hbondCutoff
	hydroCutoff := 4.0
	hydroCutoffSq := hydroCutoff * hydroCutoff
	ionicCutoff := 4.0
	ionicCutoffSq := ionicCutoff * ionicCutoff

	residueMap := make(map[residueKey]*residueInteractions)

	for _, ra := range receptorAtoms {
		// Find the minimum distance from this receptor atom to any ligand atom.
		minDistSq := math.MaxFloat64
		var nearestLig ligCoord
		for _, lc := range ligCoords {
			dx := ra.X - lc.x
			dy := ra.Y - lc.y
			dz := ra.Z - lc.z
			dsq := dx*dx + dy*dy + dz*dz
			if dsq < minDistSq {
				minDistSq = dsq
				nearestLig = lc
			}
		}

		if minDistSq > cutoffSq {
			continue
		}

		key := residueKey{ChainID: ra.ChainID, ResID: ra.ResID, ResName: ra.ResName}
		ri, exists := residueMap[key]
		if !exists {
			ri = &residueInteractions{MinDist: math.Sqrt(minDistSq)}
			residueMap[key] = ri
		}

		d := math.Sqrt(minDistSq)
		if d < ri.MinDist {
			ri.MinDist = d
		}
		ri.ContactAtoms++

		// Classify interactions based on the nearest ligand atom to this
		// receptor atom. We check all specific interaction types for each
		// receptor-ligand atom pair within their respective cutoffs.

		// H-bond: receptor N/O within 3.5A of ligand N/O.
		if minDistSq <= hbondCutoffSq && isHBondElement(ra.Element) && isHBondElement(nearestLig.element) {
			ri.HasHBond = true
		}

		// Hydrophobic: receptor C within 4.0A of ligand C.
		if minDistSq <= hydroCutoffSq && isCarbonElement(ra.Element) && isCarbonElement(nearestLig.element) {
			ri.HasHydro = true
		}

		// Ionic: charged residue within 4.0A of ligand N/O.
		if minDistSq <= ionicCutoffSq && chargedResidues[ra.ResName] && isHBondElement(nearestLig.element) {
			ri.HasIonic = true
		}
	}

	// Build the result slice, sorted by min distance (lowest first).
	results := make([]PocketResidue, 0, len(residueMap))
	for key, ri := range residueMap {
		var interactions []string
		if ri.HasHBond {
			interactions = append(interactions, "hbond")
		}
		if ri.HasHydro {
			interactions = append(interactions, "hydrophobic")
		}
		if ri.HasIonic {
			interactions = append(interactions, "ionic")
		}
		interactions = append(interactions, "contact")

		results = append(results, PocketResidue{
			ChainID:      key.ChainID,
			ResID:        key.ResID,
			ResName:      key.ResName,
			MinDistance:   math.Round(ri.MinDist*100) / 100, // 2 decimal places
			Interactions: interactions,
			ContactAtoms: ri.ContactAtoms,
		})
	}

	// Sort by min distance ascending for a consistent, useful ordering.
	sortPocketResidues(results)
	return results
}

// sortPocketResidues sorts residues by MinDistance ascending using insertion
// sort (fine for the expected ~20-50 pocket residues).
func sortPocketResidues(residues []PocketResidue) {
	for i := 1; i < len(residues); i++ {
		key := residues[i]
		j := i - 1
		for j >= 0 && residues[j].MinDistance > key.MinDistance {
			residues[j+1] = residues[j]
			j--
		}
		residues[j+1] = key
	}
}

// --- HTTP handler ---

// PocketAnalysis handles GET /api/v1/docking/pocket/{jobName}/{compoundId}.
// It fetches the receptor and docked ligand PDBQT data, parses atomic
// coordinates, identifies binding pocket residues within a distance cutoff,
// and classifies protein-ligand interactions.
func (h *APIHandler) PocketAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/v1/docking/pocket/{jobName}/{compoundId}
	const basePath = "/api/v1/docking/pocket/"
	remainder := strings.TrimPrefix(r.URL.Path, basePath)
	remainder = strings.TrimRight(remainder, "/")
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeError(w, "path must be /api/v1/docking/pocket/{jobName}/{compoundId}", http.StatusBadRequest)
		return
	}
	jobName := parts[0]
	compoundID := parts[1]

	// Parse optional cutoff query parameter (default 5.0 Angstroms).
	cutoff := 5.0
	if v := r.URL.Query().Get("cutoff"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed > 0 && parsed <= 20 {
			cutoff = parsed
		} else {
			writeError(w, "cutoff must be a number between 0 and 20", http.StatusBadRequest)
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

	// Fetch docked ligand PDBQT from docking_results.
	var dockedPDBQT *string
	err = db.QueryRowContext(r.Context(),
		`SELECT docked_pdbqt FROM docking_results WHERE workflow_name = ? AND compound_id = ?`,
		jobName, compoundID,
	).Scan(&dockedPDBQT)
	if err != nil {
		writeError(w, fmt.Sprintf("no docking result for compound %q in job %q", compoundID, jobName), http.StatusNotFound)
		return
	}
	if dockedPDBQT == nil || *dockedPDBQT == "" {
		writeError(w, fmt.Sprintf("no docked pose available for compound %q", compoundID), http.StatusNotFound)
		return
	}

	// Parse atoms.
	receptorAtoms := parseAllAtoms(*receptorPDBQT)
	ligandAtoms := parseModel1Atoms(*dockedPDBQT)

	if len(receptorAtoms) == 0 {
		writeError(w, "failed to parse any receptor atoms from PDBQT", http.StatusInternalServerError)
		return
	}
	if len(ligandAtoms) == 0 {
		writeError(w, "failed to parse any ligand atoms from PDBQT MODEL 1", http.StatusInternalServerError)
		return
	}

	// Classify pocket.
	pocketResidues := classifyPocket(receptorAtoms, ligandAtoms, cutoff)

	// Sum total contact atoms across all residues.
	totalContacts := 0
	for _, pr := range pocketResidues {
		totalContacts += pr.ContactAtoms
	}

	resp := PocketAnalysisResponse{
		CompoundID:     compoundID,
		CutoffAngstrom: cutoff,
		PocketResidues: pocketResidues,
		TotalContacts:  totalContacts,
		LigandAtoms:    len(ligandAtoms),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
