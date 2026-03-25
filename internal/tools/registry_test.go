package tools

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bridgekeeper/internal/sandbox"
)

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	validator, err := sandbox.NewValidator(dir)
	if err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(dir, validator)
	got, err := registry.ReadFile(context.Background(), ReadFileArgs{Path: path})
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("ReadFile() = %q, want hello", got)
	}
}

func TestListDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	validator, err := sandbox.NewValidator(dir)
	if err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(dir, validator)
	got, err := registry.ListDirectory(context.Background(), ListDirectoryArgs{Path: dir})
	if err != nil {
		t.Fatalf("ListDirectory() error = %v", err)
	}
	if !strings.Contains(got, "a.txt") || !strings.Contains(got, "sub"+string(filepath.Separator)) {
		t.Fatalf("ListDirectory() output missing expected entries: %q", got)
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")

	validator, err := sandbox.NewValidator(dir)
	if err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(dir, validator)
	got, err := registry.WriteFile(context.Background(), WriteFileArgs{
		Path:    path,
		Content: "hello world",
	})
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if !strings.Contains(got, "Wrote 11 bytes") {
		t.Fatalf("unexpected write result %q", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after write error = %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("file content = %q, want hello world", string(data))
	}
}

func TestHTTPGet(t *testing.T) {
	validator, err := sandbox.NewValidator(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(t.TempDir(), validator)
	registry.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("hello from server")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	}

	got, err := registry.HTTPGet(context.Background(), HTTPGetArgs{URL: "https://example.com/data"})
	if err != nil {
		t.Fatalf("HTTPGet() error = %v", err)
	}
	if got != "hello from server" {
		t.Fatalf("HTTPGet() = %q, want hello from server", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRunSubprocess_Timeout(t *testing.T) {
	validator, err := sandbox.NewValidator(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(t.TempDir(), validator)

	_, err = registry.runSubprocess(context.Background(), subprocessSpec{
		name:      "python3",
		args:      []string{"-c", "import time; time.sleep(1)"},
		timeout:   20 * time.Millisecond,
		maxOutput: 1024,
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestRunSubprocess_OutputLimit(t *testing.T) {
	validator, err := sandbox.NewValidator(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(t.TempDir(), validator)

	_, err = registry.runSubprocess(context.Background(), subprocessSpec{
		name:      "python3",
		args:      []string{"-c", "print('x' * 2000)"},
		timeout:   time.Second,
		maxOutput: 128,
	})
	if err == nil || !strings.Contains(err.Error(), "output exceeded") {
		t.Fatalf("expected output limit error, got %v", err)
	}
}

func TestRunSubprocess_PreservesLeadingWhitespace(t *testing.T) {
	validator, err := sandbox.NewValidator(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(t.TempDir(), validator)

	got, err := registry.runSubprocess(context.Background(), subprocessSpec{
		name:      "python3",
		args:      []string{"-c", "print(' M file.txt', end='')"},
		timeout:   time.Second,
		maxOutput: 1024,
	})
	if err != nil {
		t.Fatalf("runSubprocess() error = %v", err)
	}
	if got != " M file.txt" {
		t.Fatalf("runSubprocess() = %q, want leading whitespace preserved", got)
	}
}

func TestMinimalEnv_PreservesSSHAgentVars(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/test-agent.sock")
	t.Setenv("SSH_AGENT_PID", "1234")
	t.Setenv("PATH", os.Getenv("PATH"))

	env := minimalEnv([]string{"PATH", "SSH_AUTH_SOCK", "SSH_AGENT_PID"})
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "SSH_AUTH_SOCK=/tmp/test-agent.sock") {
		t.Fatalf("expected SSH_AUTH_SOCK in env, got %v", env)
	}
	if !strings.Contains(joined, "SSH_AGENT_PID=1234") {
		t.Fatalf("expected SSH_AGENT_PID in env, got %v", env)
	}
	if !strings.Contains(joined, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("expected GIT_TERMINAL_PROMPT=0 in env, got %v", env)
	}
}
