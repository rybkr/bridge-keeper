package policy

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"bridgekeeper/internal/types"
)

// Engine evaluates tool calls against a loaded PolicyFile.
// It is safe for concurrent use after construction — all state is read-only.
type Engine struct {
	policy *PolicyFile
}

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
func (e *Engine) Evaluate(_ context.Context, call types.ToolCall) types.PolicyDecision {
	for _, cap := range e.policy.Capabilities {
		if !capabilityMatches(cap, call) {
			continue
		}

		// Capability matched — check constraints before honoring its decision.
		if cap.Constraints != nil {
			if violation, ok := checkConstraints(cap.Constraints, call); !ok {
				return types.PolicyDecision{
					Decision: types.Deny,
					Reason:   violation,
					Rule:     cap.Name,
				}
			}
		}

		return types.PolicyDecision{
			Decision: types.Decision(cap.Decision),
			Reason:   fmt.Sprintf("matched capability %q", cap.Name),
			Rule:     cap.Name,
		}
	}

	// No capability matched — fall back to file-level default.
	def := e.policy.Default
	if def == "" {
		def = string(types.Deny)
	}

	return types.PolicyDecision{
		Decision: types.Decision(def),
		Reason:   "no matching capability; using default decision",
		Rule:     "default",
	}
}

// capabilityMatches returns true when cap covers the tool and action of call.
func capabilityMatches(cap Capability, call types.ToolCall) bool {
	if cap.Tool != call.Tool {
		return false
	}
	for _, a := range cap.Actions {
		if a == call.Action {
			return true
		}
	}
	return false
}

// checkConstraints evaluates all non-nil constraint groups against call.
// It returns a human-readable violation message and false on the first
// violation found. On success it returns ("", true).
func checkConstraints(c *Constraints, call types.ToolCall) (string, bool) {
	// Path constraint: look for a "path" arg in the call arguments.
	if c.Paths != nil {
		if rawPath, ok := call.Args["path"]; ok {
			path, _ := rawPath.(string)
			if msg, ok := checkAllowDenyGlob(c.Paths, path, "path"); !ok {
				return msg, false
			}
		}
	}

	// Command constraint: look for a "command" arg in the call arguments.
	// Shell-style matching is used here so that patterns like "ls *" match
	// "ls /some/path" — filepath.Match would fail because its * stops at '/'.
	if c.Commands != nil {
		if rawCmd, ok := call.Args["command"]; ok {
			cmd, _ := rawCmd.(string)
			if msg, ok := checkAllowDenyShell(c.Commands, cmd, "command"); !ok {
				return msg, false
			}
		}
	}

	// Domain constraint: look for "domain", "host", or "url" in args.
	if c.Domains != nil {
		domain := extractDomain(call.Args)
		if domain != "" {
			if msg, ok := checkDomain(c.Domains, domain); !ok {
				return msg, false
			}
		}
	}

	// Max payload size constraint: applies to common request body fields used
	// by tools that send or write content.
	if c.MaxSizeBytes > 0 {
		if size, key := payloadSize(call.Args); key != "" && size > c.MaxSizeBytes {
			return fmt.Sprintf("%s payload size %d exceeds max_size_bytes %d", key, size, c.MaxSizeBytes), false
		}
	}

	// Timeout constraint: if a timeout arg is supplied by the caller it must
	// not exceed the capability-level timeout limit.
	if c.TimeoutSeconds > 0 {
		if timeout, ok := timeoutSeconds(call.Args); ok && timeout > c.TimeoutSeconds {
			return fmt.Sprintf("timeout %d exceeds timeout_seconds %d", timeout, c.TimeoutSeconds), false
		}
	}

	return "", true
}

// payloadSize returns the byte size of the first known payload arg present in
// args and the corresponding arg key. If no known payload arg exists it returns
// (0, "").
func payloadSize(args map[string]any) (int64, string) {
	for _, key := range []string{"content", "body", "payload"} {
		raw, ok := args[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case string:
			return int64(len(v)), key
		case []byte:
			return int64(len(v)), key
		}
	}
	return 0, ""
}

// timeoutSeconds extracts a timeout value from args and normalizes it into
// seconds. Returns (0, false) when timeout is missing or malformed.
func timeoutSeconds(args map[string]any) (int, bool) {
	raw, ok := args["timeout"]
	if !ok {
		return 0, false
	}

	switch v := raw.(type) {
	case float64:
		if v > 0 {
			return int(v), true
		}
	case int:
		if v > 0 {
			return v, true
		}
	case int64:
		if v > 0 {
			return int(v), true
		}
	}

	return 0, false
}

// checkAllowDenyGlob applies an AllowDeny rule to value using path glob patterns.
// It is used for path constraints where filepath.Match semantics are appropriate
// (single * does not cross directory boundaries).
// Deny patterns are evaluated before allow patterns (deny takes precedence).
// If an allow list is present, the value must match at least one allow pattern.
func checkAllowDenyGlob(ad *AllowDeny, value, label string) (string, bool) {
	// Check deny patterns first — a match here is always a violation.
	for _, pattern := range ad.Deny {
		if matchGlob(pattern, value) {
			return fmt.Sprintf("%s %q matches deny pattern %q", label, value, pattern), false
		}
	}

	// If an allow list is specified the value must appear in it.
	if len(ad.Allow) > 0 {
		for _, pattern := range ad.Allow {
			if matchGlob(pattern, value) {
				return "", true
			}
		}
		return fmt.Sprintf("%s %q does not match any allow pattern", label, value), false
	}

	return "", true
}

// checkAllowDenyShell applies an AllowDeny rule using shell-style glob matching
// where * matches any sequence of characters including path separators. This is
// appropriate for command constraints where patterns like "ls *" must match
// "ls /some/path" — a use case that filepath.Match cannot handle because its
// single * stops at '/'.
func checkAllowDenyShell(ad *AllowDeny, value, label string) (string, bool) {
	// Deny patterns take precedence.
	for _, pattern := range ad.Deny {
		if matchShellGlob(pattern, value) {
			return fmt.Sprintf("%s %q matches deny pattern %q", label, value, pattern), false
		}
	}

	// If an allow list is specified the value must appear in at least one pattern.
	if len(ad.Allow) > 0 {
		for _, pattern := range ad.Allow {
			if matchShellGlob(pattern, value) {
				return "", true
			}
		}
		return fmt.Sprintf("%s %q does not match any allow pattern", label, value), false
	}

	return "", true
}

// matchGlob matches value against pattern using filepath.Match extended with
// rudimentary ** support:
//
//   - If the pattern contains "**", everything up to (and including) "**" is
//     treated as a path prefix. The value matches if it starts with that prefix.
//     This covers the common policy idiom "/etc/**" meaning "anything under /etc/".
//   - For patterns without "**", filepath.Match is used verbatim.
func matchGlob(pattern, value string) bool {
	if strings.Contains(pattern, "**") {
		// Split on the first occurrence of "**".
		parts := strings.SplitN(pattern, "**", 2)
		prefix := parts[0]
		// The value must start with the prefix (e.g. "/etc/").
		return strings.HasPrefix(value, prefix)
	}

	// Standard single-level glob via filepath.Match.
	matched, err := filepath.Match(pattern, value)
	if err != nil {
		// filepath.Match only errors on malformed patterns; treat as no-match.
		return false
	}
	return matched
}

// matchShellGlob matches value against a shell-style pattern where * matches
// any sequence of characters, including '/' (unlike filepath.Match).
// It handles the "**" idiom as a pure prefix match for consistency with matchGlob.
func matchShellGlob(pattern, value string) bool {
	if strings.Contains(pattern, "**") {
		prefix := strings.SplitN(pattern, "**", 2)[0]
		return strings.HasPrefix(value, prefix)
	}

	// Manually implement glob: walk through pattern and value simultaneously.
	return shellGlobMatch(pattern, value)
}

// shellGlobMatch is a recursive glob matcher where * matches any sequence of
// characters (including none), and ? matches any single character.
// Unlike filepath.Match, the / character has no special meaning here.
func shellGlobMatch(pattern, value string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			// Skip consecutive stars.
			for len(pattern) > 0 && pattern[0] == '*' {
				pattern = pattern[1:]
			}
			if len(pattern) == 0 {
				// Trailing * matches everything remaining.
				return true
			}
			// Try matching the rest of the pattern against every suffix of value.
			for i := 0; i <= len(value); i++ {
				if shellGlobMatch(pattern, value[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(value) == 0 {
				return false
			}
			pattern = pattern[1:]
			value = value[1:]
		default:
			if len(value) == 0 || pattern[0] != value[0] {
				return false
			}
			pattern = pattern[1:]
			value = value[1:]
		}
	}
	return len(value) == 0
}

// extractDomain pulls a domain/host value out of a tool call's args.
// It checks "domain", "host", and "url" keys in that priority order.
// For "url" values it extracts just the hostname portion.
func extractDomain(args map[string]any) string {
	for _, key := range []string{"domain", "host"} {
		if v, ok := args[key]; ok {
			if s, _ := v.(string); s != "" {
				return s
			}
		}
	}

	// Fall back to parsing "url" — extract host segment only.
	if v, ok := args["url"]; ok {
		if s, _ := v.(string); s != "" {
			return hostFromURL(s)
		}
	}

	return ""
}

// hostFromURL returns just the host portion of a URL string without
// importing net/url — we only need to strip the scheme and path.
// e.g. "https://api.example.com/v1" → "api.example.com"
func hostFromURL(rawURL string) string {
	// Strip scheme.
	if i := strings.Index(rawURL, "://"); i >= 0 {
		rawURL = rawURL[i+3:]
	}
	// Strip path, query, fragment.
	if i := strings.IndexAny(rawURL, "/?#"); i >= 0 {
		rawURL = rawURL[:i]
	}
	// Strip port.
	if i := strings.LastIndex(rawURL, ":"); i >= 0 {
		rawURL = rawURL[:i]
	}
	return rawURL
}

// checkDomain applies an AllowDeny rule to a domain value.
// Patterns may be exact ("localhost", "127.0.0.1") or wildcard ("*.example.com").
func checkDomain(ad *AllowDeny, domain string) (string, bool) {
	// Deny patterns take precedence.
	for _, pattern := range ad.Deny {
		if matchDomain(pattern, domain) {
			return fmt.Sprintf("domain %q matches deny pattern %q", domain, pattern), false
		}
	}

	// If an allow list is present the domain must appear in it.
	if len(ad.Allow) > 0 {
		for _, pattern := range ad.Allow {
			if matchDomain(pattern, domain) {
				return "", true
			}
		}
		return fmt.Sprintf("domain %q does not match any allow pattern", domain), false
	}

	return "", true
}

// matchDomain matches a domain against a pattern.
// A leading "*." wildcard matches any single subdomain prefix:
//   - "*.example.com" matches "api.example.com" but NOT "example.com" or
//     "deep.api.example.com" (only one subdomain level).
//
// Exact patterns match case-insensitively.
func matchDomain(pattern, domain string) bool {
	pattern = strings.ToLower(pattern)
	domain = strings.ToLower(domain)

	if strings.HasPrefix(pattern, "*.") {
		// Wildcard: domain must end with the suffix after "*" (i.e. ".example.com")
		// and have exactly one label in front of it.
		suffix := pattern[1:] // ".example.com"
		if !strings.HasSuffix(domain, suffix) {
			return false
		}
		// The part before the suffix must not contain another dot (one level only).
		prefix := domain[:len(domain)-len(suffix)]
		return prefix != "" && !strings.Contains(prefix, ".")
	}

	return pattern == domain
}
