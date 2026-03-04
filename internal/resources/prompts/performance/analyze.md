# Performance Analysis

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}
Baseline: {{phase_dir}}/baseline.md

## Your Task

Profile the code to identify performance bottlenecks.

## Steps

1. **Run profiler** on the target code
2. **Identify hot spots** - where is time being spent?
3. **Analyze causes** - why is it slow?
4. **Prioritize** - which optimizations will have biggest impact?

## Profiling Tools

- Rust: `cargo flamegraph`, `perf`, `valgrind --tool=callgrind`
- General: CPU profilers, memory profilers, I/O profilers

## Analysis Document

Create `{{phase_dir}}/analysis.md`:

```markdown
# Performance Analysis: {{phase}}

## Profiling Method
<tool used, how it was run>

## Hot Spots

### Hot Spot 1: <function/area>
- **Time spent**: X% of total
- **Location**: <file:line>
- **Cause**: <why it's slow>
- **Optimization potential**: high/medium/low

### Hot Spot 2: ...

## Bottleneck Categories
- [ ] CPU-bound: <details>
- [ ] Memory-bound: <details>
- [ ] I/O-bound: <details>
- [ ] Algorithmic: <details>

## Recommended Optimizations (priority order)
1. <optimization> - expected impact: X%
2. <optimization> - expected impact: X%

## Optimizations NOT Recommended
<things that look like optimizations but won't help>
```

## Rules
- Profile, don't guess
- Focus on high-impact optimizations
- Document expected impact for each recommendation
- **Do NOT commit** - orchestrator handles commits

When done, write:

## Memory
[Key findings, metrics, decisions made. Future runs of this state will see this.]

## Verdict
done
