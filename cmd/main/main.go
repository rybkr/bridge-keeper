package main

import (
	"bridgekeeper/internal/agents"
	"bridgekeeper/internal/audit"
	"bridgekeeper/internal/cli"
	"bridgekeeper/internal/env"
	"bridgekeeper/internal/hitl"
	"bridgekeeper/internal/parser"
	"bridgekeeper/internal/policy"
	"bridgekeeper/internal/redact"
	"bridgekeeper/internal/sandbox"
	"bridgekeeper/internal/session"
	"bridgekeeper/internal/tools"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {

	var err error
	env.EPolicyFile, err = policy.LoadPath(*env.EPolicyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: loading policy path: %v\n", err)
		os.Exit(1)
	}
	env.EEngine = policy.NewEngine(env.EPolicyFile)

	// Set up audit log writer.
	var auditWriter *os.File
	if *env.SharedLogFile != "" {
		f, err := os.OpenFile(*env.SharedLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot open log file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		auditWriter = f
	} else {
		auditWriter = os.Stderr
	}
	env.Auditor = audit.NewLogger(auditWriter, audit.Warning)

	workspaceRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine working directory: %v\n", err)
		os.Exit(1)
	}
	workspaceRoot, err = filepath.Abs(workspaceRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot resolve working directory: %v\n", err)
		os.Exit(1)
	}
	validator, err := sandbox.NewValidator(workspaceRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot initialize sandbox validator: %v\n", err)
		os.Exit(1)
	}
	env.RuntimeRegistry = tools.NewRegistry(workspaceRoot, validator)

	// Set up approver.
	var approver sandbox.Approver
	if env.NoHITL {
		approver = &hitl.AutoApprover{}
	} else {
		ta, err := hitl.NewTerminalApprover()
		if err != nil {
			// If we can't open /dev/tty (e.g. in a pipe), fall back to auto-deny.
			fmt.Fprintf(os.Stderr, "warning: cannot open terminal for approval, falling back to auto-deny: %v\n", err)
			approver = &hitl.AutoDenier{}
		} else {
			approver = ta
		}
	}

	// Set up signal handling.
	defer env.CtxCancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		env.CtxCancel()
	}()
	env.EMediator = &sandbox.Mediator{
		Policy:   env.EEngine,
		Approver: approver,
		Audit:    env.Auditor,
		Sandbox:  validator,
		Redactor: redact.New(),
	}

	env.Auditor.Log(audit.Info, "runtime_started", map[string]any{"mode": env.Endpoint})
	if env.Verbose {
		fmt.Fprintf(os.Stderr, "bridgekeeper: workspace root %s\n", workspaceRoot)
	}

	env.ESession, err = session.NewSession(os.Stdin, os.Stdout)
	if err != nil {
		fmt.Printf("%s\n", err.Error())
		os.Exit(1)
	}

	fmt.Print(parser.CommandHELP())
	for {
		if strings.ToLower(env.Endpoint) == "ollama" && env.OllamaProc == nil {
			agents.NewOllamaAgent()
		} else if strings.ToLower(env.Endpoint) == "gemini" && agents.RuntimeGeminiAgent == nil {
			agents.RuntimeGeminiAgent = agents.NewGeminiAgent()
		}

		err = cli.Rep()
		if err != nil {
			fmt.Printf("%s\n", err.Error())
			os.Exit(1)
		}
	}
}
