#!/usr/bin/env bash
# sign-plugin.sh — Sign PEPA plugin binaries and manifests with Ed25519.
#
# Usage:
#   ./scripts/sign-plugin.sh                     # sign all plugins
#   ./scripts/sign-plugin.sh github              # sign single plugin
#   ./scripts/sign-plugin.sh --verify github     # verify signature only
#
# Requires:
#   - PEPA_PLUGINS_PRIVATE_KEY env var or ./pepa-plugins-private.pem
#   - openssl (Ed25519 support, OpenSSL 1.1.1+)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

PLUGIN_BIN_DIR="${PROJECT_DIR}/plugins/bin"
PLUGIN_BUILTIN_DIR="${PROJECT_DIR}/plugins/builtin"
PUBLIC_KEY="${PROJECT_DIR}/internal/plugin/signature/pepa-plugins-public.pem"

# ── Functions ────────────────────────────────────────────────────

sign_binary() {
    local plugin_name="$1"
    local category="$2"
    local bin_path="${PLUGIN_BIN_DIR}/${category}/${plugin_name}/${plugin_name}"

    if [[ ! -f "$bin_path" ]]; then
        echo "  SKIP: binary not found: $bin_path"
        return 0
    fi

    local checksum_path="${PLUGIN_BIN_DIR}/${category}/${plugin_name}/checksum"
    local sig_path="${checksum_path}.sig"

    # SHA-256 hash of binary
    printf '%s' "$(sha256sum "$bin_path" | awk '{print $1}')" > "$checksum_path"

    # Sign the checksum with Ed25519
    openssl pkeyutl -sign \
        -inkey "$PRIVATE_KEY" \
        -in "$checksum_path" \
        -out "$sig_path"

    echo "  SIGNED: ${plugin_name} (binary)"
}

sign_yaml() {
    local plugin_name="$1"
    local yaml_path="${PLUGIN_BUILTIN_DIR}/${plugin_name}/plugin.yaml"

    if [[ ! -f "$yaml_path" ]]; then
        echo "  SKIP: plugin.yaml not found: $yaml_path"
        return 0
    fi

    local checksum_path="${PLUGIN_BUILTIN_DIR}/${plugin_name}/plugin.yaml.checksum"
    local sig_path="${checksum_path}.sig"

    printf '%s' "$(sha256sum "$yaml_path" | awk '{print $1}')" > "$checksum_path"

    openssl pkeyutl -sign \
        -inkey "$PRIVATE_KEY" \
        -in "$checksum_path" \
        -out "$sig_path"

    echo "  SIGNED: ${plugin_name} (plugin.yaml)"
}

verify_binary() {
    local plugin_name="$1"
    local category="$2"
    local bin_path="${PLUGIN_BIN_DIR}/${category}/${plugin_name}/${plugin_name}"
    local checksum_path="${PLUGIN_BIN_DIR}/${category}/${plugin_name}/checksum"
    local sig_path="${checksum_path}.sig"

    if [[ ! -f "$bin_path" ]]; then
        echo "  SKIP: binary not built yet: $bin_path"
        return 0
    fi
    if [[ ! -f "$checksum_path" ]] || [[ ! -f "$sig_path" ]]; then
        echo "  SKIP: ${plugin_name} binary not signed (run sign-plugins first)"
        return 0
    fi

    # Verify hash matches
    local actual_hash
    actual_hash=$(sha256sum "$bin_path" | awk '{print $1}')
    local stored_hash
    stored_hash=$(cat "$checksum_path" | tr -d '[:space:]')

    if [[ "$actual_hash" != "$stored_hash" ]]; then
        echo "  TAMPERED: ${plugin_name} (hash mismatch!)"
        return 1
    fi

    # Verify signature
    if openssl pkeyutl -verify \
        -pubin -inkey "$PUBLIC_KEY" \
        -sigfile "$sig_path" \
        -in "$checksum_path" >/dev/null 2>&1; then
        echo "  VERIFIED: ${plugin_name} (binary)"
        return 0
    else
        echo "  INVALID SIG: ${plugin_name} (binary)"
        return 1
    fi
}

verify_yaml() {
    local plugin_name="$1"
    local yaml_path="${PLUGIN_BUILTIN_DIR}/${plugin_name}/plugin.yaml"
    local checksum_path="${PLUGIN_BUILTIN_DIR}/${plugin_name}/plugin.yaml.checksum"
    local sig_path="${checksum_path}.sig"

    if [[ ! -f "$yaml_path" ]]; then
        echo "  SKIP: plugin.yaml not found: $yaml_path"
        return 0
    fi
    if [[ ! -f "$checksum_path" ]] || [[ ! -f "$sig_path" ]]; then
        echo "  UNSIGNED: ${plugin_name} (plugin.yaml — no checksum/sig files)"
        return 1
    fi

    local actual_hash
    actual_hash=$(sha256sum "$yaml_path" | awk '{print $1}')
    local stored_hash
    stored_hash=$(cat "$checksum_path" | tr -d '[:space:]')

    if [[ "$actual_hash" != "$stored_hash" ]]; then
        echo "  TAMPERED: ${plugin_name} (plugin.yaml hash mismatch!)"
        return 1
    fi

    if openssl pkeyutl -verify \
        -pubin -inkey "$PUBLIC_KEY" \
        -sigfile "$sig_path" \
        -in "$checksum_path" >/dev/null 2>&1; then
        echo "  VERIFIED: ${plugin_name} (plugin.yaml)"
        return 0
    else
        echo "  INVALID SIG: ${plugin_name} (plugin.yaml)"
        return 1
    fi
}

# ── Main ─────────────────────────────────────────────────────────

MODE="sign"
TARGET=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --verify)
            MODE="verify"
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [--verify] [plugin_name]"
            echo ""
            echo "  (no args)     Sign all plugin binaries and plugin.yaml manifests"
            echo "  <name>        Sign/verify a single plugin"
            echo "  --verify      Verify signatures instead of signing"
            exit 0
            ;;
        *)
            TARGET="$1"
            shift
            ;;
    esac
done

# ── Resolve keys based on mode ──────────────────────────────────
if [[ "$MODE" == "sign" ]]; then
    # Locate private key
    if [[ -n "${PEPA_PLUGINS_PRIVATE_KEY:-}" ]]; then
        PRIVATE_KEY="$PEPA_PLUGINS_PRIVATE_KEY"
    elif [[ -f "${PROJECT_DIR}/pepa-plugins-private.pem" ]]; then
        PRIVATE_KEY="${PROJECT_DIR}/pepa-plugins-private.pem"
    elif [[ -f "${PROJECT_DIR}/internal/plugin/pepa-plugins-private.pem" ]]; then
        PRIVATE_KEY="${PROJECT_DIR}/internal/plugin/pepa-plugins-private.pem"
    else
        echo "ERROR: Private key not found."
        echo "  Set PEPA_PLUGINS_PRIVATE_KEY or place pepa-plugins-private.pem in project root."
        exit 1
    fi
    if [[ ! -f "$PRIVATE_KEY" ]]; then
        echo "ERROR: Private key file not found: $PRIVATE_KEY"
        exit 1
    fi
else
    # Verify mode — only public key needed
    if [[ ! -f "$PUBLIC_KEY" ]]; then
        echo "ERROR: Public key not found at $PUBLIC_KEY"
        exit 1
    fi
fi

# Determine which plugins to process
# PLUGINS and CATEGORIES are parallel arrays
if [[ -n "$TARGET" ]]; then
    PLUGINS=("$TARGET")
    # Auto-detect category for single target
    if [[ -d "${PLUGIN_BIN_DIR}/builtin/${TARGET}" ]]; then
        CATEGORIES=("builtin")
    elif [[ -d "${PLUGIN_BIN_DIR}/community/${TARGET}" ]]; then
        CATEGORIES=("community")
    else
        CATEGORIES=("builtin")  # fallback
    fi
else
    PLUGINS=()
    CATEGORIES=()
    if [[ -d "$PLUGIN_BIN_DIR" ]]; then
        for category_dir in "$PLUGIN_BIN_DIR"/*/; do
            [[ -d "$category_dir" ]] || continue
            local_category="$(basename "$category_dir")"
            for plugin_dir in "$category_dir"*/; do
                [[ -d "$plugin_dir" ]] || continue
                PLUGINS+=("$(basename "$plugin_dir")")
                CATEGORIES+=("$local_category")
            done
        done
    fi
    if [[ ${#PLUGINS[@]} -eq 0 ]]; then
        echo "No plugins found in $PLUGIN_BIN_DIR. Run 'make plugins' first."
        exit 1
    fi
fi

echo "=== PEPA Plugin Signer ==="
echo "Mode:   $MODE"
echo "Plugins: ${PLUGINS[*]}"
echo ""

FAILED=0

for i in "${!PLUGINS[@]}"; do
    name="${PLUGINS[$i]}"
    category="${CATEGORIES[$i]}"
    if [[ "$MODE" == "sign" ]]; then
        echo "[$name]"
        sign_binary "$name" "$category"
        sign_yaml "$name"
    elif [[ "$MODE" == "verify" ]]; then
        echo "[$name]"
        if ! verify_binary "$name" "$category"; then
            FAILED=$((FAILED + 1))
        fi
        if ! verify_yaml "$name"; then
            FAILED=$((FAILED + 1))
        fi
    fi
done

echo ""
if [[ "$MODE" == "sign" ]]; then
    echo "Done. All plugins signed."
else
    if [[ $FAILED -gt 0 ]]; then
        echo "VERIFICATION FAILED: $FAILED plugin(s) failed."
        exit 1
    else
        echo "All plugins verified successfully."
    fi
fi
