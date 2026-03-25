package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"

	"bridgekeeper/internal/types"
)

// Validator enforces runtime-local filesystem and payload constraints below policy.
type Validator struct {
	WorkspaceRoot          string
	MaxOutputBytes         int
	MaxReadBytes           int64
	MaxCommandArgs         int
	SubprocessTimeoutSecs  int
	SubprocessEnvAllowlist []string
}

// NewValidator constructs a validator rooted at workspaceRoot.
func NewValidator(workspaceRoot string) (*Validator, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil, fmt.Errorf("workspace root is required")
	}

	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}

	return &Validator{
		WorkspaceRoot:          filepath.Clean(root),
		MaxOutputBytes:         64 * 1024,
		MaxReadBytes:           64 * 1024,
		MaxCommandArgs:         32,
		SubprocessTimeoutSecs:  5,
		SubprocessEnvAllowlist: []string{"PATH", "HOME", "LANG", "LC_ALL", "TERM", "SSH_AUTH_SOCK", "SSH_AGENT_PID", "SSH_ASKPASS"},
	}, nil
}

// ValidateToolCall normalizes and validates a tool call before execution.
func (v *Validator) ValidateToolCall(call types.ToolCall) (types.ToolCall, error) {
	if v == nil {
		return call, nil
	}

	args := cloneArgs(call.Args)
	switch call.Tool {
	case "fs":
		path, err := v.pathArg(args, "path")
		if err != nil {
			return call, err
		}
		args["path"] = path
	case "git":
		path, err := v.pathArg(args, "path")
		if err == nil {
			args["path"] = path
		}
		if err := v.validateGitArgs(args); err != nil {
			return call, err
		}
	}

	call.Args = args
	return call, nil
}

// ValidateToolResult rejects unexpectedly large outputs.
func (v *Validator) ValidateToolResult(result string) error {
	if v == nil || v.MaxOutputBytes <= 0 {
		return nil
	}
	if len(result) > v.MaxOutputBytes {
		return fmt.Errorf("tool result exceeds max output size of %d bytes", v.MaxOutputBytes)
	}
	return nil
}

func (v *Validator) validateGitArgs(args map[string]any) error {
	rawArgs, ok := args["args"]
	if !ok {
		return fmt.Errorf("git args are required")
	}

	items, ok := rawArgs.([]any)
	if !ok {
		return fmt.Errorf("git args must be an array")
	}
	if len(items) == 0 {
		return fmt.Errorf("git args must not be empty")
	}
	if v.MaxCommandArgs > 0 && len(items) > v.MaxCommandArgs {
		return fmt.Errorf("git args exceed max of %d", v.MaxCommandArgs)
	}

	for _, item := range items {
		arg, ok := item.(string)
		if !ok || strings.TrimSpace(arg) == "" {
			return fmt.Errorf("git args must contain non-empty strings")
		}
		if strings.ContainsRune(arg, 0) {
			return fmt.Errorf("git args contain invalid NUL byte")
		}
	}

	return nil
}

func (v *Validator) pathArg(args map[string]any, key string) (string, error) {
	raw, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	path, ok := raw.(string)
	if !ok || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}
	return v.resolveWorkspacePath(path)
}

func (v *Validator) resolveWorkspacePath(path string) (string, error) {
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("path contains invalid NUL byte")
	}

	var resolved string
	if filepath.IsAbs(path) {
		resolved = filepath.Clean(path)
	} else {
		resolved = filepath.Join(v.WorkspaceRoot, path)
	}

	rel, err := filepath.Rel(v.WorkspaceRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace root %q", path, v.WorkspaceRoot)
	}

	return filepath.Clean(resolved), nil
}

func cloneArgs(args map[string]any) map[string]any {
	if args == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(args))
	for key, value := range args {
		out[key] = value
	}
	return out
}
