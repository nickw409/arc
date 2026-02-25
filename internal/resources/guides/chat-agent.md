You have Arc orchestration tools available via MCP. Arc is an AI-powered workflow engine that breaks complex software engineering tasks into phases, each driven by a state machine with AI agents. You also have your normal Claude Code tools (Read, Edit, Bash, etc.) — use those for simple tasks and Arc for complex multi-step work.

## When to Use Arc vs Normal Tools

**Use your normal tools** (Read, Edit, Bash, etc.) for:
- Quick fixes, typos, small refactors
- Answering questions about the codebase
- Running tests, checking status
- Anything completable in a few edits

**Use `arc_dev`** for:
- Medium tasks where you'd benefit from automated planning and execution
- Tasks you could describe in a sentence or two
- When you're unsure about complexity — `arc_dev` analyzes the task and picks the right approach

**Use `arc_plan` → `arc_review` → `arc_run`** for:
- Large multi-phase work (new features, major refactors)
- Tasks where you want fine control over the plan and phases
- When you need to customize workflow type or phase structure

## Quick Start

1. **Simplest path**: `arc_dev` with a task description — it handles everything.
2. **Manual path**: `arc_plan` → fill in plan.md → `arc_review` → `arc_run`.
3. **Check on things**: `arc_status` or `arc_list_plans` at any time.

## Available Tools

| Tool | Purpose |
|------|---------|
| `arc_dev` | End-to-end: analyze task, generate plan, review, execute |
| `arc_plan` | Create a plan with named phases and workflow type |
| `arc_review` | Adversarial review with auto-remediation (required before `arc_run`) |
| `arc_run` | Launch orchestrator asynchronously — returns immediately |
| `arc_run_status` | Check progress of a running/completed arc_run |
| `arc_run_cancel` | Cancel a running arc_run |
| `arc_iterate` | Run a single phase iteration |
| `arc_status` | Show plan/phase status |
| `arc_list_plans` | List all active plans with workflow type and review status |
| `arc_manage` | Manage phase state (see actions below) |
| `arc_archive` | Archive a completed plan |
| `arc_guide` | Print the full Arc reference guide (detailed) |

## Workflow Types

Choose the workflow type that matches the task:

| Type | Flow | When to use |
|------|------|-------------|
| **feature** | qa → qa_review → impl → impl_review | New functions, types, modules, APIs |
| **bugfix** | investigate → regression_tests → test_review → fix → fix_review | Fixing incorrect behavior |
| **refactor** | characterize → char_review → refactor → verify | Restructuring without changing behavior |
| **investigation** | research → draft → review | Research, audits, answering questions (no code changes) |
| **performance** | baseline → analyze → optimize → benchmark | Making code faster, reducing memory |
| **adversarial** | impl → adversary-loop | Implementation with adversarial testing |
| **audit** | adversary-loop | Adversarial-only testing of existing code |
| **direct** | impl | Single-phase, simple tasks |

## Writing Good Plans

When using `arc_plan`, the scaffolding creates empty `plan.md` templates. Each phase's `plan.md` needs:

- **Objective** — One sentence describing what the phase accomplishes
- **Files** — Explicit paths to create/modify (no vague references)
- **Types and Signatures** — Complete, exact function signatures. No pseudocode
- **Test Cases** — Concrete inputs and expected outputs with real values
- **DO NOT** — Anticipated mistakes the agent should avoid
- **Edge Cases** — Enumerated boundary conditions

Key rules:
- Be concrete, not vague. "Handle errors appropriately" → write the exact error types.
- Keep phases small — aim for ~15 iterations max. If a phase touches >10 files, split it.
- Test cases need real values: name, input, expected output. Not "test the edge cases."
- Call `arc_guide` for the full plan template and detailed guidelines.

## Managing Phases

`arc_manage` supports these actions:

| Action | Required args | Purpose |
|--------|--------------|---------|
| `show` | — | Display phase state.json |
| `complete` | — | Mark phase as done |
| `pending` | — | Reset phase to pending |
| `defer` | `reason` | Defer phase with explanation |
| `block` | `reason` | Block phase (needs human intervention) |
| `tests` | `passing`, `total` | Update test counts |
| `packages` | `packages` (comma-separated) | Set package list |
| `note` | `note` | Set phase notes |
| `iteration` | `iteration` | Set iteration number |
| `copy-from` | `source_phase` | Copy state from another phase |

## Supervising Runs

`arc_run` is async — it launches the orchestrator in the background and returns immediately. This lets you do other work (plan the next task, answer questions, read code) while phases execute. Use this workflow to supervise:

1. **Start the run**: `arc_run` with the plan name. You get back a confirmation immediately.
2. **Poll for progress**: Call `arc_run_status` periodically (every 30-60 seconds for long runs, or when the user asks). It shows elapsed time and current phase states.
3. **On success**: `arc_run_status` returns the final result with status "complete" and a phase summary. Report this to the user and suggest `arc_archive` to clean up.
4. **On failure**: The run stops automatically when a phase fails. `arc_run_status` returns status "failed" with the failed phase name and reason. Now you can intervene:
   - Read the failed phase's state: `arc_manage` with action "show"
   - Inspect the code and test output using your normal tools (Read, Bash)
   - Fix the issue directly with Edit, or reset the phase with `arc_manage` action "pending"
   - Re-run with `arc_run`
5. **To cancel**: `arc_run_cancel` stops a running orchestrator. Use this if the user wants to abort or if you need to change the plan mid-run.

**Key point**: You are not blocked while a run is in progress. You can plan ahead, investigate other parts of the codebase, or work on unrelated tasks. Check back on the run when you're ready or when the user asks.

## Important Notes

- **Review before run**: `arc_run` requires review status "approved" or "conditional". Always run `arc_review` first.
- **`arc_dev` is self-contained**: It handles planning, review, and execution automatically. Use `arc_dev` when you don't need manual control.
- **`arc_iterate` for single steps**: If you want to advance one phase by one iteration (useful for debugging or manual control).
- **Archive when done**: Use `arc_archive` to move completed plans out of the active directory.
