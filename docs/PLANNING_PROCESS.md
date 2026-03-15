# Planning Process

## Overview

A plan is a directory on disk that defines a set of phases and their relationships. Planning produces the files the gate orchestrator needs to run: `plan.json`, and per-phase `spec.yaml`, `plan.md`, and `state.json`.

## Manual Planning (`arc plan`)

```bash
arc plan my-feature impl qa
arc plan my-bugfix investigate fix --type bugfix
arc plan my-research analyze document --role investigate
```

This creates:
```
.plans/active/my-feature/
  plan.json                  — PlanMeta (phases, deps, review status)
  session_id                 — UUID for this planning session
  phases/
    impl/
      spec.yaml              — PhaseSpec (role, gate assertions, checkpoints)
      plan.md                — Human-written context (from template; you edit this)
      state.json             — Initial state (phase_status: pending)
    qa/
      spec.yaml
      plan.md
      state.json
```

After creating the plan, you fill in each `spec.yaml` with:
- `spec`: what the phase should accomplish
- `gate.assertions`: what "done" looks like (file_exists, test_exists, build_passes, ...)
- `checkpoints`: named milestones with test commands

## Automated Planning (`arc dispatch`)

`arc dispatch` automates the full lifecycle from a task description:

```
arc dispatch "Add JWT authentication to the user API"
```

### Pipeline

```
1. Discovery agent
   → Reads codebase (glob, grep, read files)
   → Outputs JSON: complexity (simple/medium/complex), suggested phases, approach

2. ValidateComplexity
   → Heuristic overrides if agent under/over-estimated
   → Rules: >5 phases → complex, >10 files → complex, etc.

3. Clarification loop (skipped with --yes)
   → Interactive Q&A to resolve ambiguity

4. GeneratePlanName
   → Strips stop words, takes first 4 words, adds -N suffix on conflicts

5. GeneratePlan (branches by complexity)
   simple   → 1-phase direct plan with execute phase
   medium   → N-phase plan with phase-level descriptions from discovery
   complex  → 3 parallel architect agents (minimal/clean/pragmatic approaches)
              → SelectProposal (prefers "pragmatic")
              → GeneratePlan with architect's phase content

6. arc review (optional, skip with --skip-review)
   → 4 adversary agents review the plan.md files
   → Synthesizer rewrites plan.md based on adversary critiques
   → Loop until all pass or max iterations

7. orchestrator.Launch
   → Runs the plan end-to-end

8. RunCodeReview (post-execution)
   → Reviews the git diff for quality issues
```

## Adversarial Plan Review (`arc review`)

Plan review runs before execution to catch problems in `plan.md` files before an agent wastes turns on a flawed plan.

```bash
arc review my-plan           # review all phases
arc review my-plan --phase impl  # review one phase
```

### How it works

Four adversary agents examine the plan, each from a different angle:

| Adversary | Question |
|-----------|----------|
| `scope` | Is this phase small enough to execute reliably? (pre-check, runs first) |
| `spec-quality` | Is the spec concrete, unambiguous, and actionable? |
| `correctness` | Are types, names, and contracts consistent and correct? |
| `gate` | Do gate assertions cover every integration point? |

If any adversary fails, a synthesizer agent rewrites `plan.md` based on the critique files. The loop repeats until all adversaries pass or max iterations are reached.

Results are stored in `plan.json` as `review_status` and `review_results`.

## Phase Dependencies

By default, all phases are independent and run in parallel. Add dependencies to sequence them:

```bash
arc plan update-deps my-plan qa --deps impl

# Result in plan.json:
"dependencies": {"qa": ["impl"]}
```

The orchestrator reads `plan.json` at startup and schedules phases accordingly:
- Phases with no unmet dependencies → start immediately
- Phases with dependencies → wait for deps to reach `complete`

## Gate Configuration

The most important part of a plan is the gate assertions in `spec.yaml`. Gates define what "done" means — phases complete when gates pass, not when agents say they're done.

### Writing good gate assertions

**Too weak** (easy to fake):
```yaml
gate:
  assertions:
    - type: file_exists
      path: internal/auth/handler.go
```

**Better** (verifies the actual work):
```yaml
gate:
  assertions:
    - type: file_exists
      path: internal/auth/handler.go
    - type: test_exists
      name: TestJWTValidation
    - type: build_passes
      command: go test ./internal/auth/ -run TestJWT
    - type: grep
      pattern: "func.*JWTMiddleware"
```

For review/investigate phases, use `verifier_agent: true` instead of assertions — a second agent verifies the findings.

## After the Plan Runs

On completion, the orchestrator generates:
- `COMPLETION_REPORT.md` — per-phase results, test counts, costs
- `SUMMARY.md` — high-level summary with git diff stats
