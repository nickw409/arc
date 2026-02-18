# State Schema

## Overview

Each phase has a `state.json` file that tracks execution progress. The schema evolves with workflow versions but maintains backwards compatibility.

## Terminology

| Term | Meaning | Example |
|------|---------|---------|
| **current_state** | Current position in the workflow state machine | `"impl"`, `"qa_review"` |
| **phase** | A portion of a plan being executed | `"port-pcg-algorithm"` |
| **phase_status** | **DEPRECATED** - Old term, use `current_state` | Migration maps this to `current_state` |

**Note:** The old schema used `phase_status` to track progress. The new schema uses `current_state` because:
1. "State" aligns with workflow state machine terminology
2. Distinguishes workflow state from phase identity
3. Avoids confusion with plan-level status

## Current Schema (V4 Target)

```json
{
  "plan": "plan-name",
  "phase": "phase-name",
  "workflow_type": "bugfix",
  "workflow_version": 4,

  "current_state": "fix",
  "previous_state": "test_review",
  "entry_time": "2024-01-15T10:00:00Z",

  "iteration": 5,
  "stuck_iterations": 2,
  "max_iterations": 15,

  "tests_passing": 8,
  "tests_total": 12,
  "last_test_run": "2024-01-15T10:30:00Z",

  "last_verdict": "some_failing",
  "verdicts_history": [
    {"iteration": 3, "state": "test_review", "verdict": "gaps_found"},
    {"iteration": 4, "state": "test_review", "verdict": "approved"},
    {"iteration": 5, "state": "fix", "verdict": "some_failing"}
  ],

  "packages": ["my-package"],

  "artifacts": {
    "findings.md": {"exists": true, "updated": "2024-01-15T09:30:00Z"},
    "qa_reasoning.md": {"exists": true, "updated": "2024-01-15T10:00:00Z"},
    "impl_reasoning.md": {"exists": true, "updated": "2024-01-15T10:30:00Z"}
  },

  "disputes": [
    {
      "id": "dispute_001",
      "test_name": "test_pcg_sequence",
      "reason": "Expected values don't match PCG specification",
      "filed_at": "2024-01-15T10:15:00Z",
      "status": "approved",
      "resolution": "Test values were from wrong RNG",
      "resolved_at": "2024-01-15T10:20:00Z"
    }
  ],
  "last_cleared_disputes": [],

  "escalation_history": [
    {"iteration": 3, "action": "analyze_stuck", "result": "Generated analysis"},
    {"iteration": 5, "action": "switch_model", "params": {"model": "opus"}}
  ],
  "current_model": "opus",

  "intervention_request": null,

  "overrides_used": [],

  "hang_count": 0,
  "last_hang": null,

  "transitions": [
    {"from": "pending", "to": "qa", "at": "2024-01-15T09:00:00Z"},
    {"from": "qa", "to": "qa_review", "at": "2024-01-15T09:30:00Z"},
    {"from": "qa_review", "to": "impl", "at": "2024-01-15T09:45:00Z", "verdict": "approved"},
    {"from": "impl", "to": "fix", "at": "2024-01-15T10:00:00Z"}
  ],

  "created_at": "2024-01-15T09:00:00Z",
  "updated_at": "2024-01-15T10:30:00Z"
}
```

## Field Definitions

### Identity Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `plan` | string | Yes | Plan name |
| `phase` | string | Yes | Phase name |
| `workflow_type` | string | Yes | Workflow type (feature, bugfix, etc.) |
| `workflow_version` | number | Yes | Workflow schema version |

### State Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `current_state` | string | Yes | Current workflow state |
| `previous_state` | string | No | Previous state (for back-tracking) |
| `entry_time` | ISO8601 | No | When current state was entered |

### Iteration Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `iteration` | number | Yes | Current iteration count |
| `stuck_iterations` | number | Yes | Consecutive iterations without progress |
| `max_iterations` | number | Yes | Maximum allowed iterations |

### Test Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `tests_passing` | number | No | Number of passing tests |
| `tests_total` | number | No | Total number of tests |
| `last_test_run` | ISO8601 | No | When tests were last run |
| `test_files` | array | No | Paths to test files for this phase (relative to project root) |

#### Test File Discovery

iterate.sh discovers test files using this priority order:

1. **Explicit:** Read `test_files[]` from state.json if present
2. **Manifest:** Check for `<phase_dir>/tests.txt` (one path per line)

If neither is configured, no tests will run. Always register test files explicitly.

**Registering test files:**

```bash
# Via update-state.sh (adds to array, doesn't replace)
update-state.sh my-plan phase add-test-file "$ARC_HOME/tests/my_feature.bats"

# Clear all registered test files
update-state.sh my-plan phase clear-test-files

# Or QA agent can edit state.json directly:
jq '.test_files = ["$ARC_HOME/tests/my_feature.bats"]' state.json > tmp && mv tmp state.json
```

**Benefits of explicit test file registration:**
- Functional naming: Tests named by what they test, not phase identity
- Multiple files: A phase can have multiple test files
- Cross-phase reuse: Same test file can be referenced by multiple phases

### Verdict Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `last_verdict` | string | No | Most recent verdict |
| `verdicts_history` | array | No | History of all verdicts |

### Crates

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `crates` | array | No | List of affected crate names |

### Artifacts

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `artifacts` | object | No | Map of artifact paths to metadata |

### Disputes

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `disputes` | array | No | Active and resolved disputes |
| `last_cleared_disputes` | array | No | Recently cleared (for context injection) |

### Escalation

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `escalation_history` | array | No | Record of escalation actions |
| `current_model` | string | No | Current AI model (after switch) |

### Intervention

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `intervention_request` | object | No | Pending human intervention request |

### Overrides

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `overrides_used` | array | No | Record of override flags used |

### Hang Detection

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `hang_count` | number | No | Number of timeouts |
| `last_hang` | ISO8601 | No | When last hang occurred |

### Transitions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `transitions` | array | No | History of all state transitions (see Transition Entry schema) |

### Timestamps

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `created_at` | ISO8601 | Yes | When state file was created |
| `updated_at` | ISO8601 | Yes | Last modification time |

## Nested Object Schemas

### Verdict History Entry

```json
{
  "iteration": 3,
  "state": "test_review",
  "verdict": "gaps_found",
  "timestamp": "2024-01-15T10:00:00Z"
}
```

### Artifact Entry

```json
{
  "exists": true,
  "updated": "2024-01-15T10:00:00Z",
  "size": 1234
}
```

### Dispute Entry

```json
{
  "id": "dispute_001",
  "test_name": "test_name",
  "reason": "Why this is disputed",
  "filed_at": "2024-01-15T10:00:00Z",
  "status": "pending|approved|rejected",
  "resolution": "How it was resolved",
  "resolved_at": "2024-01-15T10:15:00Z",
  "resolved_by": "orchestrator|human"
}
```

### Escalation Entry

```json
{
  "iteration": 5,
  "action": "switch_model",
  "params": {"model": "opus"},
  "result": "Upgraded to opus",
  "timestamp": "2024-01-15T10:00:00Z"
}
```

### Intervention Request

```json
{
  "reason": "Max iterations exceeded",
  "context": {
    "failing_tests": ["test_a", "test_b"],
    "attempts": 15,
    "escalations_tried": ["analyze_stuck", "switch_model"]
  },
  "requested_at": "2024-01-15T10:00:00Z",
  "options": ["resolve", "modify_workflow", "skip", "abort"]
}
```

### Override Entry

```json
{
  "timestamp": "2024-01-15T10:00:00Z",
  "override": "ALLOW_TEST_CHANGES",
  "context": "Test had typo",
  "iteration": 8,
  "by": "human"
}
```

### Transition Entry

```json
{
  "from": "test_review",
  "to": "fix",
  "at": "2024-01-15T10:00:00Z",
  "verdict": "approved",
  "iteration": 5
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from` | string | Yes | Source state |
| `to` | string | Yes | Destination state |
| `at` | ISO8601 | Yes | When transition occurred |
| `verdict` | string | No | Verdict that triggered transition (if review state) |
| `iteration` | number | No | Iteration when transition occurred |

## State Transitions

### Valid Transitions

Transitions are defined by the workflow. The state.json tracks what happened but doesn't enforce rules — that's the workflow's job.

### Transition Logging

Every state change is recorded:

```json
{
  "transitions": [
    {"from": "pending", "to": "investigate", "at": "2024-01-15T09:00:00Z"},
    {"from": "investigate", "to": "regression_tests", "at": "2024-01-15T09:30:00Z"},
    {"from": "regression_tests", "to": "test_review", "at": "2024-01-15T10:00:00Z"},
    {"from": "test_review", "to": "regression_tests", "at": "2024-01-15T10:05:00Z", "verdict": "gaps_found"},
    {"from": "regression_tests", "to": "test_review", "at": "2024-01-15T10:15:00Z"},
    {"from": "test_review", "to": "fix", "at": "2024-01-15T10:20:00Z", "verdict": "approved"}
  ]
}
```

## Migration from Old Schema

### Old Schema (pre-V1)

```json
{
  "phase_status": "implementing",
  "iteration": {"current": 5, "max": 25},
  "tests_passing": 8,
  "tests_total": 12,
  "packages": ["my-package"],
  "disputes": [],
  "stuck_iterations": 0
}
```

### Migration Rules

```javascript
function migrate(old) {
  return {
    plan: old.plan || detectFromPath(),
    phase: old.phase || detectFromPath(),
    workflow_type: "feature",  // Default
    workflow_version: 1,

    current_state: mapOldStatus(old.phase_status),

    iteration: old.iteration?.current || old.iteration || 0,
    stuck_iterations: old.stuck_iterations || 0,
    max_iterations: old.iteration?.max || 25,

    tests_passing: old.tests_passing || 0,
    tests_total: old.tests_total || 0,

    packages: old.packages || [],
    disputes: old.disputes || [],

    created_at: old.created_at || new Date().toISOString(),
    updated_at: new Date().toISOString()
  };
}

function mapOldStatus(status) {
  const map = {
    "pending": "pending",
    "qa": "qa",
    "qa_review": "qa_review",
    "implementing": "impl",
    "impl_review": "impl_review",
    "complete": "complete",
    "disputed": "fix",  // Map to fix state
    "blocked": "blocked"
  };
  return map[status] || status;
}
```

## Scripts for State Management

### Reading State

```bash
# get-state.sh - Human readable output
get-state.sh my-plan phase

# Output:
# Plan: my-plan
# Phase: phase
# State: fix (iteration 5/15)
# Tests: 8/12 passing
# Stuck: 2 iterations
# Model: opus (escalated)
```

### Updating State

```bash
# update-state.sh - Atomic updates

# Set current state
update-state.sh my-plan phase state fix

# Update test counts
update-state.sh my-plan phase tests 10 12

# Record verdict
update-state.sh my-plan phase verdict approved

# Increment iteration
update-state.sh my-plan phase increment-iteration

# File dispute
update-state.sh my-plan phase dispute "test_name" "reason"

# Clear dispute
update-state.sh my-plan phase clear-dispute

# Request intervention
update-state.sh my-plan phase request-intervention "reason"

# Resolve intervention
update-state.sh my-plan phase resolve-intervention "what was done"
```

## Validation

State files are validated on read:

```bash
validate_state() {
    local state_file="$1"

    # Required fields
    jq -e '.plan and .phase and .current_state' "$state_file" || return 1

    # Type checks
    jq -e '.iteration | type == "number"' "$state_file" || return 1
    jq -e '.stuck_iterations | type == "number"' "$state_file" || return 1

    # Range checks
    jq -e '.iteration >= 0' "$state_file" || return 1
    jq -e '.stuck_iterations >= 0' "$state_file" || return 1

    return 0
}
```
