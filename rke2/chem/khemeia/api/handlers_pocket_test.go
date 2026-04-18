package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// sampleReceptorPDBQT is a minimal receptor PDBQT excerpt with a few residues
// at known coordinates. Columns are fixed-width per PDB/PDBQT convention.
const sampleReceptorPDBQT = `ATOM      1  N   ASP A 107      10.000  10.000  10.000  1.00  0.00     0.000 NA
ATOM      2  CA  ASP A 107      11.000  10.000  10.000  1.00  0.00     0.000 C
ATOM      3  C   ASP A 107      12.000  10.000  10.000  1.00  0.00     0.000 C
ATOM      4  O   ASP A 107      10.500  10.500  10.000  1.00  0.00     0.000 OA
ATOM      5  CB  ASP A 107      10.200  10.200  10.200  1.00  0.00     0.000 C
ATOM     10  N   LEU A 110      20.000  20.000  20.000  1.00  0.00     0.000 NA
ATOM     11  CA  LEU A 110      21.000  20.000  20.000  1.00  0.00     0.000 C
ATOM     12  CB  LEU A 110      21.500  20.000  20.000  1.00  0.00     0.000 C
ATOM     20  CA  ALA A 200      50.000  50.000  50.000  1.00  0.00     0.000 C
`

// sampleDockedPDBQT is a minimal docked ligand PDBQT with MODEL 1 containing
// atoms near the ASP 107 residue and a MODEL 2 that should be ignored.
const sampleDockedPDBQT = `MODEL 1
ROOT
HETATM    1  C1  LIG A   1      10.500  10.000  10.000  1.00  0.00     0.000 C
HETATM    2  N1  LIG A   1      10.200  10.100  10.000  1.00  0.00     0.000 NA
HETATM    3  O1  LIG A   1      10.100  10.000  10.100  1.00  0.00     0.000 OA
HETATM    4  C2  LIG A   1      11.200  10.000  10.000  1.00  0.00     0.000 C
ENDROOT
BRANCH   1   5
HETATM    5  C3  LIG A   1      10.800  10.300  10.000  1.00  0.00     0.000 C
ENDBRANCH   1   5
TORSDOF 1
ENDMDL
MODEL 2
HETATM    1  C1  LIG A   1      99.000  99.000  99.000  1.00  0.00     0.000 C
ENDMDL
`

func TestParsePDBQTAtom(t *testing.T) {
	line := "ATOM      1  N   ASP A 107      10.000  10.000  10.000  1.00  0.00     0.000 NA"
	atom := parsePDBQTAtom(line)
	if atom == nil {
		t.Fatal("parsePDBQTAtom returned nil for valid ATOM line")
	}

	if atom.Serial != 1 {
		t.Errorf("serial: got %d, want 1", atom.Serial)
	}
	if atom.Name != "N" {
		t.Errorf("name: got %q, want %q", atom.Name, "N")
	}
	if atom.ResName != "ASP" {
		t.Errorf("resName: got %q, want %q", atom.ResName, "ASP")
	}
	if atom.ChainID != "A" {
		t.Errorf("chainID: got %q, want %q", atom.ChainID, "A")
	}
	if atom.ResID != 107 {
		t.Errorf("resID: got %d, want 107", atom.ResID)
	}
	if atom.X != 10.0 || atom.Y != 10.0 || atom.Z != 10.0 {
		t.Errorf("coords: got (%.1f, %.1f, %.1f), want (10.0, 10.0, 10.0)", atom.X, atom.Y, atom.Z)
	}
	if atom.Element != "N" {
		t.Errorf("element: got %q, want %q", atom.Element, "N")
	}
}

func TestParsePDBQTAtomHETATM(t *testing.T) {
	line := "HETATM    1  C1  LIG A   1      10.500  10.000  10.000  1.00  0.00     0.000 C"
	atom := parsePDBQTAtom(line)
	if atom == nil {
		t.Fatal("parsePDBQTAtom returned nil for valid HETATM line")
	}
	if atom.Element != "C" {
		t.Errorf("element: got %q, want %q", atom.Element, "C")
	}
}

func TestParsePDBQTAtomShortLine(t *testing.T) {
	// Line too short to contain coordinates.
	atom := parsePDBQTAtom("ATOM      1  N   ASP A 107")
	if atom != nil {
		t.Error("expected nil for short line")
	}
}

func TestParsePDBQTAtomNonAtomLine(t *testing.T) {
	atom := parsePDBQTAtom("REMARK   This is a remark line")
	if atom != nil {
		t.Error("expected nil for non-ATOM/HETATM line")
	}
}

func TestParseAllAtoms(t *testing.T) {
	atoms := parseAllAtoms(sampleReceptorPDBQT)
	if len(atoms) != 9 {
		t.Errorf("got %d atoms, want 9", len(atoms))
	}

	// Verify first and last atom.
	if atoms[0].ResName != "ASP" || atoms[0].ResID != 107 {
		t.Errorf("first atom: got %s %d, want ASP 107", atoms[0].ResName, atoms[0].ResID)
	}
	if atoms[8].ResName != "ALA" || atoms[8].ResID != 200 {
		t.Errorf("last atom: got %s %d, want ALA 200", atoms[8].ResName, atoms[8].ResID)
	}
}

func TestParseModel1Atoms(t *testing.T) {
	atoms := parseModel1Atoms(sampleDockedPDBQT)

	// Should get 5 atoms from MODEL 1 only (ROOT/BRANCH/ENDROOT/ENDBRANCH/TORSDOF skipped).
	if len(atoms) != 5 {
		t.Errorf("got %d atoms from MODEL 1, want 5", len(atoms))
		for i, a := range atoms {
			t.Logf("  atom %d: %s %s %d (%.1f, %.1f, %.1f)", i, a.Name, a.ResName, a.ResID, a.X, a.Y, a.Z)
		}
	}

	// None should have coordinates from MODEL 2 (99, 99, 99).
	for _, a := range atoms {
		if a.X > 50 {
			t.Errorf("atom from MODEL 2 leaked through: %+v", a)
		}
	}
}

func TestParseModel1AtomsSingleModel(t *testing.T) {
	// A PDBQT without MODEL records should return all atoms.
	singleModel := `ATOM      1  N   ALA A   1      1.000   2.000   3.000  1.00  0.00     0.000 NA
ATOM      2  CA  ALA A   1      2.000   3.000   4.000  1.00  0.00     0.000 C
`
	atoms := parseModel1Atoms(singleModel)
	if len(atoms) != 2 {
		t.Errorf("got %d atoms, want 2 for single-model PDBQT", len(atoms))
	}
}

func TestClassifyPocket(t *testing.T) {
	receptorAtoms := parseAllAtoms(sampleReceptorPDBQT)
	ligandAtoms := parseModel1Atoms(sampleDockedPDBQT)

	results, _ := classifyPocket(receptorAtoms, ligandAtoms, 5.0)

	// ASP 107 should be in the pocket (ligand atoms are at ~10.x coords).
	foundASP := false
	for _, pr := range results {
		if pr.ResName == "ASP" && pr.ResID == 107 {
			foundASP = true

			// Min distance should be small (< 1.0 Angstrom for overlapping coords).
			if pr.MinDistance > 2.0 {
				t.Errorf("ASP 107 min distance: got %.2f, want < 2.0", pr.MinDistance)
			}

			// Should have contact interaction at minimum.
			hasContact := false
			for _, inter := range pr.Interactions {
				if inter == "contact" {
					hasContact = true
				}
			}
			if !hasContact {
				t.Errorf("ASP 107 missing 'contact' interaction, got %v", pr.Interactions)
			}

			// ASP is charged and ligand has N/O atoms nearby -> should have ionic.
			hasIonic := false
			for _, inter := range pr.Interactions {
				if inter == "ionic" {
					hasIonic = true
				}
			}
			if !hasIonic {
				t.Errorf("ASP 107 should have 'ionic' interaction (charged + ligand N/O nearby), got %v", pr.Interactions)
			}
		}
	}
	if !foundASP {
		t.Error("ASP 107 not found in pocket residues")
	}

	// ALA 200 at (50, 50, 50) should NOT be in the pocket.
	for _, pr := range results {
		if pr.ResName == "ALA" && pr.ResID == 200 {
			t.Error("ALA 200 should not be in the pocket (too far from ligand)")
		}
	}
}

func TestClassifyPocketHBond(t *testing.T) {
	// Create a receptor with a nitrogen at exactly 3.0A from a ligand oxygen.
	receptor := []pdbqtAtom{{
		Serial: 1, Name: "N", ResName: "SER", ChainID: "A", ResID: 50,
		X: 0, Y: 0, Z: 0, Element: "N",
	}}
	ligand := []pdbqtAtom{{
		Serial: 1, Name: "O1", ResName: "LIG", ChainID: "A", ResID: 1,
		X: 3.0, Y: 0, Z: 0, Element: "O",
	}}

	results, _ := classifyPocket(receptor, ligand, 5.0)
	if len(results) != 1 {
		t.Fatalf("got %d residues, want 1", len(results))
	}

	hasHBond := false
	for _, inter := range results[0].Interactions {
		if inter == "hbond" {
			hasHBond = true
		}
	}
	if !hasHBond {
		t.Errorf("expected hbond interaction for N...O at 3.0A, got %v", results[0].Interactions)
	}
}

func TestClassifyPocketHydrophobic(t *testing.T) {
	// C...C at 3.5A should be hydrophobic.
	receptor := []pdbqtAtom{{
		Serial: 1, Name: "CB", ResName: "VAL", ChainID: "A", ResID: 30,
		X: 0, Y: 0, Z: 0, Element: "C",
	}}
	ligand := []pdbqtAtom{{
		Serial: 1, Name: "C1", ResName: "LIG", ChainID: "A", ResID: 1,
		X: 3.5, Y: 0, Z: 0, Element: "C",
	}}

	results, _ := classifyPocket(receptor, ligand, 5.0)
	if len(results) != 1 {
		t.Fatalf("got %d residues, want 1", len(results))
	}

	hasHydro := false
	for _, inter := range results[0].Interactions {
		if inter == "hydrophobic" {
			hasHydro = true
		}
	}
	if !hasHydro {
		t.Errorf("expected hydrophobic interaction for C...C at 3.5A, got %v", results[0].Interactions)
	}
}

func TestClassifyPocketEmpty(t *testing.T) {
	results, _ := classifyPocket(nil, nil, 5.0)
	if len(results) != 0 {
		t.Errorf("expected empty results for nil atoms, got %d", len(results))
	}
}

func TestDistFunction(t *testing.T) {
	a := pdbqtAtom{X: 0, Y: 0, Z: 0}
	b := pdbqtAtom{X: 3, Y: 4, Z: 0}
	d := dist(a, b)
	if math.Abs(d-5.0) > 0.001 {
		t.Errorf("dist: got %.3f, want 5.0", d)
	}
}

func TestSortPocketResidues(t *testing.T) {
	residues := []PocketResidue{
		{ResName: "ALA", ResID: 1, MinDistance: 5.0},
		{ResName: "GLY", ResID: 2, MinDistance: 2.0},
		{ResName: "SER", ResID: 3, MinDistance: 3.5},
	}
	sortPocketResidues(residues)

	if residues[0].ResID != 2 || residues[1].ResID != 3 || residues[2].ResID != 1 {
		t.Errorf("sort order wrong: %v", residues)
	}
}

func TestGuessElementFromName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"N", "N"},
		{"NZ", "N"},
		{"OG", "O"},
		{"CA", "C"},
		{"CB", "C"},
		{"SD", "S"},
		{"H", "H"},
		{"", "C"},
	}
	for _, tt := range tests {
		got := guessElementFromName(tt.name)
		if got != tt.want {
			t.Errorf("guessElementFromName(%q): got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestVinaTypeMapping(t *testing.T) {
	tests := []struct {
		vinaType string
		want     string
	}{
		{"OA", "O"},
		{"NA", "N"},
		{"SA", "S"},
		{"HD", "H"},
		{"A", "C"},
		{"C", "C"},
	}
	for _, tt := range tests {
		got, ok := vinaTypeToElement[tt.vinaType]
		if !ok {
			t.Errorf("vinaTypeToElement missing key %q", tt.vinaType)
			continue
		}
		if got != tt.want {
			t.Errorf("vinaTypeToElement[%q]: got %q, want %q", tt.vinaType, got, tt.want)
		}
	}
}

func TestLEU110InPocket(t *testing.T) {
	// LEU 110 at (20, 20, 20) is > 5A from ligand at (~10, 10, 10) but should
	// be included with a larger cutoff.
	receptorAtoms := parseAllAtoms(sampleReceptorPDBQT)
	ligandAtoms := parseModel1Atoms(sampleDockedPDBQT)

	// With 5A cutoff, LEU should not appear.
	results5, _ := classifyPocket(receptorAtoms, ligandAtoms, 5.0)
	for _, pr := range results5 {
		if pr.ResName == "LEU" && pr.ResID == 110 {
			t.Error("LEU 110 should not be in pocket at 5.0A cutoff")
		}
	}

	// LEU 110 at (20,20,20) vs ligand at (11.2,10,10) = dist ~13.4A.
	// With a 20A cutoff, LEU should appear.
	results20, _ := classifyPocket(receptorAtoms, ligandAtoms, 20.0)
	foundLEU := false
	for _, pr := range results20 {
		if pr.ResName == "LEU" && pr.ResID == 110 {
			foundLEU = true
		}
	}
	if !foundLEU {
		// Compute expected distance to verify test expectations.
		leuAtom := pdbqtAtom{X: 20, Y: 20, Z: 20}
		ligAtom := pdbqtAtom{X: 11.2, Y: 10, Z: 10}
		d := dist(leuAtom, ligAtom)
		t.Errorf("LEU 110 not found in pocket at 20A cutoff (expected dist ~%.1fA)", d)
	}
}

func TestParsePDBQTAtomOxygenTypes(t *testing.T) {
	// Test OA (oxygen acceptor) mapping.
	line := "ATOM      4  O   ASP A 107      10.500  10.500  10.000  1.00  0.00     0.000 OA"
	atom := parsePDBQTAtom(line)
	if atom == nil {
		t.Fatal("parsePDBQTAtom returned nil")
	}
	if atom.Element != "O" {
		t.Errorf("element: got %q, want %q for OA type", atom.Element, "O")
	}
}

func TestResponseFieldCompleteness(t *testing.T) {
	// Verify the response struct has all expected JSON fields by checking
	// that a PocketAnalysisResponse can be marshalled and re-parsed.
	resp := PocketAnalysisResponse{
		CompoundID:     "CHEMBL123",
		CutoffAngstrom: 5.0,
		PocketResidues: []PocketResidue{
			{
				ChainID:      "A",
				ResID:        107,
				ResName:      "ASP",
				MinDistance:   2.8,
				Interactions: []string{"hbond", "ionic", "contact"},
				ContactAtoms: 5,
			},
		},
		TotalContacts: 5,
		LigandAtoms:   31,
	}

	// Verify the JSON roundtrip contains expected keys.
	data, err := jsonMarshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	s := string(data)
	for _, key := range []string{"compound_id", "cutoff_angstrom", "pocket_residues", "total_contacts", "ligand_atoms"} {
		if !strings.Contains(s, key) {
			t.Errorf("JSON missing key %q: %s", key, s)
		}
	}
	for _, key := range []string{"chain_id", "res_id", "res_name", "min_distance", "interactions", "contact_atoms"} {
		if !strings.Contains(s, key) {
			t.Errorf("JSON missing residue key %q: %s", key, s)
		}
	}
}

// jsonMarshal is a simple test helper to avoid importing encoding/json
// in the test assertion (it is already imported in the source).
func jsonMarshal(v interface{}) ([]byte, error) {
	// The encoding/json package is imported in handlers_pocket.go.
	// We can use it directly here since this is the same package.
	return json.Marshal(v)
}
