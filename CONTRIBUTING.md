# Contributing to Arc

## Development Setup

Arc is a collection of shell scripts, so there's no build step. But since arc can orchestrate changes to itself, you need to keep a stable copy separate from your development copy to avoid breaking your tooling mid-edit.

### Install both copies

```bash
# 1. Clone the repo (your development copy)
git clone https://github.com/nickw409/arc.git ~/projects/arc

# 2. Run the installer (creates a stable copy at ~/.arc)
~/projects/arc/install.sh

# 3. Symlink the stable copy as `arc`
ln -sf ~/.arc/bin/arc ~/.local/bin/arc

# 4. Symlink the dev copy as `arc-dev`
ln -s ~/projects/arc/bin/arc ~/.local/bin/arc-dev
```

This gives you:

| Command | Points to | Purpose |
|---------|-----------|---------|
| `arc` | `~/.arc/bin/arc` | Stable — safe to use in all projects |
| `arc-dev` | `~/projects/arc/bin/arc` | Development — reflects your working tree |

### Day-to-day workflow

1. Make changes in `~/projects/arc`
2. Test with `arc-dev` (e.g., `arc-dev init`, `arc-dev plan ...`)
3. Commit and push
4. Run `arc update` to pull changes into the stable `~/.arc` copy

This way a broken edit in the dev repo never takes down `arc` in your other projects.

## Running Tests

Arc uses [BATS](https://github.com/bats-core/bats-core) (Bash Automated Testing System):

```bash
cd ~/projects/arc
bats tests/              # run all tests
bats tests/actions.bats  # run a specific test file
```

## Project Structure

See the [README](README.md#directory-structure) for the full directory layout. Key areas for contributors:

- `bin/arc` — CLI entry point and command routing
- `scripts/` — core execution engine (iterate, state management, orchestrator)
- `workflows/` — YAML state machine definitions
- `prompts/` — prompt templates by work type
- `runners/` — test runner plugins (one per language/tool)
- `hooks/` — Claude Code hooks for agent enforcement
- `enforcement/` — git hooks, validation, schemas
- `tests/` — BATS test suite

## Adding a Test Runner

Runner plugins live in `runners/<name>/` and must provide a `run.sh` with this interface:

```bash
./run.sh <test_target> [--filter PATTERN] [--timeout SECONDS] [--extra-args ARGS]
```

Output must be JSON:

```json
{
  "total": 12,
  "passed": 10,
  "failed": 2,
  "failed_names": ["test_foo", "test_bar"],
  "raw_output": "..."
}
```

Exit codes: 0 (all pass), 1 (failures), 2 (no tests found).

After adding the runner, update `scripts/init-project.sh` to detect it and add it as a valid `runner` value in the `.arc.yaml` docs.

## Adding a Workflow

Workflow definitions live in `workflows/<name>.yaml`. See `docs/WORKFLOW_SCHEMA.md` for the full spec. Each workflow needs:

1. A YAML state machine in `workflows/`
2. Prompt templates in `prompts/<name>/` (one per state)
3. Detection logic in `scripts/init-plan.sh` if it should be auto-selectable via `--type`
