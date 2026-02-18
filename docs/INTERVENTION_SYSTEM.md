# Intervention System

## Overview

When the orchestrator encounters situations it cannot handle within workflow constraints, it needs escape hatches. The intervention system provides multiple levels of escalation from autonomous recovery to human takeover.

## Design Principles

1. **Fail loudly** -- Never silently continue when stuck
2. **Preserve context** -- Human should understand what happened
3. **Minimize human effort** -- Provide clear options and commands
4. **Audit trail** -- All interventions are logged

## Intervention Levels

```
Level 1: Autonomous Recovery
    |   (escalation, model switch, retry)
    |
    |   If fails after N attempts
    v
Level 2: Workflow Change Proposal
    |   (orchestrator proposes, human approves)
    |
    |   If proposal rejected or impossible
    v
Level 3: Human Intervention Request
    |   (orchestrator stops, human takes action)
    |
    |   If workflow fundamentally broken
    v
Level 4: Emergency Override
        (bypass workflow constraints)
```

## Level 1: Autonomous Recovery

Handled automatically via workflow `escalation` configuration.

### Triggers
- Stuck iterations (same tests failing repeatedly)
- Test flakiness (inconsistent results)
- Resource exhaustion (timeout, memory)

### Recovery Actions

| Action | When | What It Does |
|--------|------|--------------|
| `analyze_stuck` | stuck >= 3 | Generate analysis of what's been tried |
| `switch_model` | stuck >= 5 | Upgrade to more capable model (opus) |
| `attempt_split` | stuck >= 8 | Try to split phase into smaller pieces |

### Configuration

```yaml
# In workflow state definition
escalation:
  - at_iteration: 3
    action: analyze_stuck
  - at_iteration: 5
    action: switch_model
    params:
      model: opus
  - at_iteration: 8
    action: attempt_split
  - at_iteration: 10
    action: request_intervention
    params:
      message: "Exceeded maximum autonomous recovery attempts"
```

### State Tracking

```json
// state.json during escalation
{
  "current_state": "fix",
  "iteration": 7,
  "stuck_iterations": 5,
  "escalation_history": [
    {
      "iteration": 3,
      "action": "analyze_stuck",
      "result": "Generated analysis"
    },
    {
      "iteration": 5,
      "action": "switch_model",
      "params": {"model": "opus"},
      "result": "Upgraded to opus"
    }
  ]
}
```

## Level 2: Workflow Change Proposal

When the current workflow cannot handle the situation, orchestrator proposes changes.

### When This Happens
- Missing state needed (e.g., need server setup)
- Wrong state order discovered
- Need to add parallel branches
- Constraints too tight or too loose

### Proposal Format

```yaml
# .plans/active/<plan>/workflow_changes.yaml

pending_changes:
  - id: change_001
    proposed_at: "2024-01-15T10:30:00Z"
    proposed_by: orchestrator
    reason: |
      Test requires running gRPC server but workflow has no server setup state.
      Investigation state discovered this dependency.

    change:
      type: add_state
      state:
        name: start_server
        description: Start test gRPC server
        prompt: prompts/common/start-server.md
        params:
          server: my-server
          port: 50051
        after:
          - action: script
            params:
              path: scripts/wait-for-server.sh
        insert_before: regression_tests

    impact:
      - Adds one state to workflow
      - Regression tests will now have server available
      - No existing work invalidated

    status: pending_review
```

### Human Review Commands

```bash
# View pending changes
arc review-workflow-changes my-plan

# Output:
# ===================================================
# WORKFLOW CHANGE PROPOSAL: change_001
# ===================================================
#
# Reason: Test requires running gRPC server...
#
# Change: Add state 'start_server' before 'regression_tests'
#
# Impact:
#   - Adds one state to workflow
#   - Regression tests will now have server available
#
# Commands:
#   arc approve-workflow-change my-plan change_001
#   arc reject-workflow-change my-plan change_001 "reason"
#   arc modify-workflow-change my-plan change_001  # opens editor

# Approve the change
arc approve-workflow-change my-plan change_001

# Reject with reason
arc reject-workflow-change my-plan change_001 "Use mock server instead"
```

### After Approval

```yaml
# workflow_changes.yaml updated
pending_changes:
  - id: change_001
    ...
    status: approved
    approved_at: "2024-01-15T10:35:00Z"
    approved_by: human
```

The change is applied to `workflow.yaml` and orchestrator continues.

## Level 3: Human Intervention Request

When orchestrator cannot proceed and cannot propose a fix.

### Triggers

```yaml
# Workflow-defined triggers
intervention_triggers:
  - condition: "stuck_iterations >= max_iterations"
    action: request_human
    message: "Exceeded {{max_iterations}} iterations without progress"

  - condition: "escalation_exhausted"
    action: request_human
    message: "All autonomous recovery options exhausted"

  - condition: "workflow_insufficient"
    action: request_human
    message: "Current workflow cannot handle this situation"

  - condition: "dispute_unresolvable"
    action: request_human
    message: "Cannot determine if test or implementation is wrong"
```

### Intervention Request Output

```
===================================================================
HUMAN INTERVENTION REQUIRED
===================================================================

Plan: fix-wasm-rng
Phase: port-pcg
State: fix
Iteration: 12/15

REASON:
Exceeded maximum iterations without progress. Tests still failing
after trying multiple approaches including model upgrade.

CONTEXT:
- Failing tests: test_pcg_sequence, test_seed_consistency
- Error: PCG sequence doesn't match reference values
- Tried: Direct port, bit-by-bit comparison, different seeding
- Model: Already upgraded to opus at iteration 5

LAST TEST OUTPUT:
  FAIL test_pcg_sequence
    Expected: [1234567890, ...]
    Got:      [9876543210, ...]

OPTIONS:

1. Fix manually and continue:
   # Make your changes, then:
   arc update-state fix-wasm-rng port-pcg resolve-intervention

2. Modify the workflow:
   # Edit workflow.yaml, then:
   arc update-state fix-wasm-rng port-pcg apply-workflow-change

3. Modify the tests (if test is wrong):
   arc update-state fix-wasm-rng port-pcg approve-dispute "test values incorrect"
   arc iterate fix-wasm-rng port-pcg fix

4. Skip this phase:
   arc update-state fix-wasm-rng port-pcg skip "reason for skipping"

5. Abort the plan:
   arc update-state fix-wasm-rng port-pcg abort "reason for aborting"

===================================================================
```

### State During Intervention

```json
{
  "current_state": "awaiting_human",
  "previous_state": "fix",
  "intervention_request": {
    "reason": "Exceeded maximum iterations without progress",
    "context": {
      "failing_tests": ["test_pcg_sequence", "test_seed_consistency"],
      "attempts": 12,
      "escalations_tried": ["analyze_stuck", "switch_model"]
    },
    "requested_at": "2024-01-15T10:30:00Z",
    "options": ["resolve", "modify_workflow", "approve_dispute", "skip", "abort"]
  }
}
```

### Resolution Commands

```bash
# After fixing manually
arc update-state my-plan phase resolve-intervention

# With notes about what was done
arc update-state my-plan phase resolve-intervention "Fixed endianness issue in PCG implementation"
```

## Level 4: Emergency Override

When the workflow itself is preventing progress and needs to be bypassed.

### Override Flags

| Flag | Effect |
|------|--------|
| `WORKFLOW_OVERRIDE=1` | Ignore all workflow constraints |
| `SKIP_CONSTRAINTS=1` | Skip artifact/iteration checks |
| `ALLOW_TEST_CHANGES=1` | Allow impl to modify tests |
| `IGNORE_VERDICTS=1` | Don't require review verdicts |
| `FORCE_TRANSITION=1` | Allow any state transition |

### Usage

```bash
# Run single iteration with override
ALLOW_TEST_CHANGES=1 arc iterate my-plan phase fix

# Run with full override
WORKFLOW_OVERRIDE=1 arc iterate my-plan phase impl

# Force transition to specific state
FORCE_TRANSITION=1 arc update-state my-plan phase transition complete
```

### Override Logging

All overrides are logged in state.json:

```json
{
  "overrides_used": [
    {
      "timestamp": "2024-01-15T11:00:00Z",
      "override": "ALLOW_TEST_CHANGES",
      "context": "Test had typo that blocked progress",
      "iteration": 8,
      "by": "human"
    },
    {
      "timestamp": "2024-01-15T11:30:00Z",
      "override": "FORCE_TRANSITION",
      "context": "Manually verified tests pass, forcing completion",
      "from_state": "fix",
      "to_state": "complete",
      "by": "human"
    }
  ]
}
```

### Override Warnings

```bash
$ WORKFLOW_OVERRIDE=1 arc iterate my-plan phase impl

WARNING: WORKFLOW_OVERRIDE active
WARNING: All workflow constraints are DISABLED
WARNING: This override will be logged

Continue? [y/N]
```

## Context Injection for Sub-Agents

When orchestrator has useful context, it can inject it into sub-agent prompts.

### orchestrator_notes.md

```bash
# Orchestrator writes notes before spawning sub-agent
write_orchestrator_notes() {
    cat > "$PHASE_DIR/orchestrator_notes.md" << 'EOF'
## Context from Orchestrator

I noticed from previous iterations:

1. The PCG implementation uses a specific increment formula
   - See `rng.rs:15-20` for the exact calculation
   - The increment must be odd

2. WASM has different integer overflow behavior
   - Use `wrapping_mul` and `wrapping_add` explicitly
   - Don't rely on implicit wrapping

3. The test reference values were generated with seed=12345
   - Make sure you're using the same seed
   - Check endianness if values are close but wrong

Focus on: Getting the basic algorithm right first, then worry about optimization.
EOF
}
```

### Prompt Template Integration

```markdown
{{#if orchestrator_notes}}
## Notes from Orchestrator

{{orchestrator_notes}}

---
{{/if}}

## Your Task
...
```

### Automatic Context Injection

Some context is automatically injected based on state:

```yaml
# In workflow state definition
context_sources:
  - source: orchestrator_notes
    file: "{{phase_dir}}/orchestrator_notes.md"
    optional: true

  - source: escalation_active
    condition: "{{state.stuck_iterations}} >= 3"

  - source: recent_test_failures
    file: "{{phase_dir}}/last_test_output.txt"
    extract: "FAIL|FAILED|error\\["
    limit: 20

  - source: previous_attempts
    file: "{{phase_dir}}/impl_reasoning.md"
    parse: attempts_list
```

## Audit Trail

All interventions create an audit trail:

```
.plans/active/<plan>/
+-- audit_log.jsonl              # Append-only log
+-- workflow_changes.yaml        # Change proposals and resolutions
+-- phases/<phase>/
    +-- intervention_history.md  # Human-readable history
    +-- state.json               # Current state with override log
```

### audit_log.jsonl

```json
{"timestamp": "2024-01-15T10:00:00Z", "event": "escalation", "level": 1, "action": "analyze_stuck", "iteration": 3}
{"timestamp": "2024-01-15T10:15:00Z", "event": "escalation", "level": 1, "action": "switch_model", "params": {"model": "opus"}, "iteration": 5}
{"timestamp": "2024-01-15T10:30:00Z", "event": "intervention_requested", "level": 3, "reason": "max iterations exceeded"}
{"timestamp": "2024-01-15T10:45:00Z", "event": "intervention_resolved", "resolution": "manual fix", "by": "human"}
{"timestamp": "2024-01-15T11:00:00Z", "event": "override_used", "level": 4, "override": "ALLOW_TEST_CHANGES", "by": "human"}
```

## Recovery Procedures

### Stuck Phase Recovery

```bash
# 1. Check current state
arc get-state my-plan phase

# 2. Review what's been tried
cat .plans/active/my-plan/phases/phase/impl_reasoning.md

# 3. Options:
#    a) Provide guidance and continue
arc write-orchestrator-notes my-plan phase "Try X instead of Y"
arc iterate my-plan phase impl

#    b) Fix the tests
arc update-state my-plan phase approve-dispute "test expectation wrong"
arc iterate my-plan phase fix

#    c) Fix manually
# Make changes
arc update-state my-plan phase resolve-intervention

#    d) Split the phase
arc split-phase my-plan phase

#    e) Skip
arc update-state my-plan phase skip "blocked by external dependency"
```

### Corrupted State Recovery

```bash
# Reset to last known good state
arc reset-phase my-plan phase --to-iteration 5

# Or reset completely
arc reset-phase my-plan phase --full

# Recover from git
git checkout -- .plans/active/my-plan/phases/phase/state.json
```

### Workflow Recovery

```bash
# Validate current workflow
arc validate-workflow my-plan

# Reset to base workflow
arc reset-workflow my-plan --to-base bugfix

# Manually edit and revalidate
vim .plans/active/my-plan/workflow.yaml
arc validate-workflow my-plan
```
