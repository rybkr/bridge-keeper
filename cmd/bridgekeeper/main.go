package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	bkagent "bridgekeeper/internal/agent"
	"bridgekeeper/internal/audit"
	"bridgekeeper/internal/console"
	"bridgekeeper/internal/hitl"
	"bridgekeeper/internal/policy"
	"bridgekeeper/internal/redact"
	"bridgekeeper/internal/runtime"
	"bridgekeeper/internal/sandbox"
	"bridgekeeper/internal/tools"

	"github.com/joho/godotenv"
)

/////// Toolchain placeholders ///////

// / Deferred shutdown ///
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

func runGeminiModel(mediator *runtime.Mediator, registry *tools.Registry) {
	ctx := context.Background()
	var conciseMode bool = true

	apiKey := loadGeminiAPIKey()

	// Initialize the Gemini Agent
	agent := bkagent.NewGeminiAgent(ctx, apiKey, mediator, registry)
	session, err := console.NewSession(os.Stdin, os.Stdout)
	if err != nil {
		log.Fatal(err)
	}

	printGeminiCommands(agent)

	for {
		input, err := session.ReadLine("> ")
		if err != nil {
			if console.IsInterrupt(err) {
				fmt.Println("\nGoodbye!")
				return
			}
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
			if err := getModelResponse(agent, ctx, input, conciseMode); err != nil {
				if console.IsInterrupt(err) {
					fmt.Println("\nGoodbye!")
					return
				}
				log.Printf("\nError getting response: %v\n", err)
			}
		}
	}
}

func getModelResponse(agent *bkagent.GeminiAgent, ctx context.Context, input string, conciseMode bool) error {
	fmt.Printf("Thinking (%s)...\n", agent.CurrentModel())

	// Uses the new autonomous execution loop
	response, err := agent.SendMessageWithTools(ctx, input, conciseMode)
	if err != nil {
		return err
	}

	fmt.Println("\n(Gemini) - " + response + "\n")
	return nil
}

func printGeminiCommands(agent *bkagent.GeminiAgent) {
	fmt.Println("--- BridgeKeeper Gemini ---")
	fmt.Printf("Current Model: %s\n", agent.CurrentModel())
	fmt.Println("Commands:")
	fmt.Println("  /help          - Show this help message")
	fmt.Println("  /list          - List available models")
	fmt.Println("  /model <name>  - Select a model (e.g., /model gemini-1.5-pro)")
	fmt.Println("  /concise       - Toggle the verboseness of the Model")
	fmt.Println("  <your prompt>  - Chat with the AI (Auto-Tools Enabled)")
	fmt.Println("  /exit          - Quit")
	fmt.Println("-------------------------------")
}

func fetchGeminiModels(agent *bkagent.GeminiAgent, ctx context.Context) {
	fmt.Println("Fetching available models...")
	models, err := agent.ListModels(ctx)
	if err != nil {
		log.Printf("Error listing models: %v\n", err)
		return
	}
	for _, model := range models {
		fmt.Println("- " + bkagent.TrimModelName(model))
	}
}

func selectGeminiModel(agent *bkagent.GeminiAgent, parts []string) {
	if len(parts) < 2 {
		fmt.Println("Usage: /model <model_name>")
		return
	}
	agent.SetModel(parts[1])
	fmt.Printf("Model changed to: %s\n", agent.CurrentModel())
}

func toggleGeminiConciseness(conciseMode *bool) {
	*conciseMode = !*conciseMode
	if *conciseMode {
		fmt.Println("The model will respond in a more direct manner.")
	} else {
		fmt.Println("The model will respond in a more verbose manner.")
	}
}

// ///// MAIN ///////
func main() {
	policyPath := flag.String("policy", "policies", "path to policy YAML file or directory")
	logFile := flag.String("log-file", "", "audit log file path (default: stderr)")
	verbose := flag.Bool("verbose", false, "enable verbose output")
	noHITL := flag.Bool("no-hitl", false, "disable human-in-the-loop approval (auto-approve all)")
	mode := flag.String("mode", "", "mode to run the agent in (ollama or gemini)")
	flag.Parse()

	pf, err := policy.LoadPath(*policyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: loading policy path: %v\n", err)
		os.Exit(1)
	}
	policyEngine := policy.NewEngine(pf)

	// Set up audit log writer.
	var auditWriter *os.File
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot open log file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		auditWriter = f
	} else {
		auditWriter = os.Stderr
	}
	auditLogger := audit.NewLogger(auditWriter, audit.Info)

	workspaceRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine working directory: %v\n", err)
		os.Exit(1)
	}
	workspaceRoot, err = filepath.Abs(workspaceRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot resolve working directory: %v\n", err)
		os.Exit(1)
	}
	validator, err := sandbox.NewValidator(workspaceRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot initialize sandbox validator: %v\n", err)
		os.Exit(1)
	}
	registry := tools.NewRegistry(workspaceRoot, validator)

	// Set up approver.
	var approver runtime.Approver
	if *noHITL {
		approver = &hitl.AutoApprover{}
	} else {
		ta, err := hitl.NewTerminalApprover()
		if err != nil {
			// If we can't open /dev/tty (e.g. in a pipe), fall back to auto-deny.
			fmt.Fprintf(os.Stderr, "warning: cannot open terminal for approval, falling back to auto-deny: %v\n", err)
			approver = &hitl.AutoDenier{}
		} else {
			approver = ta
		}
	}

	// Set up signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()
	mediator := &runtime.Mediator{
		Policy:   policyEngine,
		Approver: approver,
		Audit:    auditLogger,
		Sandbox:  validator,
		Redactor: redact.New(),
	}
	toolchain := runtimeVersionTools(registry)

	if *mode == "" {
		fmt.Println("Invalid selection please select Gemini or Ollama with --mode flag.")
		os.Exit(1)
	}

	auditLogger.Log(audit.Info, "runtime_started", map[string]any{"mode": *mode})
	if *verbose {
		fmt.Fprintf(os.Stderr, "bridgekeeper: workspace root %s\n", workspaceRoot)
	}

	switch *mode {

	case "ollama", "Ollama":
		/////// OLLAMA ///////
		// This just runs through a list of prompts for testing

		if err := runtime.Initialize(11434); nil != err {
			log.Fatalf("Could not initialize: %s", err)
		}

		// Call the anonymous function once main exits scope
		defer deferredShutdown()

		// simple tests - replace with filtered promtps later
		prompts := []string{
			"Get the version of Go",
			"Get the version of rust with Cargo",
		}

		// send the list of prompts
		for _, prompt := range prompts {
			fmt.Printf("\n> %s\n", prompt)
			response, err := runtime.QueryWithTools(ctx, prompt, toolchain, mediator)
			if nil != err {
				log.Printf("Query error %v", err)
				auditLogger.Log(audit.Error, "ollama_query_error", map[string]any{"error": err.Error()})
				continue // attempt other prompts
			}
			fmt.Printf("< %s\n", response)
		}

	case "gemini", "Gemini":
		/////// GEMINI ///////
		// This actually runs as a chat
		runGeminiModel(mediator, registry)

	default:
		fmt.Fprintf(os.Stderr, "Usage: %s --mode <ollama|gemini>\n", os.Args[0])
		os.Exit(1)
	}
}

func runtimeVersionTools(registry *tools.Registry) []runtime.ToolDef {
	return []runtime.ToolDef{
		{
			Name:        "go_version",
			Tool:        "pkg",
			Action:      "list",
			Description: "Get the current version of Go.",
			Handler: func(ctx context.Context, _ map[string]any) (string, error) {
				return registry.GoVersion(ctx)
			},
		},
		{
			Name:        "rust_version",
			Tool:        "pkg",
			Action:      "list",
			Description: "Get the current version of Rust.",
			Handler: func(ctx context.Context, _ map[string]any) (string, error) {
				return registry.RustVersion(ctx)
			},
		},
	}
}
