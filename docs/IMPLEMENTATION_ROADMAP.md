# Implementation Roadmap

## Overview

Implementation proceeds in versions, each building on the previous. All versions are backwards compatible -- V1 workflows run unchanged on V4 engine.

## Prerequisites

### Required Tools

| Tool | Version | Purpose | Installation |
|------|---------|---------|--------------|
| `yq` | v4.35+ (Go version) | YAML parsing | `brew install yq` or `go install github.com/mikefarah/yq/v4@latest` |
| `jq` | 1.6+ | JSON processing | `brew install jq` or `apt install jq` |
| `python3` | 3.8+ | Template engine (V3+) | Usually pre-installed |

**Important:** The `yq` tool must be the **Go version** by Mike Farah (mikefarah/yq), NOT the Python `yq` wrapper. The syntax is incompatible.

### Verification Script

```bash
#!/usr/bin/env bash
# $ARC_SCRIPTS_DIR/check-prerequisites.sh

set -euo pipefail

check_command() {
    if ! command -v "$1" &> /dev/null; then
        echo "FAIL $1 not found"
        return 1
    fi
    echo "PASS $1 found: $(command -v "$1")"
}

check_yq_version() {
    local version=$(yq --version 2>&1)
    if [[ "$version" != *"mikefarah"* ]] && [[ "$version" != *"version v4"* ]]; then
        echo "FAIL Wrong yq version. Need mikefarah/yq v4+, got: $version"
        return 1
    fi
    echo "PASS yq version OK: $version"
}

check_python_version() {
    local version=$(python3 --version 2>&1 | grep -oP '\d+\.\d+')
    local major=$(echo "$version" | cut -d. -f1)
    local minor=$(echo "$version" | cut -d. -f2)
    if [[ $major -lt 3 ]] || [[ $major -eq 3 && $minor -lt 8 ]]; then
        echo "FAIL Python 3.8+ required, got: $version"
        return 1
    fi
    echo "PASS python3 version OK: $version"
}

echo "Checking prerequisites..."
failed=0

check_command yq && check_yq_version || failed=1
check_command jq || failed=1
check_command python3 && check_python_version || failed=1

echo ""
if [[ $failed -eq 0 ]]; then
    echo "All prerequisites satisfied."
else
    echo "Some prerequisites missing. Install them and try again."
    exit 1
fi
```

---

## Agent Spawning Mechanism

The orchestration system spawns Claude agents using Claude Code CLI. This is the fundamental mechanism.

### CLI Invocation

```bash
# Spawn a sub-agent with a prompt file
spawn_agent() {
    local prompt_file="$1"
    local output_file="$2"
    local timeout="${3:-600}"  # Default 10 minutes

    # Claude Code CLI invocation
    timeout "${timeout}s" claude \
        --print \
        --output-format text \
        --max-turns 50 \
        < "$prompt_file" \
        > "$output_file" 2>&1

    local exit_code=$?

    if [[ $exit_code -eq 124 ]]; then
        echo "TIMEOUT" >> "$output_file"
        return 124
    fi

    return $exit_code
}
```

### Agent Output Capture

All agent output goes to files in the phase directory:

```
.plans/active/<plan>/phases/<phase>/
+-- state.json              # Current state
+-- iteration_001/          # Per-iteration outputs
|   +-- prompt.md           # Rendered prompt sent to agent
|   +-- output.txt          # Raw agent output
|   +-- verdict.txt         # Extracted verdict (if review)
|   +-- artifacts/          # Files created by agent
+-- iteration_002/
|   +-- ...
+-- orchestrator_notes.md   # Context from orchestrator
```

### Timeout Handling

| State Type | Default Timeout | Configurable |
|------------|-----------------|--------------|
| Implementation | 600s (10 min) | Yes, via `timeout` in workflow |
| Review | 300s (5 min) | Yes |
| Investigation | 900s (15 min) | Yes |

When timeout occurs:
1. Agent process is killed
2. `TIMEOUT` written to output file
3. State transitions to stuck handling
4. Iteration counter increments

### Prompt Rendering Pipeline

```bash
render_and_spawn() {
    local state_name="$1"
    local iteration="$2"

    # 1. Get prompt template path from workflow
    local template=$(yq ".states[] | select(.name == \"$state_name\") | .prompt" "$WORKFLOW_FILE")

    # 2. Build context JSON
    local context=$(build_context "$state_name" "$iteration")

    # 3. Render template (V1: simple substitution, V3: full Handlebars)
    local rendered=$(render_template "$ARC_HOME/$template" "$context")

    # 4. Save rendered prompt
    local iter_dir="$PHASE_DIR/iteration_$(printf '%03d' $iteration)"
    mkdir -p "$iter_dir"
    echo "$rendered" > "$iter_dir/prompt.md"

    # 5. Spawn agent
    spawn_agent "$iter_dir/prompt.md" "$iter_dir/output.txt"
}
```

---

## Bootstrap Strategy

**Problem:** To implement V1, we need the orchestration system. But V1 IS the orchestration system.

**Solution:** Manual bootstrap with incremental automation.

### Phase 0: Manual Foundation (No Orchestration)

Implement these files manually without any orchestration:

1. **check-prerequisites.sh** -- Verify yq, jq, python3
2. **validate-workflow.sh** -- Basic YAML syntax check only
3. **feature.yaml** -- V1 workflow definition
4. **One prompt file** -- `prompts/feature/impl.md` (most used)

Test manually:
```bash
./check-prerequisites.sh
./validate-workflow.sh workflows/feature.yaml
```

### Phase 1: Self-Hosting (Use Partial System)

Once Phase 0 works, use the minimal system to build the rest:

```bash
# Create a plan for building V1
arc init build-orchestration-v1 \
    --type feature \
    --phases "workflow-files,prompt-extraction,script-refactor"
```

Now the orchestration system can help build itself.

### Bootstrap Dependency Graph

```
Manual Work (no automation):
    check-prerequisites.sh
    validate-workflow.sh (basic)
            |
            v
Self-Hosted V1a (linear workflow only):
    feature.yaml
    iterate.sh (simplified)
    get-next-state.sh
            |
            v
Self-Hosted V1b (prompts + templates):
    All prompt files
    Plan templates
    Full iterate.sh
            |
            v
Full V1 (complete):
    All 5 workflow types
    Full validation
    Migration logic
```

---

## Error Handling

All scripts must use strict error handling and provide clear diagnostics.

### Bash Script Template

```bash
#!/usr/bin/env bash
# Script: <name>.sh
# Purpose: <description>

set -euo pipefail

# Constants
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARC_HOME="$(dirname "$SCRIPT_DIR")"

# Error handling
error() {
    echo "ERROR: $*" >&2
    exit 1
}

warn() {
    echo "WARNING: $*" >&2
}

# Validate required commands
require_command() {
    command -v "$1" &> /dev/null || error "$1 is required but not installed"
}

require_command yq
require_command jq

# Validate arguments
[[ $# -ge 1 ]] || error "Usage: $0 <plan-name> [phase]"
```

### Error Categories

| Category | Example | Handling |
|----------|---------|----------|
| **Prerequisite** | yq not installed | Exit with installation instructions |
| **Validation** | Invalid YAML | Exit with line number and error |
| **Runtime** | Agent timeout | Log, increment stuck_iterations, retry |
| **State** | Corrupted state.json | Attempt recovery, request human if fails |

### YAML Parsing Errors

```bash
parse_workflow() {
    local workflow_file="$1"

    # Validate file exists
    [[ -f "$workflow_file" ]] || error "Workflow file not found: $workflow_file"

    # Validate YAML syntax
    if ! yq '.' "$workflow_file" > /dev/null 2>&1; then
        local line=$(yq '.' "$workflow_file" 2>&1 | grep -oP 'line \d+' | head -1)
        error "Invalid YAML in $workflow_file at $line"
    fi

    # Validate required fields
    local name=$(yq '.name // ""' "$workflow_file")
    [[ -n "$name" ]] || error "Workflow missing required field: name"

    local entry=$(yq '.entry_state // ""' "$workflow_file")
    [[ -n "$entry" ]] || error "Workflow missing required field: entry_state"
}
```

### Agent Failure Handling

```bash
handle_agent_result() {
    local exit_code="$1"
    local output_file="$2"
    local state_name="$3"

    case $exit_code in
        0)
            # Success - extract verdict if review state
            if is_review_state "$state_name"; then
                extract_verdict "$output_file"
            fi
            ;;
        124)
            # Timeout
            warn "Agent timed out in state $state_name"
            increment_stuck_iterations
            ;;
        *)
            # Other failure
            error "Agent failed with exit code $exit_code. Output: $(tail -20 "$output_file")"
            ;;
    esac
}
```

---

## Stuck Detection Algorithm

"Stuck" means the phase is not making progress despite iterations.

### Definition

A phase is **stuck** when:
1. Same tests are failing for 2+ consecutive iterations, AND
2. The failure signatures are substantially similar (>80% match)

A phase is **NOT stuck** when:
- Different tests fail each iteration (progress, even if negative)
- Same tests fail but error messages change significantly
- Tests pass but review rejects (that's a review loop, not stuck)

### Algorithm

```bash
detect_stuck() {
    local phase_dir="$1"
    local current_iteration="$2"

    # Need at least 2 iterations to compare
    [[ $current_iteration -lt 2 ]] && echo "false" && return

    local prev_iter=$((current_iteration - 1))
    local curr_failures="$phase_dir/iteration_$(printf '%03d' $current_iteration)/test_failures.txt"
    local prev_failures="$phase_dir/iteration_$(printf '%03d' $prev_iter)/test_failures.txt"

    # Extract failing test names
    local curr_tests=$(grep -oP 'FAIL \K\S+' "$curr_failures" 2>/dev/null | sort)
    local prev_tests=$(grep -oP 'FAIL \K\S+' "$prev_failures" 2>/dev/null | sort)

    # Compare test sets
    if [[ "$curr_tests" == "$prev_tests" ]]; then
        # Same tests failing - check error similarity (returns percentage 0-100)
        local similarity=$(compare_error_signatures "$curr_failures" "$prev_failures")
        if [[ $similarity -gt 80 ]]; then
            echo "true"
            return
        fi
    fi

    echo "false"
}

compare_error_signatures() {
    local file1="$1"
    local file2="$2"

    # Extract error lines, normalize whitespace, compute similarity
    local sig1=$(grep -E 'error|panic|assert' "$file1" 2>/dev/null | tr -s ' ' | sort -u)
    local sig2=$(grep -E 'error|panic|assert' "$file2" 2>/dev/null | tr -s ' ' | sort -u)

    # Jaccard similarity as percentage (0-100) - avoids bc dependency
    local intersection=$(comm -12 <(echo "$sig1") <(echo "$sig2") | wc -l)
    local union=$(sort -u <(echo "$sig1") <(echo "$sig2") | wc -l)

    if [[ $union -eq 0 ]]; then
        echo "0"
    else
        # Integer percentage: (intersection * 100) / union
        echo $(( (intersection * 100) / union ))
    fi
}
```

### Stuck State Updates

```bash
update_stuck_status() {
    local phase_dir="$1"
    local state_file="$phase_dir/state.json"

    if [[ $(detect_stuck "$phase_dir" "$(jq '.iteration' "$state_file")") == "true" ]]; then
        # Increment stuck counter
        jq '.stuck_iterations += 1' "$state_file" > tmp && mv tmp "$state_file"
    else
        # Reset stuck counter (progress was made)
        jq '.stuck_iterations = 0' "$state_file" > tmp && mv tmp "$state_file"
    fi
}
```

---

## Version Summary

| Version | Features | Estimated Effort |
|---------|----------|------------------|
| V1a | Workflow engine, basic validation | 2-3 days |
| V1b | Prompt extraction, templates | 2-3 days |
| V2 | Conditional branches, verdicts | 1-2 days |
| V3 | Parameters, template variables | 2 days |
| V4 | Hooks, escalation, actions | 2-3 days |
| V5 | Parallel states | 3+ days |

Total: ~12-16 days for full implementation

---

## V1a: Workflow Engine Foundation

### Goals
- Implement workflow parsing and validation
- Create base workflow files (feature only initially)
- Implement state transition logic
- Minimal iterate.sh changes to read workflows

### Why Split?
V1 was originally scoped at "2-3 days" but actually requires 5-8 days of work. Splitting into V1a (engine) and V1b (content) allows:
- Earlier validation that the approach works
- Smaller, testable increments
- Self-hosting after V1a (use system to build V1b)

### V1a Deliverables

#### 1. Scripts (V1a Only)

| Script | Purpose | Complexity |
|--------|---------|------------|
| `check-prerequisites.sh` | Verify yq, jq, etc. | Low |
| `validate-workflow.sh` | YAML syntax + basic schema | Medium |
| `get-next-state.sh` | Linear state transitions | Low |
| `iterate.sh` (minimal) | Read workflow, call existing prompts | Medium |

#### 2. Workflow Files (V1a Only)

Only `feature.yaml` initially -- other types in V1b.

#### 3. V1a Tasks

- [ ] Create check-prerequisites.sh
- [ ] Create validate-workflow.sh (syntax + required fields only)
- [ ] Create get-next-state.sh (linear transitions only)
- [ ] Create feature.yaml (V1 schema)
- [ ] Modify iterate.sh to detect and use workflow
- [ ] Test with existing feature plan
- [ ] Verify backwards compatibility (no workflow = old behavior)

### V1a Validation Criteria

V1a is complete when:
1. `./validate-workflow.sh feature.yaml` passes
2. `./iterate.sh <plan> <phase> qa` uses workflow to determine next state
3. Old plans without workflow.yaml still work

---

## V1b: Prompts and Templates

### Goals
- Extract all prompts from iterate.sh to files
- Create remaining workflow files (bugfix, investigation, refactor, performance)
- Create plan templates for all work types
- Implement full workflow validation (graph reachability, prompt existence)

### V1b Deliverables

#### 1. Directory Structure
```
$ARC_HOME/
+-- workflows/
|   +-- feature.yaml      # Base feature workflow
|   +-- bugfix.yaml       # Base bugfix workflow
|   +-- investigation.yaml
|   +-- refactor.yaml
|   +-- performance.yaml
+-- prompts/
|   +-- feature/
|   |   +-- qa.md
|   |   +-- qa-review.md
|   |   +-- impl.md
|   |   +-- impl-review.md
|   |   +-- fix.md
|   +-- bugfix/
|   |   +-- investigate.md
|   |   +-- regression-tests.md
|   |   +-- test-review.md
|   |   +-- fix.md
|   |   +-- fix-review.md
|   +-- investigation/
|   +-- refactor/
|   +-- performance/
|   +-- common/
|   |   +-- test-commands.md
|   |   +-- do-not-rules.md
|   |   +-- reasoning-format.md
|   +-- adversaries/
|       +-- coverage.md
|       +-- ambiguity.md
|       +-- scope.md
|       +-- consistency.md
|       +-- executability.md
+-- templates/
|   +-- plan-feature.md
|   +-- plan-bugfix.md
|   +-- plan-investigation.md
|   +-- plan-refactor.md
|   +-- plan-performance.md
+-- scripts/
    +-- iterate.sh          # Refactored to read workflow
    +-- validate-workflow.sh # New
    +-- get-next-state.sh   # New
    +-- ...
```

#### 2. Scripts to Create/Modify

**Core Scripts (V1) with Complexity Assessment:**

| Script | Status | Complexity | Details |
|--------|--------|------------|---------|
| `iterate.sh` | Modify | **High** | Near-complete rewrite. Currently hardcoded prompts -> must load from files, build context, render templates, extract verdicts. ~300 lines of changes. |
| `validate-workflow.sh` | New | **High** | YAML syntax validation, required field checks, graph reachability (BFS/DFS), cycle detection with escape conditions, prompt file existence, verdict consistency. ~200 lines. |
| `get-next-state.sh` | New | **Low** | Parse workflow YAML, look up current state, return `next` field. ~30 lines. |
| `init-plan.sh` | Modify | **Medium** | Add `--type` flag, copy appropriate base workflow, create directory structure. ~50 lines of changes. |
| `update-state.sh` | Modify | **Medium** | Rename `phase_status` -> `current_state`, add new fields (stuck_iterations, verdicts_history). ~40 lines. |
| `check-prerequisites.sh` | New | **Low** | Check yq, jq, python3 versions. ~40 lines. |

**Complexity Legend:**
- **Low**: <50 lines, straightforward logic, testable in isolation
- **Medium**: 50-150 lines, some branching logic, may need integration testing
- **High**: >150 lines, complex logic (graphs, parsing), needs unit + integration tests

**Adversarial Review Scripts (Parallel Track):**

| Script | Status | Complexity | Purpose |
|--------|--------|------------|---------|
| `plan-review-loop.sh` | New | **High** | Orchestrate adversary committee: spawn 5 agents in parallel, collect results, aggregate failures, invoke plan agent for fixes, iterate until pass or max. ~150 lines. |
| `run-adversary.sh` | New | **Medium** | Spawn single adversary agent, capture output, extract verdict. ~60 lines. |
| `collect-failures.sh` | New | **Medium** | Parse all adversary outputs, extract failure items, deduplicate, format for plan agent. ~80 lines. |
| `approve-plan.sh` | New | **Low** | Update plan status, log approval, support `--override` flag. ~40 lines. |

**Intervention Scripts (V4):**

| Script | Status | Complexity | Purpose |
|--------|--------|------------|---------|
| `review-workflow-changes.sh` | New | **Low** | Display pending changes from workflow_changes.yaml. ~30 lines. |
| `approve-workflow-change.sh` | New | **Medium** | Apply change to workflow.yaml, update status, log. ~50 lines. |
| `reject-workflow-change.sh` | New | **Low** | Update status to rejected, log reason. ~25 lines. |
| `write-orchestrator-notes.sh` | New | **Low** | Write context to orchestrator_notes.md. ~20 lines. |
| `reset-phase.sh` | New | **Medium** | Reset state.json to previous iteration or clean. ~60 lines. |
| `reset-workflow.sh` | New | **Low** | Copy base workflow, clear customizations. ~30 lines. |
| `split-phase.sh` | New | **High** | Analyze stuck phase, propose split, create sub-phase directories. ~120 lines. |

#### 3. Workflow Schema (V1 Subset)

```yaml
name: string              # Required
version: 1                # Required, must be 1
description: string       # Optional

states:
  - name: string          # Required, unique
    description: string   # Optional
    prompt: string        # Required, path to prompt file
    next: string          # Required, next state name (linear only)

entry_state: string       # Required
terminal_states:          # Required
  - string
```

#### 3b. Base Workflow Definitions

**Feature Workflow** (feature.yaml):
```yaml
name: feature
version: 1
description: New capability using TDD

states:
  - name: qa
    description: Write tests based on specification
    prompt: prompts/feature/qa.md
    next: qa_review
  - name: qa_review
    description: Review test coverage
    prompt: prompts/feature/qa-review.md
    next: impl
  - name: impl
    description: Implement to pass tests
    prompt: prompts/feature/impl.md
    next: impl_review
  - name: impl_review
    description: Review implementation
    prompt: prompts/feature/impl-review.md
    next: complete

entry_state: qa
terminal_states: [complete, blocked]
```

**Note:** The `fix` state (for disputed tests) is added in V2 when branching is available:
```yaml
# V2 addition to feature workflow
states:
  - name: qa_review
    verdicts: [approved, needs_fix]
    next:
      approved: impl
      needs_fix: fix
  - name: fix
    description: Fix disputed tests
    prompt: prompts/feature/fix.md
    next: qa_review
```

**Bugfix Workflow** (bugfix.yaml):
```yaml
name: bugfix
version: 1
description: Fix incorrect behavior

states:
  - name: investigate
    description: Understand current behavior and root cause
    prompt: prompts/bugfix/investigate.md
    next: regression_tests
  - name: regression_tests
    description: Write tests defining correct behavior
    prompt: prompts/bugfix/regression-tests.md
    next: test_review
  - name: test_review
    description: Review regression test coverage
    prompt: prompts/bugfix/test-review.md
    next: fix
  - name: fix
    description: Implement the fix
    prompt: prompts/bugfix/fix.md
    next: fix_review
  - name: fix_review
    description: Review fix implementation
    prompt: prompts/bugfix/fix-review.md
    next: complete

entry_state: investigate
terminal_states: [complete, blocked]
```

**Investigation Workflow** (investigation.yaml):
```yaml
name: investigation
version: 1
description: Research and produce findings

states:
  - name: research
    description: Examine codebase and gather information
    prompt: prompts/investigation/research.md
    next: draft
  - name: draft
    description: Write findings document
    prompt: prompts/investigation/draft.md
    next: review
  - name: review
    description: Review findings for completeness
    prompt: prompts/investigation/review.md
    next: complete

entry_state: research
terminal_states: [complete, blocked]
```

**Refactor Workflow** (refactor.yaml):
```yaml
name: refactor
version: 1
description: Change structure without changing behavior

states:
  - name: characterize
    description: Write tests capturing current behavior
    prompt: prompts/refactor/characterize.md
    next: char_review
  - name: char_review
    description: Review characterization tests
    prompt: prompts/refactor/char-review.md
    next: refactor
  - name: refactor
    description: Perform structural changes
    prompt: prompts/refactor/refactor.md
    next: verify
  - name: verify
    description: Verify behavior unchanged
    prompt: prompts/refactor/verify.md
    next: complete

entry_state: characterize
terminal_states: [complete, blocked]
```

**Performance Workflow** (performance.yaml):
```yaml
name: performance
version: 1
description: Optimize without changing behavior

states:
  - name: baseline
    description: Establish performance baseline
    prompt: prompts/performance/baseline.md
    next: analyze
  - name: analyze
    description: Profile and identify bottlenecks
    prompt: prompts/performance/analyze.md
    next: optimize
  - name: optimize
    description: Implement optimization
    prompt: prompts/performance/optimize.md
    next: benchmark
  - name: benchmark
    description: Verify improvement and correctness
    prompt: prompts/performance/benchmark.md
    next: complete

entry_state: baseline
terminal_states: [complete, blocked]
```

#### 4. Validation Rules (V1)

- Valid YAML syntax
- Required fields present
- All states have unique names
- All states have prompt file that exists
- Entry state exists
- Terminal states exist
- All non-terminal states have `next`
- All `next` values point to valid states
- No unreachable states
- All states can reach a terminal

#### 5. Migration Logic

```bash
# In iterate.sh
detect_workflow_type() {
    # 1. Check for workflow.yaml in plan dir
    # 2. Check for type in plan.md frontmatter
    # 3. Default to feature
}

migrate_old_state() {
    # Map phase_status to current_state
    # pending -> entry_state
    # qa -> qa
    # qa_review -> qa_review
    # implementing -> impl
    # impl_review -> impl_review
    # complete -> complete
}
```

### V1b Tasks

**Workflow Files:**
- [ ] Create bugfix.yaml workflow
- [ ] Create investigation.yaml workflow
- [ ] Create refactor.yaml workflow
- [ ] Create performance.yaml workflow

**Prompt Extraction:**
- [ ] Extract qa.md from iterate.sh
- [ ] Extract qa-review.md from iterate.sh
- [ ] Extract impl.md from iterate.sh
- [ ] Extract impl-review.md from iterate.sh
- [ ] Create prompts for other 4 work types (20 files)

**Common Includes:**
- [ ] Create common/test-commands.md
- [ ] Create common/do-not-rules.md
- [ ] Create common/reasoning-format.md

**Plan Templates:**
- [ ] Create plan-feature.md template
- [ ] Create plan-bugfix.md template
- [ ] Create plan-investigation.md template
- [ ] Create plan-refactor.md template
- [ ] Create plan-performance.md template

**Scripts:**
- [ ] Enhance validate-workflow.sh (graph reachability, prompt existence)
- [ ] Update init-plan.sh for --type flag
- [ ] Update state.json schema (current_state)
- [ ] Write migration logic

**Testing:**
- [ ] Test backwards compatibility with old plans
- [ ] Test new plan with each work type (5 tests)

---

## V2: Conditional Branches

### Goals
- Support branching based on verdicts
- Reviews can loop back or proceed
- Extract verdict from review output

### New Schema Elements

```yaml
states:
  - name: review
    prompt: prompts/review.md
    verdicts:           # V2: List of valid verdicts
      - approved
      - gaps_found
    next:               # V2: Object instead of string
      approved: next_state
      gaps_found: previous_state
```

### Deliverables

#### 1. Verdict Extraction

```bash
# Extract verdict from review output
extract_verdict() {
    local review_file="$1"
    local valid_verdicts="$2"  # comma-separated

    # Look for "Verdict:" or "## Verdict" section
    local verdict=$(grep -iE "^(##\s*)?verdict:?\s*" "$review_file" | \
                    head -1 | \
                    sed 's/.*verdict:*\s*//i' | \
                    tr '[:upper:]' '[:lower:]' | \
                    tr -d '[:space:]')

    # Validate against allowed verdicts
    if echo "$valid_verdicts" | grep -qw "$verdict"; then
        echo "$verdict"
    else
        echo "unknown"
    fi
}
```

#### 2. Branch Resolution

```bash
# get-next-state.sh updated
get_next_state() {
    local current="$1"
    local verdict="$2"
    local workflow="$3"

    local next=$(yq ".states[] | select(.name == \"$current\") | .next" "$workflow")

    # If next is a string (V1), return it
    if [[ "$next" != *":"* ]]; then
        echo "$next"
        return
    fi

    # If next is an object (V2), resolve by verdict
    yq ".states[] | select(.name == \"$current\") | .next.$verdict" "$workflow"
}
```

### Tasks

- [ ] Add verdicts field to workflow schema
- [ ] Update validation for verdict consistency
- [ ] Implement verdict extraction
- [ ] Update get-next-state.sh for branching
- [ ] Update iterate.sh to capture and use verdicts
- [ ] Update review prompts to output verdicts
- [ ] Test branching workflows

---

## V3: Parameters + Templates

### Goals
- Pass parameters to prompts
- Template variable substitution
- Reduce prompt duplication

### New Schema Elements

```yaml
defaults:
  max_iterations: 10
  timeout: 600

variables:
  custom_var: "value"

states:
  - name: fix
    prompt: prompts/fix.md
    params:                 # V3: Parameters for prompt
      allow_test_changes: false
      focus_area: "RNG implementation"
```

### Dependencies

**Python 3 Requirement:**

The template engine uses Python 3 for template processing. This is required because:
- Complex template logic (conditionals, loops) is difficult in pure bash
- JSON parsing and manipulation is cleaner in Python
- jq alone can't handle template rendering

**V3+ REQUIRES Python 3. There is no alternative.**

- `envsubst` only handles `${VAR}` substitution -- cannot do `{{#if}}`, `{{#each}}`, or `{{> includes}}`
- Pure bash cannot handle nested conditionals or JSON context parsing
- If Python 3 is unavailable, you are limited to V2 functionality (no template parameters)

**Fallback for V1-V2 only:**
```bash
# V1-V2 fallback: simple sed substitution (no conditionals)
render_template_simple() {
    local template="$1"
    local context_json="$2"

    local result=$(cat "$template")
    for key in $(echo "$context_json" | jq -r 'keys[]'); do
        local value=$(echo "$context_json" | jq -r --arg k "$key" '.[$k]')
        result=$(echo "$result" | sed "s/{{$key}}/$value/g")
    done
    echo "$result"
}
```
This fallback does NOT support `{{#if}}`, `{{#each}}`, `{{> includes}}`, or nested variables.

### Deliverables

#### 1. Template Engine

The template engine must handle:
- `{{variable}}` -- Simple substitution
- `{{#if condition}}...{{/if}}` -- Conditionals
- `{{#each array}}...{{/each}}` -- Iteration
- `{{#unless condition}}...{{/unless}}` -- Negative conditionals
- `{{> path/to/include}}` -- File includes
- `\{{escaped}}` -- Literal braces (escape with backslash)

**Full Python Implementation:**

```python
#!/usr/bin/env python3
# $ARC_SCRIPTS_DIR/render_template.py

import sys
import json
import re
import os

def get_value(context, key):
    """Get nested value from context dict."""
    value = context
    for part in key.strip().split('.'):
        if isinstance(value, dict):
            value = value.get(part, '')
        elif isinstance(value, list) and part.isdigit():
            value = value[int(part)] if int(part) < len(value) else ''
        else:
            return ''
    return value

def is_truthy(value):
    """Check if value is truthy for conditionals."""
    if value is None:
        return False
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return value != 0
    if isinstance(value, str):
        return len(value) > 0
    if isinstance(value, (list, dict)):
        return len(value) > 0
    return True

def process_conditionals(template, context):
    """Process {{#if}}, {{#unless}}, {{else}}, {{/if}}."""

    # Handle {{#if condition}}...{{else}}...{{/if}}
    if_else_pattern = r'\{\{#if\s+([^}]+)\}\}(.*?)\{\{else\}\}(.*?)\{\{/if\}\}'
    def replace_if_else(match):
        condition = match.group(1).strip()
        if_block = match.group(2)
        else_block = match.group(3)
        value = get_value(context, condition)
        return if_block if is_truthy(value) else else_block

    template = re.sub(if_else_pattern, replace_if_else, template, flags=re.DOTALL)

    # Handle {{#if condition}}...{{/if}} (no else)
    if_pattern = r'\{\{#if\s+([^}]+)\}\}(.*?)\{\{/if\}\}'
    def replace_if(match):
        condition = match.group(1).strip()
        block = match.group(2)
        value = get_value(context, condition)
        return block if is_truthy(value) else ''

    template = re.sub(if_pattern, replace_if, template, flags=re.DOTALL)

    # Handle {{#unless condition}}...{{/unless}}
    unless_pattern = r'\{\{#unless\s+([^}]+)\}\}(.*?)\{\{/unless\}\}'
    def replace_unless(match):
        condition = match.group(1).strip()
        block = match.group(2)
        value = get_value(context, condition)
        return block if not is_truthy(value) else ''

    template = re.sub(unless_pattern, replace_unless, template, flags=re.DOTALL)

    return template

def process_each(template, context):
    """Process {{#each array}}...{{/each}}."""
    each_pattern = r'\{\{#each\s+([^}]+)\}\}(.*?)\{\{/each\}\}'

    def replace_each(match):
        array_key = match.group(1).strip()
        block = match.group(2)
        array = get_value(context, array_key)

        if not isinstance(array, list):
            return ''

        result = []
        for i, item in enumerate(array):
            # Create iteration context
            iter_context = {**context, 'this': item, '@index': i, '@first': i == 0, '@last': i == len(array) - 1}
            # Process block with iteration context
            processed = process_variables(block, iter_context)
            result.append(processed)

        return ''.join(result)

    return re.sub(each_pattern, replace_each, template, flags=re.DOTALL)

def process_includes(template, context, base_dir):
    """Process {{> path/to/include}}."""
    include_pattern = r'\{\{>\s*([^}]+)\}\}'

    def replace_include(match):
        include_path = match.group(1).strip()
        full_path = os.path.join(base_dir, include_path)

        if not os.path.exists(full_path):
            return f'<!-- Include not found: {include_path} -->'

        with open(full_path) as f:
            included = f.read()

        # Recursively process the included content
        return render(included, context, base_dir)

    return re.sub(include_pattern, replace_include, template)

def process_variables(template, context):
    """Process {{variable}} substitutions."""
    # Handle escaped braces: \{{ -> {{
    template = template.replace('\\{{', '\x00ESCAPED_OPEN\x00')
    template = template.replace('\\}}', '\x00ESCAPED_CLOSE\x00')

    # Replace variables (not starting with #, /, or >)
    var_pattern = r'\{\{([^#/>][^}]*)\}\}'
    def replace_var(match):
        key = match.group(1).strip()
        # Handle default values: {{var | default: "value"}}
        if ' | default:' in key:
            parts = key.split(' | default:')
            key = parts[0].strip()
            default = parts[1].strip().strip('"\'')
            value = get_value(context, key)
            return str(value) if is_truthy(value) else default
        value = get_value(context, key)
        if isinstance(value, (dict, list)):
            return json.dumps(value)
        return str(value) if value is not None else ''

    template = re.sub(var_pattern, replace_var, template)

    # Restore escaped braces
    template = template.replace('\x00ESCAPED_OPEN\x00', '{{')
    template = template.replace('\x00ESCAPED_CLOSE\x00', '}}')

    return template

def render(template, context, base_dir):
    """Full template rendering pipeline."""
    # Order matters: includes first, then each, then conditionals, then variables
    template = process_includes(template, context, base_dir)
    template = process_each(template, context)
    template = process_conditionals(template, context)
    template = process_variables(template, context)
    return template

if __name__ == '__main__':
    if len(sys.argv) < 3:
        print("Usage: render_template.py <template_file> <context_json>", file=sys.stderr)
        sys.exit(1)

    template_file = sys.argv[1]
    context_json = sys.argv[2]

    with open(template_file) as f:
        template = f.read()

    context = json.loads(context_json)
    base_dir = os.path.dirname(os.path.abspath(template_file))

    result = render(template, context, base_dir)
    print(result)
```

**Bash Wrapper:**

```bash
render_template() {
    local template="$1"
    local context_json="$2"

    python3 "$ARC_HOME/scripts/render_template.py" "$template" "$context_json"
}
```

**Missing Variable Handling:**
- Missing variables render as empty string `""`
- Use `{{var | default: "fallback"}}` for explicit defaults
- Templates should use `{{#if var}}` to guard optional sections

#### 2. Context Building

```bash
build_context() {
    local state_file="$1"
    local workflow="$2"
    local plan_md="$3"
    local state_name="$4"

    # Merge: defaults + state params + computed values
    jq -n \
        --slurpfile state "$state_file" \
        --slurpfile workflow "$workflow" \
        --arg current "$state_name" \
        '
        ($workflow[0].defaults // {}) +
        ($workflow[0].variables // {}) +
        ($workflow[0].states[] | select(.name == $current) | .params // {}) +
        {
            state: $state[0],
            iteration: $state[0].iteration,
            current_state: $current
        }
        '
}
```

### Tasks

- [ ] Add defaults, variables, params to schema
- [ ] Implement template engine (variable substitution)
- [ ] Implement conditional blocks (if/unless)
- [ ] Implement iteration blocks (each)
- [ ] Implement includes
- [ ] Build context from multiple sources
- [ ] Update iterate.sh to render templates
- [ ] Convert existing prompts to use templates
- [ ] Test parameterized workflows

---

## V4: Hooks + Escalation

### Goals
- Run actions after state completion
- Automatic escalation when stuck
- Commit automation
- Custom scripts

### New Schema Elements

```yaml
states:
  - name: fix
    prompt: prompts/fix.md
    constraints:                    # V4: Enforcement rules
      max_iterations: 15
      require_artifacts_in:
        - qa_reasoning.md
      require_artifacts_out:
        - impl_reasoning.md

    after:                          # V4: Post-state actions
      - action: run_tests
        params:
          pattern: "qa_{{phase}}"
          save_to: test_output.txt
      - action: commit
        when: approved
        params:
          message: "feat({{phase}}): implementation"

    escalation:                     # V4: Stuck handling
      - at_iteration: 3
        action: analyze_stuck
      - at_iteration: 5
        action: switch_model
        params:
          model: opus

intervention_triggers:              # V4: Human escalation
  - condition: "stuck_iterations >= max_iterations"
    action: request_human
    message: "Max iterations exceeded"
```

### Deliverables

#### 1. Action Registry

```bash
# actions.sh - Action implementations

action_run_tests() {
    local pattern="$1"
    local save_to="$2"
    local expect_failure="${3:-false}"

    local output=$(run_configured_test_command "$PACKAGE" "$pattern" 2>&1)
    echo "$output" > "$PHASE_DIR/$save_to"

    # Return success based on expect_failure
    if [[ "$expect_failure" == "true" ]]; then
        echo "$output" | grep -q "FAILED" && return 0 || return 1
    else
        echo "$output" | grep -q "FAILED" && return 1 || return 0
    fi
}

action_commit() {
    local message="$1"
    local when="$2"

    if [[ -n "$when" && "$VERDICT" != "$when" ]]; then
        return 0  # Skip, condition not met
    fi

    # Check commit gates
    check_commit_allowed || return 1

    git add -A
    git commit -m "$message"
}

action_switch_model() {
    local model="$1"
    jq --arg model "$model" '.model = $model' "$STATE_FILE" > tmp && mv tmp "$STATE_FILE"
}

action_analyze_stuck() {
    # Generate stuck analysis
    generate_stuck_analysis "$PHASE_DIR"
}

action_request_human() {
    local message="$1"
    request_human_intervention "$message"
}
```

#### 2. Hook Execution

```bash
run_after_hooks() {
    local state_name="$1"
    local verdict="$2"

    local hooks=$(yq ".states[] | select(.name == \"$state_name\") | .after[]" "$WORKFLOW")

    for hook in $hooks; do
        local action=$(echo "$hook" | yq '.action')
        local when=$(echo "$hook" | yq '.when // ""')
        local params=$(echo "$hook" | yq '.params // {}')

        # Check when condition
        if [[ -n "$when" && "$verdict" != "$when" ]]; then
            continue
        fi

        # Execute action
        "action_$action" $(echo "$params" | jq -r 'to_entries | .[] | .value')
    done
}
```

#### 3. Escalation Logic

```bash
check_escalation() {
    local iteration="$1"
    local state_name="$2"

    local escalations=$(yq ".states[] | select(.name == \"$state_name\") | .escalation[]" "$WORKFLOW")

    for esc in $escalations; do
        local at=$(echo "$esc" | yq '.at_iteration')
        if [[ "$iteration" -eq "$at" ]]; then
            local action=$(echo "$esc" | yq '.action')
            local params=$(echo "$esc" | yq '.params // {}')
            "action_$action" $(echo "$params" | jq -r 'to_entries | .[] | .value')
        fi
    done
}
```

### Tasks

- [ ] Add constraints, after, escalation to schema
- [ ] Implement action registry
- [ ] Implement constraint checking
- [ ] Implement hook execution
- [ ] Implement escalation triggers
- [ ] Implement intervention triggers
- [ ] Update iterate.sh to use hooks
- [ ] Update validation for actions
- [ ] Test automated commits
- [ ] Test escalation ladder
- [ ] Test human intervention

---

## V5: Parallel States

### Goals
- Run multiple branches concurrently
- Join strategies (all, any, n_of_m)
- Handle partial failures

### New Schema Elements

```yaml
states:
  - name: characterize
    parallel:                       # V5: Parallel execution
      strategy: all                 # all | any | n_of_m
      n: 2                          # For n_of_m
      branches:
        - name: char_module_a
          prompt: prompts/characterize.md
          params:
            module: module_a
        - name: char_module_b
          prompt: prompts/characterize.md
          params:
            module: module_b
    verdicts:
      - all_complete
      - any_failed
    next:
      all_complete: refactor
      any_failed: blocked
```

### Deliverables

#### 1. Parallel Execution

```bash
run_parallel_state() {
    local state_name="$1"
    local branches=$(yq ".states[] | select(.name == \"$state_name\") | .parallel.branches[]" "$WORKFLOW")
    local strategy=$(yq ".states[] | select(.name == \"$state_name\") | .parallel.strategy" "$WORKFLOW")

    local pids=()
    local results_dir="$PHASE_DIR/parallel_${state_name}"
    mkdir -p "$results_dir"

    # Launch all branches
    for branch in $branches; do
        local branch_name=$(echo "$branch" | yq '.name')
        local prompt=$(echo "$branch" | yq '.prompt')
        local params=$(echo "$branch" | yq '.params // {}')

        (
            run_branch "$branch_name" "$prompt" "$params" > "$results_dir/$branch_name.log" 2>&1
            echo $? > "$results_dir/$branch_name.exit"
        ) &
        pids+=($!)
    done

    # Wait based on strategy
    case "$strategy" in
        all)
            wait_for_all "${pids[@]}"
            ;;
        any)
            wait_for_any "${pids[@]}"
            ;;
        n_of_m)
            local n=$(yq ".states[] | select(.name == \"$state_name\") | .parallel.n" "$WORKFLOW")
            wait_for_n "$n" "${pids[@]}"
            ;;
    esac

    # Determine verdict
    determine_parallel_verdict "$results_dir" "$strategy"
}
```

#### 2. Join Strategies

```bash
wait_for_all() {
    local pids=("$@")
    for pid in "${pids[@]}"; do
        wait "$pid"
    done
}

wait_for_any() {
    local pids=("$@")
    # Returns: index of first finished process (0-based), sets FINISHED_PID
    while true; do
        for i in "${!pids[@]}"; do
            local pid="${pids[$i]}"
            if ! kill -0 "$pid" 2>/dev/null; then
                # This one finished - return its index and set global
                FINISHED_PID="$pid"
                FINISHED_INDEX="$i"
                wait "$pid"  # Reap the process and get exit code
                return $?    # Return the exit status
            fi
        done
        sleep 1
    done
}

wait_for_n() {
    local n="$1"
    shift
    local pids=("$@")
    local -A finished=()  # Track which PIDs have been counted
    local completed=0

    while [[ $completed -lt $n ]]; do
        for pid in "${pids[@]}"; do
            # Skip if already counted
            [[ -n "${finished[$pid]:-}" ]] && continue

            if ! kill -0 "$pid" 2>/dev/null; then
                finished[$pid]=1
                ((completed++))
                wait "$pid"  # Reap the process
            fi
        done
        sleep 1
    done
}
```

### Tasks

- [ ] Add parallel to schema
- [ ] Implement parallel execution
- [ ] Implement join strategies
- [ ] Handle partial failures
- [ ] Update state tracking for branches
- [ ] Update validation for parallel
- [ ] Test parallel workflows
- [ ] Test failure scenarios

---

## Adversarial System (Parallel Track)

Can be implemented alongside V1-V2. This is a full subsystem requiring dedicated implementation phases.

### Estimated Effort: 3-4 days

### Phase A1: Adversary Infrastructure

**Scripts:**
- [ ] Create `run-adversary.sh` -- Spawn single adversary agent
- [ ] Create `collect-failures.sh` -- Parse adversary output, extract failures
- [ ] Create `adversaries.yaml` -- Define adversary committee configuration

**Output Format Spec:**
```yaml
# adversaries.yaml
max_iterations: 5
require_all_pass: false  # Can proceed with warnings

adversaries:
  - name: coverage
    prompt: prompts/adversaries/coverage.md
    required: true
    pass_verdict: coverage_sufficient
    fail_verdict: coverage_gaps

  - name: ambiguity
    prompt: prompts/adversaries/ambiguity.md
    required: true
    pass_verdict: unambiguous
    fail_verdict: ambiguous

  # ... (scope, consistency, executability)
```

**Tasks:**
- [ ] Define adversary output schema (JSON)
- [ ] Implement verdict extraction from adversary output
- [ ] Implement failure extraction (specific issues found)
- [ ] Test single adversary execution

### Phase A2: Review Loop

**Scripts:**
- [ ] Create `plan-review-loop.sh` -- Full adversary committee loop
- [ ] Create `aggregate-failures.sh` -- Combine failures for plan agent

**Loop Logic:**
```bash
for iteration in 1..max_iterations:
    results = run_all_adversaries(plan)

    if all_required_pass(results):
        return "approved"

    if iteration == max_iterations:
        return "needs_human_review"

    failures = aggregate_failures(results)
    invoke_plan_agent_fix(plan, failures)
```

**Tasks:**
- [ ] Implement parallel adversary execution
- [ ] Implement failure aggregation
- [ ] Implement plan agent fix invocation
- [ ] Implement iteration tracking
- [ ] Test full loop with mock plan

### Phase A3: Adversary Prompts

Write all 5 adversary prompts with specific output formats.

**Tasks:**
- [ ] Create prompts/adversaries/coverage.md
- [ ] Create prompts/adversaries/ambiguity.md
- [ ] Create prompts/adversaries/scope.md
- [ ] Create prompts/adversaries/consistency.md
- [ ] Create prompts/adversaries/executability.md
- [ ] Test each prompt produces parseable output

**Required Output Format (all adversaries):**
```markdown
## Analysis

[Detailed analysis]

## Issues Found

### Critical
- [ ] Issue description with file:line reference

### Warning
- [ ] Issue description

## Verdict
<verdict_name>
```

### Phase A4: Integration

Connect adversary system to planning process.

**Tasks:**
- [ ] Modify init-plan.sh to trigger review loop
- [ ] Create approve-plan.sh with override support
- [ ] Add review results to plan directory structure
- [ ] Update Plan agent prompt to handle adversary feedback
- [ ] Test end-to-end: plan creation -> adversary review -> approval

---

## Test Infrastructure

### Test Naming Convention

Tests must follow the naming pattern `qa_<phase>` to be discovered by the orchestration system.

**Enforcement:** The `run_tests` action uses this pattern:
```bash
# The configured test command runs with the phase-specific pattern
run_configured_test_command "$PACKAGE" "qa_${PHASE}"
```

**Who Enforces:** The QA sub-agent is instructed in its prompt to name tests following this convention. The qa-review adversary checks for compliance.

**Example:**
```rust
// For phase "port-pcg", tests should be in a file like:
// {{default_package}}/tests/qa_port_pcg.rs

#[cfg(test)]
mod qa_port_pcg {
    #[test]
    fn test_pcg_sequence() { ... }

    #[test]
    fn test_seed_consistency() { ... }
}
```

### Test Result Parsing

Test output is parsed to determine verdicts:

```bash
parse_test_results() {
    local output_file="$1"

    local total=$(grep -oP '\d+ tests' "$output_file" | head -1 | grep -oP '\d+')
    local passed=$(grep -oP '\d+ passed' "$output_file" | grep -oP '\d+' || echo 0)
    local failed=$(grep -oP '\d+ failed' "$output_file" | grep -oP '\d+' || echo 0)

    # Update state.json
    jq --argjson total "$total" \
       --argjson passed "$passed" \
       '.tests_total = $total | .tests_passing = $passed' \
       "$STATE_FILE" > tmp && mv tmp "$STATE_FILE"

    # Return verdict
    if [[ $failed -eq 0 ]]; then
        echo "all_passing"
    else
        echo "some_failing"
    fi
}
```

---

## Testing Strategy

### Unit Tests
- Workflow validation
- Template rendering
- Verdict extraction
- State transitions

### Integration Tests
- Full workflow execution (each type)
- Backwards compatibility
- Migration logic
- Error handling

### Manual Tests
- Create plan with each type
- Execute through to completion
- Test intervention scenarios
- Test override modes
