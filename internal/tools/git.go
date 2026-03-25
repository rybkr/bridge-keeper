package tools

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

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
