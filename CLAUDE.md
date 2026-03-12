# Arc

AI-powered workflow engine for orchestrating multi-phase software engineering tasks through AI agents. Written in Go.

## Quick Reference

```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/gate/

# Run a single test
go test ./internal/gate/ -run TestName

# Build the CLI binary
go build -o arc ./cmd/arc/

# Build + install to ~/.local/bin
go build -o ~/.local/bin/arc ./cmd/arc/
```

## Project Structure

```
cmd/arc/          Entry point (main.go)
internal/         All Go packages:
  adapter/        Multi-provider AI adapter (claude, codex, generic)
  agent/          Agent spawning (claude CLI subprocess)
  arc/            Core types (PhaseSpec, PhaseState, GateResult, PlanMeta)
  cli/            Cobra command definitions
  config/         .arc.yaml parsing
  daemon/         Background orchestration daemon (persistent, multi-plan)
  dev/            arc dispatch pipeline (discovery → architecture → plan generation)
  gate/           Gate assertion evaluation (file_exists, grep, build_passes, ...)
  gitops/         Git commit operations
  guide/          Agent-facing reference guide (arc guide)
  intelligence/   Project intelligence store (test cmds, flaky tests, costs)
  logging/        Structured JSONL logger (PlanLogger, plan.jsonl)
  migrate/        State migration utilities
  monitor/        Live TUI (bubbletea) for arc status --live
  orchestrator/   Orchestration engine:
    orchestrator.go   Launch() entry point, LaunchOptions, lock management
    launch_gated.go   LaunchGated(): phase scheduling, worktrees, regression suite
    gated.go          RunPhaseGated(): per-phase session→gate→retry loop
    phase_types.go    RunPhaseOptions, commitPhase, discoverNewTestFiles
    strategic.go      RunStrategicIntervention(): AI agent for stuck phases
    adversary.go      Post-plan adversarial test session
    classify.go       Error tier classification (Transient/Feedback/Strategic/GiveUp)
  plan/           Plan creation, status, manage mutations, archival, summaries
  project/        Project detection & init (.arc.yaml, .plans/)
  prompt/         Prompt rendering (Handlebars shim over Go templates)
  resources/      Embedded static assets:
    prompts/      Agent prompt templates (.md) — gate/, dev/, adversaries/, validate/
    templates/    Plan scaffolding templates (.md)
    enforcement/  Hook scripts
    guides/       Agent-facing reference docs
    recipes/      Built-in recipe definitions (.yaml)
  review/         Adversarial plan review (5 adversaries, auto-remediation)
  selfupdate/     Self-update (GitHub releases, SHA256 verification)
  state/          Phase state.json read/write/update
  testcmd/        Test command resolution and execution
  validate/       AI-powered test quality audit
  worktree/       Git worktree isolation for parallel phase execution
docs/             Documentation
```

## What Is NOT in Arc

Arc does **not** have (do not introduce these):
- `internal/pipeline/`, `internal/workflow/`, `internal/block/`, `internal/runner/` packages
- `internal/mcp/` package — MCP server was removed; `arc chat` uses CLI directly
- Workflow YAML files or state machine definitions
- Verdict extraction from agent output (`## Verdict` parsing)
- `arc iterate` command
- `arc serve` command — removed along with the MCP server
- `feature/`, `bugfix/`, `blocks/`, `common/` prompt directories
- `IsGatedPlan()` function — all plans are gated, `Launch()` always calls `LaunchGated()`

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
arc review <plan-name>                      # Run adversarial review (5 adversaries, max 5 iterations)
arc review <plan-name> --phase <phase>      # Review a single phase
arc run <plan-name>                         # Launch orchestrator for all phases
arc status [plan-name]                      # Show plan/phase status
arc archive [--force] <plan-name>           # Archive completed plan
arc guide                                   # Print agent-facing reference
arc validate [paths...]                     # Audit test quality using AI agent
arc dispatch <task description...>               # Auto-generate plan from description and run it

# Interactive
arc chat                                    # Launch Claude session with Arc guide as system context
arc chat --model opus                       # Use a specific model

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
arc manage reset-review <plan> <phase>     # Clear review cache and iteration counter
```

### Workflow Types

`--type` sets `workflow_type` in `plan.json` and `state.json`. It is metadata only — it does **not** define a state machine or phase sequence. All phases run through the same `session → gate → retry` loop regardless of workflow type.

| Type | Typical use |
|------|-------------|
| `feature` | New functionality |
| `bugfix` | Fixing incorrect behavior |
| `investigation` | Research / exploration |
| `refactor` | Restructuring without behavior change |
| `performance` | Optimization |
| `direct` | Simple single-phase task (used by `arc dispatch`) |

### Phase Roles
Phases have a `role` that determines their prompt and gate behavior:
- **impl** (default) — write code, verified by gate assertions
- **review** — analyze code, verified by AI verifier
- **investigate** — research questions, verified by AI verifier
- **audit** — security/quality audit, verified by AI verifier

## Releases

Releases are built by goreleaser via `.github/workflows/release.yml` on `v*` tag push. Version is injected via `-ldflags -X github.com/nwiley/arc/internal/cli.Version=...`. The `selfupdate` package downloads releases from the GitHub API with SHA256 checksum verification.

```bash
# Tag and release
git tag v0.X.0 && git push origin v0.X.0

# Validate goreleaser config locally
goreleaser check
goreleaser build --snapshot --clean
```

## Benchmarking

The `bench/` directory contains a benchmarking suite that compares Arc orchestration
against single-agent Claude Code. See `bench/README.md` for details.

```bash
# Run full benchmark (4 tasks × 3 approaches × 3 runs)
bash bench/harness/bench.sh

# Single task, single approach
bash bench/harness/bench.sh --task task2-bugfix --approach arc --runs 1
```

## Dependencies

Runtime: `claude` (Claude Code CLI), `git`, `jq`, `yq` (mikefarah v4+), `python3`
