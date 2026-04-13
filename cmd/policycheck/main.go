package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"bridgekeeper/internal/policy"
	"bridgekeeper/internal/types"
)

const maxInputLineBytes = 1024 * 1024

type evalOutput struct {
	Line     int                   `json:"line"`
	Call     *types.ToolCall       `json:"call,omitempty"`
	Decision *types.PolicyDecision `json:"decision,omitempty"`
	Error    string                `json:"error,omitempty"`
}

func main() {
	policyPath := flag.String("policy", "policies", "path to policy YAML file or directory")
	inputPath := flag.String("input", "-", "input NDJSON path, or '-' for stdin")
	flag.Parse()

	pf, err := policy.LoadPath(*policyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: loading policy path: %v\n", err)
		os.Exit(1)
	}

	eng := policy.NewEngine(pf)

	in, closeFn, err := openInput(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: opening input: %v\n", err)
		os.Exit(1)
	}
	if closeFn != nil {
		defer closeFn()
	}

	parseErrors, err := run(context.Background(), in, os.Stdout, eng)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: evaluating input: %v\n", err)
		os.Exit(1)
	}
	if parseErrors > 0 {
		os.Exit(2)
	}
}

func openInput(path string) (io.Reader, func() error, error) {
	if path == "" || path == "-" {
		return os.Stdin, nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	return f, f.Close, nil
}

func run(ctx context.Context, in io.Reader, out io.Writer, eng *policy.Engine) (int, error) {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), maxInputLineBytes)

	enc := json.NewEncoder(out)
	lineNum := 0
	parseErrors := 0

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return parseErrors, ctx.Err()
		default:
		}

		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		call, err := parseToolCallLine(line)
		if err != nil {
			parseErrors++
			if err := enc.Encode(evalOutput{Line: lineNum, Error: err.Error()}); err != nil {
				return parseErrors, err
			}
			continue
		}

		decision := eng.Evaluate(ctx, call)
		if err := enc.Encode(evalOutput{Line: lineNum, Call: &call, Decision: &decision}); err != nil {
			return parseErrors, err
		}
	}

	if err := scanner.Err(); err != nil {
		return parseErrors, err
	}

	return parseErrors, nil
}

func parseToolCallLine(line []byte) (types.ToolCall, error) {
	var call types.ToolCall

	var req types.JSONRPCRequest
	if err := json.Unmarshal(line, &req); err == nil && req.Method != "" {
		if req.Method != "tool_call" {
			return call, fmt.Errorf("unsupported json-rpc method %q", req.Method)
		}
		parsed, err := parseToolCallFromParams(req.Params)
		if err != nil {
			return call, err
		}
		return parsed, nil
	}

	if err := json.Unmarshal(line, &call); err == nil {
		if call.Tool != "" && call.Action != "" {
			if call.ID == "" {
				call.ID = "line"
			}
			return call, nil
		}
	}

	var wrapped struct {
		Call types.ToolCall `json:"call"`
	}
	if err := json.Unmarshal(line, &wrapped); err == nil {
		if wrapped.Call.Tool != "" && wrapped.Call.Action != "" {
			if wrapped.Call.ID == "" {
				wrapped.Call.ID = "line"
			}
			return wrapped.Call, nil
		}
	}

	var requestWrapped struct {
		Request json.RawMessage `json:"request"`
	}
	if err := json.Unmarshal(line, &requestWrapped); err == nil && len(requestWrapped.Request) > 0 {
		parsed, err := parseInnerRequest(requestWrapped.Request)
		if err != nil {
			return call, err
		}
		return parsed, nil
	}

	var workflowWrapped struct {
		Workflow []json.RawMessage `json:"workflow"`
	}
	if err := json.Unmarshal(line, &workflowWrapped); err == nil && len(workflowWrapped.Workflow) > 0 {
		parsed, err := parseInnerRequest(workflowWrapped.Workflow[0])
		if err != nil {
			return call, err
		}
		return parsed, nil
	}

	return call, errors.New("line is not a valid tool_call json-rpc request, ToolCall object, or {\"call\": ToolCall} wrapper")
}

func parseInnerRequest(raw json.RawMessage) (types.ToolCall, error) {
	var req types.JSONRPCRequest
	if err := json.Unmarshal(raw, &req); err == nil && req.Method != "" {
		if req.Method != "tool_call" {
			return types.ToolCall{}, fmt.Errorf("unsupported json-rpc method %q", req.Method)
		}
		return parseToolCallFromParams(req.Params)
	}

	var call types.ToolCall
	if err := json.Unmarshal(raw, &call); err == nil && call.Tool != "" && call.Action != "" {
		if call.ID == "" {
			call.ID = "line"
		}
		return call, nil
	}

	return types.ToolCall{}, errors.New("wrapper request/workflow item is not a valid tool_call")
}

func parseToolCallFromParams(params map[string]any) (types.ToolCall, error) {
	var call types.ToolCall
	if params == nil {
		return call, errors.New("json-rpc tool_call params are required")
	}

	data, err := json.Marshal(params)
	if err != nil {
		return call, fmt.Errorf("invalid tool_call params: %w", err)
	}
	if err := json.Unmarshal(data, &call); err != nil {
		return call, fmt.Errorf("invalid tool_call params: %w", err)
	}

	if call.Tool == "" {
		return call, errors.New("tool is required")
	}
	if call.Action == "" {
		return call, errors.New("action is required")
	}
	if call.ID == "" {
		call.ID = "line"
	}

	return call, nil
}
