You have Arc orchestration tools available via MCP. Arc is an AI-powered workflow engine for multi-phase software engineering tasks.

## Quick Start

- **Simple tasks**: Use `arc_dev` — it auto-generates a plan, reviews it, and runs the orchestrator.
- **Complex tasks**: Compose tools manually: `arc_plan` → `arc_review` → `arc_run`.

## Available Tools

| Tool | Purpose |
|------|---------|
| `arc_dev` | End-to-end: analyze task, generate plan, review, execute |
| `arc_plan` | Create a plan with named phases |
| `arc_review` | Adversarial review (required before `arc_run`) |
| `arc_run` | Launch orchestrator for all phases (long-running) |
| `arc_iterate` | Run a single phase iteration |
| `arc_status` | Show plan/phase status |
| `arc_list_plans` | List all active plans |
| `arc_manage` | Manage phase state (complete, defer, block, etc.) |
| `arc_archive` | Archive a completed plan |
| `arc_guide` | Print the full Arc reference guide |

## Workflow Types

- **feature** — TDD: qa → qa_review → impl → impl_review
- **bugfix** — investigate → regression_tests → test_review → fix → fix_review
- **refactor** — characterize → char_review → refactor → verify
- **investigation** — research → draft → review
- **direct** — Single-phase impl (good for simple tasks)

## Key Notes

- Plans must be reviewed (`arc_review`) before running (`arc_run`). Review status must be "approved" or "conditional".
- `arc_run` is long-running — it launches the full orchestrator loop across all phases.
- Each phase needs a `plan.md` file describing what to do. `arc_plan` creates scaffolding but you should fill in the plan content.
- Use `arc_status` to check progress at any time.
- Use `arc_guide` for detailed reference on plan structure and workflow mechanics.
