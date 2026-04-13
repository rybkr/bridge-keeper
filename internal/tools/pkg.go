package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

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
