#!/usr/bin/env bash
# Benchmark configuration

BENCH_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TESTBED_DIR="$BENCH_ROOT/testbed"
TASKS_DIR="$BENCH_ROOT/tasks"
RESULTS_DIR="$BENCH_ROOT/results"
WORKDIR="$BENCH_ROOT/.work"

# Tasks to benchmark
TASKS=(task1-feature task2-bugfix task3-refactor task4-investigation)

# Approaches
APPROACHES=(arc claude-minimal claude-plan)

# Number of runs per task per approach
RUNS_PER=3

# Timeout per run (seconds)
RUN_TIMEOUT=2700

# Claude Code binary
CLAUDE_BIN="${CLAUDE_BIN:-claude}"

# Arc binary
ARC_BIN="${ARC_BIN:-arc}"
