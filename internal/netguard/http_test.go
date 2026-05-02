package netguard

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestValidateHTTPURLBlocksUnsafeLiteralHosts(t *testing.T) {
	tests := []string{
		"http://localhost/admin",
		"http://localhost./admin",
		"http://127.0.0.1:8080/metrics",
		"http://127.0.0.1./metrics",
		"http://127.1/admin",
		"http://0.0.0.0:22",
		"http://2130706433/",
		"http://0x7f000001/",
		"http://0177.0.0.1/",
		"http://[::1]/",
		"http://[::ffff:127.0.0.1]/",
		"http://169.254.169.254/latest/meta-data/",
		"http://100.100.100.200/latest/meta-data/",
		"http://10.0.0.1/",
		"http://172.16.0.1/",
		"http://192.168.1.1/",
		"http://[fc00::1]/",
	}

	for _, rawURL := range tests {
		t.Run(rawURL, func(t *testing.T) {
			if _, err := ValidateHTTPURL(rawURL); err == nil {
				t.Fatal("expected URL to be blocked")
			}
		})
	}
}

func TestValidateHTTPURLAllowsPublicHost(t *testing.T) {
	if _, err := ValidateHTTPURL("https://api.example.com/data"); err != nil {
		t.Fatalf("ValidateHTTPURL() error = %v", err)
	}
}

func TestBlockedHostReasonBlocksMetadataNames(t *testing.T) {
	for _, host := range []string{"metadata", "metadata.google.internal", "metadata.goog"} {
		t.Run(host, func(t *testing.T) {
			if _, blocked := BlockedHostReason(host); !blocked {
				t.Fatal("expected metadata host to be blocked")
			}
		})
	}
}

func TestGuardedHTTPClientBlocksRedirectToUnsafeHost(t *testing.T) {
	client := NewGuardedHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Status:     "302 Found",
				Header: http.Header{
					"Location": []string{"http://127.0.0.1/admin"},
				},
				Body:    io.NopCloser(strings.NewReader("redirecting")),
				Request: req,
			}, nil
		}),
	})

	_, err := client.Get("https://example.com/start")
	if err == nil {
		t.Fatal("expected redirect to unsafe host to be blocked")
	}
	if !strings.Contains(err.Error(), "redirect blocked") {
		t.Fatalf("error = %q, want redirect blocked", err.Error())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
