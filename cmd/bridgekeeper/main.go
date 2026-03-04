package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"bridgekeeper/internal/engine"
	"bridgekeeper/internal/hitl"
)

func main() {
	policyPath := flag.String("policy", "policies", "path to policy YAML file or directory")
	logFile := flag.String("log-file", "", "audit log file path (default: stderr)")
	verbose := flag.Bool("verbose", false, "enable verbose output")
	noHITL := flag.Bool("no-hitl", false, "disable human-in-the-loop approval (auto-approve all)")
	flag.Parse()

	cfg := engine.Config{
		PolicyDir: *policyPath,
		LogFile:   *logFile,
		Verbose:   *verbose,
	}

	// Set up audit log writer.
	var auditWriter *os.File
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot open log file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		auditWriter = f
	} else {
		auditWriter = os.Stderr
	}

	// Set up approver.
	var approver engine.Approver
	if *noHITL {
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Create and run engine.
	eng, err := engine.New(cfg, os.Stdin, os.Stdout, auditWriter, approver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := eng.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
