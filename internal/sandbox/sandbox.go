package sandbox

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"bridgekeeper/internal/types"
)

// Validator enforces runtime-local filesystem and payload constraints below policy.
type Validator struct {
	WorkspaceRoot          string
	MaxOutputBytes         int
	MaxReadBytes           int64
	MaxWriteBytes          int64
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
		MaxWriteBytes:          64 * 1024,
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
		if call.Action == "write_file" {
			if err := v.validateContentSize(args); err != nil {
				return call, err
			}
		}
	case "git":
		path, err := v.pathArg(args, "path")
		if err == nil {
			args["path"] = path
		}
		if err := v.validateGitArgs(args); err != nil {
			return call, err
		}
	case "http":
		rawURL, err := v.urlArg(args, "url")
		if err != nil {
			return call, err
		}
		args["url"] = rawURL
	}

	call.Args = args
	return call, nil
}

func (v *Validator) urlArg(args map[string]any, key string) (string, error) {
	raw, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	value, ok := raw.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("%s is not a valid URL: %w", key, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%s must use http or https", key)
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("%s must include a host", key)
	}
	return parsed.String(), nil
}

func (v *Validator) validateContentSize(args map[string]any) error {
	if v.MaxWriteBytes <= 0 {
		return nil
	}

	raw, ok := args["content"]
	if !ok {
		return fmt.Errorf("content is required")
	}

	content, ok := raw.(string)
	if !ok {
		return fmt.Errorf("content must be a string")
	}
	if int64(len(content)) > v.MaxWriteBytes {
		return fmt.Errorf("content exceeds max write size of %d bytes", v.MaxWriteBytes)
	}
	return nil
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
