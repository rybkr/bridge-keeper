package tools

import (
	"context"
	"errors"
)

func (r *Registry) ExecuteGitCommand(ctx context.Context, req GitExecArgs) (string, error) {
	if len(req.Args) == 0 {
		return "", errors.New("git args are required")
	}
	return r.runSubprocess(ctx, subprocessSpec{
		name: "git",
		args: req.Args,
		dir:  req.Path,
	})
}
