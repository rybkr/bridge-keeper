package agents

// The API request
type OllamaAPIRequest struct {
	Model     string      `json:"model"`
	Messages  []Message   `json:"messages"`
	Stream    bool        `json:"stream"`     // For our usage, always False
	Think     bool        `json:"think"`      // For our usage, extended thinking is disabled
	KeepAlive string      `json:"keep_alive"` // Time to keep model loaded
	Tools     []ModelTool `json:"tools"`
}

// The API Response
type OllamaResponse struct {
	Model              string  `json:"model"`
	CreatedAt          string  `json:"created_at"`
	Message            Message `json:"message"`
	Done               bool    `json:"done"`
	DoneReason         string  `json:"done_reason"`
	TotalDuration      int     `json:"total_duration"`       // total time nanoseconds
	LoadDuration       int     `json:"load_duration"`        // nanoseconds to load model
	PromptEvalCount    int     `json:"prompt_eval_count"`    // prompt tokens consumed
	PromptEvalDuration int     `json:"prompt_eval_duration"` // nanoseconds to parse prompt
	EvalCount          int     `json:"eval_count"`           // response tokens consumed
	EvalDuration       int     `json:"eval_duration"`        // nanoseconds to generate output
}

type Message struct {
	Role      string      `json:"role"`
	Content   string      `json:"content"`            // Timestamp ISO 8601
	Thinking  string      `json:"thinking,omitempty"` // model's "thinking"
	ToolCalls []ModelTool `json:"tool_calls,omitempty"`
}

var OllamaTools []ModelTool = []ModelTool{
	{
		Type: "FUNCTION",
		Function: ToolFunction{
			Name:        "read_file",
			Description: "Read a file from the file system",
			Parameters: ToolFunctionParams{
				Type:     "OBJECT",
				Required: []string{"path"},
				Properties: map[string]FuncParamItem{
					"path": FuncParamItem{
						Type:        "STRING",
						Description: "Path to the file",
					},
				},
			},
		},
	},
	{
		Type: "FUNCTION",
		Function: ToolFunction{
			Name:        "write_file",
			Description: "Write a file",
			Parameters: ToolFunctionParams{
				Type:     "OBJECT",
				Required: []string{"path", "content"},
				Properties: map[string]FuncParamItem{
					"path": FuncParamItem{
						Type:        "STRING",
						Description: "Path to the file",
					},
					"content": FuncParamItem{
						Type:        "STRING",
						Description: "Content of the file",
					},
				},
			},
		},
	},
	{
		Type: "FUNCTION",
		Function: ToolFunction{
			Name:        "list_directory",
			Description: "List the files of the file directory",
			Parameters: ToolFunctionParams{
				Type:     "OBJECT",
				Required: []string{"path"},
				Properties: map[string]FuncParamItem{
					"path": FuncParamItem{
						Type:        "STRING",
						Description: "Path to the directory",
					},
				},
			},
		},
	},
	{
		Type: "FUNCTION",
		Function: ToolFunction{
			Name:        "git",
			Description: "Popular version control toolcahin",
			Parameters: ToolFunctionParams{
				Type:     "OBJECT",
				Required: []string{"action"},
				Properties: map[string]FuncParamItem{
					"action": FuncParamItem{
						Type:        "STRING",
						Description: "An action for git. One of `status`,`add`,`commit`,`pull`,`push`,`restore`,`branch`,`checkout`",
					},
					"args": FuncParamItem{
						Type:        "ARRAY",
						Description: "A collection of additional argument flags and values",
						Items: FuncParamItemArrType{
							Type: "STRING",
						},
					},
				},
			},
		},
	},
	{
		Type: "FUNCTION",
		Function: ToolFunction{
			Name:        "http_get",
			Description: "Use HTTP to query a URL",
			Parameters: ToolFunctionParams{
				Type:     "OBJECT",
				Required: []string{"URL"},
				Properties: map[string]FuncParamItem{
					"URL": FuncParamItem{
						Type:        "STRING",
						Description: "A URL to request from",
					},
				},
			},
		},
	},
	{
		Type: "FUNCTION",
		Function: ToolFunction{
			Name:        "go_version",
			Description: "Get the install version of Go programming language",
			Parameters: ToolFunctionParams{
				Type:       "OBJECT",
				Required:   []string{""},
				Properties: map[string]FuncParamItem{},
			},
		},
	},
	{
		Type: "FUNCTION",
		Function: ToolFunction{
			Name:        "rust_version",
			Description: "Get the install version of Rust programming language",
			Parameters: ToolFunctionParams{
				Type:       "OBJECT",
				Required:   []string{""},
				Properties: map[string]FuncParamItem{},
			},
		},
	},
}

type ModelTool struct {
	ID       string       `json:"id,omitempty"` // Returned in response
	Type     string       `json:"type"`         // Always "function"
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Id          int                `json:"id,omitempty"` // Returned in response
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Parameters  ToolFunctionParams `json:"parameters"`
	Arguments   map[string]any     `json:"arguments,omitempty"` // Returned in response
}

// An argument for a toolcall function
type ToolFunctionParams struct {
	Type       string                   `json:"type"`       // Always "object"
	Required   []string                 `json:"required"`   // list of names of required arguments
	Properties map[string]FuncParamItem `json:"properties"` // dict of actual argument descriptions
}

// The properties of an argument for a toolcall function
type FuncParamItem struct {
	Type        string               `json:"type"`
	Description string               `json:"description"`
	Items       FuncParamItemArrType `json:"items,omitempty"`
}

type FuncParamItemArrType struct {
	Type string `json:"type"`
}
