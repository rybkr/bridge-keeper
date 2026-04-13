package cli

import (
	"bridgekeeper/internal/env"
	"bridgekeeper/internal/parser"
	"fmt"
)

// read-evaluate-print
func Rep() error {

	// get user's input
	userIn, err := env.ESession.ReadLine(parser.UserPrompt)
	if err != nil {
		return err
	}

	// evaluate
	appOut := parser.ParseInput(userIn)

	// print result
	fmt.Print(appOut)

	return nil
	// loops in main
}
