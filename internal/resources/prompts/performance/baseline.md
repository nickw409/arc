# Performance Baseline

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}

## Your Task

Establish a performance baseline for the code to be optimized.

## Steps

1. **Read the plan** to understand what needs optimization
2. **Identify metrics** to measure (time, memory, throughput, etc.)
3. **Create benchmark tests** that measure current performance
4. **Run benchmarks multiple times** to get stable measurements
5. **Document baseline** in `{{phase_dir}}/baseline.md`

## Baseline Document

Create `{{phase_dir}}/baseline.md`:

```markdown
# Performance Baseline: {{phase}}

## Target Code
<what code/operation is being measured>

## Metrics
| Metric | Value | Units | Std Dev |
|--------|-------|-------|---------|
| Execution time | X | ms | ± Y |
| Memory usage | X | MB | ± Y |
| Throughput | X | ops/sec | ± Y |

## Benchmark Configuration
- Hardware: <CPU, RAM, etc.>
- Dataset size: <N items>
- Number of runs: <N>
- Warm-up runs: <N>

## Benchmark Code Location
- Place benchmarks according to your project's conventions

## Notes
<any observations about current performance>
```

## Benchmark Requirements

- Use criterion or similar for statistical rigor
- Run enough iterations for stable results
- Document configuration for reproducibility
- Include warm-up runs

## Rules
- Do NOT optimize yet - measure first
- Ensure measurements are reproducible
- **Do NOT commit** - orchestrator handles commits

When done, write:

## Memory
[Key findings, metrics, decisions made. Future runs of this state will see this.]

## Verdict
done
