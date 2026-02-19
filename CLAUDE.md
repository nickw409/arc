# Arc

AI-powered workflow engine for orchestrating multi-phase software engineering tasks through AI agents. Written in Go.

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

## Arc CLI Commands

```bash
# Project setup
arc init                                    # Initialize project (.arc.yaml, .plans/)
arc init --force                            # Re-initialize existing project

# Plan lifecycle
arc plan <name> <phase1> [phase2] ...       # Create plan scaffolding
arc plan --type bugfix <name> <phases...>   # Create with specific workflow type
arc review <plan-name>                      # Run adversarial review (5 adversaries)
arc run <plan-name>                         # Launch orchestrator for all phases
arc iterate <plan-name> <phase-name>        # Run single iteration for a phase
arc status [plan-name]                      # Show plan/phase status
arc archive [--force] <plan-name>           # Archive completed plan
arc guide                                   # Print agent-facing reference

# Phase management
arc manage <plan> <phase> complete          # Mark phase complete
arc manage <plan> <phase> pending           # Reset phase to pending
arc manage <plan> <phase> defer <reason>    # Defer phase with reason
arc manage <plan> <phase> block <reason>    # Block phase with reason
arc manage <plan> <phase> tests <pass> <total>  # Update test counts
arc manage <plan> <phase> packages <pkg...> # Set packages list
arc manage <plan> <phase> note <text>       # Set phase notes
arc manage <plan> <phase> iteration <n>     # Set iteration number
arc manage <plan> <phase> copy-from <src>   # Copy state from another phase
arc manage <plan> <phase> show              # Show phase state.json
```

### Workflow Types

- **feature** — TDD: `qa → qa_review → impl → impl_review → complete`
- **bugfix** — Linear: `investigate → regression_tests → test_review → fix → fix_review → complete`
- **investigation** — Research: `research → draft → review → complete`
- **refactor** — Preserve behavior: `characterize → char_review → refactor → verify → complete`
- **performance** — Benchmark-driven: `baseline → analyze → optimize → benchmark → complete`

## Dependencies

Runtime: `claude` (Claude Code CLI), `git`, `jq`, `yq` (mikefarah v4+), `python3`
