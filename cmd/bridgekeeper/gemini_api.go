package main

import (
	"context"
	"fmt"
	"iter"
	"log"
	"math/rand/v2"
	"strings"

	"bridgekeeper/internal/tools"

	"google.golang.org/genai"
)

// GeminiAgent serves as the API layer for interaction with the service.
type GeminiAgent struct {
	client       *genai.Client
	currentModel string
}

// createDefaultGeminiAgent initializes the agent with a default model.
func createDefaultGeminiAgent(ctx context.Context, apiKey string) *GeminiAgent {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})

	if err != nil {
		log.Fatal(err)
	}

	// 3. Initialize our Agent API
	agent := GeminiAgent{
		client:       client,
		currentModel: "gemini-2.5-flash-lite",
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
	fmt.Println("  /git           - Git Agent Mode (Execute repository commands)")
	fmt.Println("  /concise       - Toggle the verboseness of the Model")
	fmt.Println("  <your prompt>  - Chat with the AI")
	fmt.Println("  /exit          - Quit")
	fmt.Println("-------------------------------")
}

// Endpoint 1: GenerateResponse (Chat)
// Accepts user input and feeds it to the selected model.
func (agent *GeminiAgent) GenerateStream(ctx context.Context, prompt string, toggle bool) iter.Seq2[*genai.GenerateContentResponse, error] {
	var config *genai.GenerateContentConfig

	if toggle {
		config = &genai.GenerateContentConfig{
			// 1. System Instruction: The strongest way to enforce a concise style
			SystemInstruction: &genai.Content{
				Role: "model",
				Parts: []*genai.Part{
					{Text: "You are a highly efficient assistant. Always provide extremely concise, direct, and brief answers. Omit unnecessary pleasantries, filler words, or long explanations unless explicitly asked."},
				},
			},
			Temperature: genai.Ptr(rand.Float32() * 0.5), // Lower temperature for more deterministic and concise responses
		}
	} else {
		config = &genai.GenerateContentConfig{
			// 1. System Instruction: The strongest way to enforce a verbose style
			SystemInstruction: &genai.Content{
				Role: "model",
				Parts: []*genai.Part{
					{Text: "You are a verbose assistant. Always provide detailed, comprehensive, and thorough answers. Include all relevant information and context unless explicitly asked to be concise."},
				},
			},
			Temperature: genai.Ptr(1.0 + rand.Float32()), // Higher temperature for more creative and verbose responses
		}
	}

	resp := agent.client.Models.GenerateContentStream(
		ctx,
		agent.currentModel,
		genai.Text(prompt),
		config,
	)

	return resp
}

// Endpoint 2: ListModels
// Helper function to fetch and print available models
func fetchGeminiModels(agent *GeminiAgent, ctx context.Context) {
	fmt.Println("Fetching available models...")
	models, err := agent.ListModels(ctx)
	if err != nil {
		log.Printf("Error listing models: %v\n", err)
	} else {
		for _, m := range models {
			fmt.Println("- " + m)
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

// Endpoint 3: SelectModel
// Allows user to update the active model.
func (agent *GeminiAgent) SelectModel(modelName string) {
	agent.currentModel = modelName
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

func getModelResponse(agent *GeminiAgent, ctx context.Context, input string, conciseMode bool) {
	// Call Endpoint 1 & 4 (Process Input -> Output)
	fmt.Printf("Thinking (%s)...\n", agent.currentModel)
	responses := agent.GenerateStream(ctx, input, conciseMode)

	fmt.Print("(Gemini) - ")
	for chunk, err := range responses {
		if err != nil {
			log.Printf("\nError reading stream: %v\n", err)
			break
		}

		// Print each chunk directly to the console without a newline
		fmt.Print(chunk.Text())
	}
	fmt.Println()
}

func gitEndPoint(agent *GeminiAgent, ctx context.Context, prompt string, defaultRepoPath string) {
	// Trigger the Git Agent Mode
	gitInput := strings.TrimSpace(prompt)

	// Assuming the target repository is the current directory
	if defaultRepoPath == "" {
		defaultRepoPath = "./"
	}
	response, err := agent.AnalyzeWithGit(ctx, gitInput, defaultRepoPath)
	if err != nil {
		log.Printf("Error analyzing repository: %v\n", err)
	} else {
		fmt.Println("\n(Gemini) - " + response)
	}
}

// Endpoint: AnalyzeWithGit
// Enables function calling specifically for Git operations
func (agent *GeminiAgent) AnalyzeWithGit(ctx context.Context, prompt string, defaultRepoPath string) (string, error) {
	gitTool := &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "execute_git_command",
				Description: "Executes a git command in a local repository. Only provide the arguments, not the 'git' binary itself.",
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
					},
					Required: []string{"args"},
				},
			},
		},
	}

	config := &genai.GenerateContentConfig{
		Tools: []*genai.Tool{gitTool},
	}

	// Initialize the chat session
	chat, err := agent.client.Chats.Create(ctx, agent.currentModel, config, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create chat session: %w", err)
	}

	// Send the user's prompt
	resp, err := chat.SendMessage(ctx, genai.Part{Text: prompt})
	if err != nil {
		return "", err
	}

	// Check if the model decided to call our function
	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		part := resp.Candidates[0].Content.Parts[0]

		// Check if this part is a function call
		if part.FunctionCall != nil && part.FunctionCall.Name == "execute_git_command" {
			funcCall := part.FunctionCall

			// Extract arguments safely
			argsAny, exists := funcCall.Args["args"].([]any)
			if !exists {
				return "", fmt.Errorf("model failed to provide git arguments")
			}

			var gitArgs []string
			for _, arg := range argsAny {
				if strArg, ok := arg.(string); ok {
					gitArgs = append(gitArgs, strArg)
				}
			}

			// Execute the local Git command
			gitOutput := tools.ExecuteGitCommand(defaultRepoPath, gitArgs)

			// Package terminal output back to the model
			funcResponse := genai.FunctionResponse{
				Name: funcCall.Name,
				Response: map[string]any{
					"terminal_output": gitOutput,
				},
			}

			fmt.Printf("Handing terminal output back to %s for analysis...\n", agent.currentModel)
			finalResp, err := chat.SendMessage(ctx, genai.Part{FunctionResponse: &funcResponse})
			if err != nil {
				return "", err
			}

			return finalResp.Text(), nil
		}
	}

	// Return standard text response if no function was called
	return resp.Text(), nil
}
