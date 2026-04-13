package policy

import (
	"strings"
	"testing"
)

func TestFormatPolicy(t *testing.T) {
	pf := &PolicyFile{
		Version: "1",
		Default: "deny",
		Capabilities: []Capability{
			{
				Name:     "read-files",
				Tool:     "fs",
				Actions:  []string{"read_file", "list_dir"},
				Decision: "allow",
				Constraints: &Constraints{
					Paths: &AllowDeny{
						Allow: []string{"./**"},
						Deny:  []string{"/etc/**"},
					},
					MaxSizeBytes:   1024,
					TimeoutSeconds: 5,
				},
			},
		},
	}

	got := FormatPolicy(pf)
	for _, want := range []string{
		"Current Policy",
		"Version: 1",
		"Default: deny",
		"[1] read-files",
		"Tool: fs",
		"Actions: read_file, list_dir",
		"Decision: allow",
		"paths:",
		"allow: ./**",
		"deny: /etc/**",
		"max_size_bytes: 1024",
		"timeout_seconds: 5",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted policy missing %q:\n%s", want, got)
		}
	}
}
