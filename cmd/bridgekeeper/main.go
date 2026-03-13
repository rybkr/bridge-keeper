package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"bridgekeeper/internal/audit"
	"bridgekeeper/internal/engine"
	"bridgekeeper/internal/hitl"
	"bridgekeeper/internal/policy"
	"bridgekeeper/internal/runtime"

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
		Name:        "go_version",
		Description: "Get the current version of go",
		Handler:     handleGoVersion,
	},
	{
		Name:        "rust_version",
		Description: "Get the current version of Rust",
		Handler:     handleRustVersion,
	},
}

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

func runGeminiModel(eng *policy.Engine) {
	ctx := context.Background()
	var conciseMode bool = true

	apiKey := loadGeminiAPIKey()

	// Initialize the Gemini Agent
	agent := createDefaultGeminiAgent(ctx, apiKey, eng)

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

		audit.LogEvent("User input received: "+input, audit.Info)

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

// ///// MAIN ///////
func main() {
	audit.LogEvent("This is a test", audit.Debug)

	policyPath := flag.String("policy", "policies", "path to policy YAML file or directory")
	logFile := flag.String("log-file", "", "audit log file path (default: stderr)")
	verbose := flag.Bool("verbose", false, "enable verbose output")
	noHITL := flag.Bool("no-hitl", false, "disable human-in-the-loop approval (auto-approve all)")
	mode := flag.String("mode", "", "mode to run the agent in (ollama or gemini)")
	flag.Parse()

	cfg := engine.Config{
		PolicyDir: *policyPath,
		LogFile:   *logFile,
		Verbose:   *verbose,
	}

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

	// Set up approver.
	var approver engine.Approver
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

	// Create and run engine.
	eng, err := engine.New(cfg, os.Stdin, os.Stdout, auditWriter, approver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := eng.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *policyPath != "" {
		*policyPath = "../../policies/default.yaml"
	}

	pf, err := policy.LoadPath(*policyPath)
	if err != nil {
		log.Printf("\n[WARNING] Could not load policy files: %v", err)
		pf = &policy.PolicyFile{}
	}
	directEngine := policy.NewEngine(pf)

	if *mode == "" {
		fmt.Println("Invalid selection please select Gemini or Ollama with --mode flag.")
		os.Exit(1)
	}

	audit.LogEvent(fmt.Sprintf("System initialized. Entering %s mode.", *mode), audit.Info)

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
			audit.LogEvent("Sending Ollama Prompt: "+prompt, audit.Info)
			response, err := runtime.QueryWithTools(prompt, toolchain, directEngine)
			if nil != err {
				log.Printf("Query error %v", err)
				audit.LogEvent(fmt.Sprintf("Ollama Query Error: %v", err), audit.Error)
				continue // attempt other prompts
			}
			fmt.Printf("< %s\n", response)
		}

	case "gemini", "Gemini":
		/////// GEMINI ///////
		// This actually runs as a chat
		runGeminiModel(directEngine)

	default:
		fmt.Fprintf(os.Stderr, "Usage: %s --mode <ollama|gemini>\n", os.Args[0])
		os.Exit(1)
	}
}
