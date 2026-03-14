package tools

import (
	"fmt"
	"os"
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

func ReadFile(filePath string) string {
	fmt.Printf("\n[SYSTEM] Executing: read file %s...\n", filePath)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err)
	}

	return string(content)
}

func ListDirectory(repoPath string) string {
	fmt.Printf("\n[SYSTEM] Listing files in %s...\n", repoPath)

	cmd := exec.Command("git", "ls-tree", "--full-tree", "-r", "--name-only", "HEAD")
	cmd.Dir = repoPath

	// CombinedOutput captures both stdout and stderr
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error listing files: %v\nOutput: %s", err, string(out))
	}

	return string(out)
}
