# Orchestrator Phase-Splitting Logic (CONCISE)

Add this to orchestrator prompt after "Decision Tree" section:

## When to Split Phases (Auto-Detection)

After each iteration, check state for these conditions:

### Split Immediately (Exit iteration loop, run split)
```bash
if stuck_iterations >= 5 OR
   hang_count >= 2 OR
   (iteration >= 10 AND tests_passing/tests_total < 0.5) OR
   status == "blocked"
then
   Run: arc analyze-phase <plan> <phase>
   Read recommendations
   Execute: arc split-phase <plan> <phase> <sub1> <sub2> ...
   Mark completed sub-phases with: arc manage-phase <plan> <sub> complete
   Continue with first incomplete sub-phase
fi
```

### Intervene (Provide targeted instructions, 1-2 more iterations)
```bash
if stuck_iterations >= 3 OR
   packages.length >= 3 OR
   (compilation errors in >= 3 files)
then
   Read impl_reasoning.md
   Provide specific guidance to next iteration
   If still stuck after 2 iterations -> Split
fi
```

### Split Strategies
- **stuck_iterations >= 5**: Agent can't make progress -- Split by concern/module
- **hang_count >= 2**: Infinite loop/paralysis -- Manual fix, then split
- **packages >= 3**: Cross-package coordination -- Split by package
- **Many file errors**: Cascading changes -- Split by file/module groups
- **Multi-language** (Rust+C/CUDA): Split by language boundary

## Commands
```bash
# Check if split needed (returns exit code 0/1/2)
arc analyze-phase <plan> <phase>

# Split phase
arc split-phase <plan> <phase> <sub1> [sub2] ...

# Mark completed sub-phases
arc manage-phase <plan> <sub> complete [note]
arc manage-phase <plan> <sub> tests <pass> <total>
arc manage-phase <plan> <sub> packages <pkg1> [pkg2]

# List all phases
arc list-phases <plan>
```

## Detection Example
```
Iteration 4 complete: tests 0/57, stuck_iterations=3
-> Check: stuck >= 3? YES -> Intervene
-> Provide targeted instructions

Iteration 5 complete: tests 0/57, stuck_iterations=4
-> Still stuck -> Provide more specific instructions

Iteration 6 complete: tests 0/57, stuck_iterations=5, hang_count=1
-> Check: stuck >= 5? YES -> Split recommended
-> Run: arc analyze-phase my-feature cuda-gpu
-> Exit code 2 (critical)
-> Execute split: arc split-phase ... cuda-rust cuda-c cuda-kernels
-> Continue with cuda-rust
```
