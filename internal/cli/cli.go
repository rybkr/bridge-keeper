package cli

import (
	"bridgekeeper/internal/audit"
	"bridgekeeper/internal/policy"
	"bridgekeeper/internal/runtime"
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
)

func CLILoop(eng *policy.Engine, mode string) error {
	ctx := context.Background()
	var conciseMode bool = true

	var modelstring string
	var apiKey string
	var geminiAgent *runtime.GeminiAgent

	var whichModel bool

	switch mode {
	case "ollama", "Ollama":
		modelstring = fmt.Sprintf("Ollama %s", runtime.OllamaSelectedModel)
		if err := runtime.OllamaInitialize(); nil != err {
			return fmt.Errorf("Could not initialize: %s", err)
		}
		defer runtime.OllamaDeferShutdown()
		whichModel = false
	case "gemini", "Gemini":
		apiKey = runtime.LoadGeminiAPIKey()
		geminiAgent = runtime.CreateDefaultGeminiAgent(ctx, apiKey, eng)
		modelstring = fmt.Sprintf("%s", geminiAgent.CurrentModel)
		whichModel = true
	default:
		audit.LogEvent("Unknown mode selected.", audit.Error)
		os.Exit(1)
	}

	// Start the CLI interactive loop
	reader := bufio.NewReader(os.Stdin)

	for {

		// Print the commands
		fmt.Println("--- BridgeKeeper ---")
		fmt.Printf("Current Model: %s\n", modelstring)
		fmt.Println("Commands:")
		fmt.Println("  /list          - List available models")
		fmt.Println("  /model <name>  - Select a model (e.g., /model gemini-1.5-pro)")
		fmt.Println("  /concise       - Toggle the verboseness of the Model (Gemini only)")
		fmt.Println("  <your prompt>  - Chat with the AI (Auto-Tools Enabled)")
		fmt.Println("  /exit          - Quit")
		fmt.Println("-------------------------------")

		fmt.Print("> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		audit.LogEvent("User input received: "+input, audit.Info)

		// Handle commands vs. prompts
		if strings.HasPrefix(input, "/") {
			parts := strings.Fields(input)
			command := parts[0]

			switch command {
			case "/exit", "/quit":
				fmt.Println("Goodbye!")
				return nil

			case "/list":
				if whichModel {
					runtime.FetchGeminiModels(geminiAgent, ctx)
				} else {
					runtime.OllamaLS()
				}

			case "/model":
				if whichModel {
					runtime.SelectGeminiModel(geminiAgent, parts)
				} else {
					runtime.OllamaSelectModel(parts)
				}
				switch mode {
				case "ollama", "Ollama":
					modelstring = fmt.Sprintf("Ollama %s", runtime.OllamaSelectedModel)
				case "gemini", "Gemini":
					modelstring = fmt.Sprintf("%s", geminiAgent.CurrentModel)
				}

			case "/concise":
				runtime.ToggleGeminiConciseness(&conciseMode)

			default:
				fmt.Println("Unknown command. Try /help to list commands.")
			}

		} else if whichModel {
			geminiAgent.SendMessageWithTools(ctx, input, conciseMode)
		} else {
			runtime.SendOllamaMessage(input, eng, ctx)
		}

	}

}

func getModelResponse(agent *runtime.GeminiAgent, ctx context.Context, input string, conciseMode bool) {
	fmt.Printf("Thinking (%s)...\n", agent.CurrentModel)

	// Uses the new autonomous execution loop
	response, err := agent.SendMessageWithTools(ctx, input, conciseMode)
	if err != nil {
		log.Printf("\nError getting response: %v\n", err)
		return
	}

	fmt.Println("\n(Gemini) - " + response + "\n")
}
