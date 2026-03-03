package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func loadGeminiAPIKey() string {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: Error loading .env file (using system env vars if available): %v\n", err)
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY is not set.")
	}
	return apiKey
}

func runGeminiModel() {
	ctx := context.Background()
	var conciseMode bool = true

	apiKey := loadGeminiAPIKey()

	// Initialize the Gemini Agent
	agent := createDefaultGeminiAgent(ctx, apiKey)

	// Start the CLI interactive loop
	reader := bufio.NewReader(os.Stdin)
	printGeminiCommands(agent)

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
				fetchGeminiModels(agent, ctx)

			case "/model":
				selectGeminiModel(agent, parts)

			case "/concise":
				toggleGeminiConciseness(&conciseMode)

			case "/help":
				printGeminiCommands(agent)

			default:
				fmt.Println("Unknown command. Try /help to list commands.")
			}

		} else {
			getModelResponse(agent, ctx, input, conciseMode)
		}
	}
}

func getModelResponse(agent *GeminiAgent, ctx context.Context, input string, conciseMode bool) {
	fmt.Printf("Thinking (%s)...\n", agent.currentModel)

	// Uses the new autonomous execution loop
	response, err := agent.SendMessageWithTools(ctx, input, conciseMode)
	if err != nil {
		log.Printf("\nError getting response: %v\n", err)
		return
	}

	fmt.Println("\n(Gemini) - " + response + "\n")
}

func main() {
	runGeminiModel()
}
