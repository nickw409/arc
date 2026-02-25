# Multi-Phase Direct Execution: {{plan}}

You are executing a multi-phase plan. Process each phase **in order**. After completing
all the work for a phase, mark it complete:

```
arc manage {{plan}} <phase-name> complete
```

If you are blocked on a phase, mark it blocked and continue to the next one:

```
arc manage {{plan}} <phase-name> block "<reason>"
```

---

## Phases

{{plan_md}}

---

## Instructions

1. Work through each phase sequentially from top to bottom
2. For each phase: read the spec, explore the relevant code, implement, test
3. Use `arc manage {{plan}} <phase-name> note <text>` to record intermediate progress
4. Use `arc manage {{plan}} <phase-name> tests <passing> <total>` after running tests
5. After completing all work for a phase, **always** run `arc manage {{plan}} <phase-name> complete`
6. If a phase is blocked, run the block command above and move on — do not stop entirely

## Output Format

When all phases are processed, write:

## Memory
[What was done, what worked, what failed, current state of the codebase. Future sessions will see this.]

## Verdict
done
