package redact

import "regexp"

// Classification describes whether output looks sensitive enough to redact.
type Classification struct {
	Sensitive bool     `json:"sensitive"`
	Reasons   []string `json:"reasons,omitempty"`
}

// Redactor masks known secret formats before audit logging or model handoff.
type Redactor struct {
	patterns []pattern
}

type pattern struct {
	reason string
	expr   *regexp.Regexp
}

// New returns a Redactor with a small set of pragmatic secret detectors.
func New() *Redactor {
	return &Redactor{
		patterns: []pattern{
			{reason: "bearer_token", expr: regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s]+`)},
			{reason: "openai_key", expr: regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9_-]{10,})\b`)},
			{reason: "github_token", expr: regexp.MustCompile(`(?i)\b(ghp_[A-Za-z0-9]{10,}|github_pat_[A-Za-z0-9_]{10,})\b`)},
			{reason: "google_api_key", expr: regexp.MustCompile(`(?i)\b(AIza[0-9A-Za-z\-_]{10,})\b`)},
			{reason: "api_key_assignment", expr: regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\b\s*[:=]\s*['"]?[^\s'"]+['"]?`)},
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

// Detect classifies text using the same secret-oriented heuristics used by redaction.
func (r *Redactor) Detect(text string) Classification {
	if r == nil || text == "" {
		return Classification{}
	}

	var out Classification
	for _, pattern := range r.patterns {
		if pattern.expr.MatchString(text) {
			out.Sensitive = true
			out.Reasons = append(out.Reasons, pattern.reason)
		}
	}
	return out
}
