# Phase Blocked

Plan: {{plan_name}}
Phase: {{phase}}

This phase is blocked and requires human intervention.

## Possible Causes
- Hit maximum iteration limit without progress
- Hit maximum rollback limit
- Sub-agent hung multiple times
- Unresolvable dispute

## Required Actions
1. Review the phase state: `arc get-state.sh {{plan_name}} {{phase}}`
2. Check impl_reasoning.md for what was attempted
3. Manually investigate and fix the issue
4. Reset the phase: `arc update-state.sh {{plan_name}} {{phase}} reset-blocked`

When done, exit.
