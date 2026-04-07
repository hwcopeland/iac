package main

import (
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
	if len(qe.Output) != 3 {
		t.Errorf("QE outputs: expected 3, got %d", len(qe.Output))
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
