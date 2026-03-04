# Agent

Spawns Claude CLI sub-agents as child processes. Single file, single function.

## File Map

| File | Purpose |
|------|---------|
| `spawn.go` | `Spawn()` — builds and executes `claude --print --output-format json` as a subprocess. Pipes prompt via stdin, captures JSON-envelope output, extracts token usage. |

## Key Details

- **Defaults**: command = `"claude"`, max turns = 15, timeout = 600s, format = `"json"`, tools = `["View","Edit","Write","Bash"]`.
- **Process group isolation**: `Setpgid: true` so the whole group can be killed on timeout via `SIGKILL`.
- **Strips `CLAUDECODE` env var** so nested invocations aren't blocked by Claude's nested-session guard.
- **Timeout is graceful**: sets `TimedOut: true` rather than returning an error, allowing callers to handle partial output.
- **JSON envelope**: `parseJSONOutput` unwraps the `claude --output-format json` shape (`result` + `usage` + `total_cost_usd`).

## Test Mock

`testdata/mockagent/main.go` substitutes for the real `claude` CLI in tests. Controlled via env vars (`MOCK_OUTPUT`, `MOCK_EXIT_CODE`, `MOCK_SLEEP_MS`, `MOCK_JSON_WRAP`, `MOCK_SCRIPT_DIR` for sequential scripted responses).
