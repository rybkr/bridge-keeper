package types

// ToolCall represents a request from the LLM to execute a specific tool.
type ToolCall struct {
	ID     string         `json:"id,omitempty"`   // e.g., "line" or a genai ID
	Tool   string         `json:"tool"`           // e.g., "git"
	Action string         `json:"action"`         // e.g., "execute_git_command"
	Args   map[string]any `json:"args,omitempty"` // The arguments passed to the tool
}

// Decision represents the outcome of a policy evaluation.
type Decision string

const (
	Allow Decision = "allow"
	Deny  Decision = "deny"
	Ask   Decision = "ask"
)

// PolicyDecision represents the final evaluated result.
type PolicyDecision struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason"`
	Rule     string   `json:"rule"`
}

// JSONRPCRequest represents a standard JSON-RPC request wrapper.
type JSONRPCRequest struct {
	JSONRPC string         `json:"jsonrpc,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
	ID      any            `json:"id,omitempty"`
}
