package runtime

// Constants
const ollamaPreferredModel = "qwen3.5:9b"
const ollamaModelKeepAlive = "10m" // ten minutes
const OJsonFunction = "function"

// Enum Types
type ORole string
type JsonTypes string

const (
	User       ORole     = "user"
	Assistant  ORole     = "assistant"
	JSONString JsonTypes = "string"
	JSONObject JsonTypes = "object"
	JSONNumber JsonTypes = "number"
	JSONArray  JsonTypes = "array"
	JSONTrue   JsonTypes = "true"
	JSONFalse  JsonTypes = "false"
	JSONNull   JsonTypes = "null"
)

// Struct Types
// All Ollama API types start with captial letter O

// The API request
type OllamaAPIRequest struct {
	Model     string     `json:"model"`
	Messages  []OMessage `json:"messages"`
	Stream    bool       `json:"stream"`     // For our usage, always False
	Think     bool       `json:"think"`      // For our usage, extended thinking is disabled
	KeepAlive string     `json:"keep_alive"` // Time to keep model loaded
	Tools     []OTool    `json:"tools"`
}

// The API Response
type OllamaResponse struct {
	Model              string   `json:"model"`
	CreatedAt          string   `json:"created_at"`
	Message            OMessage `json:"message"`
	Done               bool     `json:"done"`
	DoneReason         string   `json:"done_reason"`
	TotalDuration      int      `json:"total_duration"`       // total time nanoseconds
	LoadDuration       int      `json:"load_duration"`        // nanoseconds to load model
	PromptEvalCount    int      `json:"prompt_eval_count"`    // prompt tokens consumed
	PromptEvalDuration int      `json:"prompt_eval_duration"` // nanoseconds to parse prompt
	EvalCount          int      `json:"eval_count"`           // response tokens consumed
	EvalDuration       int      `json:"eval_duration"`        // nanoseconds to generate output
}

// The basic message type
type OMessage struct {
	Role      ORole   `json:"role"`               // "user"
	Content   string  `json:"content"`            // Timestamp ISO 8601
	Thinking  string  `json:"thinking,omitempty"` // model's "thinking"
	ToolCalls []OTool `json:"tool_calls"`
}

// A tool available for the model to choose from
type OTool struct {
	ID       string    `json:"id,omitempty"` // Returned in response
	Type     string    `json:"type"`         // Always "function"
	Function OFunction `json:"function"`
}

// A function for the toolcall
type OFunction struct {
	Index       int            `json:"id,omitempty"` // Returned in response
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  OFnParams      `json:"parameters"`
	Arguments   map[string]any `json:"arguments,omitempty"` // Returned in response
}

// An argument for a toolcall function
type OFnParams struct {
	Type       string                 `json:"type"`       // Always "object"
	Required   []string               `json:"required"`   // list of names of required arguments
	Properties map[string]OFnPmPropts `json:"properties"` // dict of actual argument descriptions
}

// The properties of an argument for a toolcall function
type OFnPmPropts struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}
