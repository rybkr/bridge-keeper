/*
Translate user commands to application actions
and manipulate strings
*/
package parser

import (
	"bridgekeeper/internal/env"
	"strings"
)

// Parse user input for a command or a chat
func ParseInput(userInRaw string) string {

	// Trim and normalize input
	userIn := strings.TrimSpace(userInRaw)

	// Empty or whitespace only --> Help
	switch len(userIn) {
	case 0, 1:
		return CommandHELP()
	}

	// Check if the input begins with the command token
	if !strings.HasPrefix(userIn, env.MenuOptionPrefix) {
		return CommandSENDCHAT(userIn)
	}

	// Remove the leading delimiter and space to get the command tokens
	tokens := strings.Fields(userIn[1:])

	// Extract the actual command
	cmd := tokens[0]
	if len(cmd) > 2 {
		cmd = strings.ToLower(cmd)
	}

	// These should match the options defined in menuOption
	switch cmd {
	case "help", "?":
		return CommandHELP()
	case "list", "ls":
		return CommandLIST()
	case "verbose", "V":
		return CommandVERBOSE()
	case "concise", "c":
		return CommandCONCISE()
	case "clear", "R":
		return CommandCLEAR()
	case "exit", "bye":
		CommandEXIT()
		// unreachable
	case "policy", "po":
		return CommandPOLICY()
	default:
		return CommandHELP()
	}

	return "Unknown Error"
}
