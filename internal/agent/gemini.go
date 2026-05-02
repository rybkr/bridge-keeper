package agent

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"

	"bridgekeeper/internal/redact"
	"bridgekeeper/internal/runtime"
	"bridgekeeper/internal/tools"
	"bridgekeeper/internal/types"

	"google.golang.org/genai"
)

// GeminiAgent serves as the API layer for interaction with Gemini.
type GeminiAgent struct {
	client       *genai.Client
	currentModel string
	chatSession  *genai.Chat
	isConcise    bool
	lastPath     string
	mediator     *runtime.Mediator
	registry     *tools.Registry
	taint        *redact.TaintTracker
}

func NewGeminiAgent(ctx context.Context, apiKey string, mediator *runtime.Mediator, registry *tools.Registry) *GeminiAgent {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		log.Fatal(err)
	}

	return &GeminiAgent{
		client:       client,
		currentModel: "gemini-2.5-flash-lite",
		lastPath:     registry.WorkspaceRoot,
		mediator:     mediator,
		registry:     registry,
		taint:        redact.NewTaintTracker(),
	}
}

func (agent *GeminiAgent) CurrentModel() string {
	return agent.currentModel
}

func (agent *GeminiAgent) SendMessageWithTools(ctx context.Context, prompt string, conciseMode bool) (string, error) {
	if agent.chatSession == nil || agent.isConcise != conciseMode {
		config := agent.getChatConfig(conciseMode)
		chat, err := agent.client.Chats.Create(ctx, agent.currentModel, config, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create chat session: %w", err)
		}
		agent.chatSession = chat
		agent.isConcise = conciseMode
		agent.taint = redact.NewTaintTracker()
	}
	ctx = runtime.WithTaintTracker(ctx, agent.taint)

	resp, err := agent.chatSession.SendMessage(ctx, genai.Part{Text: prompt})
	if err != nil {
		return "", err
	}

	for {
		if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
			break
		}

		var functionResponses []genai.Part
		hasFunctionCall := false

		for _, part := range resp.Candidates[0].Content.Parts {
			if funcCall := part.FunctionCall; funcCall != nil {
				hasFunctionCall = true

				toolCall := types.ToolCall{
					ID:     "genai-internal",
					Tool:   geminiToolFamily(funcCall.Name),
					Action: geminiActionName(funcCall.Name, funcCall.Args),
					Args:   funcCall.Args,
				}

				responseContent, err := agent.mediator.Execute(ctx, toolCall, func(_ context.Context, args map[string]any) (string, error) {
					return agent.executeTool(ctx, funcCall.Name, args)
				})
				if err != nil {
					return "", err
				}

				functionResponses = append(functionResponses, genai.Part{
					FunctionResponse: &genai.FunctionResponse{
						ID:   funcCall.ID,
						Name: funcCall.Name,
						Response: map[string]any{
							"result": responseContent,
						},
					},
				})
			}
		}

		if !hasFunctionCall {
			break
		}

		resp, err = agent.chatSession.SendMessage(ctx, functionResponses...)
		if err != nil {
			return "", err
		}
	}

	return resp.Text(), nil
}

func (agent *GeminiAgent) ListModels(ctx context.Context) ([]string, error) {
	var modelNames []string
	models, err := agent.client.Models.List(ctx, &genai.ListModelsConfig{})
	if err != nil {
		return nil, err
	}
	for _, m := range models.Items {
		modelNames = append(modelNames, m.Name)
	}
	return modelNames, nil
}

func (agent *GeminiAgent) SetModel(modelName string) {
	agent.currentModel = modelName
	agent.chatSession = nil
}

func (agent *GeminiAgent) executeTool(ctx context.Context, name string, args map[string]any) (string, error) {
	switch name {
	case "execute_git_command":
		if pathAny, exists := args["path"]; exists {
			if pathStr, ok := pathAny.(string); ok && pathStr != "" {
				agent.lastPath = pathStr
			}
		}

		argsAny, exists := args["args"].([]any)
		if !exists {
			return "Error: model failed to provide git arguments.", nil
		}

		var gitArgs []string
		for _, arg := range argsAny {
			if strArg, ok := arg.(string); ok {
				gitArgs = append(gitArgs, strArg)
			}
		}
		return agent.registry.ExecuteGitCommand(ctx, tools.GitExecArgs{Path: agent.lastPath, Args: gitArgs})
	case "read_file":
		pathAny, exists := args["path"]
		if !exists {
			return "Error: model failed to provide file path.", nil
		}
		pathStr, ok := pathAny.(string)
		if !ok || pathStr == "" {
			return "Error: path argument is invalid or empty.", nil
		}
		return agent.registry.ReadFile(ctx, tools.ReadFileArgs{Path: pathStr})
	case "write_file":
		pathAny, hasPath := args["path"]
		contentAny, hasContent := args["content"]
		if !hasPath || !hasContent {
			return "Error: model failed to provide file path or content.", nil
		}
		pathStr, ok := pathAny.(string)
		if !ok || pathStr == "" {
			return "Error: path argument is invalid or empty.", nil
		}
		contentStr, ok := contentAny.(string)
		if !ok {
			return "Error: content argument must be a string.", nil
		}
		return agent.registry.WriteFile(ctx, tools.WriteFileArgs{Path: pathStr, Content: contentStr})
	case "http_get":
		urlAny, hasURL := args["url"]
		if !hasURL {
			return "Error: model failed to provide a URL.", nil
		}
		urlStr, ok := urlAny.(string)
		if !ok || urlStr == "" {
			return "Error: url argument is invalid or empty.", nil
		}
		return agent.registry.HTTPGet(ctx, tools.HTTPGetArgs{URL: urlStr})
	case "http_post":
		urlStr, ok := stringArg(args, "url")
		if !ok {
			return "Error: url argument is invalid or empty.", nil
		}
		bodyStr, ok := stringArg(args, "body")
		if !ok {
			return "Error: body argument is invalid or empty.", nil
		}
		contentType, _ := stringArg(args, "content_type")
		return agent.registry.HTTPPost(ctx, tools.HTTPPostArgs{URL: urlStr, Body: bodyStr, ContentType: contentType})
	case "shell_exec":
		command, ok := stringArg(args, "command")
		if !ok {
			return "Error: command argument is invalid or empty.", nil
		}
		path, _ := stringArg(args, "path")
		return agent.registry.ExecuteShellCommand(ctx, tools.ShellExecArgs{Command: command, Path: path})
	case "pkg_list":
		manager, _ := stringArg(args, "manager")
		path, _ := stringArg(args, "path")
		return agent.registry.PackageList(ctx, tools.PackageListArgs{Manager: manager, Path: path})
	case "pkg_query":
		manager, _ := stringArg(args, "manager")
		pkg, ok := stringArg(args, "package")
		if !ok {
			return "Error: package argument is invalid or empty.", nil
		}
		path, _ := stringArg(args, "path")
		return agent.registry.PackageQuery(ctx, tools.PackageQueryArgs{Manager: manager, Package: pkg, Path: path})
	case "pkg_install":
		manager, _ := stringArg(args, "manager")
		pkg, ok := stringArg(args, "package")
		if !ok {
			return "Error: package argument is invalid or empty.", nil
		}
		version, _ := stringArg(args, "version")
		path, _ := stringArg(args, "path")
		return agent.registry.PackageInstall(ctx, tools.PackageInstallArgs{Manager: manager, Package: pkg, Version: version, Path: path})
	case "pkg_update":
		manager, _ := stringArg(args, "manager")
		pkg, _ := stringArg(args, "package")
		path, _ := stringArg(args, "path")
		return agent.registry.PackageUpdate(ctx, tools.PackageUpdateArgs{Manager: manager, Package: pkg, Path: path})
	case "list_directory":
		if pathAny, exists := args["path"]; exists {
			if pathStr, ok := pathAny.(string); ok && pathStr != "" {
				agent.lastPath = pathStr
			}
		}
		return agent.registry.ListDirectory(ctx, tools.ListDirectoryArgs{Path: agent.lastPath})
	default:
		return fmt.Sprintf("Error: Unknown function %s called.", name), nil
	}
}

func stringArg(args map[string]any, key string) (string, bool) {
	raw, ok := args[key]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

func packageToolSchema(required []string) *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"manager": {
				Type:        genai.TypeString,
				Description: "The package manager to use: go or cargo. If omitted, Bridgekeeper detects go.mod or Cargo.toml.",
			},
			"package": {
				Type:        genai.TypeString,
				Description: "The package, module, or crate name.",
			},
			"version": {
				Type:        genai.TypeString,
				Description: "Optional version for dependency installation.",
			},
			"path": {
				Type:        genai.TypeString,
				Description: "The project directory. Defaults to the workspace root.",
			},
		},
		Required: required,
	}
}

func (agent *GeminiAgent) getChatConfig(conciseMode bool) *genai.GenerateContentConfig {
	toolBox := &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "execute_git_command",
				Description: "Executes a git command in a local repository. Use this to check status, view logs, examine diffs, etc. Only provide the arguments, not the 'git' binary itself.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"args": {
							Type:        genai.TypeArray,
							Description: "A list of strings representing the git arguments (e.g., ['log', '-n', '3']).",
							Items:       &genai.Schema{Type: genai.TypeString},
						},
						"path": {
							Type:        genai.TypeString,
							Description: "The directory path of the git repository. If omitted, the agent will use the previously accessed repository.",
						},
					},
					Required: []string{"args"},
				},
			},
			{
				Name:        "read_file",
				Description: "Reads the full contents of a local file. Use this to analyze, summarize, or reference specific parts of a file. Provide the path to the file.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"path": {
							Type:        genai.TypeString,
							Description: "The absolute or relative path to the file to read.",
						},
					},
					Required: []string{"path"},
				},
			},
			{
				Name:        "write_file",
				Description: "Writes text content to a local file. Use this only when the user explicitly wants to create or update a file.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"path": {
							Type:        genai.TypeString,
							Description: "The absolute or relative path to the file to write.",
						},
						"content": {
							Type:        genai.TypeString,
							Description: "The text content to write to the file.",
						},
					},
					Required: []string{"path", "content"},
				},
			},
			{
				Name:        "http_get",
				Description: "Fetches the contents of an HTTP or HTTPS URL. Use this for read-only network retrieval.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"url": {
							Type:        genai.TypeString,
							Description: "The HTTP or HTTPS URL to fetch.",
						},
					},
					Required: []string{"url"},
				},
			},
			{
				Name:        "http_post",
				Description: "Sends a bounded HTTP POST request to an HTTP or HTTPS URL.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"url": {
							Type:        genai.TypeString,
							Description: "The HTTP or HTTPS URL to send the request to.",
						},
						"body": {
							Type:        genai.TypeString,
							Description: "The request body to send.",
						},
						"content_type": {
							Type:        genai.TypeString,
							Description: "The request content type. Defaults to application/json.",
						},
					},
					Required: []string{"url", "body"},
				},
			},
			{
				Name:        "shell_exec",
				Description: "Runs a simple allowlisted local command without shell metacharacters.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"command": {
							Type:        genai.TypeString,
							Description: "The exact command to run, for example 'ls .' or 'wc README.md'.",
						},
						"path": {
							Type:        genai.TypeString,
							Description: "The workspace directory to run the command in.",
						},
					},
					Required: []string{"command"},
				},
			},
			{
				Name:        "pkg_list",
				Description: "Lists dependencies for a Go module or Cargo project.",
				Parameters:  packageToolSchema([]string{}),
			},
			{
				Name:        "pkg_query",
				Description: "Queries available versions or registry information for a package.",
				Parameters:  packageToolSchema([]string{"package"}),
			},
			{
				Name:        "pkg_install",
				Description: "Adds or updates a dependency in a Go module or Cargo project.",
				Parameters:  packageToolSchema([]string{"package"}),
			},
			{
				Name:        "pkg_update",
				Description: "Updates dependencies in a Go module or Cargo project.",
				Parameters:  packageToolSchema([]string{}),
			},
			{
				Name:        "list_directory",
				Description: "Lists the contents of a specified directory. Use this to explore the repository structure, find files, or check for the presence of specific items.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"path": {
							Type:        genai.TypeString,
							Description: "The path to the directory to list.",
						},
					},
					Required: []string{"path"},
				},
			},
		},
	}

	config := &genai.GenerateContentConfig{
		Tools: []*genai.Tool{toolBox},
	}

	if conciseMode {
		config.SystemInstruction = &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{Text: "You are a highly efficient, general-purpose assistant. You can answer general knowledge questions, write code, and chat normally. You ALSO have access to tools to interact with Git repositories. Always provide extremely concise, direct, and brief answers. Omit unnecessary pleasantries, filler words, or long explanations unless explicitly asked."},
			},
		}
		config.Temperature = genai.Ptr(rand.Float32() * 0.5)
	} else {
		config.SystemInstruction = &genai.Content{
			Role: "model",
			Parts: []*genai.Part{
				{Text: "You are a verbose, general-purpose assistant. You can answer general knowledge questions, write code, and chat normally. You ALSO have access to tools to interact with Git repositories. Always provide detailed, comprehensive, and thorough answers. Include all relevant information and context unless explicitly asked to be concise."},
			},
		}
		config.Temperature = genai.Ptr(rand.Float32() + 1.0)
	}

	return config
}

func geminiToolFamily(name string) string {
	switch name {
	case "execute_git_command":
		return "git"
	case "http_get", "http_post":
		return "http"
	case "shell_exec":
		return "shell"
	case "pkg_list", "pkg_query", "pkg_install", "pkg_update":
		return "pkg"
	case "read_file", "write_file", "list_directory":
		return "fs"
	default:
		return "unknown"
	}
}

func geminiActionName(name string, args map[string]any) string {
	switch name {
	case "execute_git_command":
		if argsAny, ok := args["args"].([]any); ok && len(argsAny) > 0 {
			if subCommand, ok := argsAny[0].(string); ok {
				return subCommand
			}
		}
		return ""
	case "read_file":
		return "read_file"
	case "write_file":
		return "write_file"
	case "http_get":
		return "get"
	case "http_post":
		return "post"
	case "shell_exec":
		return "exec"
	case "pkg_list":
		return "list"
	case "pkg_query":
		return "query"
	case "pkg_install":
		return "install"
	case "pkg_update":
		return "update"
	case "list_directory":
		return "list_dir"
	default:
		return ""
	}
}

func TrimModelName(name string) string {
	return strings.TrimLeft(name, "models/")
}
