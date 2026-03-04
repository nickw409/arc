# MCP

MCP server exposing Arc tools to AI agents via stdio transport. This is the backend for `arc chat` and `arc serve`.

**Start here:** `server.go` for lifecycle, `tools.go` for all tool implementations.

## File Map

| File | Purpose |
|------|---------|
| `server.go` | `Run()` — entry point. Creates MCP server, registers tools, listens on stdio. Manages `jobsCtx` for background job cancellation and `drainJobs` for graceful shutdown. |
| `tools.go` | All 11 tool handler implementations and `handlerContext` (shared state including the `jobs` map for tracking async runs). |

## Tools Registered

| Tool | Sync/Async | Handler |
|------|-----------|---------|
| `arc_plan` | sync | Creates plan scaffolding |
| `arc_review` | sync | Runs adversarial review (phases concurrent, max 3) |
| `arc_run` | **async** | Launches orchestrator in goroutine, returns immediately |
| `arc_run_status` | sync | Polls/checks async run job |
| `arc_run_cancel` | sync | Cancels async run |
| `arc_iterate` | sync | Single phase iteration via `pipeline.RunState` |
| `arc_manage` | sync | Phase state mutations (12 sub-actions) |
| `arc_status` | sync | Plan/phase status display |
| `arc_list_plans` | sync | List all active plans |
| `arc_archive` | sync | Move plan to archive |
| `arc_guide` | sync | Print agent reference guide |

## Key Design Decisions

- **Only `arc_run` is async** — it spawns `orchestrator.Launch` in a goroutine tracked via `runJob` with a `Done` channel. All other handlers block until complete.
- **`Done` channel pattern** over WaitGroup — enables non-blocking status checks via `select`.
- **`handleRunStatus` releases and reacquires the mutex mid-function** — can't hold the lock during `plan.Status` file I/O.
- **Stale job cleanup**: re-running a plan whose previous job finished silently replaces the entry. Still-running jobs return an error.
- **10-second drain timeout** on shutdown — accepts potential orphan processes.
- **`handleReview`** writes results directly to `plan.json` rather than delegating to `plan.*` functions.
