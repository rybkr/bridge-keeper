package tools

import (
	"net/http"

	"bridgekeeper/internal/sandbox"
)

// Registry holds typed tool implementations rooted in a workspace.
type Registry struct {
	WorkspaceRoot string
	Validator     *sandbox.Validator
	HTTPClient    *http.Client
}

// NewRegistry constructs a tool registry for a workspace root.
func NewRegistry(workspaceRoot string, validator *sandbox.Validator) *Registry {
	return &Registry{
		WorkspaceRoot: workspaceRoot,
		Validator:     validator,
	}
}

type GitExecArgs struct {
	Path string
	Args []string
}

type ReadFileArgs struct {
	Path string
}

type WriteFileArgs struct {
	Path    string
	Content string
}

type ListDirectoryArgs struct {
	Path string
}

type HTTPGetArgs struct {
	URL string
}

type HTTPPostArgs struct {
	URL         string
	Body        string
	ContentType string
}

type ShellExecArgs struct {
	Command string
	Path    string
}

type PackageListArgs struct {
	Manager string
	Path    string
}

type PackageQueryArgs struct {
	Manager string
	Package string
	Path    string
}

type PackageInstallArgs struct {
	Manager string
	Package string
	Version string
	Path    string
}

type PackageUpdateArgs struct {
	Manager string
	Package string
	Path    string
}
