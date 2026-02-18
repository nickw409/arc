#!/usr/bin/env bash
# Script: check-prerequisites.sh
# Purpose: Verify required tools are installed for orchestration system

set -euo pipefail

check_command() {
    if ! command -v "$1" &> /dev/null; then
        echo "❌ $1 not found"
        return 1
    fi
    echo "✓ $1 found: $(command -v "$1")"
}

check_yq_version() {
    local version
    version=$(yq --version 2>&1)
    if [[ "$version" != *"mikefarah"* ]] && [[ "$version" != *"version v4"* ]]; then
        echo "❌ Wrong yq version. Need mikefarah/yq v4+, got: $version"
        return 1
    fi
    echo "✓ yq version OK: $version"
}

check_python_version() {
    local version major minor
    version=$(python3 --version 2>&1 | grep -oP '\d+\.\d+' || echo "0.0")
    major=$(echo "$version" | cut -d. -f1)
    minor=$(echo "$version" | cut -d. -f2)
    if [[ $major -lt 3 ]] || [[ $major -eq 3 && $minor -lt 8 ]]; then
        echo "❌ Python 3.8+ required, got: $version"
        return 1
    fi
    echo "✓ python3 version OK: $version"
}

check_nextest() {
    if ! cargo nextest --version &> /dev/null; then
        echo "❌ cargo-nextest not found"
        echo "   Install with: cargo install cargo-nextest"
        return 1
    fi
    echo "✓ cargo-nextest found"
}

echo "Checking prerequisites..."
echo ""
failed=0

check_command yq && check_yq_version || failed=1
check_command jq || failed=1
check_command python3 && check_python_version || failed=1
check_command cargo || failed=1
check_nextest || failed=1

echo ""
if [[ $failed -eq 0 ]]; then
    echo "All prerequisites satisfied."
else
    echo "Some prerequisites missing. Install them and try again."
    exit 1
fi
