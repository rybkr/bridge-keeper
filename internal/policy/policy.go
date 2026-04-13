/* policy package enforces policies on tool calls from LLMs */
package policy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// NewEngine constructs an Engine from a parsed PolicyFile. The engine holds a
// reference to policy; callers should not mutate the PolicyFile after passing
// it here.
func NewEngine(policy *PolicyFile) *Engine {
	return &Engine{policy: policy}
}

// Evaluate checks call against the policy and returns a PolicyDecision.
//
// Evaluation order:
//  1. Iterate capabilities in declaration order; the first capability whose
//     tool and action match the call is selected (first-match-wins).
//  2. If a matching capability has constraints, each non-nil constraint group
//     is checked. Any violation produces an immediate Deny.
//  3. If no constraint is violated the capability's own decision is returned.
//  4. If no capability matched, the file-level Default decision is used
//     (falling back to "deny" when Default is empty).
func (e *Engine) Evaluate(_ context.Context, call ToolCall) PolicyDecision {
	for _, cap := range e.policy.Capabilities {
		if !capabilityMatches(cap, call) {
			continue
		}

		// Capability matched — check constraints before honoring its decision.
		if cap.Constraints != nil {
			if violation, ok := checkConstraints(cap.Constraints, call); !ok {
				return PolicyDecision{
					Decision: Deny,
					Reason:   violation,
					Rule:     cap.Name,
				}
			}
		}

		decision, normalized := normalizeDecision(cap.Decision)
		reason := fmt.Sprintf("matched capability %q", cap.Name)
		if !normalized {
			reason = fmt.Sprintf("invalid capability decision %q for %q; failing closed to deny", cap.Decision, cap.Name)
		}

		return PolicyDecision{
			Decision: decision,
			Reason:   reason,
			Rule:     cap.Name,
		}
	}

	// No capability matched — fall back to file-level default.
	def, normalized := normalizeDecision(e.policy.Default)
	reason := "no matching capability; using default decision"
	if !normalized && strings.TrimSpace(e.policy.Default) != "" {
		reason = fmt.Sprintf("no matching capability; invalid default decision %q so failing closed to deny", e.policy.Default)
	}

	return PolicyDecision{
		Decision: def,
		Reason:   reason,
		Rule:     "default",
	}
}

// LoadPath loads and parses a PolicyFile from a file or directory path.
func LoadPath(path string) (*PolicyFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// If the user provided a directory (like "policies"), default to looking for "default.yaml" inside it.
	if info.IsDir() {
		path = filepath.Join(path, "default.yaml")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy file: %w", err)
	}

	var pf PolicyFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("failed to parse policy YAML: %w", err)
	}

	return &pf, nil
}
