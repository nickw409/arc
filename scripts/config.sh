#!/usr/bin/env bash
#
# config.sh - Load project configuration from .arc.yaml
#
# Source this file to get project config values.
# Requires: yq, ARC_PROJECT_ROOT
#
# Exports:
#   ARC_LANG          - Project language (rust, typescript, python, go)
#   ARC_RUNNER        - Test runner name (cargo-nextest, vitest, pytest, go-test)
#   ARC_RUNNER_DIR    - Path to runner plugin directory
#   ARC_TEST_CMD      - Full test command
#   ARC_BUILD_CMD     - Full build command
#   ARC_PACKAGE_TERM  - Language-specific term (crate, package, module)
#   ARC_GIT_STYLE     - Commit message style (conventional, freeform)
#   ARC_GIT_SIGN      - Whether to sign commits (true/false)
#   ARC_DEFAULT_PKG   - Default package/crate name

set -euo pipefail

ARC_CONFIG_FILE="${ARC_PROJECT_ROOT:-.}/.arc.yaml"

if [[ ! -f "$ARC_CONFIG_FILE" ]]; then
    echo "Error: No .arc.yaml found in $ARC_PROJECT_ROOT" >&2
    echo "Run 'arc init' to set up your project." >&2
    return 1 2>/dev/null || exit 1
fi

# Load values from .arc.yaml with defaults
export ARC_LANG=$(yq '.language // "unknown"' "$ARC_CONFIG_FILE")
export ARC_RUNNER=$(yq '.runner // "unknown"' "$ARC_CONFIG_FILE")
export ARC_DEFAULT_PKG=$(yq '.default_package // ""' "$ARC_CONFIG_FILE")
export ARC_BUILD_CMD=$(yq '.build_command // ""' "$ARC_CONFIG_FILE")
export ARC_TEST_CMD=$(yq '.test_command // ""' "$ARC_CONFIG_FILE")

# Git config
export ARC_GIT_STYLE=$(yq '.git.commit_style // "conventional"' "$ARC_CONFIG_FILE")
export ARC_GIT_SIGN=$(yq '.git.sign // "false"' "$ARC_CONFIG_FILE")
export ARC_GIT_COAUTHOR=$(yq '.git.co_author // "false"' "$ARC_CONFIG_FILE")

# Derived values
case "$ARC_LANG" in
    rust)    export ARC_PACKAGE_TERM="crate" ;;
    go)      export ARC_PACKAGE_TERM="module" ;;
    *)       export ARC_PACKAGE_TERM="package" ;;
esac

# Runner directory
export ARC_RUNNER_DIR="${ARC_HOME}/runners/${ARC_RUNNER}"
if [[ ! -d "$ARC_RUNNER_DIR" ]]; then
    echo "Warning: Runner '${ARC_RUNNER}' not found at $ARC_RUNNER_DIR" >&2
fi
