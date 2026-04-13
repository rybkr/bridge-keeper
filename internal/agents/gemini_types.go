package agents

import (
	"bridgekeeper/internal/sandbox"
	"bridgekeeper/internal/tools"

	"google.golang.org/genai"
)

// GeminiAgent serves as the API layer for interaction with Gemini.
type GeminiAgent struct {
	client       *genai.Client
	currentModel string
	chatSession  *genai.Chat
	isConcise    bool
	lastPath     string
	mediator     *sandbox.Mediator
	registry     *tools.Registry
}

var GeminiTools = &genai.Tool{
	FunctionDeclarations: []*genai.FunctionDeclaration{
		{
			Name:        "read_file",
			Description: "Read a file from the file system",
			Parameters: &genai.Schema{
				Type:     genai.TypeObject,
				Required: []string{"path"},
				Properties: map[string]*genai.Schema{ // Corrected structure for properties
					"path": {
						Type:        genai.TypeString,
						Description: "Path to the file",
					},
				},
			},
		},
		{
			Name:        "write_file",
			Description: "Write a file",
			Parameters: &genai.Schema{
				Type:     genai.TypeObject,
				Required: []string{"path", "content"},
				Properties: map[string]*genai.Schema{ // Corrected structure for properties
					"path": {
						Type:        genai.TypeString,
						Description: "Path to the file",
					},
					"content": {
						Type:        genai.TypeString,
						Description: "Content of the file",
					},
				},
			},
		},
		{
			Name:        "list_directory",
			Description: "List the files of the file directory",
			Parameters: &genai.Schema{
				Type:     genai.TypeObject,
				Required: []string{"path"},
				Properties: map[string]*genai.Schema{ // Corrected structure for properties
					"path": {
						Type:        genai.TypeString,
						Description: "Path to the directory",
					},
				},
			},
		},
		{
			Name:        "git",
			Description: "Popular version control toolcahin",
			Parameters: &genai.Schema{
				Type:     genai.TypeObject,
				Required: []string{"action", "args"},
				Properties: map[string]*genai.Schema{ // Corrected structure for properties
					"action": {
						Type:        genai.TypeString,
						Description: "An action for git. One of `status`,`add`,`commit`,`pull`,`push`,`restore`,`branch`,`checkout`",
					},
					"args": {
						Type:        genai.TypeArray,
						Description: "A collection of additional argument flags and values",
						Items: &genai.Schema{
							Type: genai.TypeString,
						},
					},
				},
			},
		},
		{
			Name:        "http_get",
			Description: "Use HTTP to query a URL",
			Parameters: &genai.Schema{
				Type:     genai.TypeObject,
				Required: []string{"URL"},
				Properties: map[string]*genai.Schema{ // Corrected structure for properties
					"URL": {
						Type:        genai.TypeString,
						Description: "A URL to request from",
					},
				},
			},
		},
		{
			Name:        "go_version",
			Description: "Get the install version of Go programming language",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				// Corrected required field list
				Required:   []string{},
				Properties: map[string]*genai.Schema{},
			},
		},
		{
			Name:        "rust_version",
			Description: "Get the install version of Rust programming language",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				// Corrected required field list
				Required:   []string{},
				Properties: map[string]*genai.Schema{},
			},
		},
	},
}
