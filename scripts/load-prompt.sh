#!/usr/bin/env bash
#
# load-prompt.sh - Load and render prompt templates
#
# Usage: load-prompt.sh <prompt-file> [VAR=value ...]
#
# Loads a prompt template and substitutes {{variable}} placeholders.
# Variables can be passed as arguments or exported as PROMPT_VAR_<name>.
#
# For multiline values, use environment variables:
#   export PROMPT_VAR_orchestrator_section="$ORCH_SECTION"
#   load-prompt.sh prompts/feature/impl.md plan_name=my-plan
#
# Simple values can be passed as arguments:
#   load-prompt.sh prompts/feature/impl.md plan_name=my-plan phase=phase1
#
# Output: Rendered prompt to stdout

set -euo pipefail

ARC_SCRIPTS_DIR="${ARC_SCRIPTS_DIR:-$ARC_HOME/scripts}"
ARC_HOME="${ARC_HOME:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"

PROMPT_FILE="${1:-}"

if [[ -z "$PROMPT_FILE" ]]; then
    echo "Usage: load-prompt.sh <prompt-file> [VAR=value ...]" >&2
    echo "" >&2
    echo "For multiline values, export as PROMPT_VAR_<name>:" >&2
    echo "  export PROMPT_VAR_section=\"\$CONTENT\"" >&2
    exit 1
fi

# Resolve prompt file path (relative to arc/ or absolute)
if [[ "$PROMPT_FILE" != /* ]]; then
    PROMPT_FILE="$ARC_HOME/$PROMPT_FILE"
fi

if [[ ! -f "$PROMPT_FILE" ]]; then
    echo "Error: Prompt file not found: $PROMPT_FILE" >&2
    exit 1
fi

# Shift past the prompt file argument
shift || true

# Build Python variable dict from:
# 1. Environment variables prefixed with PROMPT_VAR_
# 2. Command line VAR=value arguments

python3 - "$PROMPT_FILE" "$@" << 'PYTHON_SCRIPT'
import sys
import os
import re

template_file = sys.argv[1]
args = sys.argv[2:]

# Read template
with open(template_file, 'r') as f:
    content = f.read()

# Collect variables
variables = {}

# 1. From environment (PROMPT_VAR_<name> -> {{name}})
for key, value in os.environ.items():
    if key.startswith('PROMPT_VAR_'):
        var_name = key[len('PROMPT_VAR_'):]
        variables[var_name] = value

# 2. From command line arguments (VAR=value)
for arg in args:
    if '=' in arg:
        key, value = arg.split('=', 1)
        variables[key] = value

# Substitute {{variable}} patterns
def replace_var(match):
    var_name = match.group(1)
    return variables.get(var_name, match.group(0))  # Keep original if not found

content = re.sub(r'\{\{(\w+)\}\}', replace_var, content)

print(content)
PYTHON_SCRIPT
