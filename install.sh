#!/usr/bin/env bash
#
# Arc Installer
#
# Installs arc to ~/.arc and adds it to PATH.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/your-org/arc/main/install.sh | bash
#   or: ./install.sh
#
# To uninstall: rm -rf ~/.arc && remove PATH entry from shell config

set -euo pipefail

ARC_INSTALL_DIR="${ARC_INSTALL_DIR:-$HOME/.arc}"
ARC_REPO="${ARC_REPO:-https://github.com/your-org/arc.git}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()    { echo -e "${GREEN}[arc]${NC} $*"; }
warn()    { echo -e "${YELLOW}[arc]${NC} $*"; }
error()   { echo -e "${RED}[arc]${NC} $*" >&2; }

# Check dependencies
check_deps() {
    local missing=()

    command -v jq &>/dev/null   || missing+=("jq")
    command -v yq &>/dev/null   || missing+=("yq (mikefarah v4+)")
    command -v python3 &>/dev/null || missing+=("python3 (3.8+)")
    command -v claude &>/dev/null || missing+=("claude (Claude Code CLI)")
    command -v git &>/dev/null  || missing+=("git")

    if [[ ${#missing[@]} -gt 0 ]]; then
        error "Missing required dependencies:"
        for dep in "${missing[@]}"; do
            echo "  - $dep"
        done
        echo ""
        echo "Install them and re-run the installer."
        exit 1
    fi
}

# Detect shell config file
detect_shell_config() {
    if [[ -n "${ZSH_VERSION:-}" ]] || [[ "$SHELL" == */zsh ]]; then
        echo "$HOME/.zshrc"
    elif [[ -f "$HOME/.bashrc" ]]; then
        echo "$HOME/.bashrc"
    elif [[ -f "$HOME/.bash_profile" ]]; then
        echo "$HOME/.bash_profile"
    else
        echo "$HOME/.profile"
    fi
}

echo ""
echo "=========================================="
echo "  Arc Installer"
echo "=========================================="
echo ""

# Check dependencies
info "Checking dependencies..."
check_deps
info "All dependencies found."
echo ""

# Install or update
if [[ -d "$ARC_INSTALL_DIR" ]]; then
    info "Existing installation found at $ARC_INSTALL_DIR"
    info "Updating..."
    cd "$ARC_INSTALL_DIR"
    git pull --quiet origin main 2>/dev/null || {
        warn "Git pull failed. This might be a local install."
        warn "To update manually: cd $ARC_INSTALL_DIR && git pull"
    }
else
    info "Installing to $ARC_INSTALL_DIR..."

    # If running from the repo itself (not curl), copy instead of clone
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    if [[ -f "$SCRIPT_DIR/bin/arc" ]]; then
        info "Installing from local source..."
        cp -r "$SCRIPT_DIR" "$ARC_INSTALL_DIR"
    else
        info "Cloning from $ARC_REPO..."
        git clone --quiet "$ARC_REPO" "$ARC_INSTALL_DIR"
    fi
fi

# Make scripts executable
chmod +x "$ARC_INSTALL_DIR/bin/arc"
find "$ARC_INSTALL_DIR/scripts" -name "*.sh" -exec chmod +x {} \;
find "$ARC_INSTALL_DIR/runners" -name "*.sh" -exec chmod +x {} \;

# Add to PATH if not already there
SHELL_CONFIG=$(detect_shell_config)
PATH_LINE='export PATH="$HOME/.arc/bin:$PATH"'

if ! grep -qF '.arc/bin' "$SHELL_CONFIG" 2>/dev/null; then
    echo "" >> "$SHELL_CONFIG"
    echo "# Arc - AI orchestration tool" >> "$SHELL_CONFIG"
    echo "$PATH_LINE" >> "$SHELL_CONFIG"
    info "Added arc to PATH in $SHELL_CONFIG"
    warn "Run 'source $SHELL_CONFIG' or open a new terminal to use arc."
else
    info "PATH already configured in $SHELL_CONFIG"
fi

echo ""
echo "=========================================="
info "Arc installed successfully!"
echo "=========================================="
echo ""
echo "Quick start:"
echo "  cd your-project"
echo "  arc init          # Set up arc in your project"
echo "  arc plan my-feat phase1 phase2"
echo "  arc review my-feat"
echo "  arc run my-feat"
echo ""
echo "Or use /arc-plan in Claude Code for interactive planning."
echo ""
