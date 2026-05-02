package tools

import (
	"context"
	"errors"
	"strings"
)

func (r *Registry) ExecuteShellCommand(ctx context.Context, req ShellExecArgs) (string, error) {
	argv, err := splitSimpleCommand(req.Command)
	if err != nil {
		return "", err
	}

	dir := req.Path
	if strings.TrimSpace(dir) == "" && r != nil {
		dir = r.WorkspaceRoot
	}

	return r.runSubprocess(ctx, subprocessSpec{
		name: argv[0],
		args: argv[1:],
		dir:  dir,
	})
}

func splitSimpleCommand(command string) ([]string, error) {
	argv := strings.Fields(command)
	if len(argv) == 0 {
		return nil, errors.New("shell command is required")
	}
	return argv, nil
}
