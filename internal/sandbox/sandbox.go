package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"

	"bridgekeeper/internal/netguard"
	"bridgekeeper/internal/types"
)

// Validator enforces runtime-local filesystem and payload constraints below policy.
type Validator struct {
	WorkspaceRoot          string
	MaxOutputBytes         int
	MaxReadBytes           int64
	MaxWriteBytes          int64
	MaxCommandArgs         int
	SubprocessTimeoutSecs  int
	SubprocessEnvAllowlist []string
}

// NewValidator constructs a validator rooted at workspaceRoot.
func NewValidator(workspaceRoot string) (*Validator, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return nil, fmt.Errorf("workspace root is required")
	}

	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}

	return &Validator{
		WorkspaceRoot:          filepath.Clean(root),
		MaxOutputBytes:         64 * 1024,
		MaxReadBytes:           64 * 1024,
		MaxWriteBytes:          64 * 1024,
		MaxCommandArgs:         32,
		SubprocessTimeoutSecs:  5,
		SubprocessEnvAllowlist: []string{"PATH", "HOME", "LANG", "LC_ALL", "TERM", "SSH_AUTH_SOCK", "SSH_AGENT_PID", "SSH_ASKPASS"},
	}, nil
}

// ValidateToolCall normalizes and validates a tool call before execution.
func (v *Validator) ValidateToolCall(call types.ToolCall) (types.ToolCall, error) {
	if v == nil {
		return call, nil
	}

	args := cloneArgs(call.Args)
	switch call.Tool {
	case "fs":
		path, err := v.pathArg(args, "path")
		if err != nil {
			return call, err
		}
		args["path"] = path
		if call.Action == "write_file" {
			if err := v.validateContentSize(args); err != nil {
				return call, err
			}
		}
	case "git":
		if _, exists := args["path"]; exists {
			path, err := v.pathArg(args, "path")
			if err != nil {
				return call, err
			}
			args["path"] = path
		}
		if err := v.validateGitArgs(call.Action, args); err != nil {
			return call, err
		}
	case "http":
		rawURL, err := v.urlArg(args, "url")
		if err != nil {
			return call, err
		}
		args["url"] = rawURL
		if call.Action == "post" {
			if err := v.validateHTTPPostArgs(args); err != nil {
				return call, err
			}
		}
	case "shell":
		command, err := v.commandArg(args, "command")
		if err != nil {
			return call, err
		}
		args["command"] = command
		if path, err := v.pathArg(args, "path"); err == nil {
			args["path"] = path
		}
	case "pkg":
		if path, err := v.pathArg(args, "path"); err == nil {
			args["path"] = path
		}
		if err := v.validatePackageArgs(call.Action, args); err != nil {
			return call, err
		}
	}

	call.Args = args
	return call, nil
}

func (v *Validator) urlArg(args map[string]any, key string) (string, error) {
	raw, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	value, ok := raw.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}

	parsed, err := netguard.ValidateHTTPURL(value)
	if err != nil {
		return "", fmt.Errorf("%s is not an allowed URL: %w", key, err)
	}
	return parsed.String(), nil
}

func (v *Validator) validateContentSize(args map[string]any) error {
	if v.MaxWriteBytes <= 0 {
		return nil
	}

	raw, ok := args["content"]
	if !ok {
		return fmt.Errorf("content is required")
	}

	content, ok := raw.(string)
	if !ok {
		return fmt.Errorf("content must be a string")
	}
	if int64(len(content)) > v.MaxWriteBytes {
		return fmt.Errorf("content exceeds max write size of %d bytes", v.MaxWriteBytes)
	}
	return nil
}

func (v *Validator) validateHTTPPostArgs(args map[string]any) error {
	if v.MaxWriteBytes <= 0 {
		return nil
	}
	raw, ok := args["body"]
	if !ok {
		return fmt.Errorf("body is required")
	}
	body, ok := raw.(string)
	if !ok {
		return fmt.Errorf("body must be a string")
	}
	if int64(len(body)) > v.MaxWriteBytes {
		return fmt.Errorf("body exceeds max post size of %d bytes", v.MaxWriteBytes)
	}
	if rawContentType, ok := args["content_type"]; ok {
		contentType, ok := rawContentType.(string)
		if !ok || strings.TrimSpace(contentType) == "" {
			return fmt.Errorf("content_type must be a non-empty string")
		}
		if strings.ContainsAny(contentType, "\r\n") {
			return fmt.Errorf("content_type contains invalid newline")
		}
	}
	return nil
}

func (v *Validator) commandArg(args map[string]any, key string) (string, error) {
	raw, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	command, ok := raw.(string)
	command = strings.TrimSpace(command)
	if !ok || command == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}
	if err := validateSimpleShellCommand(command); err != nil {
		return "", err
	}
	return command, nil
}

func validateSimpleShellCommand(command string) error {
	if strings.ContainsRune(command, 0) {
		return fmt.Errorf("shell command contains invalid NUL byte")
	}
	if strings.ContainsAny(command, "\n\r;&|$`<>\\\"'(){}[]") {
		return fmt.Errorf("shell command contains unsupported shell syntax")
	}

	argv := strings.Fields(command)
	if len(argv) == 0 {
		return fmt.Errorf("shell command is required")
	}
	if len(argv) > 32 {
		return fmt.Errorf("shell command exceeds max of 32 arguments")
	}
	if !isAllowedSimpleCommand(argv[0]) {
		return fmt.Errorf("shell command %q is not allowed by the runtime sandbox", argv[0])
	}
	for _, arg := range argv[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if looksLikePath(arg) {
			if err := validateWorkspaceOperand(arg); err != nil {
				return fmt.Errorf("shell operand %q is not allowed: %w", arg, err)
			}
		}
	}
	return nil
}

func isAllowedSimpleCommand(name string) bool {
	switch name {
	case "echo", "cat", "ls", "grep", "wc", "head", "tail":
		return true
	default:
		return false
	}
}

func looksLikePath(arg string) bool {
	return arg == "." || arg == ".." || strings.ContainsRune(arg, filepath.Separator) || strings.HasPrefix(arg, "~")
}

func validateWorkspaceOperand(arg string) error {
	if strings.TrimSpace(arg) == "" {
		return fmt.Errorf("empty operand")
	}
	if filepath.IsAbs(arg) {
		return fmt.Errorf("absolute paths are not allowed")
	}
	if strings.HasPrefix(arg, "~") {
		return fmt.Errorf("home-relative paths are not allowed")
	}
	clean := filepath.Clean(arg)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("workspace path escapes are not allowed")
	}
	return nil
}

func (v *Validator) validatePackageArgs(action string, args map[string]any) error {
	if rawManager, ok := args["manager"]; ok {
		manager, ok := rawManager.(string)
		if !ok {
			return fmt.Errorf("manager must be a string")
		}
		if manager = strings.ToLower(strings.TrimSpace(manager)); manager != "" && manager != "go" && manager != "cargo" {
			return fmt.Errorf("unsupported package manager %q", manager)
		}
		args["manager"] = manager
	}

	switch action {
	case "list":
		return nil
	case "query", "install":
		return validatePackageNameArg(args, true)
	case "update":
		return validatePackageNameArg(args, false)
	default:
		return fmt.Errorf("package action %q is not allowed by the runtime sandbox", action)
	}
}

func validatePackageNameArg(args map[string]any, required bool) error {
	raw, ok := args["package"]
	if !ok {
		if required {
			return fmt.Errorf("package is required")
		}
		return nil
	}
	pkg, ok := raw.(string)
	pkg = strings.TrimSpace(pkg)
	if !ok || pkg == "" {
		if required {
			return fmt.Errorf("package must be a non-empty string")
		}
		args["package"] = ""
		return nil
	}
	if !isSafePackageToken(pkg) {
		return fmt.Errorf("package %q contains unsupported characters", pkg)
	}
	args["package"] = pkg

	if rawVersion, ok := args["version"]; ok {
		version, ok := rawVersion.(string)
		version = strings.TrimSpace(version)
		if !ok || version == "" {
			return fmt.Errorf("version must be a non-empty string")
		}
		if !isSafePackageToken(version) {
			return fmt.Errorf("version %q contains unsupported characters", version)
		}
		args["version"] = version
	}
	return nil
}

func isSafePackageToken(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("./_:-+@~", r):
		default:
			return false
		}
	}
	return true
}

// ValidateToolResult rejects unexpectedly large outputs.
func (v *Validator) ValidateToolResult(result string) error {
	if v == nil || v.MaxOutputBytes <= 0 {
		return nil
	}
	if len(result) > v.MaxOutputBytes {
		return fmt.Errorf("tool result exceeds max output size of %d bytes", v.MaxOutputBytes)
	}
	return nil
}

func (v *Validator) validateGitArgs(action string, args map[string]any) error {
	rawArgs, ok := args["args"]
	if !ok {
		return fmt.Errorf("git args are required")
	}

	items, ok := rawArgs.([]any)
	if !ok {
		return fmt.Errorf("git args must be an array")
	}
	if v.MaxCommandArgs > 0 && len(items) > v.MaxCommandArgs {
		return fmt.Errorf("git args exceed max of %d", v.MaxCommandArgs)
	}

	argv := make([]string, 0, len(items))
	for _, item := range items {
		arg, ok := item.(string)
		if !ok || strings.TrimSpace(arg) == "" {
			return fmt.Errorf("git args must contain non-empty strings")
		}
		if strings.ContainsRune(arg, 0) {
			return fmt.Errorf("git args contain invalid NUL byte")
		}
		argv = append(argv, arg)
	}

	return validateReadOnlyGitArgs(action, argv)
}

func validateReadOnlyGitArgs(action string, argv []string) error {
	command := strings.TrimSpace(action)
	if command == "" {
		return fmt.Errorf("git action is required")
	}
	if !isAllowedGitReadAction(command) {
		return fmt.Errorf("git action %q is not allowed by the runtime sandbox", command)
	}

	args := argv
	if len(argv) > 0 {
		first := strings.TrimSpace(argv[0])
		if first == command {
			args = argv[1:]
		} else if isKnownGitSubcommand(first) {
			return fmt.Errorf("git action %q does not match argv subcommand %q", command, first)
		}
	}

	switch command {
	case "status":
		return validateGitStatusArgs(args)
	case "log":
		return validateGitLogArgs(args)
	case "diff":
		return validateGitDiffArgs(args)
	case "show":
		return validateGitShowArgs(args)
	case "branch":
		return validateGitBranchArgs(args)
	default:
		return fmt.Errorf("git action %q is not allowed by the runtime sandbox", command)
	}
}

func validateGitStatusArgs(args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return validateGitOperands("status", args[i+1:])
		}
		if !strings.HasPrefix(arg, "-") {
			if err := validateGitOperand(arg); err != nil {
				return fmt.Errorf("git status operand %q is not allowed: %w", arg, err)
			}
			continue
		}
		if isAllowedGitFlag(arg, gitStatusFlags, gitStatusPrefixes) {
			continue
		}
		return fmt.Errorf("git status flag %q is not allowed by the runtime sandbox", arg)
	}
	return nil
}

func validateGitLogArgs(args []string) error {
	valueFlags := map[string]bool{
		"-n": true, "--max-count": true, "--skip": true, "--author": true,
		"--committer": true, "--grep": true, "--since": true, "--after": true,
		"--until": true, "--before": true, "--date": true, "--format": true,
		"--pretty": true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return validateGitOperands("log", args[i+1:])
		}
		if consumesGitFlagValue(arg, valueFlags) {
			i++
			if i >= len(args) {
				return fmt.Errorf("git log flag %q requires a value", arg)
			}
			if err := validateGitOperand(args[i]); err != nil {
				return fmt.Errorf("git log value for %q is not allowed: %w", arg, err)
			}
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			if err := validateGitOperand(arg); err != nil {
				return fmt.Errorf("git log operand %q is not allowed: %w", arg, err)
			}
			continue
		}
		if isAllowedGitFlag(arg, gitLogFlags, gitLogPrefixes) || isShortGitCount(arg) {
			continue
		}
		return fmt.Errorf("git log flag %q is not allowed by the runtime sandbox", arg)
	}
	return nil
}

func validateGitDiffArgs(args []string) error {
	valueFlags := map[string]bool{"-U": true, "--unified": true, "--diff-filter": true}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return validateGitOperands("diff", args[i+1:])
		}
		if consumesGitFlagValue(arg, valueFlags) {
			i++
			if i >= len(args) {
				return fmt.Errorf("git diff flag %q requires a value", arg)
			}
			if err := validateGitOperand(args[i]); err != nil {
				return fmt.Errorf("git diff value for %q is not allowed: %w", arg, err)
			}
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			if err := validateGitOperand(arg); err != nil {
				return fmt.Errorf("git diff operand %q is not allowed: %w", arg, err)
			}
			continue
		}
		if isAllowedGitFlag(arg, gitDiffFlags, gitDiffPrefixes) {
			continue
		}
		return fmt.Errorf("git diff flag %q is not allowed by the runtime sandbox", arg)
	}
	return nil
}

func validateGitShowArgs(args []string) error {
	valueFlags := map[string]bool{"--format": true, "--pretty": true}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return validateGitOperands("show", args[i+1:])
		}
		if consumesGitFlagValue(arg, valueFlags) {
			i++
			if i >= len(args) {
				return fmt.Errorf("git show flag %q requires a value", arg)
			}
			if err := validateGitOperand(args[i]); err != nil {
				return fmt.Errorf("git show value for %q is not allowed: %w", arg, err)
			}
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			if err := validateGitOperand(arg); err != nil {
				return fmt.Errorf("git show operand %q is not allowed: %w", arg, err)
			}
			continue
		}
		if isAllowedGitFlag(arg, gitShowFlags, gitShowPrefixes) {
			continue
		}
		return fmt.Errorf("git show flag %q is not allowed by the runtime sandbox", arg)
	}
	return nil
}

func validateGitBranchArgs(args []string) error {
	listMode := false
	valueFlags := map[string]bool{
		"--contains": true, "--no-contains": true, "--merged": true,
		"--no-merged": true, "--points-at": true, "--sort": true, "--format": true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if !listMode && len(args[i+1:]) > 0 {
				return fmt.Errorf("git branch operands require --list")
			}
			return validateGitOperands("branch", args[i+1:])
		}
		if arg == "--list" {
			listMode = true
			continue
		}
		if consumesGitFlagValue(arg, valueFlags) {
			i++
			if i >= len(args) {
				return fmt.Errorf("git branch flag %q requires a value", arg)
			}
			if err := validateGitOperand(args[i]); err != nil {
				return fmt.Errorf("git branch value for %q is not allowed: %w", arg, err)
			}
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			if !listMode {
				return fmt.Errorf("git branch operands require --list")
			}
			if err := validateGitOperand(arg); err != nil {
				return fmt.Errorf("git branch operand %q is not allowed: %w", arg, err)
			}
			continue
		}
		if isAllowedGitFlag(arg, gitBranchFlags, gitBranchPrefixes) {
			continue
		}
		return fmt.Errorf("git branch flag %q is not allowed by the runtime sandbox", arg)
	}
	return nil
}

func validateGitOperands(command string, operands []string) error {
	for _, operand := range operands {
		if err := validateGitOperand(operand); err != nil {
			return fmt.Errorf("git %s operand %q is not allowed: %w", command, operand, err)
		}
	}
	return nil
}

func validateGitOperand(arg string) error {
	if strings.TrimSpace(arg) == "" {
		return fmt.Errorf("empty operand")
	}
	if strings.ContainsRune(arg, 0) {
		return fmt.Errorf("contains invalid NUL byte")
	}
	if filepath.IsAbs(arg) {
		return fmt.Errorf("absolute paths are not allowed")
	}
	clean := filepath.Clean(arg)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("workspace path escapes are not allowed")
	}
	return nil
}

func consumesGitFlagValue(arg string, valueFlags map[string]bool) bool {
	if strings.Contains(arg, "=") {
		return false
	}
	return valueFlags[arg]
}

func isAllowedGitFlag(arg string, exact map[string]bool, prefixes []string) bool {
	if exact[arg] {
		return true
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func isShortGitCount(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' {
		return false
	}
	for _, r := range arg[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isAllowedGitReadAction(action string) bool {
	switch action {
	case "status", "log", "diff", "show", "branch":
		return true
	default:
		return false
	}
}

func isKnownGitSubcommand(arg string) bool {
	switch arg {
	case "add", "am", "apply", "bisect", "branch", "checkout", "cherry-pick",
		"clean", "clone", "commit", "config", "diff", "fetch", "gc", "init",
		"log", "merge", "mv", "pull", "push", "rebase", "reflog", "remote",
		"reset", "restore", "rm", "show", "stash", "status", "switch", "tag",
		"worktree":
		return true
	default:
		return false
	}
}

var gitStatusFlags = map[string]bool{
	"-s": true, "-b": true, "-u": true, "-uno": true, "-unormal": true,
	"-uall": true, "-z": true, "--short": true, "--branch": true,
	"--porcelain": true, "--long": true, "--ignored": true,
	"--untracked-files": true, "--ahead-behind": true, "--no-ahead-behind": true,
	"--renames": true, "--no-renames": true,
}

var gitStatusPrefixes = []string{
	"--porcelain=", "--ignored=", "--untracked-files=",
}

var gitLogFlags = map[string]bool{
	"--oneline": true, "--graph": true, "--decorate": true, "--no-decorate": true,
	"--stat": true, "--shortstat": true, "--numstat": true, "--name-only": true,
	"--name-status": true, "--patch": true, "-p": true, "--raw": true,
	"--summary": true, "--abbrev-commit": true, "--no-abbrev-commit": true,
	"--reverse": true, "--all": true, "--branches": true, "--tags": true,
	"--remotes": true, "--merges": true, "--no-merges": true,
	"--first-parent": true, "--topo-order": true, "--date-order": true,
	"--follow": true,
}

var gitLogPrefixes = []string{
	"--decorate=", "--format=", "--pretty=", "--date=", "--since=", "--after=",
	"--until=", "--before=", "--author=", "--committer=", "--grep=",
	"--max-count=", "--skip=", "--branches=", "--tags=", "--remotes=",
}

var gitDiffFlags = map[string]bool{
	"--stat": true, "--name-only": true, "--name-status": true, "--cached": true,
	"--staged": true, "--check": true, "--summary": true, "--patch": true,
	"-p": true, "--color": true, "--no-color": true, "--word-diff": true,
	"--ignore-space-at-eol": true, "-w": true, "--ignore-all-space": true,
	"-b": true, "--ignore-space-change": true, "--ignore-blank-lines": true,
	"--relative": true,
}

var gitDiffPrefixes = []string{
	"--unified=", "-U", "--color=", "--word-diff=", "--diff-filter=",
}

var gitShowFlags = map[string]bool{
	"--stat": true, "--name-only": true, "--name-status": true, "--summary": true,
	"--patch": true, "-p": true, "--no-patch": true, "-s": true, "--raw": true,
	"--abbrev-commit": true, "--decorate": true, "--color": true, "--no-color": true,
}

var gitShowPrefixes = []string{
	"--format=", "--pretty=", "--decorate=", "--color=",
}

var gitBranchFlags = map[string]bool{
	"-a": true, "-r": true, "-v": true, "-vv": true, "--all": true,
	"--remotes": true, "--verbose": true, "--contains": true, "--no-contains": true,
	"--merged": true, "--no-merged": true, "--points-at": true, "--sort": true,
	"--format": true, "--color": true, "--no-color": true, "--ignore-case": true,
	"--column": true, "--no-column": true,
}

var gitBranchPrefixes = []string{
	"--contains=", "--no-contains=", "--merged=", "--no-merged=", "--points-at=",
	"--sort=", "--format=", "--color=", "--column=",
}

func (v *Validator) pathArg(args map[string]any, key string) (string, error) {
	raw, ok := args[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	path, ok := raw.(string)
	if !ok || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}
	return v.resolveWorkspacePath(path)
}

func (v *Validator) resolveWorkspacePath(path string) (string, error) {
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("path contains invalid NUL byte")
	}

	var resolved string
	if filepath.IsAbs(path) {
		resolved = filepath.Clean(path)
	} else {
		resolved = filepath.Join(v.WorkspaceRoot, path)
	}

	rel, err := filepath.Rel(v.WorkspaceRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace root %q", path, v.WorkspaceRoot)
	}

	return filepath.Clean(resolved), nil
}

func cloneArgs(args map[string]any) map[string]any {
	if args == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(args))
	for key, value := range args {
		out[key] = value
	}
	return out
}
