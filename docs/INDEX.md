# Orchestration System Documentation

## Quick Start

1. Read [ARCHITECTURE.md](./ARCHITECTURE.md) for system overview
2. Read [IMPLEMENTATION_ROADMAP.md](./IMPLEMENTATION_ROADMAP.md) for build plan
3. Start with V1 implementation

## Document Index

### Core Architecture

| Document | Description |
|----------|-------------|
| [ARCHITECTURE.md](./ARCHITECTURE.md) | System overview, design goals, components |
| [WORKFLOW_SCHEMA.md](./WORKFLOW_SCHEMA.md) | Complete workflow YAML specification |
| [STATE_SCHEMA.md](./STATE_SCHEMA.md) | Phase state.json specification |
| [PROMPT_TEMPLATES.md](./PROMPT_TEMPLATES.md) | Template variable system |

### Processes

| Document | Description |
|----------|-------------|
| [PLANNING_PROCESS.md](./PLANNING_PROCESS.md) | How plans are created and validated |
| [ADVERSARY_SYSTEM.md](./ADVERSARY_SYSTEM.md) | Adversarial plan review |
| [INTERVENTION_SYSTEM.md](./INTERVENTION_SYSTEM.md) | Escape hatches and overrides |

### Implementation

| Document | Description |
|----------|-------------|
| [IMPLEMENTATION_ROADMAP.md](./IMPLEMENTATION_ROADMAP.md) | Version-by-version build plan |

## Key Concepts

### Work Types

The system supports 5 work types, each with its own workflow and plan template:

- **Feature** - New capability (TDD flow)
- **Bug Fix** - Correct existing behavior
- **Investigation** - Research, produce findings
- **Refactor** - Change structure, preserve behavior
- **Performance** - Optimize speed/memory

### Decision Tiers

| Tier | Who Decides | Enforcement |
|------|-------------|-------------|
| Critical | Script gate | Hard block |
| Structural | Workflow rules | Script enforces |
| Tactical | Agent | No enforcement |

### Version Evolution

| Version | Key Features |
|---------|--------------|
| V1 | Linear workflows, basic prompts |
| V2 | Conditional branches |
| V3 | Parameters, templates |
| V4 | Hooks, escalation |
| V5 | Parallel states |

## Directory Structure

```
$ARC_HOME/
+-- docs/               # This documentation
+-- workflows/          # Base workflow definitions
+-- prompts/            # Prompt templates
|   +-- feature/
|   +-- bugfix/
|   +-- investigation/
|   +-- refactor/
|   +-- performance/
|   +-- common/
|   +-- adversaries/
+-- templates/          # Plan templates per work type
+-- adversaries/        # Adversary definitions
+-- scripts/            # Execution scripts
```

## Getting Started with Implementation

### V1 Checklist

1. [ ] Create workflow files in `workflows/`
2. [ ] Extract prompts from iterate.sh to `prompts/`
3. [ ] Write `validate-workflow.sh`
4. [ ] Refactor `iterate.sh` to read workflows
5. [ ] Update `init-plan.sh` for --type flag
6. [ ] Test backwards compatibility

### First Workflow to Implement

Start with `feature.yaml` since it's closest to the current system:

```yaml
name: feature
version: 1

states:
  - name: qa
    prompt: prompts/feature/qa.md
    next: qa_review

  - name: qa_review
    prompt: prompts/feature/qa-review.md
    next: impl

  - name: impl
    prompt: prompts/feature/impl.md
    next: impl_review

  - name: impl_review
    prompt: prompts/feature/impl-review.md
    next: complete

  - name: fix
    prompt: prompts/feature/fix.md
    next: impl

entry_state: qa
terminal_states:
  - complete
```

## Questions?

If something is unclear or missing from the documentation:

1. Check the related docs in this index
2. Look at the implementation roadmap for context
3. Ask for clarification before implementing
