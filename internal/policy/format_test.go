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
				Name:        "read-files",
				Tool:        "fs",
				Actions:     []string{"read_file", "list_dir"},
				Decision:    "allow",
				RiskLevel:   "low",
				Scope:       &Scope{Roles: []string{"developer"}},
				Conditions:  &Condition{Arg: "path", Op: "exists"},
				Remediation: "choose an allowed file",
				Constraints: &Constraints{
					Paths: &AllowDeny{
						Allow: []string{"./**"},
						Deny:  []string{"/etc/**"},
					},
					Arguments: map[string]ArgumentSpec{
						"path": {Type: "string", Required: true},
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
		"Risk: low",
		"Scope:",
		"roles: developer",
		"Conditions: configured",
		"Remediation: choose an allowed file",
		"paths:",
		"allow: ./**",
		"deny: /etc/**",
		"arguments: 1 schema entries",
		"max_size_bytes: 1024",
		"timeout_seconds: 5",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted policy missing %q:\n%s", want, got)
		}
	}
}
