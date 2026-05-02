package redact

import (
	"regexp"
	"strings"
	"sync"
)

// Classification describes whether output needs redaction or untrusted-data handling.
type Classification struct {
	Sensitive bool     `json:"sensitive"`
	Untrusted bool     `json:"untrusted,omitempty"`
	Reasons   []string `json:"reasons,omitempty"`
}

// Redactor classifies untrusted content and masks known secret formats before
// audit logging or model handoff.
type Redactor struct {
	patterns []pattern
}

type pattern struct {
	reason    string
	expr      *regexp.Regexp
	redact    bool
	sensitive bool
	untrusted bool
}

// New returns a Redactor with pragmatic secret and prompt-injection detectors.
func New() *Redactor {
	return &Redactor{
		patterns: []pattern{
			{reason: "bearer_token", expr: regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s]+`), redact: true, sensitive: true},
			{reason: "openai_key", expr: regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9_-]{10,})\b`), redact: true, sensitive: true},
			{reason: "github_token", expr: regexp.MustCompile(`(?i)\b(ghp_[A-Za-z0-9]{10,}|github_pat_[A-Za-z0-9_]{10,})\b`), redact: true, sensitive: true},
			{reason: "google_api_key", expr: regexp.MustCompile(`(?i)\b(AIza[0-9A-Za-z\-_]{10,})\b`), redact: true, sensitive: true},
			{reason: "api_key_assignment", expr: regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\b\s*[:=]\s*['"]?[^\s'"]+['"]?`), redact: true, sensitive: true},
			{reason: "prompt_injection", expr: regexp.MustCompile(`(?is)\b(ignore|disregard|override)\b.{0,120}\b(previous|prior|above|system|developer|policy|policies|instructions?)\b`), untrusted: true},
			{reason: "role_injection", expr: regexp.MustCompile(`(?im)^\s*(system|developer|assistant)\s*:`), untrusted: true},
		},
	}
}

// RedactText masks any known secret-like substrings.
func (r *Redactor) RedactText(input string) string {
	if r == nil || input == "" {
		return input
	}

	out := input
	for _, pattern := range r.patterns {
		if !pattern.redact {
			continue
		}
		out = pattern.expr.ReplaceAllStringFunc(out, func(match string) string {
			submatches := pattern.expr.FindStringSubmatch(match)
			if len(submatches) == 2 && len(submatches[1]) < len(match) {
				return submatches[1] + "[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return out
}

// RedactValue recursively redacts strings within arbitrary JSON-like values.
func (r *Redactor) RedactValue(v any) any {
	switch value := v.(type) {
	case string:
		return r.RedactText(value)
	case []any:
		out := make([]any, 0, len(value))
		for _, item := range value {
			out = append(out, r.RedactValue(item))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			out[key] = r.RedactValue(item)
		}
		return out
	default:
		return v
	}
}

// Detect classifies text using secret and prompt-injection heuristics.
func (r *Redactor) Detect(text string) Classification {
	if r == nil || text == "" {
		return Classification{}
	}

	var out Classification
	for _, pattern := range r.patterns {
		if pattern.expr.MatchString(text) {
			if pattern.sensitive {
				out.Sensitive = true
			}
			if pattern.untrusted {
				out.Untrusted = true
			}
			out.Reasons = append(out.Reasons, pattern.reason)
		}
	}
	return out
}

// TaintTracker records snippets of untrusted tool output and can detect when
// later tool arguments reuse those snippets in sensitive positions.
type TaintTracker struct {
	mu      sync.Mutex
	entries []TaintEntry
}

// TaintEntry describes one untrusted output source stored by a tracker.
type TaintEntry struct {
	Tool      string   `json:"tool"`
	Action    string   `json:"action"`
	Reasons   []string `json:"reasons,omitempty"`
	Fragments []string `json:"fragments,omitempty"`
}

// TaintMatch identifies tainted output copied into a later value.
type TaintMatch struct {
	Tool     string `json:"tool"`
	Action   string `json:"action"`
	Fragment string `json:"fragment"`
}

// NewTaintTracker constructs an empty taint tracker.
func NewTaintTracker() *TaintTracker {
	return &TaintTracker{}
}

// AddOutput stores representative fragments from one untrusted tool result.
func (t *TaintTracker) AddOutput(tool string, action string, output string, reasons []string) {
	if t == nil {
		return
	}
	fragments := extractTaintFragments(output)
	if len(fragments) == 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = append(t.entries, TaintEntry{
		Tool:      tool,
		Action:    action,
		Reasons:   append([]string(nil), reasons...),
		Fragments: fragments,
	})
}

// FindInValue reports whether v contains any previously tainted fragment.
func (t *TaintTracker) FindInValue(v any) (TaintMatch, bool) {
	if t == nil {
		return TaintMatch{}, false
	}
	text := normalizeTaintText(valueText(v))
	if text == "" {
		return TaintMatch{}, false
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	for _, entry := range t.entries {
		for _, fragment := range entry.Fragments {
			if strings.Contains(text, fragment) {
				return TaintMatch{
					Tool:     entry.Tool,
					Action:   entry.Action,
					Fragment: fragment,
				}, true
			}
		}
	}
	return TaintMatch{}, false
}

var taintTokenExpr = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9._:/@+\-]{11,}`)

func extractTaintFragments(output string) []string {
	text := normalizeTaintText(output)
	if text == "" {
		return nil
	}

	seen := map[string]struct{}{}
	add := func(fragment string) {
		fragment = normalizeTaintText(fragment)
		if len(fragment) < 12 || len(fragment) > 512 {
			return
		}
		if _, ok := seen[fragment]; ok {
			return
		}
		seen[fragment] = struct{}{}
	}

	if len(text) <= 512 {
		add(text)
	}
	for _, line := range strings.Split(output, "\n") {
		add(line)
	}
	for _, token := range taintTokenExpr.FindAllString(output, -1) {
		add(token)
	}

	fragments := make([]string, 0, len(seen))
	for fragment := range seen {
		fragments = append(fragments, fragment)
	}
	return fragments
}

func normalizeTaintText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func valueText(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if text := valueText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	case map[string]any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if text := valueText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}
