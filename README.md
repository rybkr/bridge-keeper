# Bridgekeeper

A security-constrained agent runtime that enforces capability-based security for tool use.

Bridgekeeper treats the agent as an untrusted program: it can propose tool calls, but the runtime mediates execution through a policy engine, local sandbox validation, human approval for `ask` decisions, and structured audit logging. Tool inputs are normalized before execution, and sensitive tool output is redacted before it is handed back to the model.

Current state:
- Policy evaluation for tool/action/capability matching is implemented.
- Local sandbox enforcement currently focuses on workspace-bounded filesystem access, argument validation, and output-size limits.
- Audit logging is structured JSONL.
- Sensitive output redaction and simple taint detection are implemented.
- Network sandboxing and deeper information-flow controls are still incomplete.

## Project Structure

```
bridgekeeper/
├── cmd/bridgekeeper/       # CLI entrypoint
├── internal/
│   ├── agent/              # Provider-specific agent adapters (Gemini, etc.)
│   ├── runtime/            # Core mediation and runtime loop
│   ├── policy/             # Policy engine and YAML loader
│   ├── tools/              # Typed tool implementations grouped by capability
│   ├── sandbox/            # Workspace and payload validation below policy
│   ├── redact/             # Secret redaction and sensitivity classification
│   ├── audit/              # Structured audit trail logging
│   └── hitl/               # Human-in-the-loop approval
├── policies/               # YAML policy files
├── testdata/
│   ├── adversarial/        # Prompt injection and adversarial fixtures
│   └── workflows/          # End-to-end workflow scripts
└── docs/                   # Design doc and evaluation report
```
