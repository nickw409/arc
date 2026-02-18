# Orchestration System Issues

Tracked issues to fix. Check off as completed.

## Critical Issues

- [x] **1. Single dispute at a time** - Can only track one dispute. If 5 tests are wrong, can only dispute 1. **FIXED: Now uses disputes array**

- [x] **2. Timeout kills mid-write** - If sub-agent is writing a file when killed, file could be corrupted. **FIXED: Added warning and recovery commands**

- [x] **3. hang_count never clears** - Removed `clear-hangs` from success path. Accumulates forever. **FIXED: Clear hangs after successful iteration**

- [x] **4. `|| true` swallows exit** - After `check_for_hang`, the `|| true` might prevent proper exit on timeout. **NOT A BUG: check_for_hang calls exit directly, || true handles other errors**

- [x] **5. Crates might be empty** - If QA agent forgets to set crates, impl runs zero tests and thinks it passed. **FIXED: impl mode requires crates, exits with error if empty**

## Design Issues

- [x] **6. No rollback** - Bad changes can't be easily undone. Git checkout is manual. **FIXED: Save starting SHA, show reset command on timeout**

- [x] **7. Two "stuck" counters** - `stuck_iterations` and `hang_count` track similar things differently. Confusing. **FIXED: Clarified display names - "No-progress" for stuck_iterations (test count), "Timeouts" for hang_count (agent hangs)**

- [x] **8. Review agents have Write tool** - Supposed to be read-only but can write anywhere. **BY DESIGN: Review agents need Write to create qa_review.md/impl_review.md. "Read-only" means no code changes, not no file output.**

- [x] **9. No plan.md validation** - Garbage plan = garbage output. **FIXED: Added validate_plan() in iterate.sh - checks for Objective, Files, Test Cases sections**

- [x] **10. last_test_output.txt overwrites** - Loses history, can't compare iterations. **FIXED: Added test_output/ directory with iteration_N.txt history**

## Operational Issues

- [x] **11. No locking** - Multiple orchestrator instances could corrupt state. **FIXED: Added lockfile in run-orchestrator.sh with stale lock detection**

- [x] **12. Env vars might not exist** - If not launched via run-orchestrator.sh, orchestrator is blind. **BY DESIGN: Must use run-orchestrator.sh to launch orchestrator. Env vars are set there.**

- [x] **13. Permission allowlist incomplete** - Missing commands that orchestrator needs. **FIXED: Expanded allowlist in .claude/settings.json**

- [x] **14. impl_reasoning.md might not exist** - Told to read file that may not exist. **FIXED: impl-review mode now checks for file and exits with helpful error**

- [ ] **15. No cost tracking** - Long sessions burn money invisibly. **DEFERRED: Complex to implement, requires API integration**

- [x] **16. Hooks don't work for sub-agents** - `CLAUDE_TOOL_INPUT` is empty when hooks fire for sub-agent tool calls, so hooks can't inspect or block commands. Confirmed on Claude Code 2.1.12 and 2.1.32. **FIXED: PATH shims in `$ARC_HOME/bin/` intercept cargo/bats at the OS level. Background cargo killer runs as fallback. See ARCHITECTURE.md "Sub-Agent Enforcement".**

- [x] **17. `claude -p` hangs when nested inside Claude Code** - The `-p` flag (positional prompt) causes `claude --print` to hang indefinitely when spawned from within a running Claude Code session. No output on stdout or stderr, process never exits. **Root cause**: Unknown, likely TTY/resource contention in Claude Code 2.1.32+. **Workaround**: Pipe prompt via stdin (`echo "prompt" | claude --print` or `claude --print < prompt.txt`). All scripts updated to use stdin piping.
