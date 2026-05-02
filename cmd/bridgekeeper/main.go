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

func loadEnvFile() {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: Error loading .env file (using system env vars if available): %v\n", err)
	}
}

func loadGeminiAPIKey() string {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY is not set.")
	}
	return apiKey
}

func resolveOllamaModel(flagValue string) string {
	if model := strings.TrimSpace(flagValue); model != "" {
		return model
	}
	if model := strings.TrimSpace(os.Getenv("OLLAMA_MODEL")); model != "" {
		return model
	}
	return runtime.DefaultOllamaModel
}

func runGeminiModel(mediator *runtime.Mediator, registry *tools.Registry, pf *policy.PolicyFile) {
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

			case "/policy":
				fmt.Println(policy.FormatPolicy(pf))

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

func runOllamaModel(ctx context.Context, mediator *runtime.Mediator, registry *tools.Registry, pf *policy.PolicyFile) {
	session, err := console.NewSession(os.Stdin, os.Stdout)
	if err != nil {
		log.Fatal(err)
	}

	chat := newOllamaChat(mediator, registry)
	printOllamaCommands()

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

		if strings.HasPrefix(input, "/") {
			parts := strings.Fields(input)
			command := parts[0]

			switch command {
			case "/exit", "/quit":
				fmt.Println("Goodbye!")
				return

			case "/model":
				if selectOllamaModel(parts) {
					chat = newOllamaChat(mediator, registry)
				}

			case "/policy":
				fmt.Println(policy.FormatPolicy(pf))

			case "/help":
				printOllamaCommands()

			default:
				fmt.Println("Unknown command. Try /help to list commands.")
			}
		} else {
			if err := getOllamaResponse(ctx, chat, input); err != nil {
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

func getOllamaResponse(ctx context.Context, chat *runtime.OllamaChat, input string) error {
	fmt.Printf("Thinking (%s)...\n", runtime.CurrentOllamaModel())

	response, err := chat.SendMessageWithTools(ctx, input)
	if err != nil {
		return err
	}

	fmt.Println("\n(Ollama) - " + response + "\n")
	return nil
}

func printGeminiCommands(agent *bkagent.GeminiAgent) {
	fmt.Println("--- BridgeKeeper Gemini ---")
	fmt.Printf("Current Model: %s\n", agent.CurrentModel())
	fmt.Println("Commands:")
	fmt.Println("  /help          - Show this help message")
	fmt.Println("  /list          - List available models")
	fmt.Println("  /model <name>  - Select a model (e.g., /model gemini-1.5-pro)")
	fmt.Println("  /policy        - Show the current loaded policy")
	fmt.Println("  /concise       - Toggle the verboseness of the Model")
	fmt.Println("  <your prompt>  - Chat with the AI (Auto-Tools Enabled)")
	fmt.Println("  /exit          - Quit")
	fmt.Println("-------------------------------")
}

func printOllamaCommands() {
	fmt.Println("--- BridgeKeeper Ollama ---")
	fmt.Printf("Current Model: %s\n", runtime.CurrentOllamaModel())
	fmt.Println("Commands:")
	fmt.Println("  /help          - Show this help message")
	fmt.Println("  /model <name>  - Select the Ollama model")
	fmt.Println("  /policy        - Show the current loaded policy")
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

func selectOllamaModel(parts []string) bool {
	if len(parts) < 2 {
		fmt.Println("Usage: /model <model_name>")
		return false
	}
	runtime.SetOllamaModel(parts[1])
	fmt.Printf("Model changed to: %s\n", runtime.CurrentOllamaModel())
	return true
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
	ollamaModel := flag.String("ollama-model", "", "Ollama model name (overrides OLLAMA_MODEL)")
	flag.Parse()

	loadEnvFile()

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
		runtime.SetOllamaModel(resolveOllamaModel(*ollamaModel))

		if err := runtime.Initialize(11434); nil != err {
			log.Fatalf("Could not initialize: %s", err)
		}

		// Call the anonymous function once main exits scope
		defer deferredShutdown()

		runOllamaModel(ctx, mediator, registry, pf)

	case "gemini", "Gemini":
		/////// GEMINI ///////
		// This actually runs as a chat
		runGeminiModel(mediator, registry, pf)

	default:
		fmt.Fprintf(os.Stderr, "Usage: %s --mode <ollama|gemini>\n", os.Args[0])
		os.Exit(1)
	}
}

func newOllamaChat(mediator *runtime.Mediator, registry *tools.Registry) *runtime.OllamaChat {
	return runtime.NewOllamaChat(ollamaToolchain(registry), mediator)
}

func ollamaGitAction(args map[string]any) string {
	argsAny, ok := args["args"].([]any)
	if !ok || len(argsAny) == 0 {
		return ""
	}
	subCommand, ok := argsAny[0].(string)
	if !ok {
		return ""
	}
	return subCommand
}

func requiredStringArg(args map[string]any, key string) (string, bool) {
	raw, ok := args[key]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

func optionalStringArg(args map[string]any, key string) string {
	value, _ := requiredStringArg(args, key)
	return value
}

func packageToolProperties() map[string]runtime.ToolProperty {
	return map[string]runtime.ToolProperty{
		"manager": {
			Type:       "string",
			Descrption: "The package manager to use: go or cargo. If omitted, Bridgekeeper detects go.mod or Cargo.toml.",
		},
		"package": {
			Type:       "string",
			Descrption: "The package, module, or crate name.",
		},
		"version": {
			Type:       "string",
			Descrption: "Optional version for dependency installation.",
		},
		"path": {
			Type:       "string",
			Descrption: "The project directory. Defaults to the workspace root.",
		},
	}
}

func ollamaToolchain(registry *tools.Registry) []runtime.ToolDef {
	lastPath := registry.WorkspaceRoot

	return []runtime.ToolDef{
		{
			Name:           "execute_git_command",
			Tool:           "git",
			ActionFromArgs: ollamaGitAction,
			Description:    "Executes a git command in a local repository. Use this to check status, view logs, examine diffs, etc. Only provide the arguments, not the git binary itself.",
			Parameters: map[string]runtime.ToolProperty{
				"args": {
					Type:       "array",
					Descrption: "A list of strings representing the git arguments, for example ['log', '-n', '3'].",
					Items:      &runtime.ToolProperty{Type: "string"},
				},
				"path": {
					Type:       "string",
					Descrption: "The directory path of the git repository. If omitted, the agent will use the previously accessed repository.",
				},
			},
			Required: []string{"args"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				if pathAny, exists := args["path"]; exists {
					if pathStr, ok := pathAny.(string); ok && pathStr != "" {
						lastPath = pathStr
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
				return registry.ExecuteGitCommand(ctx, tools.GitExecArgs{Path: lastPath, Args: gitArgs})
			},
		},
		{
			Name:        "read_file",
			Tool:        "fs",
			Action:      "read_file",
			Description: "Reads the full contents of a local file. Use this to analyze, summarize, or reference specific parts of a file. Provide the path to the file.",
			Parameters: map[string]runtime.ToolProperty{
				"path": {
					Type:       "string",
					Descrption: "The absolute or relative path to the file to read.",
				},
			},
			Required: []string{"path"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				pathAny, exists := args["path"]
				if !exists {
					return "Error: model failed to provide file path.", nil
				}
				pathStr, ok := pathAny.(string)
				if !ok || pathStr == "" {
					return "Error: path argument is invalid or empty.", nil
				}
				return registry.ReadFile(ctx, tools.ReadFileArgs{Path: pathStr})
			},
		},
		{
			Name:        "write_file",
			Tool:        "fs",
			Action:      "write_file",
			Description: "Writes text content to a local file. Use this only when the user explicitly wants to create or update a file.",
			Parameters: map[string]runtime.ToolProperty{
				"path": {
					Type:       "string",
					Descrption: "The absolute or relative path to the file to write.",
				},
				"content": {
					Type:       "string",
					Descrption: "The text content to write to the file.",
				},
			},
			Required: []string{"path", "content"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				pathAny, hasPath := args["path"]
				contentAny, hasContent := args["content"]
				if !hasPath || !hasContent {
					return "Error: model failed to provide file path or content.", nil
				}
				pathStr, ok := pathAny.(string)
				if !ok || pathStr == "" {
					return "Error: path argument is invalid or empty.", nil
				}
				contentStr, ok := contentAny.(string)
				if !ok {
					return "Error: content argument must be a string.", nil
				}
				return registry.WriteFile(ctx, tools.WriteFileArgs{Path: pathStr, Content: contentStr})
			},
		},
		{
			Name:        "http_get",
			Tool:        "http",
			Action:      "get",
			Description: "Fetches the contents of an HTTP or HTTPS URL. Use this for read-only network retrieval.",
			Parameters: map[string]runtime.ToolProperty{
				"url": {
					Type:       "string",
					Descrption: "The HTTP or HTTPS URL to fetch.",
				},
			},
			Required: []string{"url"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				urlAny, hasURL := args["url"]
				if !hasURL {
					return "Error: model failed to provide a URL.", nil
				}
				urlStr, ok := urlAny.(string)
				if !ok || urlStr == "" {
					return "Error: url argument is invalid or empty.", nil
				}
				return registry.HTTPGet(ctx, tools.HTTPGetArgs{URL: urlStr})
			},
		},
		{
			Name:        "http_post",
			Tool:        "http",
			Action:      "post",
			Description: "Sends a bounded HTTP POST request to an HTTP or HTTPS URL.",
			Parameters: map[string]runtime.ToolProperty{
				"url": {
					Type:       "string",
					Descrption: "The HTTP or HTTPS URL to send the request to.",
				},
				"body": {
					Type:       "string",
					Descrption: "The request body to send.",
				},
				"content_type": {
					Type:       "string",
					Descrption: "The request content type. Defaults to application/json.",
				},
			},
			Required: []string{"url", "body"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				urlStr, ok := requiredStringArg(args, "url")
				if !ok {
					return "Error: url argument is invalid or empty.", nil
				}
				bodyStr, ok := requiredStringArg(args, "body")
				if !ok {
					return "Error: body argument is invalid or empty.", nil
				}
				return registry.HTTPPost(ctx, tools.HTTPPostArgs{
					URL:         urlStr,
					Body:        bodyStr,
					ContentType: optionalStringArg(args, "content_type"),
				})
			},
		},
		{
			Name:        "shell_exec",
			Tool:        "shell",
			Action:      "exec",
			Description: "Runs a simple allowlisted local command without shell metacharacters.",
			Parameters: map[string]runtime.ToolProperty{
				"command": {
					Type:       "string",
					Descrption: "The exact command to run, for example 'ls .' or 'wc README.md'.",
				},
				"path": {
					Type:       "string",
					Descrption: "The workspace directory to run the command in.",
				},
			},
			Required: []string{"command"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				command, ok := requiredStringArg(args, "command")
				if !ok {
					return "Error: command argument is invalid or empty.", nil
				}
				return registry.ExecuteShellCommand(ctx, tools.ShellExecArgs{
					Command: command,
					Path:    optionalStringArg(args, "path"),
				})
			},
		},
		{
			Name:        "pkg_list",
			Tool:        "pkg",
			Action:      "list",
			Description: "Lists dependencies for a Go module or Cargo project.",
			Parameters:  packageToolProperties(),
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				return registry.PackageList(ctx, tools.PackageListArgs{
					Manager: optionalStringArg(args, "manager"),
					Path:    optionalStringArg(args, "path"),
				})
			},
		},
		{
			Name:        "pkg_query",
			Tool:        "pkg",
			Action:      "query",
			Description: "Queries available versions or registry information for a package.",
			Parameters:  packageToolProperties(),
			Required:    []string{"package"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				pkg, ok := requiredStringArg(args, "package")
				if !ok {
					return "Error: package argument is invalid or empty.", nil
				}
				return registry.PackageQuery(ctx, tools.PackageQueryArgs{
					Manager: optionalStringArg(args, "manager"),
					Package: pkg,
					Path:    optionalStringArg(args, "path"),
				})
			},
		},
		{
			Name:        "pkg_install",
			Tool:        "pkg",
			Action:      "install",
			Description: "Adds or updates a dependency in a Go module or Cargo project.",
			Parameters:  packageToolProperties(),
			Required:    []string{"package"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				pkg, ok := requiredStringArg(args, "package")
				if !ok {
					return "Error: package argument is invalid or empty.", nil
				}
				return registry.PackageInstall(ctx, tools.PackageInstallArgs{
					Manager: optionalStringArg(args, "manager"),
					Package: pkg,
					Version: optionalStringArg(args, "version"),
					Path:    optionalStringArg(args, "path"),
				})
			},
		},
		{
			Name:        "pkg_update",
			Tool:        "pkg",
			Action:      "update",
			Description: "Updates dependencies in a Go module or Cargo project.",
			Parameters:  packageToolProperties(),
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				return registry.PackageUpdate(ctx, tools.PackageUpdateArgs{
					Manager: optionalStringArg(args, "manager"),
					Package: optionalStringArg(args, "package"),
					Path:    optionalStringArg(args, "path"),
				})
			},
		},
		{
			Name:        "list_directory",
			Tool:        "fs",
			Action:      "list_dir",
			Description: "Lists the contents of a specified directory. Use this to explore the repository structure, find files, or check for the presence of specific items.",
			Parameters: map[string]runtime.ToolProperty{
				"path": {
					Type:       "string",
					Descrption: "The path to the directory to list.",
				},
			},
			Required: []string{"path"},
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				if pathAny, exists := args["path"]; exists {
					if pathStr, ok := pathAny.(string); ok && pathStr != "" {
						lastPath = pathStr
					}
				}
				return registry.ListDirectory(ctx, tools.ListDirectoryArgs{Path: lastPath})
			},
		},
	}
}
