# Arc

AI-powered workflow engine for orchestrating multi-phase software engineering tasks through AI agents. Written in Go with a legacy bash layer in `scripts/` and `bin/arc`.

## Quick Reference

```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/runner/

# Run a single test
go test ./internal/runner/ -run TestName

# Build the CLI binary
go build -o arc ./cmd/arc/

# Build + install to $GOPATH/bin
go install ./cmd/arc/
```

## Project Structure

```
cmd/arc/          Entry point (main.go)
internal/         All Go packages:
  cli/            Cobra command definitions
  config/         .arc.yaml parsing
  workflow/       Workflow YAML loading & validation
  state/          Phase state (state.json) management
  plan/           Plan creation & status
  project/        Project detection & init
  prompt/         Prompt rendering & extraction
  runner/         Subprocess runner (claude CLI)
  agent/          Agent spawning
  pipeline/       Phase iteration, escalation, hooks, constraints
  orchestrator/   Top-level orchestrator loop
  review/         Adversarial plan review
  gitops/         Git commit operations
  monitor/        Live TUI (bubbletea)
  resources/      Embedded templates & prompts
  logging/        Structured logger
  selfupdate/     Self-update mechanism
  migrate/        State migration
  arc/            Core types (verdict, result, errors, state)
bin/arc           Legacy bash entrypoint (shell-based CLI)
scripts/          Legacy bash scripts (being replaced by Go)
templates/        Workflow YAML templates
prompts/          Prompt templates (embedded at build)
workflows/        Workflow definitions
testdata/         Test fixtures
```

## Key Conventions

- Go 1.24, module `github.com/nwiley/arc`
- CLI built with `spf13/cobra`, TUI with `charmbracelet/bubbletea`
- Tests use stdlib `testing` only — no external test frameworks
- Test files live alongside source (`*_test.go`)
- Integration tests in `tests/integration_test.go`
- Prompts and templates are embedded via `internal/resources/`

## Dependencies

Runtime: `claude` (Claude Code CLI), `git`, `jq`, `yq` (mikefarah v4+), `python3`
