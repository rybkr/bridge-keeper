package hitl

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"bridgekeeper/internal/types"
)

// AutoApprover automatically approves requests (used when --no-hitl flag is passed).
type AutoApprover struct{}

// AutoDenier automatically denies requests (used as a safe fallback if the terminal is unavailable).
type AutoDenier struct{}

// TerminalApprover prompts a human user in the terminal to approve or deny an action.
type TerminalApprover struct {
	in  *os.File
	out *os.File
}

// NewTerminalApprover initializes a new TerminalApprover.
// It returns the approver and an error, matching the signature expected in main.go.
func NewTerminalApprover() (*TerminalApprover, error) {
	in, err := os.Open("/dev/tty")
	if err != nil {
		return nil, err
	}
	out, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		_ = in.Close()
		return nil, err
	}
	return &TerminalApprover{in: in, out: out}, nil
}

func (a *AutoApprover) Approve(_ context.Context, _ types.ToolCall, _ types.PolicyDecision) (bool, error) {
	return true, nil
}

func (a *AutoDenier) Approve(_ context.Context, _ types.ToolCall, _ types.PolicyDecision) (bool, error) {
	return false, nil
}

func (t *TerminalApprover) Approve(ctx context.Context, call types.ToolCall, decision types.PolicyDecision) (bool, error) {
	if t == nil || t.in == nil || t.out == nil {
		return false, fmt.Errorf("terminal approver is not initialized")
	}

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	if _, err := fmt.Fprintf(t.out, "Approve tool call? tool=%s action=%s reason=%s [y/N]: ", call.Tool, call.Action, decision.Reason); err != nil {
		return false, err
	}

	reader := bufio.NewReader(t.in)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
