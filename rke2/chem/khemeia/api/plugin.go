// Package main provides the YAML-driven plugin system for the Khemeia API.
// Plugins define compute backends (QE, docking, etc.) as declarative YAML files
// that are loaded at startup and used to generate API routes, database tables,
// and K8s job specifications.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Plugin represents a compute backend loaded from a YAML plugin file.
type Plugin struct {
	Name     string         `yaml:"name"`
	Slug     string         `yaml:"slug"`
	Version  string         `yaml:"version"`
	Image    string         `yaml:"image"`
	Type     string         `yaml:"type"`
	Database string         `yaml:"database"`
	Input    []PluginInput  `yaml:"input"`
	Output   []PluginOutput `yaml:"output"`
	Resources PluginResources `yaml:"resources"`
	Command  string         `yaml:"command"`
}

// PluginInput defines an input field for a plugin.
type PluginInput struct {
	Name        string      `yaml:"name" json:"name"`
	Type        string      `yaml:"type" json:"type"`
	Required    bool        `yaml:"required" json:"required"`
	Default     interface{} `yaml:"default,omitempty" json:"default,omitempty"`
	Max         interface{} `yaml:"max,omitempty" json:"max,omitempty"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
}

// PluginOutput defines an output field for a plugin.
type PluginOutput struct {
	Name  string `yaml:"name" json:"name"`
	Type  string `yaml:"type" json:"type"`
	Parse string `yaml:"parse,omitempty" json:"parse,omitempty"`
}

// PluginResources defines resource constraints for a plugin's K8s jobs.
type PluginResources struct {
	CPU     string `yaml:"cpu"`
	Memory  string `yaml:"memory"`
	Timeout string `yaml:"timeout"`
}

// TableName returns the MySQL table name for this plugin's jobs.
func (p *Plugin) TableName() string {
	return fmt.Sprintf("%s_jobs", p.Slug)
}

// TimeoutDuration parses the plugin's timeout string into a time.Duration.
// Supports formats like "4h", "1h30m", "30m".
func (p *Plugin) TimeoutDuration() time.Duration {
	d, err := time.ParseDuration(p.Resources.Timeout)
	if err != nil {
		return 4 * time.Hour // default fallback
	}
	return d
}

// GenerateTableDDL returns a CREATE TABLE IF NOT EXISTS statement for this plugin.
// All plugins share the same table schema with JSON columns for flexible input/output.
func (p *Plugin) GenerateTableDDL() string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id            INT AUTO_INCREMENT PRIMARY KEY,
		name          VARCHAR(255) NOT NULL UNIQUE,
		status        ENUM('Pending','Running','Completed','Failed') NOT NULL DEFAULT 'Pending',
		submitted_by  VARCHAR(255) NULL,
		input_data    JSON NULL,
		output_data   JSON NULL,
		error_output  MEDIUMTEXT NULL,
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at    TIMESTAMP NULL,
		completed_at  TIMESTAMP NULL,
		INDEX idx_status (status),
		INDEX idx_created_at (created_at)
	)`, p.TableName())
}

// ValidateInput validates the provided input data against the plugin's input schema.
// It checks required fields, type compatibility, and max value constraints.
func (p *Plugin) ValidateInput(data map[string]interface{}) error {
	for _, field := range p.Input {
		val, exists := data[field.Name]

		// Check required fields.
		if field.Required && (!exists || val == nil) {
			return fmt.Errorf("%s is required", field.Name)
		}

		if !exists || val == nil {
			continue
		}

		// Type validation.
		switch field.Type {
		case "string":
			if _, ok := val.(string); !ok {
				return fmt.Errorf("%s must be a string", field.Name)
			}
		case "text":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", field.Name)
			}
			if field.Required && strings.TrimSpace(s) == "" {
				return fmt.Errorf("%s is required", field.Name)
			}
		case "int":
			num, ok := toFloat64(val)
			if !ok {
				return fmt.Errorf("%s must be a number", field.Name)
			}
			// Check max constraint.
			if field.Max != nil {
				maxVal, ok := toFloat64(field.Max)
				if ok && num > maxVal {
					return fmt.Errorf("%s exceeds maximum value of %v", field.Name, field.Max)
				}
			}
		case "float":
			if _, ok := toFloat64(val); !ok {
				return fmt.Errorf("%s must be a number", field.Name)
			}
		}
	}

	return nil
}

// ApplyDefaults fills in default values for any missing input fields.
func (p *Plugin) ApplyDefaults(data map[string]interface{}) {
	for _, field := range p.Input {
		if _, exists := data[field.Name]; !exists && field.Default != nil {
			data[field.Name] = field.Default
		}
	}
}

// ExpandCommand expands template variables ({{ .field_name }}) in the plugin's
// command string with actual input values.
func (p *Plugin) ExpandCommand(data map[string]interface{}) string {
	result := p.Command
	for key, val := range data {
		placeholder := fmt.Sprintf("{{ .%s }}", key)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", val))
	}
	return result
}

// ExpandResource expands template variables in a resource string (e.g., "{{ .num_cpus }}").
func (p *Plugin) ExpandResource(template string, data map[string]interface{}) string {
	result := template
	for key, val := range data {
		placeholder := fmt.Sprintf("{{ .%s }}", key)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", val))
	}
	return result
}

// ParseOutput applies the plugin's output parse regexes to the given text
// and returns a map of parsed output values.
func (p *Plugin) ParseOutput(text string) map[string]interface{} {
	result := make(map[string]interface{})
	for _, out := range p.Output {
		if out.Parse == "" {
			continue
		}
		re, err := regexp.Compile(out.Parse)
		if err != nil {
			continue
		}
		// Use FindAllStringSubmatch and take the last match (for iterative values).
		matches := re.FindAllStringSubmatch(text, -1)
		if len(matches) == 0 {
			continue
		}
		lastMatch := matches[len(matches)-1]
		if len(lastMatch) < 2 {
			continue
		}
		result[out.Name] = lastMatch[1]
	}
	return result
}

// LoadPlugins reads all .yaml files from the given directory and returns
// the parsed Plugin definitions. It returns an error if the directory is
// unreadable or any YAML file fails to parse.
func LoadPlugins(dir string) ([]Plugin, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No plugins directory — start with zero plugins
		}
		return nil, fmt.Errorf("reading plugins directory %s: %w", dir, err)
	}

	var plugins []Plugin
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading plugin file %s: %w", path, err)
		}

		var p Plugin
		if err := yaml.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("parsing plugin file %s: %w", path, err)
		}

		// Validate required fields.
		if p.Name == "" || p.Slug == "" || p.Image == "" || p.Type == "" || p.Database == "" {
			return nil, fmt.Errorf("plugin %s missing required fields (name, slug, image, type, database)", path)
		}

		plugins = append(plugins, p)
	}

	return plugins, nil
}

// toFloat64 converts a value to float64, handling JSON number types.
// JSON numbers unmarshal as float64 by default.
func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	default:
		return 0, false
	}
}
