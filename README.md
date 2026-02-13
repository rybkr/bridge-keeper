# Bridgekeeper

A security-constrained agent runtime that enforces capability-based security for tool use.

Bridgekeeper treats the agent as an untrusted program: it can propose tool calls, but the runtime mediates execution via a policy engine that prevents unsafe actions. Policies are defined as code (YAML), denials are explainable, and every action is audit-logged.

## Project Structure

```
bridgekeeper/
├── cmd/bridgekeeper/       # CLI entrypoint
├── internal/
│   ├── runtime/            # Core agent runtime loop
│   ├── policy/             # Policy engine and YAML/JSON loader
│   ├── tools/              # Tool implementations (fs, git, shell, http, pkg)
│   ├── sandbox/            # Pre-call and post-call validation
│   ├── redact/             # Sensitive data redaction
│   ├── taint/              # Information-flow taint tracking
│   ├── audit/              # Structured audit trail logging
│   └── hitl/               # Human-in-the-loop approval
├── policies/               # YAML policy files
├── testdata/
│   ├── adversarial/        # Prompt injection and adversarial fixtures
│   └── workflows/          # End-to-end workflow scripts
└── docs/                   # Design doc and evaluation report
```
