package sandbox

import (
	"path/filepath"
	"strings"
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

func TestValidateToolCall_RejectsInvalidHTTPURL(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}

	_, err = validator.ValidateToolCall(types.ToolCall{
		Tool:   "http",
		Action: "get",
		Args: map[string]any{
			"url": "file:///etc/passwd",
		},
	})
	if err == nil {
		t.Fatal("expected invalid URL scheme error")
	}
}

func TestValidateToolCall_RejectsUnsafeHTTPURL(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}

	for _, rawURL := range []string{
		"http://127.1/admin",
		"http://2130706433/",
		"http://[::1]/",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/",
	} {
		t.Run(rawURL, func(t *testing.T) {
			_, err = validator.ValidateToolCall(types.ToolCall{
				Tool:   "http",
				Action: "get",
				Args: map[string]any{
					"url": rawURL,
				},
			})
			if err == nil {
				t.Fatal("expected unsafe URL error")
			}
		})
	}
}

func TestValidateToolCall_RejectsOversizedHTTPPostBody(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}
	validator.MaxWriteBytes = 4

	_, err = validator.ValidateToolCall(types.ToolCall{
		Tool:   "http",
		Action: "post",
		Args: map[string]any{
			"url":  "https://example.com",
			"body": "12345",
		},
	})
	if err == nil {
		t.Fatal("expected oversized HTTP post body error")
	}
}

func TestValidateToolCall_RejectsShellMetacharacters(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}

	_, err = validator.ValidateToolCall(types.ToolCall{
		Tool:   "shell",
		Action: "exec",
		Args: map[string]any{
			"command": "echo hello; rm -rf /",
		},
	})
	if err == nil {
		t.Fatal("expected shell metacharacter error")
	}
}

func TestValidateToolCall_AllowsPackageQuery(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}

	_, err = validator.ValidateToolCall(types.ToolCall{
		Tool:   "pkg",
		Action: "query",
		Args: map[string]any{
			"manager": "go",
			"package": "golang.org/x/text",
		},
	})
	if err != nil {
		t.Fatalf("ValidateToolCall() error = %v", err)
	}
}

func TestValidateToolCall_RejectsUnsafePackageName(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}

	_, err = validator.ValidateToolCall(types.ToolCall{
		Tool:   "pkg",
		Action: "install",
		Args: map[string]any{
			"manager": "go",
			"package": "example.com/pkg;rm",
		},
	})
	if err == nil {
		t.Fatal("expected unsafe package name error")
	}
}

func TestValidateToolCall_AllowsReadOnlyGitArgs(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		action string
		args   []any
	}{
		{
			name:   "runtime argv includes subcommand",
			action: "diff",
			args:   []any{"diff", "--stat", "--", "internal/sandbox/sandbox.go"},
		},
		{
			name:   "direct tool call omits subcommand",
			action: "branch",
			args:   []any{"--list", "main*"},
		},
		{
			name:   "empty direct status args",
			action: "status",
			args:   []any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.ValidateToolCall(types.ToolCall{
				Tool:   "git",
				Action: tt.action,
				Args: map[string]any{
					"args": tt.args,
				},
			})
			if err != nil {
				t.Fatalf("ValidateToolCall() error = %v", err)
			}
		})
	}
}

func TestValidateToolCall_RejectsUnsafeGitArgs(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		action      string
		args        []any
		wantMessage string
	}{
		{
			name:        "diff no-index with runtime subcommand",
			action:      "diff",
			args:        []any{"diff", "--no-index", "/etc/passwd", "/dev/null"},
			wantMessage: "--no-index",
		},
		{
			name:        "diff no-index direct call",
			action:      "diff",
			args:        []any{"--no-index", "/etc/passwd", "/dev/null"},
			wantMessage: "--no-index",
		},
		{
			name:        "branch delete with runtime subcommand",
			action:      "branch",
			args:        []any{"branch", "-D", "main"},
			wantMessage: "-D",
		},
		{
			name:        "branch delete direct call",
			action:      "branch",
			args:        []any{"-D", "main"},
			wantMessage: "-D",
		},
		{
			name:        "action argv subcommand mismatch",
			action:      "diff",
			args:        []any{"branch", "--list"},
			wantMessage: "does not match",
		},
		{
			name:        "diff rejects absolute path operands",
			action:      "diff",
			args:        []any{"diff", "--", "/etc/passwd"},
			wantMessage: "absolute paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.ValidateToolCall(types.ToolCall{
				Tool:   "git",
				Action: tt.action,
				Args: map[string]any{
					"args": tt.args,
				},
			})
			if err == nil {
				t.Fatal("expected unsafe git args to be rejected")
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("error = %q, want message containing %q", err.Error(), tt.wantMessage)
			}
		})
	}
}

func TestValidateToolCall_RejectsGitPathEscape(t *testing.T) {
	validator, err := NewValidator("/tmp/workspace")
	if err != nil {
		t.Fatal(err)
	}

	_, err = validator.ValidateToolCall(types.ToolCall{
		Tool:   "git",
		Action: "status",
		Args: map[string]any{
			"path": "../other-repo",
			"args": []any{"status"},
		},
	})
	if err == nil {
		t.Fatal("expected git path escape error")
	}
}
