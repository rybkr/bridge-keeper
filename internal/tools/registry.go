package tools

import "bridgekeeper/internal/sandbox"

// Registry holds typed tool implementations rooted in a workspace.
type Registry struct {
	WorkspaceRoot string
	Validator     *sandbox.Validator
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

type ListDirectoryArgs struct {
	Path string
}
