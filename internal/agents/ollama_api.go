package agents

import (
	"bridgekeeper/internal/audit"
	"bridgekeeper/internal/env"
	"bridgekeeper/internal/policy"
	"bridgekeeper/internal/tools"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

var OllamaConversation []Message = []Message{
	{
		Role:    "user",
		Content: env.SystemPrompt(),
	},
}

func NewOllamaAgent() error {

	// Ollama is already running and is OK to query
	if ollamaLocalServerStatus(env.OllamaBaseURL) {
		return nil
	}

	// else: Ollama not running or not ok. Try spinning up
	cmd := exec.Command("ollama", "serve")
	cmd.Env = append(cmd.Environ(), "OLLAMA_HOST=localhost:11434")

	if err := cmd.Start(); nil != err {
		return error(fmt.Errorf("Failed to start up Ollama: %w", err))
	}

	defer OllamaShutdown()

	// spin up ok, now check it's ready
	env.OllamaProc = cmd
	deadline := time.Now().Add(15 * time.Second) // set timeout 15 seconds
	for time.Now().Before(deadline) {
		// Check again if we can contact the ollama local server
		if ollamaLocalServerStatus(env.OllamaBaseURL) {
			return nil
		}
		time.Sleep(150 * time.Millisecond) // if not, sleep and wait
	}
	_ = cmd.Process.Kill()
	env.OllamaProc = nil
	return error(fmt.Errorf("Failed to start up Ollama: timeout (15s)"))
}

func SendOllamaMessage(userPrompt string) (string, error) {
	OllamaConversation = append(OllamaConversation, Message{Role: "user", Content: userPrompt})
	requestBody := OllamaAPIRequest{
		Model:     env.OllamaSelectedModel,
		Messages:  OllamaConversation,
		Stream:    false,
		Think:     env.Thinking,
		KeepAlive: env.OllamaKeepAliveDuration,
		Tools:     OllamaTools,
	}
	byteData, err := json.Marshal(requestBody)

	if nil != err {
		return "", err
	}

	// the actual post call to the server
	apiurl := env.OllamaBaseURL + "/api/chat"
	rspPtr, err := http.Post(apiurl, "application/json", bytes.NewReader(byteData))
	// Someone online said this is better for cleanup
	defer (*rspPtr).Body.Close()
	response := *rspPtr
	if nil != err {
		return "", err
	}

	// Check the response
	if response.StatusCode != http.StatusOK {
		env.Auditor.Log(audit.Error, fmt.Sprintf("Response Not Ok: %+v", response), map[string]any{"mode": env.Endpoint})
		return "", fmt.Errorf("Unexpected Response %s", response.Status)
	}

	rspData, err := io.ReadAll(response.Body)
	var rspBodyPtr *OllamaResponse = &OllamaResponse{}
	json.Unmarshal(rspData, rspBodyPtr)
	ollamaRsp := *rspBodyPtr

	env.Auditor.Log(audit.Info, fmt.Sprintf("%+v", ollamaRsp), map[string]any{"mode": env.Endpoint})
	if len(ollamaRsp.Message.ToolCalls) == 0 {
		env.Auditor.Log(audit.Warning, "No tool calls proposed.", map[string]any{"mode": env.Endpoint})
		return "", nil
	}

	toolResult := strings.Builder{}

	for _, msgtc := range ollamaRsp.Message.ToolCalls {
		proposedAction, actionArgs := extractActionArgs(msgtc.Function.Arguments)
		toolCall := policy.ToolCall{
			ID:     msgtc.ID,
			Tool:   msgtc.Function.Name,
			Action: fmt.Sprintf("%v", proposedAction),
			Args:   actionArgs,
		}

		responseContent, err := env.EMediator.Execute(env.ModelContext, toolCall, func(_ context.Context, args map[string]any) (string, error) {
			return ollamaExecuteTool(env.ModelContext, msgtc.Function.Name, toolCall)
		})
		if err != nil {
			toolResult.WriteString("")
		} else {
			toolResult.WriteString(responseContent)
		}
		toolResult.WriteRune('\n')
	}

	return toolResult.String(), nil
}

func ollamaLocalServerStatus(base string) bool {
	client := http.Client{Timeout: 1 * time.Second}
	response, err := client.Get(base + "/api/tags") // dummy query
	if nil != err {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func extractActionArgs(args map[string]any) (any, map[string]any) {
	var action any
	rmap := map[string]any{}
	for k := range args {
		if k == "action" {
			action = args[k]
		} else {
			rmap[k] = args[k]
		}
	}
	return action, rmap
}

func OllamaShutdown() error {
	if env.OllamaProc == nil || env.OllamaProc.Process == nil {
		return nil
	}
	if err := env.OllamaProc.Process.Kill(); nil != err {
		return fmt.Errorf("Failed to kill Ollama: %w", err)
	}
	_ = env.OllamaProc.Wait()
	env.OllamaProc = nil
	return nil
}

func ollamaExecuteTool(ctx context.Context, name string, toolCall policy.ToolCall) (string, error) {
	switch name {
	case "read_file":
		pathAny, exists := toolCall.Args["path"]
		if !exists {
			return "Error: model failed to provide file path.", nil
		}
		pathStr, ok := pathAny.(string)
		if !ok || pathStr == "" {
			return "Error: path argument is invalid or empty.", nil
		}
		return env.RuntimeRegistry.ReadFile(ctx, tools.ReadFileArgs{Path: pathStr})

	case "write_file":
		pathAny, hasPath := toolCall.Args["path"]
		contentAny, hasContent := toolCall.Args["content"]
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
		return env.RuntimeRegistry.WriteFile(ctx, tools.WriteFileArgs{Path: pathStr, Content: contentStr})

	case "list_directory":
		var lastPath string
		if pathAny, exists := toolCall.Args["path"]; exists {
			if pathStr, ok := pathAny.(string); ok && pathStr != "" {
				lastPath = pathStr
			}
		}
		return env.RuntimeRegistry.ListDirectory(ctx, tools.ListDirectoryArgs{Path: lastPath})

	case "git":
		var lastPath string
		if pathAny, exists := toolCall.Args["path"]; exists {
			if pathStr, ok := pathAny.(string); ok && pathStr != "" {
				lastPath = pathStr
			}
		}

		argsAny, exists := toolCall.Args["args"].([]any)
		if !exists {
			return "Error: model failed to provide git arguments.", nil
		}

		var gitArgs []string
		for _, arg := range argsAny {
			if strArg, ok := arg.(string); ok {
				gitArgs = append(gitArgs, strArg)
			}
		}
		return env.RuntimeRegistry.ExecuteGitCommand(ctx, tools.GitExecArgs{Path: lastPath, Args: gitArgs})
	case "http_get":
		urlAny, hasURL := toolCall.Args["url"]
		if !hasURL {
			return "Error: model failed to provide a URL.", nil
		}
		urlStr, ok := urlAny.(string)
		if !ok || urlStr == "" {
			return "Error: url argument is invalid or empty.", nil
		}
		return env.RuntimeRegistry.HTTPGet(ctx, tools.HTTPGetArgs{URL: urlStr})
	case "go_version":
	case "rust_version":

	default:
		return fmt.Sprintf("Error: Unknown function %s called.", name), nil
	}

	return "", fmt.Errorf("Unreachable execution reached.")
}

func OllamaLS() string {
	ecmd := exec.Command("ollama", "list")
	outputBytes, err := ecmd.CombinedOutput()
	if err != nil {
		return "Error retrieving Ollama models"
	}
	return string(outputBytes)
}
