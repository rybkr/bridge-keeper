package redact

import "regexp"

// Redactor masks known secret formats before audit logging or model handoff.
type Redactor struct {
	patterns []*regexp.Regexp
}

// New returns a Redactor with a small set of pragmatic secret detectors.
func New() *Redactor {
	return &Redactor{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s]+`),
			regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9_-]{10,})\b`),
			regexp.MustCompile(`(?i)\b(ghp_[A-Za-z0-9]{10,}|github_pat_[A-Za-z0-9_]{10,})\b`),
			regexp.MustCompile(`(?i)\b(AIza[0-9A-Za-z\-_]{10,})\b`),
			regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\b\s*[:=]\s*['"]?[^\s'"]+['"]?`),
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
		out = pattern.ReplaceAllStringFunc(out, func(match string) string {
			submatches := pattern.FindStringSubmatch(match)
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
