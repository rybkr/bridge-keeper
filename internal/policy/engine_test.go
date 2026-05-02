package policy

import (
	"context"
	"testing"

	"bridgekeeper/internal/types"
)

// makeEngine is a convenience helper that builds an Engine from inline YAML-style
// structs without touching the filesystem.
func makeEngine(pf *PolicyFile) *Engine {
	return NewEngine(pf)
}

// call builds a ToolCall for test table rows.
func call(tool, action string, args map[string]any) types.ToolCall {
	if args == nil {
		args = map[string]any{}
	}
	return types.ToolCall{
		ID:     "test-id",
		Tool:   tool,
		Action: action,
		Args:   args,
	}
}

func TestEvaluate_AllowedCall(t *testing.T) {
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{Name: "allow-fs-read", Tool: "fs", Actions: []string{"read_file"}, Decision: "allow"},
		},
	}
	eng := makeEngine(pf)

	got := eng.Evaluate(context.Background(), call("fs", "read_file", nil))
	if got.Decision != types.Allow {
		t.Errorf("Decision: want Allow, got %q", got.Decision)
	}
	if got.Rule != "allow-fs-read" {
		t.Errorf("Rule: want %q, got %q", "allow-fs-read", got.Rule)
	}
}

func TestEvaluate_DeniedByDefault(t *testing.T) {
	pf := &PolicyFile{
		Default:      "deny",
		Capabilities: []Capability{},
	}
	eng := makeEngine(pf)

	got := eng.Evaluate(context.Background(), call("shell", "exec", nil))
	if got.Decision != types.Deny {
		t.Errorf("Decision: want Deny, got %q", got.Decision)
	}
	if got.Rule != "default" {
		t.Errorf("Rule: want %q, got %q", "default", got.Rule)
	}
}

func TestEvaluate_AskDecision(t *testing.T) {
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{Name: "ask-writes", Tool: "fs", Actions: []string{"write_file"}, Decision: "ask"},
		},
	}
	eng := makeEngine(pf)

	got := eng.Evaluate(context.Background(), call("fs", "write_file", nil))
	if got.Decision != types.Ask {
		t.Errorf("Decision: want Ask, got %q", got.Decision)
	}
}

func TestEvaluate_ConstraintViolation_PathDeny(t *testing.T) {
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{
				Name:     "guarded-write",
				Tool:     "fs",
				Actions:  []string{"write_file"},
				Decision: "allow",
				Constraints: &Constraints{
					Paths: &AllowDeny{
						Deny: []string{"/etc/**"},
					},
				},
			},
		},
	}
	eng := makeEngine(pf)

	got := eng.Evaluate(context.Background(), call("fs", "write_file", map[string]any{
		"path": "/etc/passwd",
	}))
	if got.Decision != types.Deny {
		t.Errorf("Decision: want Deny (constraint violated), got %q", got.Decision)
	}
	if got.Rule != "guarded-write" {
		t.Errorf("Rule: want %q, got %q", "guarded-write", got.Rule)
	}
}

func TestEvaluate_GlobMatching(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		denyPats   []string
		wantDecide types.Decision
	}{
		{
			name:       "double-star deny matches subpath",
			path:       "/etc/nginx/nginx.conf",
			denyPats:   []string{"/etc/**"},
			wantDecide: types.Deny,
		},
		{
			name:       "double-star deny does not match sibling",
			path:       "/home/user/file.txt",
			denyPats:   []string{"/etc/**"},
			wantDecide: types.Allow,
		},
		{
			name:       "single-star deny matches filename pattern",
			path:       "/tmp/log.txt",
			denyPats:   []string{"/tmp/*.txt"},
			wantDecide: types.Deny,
		},
		{
			name:       "single-star deny does not match different extension",
			path:       "/tmp/log.go",
			denyPats:   []string{"/tmp/*.txt"},
			wantDecide: types.Allow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := &PolicyFile{
				Default: "deny",
				Capabilities: []Capability{
					{
						Name:     "cap",
						Tool:     "fs",
						Actions:  []string{"write_file"},
						Decision: "allow",
						Constraints: &Constraints{
							Paths: &AllowDeny{Deny: tt.denyPats},
						},
					},
				},
			}
			got := makeEngine(pf).Evaluate(context.Background(), call("fs", "write_file", map[string]any{
				"path": tt.path,
			}))
			if got.Decision != tt.wantDecide {
				t.Errorf("path=%q: want %q, got %q", tt.path, tt.wantDecide, got.Decision)
			}
		})
	}
}

func TestEvaluate_FirstMatchWins(t *testing.T) {
	// Two capabilities both match the same call; the first must win.
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{Name: "first", Tool: "git", Actions: []string{"status"}, Decision: "allow"},
			{Name: "second", Tool: "git", Actions: []string{"status"}, Decision: "deny"},
		},
	}
	eng := makeEngine(pf)

	got := eng.Evaluate(context.Background(), call("git", "status", nil))
	if got.Decision != types.Allow {
		t.Errorf("Decision: want Allow (first match), got %q", got.Decision)
	}
	if got.Rule != "first" {
		t.Errorf("Rule: want %q, got %q", "first", got.Rule)
	}
}

func TestEvaluate_DefaultDecision_EmptyDefault(t *testing.T) {
	// When Default is empty the engine must fall back to "deny".
	pf := &PolicyFile{
		Default:      "",
		Capabilities: []Capability{},
	}
	eng := makeEngine(pf)

	got := eng.Evaluate(context.Background(), call("unknown", "action", nil))
	if got.Decision != types.Deny {
		t.Errorf("Decision: want Deny (implicit default), got %q", got.Decision)
	}
}

func TestEvaluate_DefaultDecision_ExplicitAllow(t *testing.T) {
	pf := &PolicyFile{
		Default:      "allow",
		Capabilities: []Capability{},
	}
	eng := makeEngine(pf)

	got := eng.Evaluate(context.Background(), call("mystery", "do-something", nil))
	if got.Decision != types.Allow {
		t.Errorf("Decision: want Allow (explicit default=allow), got %q", got.Decision)
	}
}

func TestEvaluate_InvalidCapabilityDecisionFailsClosed(t *testing.T) {
	pf := &PolicyFile{
		Default: "allow",
		Capabilities: []Capability{
			{Name: "bad-rule", Tool: "fs", Actions: []string{"read_file"}, Decision: "maybe"},
		},
	}
	eng := makeEngine(pf)

	got := eng.Evaluate(context.Background(), call("fs", "read_file", nil))
	if got.Decision != types.Deny {
		t.Errorf("Decision: want Deny for invalid decision, got %q", got.Decision)
	}
	if got.Rule != "bad-rule" {
		t.Errorf("Rule: want bad-rule, got %q", got.Rule)
	}
}

func TestEvaluate_InvalidDefaultFailsClosed(t *testing.T) {
	pf := &PolicyFile{
		Default:      "unknown",
		Capabilities: []Capability{},
	}
	eng := makeEngine(pf)

	got := eng.Evaluate(context.Background(), call("http", "get", nil))
	if got.Decision != types.Deny {
		t.Errorf("Decision: want Deny for invalid default, got %q", got.Decision)
	}
	if got.Rule != "default" {
		t.Errorf("Rule: want default, got %q", got.Rule)
	}
}

func TestEvaluate_DomainMatching(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		denyPats   []string
		wantDecide types.Decision
	}{
		{
			name:       "wildcard subdomain matches",
			url:        "https://api.internal.corp/v1",
			denyPats:   []string{"*.internal.corp"},
			wantDecide: types.Deny,
		},
		{
			name:       "wildcard subdomain does not match root domain",
			url:        "https://internal.corp/v1",
			denyPats:   []string{"*.internal.corp"},
			wantDecide: types.Allow,
		},
		{
			name:       "exact match localhost",
			url:        "http://localhost/api",
			denyPats:   []string{"localhost"},
			wantDecide: types.Deny,
		},
		{
			name:       "exact IP match",
			url:        "http://127.0.0.1:8080/api",
			denyPats:   []string{"127.0.0.1"},
			wantDecide: types.Deny,
		},
		{
			name:       "safe external domain passes",
			url:        "https://api.example.com/data",
			denyPats:   []string{"*.internal.corp", "localhost"},
			wantDecide: types.Allow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf := &PolicyFile{
				Default: "deny",
				Capabilities: []Capability{
					{
						Name:     "http-get",
						Tool:     "http",
						Actions:  []string{"get"},
						Decision: "allow",
						Constraints: &Constraints{
							Domains: &AllowDeny{Deny: tt.denyPats},
						},
					},
				},
			}
			got := makeEngine(pf).Evaluate(context.Background(), call("http", "get", map[string]any{
				"url": tt.url,
			}))
			if got.Decision != tt.wantDecide {
				t.Errorf("url=%q: want %q, got %q (reason: %s)", tt.url, tt.wantDecide, got.Decision, got.Reason)
			}
		})
	}
}

func TestEvaluate_CommandConstraint(t *testing.T) {
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{
				Name:     "safe-shell",
				Tool:     "shell",
				Actions:  []string{"exec"},
				Decision: "allow",
				Constraints: &Constraints{
					Commands: &AllowDeny{
						Allow: []string{"echo *", "cat *", "ls *"},
						Deny:  []string{"rm *", "sudo *"},
					},
				},
			},
		},
	}
	eng := makeEngine(pf)

	t.Run("allowed command", func(t *testing.T) {
		got := eng.Evaluate(context.Background(), call("shell", "exec", map[string]any{
			"command": "ls /tmp",
		}))
		if got.Decision != types.Allow {
			t.Errorf("want Allow, got %q (reason: %s)", got.Decision, got.Reason)
		}
	})

	t.Run("denied command via deny list", func(t *testing.T) {
		got := eng.Evaluate(context.Background(), call("shell", "exec", map[string]any{
			"command": "rm -rf /",
		}))
		if got.Decision != types.Deny {
			t.Errorf("want Deny, got %q (reason: %s)", got.Decision, got.Reason)
		}
	})

	t.Run("command not in allow list", func(t *testing.T) {
		got := eng.Evaluate(context.Background(), call("shell", "exec", map[string]any{
			"command": "curl https://example.com",
		}))
		if got.Decision != types.Deny {
			t.Errorf("want Deny (not in allow list), got %q (reason: %s)", got.Decision, got.Reason)
		}
	})
}

func TestEvaluate_NoActionMatch(t *testing.T) {
	// Capability matches tool but not the action.
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{Name: "git-read", Tool: "git", Actions: []string{"status", "log"}, Decision: "allow"},
		},
	}
	eng := makeEngine(pf)

	got := eng.Evaluate(context.Background(), call("git", "push", nil))
	if got.Decision != types.Deny {
		t.Errorf("Decision: want Deny (action not in capability), got %q", got.Decision)
	}
	if got.Rule != "default" {
		t.Errorf("Rule: want %q, got %q", "default", got.Rule)
	}
}

func TestEvaluate_PathAllowList(t *testing.T) {
	// Capability that only allows writes inside /home/user.
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{
				Name:     "home-write",
				Tool:     "fs",
				Actions:  []string{"write_file"},
				Decision: "allow",
				Constraints: &Constraints{
					Paths: &AllowDeny{
						Allow: []string{"/home/**"},
					},
				},
			},
		},
	}
	eng := makeEngine(pf)

	t.Run("path in allow list", func(t *testing.T) {
		got := eng.Evaluate(context.Background(), call("fs", "write_file", map[string]any{
			"path": "/home/user/notes.txt",
		}))
		if got.Decision != types.Allow {
			t.Errorf("want Allow, got %q (reason: %s)", got.Decision, got.Reason)
		}
	})

	t.Run("path outside allow list", func(t *testing.T) {
		got := eng.Evaluate(context.Background(), call("fs", "write_file", map[string]any{
			"path": "/tmp/notes.txt",
		}))
		if got.Decision != types.Deny {
			t.Errorf("want Deny, got %q (reason: %s)", got.Decision, got.Reason)
		}
	})
}

func TestEvaluate_MaxSizeBytesConstraint(t *testing.T) {
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{
				Name:     "bounded-write",
				Tool:     "fs",
				Actions:  []string{"write_file"},
				Decision: "allow",
				Constraints: &Constraints{
					MaxSizeBytes: 10,
				},
			},
		},
	}
	eng := makeEngine(pf)

	t.Run("content at limit is allowed", func(t *testing.T) {
		got := eng.Evaluate(context.Background(), call("fs", "write_file", map[string]any{
			"content": "1234567890",
		}))
		if got.Decision != types.Allow {
			t.Errorf("want Allow, got %q (reason: %s)", got.Decision, got.Reason)
		}
	})

	t.Run("content over limit is denied", func(t *testing.T) {
		got := eng.Evaluate(context.Background(), call("fs", "write_file", map[string]any{
			"content": "12345678901",
		}))
		if got.Decision != types.Deny {
			t.Errorf("want Deny, got %q (reason: %s)", got.Decision, got.Reason)
		}
	})

	t.Run("body over limit is denied", func(t *testing.T) {
		got := eng.Evaluate(context.Background(), call("fs", "write_file", map[string]any{
			"body": "abcdefghijkl",
		}))
		if got.Decision != types.Deny {
			t.Errorf("want Deny, got %q (reason: %s)", got.Decision, got.Reason)
		}
	})
}

func TestEvaluate_TimeoutSecondsConstraint(t *testing.T) {
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{
				Name:     "bounded-shell",
				Tool:     "shell",
				Actions:  []string{"exec"},
				Decision: "allow",
				Constraints: &Constraints{
					TimeoutSeconds: 30,
				},
			},
		},
	}
	eng := makeEngine(pf)

	t.Run("timeout under limit is allowed", func(t *testing.T) {
		got := eng.Evaluate(context.Background(), call("shell", "exec", map[string]any{
			"timeout": float64(15),
		}))
		if got.Decision != types.Allow {
			t.Errorf("want Allow, got %q (reason: %s)", got.Decision, got.Reason)
		}
	})

	t.Run("timeout at limit is allowed", func(t *testing.T) {
		got := eng.Evaluate(context.Background(), call("shell", "exec", map[string]any{
			"timeout": float64(30),
		}))
		if got.Decision != types.Allow {
			t.Errorf("want Allow, got %q (reason: %s)", got.Decision, got.Reason)
		}
	})

	t.Run("timeout over limit is denied", func(t *testing.T) {
		got := eng.Evaluate(context.Background(), call("shell", "exec", map[string]any{
			"timeout": float64(31),
		}))
		if got.Decision != types.Deny {
			t.Errorf("want Deny, got %q (reason: %s)", got.Decision, got.Reason)
		}
	})
}

func TestEvaluate_PathTraversalGetsNormalizedBeforePolicy(t *testing.T) {
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{
				Name:     "guarded-write",
				Tool:     "fs",
				Actions:  []string{"write_file"},
				Decision: "allow",
				Constraints: &Constraints{
					Paths: &AllowDeny{
						Deny: []string{"/etc/**"},
					},
				},
			},
		},
	}

	got := makeEngine(pf).Evaluate(context.Background(), call("fs", "write_file", map[string]any{
		"path": "/tmp/../etc/passwd",
	}))
	if got.Decision != types.Deny {
		t.Fatalf("want Deny after path normalization, got %q", got.Decision)
	}
}

func TestEvaluate_DomainParsingUsesURLHostname(t *testing.T) {
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{
				Name:     "http-get",
				Tool:     "http",
				Actions:  []string{"get"},
				Decision: "allow",
				Constraints: &Constraints{
					Domains: &AllowDeny{
						Deny: []string{"evil.example"},
					},
				},
			},
		},
	}

	got := makeEngine(pf).Evaluate(context.Background(), call("http", "get", map[string]any{
		"url": "https://user:pass@evil.example:8443/path?q=1",
	}))
	if got.Decision != types.Deny {
		t.Fatalf("want Deny after hostname extraction, got %q", got.Decision)
	}
}

func TestEvaluate_ScopeUsesPolicySubject(t *testing.T) {
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{
				Name:     "developer-shell",
				Tool:     "shell",
				Actions:  []string{"exec"},
				Decision: "allow",
				Scope: &Scope{
					Roles: []string{"developer"},
				},
			},
		},
	}
	eng := makeEngine(pf)

	unscoped := eng.Evaluate(context.Background(), call("shell", "exec", nil))
	if unscoped.Decision != types.Deny || unscoped.Rule != "default" {
		t.Fatalf("unscoped call should fall through to default deny, got %+v", unscoped)
	}

	ctx := WithSubject(context.Background(), types.PolicySubject{Role: "developer", SessionID: "s-1"})
	scoped := eng.Evaluate(ctx, call("shell", "exec", nil))
	if scoped.Decision != types.Allow || scoped.Rule != "developer-shell" {
		t.Fatalf("scoped call should match role rule, got %+v", scoped)
	}
}

func TestEvaluate_ConditionsFilterCapabilities(t *testing.T) {
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{
				Name:     "read-markdown",
				Tool:     "fs",
				Actions:  []string{"read_file"},
				Decision: "allow",
				Conditions: &Condition{
					All: []Condition{
						{Arg: "path", Op: "starts_with", Value: "/workspace/"},
						{Arg: "path", Op: "ends_with", Value: ".md"},
					},
				},
			},
		},
	}
	eng := makeEngine(pf)

	allowed := eng.Evaluate(context.Background(), call("fs", "read_file", map[string]any{"path": "/workspace/README.md"}))
	if allowed.Decision != types.Allow {
		t.Fatalf("markdown path should be allowed, got %+v", allowed)
	}

	denied := eng.Evaluate(context.Background(), call("fs", "read_file", map[string]any{"path": "/workspace/main.go"}))
	if denied.Decision != types.Deny || denied.Rule != "default" {
		t.Fatalf("non-matching condition should fall through to default deny, got %+v", denied)
	}
}

func TestEvaluate_InvalidConditionFailsClosed(t *testing.T) {
	pf := &PolicyFile{
		Default: "allow",
		Capabilities: []Capability{
			{
				Name:        "bad-condition",
				Tool:        "fs",
				Actions:     []string{"read_file"},
				Decision:    "allow",
				Conditions:  &Condition{Field: "args.path", Op: "matches", Value: "["},
				Remediation: "fix the policy regex",
			},
		},
	}

	got := makeEngine(pf).Evaluate(context.Background(), call("fs", "read_file", map[string]any{"path": "README.md"}))
	if got.Decision != types.Deny {
		t.Fatalf("invalid condition should fail closed, got %+v", got)
	}
	if got.Remediation != "fix the policy regex" {
		t.Fatalf("expected remediation to be propagated, got %q", got.Remediation)
	}
}

func TestEvaluate_ArgumentSchemaValidation(t *testing.T) {
	maxLen := 12
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{
				Name:     "bounded-write",
				Tool:     "fs",
				Actions:  []string{"write_file"},
				Decision: "allow",
				Constraints: &Constraints{
					Arguments: map[string]ArgumentSpec{
						"path": {
							Type:     "string",
							Required: true,
							Pattern:  `^/workspace/[^/]+\.txt$`,
						},
						"content": {
							Type:        "string",
							Required:    true,
							MaxLength:   &maxLen,
							Remediation: "write a smaller text file",
						},
					},
				},
			},
		},
	}
	eng := makeEngine(pf)

	allowed := eng.Evaluate(context.Background(), call("fs", "write_file", map[string]any{
		"path":    "/workspace/notes.txt",
		"content": "hello",
	}))
	if allowed.Decision != types.Allow {
		t.Fatalf("valid args should be allowed, got %+v", allowed)
	}

	missing := eng.Evaluate(context.Background(), call("fs", "write_file", map[string]any{
		"content": "hello",
	}))
	if missing.Decision != types.Deny {
		t.Fatalf("missing required arg should deny, got %+v", missing)
	}

	tooLarge := eng.Evaluate(context.Background(), call("fs", "write_file", map[string]any{
		"path":    "/workspace/notes.txt",
		"content": "this is too long",
	}))
	if tooLarge.Decision != types.Deny {
		t.Fatalf("oversized content should deny, got %+v", tooLarge)
	}
	if tooLarge.Remediation != "write a smaller text file" {
		t.Fatalf("schema remediation should be used, got %q", tooLarge.Remediation)
	}
}

func TestEvaluate_MetadataIsReturnedAndApprovalCanForceAsk(t *testing.T) {
	pf := &PolicyFile{
		Default: "deny",
		Capabilities: []Capability{
			{
				Name:      "deploy",
				Tool:      "shell",
				Actions:   []string{"exec"},
				Decision:  "allow",
				RiskLevel: "high",
				Approval: &types.ApprovalMetadata{
					Required:  true,
					Reason:    "deployment changes production",
					Approvers: []string{"ops"},
				},
				Effects: []types.Effect{
					{Type: "process", Resource: "deploy", Mode: "execute"},
				},
				Audit: &types.AuditRequirement{Required: true, Level: "info"},
			},
		},
	}

	got := makeEngine(pf).Evaluate(context.Background(), call("shell", "exec", map[string]any{
		"command": "deploy production",
	}))
	if got.Decision != types.Ask {
		t.Fatalf("approval metadata should force ask, got %+v", got)
	}
	if got.RiskLevel != "high" || got.Approval == nil || len(got.Effects) != 1 || got.Audit == nil {
		t.Fatalf("expected metadata to be returned, got %+v", got)
	}
}
