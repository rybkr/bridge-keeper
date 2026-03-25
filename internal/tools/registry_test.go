package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
