#!/usr/bin/env bash
#
# Arc Installer
#
# Downloads a prebuilt binary from GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/nickw409/arc/main/install.sh | bash
#   or: ./install.sh
#
# To uninstall: rm ~/.local/bin/arc

set -euo pipefail

REPO="nickw409/arc"
INSTALL_DIR="${ARC_INSTALL_DIR:-$HOME/.local/bin}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[arc]${NC} $*"; }
warn()  { echo -e "${YELLOW}[arc]${NC} $*"; }
error() { echo -e "${RED}[arc]${NC} $*" >&2; }

detect_platform() {
    local os arch
    os="$(uname -s)"
    arch="$(uname -m)"

    case "$os" in
        Linux)  os="linux" ;;
        Darwin) os="darwin" ;;
        *)      error "Unsupported OS: $os"; exit 1 ;;
    esac

    case "$arch" in
        x86_64)  arch="amd64" ;;
        aarch64) arch="arm64" ;;
        arm64)   arch="arm64" ;;
        *)       error "Unsupported architecture: $arch"; exit 1 ;;
    esac

    echo "${os} ${arch}"
}

sha256_verify() {
    local file="$1" expected="$2"
    local actual
    if command -v sha256sum &>/dev/null; then
        actual="$(sha256sum "$file" | awk '{print $1}')"
    elif command -v shasum &>/dev/null; then
        actual="$(shasum -a 256 "$file" | awk '{print $1}')"
    else
        error "No sha256sum or shasum found"; exit 1
    fi

    if [ "$actual" != "$expected" ]; then
        error "Checksum mismatch!"
        error "  expected: $expected"
        error "  actual:   $actual"
        exit 1
    fi
}

echo ""
echo "=========================================="
echo "  Arc Installer"
echo "=========================================="
echo ""

read -r os arch <<< "$(detect_platform)"
info "Detected platform: ${os}/${arch}"

info "Fetching latest release..."
tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed 's/.*: "//;s/".*//')"
version="${tag#v}"
info "Latest version: ${version}"

asset_name="arc_${version}_${os}_${arch}.tar.gz"
asset_url="https://github.com/${REPO}/releases/download/${tag}/${asset_name}"
checksum_url="https://github.com/${REPO}/releases/download/${tag}/checksums.txt"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

info "Downloading ${asset_name}..."
curl -fsSL -o "${tmpdir}/${asset_name}" "$asset_url"
curl -fsSL -o "${tmpdir}/checksums.txt" "$checksum_url"

info "Verifying checksum..."
expected="$(grep "  ${asset_name}\$" "${tmpdir}/checksums.txt" | awk '{print $1}')"
if [ -z "$expected" ]; then
    # Also try single-space separator
    expected="$(grep " ${asset_name}\$" "${tmpdir}/checksums.txt" | awk '{print $1}')"
fi
if [ -z "$expected" ]; then
    error "Checksum not found for ${asset_name}"
    exit 1
fi
sha256_verify "${tmpdir}/${asset_name}" "$expected"
info "Checksum verified."

info "Extracting..."
tar -xzf "${tmpdir}/${asset_name}" -C "${tmpdir}"

mkdir -p "$INSTALL_DIR"
mv "${tmpdir}/arc" "${INSTALL_DIR}/arc"
chmod +x "${INSTALL_DIR}/arc"

echo ""
echo "=========================================="
info "Arc ${version} installed to ${INSTALL_DIR}/arc"
echo "=========================================="
echo ""

if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
    warn "${INSTALL_DIR} is not in your PATH."
    warn "Add it to your shell config:"
    warn "  export PATH=\"${INSTALL_DIR}:\$PATH\""
fi

echo "Quick start:"
echo "  arc init          # Set up arc in your project"
echo "  arc plan my-feat phase1 phase2"
echo "  arc review my-feat"
echo "  arc daemon submit my-feat"
echo ""
