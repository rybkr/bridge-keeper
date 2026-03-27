package runtime

// Shared values
var AvailableTools = []OTool{
	goTool,
	gitTool,
	writeFile,
}

var goTool = OTool{
	Type: OJsonFunction,
	Function: OFunction{
		Name:        "go",
		Description: "The go (golang) command",
		Parameters: OFnParams{
			Type:     string(JSONObject),
			Required: []string{"action"},
			Properties: map[string]OFnPmPropts{
				"action": {
					Type:        string(JSONString),
					Description: "ONE of version, build, fmt, vet, mod",
				},
				"args": {
					Type:        string(JSONString),
					Description: "Flags for the action, such as -o for build or -w for fmt",
				},
			},
		},
	},
}

var gitTool = OTool{
	Type: OJsonFunction,
	Function: OFunction{
		Name:        "git",
		Description: "The git versioning command",
		Parameters: OFnParams{
			Type:     string(JSONObject),
			Required: []string{"action"},
			Properties: map[string]OFnPmPropts{
				"action": {
					Type:        string(JSONString),
					Description: "ONE of version, status, commit, add, branch, switch, pull, push, merge, rebase, stash, restore, mv, rm",
				},
				"args": {
					Type:        string(JSONString),
					Description: "Flags specific to the action, such as --remote or --staged",
				},
			},
		},
	},
}

var writeFile = OTool{
	Type: OJsonFunction,
	Function: OFunction{
		Name:        "writeout",
		Description: "Write a message to a file",
		Parameters: OFnParams{
			Type:     string(JSONString),
			Required: []string{"contents"},
			Properties: map[string]OFnPmPropts{
				"contents": {
					Type:        string(JSONString),
					Description: "Any text",
				},
				"--append": {
					Type:        string(JSONBool),
					Description: "Flag to indicate append mode. Overridden by --overwrite",
				},
				"--overwrite": {
					Type:        string(JSONBool),
					Description: "(Default Behavior) Flag to indicate overwrite mode",
				},
			},
		},
	},
}
