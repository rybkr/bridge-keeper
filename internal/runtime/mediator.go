package runtime

import (
	"context"
	"fmt"

	"bridgekeeper/internal/audit"
	"bridgekeeper/internal/policy"
	"bridgekeeper/internal/redact"
	"bridgekeeper/internal/sandbox"
	"bridgekeeper/internal/types"
)

// Approver decides whether an ask-policy tool call may proceed.
type Approver interface {
	Approve(context.Context, types.ToolCall, types.PolicyDecision) (bool, error)
}

// Handler executes a mediated tool call once policy and approval allow it.
type Handler func(context.Context, map[string]any) (string, error)

// Mediator is the narrow execution choke point for all tool calls.
type Mediator struct {
	Policy   *policy.Engine
	Approver Approver
	Audit    *audit.Logger
	Sandbox  *sandbox.Validator
	Redactor *redact.Redactor
}

// Execute evaluates policy, optionally requests approval, audits the outcome,
// and runs the supplied handler when allowed.
func (m *Mediator) Execute(ctx context.Context, call types.ToolCall, handler Handler) (string, error) {
	if m == nil || m.Policy == nil {
		return "", fmt.Errorf("runtime mediator is not configured")
	}
	if handler == nil {
		return "", fmt.Errorf("tool handler is not configured")
	}

	call, err := m.validateCall(call)
	if err != nil {
		m.Audit.Log(audit.Warning, "tool_call_rejected_by_sandbox", map[string]any{
			"id":     call.ID,
			"tool":   call.Tool,
			"action": call.Action,
			"error":  err.Error(),
		})
		return denied(types.PolicyDecision{
			Decision: types.Deny,
			Rule:     "sandbox",
			Reason:   err.Error(),
		}), nil
	}

	m.Audit.Log(audit.Info, "tool_call_received", map[string]any{
		"id":     call.ID,
		"tool":   call.Tool,
		"action": call.Action,
		"args":   m.redactValue(call.Args),
	})

	decision := m.Policy.Evaluate(ctx, call)
	m.Audit.Log(audit.Info, "policy_decision", map[string]any{
		"id":       call.ID,
		"tool":     call.Tool,
		"action":   call.Action,
		"decision": decision.Decision,
		"rule":     decision.Rule,
		"reason":   decision.Reason,
	})

	switch decision.Decision {
	case types.Deny:
		return denied(decision), nil
	case types.Ask:
		if m.Approver == nil {
			m.Audit.Log(audit.Warning, "approval_missing", map[string]any{
				"id":     call.ID,
				"tool":   call.Tool,
				"action": call.Action,
			})
			return denied(types.PolicyDecision{
				Decision: types.Deny,
				Rule:     decision.Rule,
				Reason:   "approval required but no approver configured",
			}), nil
		}
		approved, err := m.Approver.Approve(ctx, call, decision)
		if err != nil {
			m.Audit.Log(audit.Error, "approval_error", map[string]any{
				"id":    call.ID,
				"error": err.Error(),
			})
			return "", fmt.Errorf("approval failed: %w", err)
		}
		if !approved {
			m.Audit.Log(audit.Warning, "approval_denied", map[string]any{
				"id":     call.ID,
				"tool":   call.Tool,
				"action": call.Action,
			})
			return denied(types.PolicyDecision{
				Decision: types.Deny,
				Rule:     decision.Rule,
				Reason:   "request denied by approver",
			}), nil
		}
		m.Audit.Log(audit.Info, "approval_granted", map[string]any{
			"id":     call.ID,
			"tool":   call.Tool,
			"action": call.Action,
		})
	}

	result, err := handler(ctx, call.Args)
	if err != nil {
		m.Audit.Log(audit.Error, "tool_execution_failed", map[string]any{
			"id":     call.ID,
			"tool":   call.Tool,
			"action": call.Action,
			"error":  err.Error(),
		})
		return "", err
	}
	if err := m.validateResult(result); err != nil {
		m.Audit.Log(audit.Warning, "tool_result_rejected_by_sandbox", map[string]any{
			"id":     call.ID,
			"tool":   call.Tool,
			"action": call.Action,
			"error":  err.Error(),
		})
		return denied(types.PolicyDecision{
			Decision: types.Deny,
			Rule:     "sandbox",
			Reason:   err.Error(),
		}), nil
	}

	classification := m.detect(result)
	safeResult := result
	if classification.Sensitive {
		safeResult = m.redactText(result)
	}

	m.Audit.Log(audit.Info, "tool_execution_succeeded", map[string]any{
		"id":     call.ID,
		"tool":   call.Tool,
		"action": call.Action,
		"taint":  classification,
	})
	return safeResult, nil
}

func denied(decision types.PolicyDecision) string {
	return fmt.Sprintf("Error: execution denied. Reason: %s", decision.Reason)
}

func (m *Mediator) validateCall(call types.ToolCall) (types.ToolCall, error) {
	if m == nil || m.Sandbox == nil {
		return call, nil
	}
	return m.Sandbox.ValidateToolCall(call)
}

func (m *Mediator) validateResult(result string) error {
	if m == nil || m.Sandbox == nil {
		return nil
	}
	return m.Sandbox.ValidateToolResult(result)
}

func (m *Mediator) redactText(text string) string {
	if m == nil || m.Redactor == nil {
		return text
	}
	return m.Redactor.RedactText(text)
}

func (m *Mediator) redactValue(value any) any {
	if m == nil || m.Redactor == nil {
		return value
	}
	return m.Redactor.RedactValue(value)
}

func (m *Mediator) detect(text string) redact.Classification {
	if m == nil || m.Redactor == nil {
		return redact.Classification{}
	}
	return m.Redactor.Detect(text)
}
