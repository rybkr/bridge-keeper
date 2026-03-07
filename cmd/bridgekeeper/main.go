package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"bridgekeeper/internal/runtime"
	"os"
	"strings"
	"github.com/joho/godotenv"
)


/////// Toolchain placeholders ///////

func handleGoVersion(args map[string]any) (string, error) {
    // TODO: Replace with better result structs
    // or exec or something
    return "go version go1.25.7 linux/amd64", nil
}

func handleRustVersion(args map[string]any) (string, error) {
    // TODO: Replace with better result structs
    // or exec or something
    return "cargo 1.65.0", nil
}

var toolchain = []runtime.ToolDef{
    {
        Name: "go_version",
        Description: "Get the current version of go",
        Handler: handleGoVersion,
    },
        {
        Name: "rust_version",
        Description: "Get the current version of Rust",
        Handler: handleRustVersion,
    },
}

/// Deferred shutdown ///
func deferredShutdown() {
    if err := runtime.Shutdown(); nil != err {
        log.Printf("shutdown %v", err)
    }
}


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
	/*pf, err := policy.LoadPath("policies")
	if err != nil {
		log.Printf("\n[WARNING] Could not load policy files from 'policies': %v", err)
		log.Printf("[WARNING] Engine will start with an empty policy (default deny behavior may apply).\n\n")
		pf = &policy.PolicyFile{}
	}
	engine := policy.NewEngine(pf)*/

	// Initialize the Gemini Agent
	agent := createDefaultGeminiAgent(ctx, apiKey /*, engine*/)

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


/////// MAIN ///////
func main() {

    // Parse --mode flag
    mode := ""
    args := os.Args[1:]
    for i, arg := range args {
        if arg == "--mode" && i+1 < len(args) {
            mode = args[i+1]
            break
        }
    }

    switch mode {

    case "ollama":
        /////// OLLAMA ///////
        // This just runs through a list of prompts for testing

        if err := runtime.Initialize(11434); nil != err {
            log.Fatalf("Could not initialize: %w", err)
        }

        // Call the anonymous function once main exits scope
        defer deferredShutdown()

        // simple tests - replace with filtered promtps later
        prompts := []string {
            "Get the version of Go",
            "Get the version of rust with Cargo",
        }

        // send the list of prompts
        for _, prompt := range prompts {
            fmt.Printf("\n> %s\n", prompt)
            response, err := runtime.QueryWithTools(prompt, toolchain)
            if nil != err {
                log.Printf("Query error %v", err)
                continue // attempt other prompts
            }
            fmt.Printf("< %s\n", response)
        }

    case "gemini":
        /////// GEMINI ///////
        // This actually runs as a chat
        runGeminiModel()

    default:
        fmt.Fprintf(os.Stderr, "Usage: %s --mode <ollama|gemini>\n", os.Args[0])
        os.Exit(1)
    }
}
