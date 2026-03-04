# Performance Optimization

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}
Baseline: {{phase_dir}}/baseline.md
Analysis: {{phase_dir}}/analysis.md

## Your Task

Implement the optimizations identified in the analysis, starting with highest impact.

## Steps

1. **Review analysis** for prioritized optimization list
2. **Implement one optimization at a time**
3. **Run benchmarks** after each change
4. **Document results** incrementally
5. **Stop when goal is reached** or diminishing returns

## Optimization Log

Create `{{phase_dir}}/optimization_log.md`:

```markdown
# Optimization Log: {{phase}}

## Target
- Baseline: <metric value>
- Goal: <target value> (<X% improvement>)

## Optimizations Applied

### Optimization 1: <description>
- **Change**: <what was changed>
- **Result**: <new metric value> (<X% improvement>)
- **Cumulative**: <X% total improvement>

### Optimization 2: ...

## Final Results
| Metric | Baseline | Final | Improvement |
|--------|----------|-------|-------------|
| Time | X ms | Y ms | Z% |

## Optimizations NOT Applied
<things considered but rejected, and why>
```

## Rules
- One optimization at a time
- Measure after each change
- Keep existing tests passing
- Do NOT change behavior
- **Do NOT commit** - orchestrator handles commits

When done, write:

## Memory
[Key findings, metrics, decisions made. Future runs of this state will see this.]

## Verdict
done
