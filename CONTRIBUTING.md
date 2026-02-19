# Contributing to Arc

## Development Setup

```bash
# Clone the repo
git clone https://github.com/nickw409/arc.git ~/projects/arc
cd ~/projects/arc

# Build and install
go install ./cmd/arc/
```

### Running a dev build

```bash
# Build with dev version
go build -o arc ./cmd/arc/
./arc --version   # "arc version dev"

# Or run directly
go run ./cmd/arc/ --version
```

### Running tests

```bash
go test ./...                          # All tests
go test ./internal/selfupdate/         # Single package
go test ./internal/runner/ -run TestName  # Single test
```

## Releases

Releases are automated via [goreleaser](https://goreleaser.com/) and GitHub Actions. To create a release:

```bash
git tag v0.4.0
git push origin v0.4.0
```

The `.github/workflows/release.yml` workflow will:
1. Run the full test suite
2. Build cross-platform binaries (linux/darwin x amd64/arm64)
3. Publish a GitHub Release with tarballs and checksums

Version is injected at build time via `-ldflags -X github.com/nwiley/arc/internal/cli.Version=...`.

To test the release config locally:

```bash
goreleaser check                      # Validate config
goreleaser build --snapshot --clean   # Local test build (no publish)
```

## Project Structure

See the [README](README.md#directory-structure) for the full directory layout. Key areas for contributors:

- `cmd/arc/` — CLI entry point
- `internal/cli/` — Cobra command definitions
- `internal/orchestrator/` — Top-level orchestrator loop
- `internal/pipeline/` — Phase iteration, escalation, hooks, constraints
- `internal/selfupdate/` — GitHub Releases-based self-update
- `workflows/` — YAML state machine definitions
- `prompts/` — prompt templates by work type
- `runners/` — test runner plugins (one per language/tool)

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

## Adding a Workflow

Workflow definitions live in `workflows/<name>.yaml`. See `docs/WORKFLOW_SCHEMA.md` for the full spec. Each workflow needs:

1. A YAML state machine in `workflows/`
2. Prompt templates in `prompts/<name>/` (one per state)
