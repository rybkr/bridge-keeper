package env

import (
	"bridgekeeper/internal/audit"
	"bridgekeeper/internal/policy"
	"bridgekeeper/internal/sandbox"
	"bridgekeeper/internal/session"
	"bridgekeeper/internal/tools"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/joho/godotenv"
)

var Endpoint = ""

var EPolicyPath *string = nil
var EPolicyFile *policy.PolicyFile = nil
var EEngine *policy.Engine = nil
var ESession *session.Session = nil
var SharedLogFile *string = nil
var Auditor *audit.Logger = nil
var EMediator *sandbox.Mediator = nil

var GeminiAPIKey = ""
var ModelContext, CtxCancel = context.WithCancel(context.Background())
var RuntimeRegistry *tools.Registry = nil

var OllamaProc *exec.Cmd
var OllamaSelectedModel = OllamaDefaultModel

var NoHITL = false
var Verbose bool = false
var Concise bool = false
var Thinking bool = false

func SystemPrompt() string {
	prompt := strings.Builder{}
	if Verbose {
		prompt.WriteString("You are a helpful, informative, methodical assistant.\n")
	} else if Concise {
		prompt.WriteString("You are an efficient, concise, and brief assistant.\n")
	} else {
		prompt.WriteString("You are a helpful assistant.\n")
	}
	prompt.WriteString("You must adhere to the following rules:\n")
	prompt.WriteString("- Use Markdown to format responses\n")
	prompt.WriteString("- Code comments should be concise; keep comment density at most 20% of text\n")
	prompt.WriteString("- Always maintain a friendly, conversational, and polite tone\n")
	prompt.WriteString("- Tool calls are enabled. Make as few tool calls as possible to fulfill the request\n")
	if Verbose {
		prompt.WriteString("- Explain your response in great detail\n")
	} else if Concise {
		prompt.WriteString("- Do not explain more than is necessary\n")
		prompt.WriteString("- Be clear and concise\n")
		prompt.WriteString("- Avoid full sentences where comments or notes suffice\n")
		prompt.WriteString("- Use bullet points insead of paragraphs when possible\n")
	}
	return prompt.String()
}

func init() {
	policyPath := flag.String("policy", "policies", "path to policy YAML file or directory")
	logFile := flag.String("log-file", "", "audit log file path (default: stderr)")
	verbose := flag.Bool("verbose", false, "enable verbose output")
	noHITL := flag.Bool("no-hitl", false, "disable human-in-the-loop approval (auto-approve all)")
	mode := flag.String("mode", "", "mode to run the agent in (ollama or gemini)")
	flag.Parse()
	EPolicyPath = policyPath
	SharedLogFile = logFile
	Verbose = *verbose
	NoHITL = *noHITL
	Endpoint = *mode

	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file (using system env vars if available): %v\n", err)
	}

	Endpoint = os.Getenv("DEFAULT_ENDPOINT")
	GeminiAPIKey = os.Getenv("GEMINI_API_KEY")

	if GeminiAPIKey == "" {
		fmt.Println("GEMINI_API_KEY was not found.")
		fmt.Println("  Gemini endpoint will not work.")
		fmt.Println("  Using Ollama.")
		Endpoint = "Ollama"
	}
	if Endpoint == "" {
		fmt.Println("DEFAULT_ENDPOINT was not found.")
		fmt.Println("  Using Ollama.")
		Endpoint = "Ollama"
	}

}
