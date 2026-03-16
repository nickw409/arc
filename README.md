# Arc

A workflow engine for orchestrating complex, multi-phase software engineering tasks through AI agents. It breaks large work into phases, drives each phase through a session → gate → retry loop, and enforces rules so agents stay on track.

## Getting Started

### Install

```bash
# Install the latest release binary
curl -fsSL https://raw.githubusercontent.com/nickw409/arc/main/install.sh | bash
```

This downloads a prebuilt binary to `~/.local/bin/arc`. No build toolchain required.

To update to the latest version:

```bash
arc update
```

For private repo access, `arc update` automatically uses your `gh` CLI credentials. You can also set `GITHUB_TOKEN` or `GH_TOKEN` explicitly.

Dependencies: `claude` (Claude Code CLI), `git`, `jq`, `yq` (mikefarah v4+), `python3`

### Initialize a Project

```bash
cd your-project
arc init
```

This detects your language and test runner, then creates:
- `.arc.yaml` — project configuration (language, runner, git settings)
- `.plans/` — plan storage directory
- `.claude/commands/arc-plan.md` — `/arc-plan` slash command for Claude Code
- Git hooks for commit and file boundary enforcement
- Claude Code hooks for orchestrator/review agent restrictions

### Interactive Mode with `arc chat`

Launch an interactive Claude session with the Arc guide injected as system context:

```bash
arc chat                        # Start interactive session
arc chat --model opus           # Use a specific model
```

The chat agent has full access to the `arc` CLI via Bash and the Arc reference guide as system context. It can plan, review, run, and monitor Arc plans conversationally.

### Quick Start with `arc dispatch`

For the fastest path from idea to code, use `arc dispatch`:

```bash
arc dispatch "Add user authentication with JWT tokens"
```

This runs the full pipeline automatically: discovers relevant code, classifies task complexity, generates a plan with phases, runs adversarial review, and submits to the daemon for execution. Options:

```bash
arc dispatch --skip-review "Fix the login bug"          # Skip adversarial review
arc dispatch --timeout 7200 "Refactor the auth module"  # Custom timeout (seconds)
arc dispatch --interactive "Add caching layer"           # Prompt before review/launch
```

### Create and Run a Plan Manually

```bash
arc plan my-feature phase1 phase2 integration   # Create a plan with phases
vim .plans/active/my-feature/phases/phase1/plan.md  # Write phase plans
arc review my-feature                            # Adversarial review (auto-remediates)
arc review my-feature --phase phase1             # Review a single phase
arc daemon submit my-feature                     # Submit to daemon for execution
```

### Manual Mode

For step-by-step control instead of full automation:

```bash
arc status my-feature                     # Check plan/phase status
arc manage my-feature phase1 show         # Inspect phase state.json
```

### Validate Tests

```bash
arc validate                    # Audit test quality in current directory
arc validate ./pkg/auth ./pkg/db  # Audit specific paths
arc validate --workers 8        # Run with more parallel agents
arc validate set-prompt my.md   # Use a custom audit prompt
arc validate clear-prompt       # Revert to built-in prompt
```

### Monitor

```bash
arc monitor my-feature   # Live TUI for orchestration progress
```

### Adversarial Audit

```bash
arc audit                                            # Audit uncommitted changes
arc audit --branch feature/auth                      # Audit branch vs HEAD
arc audit --diff origin/main...HEAD                  # Audit a diff range
arc audit internal/api/auth.go                       # Audit specific files
arc audit --diff origin/main...HEAD --format github  # GitHub Actions CI mode
```

### Task Automation

```bash
arc task "Add user authentication"              # Plan, review, and run automatically
arc task --run=false "Add caching layer"        # Plan only, don't run
arc task --skip-review "Fix the login bug"      # Skip adversarial review
arc task --model opus "Refactor auth module"    # Model override
```

### Recipe Management

```bash
arc recipe list                                        # List available recipes
arc recipe show <name>                                 # Show recipe details
arc recipe instantiate <name> --param key=value        # Create plan from recipe
arc recipe instantiate <name> --run                    # Create and run immediately
```

### Daemon Management

```bash
arc daemon start                  # Start the background daemon (auto-detaches)
arc daemon start --foreground     # Run daemon in foreground
arc daemon stop                   # Drain and stop daemon after current work
arc daemon status                 # Show all running plans
arc daemon status --plan my-plan  # Show specific plan
arc daemon submit my-plan         # Submit plan to daemon for execution
arc daemon cancel my-plan         # Cancel a plan running in daemon
```

### Reset and Cancellation

```bash
arc reset my-plan                 # Reset plan: clean worktrees, reset states, remove locks
arc cancel my-plan                # Stop a running orchestrator process
arc wait my-plan                  # Block until plan completes or any phase is blocked
```

### Gate Checks

```bash
arc gate my-plan my-phase                    # Run gate assertions for a phase
arc gate my-plan my-phase --workdir /path   # Use a specific working directory
```

### Plan Spec Management

```bash
arc plan add-phase my-plan my-phase --spec "implement X" --role impl
arc plan update-phase my-plan my-phase --spec "updated objective"
arc plan update-gate my-plan my-phase --add-assertion "file_exists:internal/x.go"
arc plan update-deps my-plan my-phase --deps "phase1,phase2"
arc plan remove-phase my-plan my-phase
arc plan show-spec my-plan my-phase
```

## Phase Roles

Phase roles control how a phase agent is prompted and how its output is verified.

| Role | Description | Verification |
|------|-------------|--------------|
| `impl` (default) | Write code and implement features | Gate assertions (file_exists, grep, test_exists, build_passes, no_untracked) |
| `review` | Analyze code and produce findings | AI verifier agent |
| `investigate` | Research questions and document findings | AI verifier agent |
| `audit` | Security or quality audit | AI verifier agent |

Set the role in a phase spec:

```bash
arc plan add-phase my-plan my-phase --spec "..." --role review
```

Or in `spec.yaml`:

```yaml
role: review
```

## Gate System

Gates enforce objective acceptance criteria after each agent session. Each phase can define assertions in its `spec.yaml`:

```yaml
gate:
  assertions:
    - type: file_exists
      description: "Implementation file created"
      file_exists: internal/pkg/feature.go
    - type: grep
      description: "Function exported"
      grep: "func NewFeature"
    - type: test_exists
      description: "Test written"
      test_exists: TestNewFeature
    - type: build_passes
      description: "Code compiles"
      build_passes: "go build ./..."
    - type: no_untracked
      description: "No debug artifacts left"
      no_untracked: "true"
```

Gate results are persisted to `gate-status.json` in the phase directory. Use `arc gate <plan> <phase>` to run checks manually.

## Adapter System

Arc supports multiple AI providers through the adapter interface. The active adapter is selected per-role in `.arc.yaml` or per-phase in `spec.yaml`.

| Adapter | Provider | Preflight Check |
|---------|----------|-----------------|
| `claude` (default) | Claude Code CLI | Binary in PATH, auth check |
| `codex` | OpenAI Codex CLI | Binary in PATH |
| `generic` | Any CLI tool | Command in PATH |

Configure in `.arc.yaml`:

```yaml
agents:
  impl: claude
  adversary: claude
```

## How It Works

### Three-Level Hierarchy

```
Plan (e.g., "fix-wasm-rng")
  └── Phase: investigate-variance
  └── Phase: port-pcg-algorithm
  └── Phase: verify-cross-engine
```

- **Plan** — The overall work request. Contains one or more phases.
- **Phase** — A self-contained unit of work with its own gate assertions, test suite, and `state.json`.

### Execution Flow

```
arc plan             Create plan directory structure and phase scaffolding
    │
    ▼
arc review           Adversarial review validates each phase plan
    │
    ▼
arc daemon submit    Submit plan to daemon for execution
    │
    ▼
  ┌────────────────────────────────────────────┐
  │  For each phase:                           │
  │    spawn agent session                     │
  │        │                                   │
  │        ▼                                   │
  │    evaluate gate assertions                │
  │        │                                   │
  │        ├── pass ──► mark complete          │
  │        │                                   │
  │        └── fail ──► retry (up to limit)    │
  └────────────────────────────────────────────┘
    │
    ▼
COMPLETION_REPORT.md  Generated when all phases finish
```

### Execution Model

Each phase runs agent sessions until gate assertions pass or `MaxGatedAttempts` (2) is exhausted. On exhaustion the phase is marked **blocked**. The daemon watcher monitors blocked phases and auto-retries them up to 3 times before giving up.

## Work Types

`workflow_type` is metadata only — it does not define a phase sequence or control flow. All phases run through the same `session → gate → retry` loop regardless of type.

| Type | Typical Use |
|------|-------------|
| **feature** | New functionality |
| **bugfix** | Fixing incorrect behavior |
| **investigation** | Research and exploration, outputs documentation |
| **refactor** | Restructuring without behavior change |
| **performance** | Optimization driven by benchmarks |
| **direct** | Simple single-phase task (used by `arc dispatch`) |

## Gate Assertions

Gates enforce objective acceptance criteria after each agent session. Five assertion types are supported:

| Type | Description |
|------|-------------|
| `file_exists` | Checks that a file path exists on disk |
| `grep` | Searches for a pattern in the codebase |
| `build_passes` | Runs a build command and checks it exits 0 |
| `test_exists` | Verifies a named test function exists in source |
| `no_untracked` | Fails if untracked files remain (catches debug artifacts) |

## Git Worktree Isolation

Agents can run in isolated git worktrees so developers can keep working in the main tree:

```bash
arc daemon submit my-plan --per-phase-worktree   # Each phase gets its own worktree branch
```

Each phase gets a branch like `arc/my-plan/phase-name` in a temp directory. On completion, the worktree branch is merged back into the main branch. On failure, the branch is preserved for inspection but the worktree directory is cleaned up.

## `arc dispatch` Pipeline

`arc dispatch` automates the full lifecycle from task description to running code:

```
Task description
    │
    ▼
Discovery agent (read-only) ──► Analyzes codebase, classifies complexity
    │
    ▼
Complexity routing:
    ├── Simple: Single "direct" phase, no review
    ├── Medium: Multi-phase with built-in workflow, adversarial review
    └── Complex: 3 architect agents propose designs, best selected,
                 custom workflow generated, adversarial review
    │
    ▼
Daemon submission ──► Executes plan phases
    │
    ▼
SUMMARY.md ──► Generated on completion with stats, cost, files changed
```

## Plan Summaries

When a plan completes, a `SUMMARY.md` is generated in the plan directory containing:

- Objective (from first phase's `plan.md`)
- Phase completion status and per-phase details (iterations, tests, commits, cost)
- Files changed across all phases (collected from git history)
- Total cost and next steps for blocked/deferred phases

## Test Runners

Arc automatically resolves the test command for your project via `internal/testcmd`. Resolution priority: explicit override → `.arc.yaml` `test_command` → project detection → `go test ./...`.

| Runner | Language | Command |
|--------|----------|---------|
| `cargo-nextest` | Rust | `cargo nextest run` |
| `vitest` | TypeScript | `npx vitest run` |
| `pytest` | Python | `pytest` |
| `go-test` | Go | `go test ./...` |

The runner is selected automatically by `arc init` based on project detection, or set manually in `.arc.yaml`.

## State Tracking

Each phase maintains a `state.json` file:

```json
{
  "phase": "port-pcg",
  "status": "blocked",
  "iteration": 5,
  "tests_passing": 8,
  "tests_total": 12,
  "gate_status": "fail",
  "notes": ""
}
```


## Plan Review

Before running a plan, each phase goes through adversarial review with auto-remediation. The review system runs 5 specialized adversary agents in parallel:

| Adversary | Focus | Priority | Required |
|-----------|-------|----------|----------|
| **executability** | No blockers that prevent sub-agent execution | 1 (highest) | Yes |
| **consistency** | Types, names, and contracts match across phases | 2 | Yes |
| **coverage** | Every function and error variant has tests | 3 | Yes |
| **ambiguity** | Nothing a sub-agent could misinterpret | 4 | Yes |
| **scope** | Phase isn't too large to execute reliably | 5 (lowest) | No (warning only) |

When adversaries find issues, they emit structured suggestions (find-and-replace blocks targeting `plan.md`). Suggestions can include confidence scores:

```
<<<ORIGINAL (confidence: 85)
exact text from plan.md
<<<SUGGESTED
replacement text
>>>
```

The review loop:

1. Runs all 5 adversaries in parallel
2. Parses suggestions from any that failed
3. Filters by confidence threshold (default: 80) — low-confidence suggestions are dropped
4. Merges suggestions by priority (higher-priority adversary wins conflicts)
5. Applies fixes to `plan.md` mechanically
6. Re-reviews until all pass or iteration limit (5) is hit

Smart caching skips re-reviewing when `plan.md` hasn't changed (SHA256 hash match). Use `arc manage reset-review <plan> <phase>` to clear the cache and iteration counter.

## Sub-Agent Enforcement

The orchestrator agent cannot edit code directly. Sub-agents are restricted from running test commands directly — they must use the runner plugins. This is enforced through:

1. **Claude Code hooks** — Block orchestrator writes and restrict review agents to their output files.
2. **Git hooks** — Enforce file boundaries based on agent role (qa, impl, review).
3. **Watchdog** — Background process that kills unauthorized test processes.

## Phase Artifacts

Each phase directory (`.plans/active/<plan>/phases/<phase>/`) contains:

| File | Written By | Purpose |
|------|------------|---------|
| `plan.md` | Human / arc dispatch | Phase specification |
| `state.json` | Orchestrator | Current status, iteration count, test results, gate status |
| `spec.yaml` | Human / arc plan | Phase spec with gate assertions and role |
| `gate-status.json` | Gate evaluator | Gate assertion results |

## Directory Structure

```
arc/
├── cmd/arc/          CLI entry point (main.go)
├── internal/         All Go packages
│   ├── adapter/      Multi-provider AI adapter system (claude, codex, generic)
│   ├── agent/        Agent spawning
│   ├── arc/          Core types (result, errors, state, gate, spec)
│   ├── cli/          Cobra command definitions
│   ├── config/       .arc.yaml parsing
│   ├── daemon/       Background orchestration daemon
│   ├── dev/          Arc dispatch pipeline (discovery → architecture → plan generation)
│   ├── gate/         Gate assertion evaluation
│   ├── gitops/       Git commit operations
│   ├── guide/        Agent-facing reference guide
│   ├── intelligence/ Project intelligence store
│   ├── logging/      Structured logger
│   ├── migrate/      State migration
│   ├── monitor/      Live TUI (bubbletea)
│   ├── orchestrator/ Top-level orchestrator loop
│   ├── plan/         Plan creation, status & summary generation
│   ├── project/      Project detection & init
│   ├── prompt/       Prompt rendering
│   ├── resources/    Embedded prompts, templates & guides
│   ├── review/       Adversarial plan review
│   ├── selfupdate/   GitHub Releases-based self-update
│   ├── state/        Phase state (state.json) management
│   ├── testcmd/      Test command abstraction (resolution + execution)
│   ├── validate/     AI-powered test quality audit
│   └── worktree/     Git worktree isolation for parallel execution
├── testdata/         Test fixtures
└── docs/             Detailed documentation
```

In each project that uses arc:

```
your-project/
├── .arc.yaml         Project configuration (language, runner, git settings)
├── .plans/
│   ├── active/       Plans currently being worked on
│   │   └── <plan>/
│   │       ├── plan.json
│   │       └── phases/
│   │           └── <phase>/
│   │               ├── plan.md
│   │               └── state.json
│   └── archive/      Completed plans
└── .claude/
    ├── settings.json  Claude Code hooks (installed by arc init)
    └── commands/
        └── arc-plan.md  /arc-plan slash command
```

## Further Reading

| Document | Content |
|----------|---------|
| `docs/ARCHITECTURE.md` | System design goals and component overview |
| `docs/STATE_SCHEMA.md` | `state.json` field definitions |
| `docs/ADVERSARY_SYSTEM.md` | Plan review design |
| `docs/PLANNING_PROCESS.md` | How to write phase plans |
| `docs/PROMPT_TEMPLATES.md` | Template variable system |
