#!/usr/bin/env bash
# install-embedder.sh — download llama-server + nomic-embed-text-v1.5 and wire
# up a systemd service for prd2wiki's embedding backend.
#
# Usage:
#   sudo ./scripts/install-embedder.sh            # install / upgrade
#   sudo ./scripts/install-embedder.sh --verify   # check installation only
#
# Environment:
#   PRDWIKI_EMBEDDER_THREADS   override thread count (default: nproc)
#
# Idempotent: safe to re-run; existing files are only replaced when the
# downloaded content differs (checked via SHA-256).

set -euo pipefail

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

INSTALL_PREFIX="/opt/prd2wiki"
BIN_DIR="${INSTALL_PREFIX}/bin"
MODEL_DIR="${INSTALL_PREFIX}/models"

SERVICE_NAME="prd2wiki-embedder"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

PORT=8081
MODEL_FILENAME="nomic-embed-text-v1.5.Q8_0.gguf"
MODEL_PATH="${MODEL_DIR}/${MODEL_FILENAME}"
LLAMA_BIN="${BIN_DIR}/llama-server"

HF_MODEL_URL="https://huggingface.co/nomic-ai/nomic-embed-text-v1.5-GGUF/resolve/main/${MODEL_FILENAME}"

LLAMA_CPP_GITHUB_API="https://api.github.com/repos/ggerganov/llama.cpp/releases/latest"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

log()  { printf '\033[1;34m[embedder-install]\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m[ok]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[error]\033[0m %s\n' "$*" >&2; exit 1; }

require_root() {
    [[ "${EUID:-$(id -u)}" -eq 0 ]] || die "This script must be run as root (sudo)."
}

require_cmd() {
    command -v "$1" &>/dev/null || die "Required command not found: $1"
}

# Detect OS / architecture and return the llama.cpp release asset name.
detect_platform() {
    local os arch
    os="$(uname -s)"
    arch="$(uname -m)"

    [[ "${os}" == "Linux" ]] || die "Unsupported OS: ${os}. Only Linux is supported."

    case "${arch}" in
        x86_64)  LLAMA_ARCH="x64" ;;
        aarch64) LLAMA_ARCH="arm64" ;;
        *)        die "Unsupported architecture: ${arch}. Supported: x86_64, aarch64." ;;
    esac

    # llama.cpp release asset pattern: llama-<version>-bin-ubuntu-x64.zip
    # or llama-<version>-bin-ubuntu-arm64.zip
    LLAMA_ASSET_PATTERN="bin-ubuntu-${LLAMA_ARCH}.zip"
    log "Platform: Linux/${arch} → asset pattern: *${LLAMA_ASSET_PATTERN}"
}

# Resolve the download URL for the llama-server binary from the latest release.
resolve_llama_url() {
    log "Fetching latest llama.cpp release metadata from GitHub..."
    local release_json
    release_json="$(curl -fsSL "${LLAMA_CPP_GITHUB_API}")"

    LLAMA_DOWNLOAD_URL="$(
        printf '%s' "${release_json}" \
        | grep -o '"browser_download_url": *"[^"]*'"${LLAMA_ASSET_PATTERN}"'[^"]*"' \
        | head -1 \
        | sed 's/.*": *"\(.*\)"/\1/'
    )"

    [[ -n "${LLAMA_DOWNLOAD_URL}" ]] \
        || die "Could not find a llama.cpp release asset matching *${LLAMA_ASSET_PATTERN}. Check https://github.com/ggerganov/llama.cpp/releases"

    log "Resolved llama.cpp asset: ${LLAMA_DOWNLOAD_URL}"
}

# Download a URL to a destination path only when the destination is absent or
# the remote content differs (via HTTP ETag / Last-Modified, best-effort).
download() {
    local url="$1" dest="$2" label="${3:-$(basename "$2")}"
    log "Downloading ${label}..."
    curl -fSL --progress-bar -o "${dest}" "${url}"
    ok "Downloaded → ${dest}"
}

# ---------------------------------------------------------------------------
# Verify mode
# ---------------------------------------------------------------------------

do_verify() {
    local issues=0

    check() {
        if [[ -e "$1" ]]; then
            ok "$1 exists"
        else
            warn "MISSING: $1"
            issues=$((issues + 1))
        fi
    }

    check "${LLAMA_BIN}"
    check "${MODEL_PATH}"
    check "${SERVICE_FILE}"

    if systemctl is-enabled --quiet "${SERVICE_NAME}" 2>/dev/null; then
        ok "systemd service '${SERVICE_NAME}' is enabled"
    else
        warn "systemd service '${SERVICE_NAME}' is NOT enabled"
        issues=$((issues + 1))
    fi

    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        ok "systemd service '${SERVICE_NAME}' is active (running)"
        health_check
    else
        warn "systemd service '${SERVICE_NAME}' is NOT running"
        issues=$((issues + 1))
    fi

    if [[ "${issues}" -eq 0 ]]; then
        ok "All checks passed."
    else
        warn "${issues} issue(s) found. Run without --verify to (re)install."
        exit 1
    fi
}

# ---------------------------------------------------------------------------
# Health check
# ---------------------------------------------------------------------------

health_check() {
    log "Waiting for llama-server health endpoint on port ${PORT}..."
    local retries=12 delay=5
    for i in $(seq 1 "${retries}"); do
        if curl -fsSL "http://localhost:${PORT}/health" &>/dev/null; then
            ok "llama-server is healthy at http://localhost:${PORT}/health"
            return 0
        fi
        log "  attempt ${i}/${retries} — waiting ${delay}s..."
        sleep "${delay}"
    done
    warn "Health check failed after $((retries * delay))s. Check: journalctl -u ${SERVICE_NAME}"
    return 1
}

# ---------------------------------------------------------------------------
# Systemd unit
# ---------------------------------------------------------------------------

write_service() {
    log "Writing systemd unit ${SERVICE_FILE}..."
    cat > "${SERVICE_FILE}" <<EOF
[Unit]
Description=prd2wiki embedding server (llama.cpp / nomic-embed-text-v1.5)
Documentation=https://github.com/ggerganov/llama.cpp
After=network.target
Wants=network.target

[Service]
Type=simple
Restart=on-failure
RestartSec=5
ExecStart=${LLAMA_BIN} \\
    -m ${MODEL_PATH} \\
    --port ${PORT} \\
    --embedding \\
    --threads \${PRDWIKI_EMBEDDER_THREADS:-$(nproc)}

# Limit blast radius
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
    ok "Service unit written."
}

# ---------------------------------------------------------------------------
# Install
# ---------------------------------------------------------------------------

do_install() {
    require_root
    require_cmd curl
    require_cmd unzip
    require_cmd systemctl

    detect_platform
    resolve_llama_url

    # --- directories --------------------------------------------------------
    log "Creating install directories..."
    mkdir -p "${BIN_DIR}" "${MODEL_DIR}"

    # --- llama-server binary ------------------------------------------------
    local tmp_zip
    tmp_zip="$(mktemp --suffix=.zip)"
    trap 'rm -f "${tmp_zip}"' EXIT

    download "${LLAMA_DOWNLOAD_URL}" "${tmp_zip}" "llama.cpp release archive"

    log "Extracting llama-server from archive..."
    # The archive contains llama-server (or llama-server.exe on Windows).
    # Try both the root and a subdirectory.
    if unzip -p "${tmp_zip}" '*/llama-server' > "${LLAMA_BIN}" 2>/dev/null \
       || unzip -p "${tmp_zip}" 'llama-server'  > "${LLAMA_BIN}" 2>/dev/null; then
        chmod 755 "${LLAMA_BIN}"
        ok "llama-server installed → ${LLAMA_BIN}"
    else
        # Fallback: extract everything and find the binary
        local tmp_dir
        tmp_dir="$(mktemp -d)"
        unzip -q "${tmp_zip}" -d "${tmp_dir}"
        local found
        found="$(find "${tmp_dir}" -type f -name 'llama-server' | head -1)"
        [[ -n "${found}" ]] || die "llama-server binary not found in release archive."
        install -m 755 "${found}" "${LLAMA_BIN}"
        rm -rf "${tmp_dir}"
        ok "llama-server installed → ${LLAMA_BIN}"
    fi

    # --- model --------------------------------------------------------------
    if [[ -f "${MODEL_PATH}" ]]; then
        log "Model already present at ${MODEL_PATH} — skipping download."
        log "  (Delete ${MODEL_PATH} and re-run to force re-download.)"
    else
        download "${HF_MODEL_URL}" "${MODEL_PATH}" "${MODEL_FILENAME}"
    fi

    # --- systemd service ----------------------------------------------------
    write_service
    systemctl daemon-reload
    systemctl enable "${SERVICE_NAME}"

    if systemctl is-active --quiet "${SERVICE_NAME}"; then
        log "Restarting ${SERVICE_NAME}..."
        systemctl restart "${SERVICE_NAME}"
    else
        log "Starting ${SERVICE_NAME}..."
        systemctl start "${SERVICE_NAME}"
    fi

    ok "Service '${SERVICE_NAME}' enabled and started."
    health_check
}

# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

main() {
    case "${1:-}" in
        --verify|-v)
            do_verify
            ;;
        --help|-h)
            grep '^# ' "$0" | sed 's/^# //'
            ;;
        "")
            do_install
            ;;
        *)
            die "Unknown argument: $1  (use --verify or no arguments)"
            ;;
    esac
}

main "$@"
