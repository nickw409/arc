# Orchestration System Architecture

## Overview

The orchestration system is a general-purpose workflow executor that handles any type of software engineering work: features, bug fixes, investigations, refactors, and performance optimizations.

**Core Principle:** Scripts define the game rules. Agents play the game.

## Design Goals

1. **Work-type agnostic** - Same system handles features, bug fixes, investigations, etc.
2. **Portable** - Can be used in any project
3. **Reliable** - Critical decisions enforced by scripts, not agent memory
4. **Flexible** - Workflows are data, not code
5. **Safe** - Multiple escape hatches when things go wrong

## Work Types Supported

| Type | Description | Key Difference |
|------|-------------|----------------|
| Feature | New capability | TDD - tests written first, fail initially |
| Bug Fix | Correct existing behavior | Tests define correct behavior, current code fails them |
| Investigation | Produce findings | No code changes, output is documentation |
| Refactor | Change structure | Characterization tests pass before AND after |
| Performance | Make faster | Benchmarks, not unit tests |

## System Components

```
$ARC_HOME/
+-- workflows/           # Workflow definitions (YAML)
+-- prompts/            # Prompt templates (Markdown)
+-- templates/          # Plan templates per work type
+-- adversaries/        # Adversarial review definitions
+-- scripts/            # Execution scripts (Bash)
+-- docs/               # This documentation
```

## Plan / Phase / State Hierarchy

Understanding the three-level hierarchy is critical:

```
Plan: fix-wasm-rng
+-- Phase: investigate-variance      <-- A portion of work
|   +-- States: research -> draft -> review -> complete
+-- Phase: port-pcg-algorithm        <-- Another portion
|   +-- States: qa -> qa_review -> impl -> impl_review -> complete
+-- Phase: verify-cross-engine       <-- Final portion
    +-- States: qa -> qa_review -> impl -> impl_review -> complete
```

| Concept | What it is | Example |
|---------|------------|---------|
| **Plan** | The overall work request | "Fix WASM RNG variance issue" |
| **Phase** | A self-contained portion of the plan | "Port PCG algorithm to WASM" |
| **State** | Current position in the workflow | `impl` (implementing) |

### Key Points

1. **Each phase runs independently through the workflow**
   - Phase A being in `impl` doesn't affect Phase B
   - Each phase has its own `state.json`

2. **Phases can have dependencies**
   - Phase B might depend on Phase A completing
   - The orchestrator manages execution order

3. **The workflow defines possible states**
   - All phases of the same type use the same workflow
   - A bugfix plan uses bugfix.yaml for all its phases

### Phase Execution Order

The orchestrator executes phases based on dependencies:

```
+-------------------------------------------------------------+
| Phase Dependencies                                          |
|                                                             |
|   investigate-variance (no deps)                            |
|          |                                                  |
|          v                                                  |
|   port-pcg-algorithm (depends on: investigate-variance)     |
|          |                                                  |
|          v                                                  |
|   verify-cross-engine (depends on: port-pcg-algorithm)      |
|                                                             |
+-------------------------------------------------------------+
```

**Execution rules:**
- Phases with no dependencies start immediately
- Phases wait for all dependencies to reach `complete` state
- Independent phases MAY run in parallel (V5+)
- If a dependency is `blocked`, dependent phases are also blocked

### Phase Dependency Specification

Dependencies are declared in plan.md frontmatter:

```yaml
---
type: feature
depends_on:
  - phase: investigate-variance
    required: true  # Must complete, not just exist
---
```

Or in the plan's manifest:

```yaml
# .plans/active/<plan>/manifest.yaml
phases:
  - name: investigate-variance
    depends_on: []
  - name: port-pcg-algorithm
    depends_on: [investigate-variance]
  - name: verify-cross-engine
    depends_on: [port-pcg-algorithm]
```

## Execution Model

```
Human Request
     |
     v
+--------------+
| Plan Agent   |------> Generates plan.md + workflow.yaml
+--------------+
     |
     v
+--------------+
| Adversary    |------> Attacks plan until flawless
| Committee    |<------  Plan Agent fixes issues
+--------------+
     |
     v
+--------------+
| Orchestrator |------> Reads workflow, calls arc iterate
+--------------+
     |
     v
+--------------+
| iterate.sh   |------> Validates, runs sub-agent, enforces constraints
+--------------+
     |
     v
+--------------+
| Sub-Agent    |------> Does actual work within constraints
+--------------+
```

## Decision Tiers

| Tier | Decision Type | Who Decides | Enforcement |
|------|--------------|-------------|-------------|
| Critical | Commit, mark complete, modify tests | Script gate | Hard block |
| Structural | State transitions, retry vs escalate | Workflow rules | Script enforces |
| Tactical | Which file to edit, how to implement | Agent | No enforcement |

## Key Innovations

### 1. Workflow Inheritance
Plans can extend base workflows and customize for specific needs:
```yaml
extends: bugfix
states:
  - name: cross_verify
    insert_after: fix
```

### 2. Adversarial Planning
Multiple specialized adversaries attack plans:
- Coverage adversary
- Ambiguity adversary
- Scope adversary
- Consistency adversary
- Executability adversary

### 3. Context Injection
Orchestrator can inject context to sub-agents via:
- Template variables in prompts
- orchestrator_notes.md file
- Escalation context from state

### 4. Intervention System
When orchestrator can't proceed:
1. Proposes workflow changes (human approves)
2. Requests human intervention
3. Emergency override mode available

## Sub-Agent Enforcement

Sub-agents spawned by the orchestrator must be prevented from running test commands directly (they must use `$ARC_SCRIPTS_DIR/run-phase-tests.sh`). Three layers enforce this:

### Layer 1: PATH Shims (Primary)

`$ARC_HOME/bin/` contains wrapper scripts for test runners. `arc run-orchestrator` prepends this directory to PATH, so when a sub-agent runs the test command, our shim executes instead. The shim checks `ORCHESTRATOR_MODE=1` and returns exit 1 with a clear error message:

```
BLOCKED: Orchestrator sub-agents cannot run test commands directly.
Use $ARC_SCRIPTS_DIR/run-phase-tests.sh instead.
```

This gives the sub-agent an immediate, actionable error rather than a mysterious failure.

### Layer 2: Tool Restrictions

Review agents (qa-review, impl-review) are not given Bash access at all -- they can only Read, Glob, Grep, and Write. This eliminates the possibility of running any commands.

### Layer 3: Background Process Killer (Fallback)

A background process in `arc run-orchestrator` kills any unauthorized test processes every 5 seconds. This catches edge cases where a sub-agent bypasses the shim (e.g., using an absolute path).

### Why Not Hooks?

Claude Code hooks (`PreToolUse` in `.claude/settings.json`) work for the top-level agent but **not for sub-agents**. The `CLAUDE_TOOL_INPUT` environment variable is empty when hooks fire for sub-agent tool calls, so the hook cannot inspect the command being run. This was confirmed on Claude Code 2.1.32. The PATH shim approach was adopted because it works at the OS level regardless of Claude Code internals.

## Version Evolution

| Version | Features | Trigger to Upgrade |
|---------|----------|-------------------|
| V1 | Linear workflows, basic prompts | Starting point |
| V2 | Conditional branches | Reviews need retry logic |
| V3 | Parameters, templates | Copy-pasting prompts |
| V4 | Hooks, escalation | Custom logic in iterate.sh |
| V5 | Parallel states | Sequential too slow |

All versions are backwards compatible - V1 workflows run on V5 engine.

## Related Documents

- [WORKFLOW_SCHEMA.md](./WORKFLOW_SCHEMA.md) - Complete workflow file specification
- [ADVERSARY_SYSTEM.md](./ADVERSARY_SYSTEM.md) - Adversarial planning design
- [PLANNING_PROCESS.md](./PLANNING_PROCESS.md) - How plans are created
- [INTERVENTION_SYSTEM.md](./INTERVENTION_SYSTEM.md) - Escape hatches and overrides
- [PROMPT_TEMPLATES.md](./PROMPT_TEMPLATES.md) - Template variable system
- [../ISSUES.md](../ISSUES.md) - Known issues and resolutions
