# When to Split Phases - Decision Guide

## Automatic Signals

Run `analyze-phase.sh <plan> <phase>` after each iteration to check these conditions:

### 🚨 CRITICAL - Split Immediately

1. **Stuck iterations ≥ 5** - Same test count for 5+ iterations
2. **Hang count ≥ 2** - Phase timed out (10+ min) twice  
3. **10+ iterations with <50% tests passing** - Long struggle, minimal progress
4. **Compilation errors in ≥5 files** - Changes cascading across too many files
5. **Status = "blocked"** - Auto-blocked by system

### ⚠️ WARNING - Intervene (1-2 iterations) or Prepare to Split

1. **Stuck iterations ≥ 3** - Progress stalling
2. **Multiple crates (≥3)** - Cross-crate coordination complexity
3. **Compilation errors in ≥3 files** - Spreading complexity

## Split Strategies

| When | Strategy | Example |
|------|----------|---------|
| ≥3 crates | **By Crate** | `split-phase.sh plan phase phase-core phase-app phase-shared` |
| ≥5 files with errors | **By File/Module** | `split-phase.sh plan phase types helpers integration` |
| Rust + C/CUDA | **By Language** | `split-phase.sh plan cuda rust-structs c-structs kernels` |
| Multiple concerns | **By Separation** | `split-phase.sh plan phase data-structures algorithms api` |
| Ordered steps | **Sequential** | `split-phase.sh plan migration prep execute validate cleanup` |

## Orchestrator Decision Tree

```
After iteration N:
  ├─ Tests passing? → Complete ✓
  ├─ Run analyze-phase.sh
  │  ├─ Exit 0 → Continue iteration
  │  ├─ Exit 1 → Provide targeted instructions (1-2 more iterations)
  │  └─ Exit 2 → Split phase
  └─ Check disputes → Resolve or split
```

## Usage Examples

```bash
# Check if phase needs splitting
analyze-phase.sh my-feature cuda-gpu

# Split into sub-phases
split-phase.sh my-feature cuda-gpu \
  cuda-rust-structs cuda-bootstrap cuda-kernels

# Mark already-completed sub-phases
manage-phase.sh my-feature cuda-rust-structs complete
manage-phase.sh my-feature cuda-rust-structs crates my-package

# Continue with next sub-phase
# (orchestrator picks up automatically from first incomplete)
```

