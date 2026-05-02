package policy

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"bridgekeeper/internal/types"

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
	Name        string                  `yaml:"name"`
	Tool        string                  `yaml:"tool"`
	Actions     []string                `yaml:"actions"`
	Decision    string                  `yaml:"decision"`
	Scope       *Scope                  `yaml:"scope,omitempty"`
	Conditions  *Condition              `yaml:"conditions,omitempty"`
	RiskLevel   string                  `yaml:"risk_level,omitempty"`
	Approval    *types.ApprovalMetadata `yaml:"approval,omitempty"`
	Effects     []types.Effect          `yaml:"effects,omitempty"`
	Audit       *types.AuditRequirement `yaml:"audit,omitempty"`
	Remediation string                  `yaml:"remediation,omitempty"`
	Constraints *Constraints            `yaml:"constraints,omitempty"`
}

// Scope limits a capability to authenticated roles, sessions, or subject
// labels supplied through the policy evaluation context.
type Scope struct {
	Roles    []string `yaml:"roles,omitempty"`
	Sessions []string `yaml:"sessions,omitempty"`
	Labels   []string `yaml:"labels,omitempty"`
}

// Condition is a small policy expression AST. It supports boolean composition
// via all/any/not and leaf comparisons against call, subject, or argument
// fields. Examples of field names are "tool", "action", "role",
// "session_id", and "args.path".
type Condition struct {
	All         []Condition `yaml:"all,omitempty"`
	Any         []Condition `yaml:"any,omitempty"`
	Not         *Condition  `yaml:"not,omitempty"`
	Field       string      `yaml:"field,omitempty"`
	Arg         string      `yaml:"arg,omitempty"`
	Op          string      `yaml:"op,omitempty"`
	Value       any         `yaml:"value,omitempty"`
	Values      []any       `yaml:"values,omitempty"`
	Remediation string      `yaml:"remediation,omitempty"`
}

// Constraints defines limits on how a capability can be used.
type Constraints struct {
	Paths            *AllowDeny              `yaml:"paths,omitempty"`
	Commands         *AllowDeny              `yaml:"commands,omitempty"`
	Domains          *AllowDeny              `yaml:"domains,omitempty"`
	Arguments        map[string]ArgumentSpec `yaml:"arguments,omitempty"`
	AllowUnknownArgs bool                    `yaml:"allow_unknown_args,omitempty"`
	MaxSizeBytes     int64                   `yaml:"max_size_bytes,omitempty"`
	TimeoutSeconds   int                     `yaml:"timeout_seconds,omitempty"`
	Remediation      string                  `yaml:"remediation,omitempty"`
}

// AllowDeny defines explicit allow and deny lists for string matching.
type AllowDeny struct {
	Allow       []string `yaml:"allow,omitempty"`
	Deny        []string `yaml:"deny,omitempty"`
	Remediation string   `yaml:"remediation,omitempty"`
}

// ArgumentSpec validates a tool argument before execution. It intentionally
// mirrors a conservative subset of JSON Schema instead of embedding a full
// expression runtime in policy evaluation.
type ArgumentSpec struct {
	Type        string                  `yaml:"type,omitempty"`
	Required    bool                    `yaml:"required,omitempty"`
	Enum        []any                   `yaml:"enum,omitempty"`
	Pattern     string                  `yaml:"pattern,omitempty"`
	Min         *float64                `yaml:"min,omitempty"`
	Max         *float64                `yaml:"max,omitempty"`
	MinLength   *int                    `yaml:"min_length,omitempty"`
	MaxLength   *int                    `yaml:"max_length,omitempty"`
	Items       *ArgumentSpec           `yaml:"items,omitempty"`
	Properties  map[string]ArgumentSpec `yaml:"properties,omitempty"`
	Remediation string                  `yaml:"remediation,omitempty"`
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
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&pf); err != nil {
		return nil, fmt.Errorf("failed to parse policy YAML: %w", err)
	}

	return &pf, nil
}
