# Worktree

Git worktree isolation for parallel phase execution. Single file.

## File Map

| File | Purpose |
|------|---------|
| `worktree.go` | `Create` — creates a git worktree with branch `arc/{plan}[/{phase}]`. `Remove` — force-removes worktree and deletes branch. `MergeBack` — merges branch into main with `--no-ff`, aborts on conflict. `CleanupPlan` — removes all worktrees matching a plan prefix. |

## Key Details

- Branch names are sanitized: non-alphanumeric → `-`, collapsed runs, trimmed trailing punctuation.
- Two modes: **shared** (one worktree per plan, created by orchestrator) and **per-phase** (one per phase, created by RunPhase).
- `MergeBack` aborts on conflict with `git merge --abort` rather than attempting resolution.
- `CleanupPlan` parses `git worktree list --porcelain` to find matching branches.
- `Create` handles stale branches: if a branch exists from a previous run but the worktree was removed, it deletes the branch and recreates from baseBranch to guarantee a clean starting point.
