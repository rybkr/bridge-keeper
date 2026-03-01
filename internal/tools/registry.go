package tools

import (
	"fmt"
	"os/exec"
	"strings"
)

// ExecuteGitCommand safely wraps the os/exec call.
func ExecuteGitCommand(repoPath string, args []string) string {
	fmt.Printf("\n[SYSTEM] Executing: git %s in %s...\n", strings.Join(args, " "), repoPath)

	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath

	// CombinedOutput captures both stdout and stderr
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error executing command: %v\nOutput: %s", err, string(out))
	}

	return string(out)
}
