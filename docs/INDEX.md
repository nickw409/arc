# Arc Documentation

## Quick Start

1. Read [ARCHITECTURE.md](./ARCHITECTURE.md) for system overview and key invariants
2. Read [PLANNING_PROCESS.md](./PLANNING_PROCESS.md) for how to create plans
3. Run `arc init` in your project and `arc plan` to create your first plan

## Document Index

| Document | Description |
|----------|-------------|
| [ARCHITECTURE.md](./ARCHITECTURE.md) | System overview, components, gate-based execution model, adapter system, invariants |
| [STATE_SCHEMA.md](./STATE_SCHEMA.md) | Phase state.json field reference |
| [PLANNING_PROCESS.md](./PLANNING_PROCESS.md) | How plans are created and validated |

## Key Concepts

### What Arc Does

Arc breaks a software engineering task into **phases**, then drives each phase through a **session → gate → retry** loop until the gate passes (or attempts are exhausted). There is no state machine — completion is defined by gate assertions in `spec.yaml`, not by agent verdicts.

### Phase Roles

| Role | Agent prompt | Completion check |
|------|-------------|-----------------|
| `impl` | `gate/impl.md` — write code | Gate assertions |
| `review` | `gate/review.md` — produce findings | AI verifier |
| `investigate` | `gate/investigate.md` — research | AI verifier |
| `audit` | `gate/review.md` — security/quality | AI verifier |

### Directory Structure

```
arc/
├── cmd/arc/          CLI entry point (main.go)
├── internal/
│   ├── adapter/      Multi-provider AI adapter (claude, codex, generic)
│   ├── agent/        Agent spawning
│   ├── arc/          Core types (PhaseSpec, PhaseState, GateResult, PlanMeta)
│   ├── cli/          Cobra commands
│   ├── config/       .arc.yaml parsing
│   ├── daemon/       Background orchestration daemon
│   ├── dev/          arc dispatch pipeline (discovery → plan generation)
│   ├── gate/         Gate assertion evaluation
│   ├── gitops/       Git commit operations
│   ├── guide/        Agent-facing reference guide
│   ├── intelligence/ Project intelligence store
│   ├── monitor/      Live TUI
│   ├── orchestrator/ Execution engine (LaunchGated, RunPhaseGated)
│   ├── plan/         Plan/phase lifecycle management
│   ├── project/      Project detection & init
│   ├── prompt/       Prompt rendering
│   ├── resources/    Embedded prompts, templates, guides, recipes
│   ├── review/       Adversarial plan review
│   ├── selfupdate/   Self-update mechanism
│   ├── state/        Phase state.json management
│   ├── testcmd/      Test command resolution and execution
│   ├── validate/     AI-powered test quality audit
│   └── worktree/     Git worktree isolation
└── docs/             This documentation
```

## What Is NOT in Arc

Arc does **not** have:
- Workflow YAML files or state machine definitions
- Verdict extraction from agent output
- `internal/pipeline/`, `internal/workflow/`, `internal/block/`, `internal/runner/` packages
- `arc iterate` command
- `feature/`, `bugfix/`, `blocks/`, `common/` prompt directories
