package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"bridgekeeper/internal/sandbox"
)

// Registry holds typed tool implementations rooted in a workspace.
type Registry struct {
	WorkspaceRoot string
	Validator     *sandbox.Validator
}

// NewRegistry constructs a tool registry for a workspace root.
func NewRegistry(workspaceRoot string, validator *sandbox.Validator) *Registry {
	return &Registry{
		WorkspaceRoot: workspaceRoot,
		Validator:     validator,
	}
}

type GitExecArgs struct {
	Path string
	Args []string
}

type ReadFileArgs struct {
	Path string
}

type ListDirectoryArgs struct {
	Path string
}

func (r *Registry) ExecuteGitCommand(ctx context.Context, req GitExecArgs) (string, error) {
	if len(req.Args) == 0 {
		return "", errors.New("git args are required")
	}

	cmd := exec.CommandContext(ctx, "git", req.Args...)
	cmd.Dir = req.Path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(req.Args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (r *Registry) ReadFile(_ context.Context, req ReadFileArgs) (string, error) {
	limit := int64(64 * 1024)
	if r != nil && r.Validator != nil && r.Validator.MaxReadBytes > 0 {
		limit = r.Validator.MaxReadBytes
	}

	f, err := os.Open(req.Path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	defer f.Close()

	content, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	if int64(len(content)) > limit {
		return "", fmt.Errorf("file exceeds max readable size of %d bytes", limit)
	}
	return string(content), nil
}

func (r *Registry) ListDirectory(_ context.Context, req ListDirectoryArgs) (string, error) {
	entries, err := os.ReadDir(req.Path)
	if err != nil {
		return "", fmt.Errorf("list directory: %w", err)
	}

	var lines []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += string(filepath.Separator)
		}
		lines = append(lines, name)
	}
	return strings.Join(lines, "\n"), nil
}

func (r *Registry) GoVersion(ctx context.Context) (string, error) {
	return runSimpleCommand(ctx, "go", "version")
}

func (r *Registry) RustVersion(ctx context.Context) (string, error) {
	return runSimpleCommand(ctx, "cargo", "--version")
}

func runSimpleCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
