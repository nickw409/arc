# State Schema

## Overview

Each phase has a `state.json` file that tracks execution progress. The gate orchestrator reads and writes this file during phase execution.

## `state.json` — Active Fields

These fields are actively written and read by the gate orchestration system:

```json
{
  "schema_version": 0,
  "plan": "my-plan",
  "phase": "impl",
  "workflow_type": "feature",

  "phase_status": "in-progress",

  "iteration": {"current": 2, "max": 25},

  "tests_passing": 8,
  "tests_total": 12,

  "blocked": {
    "is_blocked": false,
    "reason": null
  },

  "packages": ["./internal/auth/..."],
  "test_files": ["internal/auth/handler_test.go"],

  "last_commit": "abc1234",
  "notes": "Human-set notes for this phase",
  "deferred_reason": "",
  "deferred_at": "",
  "parent_phase": "",
  "split_into": [],
  "completed_at": "",
  "blocked_reason": "",
  "blocked_at": "",

  "usage": {
    "input_tokens": 12000,
    "output_tokens": 3400,
    "cost_usd": 0.42
  },

  "activity": "Running gate assertions",
  "activity_updated_at": "2025-01-15T10:30:00Z",

  "agent_pid": 12345,

  "adversary_round": 0,
  "adversary_tests": {}
}
```

### `phase_status` values

| Value | Meaning |
|-------|---------|
| `pending` | Not yet started |
| `in-progress` | Agent session running or between retries |
| `complete` | Gate passed, phase committed |
| `blocked` | Exhausted retries or requires human intervention |
| `deferred` | Intentionally skipped |
| `split` | Replaced by sub-phases |

### `iteration` object

| Field | Meaning |
|-------|---------|
| `current` | Number of agent sessions run so far |
| `max` | Cap before phase is considered stuck (default: 25) |

## `plan.json` — Plan Metadata

```json
{
  "name": "my-plan",
  "created": "2025-01-15T09:00:00Z",
  "status": "active",
  "workflow_type": "feature",
  "phases": ["impl", "qa"],
  "phase_order": {"impl": 1, "qa": 2},
  "dependencies": {
    "qa": ["impl"]
  },
  "review_status": "approved",
  "reviewed_at": "2025-01-15T09:05:00Z",
  "review_iterations": 2,
  "review_results": {
    "executability": "pass",
    "consistency": "pass"
  },
  "adversary_bugs": {}
}
```

### `review_status` values

| Value | Meaning |
|-------|---------|
| `unreviewed` | `arc review` has not been run |
| `approved` | All adversaries passed |
| `conditional` | Review passed with notes |
| `failed` | Review failed, plan needs work |

## `spec.yaml` — Phase Specification

`spec.yaml` is the primary input to the gate orchestrator. It defines what the phase should do and how to verify it's done.

```yaml
name: impl
role: impl                    # impl | review | investigate | audit
spec: |
  Add JWT authentication to the user API.
  The handler should validate tokens and return 401 on failure.
description: Implement JWT auth
complexity: medium            # simple | medium | complex
files:
  - internal/auth/handler.go
  - internal/auth/middleware.go
checkpoints:
  - name: Handler exists
    description: JWT handler is implemented
    test: go test ./internal/auth/ -run TestAuthHandler
  - name: Middleware wired
    description: Auth middleware applied to protected routes
gate:
  assertions:
    - type: file_exists
      path: internal/auth/handler.go
    - type: test_exists
      name: TestAuthHandler
    - type: build_passes
      command: go build ./...
  verifier_agent: false       # set true for review/investigate roles
```

## `gate-status.json` — Last Gate Result

Written after each gate run. Not used as input to the orchestrator — informational only for `arc status` and monitoring.

```json
{
  "passed": false,
  "run_count": 2,
  "assertions": [
    {"type": "file_exists", "path": "internal/auth/handler.go", "passed": true},
    {"type": "test_exists", "name": "TestAuthHandler", "passed": false, "message": "not found in *_test.go files"}
  ],
  "timestamp": "2025-01-15T10:31:00Z"
}
```

## Legacy / Unused Fields in `state.json`

The following fields exist in the `PhaseState` struct for backwards compatibility but are **not written or read by the current gate system**:

| Field | Was used for |
|-------|-------------|
| `current_state` | State machine position (which YAML state the phase was in) |
| `state_iterations` | Per-state visit counts |
| `stuck_iterations` | State machine retry counter |
| `hang_count` | Detected agent hangs |
| `executed_escalations` | List of escalation actions tried |
| `rollback_count` | Git rollback counter |
| `global_iterations` | Cross-state iteration total |
| `verdicts_history` | Agent verdict sequence |
| `last_verdict` | Last verdict token emitted |
| `parallel_execution` | Parallel state machine branch tracking |
| `intervention_request` | Escalation to human |
| `chunks` | Work chunking for large phases |
| `disputes` | Test dispute records |
| `last_cleared_disputes` | Resolved disputes |
| `last_reviewed_iteration` | State machine review sync |
| `last_qa_reviewed_iteration` | State machine QA sync |
