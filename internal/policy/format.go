package policy

import (
	"fmt"
	"strings"
)

// FormatPolicy returns a human-readable rendering of a loaded policy file.
func FormatPolicy(pf *PolicyFile) string {
	if pf == nil {
		return "No policy loaded."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Current Policy\n")
	fmt.Fprintf(&b, "  Version: %s\n", valueOrFallback(pf.Version, "(unset)"))
	fmt.Fprintf(&b, "  Default: %s\n", valueOrFallback(pf.Default, "deny"))
	fmt.Fprintf(&b, "  Capabilities: %d\n", len(pf.Capabilities))

	for i, cap := range pf.Capabilities {
		fmt.Fprintf(&b, "\n[%d] %s\n", i+1, valueOrFallback(cap.Name, "(unnamed)"))
		fmt.Fprintf(&b, "  Tool: %s\n", valueOrFallback(cap.Tool, "(unset)"))
		fmt.Fprintf(&b, "  Actions: %s\n", joinOrFallback(cap.Actions, "(none)"))
		fmt.Fprintf(&b, "  Decision: %s\n", valueOrFallback(cap.Decision, "(unset)"))

		if cap.Constraints == nil {
			fmt.Fprintf(&b, "  Constraints: none\n")
			continue
		}

		fmt.Fprintf(&b, "  Constraints:\n")
		writeAllowDeny(&b, "paths", cap.Constraints.Paths)
		writeAllowDeny(&b, "commands", cap.Constraints.Commands)
		writeAllowDeny(&b, "domains", cap.Constraints.Domains)
		if cap.Constraints.MaxSizeBytes > 0 {
			fmt.Fprintf(&b, "    max_size_bytes: %d\n", cap.Constraints.MaxSizeBytes)
		}
		if cap.Constraints.TimeoutSeconds > 0 {
			fmt.Fprintf(&b, "    timeout_seconds: %d\n", cap.Constraints.TimeoutSeconds)
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func writeAllowDeny(b *strings.Builder, name string, rule *AllowDeny) {
	if rule == nil {
		return
	}
	fmt.Fprintf(b, "    %s:\n", name)
	fmt.Fprintf(b, "      allow: %s\n", joinOrFallback(rule.Allow, "(none)"))
	fmt.Fprintf(b, "      deny: %s\n", joinOrFallback(rule.Deny, "(none)"))
}

func joinOrFallback(items []string, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	return strings.Join(items, ", ")
}

func valueOrFallback(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
