package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type subprocessSpec struct {
	name       string
	args       []string
	dir        string
	timeout    time.Duration
	maxOutput  int
	allowedEnv []string
	stdin      io.Reader
}

func (r *Registry) runSubprocess(ctx context.Context, spec subprocessSpec) (string, error) {
	if spec.name == "" {
		return "", errors.New("subprocess name is required")
	}
	if spec.timeout <= 0 && r != nil && r.Validator != nil && r.Validator.SubprocessTimeoutSecs > 0 {
		spec.timeout = time.Duration(r.Validator.SubprocessTimeoutSecs) * time.Second
	}
	if spec.maxOutput <= 0 && r != nil && r.Validator != nil && r.Validator.MaxOutputBytes > 0 {
		spec.maxOutput = r.Validator.MaxOutputBytes
	}
	if len(spec.allowedEnv) == 0 && r != nil && r.Validator != nil {
		spec.allowedEnv = r.Validator.SubprocessEnvAllowlist
	}

	if spec.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, spec.name, spec.args...)
	cmd.Dir = spec.dir
	cmd.Env = minimalEnv(spec.allowedEnv)
	cmd.Stdin = spec.stdin

	limiter := &limitedBuffer{limitBytes: spec.maxOutput}
	cmd.Stdout = limiter
	cmd.Stderr = limiter

	err := cmd.Run()
	output := limiter.String()

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return "", fmt.Errorf("%s timed out after %s", spec.name, spec.timeout)
	case limiter.exceeded:
		return "", fmt.Errorf("%s output exceeded %d bytes", spec.name, spec.maxOutput)
	case err != nil:
		if output == "" {
			return "", fmt.Errorf("%s %s: %w", spec.name, strings.Join(spec.args, " "), err)
		}
		return "", fmt.Errorf("%s %s: %w: %s", spec.name, strings.Join(spec.args, " "), err, output)
	default:
		return output, nil
	}
}

func minimalEnv(allowlist []string) []string {
	extras := []string{"GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1"}

	if len(allowlist) == 0 {
		return append([]string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + os.Getenv("HOME"),
			"LANG=C",
			"LC_ALL=C",
		}, extras...)
	}

	var env []string
	for _, key := range allowlist {
		value, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		env = append(env, key+"="+value)
	}
	return append(env, extras...)
}

type limitedBuffer struct {
	mu         sync.Mutex
	buf        bytes.Buffer
	limitBytes int
	exceeded   bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.limitBytes <= 0 {
		return b.buf.Write(p)
	}

	remaining := b.limitBytes - b.buf.Len()
	if remaining <= 0 {
		b.exceeded = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.exceeded = true
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
