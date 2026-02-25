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
	var conciseMode bool = false

	apiKey := loadGeminiAPIKey()

	// 3. Initialize the Gemini Agent
	agent := createDefaultGeminiAgent(ctx, apiKey)

	// 4. Start the CLI interactive loop
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
				// Call Endpoint 2
				fetchGeminiModels(agent, ctx)

			case "/model":
				// Call Endpoint 3
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

func main() {
	runGeminiModel()
}
