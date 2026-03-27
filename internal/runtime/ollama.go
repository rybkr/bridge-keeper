package runtime

import (
	"bridgekeeper/internal/audit"
	"bridgekeeper/internal/policy"
	"bridgekeeper/internal/types"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

var ollamaProc *exec.Cmd
var baseURL string
var OllamaSelectedModel = ollamaPreferredModel

// Functions
func OllamaInitialize() error {
	baseURL = "http://localhost:11434"

	// Ollama is already running and is OK to query
	if serverStatus(baseURL) {
		return nil
	}

	// else: Ollama not running or not ok. Try spinning up
	cmd := exec.Command("ollama", "serve")
	cmd.Env = append(cmd.Environ(), "OLLAMA_HOST=localhost:11434")

	if err := cmd.Start(); nil != err {
		return error(fmt.Errorf("Failed to start up Ollama: %w", err))
	}

	// spin up ok, now check it's ready
	ollamaProc = cmd
	deadline := time.Now().Add(15 * time.Second) // set timeout 15 seconds
	for time.Now().Before(deadline) {
		// Check again if we can contact the ollama local server
		if serverStatus(baseURL) {
			return nil
		}
		time.Sleep(150 * time.Millisecond) // if not, sleep and wait
	}

	_ = cmd.Process.Kill()
	ollamaProc = nil
	return error(fmt.Errorf("Failed to start up Ollama: timeout (15s)"))
}

func OllamaDeferShutdown() {
	if err := OllamaShutdown(); nil != err {
		log.Printf("shutdown %v", err)
	}
}

func serverStatus(base string) bool {
	client := http.Client{Timeout: 1 * time.Second}
	response, err := client.Get(base + "/api/tags")
	if nil != err {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func SendOllamaMessage(userPrompt string, eng *policy.Engine, ctx context.Context) error {
	requestBody := OllamaAPIRequest{
		Model: OllamaSelectedModel,
		Messages: []OMessage{{
			Role:    User,
			Content: userPrompt,
		}},
		Stream:    false,
		Think:     false,
		KeepAlive: ollamaModelKeepAlive,
		Tools:     AvailableTools,
	}
	byteData, err := json.Marshal(requestBody)

	audit.LogEvent(fmt.Sprintf("REQUEST: %s", string(byteData[:])), audit.Debug)

	if nil != err {
		return fmt.Errorf("Unable to marshal user request")
	}

	// the actual post call to the server
	apiurl := baseURL + "/api/chat"
	audit.LogEvent(fmt.Sprintf("URL: %s", apiurl), audit.Debug)
	rspPtr, err := http.Post(apiurl, "application/json", bytes.NewReader(byteData))
	// Someone online said this is better for cleanup
	defer (*rspPtr).Body.Close()
	response := *rspPtr
	if nil != err {
		return fmt.Errorf("HTTP POST failure %w", err)
	}

	// Check the response
	audit.LogEvent(fmt.Sprintf("Response: %+v", response), audit.Debug)
	if response.StatusCode != http.StatusOK {
		audit.LogEvent(fmt.Sprintf("Response Not Ok: %+v", response), audit.Warning)
		return fmt.Errorf("Unexpected Response %s", response.Status)
	}

	rspData, err := io.ReadAll(response.Body)
	var rspBodyPtr *OllamaResponse = &OllamaResponse{}
	json.Unmarshal(rspData, rspBodyPtr)
	proposedOllamaCalls := *rspBodyPtr

	audit.LogEvent("Ollama Diagnostics:", audit.Debug)
	audit.LogEvent(fmt.Sprintf("%+v", proposedOllamaCalls), audit.Debug)
	if len(proposedOllamaCalls.Message.ToolCalls) == 0 {
		audit.LogEvent("No tool calls proposed.", audit.Warning)
		return nil
	}

	// Get the tool call
	action, args := extractActionArgs(proposedOllamaCalls.Message.ToolCalls[0].Function.Arguments)
	toolCall := types.ToolCall{
		ID:     proposedOllamaCalls.Message.ToolCalls[0].ID,
		Tool:   proposedOllamaCalls.Message.ToolCalls[0].Function.Name,
		Action: fmt.Sprintf("%v", action),
		Args:   args,
	}

	ndjsonBytes, _ := json.Marshal(toolCall)
	fmt.Printf("\n[POLICY ENG] Intercepted NDJSON: %s\n", string(ndjsonBytes))
	audit.LogEvent(fmt.Sprintf("Gemini tool call intercepted: %s", string(ndjsonBytes)), audit.Info)

	decision := (*eng).Evaluate(ctx, toolCall)
	fmt.Printf("[POLICY ENG] Decision: %s (Reason: %s)\n", decision.Decision, decision.Reason)
	audit.LogEvent(fmt.Sprintf("Policy Decision: %s - %s", decision.Decision, decision.Reason), audit.Info)

	if decision.Decision != types.Allow {
		// Denied: Bypass execution and tell the AI why it failed
		fmt.Printf("Error: Execution denied by policy. Reason: %s", decision.Reason)
		audit.LogEvent("Execution bypassed. Returned policy rejection to Gemini.", audit.Warning)
	} else {
		audit.LogEvent(fmt.Sprintf("Execution allowed. Running tool: %s", toolCall.Tool), audit.Info)
		ex := exec.Command(toolCall.Tool)
		for _, ag := range toolCall.Args {
			ex.Args = append(ex.Args, fmt.Sprintf("%s", ag))
		}
		ex.Stdout = os.Stdout
		terr := ex.Run()
		if nil != terr {
			return terr
		}

	}

	return nil
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
	if ollamaProc == nil || ollamaProc.Process == nil {
		return nil
	}
	if err := ollamaProc.Process.Kill(); nil != err {
		return fmt.Errorf("Failed to kill Ollama: %w", err)
	}
	_ = ollamaProc.Wait()
	ollamaProc = nil
	return nil
}

func OllamaLS() {
	ecmd := exec.Command("ollama", "list")
	ecmd.Stdout = os.Stdout
	if ecmd.Run() != nil {
		log.Fatal("Could not exec for ollama")
	}
}

func OllamaSelectModel(parts []string) {
	OllamaSelectedModel = parts[1]
}
