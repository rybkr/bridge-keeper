package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"google.golang.org/genai"
)

// GeminiAgent serves as the API layer for interaction with the service.
type GeminiAgent struct {
	client       *genai.Client
	currentModel string
}

// NewGeminiAgent initializes the agent with a default model.
func NewGeminiAgent(client *genai.Client) *GeminiAgent {
	return &GeminiAgent{
		client:       client,
		currentModel: "gemini-2.5-flash-lite", // Default fallback
	}
}

// Endpoint 1: GenerateResponse (Chat)
// Accepts user input and feeds it to the selected model.
func (agent *GeminiAgent) GenerateResponse(ctx context.Context, prompt string) (string, error) {
	// Simple text-based generation. For chat history, we would need to maintain a history buffer.
	resp, err := agent.client.Models.GenerateContent(
		ctx,
		agent.currentModel,
		genai.Text(prompt),
		nil,
	)
	if err != nil {
		return "", err
	}

	return resp.Text(), nil
}

// Endpoint 2: ListModels
// Lists available models from the API.
func (agent *GeminiAgent) ListModels(ctx context.Context) ([]string, error) {
	var modelNames []string

	// Note: The Google GenAI library might have different implementations for List()
	// depending on the version. Assuming standard "client.Models.List(ctx, nil)".
	models, err := agent.client.Models.List(ctx, &genai.ListModelsConfig{})
	if err != nil {
		return nil, err
	}

	for _, m := range models.Items {
		modelNames = append(modelNames, m.Name)
	}

	return modelNames, nil
}

// Endpoint 3: SelectModel
// Allows user to update the active model.
func (agent *GeminiAgent) SelectModel(modelName string) {
	agent.currentModel = modelName
	fmt.Printf("Model changed to: %s\n", agent.currentModel)
}

func main() {
	ctx := context.Background()
	var err error

	// 1. Load configuration
	err = godotenv.Load()
	if err != nil {
		log.Printf("Warning: Error loading .env file (using system env vars if available): %v\n", err)
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY is not set.")
	}

	// 2. Initialize the client
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		log.Fatal(err)
	}

	// 3. Initialize our Agent API
	agent := NewGeminiAgent(client)

	// 4. Start the CLI interactive loop
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("--- Simple Gemini Agent CLI ---")
	fmt.Printf("Current Model: %s\n", agent.currentModel)
	fmt.Println("Commands:")
	fmt.Println("  /help          - Show this help message")
	fmt.Println("  /list          - List available models")
	fmt.Println("  /model <name>  - Select a model (e.g., /model gemini-1.5-pro)")
	fmt.Println("  <your prompt>  - Chat with the AI")
	fmt.Println("  /exit          - Quit")
	fmt.Println("-------------------------------")

	for {
		fmt.Print("> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Handle commands vs. prompts
		if strings.HasPrefix(input, "/") {
			parts := strings.Fields(input)
			command := parts[0]

			switch command {
			case "/exit", "/quit":
				fmt.Println("Goodbye!")
				return

			case "/list":
				// Call Endpoint 2
				fmt.Println("Fetching available models...")
				models, err := agent.ListModels(ctx)
				if err != nil {
					log.Printf("Error listing models: %v\n", err)
				} else {
					for _, m := range models {
						fmt.Println("- " + m)
					}
				}

			case "/model":
				// Call Endpoint 3
				if len(parts) < 2 {
					fmt.Println("Usage: /model <model_name>")
				} else {
					agent.SelectModel(parts[1])
				}

			case "/help":
				fmt.Println("Commands:")
				fmt.Println("  /help          - Show this help message")
				fmt.Println("  /list          - List available models")
				fmt.Println("  /model <name>  - Select a model (e.g., /model gemini-1.5-pro)")
				fmt.Println("  <your prompt>  - Chat with the AI")
				fmt.Println("  /exit          - Quit")

			default:
				fmt.Println("Unknown command. Try /help, /list, /model, or /exit.")
			}

		} else {
			// Call Endpoint 1 & 4 (Process Input -> Output)
			fmt.Printf("Thinking (%s)...\n", agent.currentModel)
			response, err := agent.GenerateResponse(ctx, input)
			if err != nil {
				log.Printf("Error generating response: %v\n", err)
			} else {
				fmt.Println("Response:")
				fmt.Println(response)
			}
		}
	}
}
