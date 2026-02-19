# Performance Verification

Plan: {{plan}}
Phase: {{phase}}
Plan doc: {{plan_file}}
Baseline: {{phase_dir}}/baseline.md
Optimization log: {{phase_dir}}/optimization_log.md

## Your Task

Verify that the optimizations:
1. Achieved the performance goals
2. Did not change behavior (tests pass)
3. Did not introduce regressions in other areas

## Verification Steps

1. **Run final benchmarks** with same configuration as baseline
2. **Compare to baseline** - was goal achieved?
3. **Run full test suite** - no behavioral regressions:
   ```bash
   {{scripts_dir}}/run-phase-tests.sh {{plan}} {{phase}}
   ```
4. **Check for side effects** - memory usage, startup time, etc.

## Verification Report

Create `{{phase_dir}}/benchmark_report.md`:

```markdown
# Performance Verification: {{phase}}

## Final Benchmark Results

| Metric | Baseline | Goal | Final | Status |
|--------|----------|------|-------|--------|
| Time | X ms | Y ms | Z ms | Pass/Fail |
| Memory | X MB | Y MB | Z MB | Pass/Fail |

## Statistical Validity
- Runs: N
- Std deviation: ± X
- Confidence: 95%

## Behavioral Tests
- Unit tests: X/X passing
- Integration tests: X/X passing
- Regressions: None/<list>

## Side Effects
- [ ] Memory usage: same/changed
- [ ] Startup time: same/changed
- [ ] Binary size: same/changed

## Verdict
APPROVED - Performance goals achieved, no regressions
OR
CONCERNS - <what's wrong>
```

## Rules
- Use same benchmark configuration as baseline
- ALL behavioral tests must pass
- Document any trade-offs made
- **Do NOT commit** - orchestrator handles commits

When done, exit.
