package main

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"

	"bridgekeeper/internal/runtime"
	"bridgekeeper/internal/tools"
	"bridgekeeper/internal/types"

	"google.golang.org/genai"
)

// GeminiAgent serves as the API layer for interaction with the service.
type GeminiAgent struct {
	client       *genai.Client
	currentModel string
	chatSession  *genai.Chat // Tracks the persistent conversation
	isConcise    bool        // Tracks configuration state
	lastPath     string      // For tools that require file system context
	mediator     *runtime.Mediator
	registry     *tools.Registry
}

// createDefaultGeminiAgent initializes the agent with a default model.
func createDefaultGeminiAgent(ctx context.Context, apiKey string, mediator *runtime.Mediator, registry *tools.Registry) *GeminiAgent {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})

	if err != nil {
		log.Fatal(err)
	}

	agent := GeminiAgent{
		client:       client,
		currentModel: "gemini-2.5-flash-lite",
		lastPath:     registry.WorkspaceRoot,
		mediator:     mediator,
		registry:     registry,
	}
	return &agent
}

func printGeminiCommands(agent *GeminiAgent) {
	fmt.Println("--- BridgeKeeper Gemini ---")
	fmt.Printf("Current Model: %s\n", agent.currentModel)
	fmt.Println("Commands:")
	fmt.Println("  /help          - Show this help message")
	fmt.Println("  /list          - List available models")
	fmt.Println("  /model <name>  - Select a model (e.g., /model gemini-1.5-pro)")
	fmt.Println("  /concise       - Toggle the verboseness of the Model")
	fmt.Println("  <your prompt>  - Chat with the AI (Auto-Tools Enabled)")
	fmt.Println("  /exit          - Quit")
	fmt.Println("-------------------------------")
}

// getChatConfig centralizes the tool registry and system instructions.
func (agent *GeminiAgent) getChatConfig(conciseMode bool) *genai.GenerateContentConfig {

	// Define the Tool Registry
	ToolBox := &genai.Tool{
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
							Items: &genai.Schema{
								Type: genai.TypeString,
							},
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
		Tools: []*genai.Tool{ToolBox},
	}

	// Apply stylistic instructions based on the concise toggle
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

// Endpoint: SendMessageWithTools
// Autonomous loop that handles persistent chat and intelligent tool execution.
func (agent *GeminiAgent) SendMessageWithTools(ctx context.Context, prompt string, conciseMode bool) (string, error) {

	// Initialize or Re-initialize the chat session if settings change
	if agent.chatSession == nil || agent.isConcise != conciseMode {
		config := agent.getChatConfig(conciseMode)
		chat, err := agent.client.Chats.Create(ctx, agent.currentModel, config, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create chat session: %w", err)
		}
		agent.chatSession = chat
		agent.isConcise = conciseMode
	}

	// Send the user's prompt
	resp, err := agent.chatSession.SendMessage(ctx, genai.Part{Text: prompt})
	if err != nil {
		return "", err
	}

	// Autonomous Execution Loop
	for {
		if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
			break
		}

		var functionResponses []genai.Part
		hasFunctionCall := false

		// 1. Scan the response parts for any function calls
		for _, part := range resp.Candidates[0].Content.Parts {
			if funcCall := part.FunctionCall; funcCall != nil {
				hasFunctionCall = true
				var responseContent string

				// Determine the tool family for the policy engine
				var toolFamily string
				var actionName string
				switch funcCall.Name {
				case "execute_git_command":
					toolFamily = "git"
					if argsAny, ok := funcCall.Args["args"].([]any); ok && len(argsAny) > 0 {
						if subCommand, ok := argsAny[0].(string); ok {
							actionName = subCommand
						}
					}
				case "read_file":
					toolFamily = "fs" // Filesystem operations
					actionName = "read_file"

				case "list_directory":
					toolFamily = "fs" // Filesystem operations
					actionName = "list_dir"
				default:
					toolFamily = "unknown"
				}

				toolCall := types.ToolCall{
					ID:     "genai-internal",
					Tool:   toolFamily,
					Action: actionName,
					Args:   funcCall.Args,
				}
				responseContent, err = agent.mediator.Execute(ctx, toolCall, func(_ context.Context, args map[string]any) (string, error) {
					switch funcCall.Name {
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
					case "list_directory":
						if pathAny, exists := args["path"]; exists {
							if pathStr, ok := pathAny.(string); ok && pathStr != "" {
								agent.lastPath = pathStr
							}
						}
						return agent.registry.ListDirectory(ctx, tools.ListDirectoryArgs{Path: agent.lastPath})
					default:
						return fmt.Sprintf("Error: Unknown function %s called.", funcCall.Name), nil
					}
				})
				if err != nil {
					return "", err
				}

				// 3. Package the tool result
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

		// If no tools were requested, break the loop and return the text answer
		if !hasFunctionCall {
			break
		}

		// 4. Send the tool execution results back to the model for analysis
		fmt.Printf("Handing %d tool result(s) back to %s for final synthesis...\n", len(functionResponses), agent.currentModel)
		resp, err = agent.chatSession.SendMessage(ctx, functionResponses...)
		if err != nil {
			return "", err
		}
	}

	//print(resp.Text())

	return resp.Text(), nil
}

// Helper function to fetch and print available models
func fetchGeminiModels(agent *GeminiAgent, ctx context.Context) {
	fmt.Println("Fetching available models...")
	models, err := agent.ListModels(ctx)
	if err != nil {
		log.Printf("Error listing models: %v\n", err)
	} else {
		for _, m := range models {
			fmt.Println("- " + strings.TrimLeft(m, "models/"))
		}
	}
}

// Lists available models from the API.
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

// SelectModel allows the user to change the active model for generation.
func selectGeminiModel(agent *GeminiAgent, parts []string) {
	if len(parts) < 2 {
		fmt.Println("Usage: /model <model_name>")
	} else {
		agent.SelectModel(parts[1])
	}
}

// Allows user to update the active model and clear session history.
func (agent *GeminiAgent) SelectModel(modelName string) {
	agent.currentModel = modelName
	agent.chatSession = nil // Clear session so it re-initializes with the new model
	fmt.Printf("Model changed to: %s\n", agent.currentModel)
}

func toggleGeminiConciseness(conciseMode *bool) {
	*conciseMode = !*conciseMode
	if *conciseMode {
		fmt.Println("The model will respond in a more direct manner.")
	} else {
		fmt.Println("The model will respond in a more verbose manner.")
	}
}
