package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bridgekeeper/internal/audit"
	"bridgekeeper/internal/policy"
	"bridgekeeper/internal/redact"
	"bridgekeeper/internal/sandbox"
	"bridgekeeper/internal/types"
)

func TestAdversarialFixtures_BlockExpectedCalls(t *testing.T) {
	mediator := newFixtureMediator(t)
	calls := loadFixtureCalls(t, filepath.Join("..", "..", "testdata", "adversarial", "path_traversal.ndjson"))

	for _, call := range calls {
		result, err := mediator.Execute(context.Background(), call, func(context.Context, map[string]any) (string, error) {
			t.Fatalf("handler should not run for fixture call %+v", call)
			return "", nil
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if !strings.Contains(strings.ToLower(result), "denied") {
			t.Fatalf("expected denied result for %+v, got %q", call, result)
		}
	}
}

func TestWorkflowFixtures_DefaultPolicyShape(t *testing.T) {
	mediator := newFixtureMediator(t)
	calls := loadFixtureCalls(t, filepath.Join("..", "..", "testdata", "workflows", "read_and_summarize.ndjson"))

	for _, call := range calls {
		result, err := mediator.Execute(context.Background(), call, func(context.Context, map[string]any) (string, error) {
			return "ok", nil
		})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result != "ok" {
			t.Fatalf("expected allowed workflow call for %+v, got %q", call, result)
		}
	}
}

func TestSecretWorkflowFixture_RedactsOutput(t *testing.T) {
	mediator := newFixtureMediator(t)
	calls := loadFixtureCalls(t, filepath.Join("..", "..", "testdata", "adversarial", "secret_exfil.ndjson"))

	result, err := mediator.Execute(context.Background(), calls[0], func(context.Context, map[string]any) (string, error) {
		return "ghp_abc123def456ghi789jkl012mno345pqr678", nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(result, "ghp_abc123def456ghi789jkl012mno345pqr678") {
		t.Fatalf("expected secret to be redacted, got %q", result)
	}
}

func newFixtureMediator(t *testing.T) *Mediator {
	t.Helper()

	pf, err := policy.LoadPath(filepath.Join("..", "..", "policies", "default.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	validator, err := sandbox.NewValidator(workspace)
	if err != nil {
		t.Fatal(err)
	}

	return &Mediator{
		Policy:   policy.NewEngine(pf),
		Approver: stubApprover{approved: false},
		Audit:    audit.NewLogger(&bytes.Buffer{}, audit.Info),
		Sandbox:  validator,
		Redactor: redact.New(),
	}
}

func loadFixtureCalls(t *testing.T, path string) []types.ToolCall {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var calls []types.ToolCall
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var row struct {
			Request  json.RawMessage   `json:"request"`
			Workflow []json.RawMessage `json:"workflow"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatalf("unmarshal fixture row: %v", err)
		}

		if len(row.Request) > 0 {
			calls = append(calls, mustParseFixtureCall(t, row.Request))
		}
		for _, item := range row.Workflow {
			calls = append(calls, mustParseFixtureCall(t, item))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return calls
}

func mustParseFixtureCall(t *testing.T, raw json.RawMessage) types.ToolCall {
	t.Helper()

	var req types.JSONRPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal jsonrpc request: %v", err)
	}
	if req.Method != "tool_call" {
		t.Fatalf("unexpected method %q", req.Method)
	}

	data, err := json.Marshal(req.Params)
	if err != nil {
		t.Fatal(err)
	}

	var call types.ToolCall
	if err := json.Unmarshal(data, &call); err != nil {
		t.Fatalf("unmarshal tool call params: %v", err)
	}
	return call
}
