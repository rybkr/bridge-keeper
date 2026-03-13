package hitl

// AutoApprover automatically approves requests (used when --no-hitl flag is passed).
type AutoApprover struct{}

// AutoDenier automatically denies requests (used as a safe fallback if the terminal is unavailable).
type AutoDenier struct{}

// TerminalApprover prompts a human user in the terminal to approve or deny an action.
type TerminalApprover struct{}

// NewTerminalApprover initializes a new TerminalApprover.
// It returns the approver and an error, matching the signature expected in main.go.
func NewTerminalApprover() (*TerminalApprover, error) {
	return &TerminalApprover{}, nil
}
