package sandbox

import (
	"path/filepath"
	"testing"

	"bridgekeeper/internal/types"
)

func TestValidateToolCall_NormalizesPathWithinWorkspace(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}

	call, err := validator.ValidateToolCall(types.ToolCall{
		Tool:   "fs",
		Action: "read_file",
		Args: map[string]any{
			"path": "./docs/../README.md",
		},
	})
	if err != nil {
		t.Fatalf("ValidateToolCall() error = %v", err)
	}

	want := filepath.Clean("/tmp/workspace/README.md")
	if call.Args["path"] != want {
		t.Fatalf("path = %v, want %q", call.Args["path"], want)
	}
}

func TestValidateToolCall_RejectsPathEscape(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}

	_, err = validator.ValidateToolCall(types.ToolCall{
		Tool:   "fs",
		Action: "read_file",
		Args: map[string]any{
			"path": "../secret.txt",
		},
	})
	if err == nil {
		t.Fatal("expected path escape error")
	}
}

func TestValidateToolResult_RejectsLargeOutput(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}
	validator.MaxOutputBytes = 4

	if err := validator.ValidateToolResult("12345"); err == nil {
		t.Fatal("expected oversized result error")
	}
}

func TestValidateToolCall_RejectsOversizedWriteContent(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}
	validator.MaxWriteBytes = 4

	_, err = validator.ValidateToolCall(types.ToolCall{
		Tool:   "fs",
		Action: "write_file",
		Args: map[string]any{
			"path":    "note.txt",
			"content": "12345",
		},
	})
	if err == nil {
		t.Fatal("expected oversized write content error")
	}
}
