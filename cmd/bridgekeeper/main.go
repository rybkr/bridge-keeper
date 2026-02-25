package main

import (
	"bufio"
	"context"
	"fmt"
	"iter"
	"log"
	"math/rand/v2"
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
func (agent *GeminiAgent) GenerateStream(ctx context.Context, prompt string, toggle bool) iter.Seq2[*genai.GenerateContentResponse, error] {
	// Simple text-based generation. For chat history, we would need to maintain a history buffer.
	// Define the generation configuration
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

func printCommands(agent *GeminiAgent) {
	fmt.Println("--- BridgeKeeper Gemini ---")
	fmt.Printf("Current Model: %s\n", agent.currentModel)
	fmt.Println("Commands:")
	fmt.Println("  /help          - Show this help message")
	fmt.Println("  /list          - List available models")
	fmt.Println("  /model <name>  - Select a model (e.g., /model gemini-1.5-pro)")
	fmt.Println("  /concise       - Toggle the verboseness of the Model")
	fmt.Println("  <your prompt>  - Chat with the AI")
	fmt.Println("  /exit          - Quit")
	fmt.Println("-------------------------------")
}

func main() {
	ctx := context.Background()
	var err error
	var conciseMode bool = false

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
	printCommands(agent)

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

			case "/concise":
				conciseMode = !conciseMode
				if conciseMode {
					fmt.Println("The model will respond in a more direct manner.")
				} else {
					fmt.Println("The model will respond in a more verbose manner.")
				}

			case "/help":
				printCommands(agent)

			default:
				fmt.Println("Unknown command. Try /help to list commands.")
			}

		} else {
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
	}
}
