package taint

import "regexp"

// Classification describes whether tool output looks sensitive enough to treat
// as tainted before handing it back to a model or audit sink.
type Classification struct {
	Sensitive bool     `json:"sensitive"`
	Reasons   []string `json:"reasons,omitempty"`
}

var checks = []struct {
	reason  string
	pattern *regexp.Regexp
}{
	{reason: "bearer_token", pattern: regexp.MustCompile(`(?i)authorization\s*:\s*bearer\s+[^\s]+`)},
	{reason: "openai_key", pattern: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{10,}\b`)},
	{reason: "github_token", pattern: regexp.MustCompile(`\b(ghp_[A-Za-z0-9]{10,}|github_pat_[A-Za-z0-9_]{10,})\b`)},
	{reason: "api_key_assignment", pattern: regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\b\s*[:=]\s*['"]?[^\s'"]+['"]?`)},
}

// Detect classifies text using a small set of secret-oriented heuristics.
func Detect(text string) Classification {
	var out Classification
	for _, check := range checks {
		if check.pattern.MatchString(text) {
			out.Sensitive = true
			out.Reasons = append(out.Reasons, check.reason)
		}
	}
	return out
}
