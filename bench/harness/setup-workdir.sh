#!/usr/bin/env bash
# Sets up a fresh working directory for a single benchmark run.
# Usage: setup-workdir.sh <task-name> <approach> <run-number>
# Output: prints the path to the working directory

set -euo pipefail
source "$(dirname "$0")/config.sh"

TASK="$1"
APPROACH="$2"
RUN_NUM="$3"

RUN_DIR="$WORKDIR/${TASK}/${APPROACH}/run-${RUN_NUM}"

# Clean and recreate
rm -rf "$RUN_DIR"
mkdir -p "$RUN_DIR"

# Copy testbed
cp -r "$TESTBED_DIR" "$RUN_DIR/tkit"

# Initialize git repo in the copy so agents can commit
cd "$RUN_DIR/tkit"
git init -q
git add -A
git commit -q -m "initial"
git tag bench-baseline

# For bugfix task, copy the failing test into the working copy
if [[ "$TASK" == "task2-bugfix" ]]; then
    cp "$TASKS_DIR/task2-bugfix/failing_test.go" "$RUN_DIR/tkit/internal/store/"
fi

echo "$RUN_DIR"
