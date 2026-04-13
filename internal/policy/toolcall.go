package policy

// ToolCall represents a request from the LLM to execute a specific tool.
type ToolCall struct {
	ID     string         `json:"id,omitempty"`   // e.g., "line" or a genai ID
	Tool   string         `json:"tool"`           // e.g., "git"
	Action string         `json:"action"`         // e.g., "execute_git_command"
	Args   map[string]any `json:"args,omitempty"` // The arguments passed to the tool
}
