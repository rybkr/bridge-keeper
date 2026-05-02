package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bridgekeeper/internal/netguard"
)

func (r *Registry) HTTPGet(ctx context.Context, req HTTPGetArgs) (string, error) {
	return r.httpRequest(ctx, http.MethodGet, req.URL, "", "")
}

func (r *Registry) HTTPPost(ctx context.Context, req HTTPPostArgs) (string, error) {
	contentType := strings.TrimSpace(req.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	return r.httpRequest(ctx, http.MethodPost, req.URL, req.Body, contentType)
}

func (r *Registry) httpRequest(ctx context.Context, method string, rawURL string, body string, contentType string) (string, error) {
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

	var bodyReader io.Reader
	if method == http.MethodPost {
		bodyReader = bytes.NewReader([]byte(body))
	}

	parsedURL, err := netguard.ValidateHTTPURL(rawURL)
	if err != nil {
		return "", fmt.Errorf("http %s URL blocked: %w", strings.ToLower(method), err)
	}

	reqHTTP, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), bodyReader)
	if err != nil {
		return "", fmt.Errorf("http %s request: %w", strings.ToLower(method), err)
	}
	if contentType != "" {
		reqHTTP.Header.Set("Content-Type", contentType)
	}

	client := netguard.NewGuardedHTTPClient(&http.Client{
		Timeout: timeout,
	})
	if r != nil && r.HTTPClient != nil {
		client = netguard.NewGuardedHTTPClient(r.HTTPClient)
		if client.Timeout == 0 {
			client.Timeout = timeout
		}
	}
	resp, err := client.Do(reqHTTP)
	if err != nil {
		return "", fmt.Errorf("http %s: %w", strings.ToLower(method), err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", fmt.Errorf("http %s body: %w", strings.ToLower(method), err)
	}
	if int64(len(responseBody)) > limit {
		return "", fmt.Errorf("http %s response exceeds %d bytes", strings.ToLower(method), limit)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http %s returned %s: %s", strings.ToLower(method), resp.Status, string(responseBody))
	}
	return string(responseBody), nil
}
