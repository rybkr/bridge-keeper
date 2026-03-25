package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

func (r *Registry) HTTPGet(ctx context.Context, req HTTPGetArgs) (string, error) {
	timeout := 5 * time.Second
	limit := int64(64 * 1024)

	if r != nil && r.Validator != nil {
		if r.Validator.SubprocessTimeoutSecs > 0 {
			timeout = time.Duration(r.Validator.SubprocessTimeoutSecs) * time.Second
		}
		if r.Validator.MaxOutputBytes > 0 {
			limit = int64(r.Validator.MaxOutputBytes)
		}
	}

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return "", fmt.Errorf("http get request: %w", err)
	}

	client := &http.Client{
		Timeout: timeout,
	}
	if r != nil && r.HTTPClient != nil {
		client = r.HTTPClient
	}
	resp, err := client.Do(reqHTTP)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", fmt.Errorf("http get body: %w", err)
	}
	if int64(len(body)) > limit {
		return "", fmt.Errorf("http get response exceeds %d bytes", limit)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http get returned %s: %s", resp.Status, string(body))
	}
	return string(body), nil
}
