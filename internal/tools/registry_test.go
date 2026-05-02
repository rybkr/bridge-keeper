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

func TestHTTPGetBlocksUnsafeInitialURL(t *testing.T) {
	validator, err := sandbox.NewValidator(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(t.TempDir(), validator)
	registry.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			t.Fatal("transport should not be called for blocked URL")
			return nil, nil
		}),
	}

	if _, err := registry.HTTPGet(context.Background(), HTTPGetArgs{URL: "http://2130706433/"}); err == nil {
		t.Fatal("expected unsafe URL error")
	}
}

func TestHTTPGetBlocksRedirectToUnsafeURL(t *testing.T) {
	validator, err := sandbox.NewValidator(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(t.TempDir(), validator)
	registry.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Hostname() != "example.com" {
				t.Fatalf("redirect target should have been blocked before transport, got %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusFound,
				Status:     "302 Found",
				Body:       io.NopCloser(strings.NewReader("")),
				Header: http.Header{
					"Location": []string{"http://127.1/admin"},
				},
				Request: req,
			}, nil
		}),
	}

	if _, err := registry.HTTPGet(context.Background(), HTTPGetArgs{URL: "https://example.com/data"}); err == nil {
		t.Fatal("expected unsafe redirect error")
	}
}

func TestHTTPPost(t *testing.T) {
	validator, err := sandbox.NewValidator(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(t.TempDir(), validator)
	registry.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", req.Method)
			}
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != `{"ok":true}` {
				t.Fatalf("body = %q", string(body))
			}
			if got := req.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("content-type = %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("accepted")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	}

	got, err := registry.HTTPPost(context.Background(), HTTPPostArgs{URL: "https://example.com/data", Body: `{"ok":true}`})
	if err != nil {
		t.Fatalf("HTTPPost() error = %v", err)
	}
	if got != "accepted" {
		t.Fatalf("HTTPPost() = %q, want accepted", got)
	}
}

func TestHTTPGetBlocksRedirectToUnsafeHost(t *testing.T) {
	validator, err := sandbox.NewValidator(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(t.TempDir(), validator)
	registry.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Status:     "302 Found",
				Header: http.Header{
					"Location": []string{"http://169.254.169.254/latest/meta-data/"},
				},
				Body:    io.NopCloser(strings.NewReader("redirecting")),
				Request: req,
			}, nil
		}),
	}

	_, err = registry.HTTPGet(context.Background(), HTTPGetArgs{URL: "https://example.com/start"})
	if err == nil {
		t.Fatal("expected redirect to metadata endpoint to be blocked")
	}
	if !strings.Contains(err.Error(), "redirect blocked") {
		t.Fatalf("error = %q, want redirect blocked", err.Error())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestExecuteShellCommand(t *testing.T) {
	validator, err := sandbox.NewValidator(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(t.TempDir(), validator)

	got, err := registry.ExecuteShellCommand(context.Background(), ShellExecArgs{Command: "echo hello"})
	if err != nil {
		t.Fatalf("ExecuteShellCommand() error = %v", err)
	}
	if strings.TrimSpace(got) != "hello" {
		t.Fatalf("ExecuteShellCommand() = %q, want hello", got)
	}
}

func TestPackageListUsesGoModules(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	binDir := t.TempDir()
	goPath := filepath.Join(binDir, "go")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\"\n"
	if err := os.WriteFile(goPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	validator, err := sandbox.NewValidator(dir)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(dir, validator)

	got, err := registry.PackageList(context.Background(), PackageListArgs{})
	if err != nil {
		t.Fatalf("PackageList() error = %v", err)
	}
	if strings.TrimSpace(got) != "list -m all" {
		t.Fatalf("PackageList() = %q, want go list args", got)
	}
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
