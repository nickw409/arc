# Orchestration Reference

Full reference for the orchestration system. This document is for **human reference** — agents use the slimmed-down ORCHESTRATOR.md and SUB_AGENT_PROTOCOL.md.

## Architecture

```
Human (occasional input)
   ↓
Orchestrator (Claude Code session - persistent context)
   ↓ spawns via iterate.sh
Sub-agents (fresh context each time, isolated work, terminates)
```

## Folder Structure

```
$ARC_HOME/
├── ORCHESTRATOR.md                 # Decision tree for orchestrator (short)
├── SUB_AGENT_PROTOCOL.md           # Protocol injected into sub-agents
├── REFERENCE.md                    # This file (human reference)
├── active/                         # Plans currently being worked on
│   └── <plan-name>/
│       ├── session_id              # Shared session for all phases
│       ├── plan.json               # Metadata: phases, dependencies, status
│       └── phases/
│           └── <phase>/
│               ├── plan.md         # What this phase implements
│               └── state.json      # Progress tracking
├── archive/                        # Completed plans
├── templates/
│   └── plan-template.md            # Template for phase plans
└── scripts/
    ├── init-plan.sh                # Initialize a new plan
    ├── iterate.sh                  # Spawn sub-agent for one iteration
    ├── get-state.sh                # Read current state (human-readable)
    ├── update-state.sh             # Update state atomically
    └── archive-plan.sh             # Archive completed plan
```

## State Model

Each phase has exactly one state:

| State | Meaning |
|-------|---------|
| `PENDING` | Phase not started |
| `IN_PROGRESS` | Work is ongoing |
| `COMPLETE` | All tests pass |
| `BLOCKED` | Dispute or error requires intervention |

### state.json Schema

```json
{
  "status": "IN_PROGRESS",
  "iteration": 3,
  "max_iterations": 50,
  "tests_passing": 5,
  "tests_total": 12,
  "last_commit": "abc123",
  "blocked_reason": null,
  "dispute": null
}
```

When blocked by dispute:
```json
{
  "status": "BLOCKED",
  "blocked_reason": "dispute",
  "dispute": {
    "test_name": "test_serialize_empty",
    "claim": "Test expects error but plan section 2.1 says empty is valid",
    "filed_at": "2024-01-15T10:30:00Z",
    "resolution": null,
    "resolution_reason": null
  }
}
```

## Scripts Reference

### init-plan.sh

```bash
$ARC_HOME/scripts/init-plan.sh <plan-name> <phase1> <phase2> ...
```

Creates:
- `active/<plan-name>/session_id` (UUID for sub-agent sessions)
- `active/<plan-name>/plan.json` (metadata)
- `active/<plan-name>/phases/<phase>/state.json` (for each phase)
- `active/<plan-name>/phases/<phase>/plan.md` (empty, to be filled)

### iterate.sh

```bash
$ARC_HOME/scripts/iterate.sh <plan> <phase> <mode>
# mode: qa | impl | fix
```

Spawns a sub-agent with:
- SUB_AGENT_PROTOCOL.md (with variables substituted)
- The phase's plan.md
- Fresh context (no orchestrator history)

Uses `claude -p` to ensure clean termination.

### get-state.sh

```bash
$ARC_HOME/scripts/get-state.sh <plan> <phase>
```

Outputs human-readable state:
```
Phase: 01-types
Status: IN_PROGRESS
Iteration: 3/50
Tests: 5/12 passing
Last commit: abc123
```

### update-state.sh

```bash
# Update status
$ARC_HOME/scripts/update-state.sh <plan> <phase> status IN_PROGRESS

# File a dispute (called by sub-agent)
$ARC_HOME/scripts/update-state.sh <plan> <phase> dispute "test_name" "reason"

# Resolve dispute (called by orchestrator)
$ARC_HOME/scripts/update-state.sh <plan> <phase> reject-dispute "reason"
$ARC_HOME/scripts/update-state.sh <plan> <phase> approve-dispute "reason"

# Clear dispute after fix (called by sub-agent in fix mode)
$ARC_HOME/scripts/update-state.sh <plan> <phase> clear-dispute

# Record test progress
$ARC_HOME/scripts/update-state.sh <plan> <phase> tests 5 12

# Record commit
$ARC_HOME/scripts/update-state.sh <plan> <phase> commit "abc123"
```

All updates are atomic (write to temp file, then move).

### archive-plan.sh

```bash
$ARC_HOME/scripts/archive-plan.sh <plan-name>
```

1. Moves `active/<plan>/` to `archive/<plan>/`
2. Removes session folder from `~/.claude/`
3. Updates plan.json status to "archived"

## Plan Writing Guide

See `templates/plan-template.md` for the template.

### What Makes a Plan "Sub-Agent Proof"

Sub-agents work in isolation. They only see:
- The plan.md file
- The codebase
- Their mode and iteration

They do NOT see:
- Original conversation where plan was conceived
- Clarifications discussed with orchestrator
- Your mental model

**If ambiguous, sub-agents will guess. They will guess wrong.**

### Required Elements

1. **Exact signatures** — Full Rust/TS signatures with generics, bounds, return types
2. **Explicit file paths** — `my-package/src/data.rs`, not "the server crate"
3. **Specific error types** — Full error enum with message formats
4. **DO NOT section** — Common mistakes to avoid
5. **Dependencies with versions** — Exact Cargo.toml/package.json additions
6. **Test cases** — Concrete inputs and expected outputs
7. **Edge cases** — Enumerated explicitly
8. **Integration points** — What other phases consume/depend on this

### Plan Refinement Process

One phase at a time:

1. Orchestrator reads current plan.md back to human
2. Human points out gaps, asks clarifying questions
3. Iterate until plan is "sub-agent proof"
4. Write finalized plan to disk
5. Move to next phase (previous plan exits working memory)

This avoids context overload.

## Branching Strategy

All work happens on a **single plan branch** (`plan-<plan-name>`).

- Created from `develop` (or parent branch)
- QA and impl commits accumulate on same branch
- Each phase ends with a completion commit
- Merged back to parent when all phases complete

```
develop
   └── plan-cuda-opt
          ├── test(01-types): add QA tests
          ├── feat(01-types): implement data types
          ├── feat(01-types): complete phase
          ├── test(02-kernels): add QA tests
          ├── feat(02-kernels): implement kernels
          └── feat(02-kernels): complete phase
```

## Dispute Resolution

When a sub-agent files a dispute:

1. **Read the dispute** via `get-state.sh`
2. **Read the plan section** cited by sub-agent
3. **Read the test** in question
4. **Decide**:
   - Test is correct → `update-state.sh reject-dispute "reason"` → continue impl
   - Test is wrong → `update-state.sh approve-dispute "reason"` → run fix mode → continue impl
5. **Escalate to human if**:
   - Plan text is genuinely ambiguous
   - Same test disputed twice
   - <80% confidence

## Troubleshooting

### Sub-agent keeps making the same mistake

The plan is ambiguous. Add explicit DO NOT or edge case.

### Tests keep failing after many iterations

1. Check if sub-agent is modifying tests (shouldn't be)
2. Check if plan is realistic (maybe spec is wrong)
3. Check iteration state — is progress being made?

### Dispute loop (same test keeps getting disputed)

Plan is fundamentally ambiguous. Stop and clarify with human.

### Sub-agent exits without committing

Check for errors in iterate.sh output. Sub-agent may have crashed.

## Example Full Session

```
Human: start plan dist-fitting with phases 01-types, 02-algorithms

Orchestrator:
  $ $ARC_HOME/scripts/init-plan.sh dist-fitting 01-types 02-algorithms
  $ git checkout -b plan-dist-fitting develop
  
  "Plan initialized. Let's refine phase 01-types. Here's the template..."
  [refines plan with human]
  [writes plan.md]
  
  "Phase 01-types plan finalized. Starting QA..."
  $ $ARC_HOME/scripts/iterate.sh dist-fitting 01-types qa
  
  "QA complete: 12 tests in my-package/tests/qa_01_types.rs"
  
  "Starting implementation..."
  $ $ARC_HOME/scripts/iterate.sh dist-fitting 01-types impl
  $ $ARC_HOME/scripts/get-state.sh dist-fitting 01-types
  # Status: IN_PROGRESS, Tests: 8/12
  
  $ $ARC_HOME/scripts/iterate.sh dist-fitting 01-types impl
  $ $ARC_HOME/scripts/get-state.sh dist-fitting 01-types
  # Status: BLOCKED, Dispute on test_minimum_size
  
  "Dispute: test expects minimum 2 elements, impl says 1. Plan section 2.1 says 'at least 2 for statistical validity'. Rejecting dispute."
  $ $ARC_HOME/scripts/update-state.sh dist-fitting 01-types reject-dispute "Plan section 2.1 explicit"
  
  $ $ARC_HOME/scripts/iterate.sh dist-fitting 01-types impl
  $ $ARC_HOME/scripts/get-state.sh dist-fitting 01-types
  # Status: COMPLETE, Tests: 12/12
  
  $ git commit --allow-empty -m "feat(01-types): complete phase - core types"
  
  "Phase 01-types complete. Moving to 02-algorithms..."
  [continues]
```
