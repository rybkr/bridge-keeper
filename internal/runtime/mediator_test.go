package runtime

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"bridgekeeper/internal/audit"
	"bridgekeeper/internal/policy"
	"bridgekeeper/internal/types"
)

type stubApprover struct {
	approved bool
}

func (s stubApprover) Approve(_ context.Context, _ types.ToolCall, _ types.PolicyDecision) (bool, error) {
	return s.approved, nil
}

func TestMediatorExecute_Deny(t *testing.T) {
	pf := &policy.PolicyFile{
		Default: "deny",
	}

	var auditOut bytes.Buffer
	mediator := &Mediator{
		Policy: policy.NewEngine(pf),
		Audit:  audit.NewLogger(&auditOut, audit.Info),
	}

	result, err := mediator.Execute(context.Background(), types.ToolCall{
		ID:     "1",
		Tool:   "fs",
		Action: "write_file",
	}, func(context.Context, map[string]any) (string, error) {
		t.Fatal("handler should not run")
		return "", nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result, "execution denied") {
		t.Fatalf("unexpected result %q", result)
	}
}

func TestMediatorExecute_AskAndApprove(t *testing.T) {
	pf := &policy.PolicyFile{
		Default: "deny",
		Capabilities: []policy.Capability{
			{Name: "write", Tool: "fs", Actions: []string{"write_file"}, Decision: "ask"},
		},
	}

	mediator := &Mediator{
		Policy:   policy.NewEngine(pf),
		Approver: stubApprover{approved: true},
		Audit:    audit.NewLogger(&bytes.Buffer{}, audit.Info),
	}

	result, err := mediator.Execute(context.Background(), types.ToolCall{
		ID:     "2",
		Tool:   "fs",
		Action: "write_file",
	}, func(context.Context, map[string]any) (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result != "ok" {
		t.Fatalf("result = %q, want ok", result)
	}
}

func TestMediatorExecute_AskAndDeny(t *testing.T) {
	pf := &policy.PolicyFile{
		Default: "deny",
		Capabilities: []policy.Capability{
			{Name: "write", Tool: "fs", Actions: []string{"write_file"}, Decision: "ask"},
		},
	}

	mediator := &Mediator{
		Policy:   policy.NewEngine(pf),
		Approver: stubApprover{approved: false},
		Audit:    audit.NewLogger(&bytes.Buffer{}, audit.Info),
	}

	result, err := mediator.Execute(context.Background(), types.ToolCall{
		ID:     "3",
		Tool:   "fs",
		Action: "write_file",
	}, func(context.Context, map[string]any) (string, error) {
		t.Fatal("handler should not run")
		return "", nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result, "request denied by approver") {
		t.Fatalf("unexpected result %q", result)
	}
}
