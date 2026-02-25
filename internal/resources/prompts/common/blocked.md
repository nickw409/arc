# Phase Blocked

Plan: {{plan}}
Phase: {{phase}}

This phase is blocked and requires human intervention.

## Possible Causes
- Hit maximum iteration limit without progress
- Sub-agent hung multiple times
- Unresolvable failure

## Required Actions
1. Review the phase state: `arc manage {{plan}} {{phase}} show`
2. Inspect the last iteration directory for what was attempted
3. Manually investigate and fix the issue
4. Reset the phase: `arc manage {{plan}} {{phase}} pending`

When done, output a summary and exit.
