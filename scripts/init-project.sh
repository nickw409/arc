#!/usr/bin/env bash
#
# init-project.sh - Initialize arc in a project
#
# Detects language and test runner, creates .arc.yaml, .plans/, and
# drops .claude/commands/plan.md for the planning agent.
#
# Usage: arc init [--runner RUNNER] [--lang LANG]

set -euo pipefail

PROJECT_ROOT="${ARC_PROJECT_ROOT:-$(pwd)}"
ARC_HOME="${ARC_HOME:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"

# Parse arguments
FORCE_RUNNER=""
FORCE_LANG=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --runner|-r)
            FORCE_RUNNER="$2"
            shift 2
            ;;
        --lang|-l)
            FORCE_LANG="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: arc init [--runner RUNNER] [--lang LANG]"
            echo ""
            echo "Options:"
            echo "  --runner, -r   Force test runner (cargo-nextest, vitest, pytest, go-test)"
            echo "  --lang, -l     Force language (rust, typescript, python, go)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# Check if already initialized
if [[ -f "$PROJECT_ROOT/.arc.yaml" ]]; then
    echo "Project already initialized (.arc.yaml exists)."
    echo "Delete .arc.yaml and re-run to reinitialize."
    exit 1
fi

echo "Initializing arc in $(basename "$PROJECT_ROOT")..."
echo ""

# =============================================================================
# AUTO-DETECTION
# =============================================================================

detect_language() {
    if [[ -f "$PROJECT_ROOT/Cargo.toml" ]]; then
        echo "rust"
    elif [[ -f "$PROJECT_ROOT/go.mod" ]]; then
        echo "go"
    elif [[ -f "$PROJECT_ROOT/pyproject.toml" ]] || [[ -f "$PROJECT_ROOT/setup.py" ]] || [[ -f "$PROJECT_ROOT/requirements.txt" ]]; then
        echo "python"
    elif [[ -f "$PROJECT_ROOT/package.json" ]]; then
        echo "typescript"
    else
        echo "unknown"
    fi
}

detect_runner() {
    local lang="$1"

    case "$lang" in
        rust)
            if command -v cargo-nextest &>/dev/null || cargo nextest --version &>/dev/null 2>&1; then
                echo "cargo-nextest"
            else
                echo "cargo-test"
            fi
            ;;
        typescript)
            if [[ -f "$PROJECT_ROOT/package.json" ]]; then
                if grep -q '"vitest"' "$PROJECT_ROOT/package.json" 2>/dev/null; then
                    echo "vitest"
                elif grep -q '"jest"' "$PROJECT_ROOT/package.json" 2>/dev/null; then
                    echo "jest"
                else
                    echo "vitest"
                fi
            else
                echo "vitest"
            fi
            ;;
        python)
            if [[ -f "$PROJECT_ROOT/pyproject.toml" ]] && grep -q "pytest" "$PROJECT_ROOT/pyproject.toml" 2>/dev/null; then
                echo "pytest"
            elif pip show pytest &>/dev/null 2>&1; then
                echo "pytest"
            else
                echo "pytest"
            fi
            ;;
        go)
            echo "go-test"
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

detect_default_package() {
    local lang="$1"

    case "$lang" in
        rust)
            # Read package name from root Cargo.toml
            if [[ -f "$PROJECT_ROOT/Cargo.toml" ]]; then
                local name
                name=$(grep '^name' "$PROJECT_ROOT/Cargo.toml" | head -1 | sed 's/.*= *"\(.*\)"/\1/' 2>/dev/null || echo "")
                if [[ -n "$name" ]]; then
                    echo "$name"
                    return
                fi
            fi
            echo ""
            ;;
        typescript)
            if [[ -f "$PROJECT_ROOT/package.json" ]]; then
                local name
                name=$(grep '"name"' "$PROJECT_ROOT/package.json" | head -1 | sed 's/.*: *"\(.*\)".*/\1/' 2>/dev/null || echo "")
                echo "$name"
            fi
            ;;
        python)
            if [[ -f "$PROJECT_ROOT/pyproject.toml" ]]; then
                local name
                name=$(grep '^name' "$PROJECT_ROOT/pyproject.toml" | head -1 | sed 's/.*= *"\(.*\)"/\1/' 2>/dev/null || echo "")
                echo "$name"
            fi
            ;;
        go)
            if [[ -f "$PROJECT_ROOT/go.mod" ]]; then
                head -1 "$PROJECT_ROOT/go.mod" | awk '{print $2}' 2>/dev/null || echo ""
            fi
            ;;
        *)
            echo ""
            ;;
    esac
}

get_build_command() {
    local lang="$1"
    case "$lang" in
        rust)       echo "cargo build" ;;
        typescript) echo "npm run build" ;;
        python)     echo "" ;;
        go)         echo "go build ./..." ;;
        *)          echo "" ;;
    esac
}

get_test_command() {
    local runner="$1"
    case "$runner" in
        cargo-nextest)  echo "cargo nextest run" ;;
        cargo-test)     echo "cargo test" ;;
        vitest)         echo "npx vitest run" ;;
        jest)           echo "npx jest" ;;
        pytest)         echo "pytest" ;;
        go-test)        echo "go test ./..." ;;
        *)              echo "" ;;
    esac
}

# Run detection
LANG="${FORCE_LANG:-$(detect_language)}"
RUNNER="${FORCE_RUNNER:-$(detect_runner "$LANG")}"
DEFAULT_PKG=$(detect_default_package "$LANG")
BUILD_CMD=$(get_build_command "$LANG")
TEST_CMD=$(get_test_command "$RUNNER")

echo "Detected:"
echo "  Language:    $LANG"
echo "  Test runner: $RUNNER"
[[ -n "$DEFAULT_PKG" ]] && echo "  Package:     $DEFAULT_PKG"
echo ""

# =============================================================================
# CREATE FILES
# =============================================================================

# .arc.yaml
cat > "$PROJECT_ROOT/.arc.yaml" << EOF
# Arc project configuration
# Docs: https://github.com/your-org/arc

language: $LANG
runner: $RUNNER
default_package: "$DEFAULT_PKG"
build_command: "$BUILD_CMD"
test_command: "$TEST_CMD"

git:
  commit_style: conventional  # conventional | freeform
  sign: false
  co_author: false
EOF
echo "Created .arc.yaml"

# .plans directory
mkdir -p "$PROJECT_ROOT/.plans/active" "$PROJECT_ROOT/.plans/archive"
echo "Created .plans/"

# .claude/commands/plan.md
mkdir -p "$PROJECT_ROOT/.claude/commands"
if [[ -f "$ARC_HOME/templates/plan-command.md" ]]; then
    cp "$ARC_HOME/templates/plan-command.md" "$PROJECT_ROOT/.claude/commands/plan.md"
    echo "Created .claude/commands/plan.md"
else
    echo "Warning: plan-command.md template not found, skipping slash command"
fi

# Install git hooks (if in a git repo)
if git rev-parse --git-dir > /dev/null 2>&1; then
    GIT_HOOKS_DIR="$(git rev-parse --git-dir)/hooks"
    ARC_HOOKS_DIR="$ARC_HOME/enforcement/hooks"

    if [[ -d "$ARC_HOOKS_DIR" ]]; then
        for hook in "$ARC_HOOKS_DIR"/*; do
            [[ -f "$hook" ]] || continue
            HOOK_NAME=$(basename "$hook")
            TARGET="$GIT_HOOKS_DIR/$HOOK_NAME"

            if [[ -f "$TARGET" ]]; then
                echo "Warning: git hook '$HOOK_NAME' already exists, skipping (backup and delete it to use arc's)"
            else
                ln -sf "$hook" "$TARGET"
                echo "Installed git hook: $HOOK_NAME"
            fi
        done
    fi
fi

# Add .plans to .gitignore if not already there
if [[ -f "$PROJECT_ROOT/.gitignore" ]]; then
    if ! grep -q '^\.plans/' "$PROJECT_ROOT/.gitignore" 2>/dev/null; then
        echo "" >> "$PROJECT_ROOT/.gitignore"
        echo "# Arc orchestration plans" >> "$PROJECT_ROOT/.gitignore"
        echo ".plans/" >> "$PROJECT_ROOT/.gitignore"
        echo "Updated .gitignore"
    fi
else
    cat > "$PROJECT_ROOT/.gitignore" << 'EOF'
# Arc orchestration plans
.plans/
EOF
    echo "Created .gitignore"
fi

echo ""
echo "Arc initialized!"
echo ""
echo "Next steps:"
echo "  1. Review .arc.yaml and adjust if needed"
echo "  2. Create a plan:  arc plan my-feature phase1 phase2"
echo "  3. Or use /plan in Claude Code to start interactive planning"
