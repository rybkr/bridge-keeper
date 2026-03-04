package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"bridgekeeper/internal/policy"
	"bridgekeeper/internal/types"
)

func TestParseToolCallLine_JSONRPC(t *testing.T) {
	line := []byte(`{"jsonrpc":"2.0","id":1,"method":"tool_call","params":{"id":"t1","tool":"fs","action":"read_file","args":{"path":"README.md"}}}`)

	call, err := parseToolCallLine(line)
	if err != nil {
		t.Fatalf("parseToolCallLine() error = %v", err)
	}
	if call.ID != "t1" || call.Tool != "fs" || call.Action != "read_file" {
		t.Fatalf("unexpected call: %+v", call)
	}
}

func TestParseToolCallLine_RawToolCall(t *testing.T) {
	line := []byte(`{"id":"t2","tool":"shell","action":"exec","args":{"command":"echo hi"}}`)

	call, err := parseToolCallLine(line)
	if err != nil {
		t.Fatalf("parseToolCallLine() error = %v", err)
	}
	if call.ID != "t2" || call.Tool != "shell" || call.Action != "exec" {
		t.Fatalf("unexpected call: %+v", call)
	}
}

func TestParseToolCallLine_WrappedCall(t *testing.T) {
	line := []byte(`{"call":{"id":"t3","tool":"http","action":"get","args":{"url":"https://example.com"}}}`)

	call, err := parseToolCallLine(line)
	if err != nil {
		t.Fatalf("parseToolCallLine() error = %v", err)
	}
	if call.ID != "t3" || call.Tool != "http" || call.Action != "get" {
		t.Fatalf("unexpected call: %+v", call)
	}
}

func TestParseToolCallLine_Invalid(t *testing.T) {
	_, err := parseToolCallLine([]byte(`{"jsonrpc":"2.0","id":1,"method":"list_tools"}`))
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRun_EmitsDecisionsAndErrors(t *testing.T) {
	pf := &policy.PolicyFile{
		Default: "deny",
		Capabilities: []policy.Capability{
			{Name: "read-fs", Tool: "fs", Actions: []string{"read_file"}, Decision: "allow"},
		},
	}
	eng := policy.NewEngine(pf)

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tool_call","params":{"id":"a","tool":"fs","action":"read_file","args":{"path":"README.md"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tool_call","params":{"id":"b","tool":"shell","action":"exec","args":{"command":"echo hi"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"list_tools"}`,
		"",
	}, "\n")

	var out bytes.Buffer
	parseErrors, err := run(context.Background(), strings.NewReader(input), &out, eng)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if parseErrors != 1 {
		t.Fatalf("parseErrors = %d, want 1", parseErrors)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d output lines, want 3", len(lines))
	}

	var row1 evalOutput
	if err := json.Unmarshal([]byte(lines[0]), &row1); err != nil {
		t.Fatalf("unmarshal row1: %v", err)
	}
	if row1.Decision == nil || row1.Decision.Decision != types.Allow {
		t.Fatalf("row1 decision = %+v, want allow", row1.Decision)
	}

	var row2 evalOutput
	if err := json.Unmarshal([]byte(lines[1]), &row2); err != nil {
		t.Fatalf("unmarshal row2: %v", err)
	}
	if row2.Decision == nil || row2.Decision.Decision != types.Deny {
		t.Fatalf("row2 decision = %+v, want deny", row2.Decision)
	}

	var row3 evalOutput
	if err := json.Unmarshal([]byte(lines[2]), &row3); err != nil {
		t.Fatalf("unmarshal row3: %v", err)
	}
	if row3.Error == "" {
		t.Fatalf("row3 error expected, got %+v", row3)
	}
}
