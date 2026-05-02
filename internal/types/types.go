package types

// ToolCall represents a request from the LLM to execute a specific tool.
type ToolCall struct {
	ID     string         `json:"id,omitempty"`   // e.g., "line" or a genai ID
	Tool   string         `json:"tool"`           // e.g., "git"
	Action string         `json:"action"`         // e.g., "execute_git_command"
	Args   map[string]any `json:"args,omitempty"` // The arguments passed to the tool
}

// PolicySubject describes the authenticated execution subject used for
// role/session-scoped policy rules.
type PolicySubject struct {
	Role      string   `json:"role,omitempty" yaml:"role,omitempty"`
	SessionID string   `json:"session_id,omitempty" yaml:"session_id,omitempty"`
	Labels    []string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// Decision represents the outcome of a policy evaluation.
type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
	Ask   Decision = "ask"
)

// ApprovalMetadata carries human-approval details from policy to approvers and
// audit logs.
type ApprovalMetadata struct {
	Required  bool     `json:"required,omitempty" yaml:"required,omitempty"`
	Reason    string   `json:"reason,omitempty" yaml:"reason,omitempty"`
	Message   string   `json:"message,omitempty" yaml:"message,omitempty"`
	Ticket    string   `json:"ticket,omitempty" yaml:"ticket,omitempty"`
	Approvers []string `json:"approvers,omitempty" yaml:"approvers,omitempty"`
}

// Effect is a typed declaration of the side effect a capability may perform.
type Effect struct {
	Type     string            `json:"type" yaml:"type"`
	Resource string            `json:"resource,omitempty" yaml:"resource,omitempty"`
	Mode     string            `json:"mode,omitempty" yaml:"mode,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// AuditRequirement describes the audit guarantees a policy requires for a
// matched capability.
type AuditRequirement struct {
	Required   bool     `json:"required,omitempty" yaml:"required,omitempty"`
	Level      string   `json:"level,omitempty" yaml:"level,omitempty"`
	Events     []string `json:"events,omitempty" yaml:"events,omitempty"`
	RetainDays int      `json:"retain_days,omitempty" yaml:"retain_days,omitempty"`
}

// PolicyDecision represents the final evaluated result.
type PolicyDecision struct {
	Decision    Decision          `json:"decision"`
	Reason      string            `json:"reason"`
	Rule        string            `json:"rule"`
	RiskLevel   string            `json:"risk_level,omitempty"`
	Approval    *ApprovalMetadata `json:"approval,omitempty"`
	Effects     []Effect          `json:"effects,omitempty"`
	Audit       *AuditRequirement `json:"audit,omitempty"`
	Remediation string            `json:"remediation,omitempty"`
}

// JSONRPCRequest represents a standard JSON-RPC request wrapper.
type JSONRPCRequest struct {
	JSONRPC string         `json:"jsonrpc,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
	ID      any            `json:"id,omitempty"`
}
