package policy

// Engine performs tool call evaluation against the currently loaded
// PolicyFile. Should be safe for concurrent usage since it is Read-Only.
type Engine struct {
	policy *PolicyFile
}

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

// PolicyDecision represents the final evaluated result.
type PolicyDecision struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason"`
	Rule     string   `json:"rule"`
}

// Decision represents the outcome of a policy evaluation.
type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
	Ask   Decision = "ask"
)
