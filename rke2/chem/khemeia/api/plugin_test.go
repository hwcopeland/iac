package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadPlugins(t *testing.T) {
	// Use the actual plugins directory relative to the project root.
	pluginsDir := filepath.Join("..", "plugins")
	if _, err := os.Stat(pluginsDir); os.IsNotExist(err) {
		t.Skipf("plugins directory %s not found, skipping", pluginsDir)
	}

	plugins, err := LoadPlugins(pluginsDir)
	if err != nil {
		t.Fatalf("LoadPlugins failed: %v", err)
	}

	if len(plugins) < 2 {
		t.Errorf("expected at least 2 plugins, got %d", len(plugins))
	}

	// Verify QE plugin loaded correctly.
	var qe *Plugin
	var docking *Plugin
	for i := range plugins {
		if plugins[i].Slug == "qe" {
			qe = &plugins[i]
		}
		if plugins[i].Slug == "docking" {
			docking = &plugins[i]
		}
	}

	if qe == nil {
		t.Fatal("QE plugin not found")
	}
	if qe.Name != "quantum-espresso" {
		t.Errorf("QE name: expected %q, got %q", "quantum-espresso", qe.Name)
	}
	if qe.Image != "costrouc/quantum-espresso:latest" {
		t.Errorf("QE image: expected %q, got %q", "costrouc/quantum-espresso:latest", qe.Image)
	}
	if qe.Type != "job" {
		t.Errorf("QE type: expected %q, got %q", "job", qe.Type)
	}
	if qe.Database != "qe" {
		t.Errorf("QE database: expected %q, got %q", "qe", qe.Database)
	}
	if len(qe.Input) != 4 {
		t.Errorf("QE inputs: expected 4, got %d", len(qe.Input))
	}
	if len(qe.Output) != 4 {
		t.Errorf("QE outputs: expected 4, got %d", len(qe.Output))
	}
	if qe.Command == "" {
		t.Error("QE command should not be empty")
	}

	if docking == nil {
		t.Fatal("docking plugin not found")
	}
	if docking.Name != "autodock-vina" {
		t.Errorf("docking name: expected %q, got %q", "autodock-vina", docking.Name)
	}
	if docking.Database != "docking" {
		t.Errorf("docking database: expected %q, got %q", "docking", docking.Database)
	}
}

func TestLoadPluginsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	plugins, err := LoadPlugins(dir)
	if err != nil {
		t.Fatalf("LoadPlugins on empty dir failed: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestLoadPluginsMissingDir(t *testing.T) {
	plugins, err := LoadPlugins("/nonexistent/path")
	if err != nil {
		t.Errorf("expected nil error for missing dir, got: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestLoadPluginsBadYAML(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("not: [valid: yaml"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadPlugins(dir)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadPluginsMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	// Plugin with missing slug.
	err := os.WriteFile(filepath.Join(dir, "incomplete.yaml"), []byte(`
name: incomplete
image: test:latest
type: job
database: test
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadPlugins(dir)
	if err == nil {
		t.Error("expected error for plugin missing slug")
	}
}

func TestPluginTableName(t *testing.T) {
	p := Plugin{Slug: "qe"}
	if p.TableName() != "qe_jobs" {
		t.Errorf("expected %q, got %q", "qe_jobs", p.TableName())
	}

	p2 := Plugin{Slug: "docking"}
	if p2.TableName() != "docking_jobs" {
		t.Errorf("expected %q, got %q", "docking_jobs", p2.TableName())
	}
}

func TestPluginGenerateTableDDL(t *testing.T) {
	p := Plugin{Slug: "qe"}
	ddl := p.GenerateTableDDL()

	if ddl == "" {
		t.Error("expected non-empty DDL")
	}

	// Verify it contains key components.
	mustContain := []string{
		"CREATE TABLE IF NOT EXISTS qe_jobs",
		"id            INT AUTO_INCREMENT PRIMARY KEY",
		"name          VARCHAR(255) NOT NULL UNIQUE",
		"status        ENUM",
		"input_data    JSON",
		"output_data   JSON",
		"error_output  MEDIUMTEXT",
		"submitted_by  VARCHAR(255)",
	}
	for _, s := range mustContain {
		if !contains(ddl, s) {
			t.Errorf("DDL missing %q", s)
		}
	}
}

func TestPluginTimeoutDuration(t *testing.T) {
	tests := []struct {
		timeout string
		want    time.Duration
	}{
		{"4h", 4 * time.Hour},
		{"1h", 1 * time.Hour},
		{"30m", 30 * time.Minute},
		{"1h30m", 90 * time.Minute},
		{"", 4 * time.Hour},        // default for empty
		{"invalid", 4 * time.Hour}, // default for invalid
	}

	for _, tt := range tests {
		p := Plugin{Resources: PluginResources{Timeout: tt.timeout}}
		got := p.TimeoutDuration()
		if got != tt.want {
			t.Errorf("TimeoutDuration(%q) = %v, want %v", tt.timeout, got, tt.want)
		}
	}
}

func TestPluginValidateInput(t *testing.T) {
	p := Plugin{
		Input: []PluginInput{
			{Name: "input_file", Type: "text", Required: true},
			{Name: "executable", Type: "string", Default: "pw.x"},
			{Name: "num_cpus", Type: "int", Default: 1, Max: 20},
			{Name: "memory_mb", Type: "int", Default: 2048, Max: 32768},
		},
	}

	// Valid input.
	err := p.ValidateInput(map[string]interface{}{
		"input_file": "test content",
		"num_cpus":   float64(4),
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Missing required field.
	err = p.ValidateInput(map[string]interface{}{
		"num_cpus": float64(4),
	})
	if err == nil {
		t.Error("expected error for missing required field")
	}

	// Empty text field that is required.
	err = p.ValidateInput(map[string]interface{}{
		"input_file": "   ",
	})
	if err == nil {
		t.Error("expected error for empty required text field")
	}

	// Exceeds max value.
	err = p.ValidateInput(map[string]interface{}{
		"input_file": "test",
		"num_cpus":   float64(25),
	})
	if err == nil {
		t.Error("expected error for exceeding max value")
	}

	// Wrong type.
	err = p.ValidateInput(map[string]interface{}{
		"input_file": "test",
		"num_cpus":   "not a number",
	})
	if err == nil {
		t.Error("expected error for wrong type")
	}
}

func TestPluginApplyDefaults(t *testing.T) {
	p := Plugin{
		Input: []PluginInput{
			{Name: "input_file", Type: "text", Required: true},
			{Name: "executable", Type: "string", Default: "pw.x"},
			{Name: "num_cpus", Type: "int", Default: 1},
		},
	}

	data := map[string]interface{}{
		"input_file": "test",
	}
	p.ApplyDefaults(data)

	if data["executable"] != "pw.x" {
		t.Errorf("expected default executable %q, got %v", "pw.x", data["executable"])
	}
	if data["num_cpus"] != 1 {
		t.Errorf("expected default num_cpus %d, got %v", 1, data["num_cpus"])
	}

	// Should not override existing values.
	data2 := map[string]interface{}{
		"input_file": "test",
		"executable": "cp.x",
	}
	p.ApplyDefaults(data2)

	if data2["executable"] != "cp.x" {
		t.Errorf("expected existing value %q, got %v", "cp.x", data2["executable"])
	}
}

func TestPluginExpandCommand(t *testing.T) {
	p := Plugin{
		Command: `if [ {{ .num_cpus }} -gt 1 ]; then mpirun -np {{ .num_cpus }} {{ .executable }} -in input.in; fi`,
	}

	data := map[string]interface{}{
		"num_cpus":   4,
		"executable": "pw.x",
	}

	expanded := p.ExpandCommand(data)
	expected := `if [ 4 -gt 1 ]; then mpirun -np 4 pw.x -in input.in; fi`
	if expanded != expected {
		t.Errorf("expected %q, got %q", expected, expanded)
	}
}

func TestPluginExpandResource(t *testing.T) {
	p := Plugin{}

	result := p.ExpandResource("{{ .num_cpus }}", map[string]interface{}{"num_cpus": 4})
	if result != "4" {
		t.Errorf("expected %q, got %q", "4", result)
	}

	result2 := p.ExpandResource("{{ .memory_mb }}Mi", map[string]interface{}{"memory_mb": 2048})
	if result2 != "2048Mi" {
		t.Errorf("expected %q, got %q", "2048Mi", result2)
	}

	// Static resource (no template).
	result3 := p.ExpandResource("1", map[string]interface{}{})
	if result3 != "1" {
		t.Errorf("expected %q, got %q", "1", result3)
	}
}

func TestPluginParseOutput(t *testing.T) {
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:  "total_energy",
				Type:  "float",
				Parse: `!\s+total\s+energy\s+=\s+([-\d.]+)\s+Ry`,
			},
			{
				Name:  "wall_time_sec",
				Type:  "float",
				Parse: `PWSCF\s+:\s+[\d.]+s CPU\s+([\d.]+)s WALL`,
			},
			{
				Name: "output_file",
				Type: "text",
				// No parse regex — captures full text.
			},
		},
	}

	output := `
     Program PWSCF v.7.0 starts on  1Jan2025 at  0: 0: 0

     Self-consistent Calculation

!    total   energy              =     -32.44928392 Ry

     PWSCF        :     12.34s CPU     13.56s WALL
`

	result := p.ParseOutput(output)

	if result["total_energy"] != "-32.44928392" {
		t.Errorf("total_energy: expected %q, got %v", "-32.44928392", result["total_energy"])
	}
	if result["wall_time_sec"] != "13.56" {
		t.Errorf("wall_time_sec: expected %q, got %v", "13.56", result["wall_time_sec"])
	}
}

func TestPluginParseOutputMultipleMatches(t *testing.T) {
	// Should take the last match (final SCF iteration).
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:  "total_energy",
				Type:  "float",
				Parse: `!\s+total\s+energy\s+=\s+([-\d.]+)\s+Ry`,
			},
		},
	}

	output := `
!    total   energy              =     -30.00000000 Ry
!    total   energy              =     -32.44928392 Ry
`

	result := p.ParseOutput(output)
	if result["total_energy"] != "-32.44928392" {
		t.Errorf("expected last match %q, got %v", "-32.44928392", result["total_energy"])
	}
}

func TestPluginParseOutputReduceMin(t *testing.T) {
	// Docking output: multiple affinity values, reduce: min should pick the
	// most negative (best binding affinity).
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:   "best_affinity",
				Type:   "float",
				Parse:  `affinity=([-\d.]+)`,
				Reduce: "min",
			},
		},
	}

	output := `
affinity=-6.2
affinity=-8.9
affinity=-7.1
affinity=-5.4
`

	result := p.ParseOutput(output)
	if result["best_affinity"] != "-8.9" {
		t.Errorf("best_affinity: expected %q, got %v", "-8.9", result["best_affinity"])
	}
}

func TestPluginParseOutputReduceMax(t *testing.T) {
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:   "worst_affinity",
				Type:   "float",
				Parse:  `affinity=([-\d.]+)`,
				Reduce: "max",
			},
		},
	}

	output := `
affinity=-6.2
affinity=-8.9
affinity=-7.1
affinity=-5.4
`

	result := p.ParseOutput(output)
	if result["worst_affinity"] != "-5.4" {
		t.Errorf("worst_affinity: expected %q, got %v", "-5.4", result["worst_affinity"])
	}
}

func TestPluginParseOutputReduceSingleMatch(t *testing.T) {
	// With reduce: min and only one match, that match should be returned.
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:   "best_affinity",
				Type:   "float",
				Parse:  `affinity=([-\d.]+)`,
				Reduce: "min",
			},
		},
	}

	result := p.ParseOutput("affinity=-7.3")
	if result["best_affinity"] != "-7.3" {
		t.Errorf("best_affinity: expected %q, got %v", "-7.3", result["best_affinity"])
	}
}

func TestPluginParseOutputReduceDefaultIsLast(t *testing.T) {
	// No reduce field: should use last match (backward compat).
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:  "value",
				Type:  "float",
				Parse: `val=([-\d.]+)`,
			},
		},
	}

	output := "val=-10.0\nval=-3.0\nval=-5.0\n"
	result := p.ParseOutput(output)
	// Last match is -5.0, not the min (-10.0).
	if result["value"] != "-5.0" {
		t.Errorf("value: expected %q (last match), got %v", "-5.0", result["value"])
	}
}

func TestPluginParseOutputNoMatch(t *testing.T) {
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:  "total_energy",
				Type:  "float",
				Parse: `!\s+total\s+energy\s+=\s+([-\d.]+)\s+Ry`,
			},
		},
	}

	result := p.ParseOutput("no energy here")
	if _, exists := result["total_energy"]; exists {
		t.Error("expected no match for total_energy")
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input interface{}
		want  float64
		ok    bool
	}{
		{float64(42.5), 42.5, true},
		{float32(42.5), 42.5, true},
		{int(42), 42.0, true},
		{int64(42), 42.0, true},
		{int32(42), 42.0, true},
		{"not a number", 0, false},
		{true, 0, false},
		{nil, 0, false},
	}

	for _, tt := range tests {
		got, ok := toFloat64(tt.input)
		if ok != tt.ok {
			t.Errorf("toFloat64(%v): ok = %v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("toFloat64(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestPluginInputTypes(t *testing.T) {
	p := Plugin{
		Input: []PluginInput{
			{Name: "text_field", Type: "text"},
			{Name: "str_field", Type: "string"},
			{Name: "int_field", Type: "int"},
			{Name: "float_field", Type: "float"},
		},
	}

	// All valid types.
	err := p.ValidateInput(map[string]interface{}{
		"text_field":  "some text",
		"str_field":   "a string",
		"int_field":   float64(42),
		"float_field": float64(3.14),
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// String field with wrong type.
	err = p.ValidateInput(map[string]interface{}{
		"str_field": float64(42),
	})
	if err == nil {
		t.Error("expected error for string field with number value")
	}

	// Float field with wrong type.
	err = p.ValidateInput(map[string]interface{}{
		"float_field": "not a number",
	})
	if err == nil {
		t.Error("expected error for float field with string value")
	}
}

func TestPluginParseOutputReduceAll(t *testing.T) {
	// reduce: all should collect every match as a []string.
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:   "scf_energies",
				Type:   "float",
				Parse:  `total energy\s+=\s+([-\d.]+)\s+Ry`,
				Reduce: "all",
			},
		},
	}

	output := `
     total energy          =     -30.00000000 Ry
     total energy          =     -31.50000000 Ry
!    total   energy        =     -32.44928392 Ry
     total energy          =     -32.44928392 Ry
`

	result := p.ParseOutput(output)
	values, ok := result["scf_energies"].([]string)
	if !ok {
		t.Fatalf("scf_energies: expected []string, got %T", result["scf_energies"])
	}
	if len(values) != 3 {
		t.Fatalf("scf_energies: expected 3 values, got %d: %v", len(values), values)
	}

	expected := []string{"-30.00000000", "-31.50000000", "-32.44928392"}
	for i, want := range expected {
		if values[i] != want {
			t.Errorf("scf_energies[%d]: expected %q, got %q", i, want, values[i])
		}
	}
}

func TestPluginParseOutputReduceAllSingleMatch(t *testing.T) {
	// reduce: all with a single match should still produce a []string with one element.
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:   "scf_energies",
				Type:   "float",
				Parse:  `energy=([-\d.]+)`,
				Reduce: "all",
			},
		},
	}

	result := p.ParseOutput("energy=-42.5")
	values, ok := result["scf_energies"].([]string)
	if !ok {
		t.Fatalf("scf_energies: expected []string, got %T", result["scf_energies"])
	}
	if len(values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(values))
	}
	if values[0] != "-42.5" {
		t.Errorf("expected %q, got %q", "-42.5", values[0])
	}
}

func TestPluginParseOutputReduceAllNoMatch(t *testing.T) {
	// reduce: all with no matches should not set the field.
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:   "scf_energies",
				Type:   "float",
				Parse:  `energy=([-\d.]+)`,
				Reduce: "all",
			},
		},
	}

	result := p.ParseOutput("nothing here")
	if _, exists := result["scf_energies"]; exists {
		t.Error("expected no scf_energies entry when there are no matches")
	}
}

func TestPluginParseOutputReduceAllPsi4SCF(t *testing.T) {
	// Verify the Psi4 scf_energies regex against realistic Psi4 output.
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:   "scf_energies",
				Type:   "float",
				Parse:  `@R(?:HF|KS|OHF|UHF|UKS) iter\s+\d+:\s+([-\d.]+)`,
				Reduce: "all",
			},
		},
	}

	output := `
   ==> Iterations <==

                        Total Energy        Delta E     RMS |[F,P]|

   @RHF iter   1:   -75.98012345   -7.59801e+01   6.34e-02
   @RHF iter   2:   -76.00123456   -2.11111e-02   7.89e-03
   @RHF iter   3:   -76.01060209   -9.36753e-03   1.23e-03
   @RHF iter   4:   -76.01090000   -2.97910e-04   2.45e-04

   ==> Post-Iterations <==
`

	result := p.ParseOutput(output)
	values, ok := result["scf_energies"].([]string)
	if !ok {
		t.Fatalf("scf_energies: expected []string, got %T", result["scf_energies"])
	}
	if len(values) != 4 {
		t.Fatalf("expected 4 values, got %d: %v", len(values), values)
	}
	if values[0] != "-75.98012345" {
		t.Errorf("first iter: expected %q, got %q", "-75.98012345", values[0])
	}
	if values[3] != "-76.01090000" {
		t.Errorf("last iter: expected %q, got %q", "-76.01090000", values[3])
	}
}

func TestPluginParseOutputReduceAllNWChemSCF(t *testing.T) {
	// Verify the NWChem scf_energies regex against realistic NWChem output.
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:   "scf_energies",
				Type:   "float",
				Parse:  `Total (?:DFT|SCF) energy\s+=\s+([-\d.]+)`,
				Reduce: "all",
			},
		},
	}

	// NWChem geometry optimization: multiple SCF cycles, each ending with
	// a "Total DFT energy" line.
	output := `
         Total DFT energy =      -76.010602091174
      ...geometry step 2...
         Total DFT energy =      -76.012345678901
      ...geometry step 3...
         Total DFT energy =      -76.013000000000
`

	result := p.ParseOutput(output)
	values, ok := result["scf_energies"].([]string)
	if !ok {
		t.Fatalf("scf_energies: expected []string, got %T", result["scf_energies"])
	}
	if len(values) != 3 {
		t.Fatalf("expected 3 values, got %d: %v", len(values), values)
	}
	if values[0] != "-76.010602091174" {
		t.Errorf("first cycle: expected %q, got %q", "-76.010602091174", values[0])
	}
	if values[2] != "-76.013000000000" {
		t.Errorf("last cycle: expected %q, got %q", "-76.013000000000", values[2])
	}
}

func TestPluginParseOutputReduceAllDFRATOM(t *testing.T) {
	// Verify the DFRATOM scf_energies regex against realistic DFRATOM output.
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:   "scf_energies",
				Type:   "float",
				Parse:  `TOTAL ENERGY = ([-\d.E+-]+)`,
				Reduce: "all",
			},
		},
	}

	output := `
 SCF ITERATION NO   1
 NOTE - TOTAL ENERGY = -1.2345E+02
 SCF ITERATION NO   2
 NOTE - TOTAL ENERGY = -1.2350E+02
 SCF ITERATION NO   3
 NOTE - TOTAL ENERGY = -1.2351E+02
`

	result := p.ParseOutput(output)
	values, ok := result["scf_energies"].([]string)
	if !ok {
		t.Fatalf("scf_energies: expected []string, got %T", result["scf_energies"])
	}
	if len(values) != 3 {
		t.Fatalf("expected 3 values, got %d: %v", len(values), values)
	}
	if values[0] != "-1.2345E+02" {
		t.Errorf("first iter: expected %q, got %q", "-1.2345E+02", values[0])
	}
	if values[2] != "-1.2351E+02" {
		t.Errorf("last iter: expected %q, got %q", "-1.2351E+02", values[2])
	}
}

func TestPluginParseOutputDFRATOMEnergyDecomposition(t *testing.T) {
	// Verify the DFRATOM energy decomposition regexes parse the ENERGIES section
	// correctly. The output has been through sed D->E conversion.
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:  "total_energy",
				Type:  "float",
				Parse: `TOTAL\s+(-?\d+\.\d+E[+-]\d+)`,
			},
			{
				Name:  "kinetic_energy",
				Type:  "float",
				Parse: `KINETIC <T>\s*([-\d.E+-]+)`,
			},
			{
				Name:  "potential_energy",
				Type:  "float",
				Parse: `POTENTIAL <V>\s*([-\d.E+-]+)`,
			},
			{
				Name:  "virial_ratio",
				Type:  "float",
				Parse: `VIRIAL\s+.*\s+([-\d.E+-]+)`,
			},
			{
				Name:  "rest_mass_energy",
				Type:  "float",
				Parse: `REST MASS\s*([-\d.E+-]+)`,
			},
		},
	}

	// Simulate DFRATOM output after sed D->E conversion.
	// Fortran FORMAT: ' TOTAL      ',D26.14/' REST MASS  ',D26.14/
	//                 ' KINETIC <T>',D26.14/' POTENTIAL <V>',D24.14
	// VIRIAL <V>/<T> with D24.14
	output := `
ENERGIES
 TOTAL       -3.76760357418128E+01
 REST MASS   -3.76760432775822E+01
 KINETIC <T>  7.53848058503390E+01
 POTENTIAL <V>-7.53847983145696E+01

VIRIAL <V>/<T> -1.99999990017786E+00

SCF ITERATION NO    42
`

	result := p.ParseOutput(output)

	if result["total_energy"] != "-3.76760357418128E+01" {
		t.Errorf("total_energy: expected %q, got %v", "-3.76760357418128E+01", result["total_energy"])
	}
	if result["kinetic_energy"] != "7.53848058503390E+01" {
		t.Errorf("kinetic_energy: expected %q, got %v", "7.53848058503390E+01", result["kinetic_energy"])
	}
	if result["potential_energy"] != "-7.53847983145696E+01" {
		t.Errorf("potential_energy: expected %q, got %v", "-7.53847983145696E+01", result["potential_energy"])
	}
	if result["virial_ratio"] != "-1.99999990017786E+00" {
		t.Errorf("virial_ratio: expected %q, got %v", "-1.99999990017786E+00", result["virial_ratio"])
	}
	if result["rest_mass_energy"] != "-3.76760432775822E+01" {
		t.Errorf("rest_mass_energy: expected %q, got %v", "-3.76760432775822E+01", result["rest_mass_energy"])
	}
}

func TestPluginParseOutputDFRATOMVirialFiniteNucleus(t *testing.T) {
	// The virial regex should also match the finite-sphere nucleus variant.
	p := Plugin{
		Output: []PluginOutput{
			{
				Name:  "virial_ratio",
				Type:  "float",
				Parse: `VIRIAL\s+.*\s+([-\d.E+-]+)`,
			},
		},
	}

	output := `VIRIAL (<V>+CORRECTION)/<T> -1.99999850012345E+00`

	result := p.ParseOutput(output)
	if result["virial_ratio"] != "-1.99999850012345E+00" {
		t.Errorf("virial_ratio (finite nucleus): expected %q, got %v",
			"-1.99999850012345E+00", result["virial_ratio"])
	}
}

// --- Artifact extraction and content type tests ---

func TestExtractArtifactsSingleFile(t *testing.T) {
	content := []byte("hello world")
	b64 := base64.StdEncoding.EncodeToString(content)

	logOutput := "some log line\n===ARTIFACT:output.cube===\n" + b64 + "\n===END_ARTIFACT===\nmore log\n"

	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	got, ok := artifacts["output.cube"]
	if !ok {
		t.Fatal("artifact output.cube not found")
	}
	if string(got) != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", string(got))
	}
}

func TestExtractArtifactsMultipleFiles(t *testing.T) {
	cube := base64.StdEncoding.EncodeToString([]byte("cube data"))
	molden := base64.StdEncoding.EncodeToString([]byte("molden data"))

	logOutput := "header\n" +
		"===ARTIFACT:density.cube===\n" + cube + "\n===END_ARTIFACT===\n" +
		"middle log\n" +
		"===ARTIFACT:orbitals.molden===\n" + molden + "\n===END_ARTIFACT===\n" +
		"footer\n"

	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}
	if string(artifacts["density.cube"]) != "cube data" {
		t.Errorf("density.cube: expected %q, got %q", "cube data", string(artifacts["density.cube"]))
	}
	if string(artifacts["orbitals.molden"]) != "molden data" {
		t.Errorf("orbitals.molden: expected %q, got %q", "molden data", string(artifacts["orbitals.molden"]))
	}
}

func TestExtractArtifactsNoArtifacts(t *testing.T) {
	logOutput := "just regular log output\nno markers here\n"
	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(artifacts))
	}
}

func TestExtractArtifactsMissingEndMarker(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("truncated"))
	logOutput := "===ARTIFACT:broken.dat===\n" + content + "\n"

	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for missing end marker, got %d", len(artifacts))
	}
}

func TestExtractArtifactsInvalidBase64(t *testing.T) {
	logOutput := "===ARTIFACT:bad.dat===\nnot-valid-base64!!!\n===END_ARTIFACT===\n"
	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for invalid base64, got %d", len(artifacts))
	}
}

func TestExtractArtifactsEmptyFilename(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("data"))
	logOutput := "===ARTIFACT:===\n" + content + "\n===END_ARTIFACT===\n"
	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for empty filename, got %d", len(artifacts))
	}
}

func TestExtractArtifactsMultiLineBase64(t *testing.T) {
	// Large content gets split across multiple lines by base64 encoders.
	data := make([]byte, 200)
	for i := range data {
		data[i] = byte(i % 256)
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	// Split into 76-char lines (standard base64 line length).
	var b64Lines []string
	for len(b64) > 76 {
		b64Lines = append(b64Lines, b64[:76])
		b64 = b64[76:]
	}
	if len(b64) > 0 {
		b64Lines = append(b64Lines, b64)
	}

	logOutput := "===ARTIFACT:big.dat===\n"
	for _, line := range b64Lines {
		logOutput += line + "\n"
	}
	logOutput += "===END_ARTIFACT===\n"

	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	got := artifacts["big.dat"]
	if len(got) != len(data) {
		t.Errorf("expected %d bytes, got %d", len(data), len(got))
	}
	for i := range data {
		if got[i] != data[i] {
			t.Errorf("byte %d: expected %d, got %d", i, data[i], got[i])
			break
		}
	}
}

func TestStripArtifactBlocks(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("data"))
	input := "line1\nline2\n===ARTIFACT:file.cube===\n" + content + "\n===END_ARTIFACT===\nline3\n"
	got := stripArtifactBlocks(input)
	if contains(got, "===ARTIFACT") {
		t.Error("stripped output still contains ARTIFACT marker")
	}
	if contains(got, content) {
		t.Error("stripped output still contains base64 content")
	}
	if !contains(got, "line1") || !contains(got, "line2") || !contains(got, "line3") {
		t.Errorf("stripped output missing non-artifact lines: %q", got)
	}
}

func TestStripArtifactBlocksNoArtifacts(t *testing.T) {
	input := "line1\nline2\nline3\n"
	got := stripArtifactBlocks(input)
	if got != input {
		t.Errorf("expected unchanged output, got %q", got)
	}
}

func TestStripArtifactBlocksMultiple(t *testing.T) {
	b641 := base64.StdEncoding.EncodeToString([]byte("a"))
	b642 := base64.StdEncoding.EncodeToString([]byte("b"))

	input := "before\n" +
		"===ARTIFACT:a.dat===\n" + b641 + "\n===END_ARTIFACT===\n" +
		"middle\n" +
		"===ARTIFACT:b.dat===\n" + b642 + "\n===END_ARTIFACT===\n" +
		"after\n"

	got := stripArtifactBlocks(input)
	if contains(got, "===ARTIFACT") {
		t.Error("stripped output still contains ARTIFACT markers")
	}
	if !contains(got, "before") || !contains(got, "middle") || !contains(got, "after") {
		t.Errorf("stripped output missing non-artifact lines: %q", got)
	}
}

func TestGuessContentType(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"density.cube", "chemical/x-cube"},
		{"orbitals.molden", "chemical/x-molden"},
		{"docked.pdbqt", "chemical/x-pdbqt"},
		{"output.json", "application/json"},
		{"dos.dat", "text/plain"},
		{"freq.hess", "application/octet-stream"},
		{"unknown.xyz", "application/octet-stream"},
		{"no_extension", "application/octet-stream"},
		// Case insensitivity.
		{"DENSITY.CUBE", "chemical/x-cube"},
		{"output.JSON", "application/json"},
	}

	for _, tt := range tests {
		got := guessContentType(tt.filename)
		if got != tt.want {
			t.Errorf("guessContentType(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestLoadPluginsArtifacts(t *testing.T) {
	// Verify that artifact definitions are loaded from the plugin YAMLs.
	pluginsDir := filepath.Join("..", "plugins")
	if _, err := os.Stat(pluginsDir); os.IsNotExist(err) {
		t.Skipf("plugins directory %s not found, skipping", pluginsDir)
	}

	plugins, err := LoadPlugins(pluginsDir)
	if err != nil {
		t.Fatalf("LoadPlugins failed: %v", err)
	}

	// Build a lookup map.
	bySlug := make(map[string]*Plugin)
	for i := range plugins {
		bySlug[plugins[i].Slug] = &plugins[i]
	}

	// QE should have cube and dat artifacts.
	qe := bySlug["qe"]
	if qe == nil {
		t.Fatal("QE plugin not found")
	}
	if len(qe.Artifacts) != 2 {
		t.Errorf("QE artifacts: expected 2, got %d", len(qe.Artifacts))
	}

	// Psi4 should have cube, molden, and json artifacts.
	psi4 := bySlug["psi4"]
	if psi4 == nil {
		t.Fatal("Psi4 plugin not found")
	}
	if len(psi4.Artifacts) != 3 {
		t.Errorf("Psi4 artifacts: expected 3, got %d", len(psi4.Artifacts))
	}

	// NWChem should have molden and hess artifacts.
	nwchem := bySlug["nwchem"]
	if nwchem == nil {
		t.Fatal("NWChem plugin not found")
	}
	if len(nwchem.Artifacts) != 2 {
		t.Errorf("NWChem artifacts: expected 2, got %d", len(nwchem.Artifacts))
	}

	// DFRATOM should have dat artifacts.
	dfratom := bySlug["dfratom"]
	if dfratom == nil {
		t.Fatal("DFRATOM plugin not found")
	}
	if len(dfratom.Artifacts) != 1 {
		t.Errorf("DFRATOM artifacts: expected 1, got %d", len(dfratom.Artifacts))
	}

	// Docking should have pdbqt artifacts.
	docking := bySlug["docking"]
	if docking == nil {
		t.Fatal("docking plugin not found")
	}
	if len(docking.Artifacts) != 1 {
		t.Errorf("docking artifacts: expected 1, got %d", len(docking.Artifacts))
	}
}

// --- Additional extractArtifacts edge cases ---

func TestExtractArtifactsMalformedStartMarker(t *testing.T) {
	// Start marker missing trailing "===" should be skipped.
	content := base64.StdEncoding.EncodeToString([]byte("data"))
	logOutput := "===ARTIFACT:broken.dat\n" + content + "\n===END_ARTIFACT===\n"

	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for malformed start marker, got %d", len(artifacts))
	}
}

func TestExtractArtifactsPartialPrefix(t *testing.T) {
	// A line that starts with "===ARTIFACT" but has no colon should not match.
	logOutput := "===ARTIFACT===\nsome data\n===END_ARTIFACT===\n"

	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for partial prefix, got %d", len(artifacts))
	}
}

func TestExtractArtifactsEmptyContent(t *testing.T) {
	// Artifact block with no content lines between start and end markers.
	// base64 decode of empty string produces empty []byte.
	logOutput := "===ARTIFACT:empty.dat===\n===END_ARTIFACT===\n"

	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	got := artifacts["empty.dat"]
	if len(got) != 0 {
		t.Errorf("expected empty content, got %d bytes", len(got))
	}
}

func TestExtractArtifactsWhitespaceOnlyContent(t *testing.T) {
	// Artifact block where all content lines are whitespace. After trimming
	// and joining, this is an empty string which is valid empty base64.
	logOutput := "===ARTIFACT:ws.dat===\n   \n  \n===END_ARTIFACT===\n"

	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	got := artifacts["ws.dat"]
	if len(got) != 0 {
		t.Errorf("expected empty content from whitespace-only lines, got %d bytes", len(got))
	}
}

func TestExtractArtifactsConsecutiveBlocks(t *testing.T) {
	// Two artifact blocks back to back with no separating log line.
	a := base64.StdEncoding.EncodeToString([]byte("first"))
	b := base64.StdEncoding.EncodeToString([]byte("second"))

	logOutput := "===ARTIFACT:a.dat===\n" + a + "\n===END_ARTIFACT===\n" +
		"===ARTIFACT:b.dat===\n" + b + "\n===END_ARTIFACT===\n"

	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}
	if string(artifacts["a.dat"]) != "first" {
		t.Errorf("a.dat: expected %q, got %q", "first", string(artifacts["a.dat"]))
	}
	if string(artifacts["b.dat"]) != "second" {
		t.Errorf("b.dat: expected %q, got %q", "second", string(artifacts["b.dat"]))
	}
}

func TestExtractArtifactsDuplicateFilename(t *testing.T) {
	// Two blocks with the same filename. The second should overwrite the first.
	a := base64.StdEncoding.EncodeToString([]byte("old"))
	b := base64.StdEncoding.EncodeToString([]byte("new"))

	logOutput := "===ARTIFACT:dup.dat===\n" + a + "\n===END_ARTIFACT===\n" +
		"===ARTIFACT:dup.dat===\n" + b + "\n===END_ARTIFACT===\n"

	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact (deduped), got %d", len(artifacts))
	}
	if string(artifacts["dup.dat"]) != "new" {
		t.Errorf("dup.dat: expected %q (second value), got %q", "new", string(artifacts["dup.dat"]))
	}
}

func TestExtractArtifactsWithLeadingWhitespace(t *testing.T) {
	// Artifact markers may have leading whitespace in log output (indentation).
	content := base64.StdEncoding.EncodeToString([]byte("indented"))
	logOutput := "  ===ARTIFACT:indented.dat===\n  " + content + "\n  ===END_ARTIFACT===\n"

	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact with leading whitespace, got %d", len(artifacts))
	}
	if string(artifacts["indented.dat"]) != "indented" {
		t.Errorf("indented.dat: expected %q, got %q", "indented", string(artifacts["indented.dat"]))
	}
}

func TestExtractArtifactsFilenameWithPath(t *testing.T) {
	// Filename containing path separators should be preserved as-is.
	content := base64.StdEncoding.EncodeToString([]byte("nested"))
	logOutput := "===ARTIFACT:output/results/density.cube===\n" + content + "\n===END_ARTIFACT===\n"

	artifacts := extractArtifacts(logOutput)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if _, ok := artifacts["output/results/density.cube"]; !ok {
		t.Error("artifact with path filename not found")
	}
}

// --- Additional stripArtifactBlocks edge cases ---

func TestStripArtifactBlocksMissingEndMarker(t *testing.T) {
	// When end marker is missing, strip should consume from start marker through EOF.
	content := base64.StdEncoding.EncodeToString([]byte("data"))
	input := "before\n===ARTIFACT:orphan.dat===\n" + content + "\ntrailing line\n"

	got := stripArtifactBlocks(input)
	if contains(got, "===ARTIFACT") {
		t.Error("stripped output still contains ARTIFACT marker")
	}
	if contains(got, content) {
		t.Error("stripped output still contains base64 content")
	}
	if !contains(got, "before") {
		t.Error("stripped output missing 'before' line")
	}
	// "trailing line" comes after the start marker without an end marker,
	// so it should be consumed as part of the artifact block.
	if contains(got, "trailing line") {
		t.Error("trailing line after unclosed artifact should be stripped")
	}
}

func TestStripArtifactBlocksEmptyInput(t *testing.T) {
	got := stripArtifactBlocks("")
	if got != "" {
		t.Errorf("expected empty output, got %q", got)
	}
}

func TestStripArtifactBlocksAtStart(t *testing.T) {
	// Artifact block as the very first content in the output.
	content := base64.StdEncoding.EncodeToString([]byte("first"))
	input := "===ARTIFACT:first.dat===\n" + content + "\n===END_ARTIFACT===\nafter\n"

	got := stripArtifactBlocks(input)
	if contains(got, "===ARTIFACT") {
		t.Error("stripped output still contains ARTIFACT marker")
	}
	if !contains(got, "after") {
		t.Error("stripped output missing 'after' line")
	}
}

func TestStripArtifactBlocksAtEnd(t *testing.T) {
	// Artifact block as the very last content, no trailing newline.
	content := base64.StdEncoding.EncodeToString([]byte("last"))
	input := "before\n===ARTIFACT:last.dat===\n" + content + "\n===END_ARTIFACT==="

	got := stripArtifactBlocks(input)
	if contains(got, "===ARTIFACT") {
		t.Error("stripped output still contains ARTIFACT marker")
	}
	if !contains(got, "before") {
		t.Error("stripped output missing 'before' line")
	}
}

func TestStripArtifactBlocksPreservesExactNonArtifactContent(t *testing.T) {
	// Verify that non-artifact content including blank lines is preserved exactly.
	input := "line1\n\nline3\n===ARTIFACT:x.dat===\nZGF0YQ==\n===END_ARTIFACT===\n\nline5\n"

	got := stripArtifactBlocks(input)
	if contains(got, "===ARTIFACT") {
		t.Error("stripped output still contains ARTIFACT marker")
	}
	if !contains(got, "line1") || !contains(got, "line3") || !contains(got, "line5") {
		t.Errorf("stripped output missing expected lines: %q", got)
	}
}

// --- buildJobEnv tests ---

func TestBuildJobEnvBasic(t *testing.T) {
	plugin := Plugin{
		Slug:     "qe",
		Database: "qe",
		Input: []PluginInput{
			{Name: "input_file", Type: "text"},
			{Name: "num_cpus", Type: "int"},
			{Name: "executable", Type: "string"},
		},
	}

	input := map[string]interface{}{
		"input_file": "some content",
		"num_cpus":   4,
		"executable": "pw.x",
	}

	envs := buildJobEnv(plugin, "test-job-1", input)

	// Build a lookup map for easier assertion.
	envMap := make(map[string]string)
	for _, e := range envs {
		envMap[e.Name] = e.Value
	}

	// Standard env vars should always be present.
	if envMap["JOB_NAME"] != "test-job-1" {
		t.Errorf("JOB_NAME: expected %q, got %q", "test-job-1", envMap["JOB_NAME"])
	}
	if envMap["WORKFLOW_NAME"] != "test-job-1" {
		t.Errorf("WORKFLOW_NAME: expected %q, got %q", "test-job-1", envMap["WORKFLOW_NAME"])
	}
	if envMap["MYSQL_DATABASE"] != "qe" {
		t.Errorf("MYSQL_DATABASE: expected %q, got %q", "qe", envMap["MYSQL_DATABASE"])
	}

	// Non-text input fields should be passed as uppercase env vars.
	if envMap["NUM_CPUS"] != "4" {
		t.Errorf("NUM_CPUS: expected %q, got %q", "4", envMap["NUM_CPUS"])
	}
	if envMap["EXECUTABLE"] != "pw.x" {
		t.Errorf("EXECUTABLE: expected %q, got %q", "pw.x", envMap["EXECUTABLE"])
	}
}

func TestBuildJobEnvTextFieldsExcluded(t *testing.T) {
	// Text fields are mounted as files, not env vars.
	plugin := Plugin{
		Slug:     "test",
		Database: "test",
		Input: []PluginInput{
			{Name: "input_file", Type: "text"},
			{Name: "config", Type: "text"},
		},
	}

	input := map[string]interface{}{
		"input_file": "large text content",
		"config":     "config content",
	}

	envs := buildJobEnv(plugin, "job-1", input)

	envMap := make(map[string]string)
	for _, e := range envs {
		envMap[e.Name] = e.Value
	}

	if _, exists := envMap["INPUT_FILE"]; exists {
		t.Error("text field INPUT_FILE should not be in env vars")
	}
	if _, exists := envMap["CONFIG"]; exists {
		t.Error("text field CONFIG should not be in env vars")
	}
}

func TestBuildJobEnvNilValuesSkipped(t *testing.T) {
	plugin := Plugin{
		Slug:     "test",
		Database: "test",
		Input: []PluginInput{
			{Name: "optional_field", Type: "string"},
			{Name: "present_field", Type: "string"},
		},
	}

	input := map[string]interface{}{
		"optional_field": nil,
		"present_field":  "value",
	}

	envs := buildJobEnv(plugin, "job-1", input)

	envMap := make(map[string]string)
	for _, e := range envs {
		envMap[e.Name] = e.Value
	}

	if _, exists := envMap["OPTIONAL_FIELD"]; exists {
		t.Error("nil-valued field should not be in env vars")
	}
	if envMap["PRESENT_FIELD"] != "value" {
		t.Errorf("PRESENT_FIELD: expected %q, got %q", "value", envMap["PRESENT_FIELD"])
	}
}

func TestBuildJobEnvMissingInputSkipped(t *testing.T) {
	// Input fields defined in the plugin spec but absent from the input map.
	plugin := Plugin{
		Slug:     "test",
		Database: "test",
		Input: []PluginInput{
			{Name: "defined_but_missing", Type: "int"},
		},
	}

	input := map[string]interface{}{}

	envs := buildJobEnv(plugin, "job-1", input)

	envMap := make(map[string]string)
	for _, e := range envs {
		envMap[e.Name] = e.Value
	}

	if _, exists := envMap["DEFINED_BUT_MISSING"]; exists {
		t.Error("missing input field should not produce an env var")
	}
}

func TestBuildJobEnvNoInputFields(t *testing.T) {
	// Plugin with no input fields should still produce the standard env vars.
	plugin := Plugin{
		Slug:     "minimal",
		Database: "minimal",
		Input:    nil,
	}

	envs := buildJobEnv(plugin, "job-1", map[string]interface{}{})

	if len(envs) < 5 {
		t.Errorf("expected at least 5 standard env vars, got %d", len(envs))
	}

	envMap := make(map[string]string)
	for _, e := range envs {
		envMap[e.Name] = e.Value
	}

	for _, required := range []string{"JOB_NAME", "WORKFLOW_NAME", "MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_DATABASE"} {
		if _, exists := envMap[required]; !exists {
			t.Errorf("missing required env var %s", required)
		}
	}
}

func TestBuildJobEnvNumericFormatting(t *testing.T) {
	// Verify numeric values are formatted properly via fmt.Sprintf("%v", ...).
	plugin := Plugin{
		Slug:     "test",
		Database: "test",
		Input: []PluginInput{
			{Name: "count", Type: "int"},
			{Name: "threshold", Type: "float"},
		},
	}

	input := map[string]interface{}{
		"count":     float64(8),
		"threshold": float64(3.14),
	}

	envs := buildJobEnv(plugin, "job-1", input)

	envMap := make(map[string]string)
	for _, e := range envs {
		envMap[e.Name] = e.Value
	}

	if envMap["COUNT"] != "8" {
		t.Errorf("COUNT: expected %q, got %q", "8", envMap["COUNT"])
	}
	if envMap["THRESHOLD"] != "3.14" {
		t.Errorf("THRESHOLD: expected %q, got %q", "3.14", envMap["THRESHOLD"])
	}
}
