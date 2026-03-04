# Gitops

Git commit operations. Single file.

## File Map

| File | Purpose |
|------|---------|
| `commit.go` | `Commit()` — stages all changes (`git add -A`) and commits. Skips if working tree is clean. Optional GPG signing via config. `FormatCommitMessage()` — conventional (`type(scope): desc`) or freeform style. |
