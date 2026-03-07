package runtime

import (
    "bufio"
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "os/exec"
    "strings"
    "time"
)

/////// Constants ///////
const model = "functiongemma:latest"

/////// "Globals" ///////
var ollamaProc *exec.Cmd
var baseURL string

/////// Types ///////

/// Ollama API ///

// A basic message
type message struct {
	Role string `json:"role"`
	Content string `json:"content,omitempty"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

// The user request, i.e. the prompt
type chatRequest struct {
    Model string `json:"model"`
    Messages []message `json:"messages"`
    Tools []tool `json:"tools,omitempty"`
    Stream bool `json:"stream"`
}

// The response from the model
type chatResponse struct {
    Message message `json:"message"`
    Done bool `json:"done"`
    Error string `json:"error,omitempty"`
}

// This comes from the nested JSON response from Ollama API
type toolCall struct {
    Function toolCallFunction `json:"function"`
}

// This comes from the nested JSON response from Ollama API
type toolCallFunction struct {
    Name string `json:"name"`
    Arguments map[string]any `json:"arguments"`
}

/// Toolchain ///
// (to send to the model so it knows what tools it can use)

// tool structs to match the schema expected by the Ollama model
type tool struct {
    Type string `json:"type"`
    Function toolFunction `json:"function"`
}

type toolFunction struct {
    Name string `json:"name"`
    Description string `json:"description"`
    Parameters toolParameters `json:"parameters"`
}

type toolParameters struct {
    Type string `json:"type"`
    Properties map[string]ToolProperty `json:"properties"`
    Required []string `json:"required"`
}

type ToolProperty struct {
    Type string `json:"type"`
    Descrption string `json:"description"`
}

/// ToolDef struct ///
// Used by the query to bundle the schema
type ToolDef struct {
    // Must match function name the model wants to call
    Name string
    Description string
    Parameters map[string] ToolProperty

    // determines which parameter names must be used
    Required []string
    Handler func(args map[string]any) (string, error) // called with arguments chosen by the model
}

/////// Utility Functions ///////

// Convert from ToolDef to JSON for Ollama query
func (td ToolDef) ollamaJsonFormat() tool {
    return tool {
        Type: "function",
        Function: toolFunction {
            Name: td.Name,
            Description: td.Description,
            Parameters: toolParameters {
                Type: "object", // this is from JSON, not arbitrary name
                Properties: td.Parameters,
                Required: td.Required,
            },
        },
    }
}

// Return bool true if Ollama is running
func serverStatus(base string) bool {
    client := http.Client{Timeout: 1 * time.Second}
    response, err := client.Get(base + "/api/tags")
    if nil != err {
        return false
    }
    response.Body.Close()
    return response.StatusCode == http.StatusOK
}

// chatStream accumulates the conversation
// mostly useful for testing not final product
func chatStream(messages []message, tools []tool) (string, error) {
    message, err := basicChat(messages, tools)
    if nil != err {
        return "", err
    }
    return strings.TrimSpace(message.Content), nil
}

// basicChat is not basic lol
// it concatenates all the info for the caller
func basicChat(messages []message, tools []tool) (message, error) {

    requestBody := chatRequest {
        Model: model,
        Messages: messages,
        Tools: tools,
        Stream: true,
    }

    byteData, err := json.Marshal(requestBody)
    if nil != err {
        return message{}, fmt.Errorf("Failed to encode a request %w", err)
    }

    // the actual post call to the server
    response, err := http.Post(baseURL + "/api/chat", "application/json", bytes.NewReader(byteData))
    if nil != err {
        return message{}, fmt.Errorf("HTTP POST failure %w", err)
    }

    // Someone online said this is better for cleanup
    defer response.Body.Close()

    // Check the response
    if response.StatusCode != http.StatusOK {
        return message{}, fmt.Errorf("Unexpected Response %s", response.Status)
    }

    // Declare these out of loop scope to append to it
    var fullMessage message
    var contentBuffer strings.Builder

    // Iterate on the lines of the response
    scanner := bufio.NewScanner(response.Body)
    for scanner.Scan() {
        line := scanner.Bytes()
        if 0 == len(line) {
            continue
        }

        // try to detect a malformed response
        var messageChunk chatResponse
        if err := json.Unmarshal(line, &messageChunk); nil != err {
            return message{}, fmt.Errorf("Failed to decode a response %w", err)
        }
        if "" != messageChunk.Error {
            return message{}, fmt.Errorf("Model response was malformed %s", messageChunk.Error)
        }

        // Add the message chunk to the rest of it
        contentBuffer.WriteString(messageChunk.Message.Content)

        // Detect any tool calls
        if len(messageChunk.Message.ToolCalls) > 0 {
            fullMessage.ToolCalls = messageChunk.Message.ToolCalls
        }
        fullMessage.Role = messageChunk.Message.Role

        // check if this is the last message chunk and break the loop
        if messageChunk.Done {
            break
        }
    }

    if err := scanner.Err(); nil != err {
        return message{}, fmt.Errorf("Response stream error %w", err)
    }

    fullMessage.Content = contentBuffer.String()
    return fullMessage, nil
}


/////// Public-Facing Ollama API ///////

/// Initialize sthe local Ollama server on given port ///
func Initialize(port int) error {
    baseURL = fmt.Sprintf("http://localhost:%d", port)

    // Ollama is already running and is OK to query
    if serverStatus(baseURL) {
        return nil
    }

    // else: Ollama not running or not ok. Try spinning up
    cmd := exec.Command("ollama", "serve")
    cmd.Env = append(cmd.Environ(), fmt.Sprintf("OLLAMA_HOST=0.0.0.0:%d", port))

    if err := cmd.Start(); nil != err {
        return fmt.Errorf("Failed to start up Ollama: %w", err)
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
    return fmt.Errorf("Failed to start up Ollama: timeout (15s)")
}

/// Shut down ASAP ///
func Shutdown() error {
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

/// Send a prompt query ///
func Query(userPrompt string) (string, error) {

    // Force initialize to run first
    if baseURL == "" {
        return "", fmt.Errorf("Ollama not initialized")
    }

    messages := []message{{Role: "user", Content: userPrompt}}
    return chatStream(messages, nil)
}

func QueryWithTools(userPrompt string, tools []ToolDef) (string, error) {

    // Force initialize to run first
    if baseURL == "" {
        return "", fmt.Errorf("Ollama not initialized")
    }

    // this one is ai magic tbh
    toolset := make([]tool, len(tools))
    handlers := make(map[string]func(map[string]any) (string, error), len(tools))

    for i, toold := range tools {
        toolset[i] = toold.ollamaJsonFormat()
        handlers[toold.Name] = toold.Handler
    }

    // First need to send prompt and tool schema
    messages := []message{{Role: "user", Content: userPrompt}}
    llmResponse, err := basicChat(messages, toolset)
    if nil != err {
        return "", fmt.Errorf("Failed to set up context")
    }

    // If the model doesn't request a tool call just send back its text
    if len(llmResponse.ToolCalls) == 0 {
        return strings.TrimSpace(llmResponse.Content), nil
    }

    // Execute tools
    tcall := llmResponse.ToolCalls[0]
    handler, ok := handlers[tcall.Function.Name]
    if !ok {
        return "", fmt.Errorf("Tool %q unknown", tcall.Function.Name)
    }

    result, err := handler(tcall.Function.Arguments)
    if nil != err {
        return "", fmt.Errorf("Tool %q returned bad %w", tcall.Function.Name, err)
    }

    // Mush assistant and tool messages together
    messages = append(
        messages,
        llmResponse,
        message{Role: "tool", Content: result},
    )

    return chatStream(messages, nil)
}
