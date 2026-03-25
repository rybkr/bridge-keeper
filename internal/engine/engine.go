package engine

import (
	"bridgekeeper/internal/runtime"
	"context"
	"io"
	"log"
)

// Config holds the configuration for the orchestration engine.
type Config struct {
	PolicyDir string
	LogFile   string
	Verbose   bool
}

// Approver defines the interface for Human-In-The-Loop (HITL) approvals.
// Engine represents the higher-level orchestration engine (distinct from the policy engine).
type Engine struct {
	config   Config
	in       io.Reader
	out      io.Writer
	auditLog io.Writer
	approver runtime.Approver
}

// New creates a new instance of the Engine.
func New(cfg Config, in io.Reader, out io.Writer, auditLog io.Writer, approver runtime.Approver) (*Engine, error) {
	return &Engine{
		config:   cfg,
		in:       in,
		out:      out,
		auditLog: auditLog,
		approver: approver,
	}, nil
}

// Run starts the engine. This is where the main orchestration logic will be implemented in the future.
func (e *Engine) Run(ctx context.Context) error {
	if e.config.Verbose {
		log.Println("[ENGINE] Orchestration engine initialized and running.")
	}

	// TODO: Add any background workers or orchestration pipelines here in the future.

	return nil
}
