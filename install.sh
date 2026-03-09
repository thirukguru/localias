#!/bin/bash
# install.sh — Install localias from GitHub releases
# Author: Thiru K
# Repo: github.com/thirukguru/localias
#
# Usage: curl -fsSL https://raw.githubusercontent.com/thirukguru/localias/main/install.sh | bash

set -euo pipefail

REPO="thirukguru/localias"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="localias"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[OK]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1" >&2; exit 1; }

# Detect OS
detect_os() {
    local os
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$os" in
        darwin) echo "darwin" ;;
        linux)  echo "linux" ;;
        *)      error "Unsupported OS: $os" ;;
    esac
}

# Detect architecture
detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)      error "Unsupported architecture: $arch" ;;
    esac
}

# Get latest release version
get_latest_version() {
    local version
    version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/')
    if [ -z "$version" ]; then
        error "Could not determine latest version"
    fi
    echo "$version"
}

main() {
    echo ""
    echo "  ⚡ Localias Installer"
    echo "  ────────────────────"
    echo ""

    local os arch version download_url tmp_dir

    os="$(detect_os)"
    arch="$(detect_arch)"
    info "Detected: ${os}/${arch}"

    # Allow version override via env
    version="${LOCALIAS_VERSION:-}"
    if [ -z "$version" ]; then
        info "Fetching latest release..."
        version="$(get_latest_version)"
    fi
    info "Version: ${version}"

    # Strip 'v' prefix for filename
    local ver_num="${version#v}"
    download_url="https://github.com/${REPO}/releases/download/${version}/${BINARY_NAME}_${ver_num}_${os}_${arch}.tar.gz"

    info "Downloading from: ${download_url}"

    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "$tmp_dir"' EXIT

    curl -fsSL "$download_url" -o "${tmp_dir}/localias.tar.gz" || error "Download failed. Check the version and URL."

    info "Extracting..."
    tar -xzf "${tmp_dir}/localias.tar.gz" -C "$tmp_dir"

    info "Installing to ${INSTALL_DIR}/${BINARY_NAME}..."
    if [ -w "$INSTALL_DIR" ]; then
        cp "${tmp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    else
        sudo cp "${tmp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    success "localias ${version} installed to ${INSTALL_DIR}/${BINARY_NAME}"
    echo ""
    echo "  Get started:"
    echo "    localias run -- npm run dev"
    echo "    localias alias myapp 3000"
    echo "    localias list"
    echo ""
}

main
