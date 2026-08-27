#!/bin/sh
# PEPA CLI installer
# Usage: curl -fsSL https://github.com/AlexsandrKotov/pepa/releases | sh
#
# Environment variables:
#   PEPA_VERSION  - specific version (default: latest)
#   PEPA_INSTALL_DIR - install directory (default: /usr/local/bin)
#   PEPA_OS       - override OS detection (linux, darwin)
#   PEPA_ARCH     - override arch detection (amd64, arm64)
#   PEPA_SKIP_CHECKSUM - set to "1" to skip checksum verification

set -e

REPO="your-username/pepa"
BINARY_NAME="pepa"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

info() { printf "${CYAN}INFO:${NC} %s\n" "$1"; }
success() { printf "${GREEN}✓${NC} %s\n" "$1"; }
error() { printf "${RED}ERROR:${NC} %s\n" "$1" >&2; exit 1; }

# Detect OS
detect_os() {
    if [ -n "$PEPA_OS" ]; then
        echo "$PEPA_OS"
        return
    fi
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       error "Unsupported OS: $(uname -s)" ;;
    esac
}

# Detect architecture
detect_arch() {
    if [ -n "$PEPA_ARCH" ]; then
        echo "$PEPA_ARCH"
        return
    fi
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *)             error "Unsupported architecture: $(uname -m)" ;;
    esac
}

# Get latest version from GitHub
get_latest_version() {
    if [ -n "$PEPA_VERSION" ]; then
        echo "$PEPA_VERSION"
        return
    fi
    _glv_version=""
    _glv_version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
        | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
    if [ -z "$_glv_version" ]; then
        error "Could not determine latest version. Set PEPA_VERSION explicitly."
    fi
    echo "$_glv_version"
}

# Verify SHA256 checksum
verify_checksum() {
    _vc_file="$1"
    _vc_url="$2"

    if [ "$PEPA_SKIP_CHECKSUM" = "1" ]; then
        info "Checksum verification skipped"
        return 0
    fi

    info "Downloading checksums..."
    _vc_sums=$(curl -fsSL "${_vc_url}" 2>/dev/null) || {
        info "Could not download checksums — skipping verification"
        return 0
    }

    _vc_filename=$(basename "$_vc_file")
    _vc_expected=$(echo "$_vc_sums" | grep "$_vc_filename" | awk '{print $1}')

    if [ -z "$_vc_expected" ]; then
        info "No checksum found for ${_vc_filename} — skipping verification"
        return 0
    fi

    info "Verifying SHA256..."
    if command -v sha256sum >/dev/null 2>&1; then
        _vc_actual=$(sha256sum "$_vc_file" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        _vc_actual=$(shasum -a 256 "$_vc_file" | awk '{print $1}')
    else
        info "No SHA256 tool found — skipping verification"
        return 0
    fi

    if [ "$_vc_expected" != "$_vc_actual" ]; then
        error "Checksum mismatch! Expected: ${_vc_expected}, Got: ${_vc_actual}"
    fi
    success "Checksum verified"
}

# Main
main() {
    echo ""
    echo "  PEPA — Platform Engineering & Pipeline Automator"
    echo "  CLI Installer"
    echo ""

    OS=$(detect_os)
    ARCH=$(detect_arch)
    VERSION=$(get_latest_version)

    info "Detected: ${OS}/${ARCH}"
    info "Version: ${VERSION}"

    DOWNLOAD_BASE="https://github.com/${REPO}/releases/download/${VERSION}"
    DOWNLOAD_URL="${DOWNLOAD_BASE}/pepa-${VERSION}-${OS}-${ARCH}"
    INSTALL_DIR="${PEPA_INSTALL_DIR:-/usr/local/bin}"
    TEMP_DIR=$(mktemp -d)

    # Download binary
    info "Downloading ${BINARY_NAME}..."
    if ! curl -fsSL -o "${TEMP_DIR}/${BINARY_NAME}" "$DOWNLOAD_URL"; then
        error "Download failed. Check your network or set PEPA_VERSION explicitly."
    fi

    # Verify checksum
    verify_checksum "${TEMP_DIR}/${BINARY_NAME}" "${DOWNLOAD_BASE}/sha256sums.txt"

    # Make executable
    chmod +x "${TEMP_DIR}/${BINARY_NAME}"

    # Install
    if [ -w "$INSTALL_DIR" ]; then
        mv "${TEMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        info "Elevated permissions required to install to ${INSTALL_DIR}"
        sudo mv "${TEMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
        sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    # Cleanup
    rm -rf "$TEMP_DIR"

    # Verify
    if command -v "$BINARY_NAME" >/dev/null 2>&1; then
        _installed_ver=$("${INSTALL_DIR}/${BINARY_NAME}" version 2>/dev/null || echo "unknown")
        success "PEPA CLI installed to ${INSTALL_DIR}/${BINARY_NAME}"
        success "Version: ${_installed_ver}"
        echo ""
        info "Get started: pepa --help"
        info "Install to K8s: pepa install --namespace pepa"
        info "Install via Docker Compose: pepa install --compose"
    else
        error "Installation completed but '${BINARY_NAME}' not found in PATH. Add ${INSTALL_DIR} to your PATH."
    fi

    echo ""
}

main "$@"
