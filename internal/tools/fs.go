package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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
