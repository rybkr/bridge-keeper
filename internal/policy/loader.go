package policy

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PolicyFile represents the root of a loaded policy YAML.
type PolicyFile struct {
	Version      string       `yaml:"version"`
	Default      string       `yaml:"default"`
	Capabilities []Capability `yaml:"capabilities"`
}

// Capability defines a specific access rule for a tool.
type Capability struct {
	Name        string       `yaml:"name"`
	Tool        string       `yaml:"tool"`
	Actions     []string     `yaml:"actions"`
	Decision    string       `yaml:"decision"`
	Constraints *Constraints `yaml:"constraints,omitempty"`
}

// Constraints defines limits on how a capability can be used.
type Constraints struct {
	Paths          *AllowDeny `yaml:"paths,omitempty"`
	Commands       *AllowDeny `yaml:"commands,omitempty"`
	Domains        *AllowDeny `yaml:"domains,omitempty"`
	MaxSizeBytes   int64      `yaml:"max_size_bytes,omitempty"`
	TimeoutSeconds int        `yaml:"timeout_seconds,omitempty"`
}

// AllowDeny defines explicit allow and deny lists for string matching.
type AllowDeny struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

// LoadPath loads and parses a PolicyFile from a file or directory path.
func LoadPath(path string) (*PolicyFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// If the user provided a directory (like "policies"), default to looking for "default.yaml" inside it.
	if info.IsDir() {
		path = filepath.Join(path, "default.yaml")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy file: %w", err)
	}

	var pf PolicyFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("failed to parse policy YAML: %w", err)
	}

	return &pf, nil
}
