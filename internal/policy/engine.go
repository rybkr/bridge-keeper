package policy

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"bridgekeeper/internal/types"
)

type subjectContextKey struct{}

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

// WithSubject returns a context carrying the authenticated policy subject used
// by scoped policy rules.
func WithSubject(ctx context.Context, subject types.PolicySubject) context.Context {
	return context.WithValue(ctx, subjectContextKey{}, subject)
}

// SubjectFromContext returns the policy subject attached by WithSubject.
func SubjectFromContext(ctx context.Context) (types.PolicySubject, bool) {
	subject, ok := ctx.Value(subjectContextKey{}).(types.PolicySubject)
	return subject, ok
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
func (e *Engine) Evaluate(ctx context.Context, call types.ToolCall) types.PolicyDecision {
	subject, _ := SubjectFromContext(ctx)

	for _, cap := range e.policy.Capabilities {
		if !capabilityMatches(cap, call) {
			continue
		}

		if !scopeMatches(cap.Scope, subject) {
			continue
		}

		if cap.Conditions != nil {
			matched, err := evaluateCondition(cap.Conditions, call, subject, cap)
			if err != nil {
				return denyForCapability(cap, fmt.Sprintf("condition evaluation failed: %v", err), conditionRemediation(cap.Conditions, cap))
			}
			if !matched {
				continue
			}
		}

		// Capability matched — check constraints before honoring its decision.
		if cap.Constraints != nil {
			if violation, ok := checkConstraints(cap.Constraints, call); !ok {
				return denyForCapability(cap, violation.Reason, violation.Remediation)
			}
		}

		decision, normalized := normalizeDecision(cap.Decision)
		reason := fmt.Sprintf("matched capability %q", cap.Name)
		if !normalized {
			reason = fmt.Sprintf("invalid capability decision %q for %q; failing closed to deny", cap.Decision, cap.Name)
		}
		if cap.Approval != nil && cap.Approval.Required && decision == types.Allow {
			decision = types.Ask
			reason += "; approval required by policy metadata"
		}

		return decisionForCapability(cap, decision, reason, "")
	}

	// No capability matched — fall back to file-level default.
	def, normalized := normalizeDecision(e.policy.Default)
	reason := "no matching capability; using default decision"
	if !normalized && strings.TrimSpace(e.policy.Default) != "" {
		reason = fmt.Sprintf("no matching capability; invalid default decision %q so failing closed to deny", e.policy.Default)
	}

	return types.PolicyDecision{
		Decision: def,
		Reason:   reason,
		Rule:     "default",
	}
}

func decisionForCapability(cap Capability, decision types.Decision, reason string, remediation string) types.PolicyDecision {
	if remediation == "" {
		remediation = cap.Remediation
	}
	return types.PolicyDecision{
		Decision:    decision,
		Reason:      reason,
		Rule:        cap.Name,
		RiskLevel:   cap.RiskLevel,
		Approval:    cap.Approval,
		Effects:     cap.Effects,
		Audit:       cap.Audit,
		Remediation: remediation,
	}
}

func denyForCapability(cap Capability, reason string, remediation string) types.PolicyDecision {
	return decisionForCapability(cap, types.Deny, reason, remediation)
}

// normalizeDecision converts an input decision to a known enum and defaults to
// deny for empty or unknown values.
func normalizeDecision(raw string) (types.Decision, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(types.Allow):
		return types.Allow, true
	case string(types.Ask):
		return types.Ask, true
	case string(types.Deny):
		return types.Deny, true
	default:
		return types.Deny, false
	}
}

func evaluateCondition(cond *Condition, call types.ToolCall, subject types.PolicySubject, cap Capability) (bool, error) {
	if cond == nil {
		return true, nil
	}

	for i := range cond.All {
		ok, err := evaluateCondition(&cond.All[i], call, subject, cap)
		if err != nil || !ok {
			return ok, err
		}
	}

	if len(cond.Any) > 0 {
		anyMatched := false
		for i := range cond.Any {
			ok, err := evaluateCondition(&cond.Any[i], call, subject, cap)
			if err != nil {
				return false, err
			}
			if ok {
				anyMatched = true
				break
			}
		}
		if !anyMatched {
			return false, nil
		}
	}

	if cond.Not != nil {
		ok, err := evaluateCondition(cond.Not, call, subject, cap)
		if err != nil {
			return false, err
		}
		if ok {
			return false, nil
		}
	}

	if cond.Field == "" && cond.Arg == "" {
		if len(cond.All) > 0 || len(cond.Any) > 0 || cond.Not != nil {
			return true, nil
		}
		return false, fmt.Errorf("condition leaf must set field or arg")
	}

	field := cond.Field
	if field == "" {
		field = "args." + cond.Arg
	}
	value, present, err := conditionFieldValue(field, call, subject, cap)
	if err != nil {
		return false, err
	}

	op := strings.ToLower(strings.TrimSpace(cond.Op))
	if op == "" {
		op = "eq"
	}
	return compareConditionValue(value, present, op, cond)
}

func conditionFieldValue(field string, call types.ToolCall, subject types.PolicySubject, cap Capability) (any, bool, error) {
	switch field {
	case "tool":
		return call.Tool, true, nil
	case "action":
		return call.Action, true, nil
	case "role":
		return subject.Role, subject.Role != "", nil
	case "session", "session_id":
		return subject.SessionID, subject.SessionID != "", nil
	case "risk", "risk_level":
		return cap.RiskLevel, cap.RiskLevel != "", nil
	}

	if strings.HasPrefix(field, "args.") {
		return nestedMapValue(call.Args, strings.TrimPrefix(field, "args."))
	}
	if strings.HasPrefix(field, "arg.") {
		return nestedMapValue(call.Args, strings.TrimPrefix(field, "arg."))
	}

	return nil, false, fmt.Errorf("unknown condition field %q", field)
}

func compareConditionValue(value any, present bool, op string, cond *Condition) (bool, error) {
	switch op {
	case "exists":
		return present, nil
	case "not_exists":
		return !present, nil
	}
	if !present {
		return false, nil
	}

	switch op {
	case "eq":
		return scalarEqual(value, cond.Value), nil
	case "ne":
		return !scalarEqual(value, cond.Value), nil
	case "in":
		for _, item := range conditionValues(cond) {
			if scalarEqual(value, item) {
				return true, nil
			}
		}
		return false, nil
	case "not_in":
		for _, item := range conditionValues(cond) {
			if scalarEqual(value, item) {
				return false, nil
			}
		}
		return true, nil
	case "contains":
		return containsValue(value, cond.Value), nil
	case "starts_with":
		text, prefix, ok := twoStrings(value, cond.Value)
		return ok && strings.HasPrefix(text, prefix), nil
	case "ends_with":
		text, suffix, ok := twoStrings(value, cond.Value)
		return ok && strings.HasSuffix(text, suffix), nil
	case "matches":
		text, pattern, ok := twoStrings(value, cond.Value)
		if !ok {
			return false, nil
		}
		matched, err := regexp.MatchString(pattern, text)
		if err != nil {
			return false, fmt.Errorf("invalid condition regex %q", pattern)
		}
		return matched, nil
	case "gt", "gte", "lt", "lte":
		left, ok := numberAsFloat(value)
		if !ok {
			return false, nil
		}
		right, ok := numberAsFloat(cond.Value)
		if !ok {
			return false, fmt.Errorf("condition %s requires numeric value", op)
		}
		switch op {
		case "gt":
			return left > right, nil
		case "gte":
			return left >= right, nil
		case "lt":
			return left < right, nil
		default:
			return left <= right, nil
		}
	default:
		return false, fmt.Errorf("unknown condition op %q", op)
	}
}

func conditionValues(cond *Condition) []any {
	if len(cond.Values) > 0 {
		return cond.Values
	}
	if values, ok := cond.Value.([]any); ok {
		return values
	}
	return []any{cond.Value}
}

func nestedMapValue(values map[string]any, path string) (any, bool, error) {
	if path == "" {
		return nil, false, fmt.Errorf("argument condition field cannot be empty")
	}
	parts := strings.Split(path, ".")
	var current any = values
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		current, ok = m[part]
		if !ok {
			return nil, false, nil
		}
	}
	return current, true, nil
}

func conditionRemediation(cond *Condition, cap Capability) string {
	if cond == nil {
		return cap.Remediation
	}
	if cond.Remediation != "" {
		return cond.Remediation
	}
	for i := range cond.All {
		if remediation := conditionRemediation(&cond.All[i], cap); remediation != "" {
			return remediation
		}
	}
	for i := range cond.Any {
		if remediation := conditionRemediation(&cond.Any[i], cap); remediation != "" {
			return remediation
		}
	}
	if cond.Not != nil {
		return conditionRemediation(cond.Not, cap)
	}
	return cap.Remediation
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

func scopeMatches(scope *Scope, subject types.PolicySubject) bool {
	if scope == nil {
		return true
	}
	if len(scope.Roles) > 0 && !containsString(scope.Roles, subject.Role) {
		return false
	}
	if len(scope.Sessions) > 0 && !containsString(scope.Sessions, subject.SessionID) {
		return false
	}
	if len(scope.Labels) > 0 && !intersectsString(scope.Labels, subject.Labels) {
		return false
	}
	return true
}

type policyViolation struct {
	Reason      string
	Remediation string
}

// checkConstraints evaluates all non-nil constraint groups against call.
// It returns a structured violation and false on the first violation found.
// On success it returns a zero violation and true.
func checkConstraints(c *Constraints, call types.ToolCall) (policyViolation, bool) {
	if len(c.Arguments) > 0 {
		if violation, ok := checkArgumentSchema(c, call.Args); !ok {
			return violation, false
		}
	}

	// Path constraint: look for a "path" arg in the call arguments.
	if c.Paths != nil {
		if rawPath, ok := call.Args["path"]; ok {
			path, _ := rawPath.(string)
			path = normalizePath(path)
			if msg, ok := checkAllowDenyGlob(c.Paths, path, "path"); !ok {
				return violation(msg, firstNonEmpty(c.Paths.Remediation, c.Remediation)), false
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
				return violation(msg, firstNonEmpty(c.Commands.Remediation, c.Remediation)), false
			}
		}
	}

	// Domain constraint: look for "domain", "host", or "url" in args.
	if c.Domains != nil {
		domain := extractDomain(call.Args)
		if domain != "" {
			if msg, ok := checkDomain(c.Domains, domain); !ok {
				return violation(msg, firstNonEmpty(c.Domains.Remediation, c.Remediation)), false
			}
		}
	}

	// Max payload size constraint: applies to common request body fields used
	// by tools that send or write content.
	if c.MaxSizeBytes > 0 {
		if size, key := payloadSize(call.Args); key != "" && size > c.MaxSizeBytes {
			return violation(fmt.Sprintf("%s payload size %d exceeds max_size_bytes %d", key, size, c.MaxSizeBytes), c.Remediation), false
		}
	}

	// Timeout constraint: if a timeout arg is supplied by the caller it must
	// not exceed the capability-level timeout limit.
	if c.TimeoutSeconds > 0 {
		if timeout, ok := timeoutSeconds(call.Args); ok && timeout > c.TimeoutSeconds {
			return violation(fmt.Sprintf("timeout %d exceeds timeout_seconds %d", timeout, c.TimeoutSeconds), c.Remediation), false
		}
	}

	return policyViolation{}, true
}

func violation(reason string, remediation string) policyViolation {
	return policyViolation{Reason: reason, Remediation: remediation}
}

func checkArgumentSchema(c *Constraints, args map[string]any) (policyViolation, bool) {
	for name, spec := range c.Arguments {
		value, ok := args[name]
		if !ok {
			if spec.Required {
				return violation(fmt.Sprintf("argument %q is required", name), firstNonEmpty(spec.Remediation, c.Remediation)), false
			}
			continue
		}
		if reason, ok := validateArgument(value, spec, "argument "+strconv.Quote(name)); !ok {
			return violation(reason, firstNonEmpty(spec.Remediation, c.Remediation)), false
		}
	}

	if !c.AllowUnknownArgs {
		for name := range args {
			if _, ok := c.Arguments[name]; !ok {
				return violation(fmt.Sprintf("argument %q is not allowed by schema", name), c.Remediation), false
			}
		}
	}

	return policyViolation{}, true
}

func validateArgument(value any, spec ArgumentSpec, label string) (string, bool) {
	if spec.Type != "" && !argumentTypeMatches(value, spec.Type) {
		return fmt.Sprintf("%s must be %s", label, spec.Type), false
	}

	if len(spec.Enum) > 0 {
		allowedValue := false
		for _, allowed := range spec.Enum {
			if scalarEqual(value, allowed) {
				allowedValue = true
				break
			}
		}
		if !allowedValue {
			return fmt.Sprintf("%s is not one of the allowed values", label), false
		}
	}

	if spec.Pattern != "" {
		text, ok := value.(string)
		if !ok {
			return fmt.Sprintf("%s must be string to match pattern", label), false
		}
		matched, err := regexp.MatchString(spec.Pattern, text)
		if err != nil {
			return fmt.Sprintf("%s has invalid schema pattern %q", label, spec.Pattern), false
		}
		if !matched {
			return fmt.Sprintf("%s does not match required pattern", label), false
		}
	}

	if spec.Min != nil || spec.Max != nil {
		number, ok := numberAsFloat(value)
		if !ok {
			return fmt.Sprintf("%s must be numeric for min/max validation", label), false
		}
		if spec.Min != nil && number < *spec.Min {
			return fmt.Sprintf("%s must be at least %v", label, trimFloat(*spec.Min)), false
		}
		if spec.Max != nil && number > *spec.Max {
			return fmt.Sprintf("%s must be at most %v", label, trimFloat(*spec.Max)), false
		}
	}

	if spec.MinLength != nil || spec.MaxLength != nil {
		length, ok := valueLength(value)
		if !ok {
			return fmt.Sprintf("%s must have a measurable length", label), false
		}
		if spec.MinLength != nil && length < *spec.MinLength {
			return fmt.Sprintf("%s length must be at least %d", label, *spec.MinLength), false
		}
		if spec.MaxLength != nil && length > *spec.MaxLength {
			return fmt.Sprintf("%s length must be at most %d", label, *spec.MaxLength), false
		}
	}

	if spec.Items != nil {
		items, ok := sliceValues(value)
		if !ok {
			return fmt.Sprintf("%s must be an array for item validation", label), false
		}
		for i, item := range items {
			if reason, ok := validateArgument(item, *spec.Items, fmt.Sprintf("%s[%d]", label, i)); !ok {
				return reason, false
			}
		}
	}

	if len(spec.Properties) > 0 {
		props, ok := value.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s must be an object for property validation", label), false
		}
		for name, child := range spec.Properties {
			childValue, ok := props[name]
			if !ok {
				if child.Required {
					return fmt.Sprintf("%s.%s is required", label, name), false
				}
				continue
			}
			if reason, ok := validateArgument(childValue, child, label+"."+name); !ok {
				return reason, false
			}
		}
	}

	return "", true
}

func argumentTypeMatches(value any, want string) bool {
	switch strings.ToLower(strings.TrimSpace(want)) {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := numberAsFloat(value)
		return ok
	case "integer":
		return isInteger(value)
	case "boolean", "bool":
		_, ok := value.(bool)
		return ok
	case "array":
		_, ok := sliceValues(value)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	default:
		return false
	}
}

func sliceValues(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case []string:
		out := make([]any, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out, true
	case []int:
		out := make([]any, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out, true
	default:
		return nil, false
	}
}

func valueLength(value any) (int, bool) {
	switch v := value.(type) {
	case string:
		return len(v), true
	default:
		items, ok := sliceValues(value)
		return len(items), ok
	}
}

func isInteger(value any) bool {
	switch v := value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return v == float64(int64(v))
	case float32:
		return v == float32(int64(v))
	default:
		return false
	}
}

func numberAsFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func trimFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
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
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
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

func normalizePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return path
	}
	return filepath.Clean(path)
}

func scalarEqual(left any, right any) bool {
	if reflect.DeepEqual(left, right) {
		return true
	}

	leftNumber, leftOK := numberAsFloat(left)
	rightNumber, rightOK := numberAsFloat(right)
	if leftOK && rightOK {
		return leftNumber == rightNumber
	}

	leftString, leftOK := left.(string)
	rightString, rightOK := right.(string)
	if leftOK && rightOK {
		return leftString == rightString
	}

	leftBool, leftOK := left.(bool)
	rightBool, rightOK := right.(bool)
	return leftOK && rightOK && leftBool == rightBool
}

func containsValue(container any, needle any) bool {
	switch v := container.(type) {
	case string:
		needleText, ok := needle.(string)
		return ok && strings.Contains(v, needleText)
	default:
		items, ok := sliceValues(container)
		if !ok {
			return false
		}
		for _, item := range items {
			if scalarEqual(item, needle) {
				return true
			}
		}
		return false
	}
}

func twoStrings(left any, right any) (string, string, bool) {
	leftString, leftOK := left.(string)
	rightString, rightOK := right.(string)
	return leftString, rightString, leftOK && rightOK
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func intersectsString(left []string, right []string) bool {
	for _, item := range left {
		if containsString(right, item) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
