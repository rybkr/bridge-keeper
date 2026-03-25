package redact

import "testing"

func TestRedactText(t *testing.T) {
	out := New().RedactText("token=supersecret sk-1234567890abcdef")
	if out == "token=supersecret sk-1234567890abcdef" {
		t.Fatal("expected redaction")
	}
}
