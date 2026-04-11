#!/bin/bash
# =============================================================================
# g8e TLS Certificate Management
# =============================================================================
# Manages the platform TLS certificates owned by VSODB (g8es).
#
# VSODB generates the CA and server certificates automatically on first start
# and stores them in its data volume at /data/ssl/. This script orchestrates
# certificate lifecycle via docker — it does not call openssl directly.
#
# Usage:
#   ./scripts/security/manage-ssl.sh <command> [options]
#
# Commands:
#   generate    Ensure certs exist. If VSODB is running and certs are present
#               this is a no-op. If certs are missing, restarts VSODB to
#               trigger generation and waits for it to become healthy.
#   rotate      Force-regenerate all certs. Wipes /data/ssl/ inside the
#               g8es container, then restarts VSODB so it generates
#               a fresh CA and server cert. Run ./g8e platform rebuild
#               afterwards to re-embed the new CA into the operator binary.
#   status      Show cert expiry, subject, and SANs for the CA and server
#               cert currently live in the vsodb-data volume.
#   trust       Install the platform CA certificate into the host OS trust
#               store. Streams the CA from the g8ep container (which mounts
#               vsodb-data read-only at /vsodb) — no file is written to the host.
#               Use --ca-file to supply a cert directly (used by the g8e CLI).
#
# Options:
#   --ca-file <path>    Use this CA cert file instead of fetching from docker
#                       (trust command only — used by the g8e CLI)
#   -h, --help          Show this help
#
# Examples:
#   ./g8e security certs generate
#   ./g8e security certs rotate
#   ./g8e security certs status
#   ./g8e security certs trust
# =============================================================================
set -euo pipefail

_footer() {
    local rc=$?
    [[ $rc -eq 0 ]] || return
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  manage-ssl.sh done"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}
trap _footer EXIT

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  manage-ssl.sh $*"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Create project-local temp directory if it doesn't exist
PROJECT_TMP="$PROJECT_ROOT/tmp"
[[ -d "$PROJECT_TMP" ]] || mkdir -p "$PROJECT_TMP"

CONTAINER="g8es"
CERT_CONTAINER="g8ep"
CERT_SSL_DIR="/vsodb/ssl"
COMPOSE_FILE="$PROJECT_ROOT/docker-compose.yml"

log()  { echo "[certs] $*"; }
warn() { echo "[certs] WARN: $*" >&2; }
die()  { echo "[certs] ERROR: $*" >&2; exit 1; }

# -----------------------------------------------------------------------------
# Helpers
# -----------------------------------------------------------------------------

_is_running() {
    docker ps --filter "name=^${CONTAINER}$" --filter "status=running" \
        --format "{{.Names}}" 2>/dev/null | grep -q "^${CONTAINER}$"
}

_wait_healthy() {
    local waited=0 timeout_s=60
    echo -n "  waiting for ${CONTAINER}: "
    until [ "$(docker inspect --format='{{.State.Health.Status}}' "$CONTAINER" 2>/dev/null)" = "healthy" ]; do
        if (( waited >= timeout_s )); then
            echo "TIMEOUT"
            docker logs --tail 20 "$CONTAINER" 2>&1 | sed 's/^/    /'
            die "${CONTAINER} did not become healthy within ${timeout_s}s"
        fi
        sleep 1
        (( waited++ )) || true
    done
    echo "ready"
}

_require_running() {
    _is_running || die "${CONTAINER} is not running — start the platform first: ./g8e platform start"
}

_cert_info() {
    local label="$1" path="$2"
    echo "  ${label}:"
    if ! docker exec "$CERT_CONTAINER" sh -c "test -f '$path'" 2>/dev/null; then
        echo "    not found"
        return
    fi
    docker exec "$CERT_CONTAINER" sh -c "
        openssl x509 -in '$path' -noout \
            -subject -issuer -startdate -enddate -ext subjectAltName 2>/dev/null \
        | sed 's/^/    /'
        enddate=\$(openssl x509 -in '$path' -noout -enddate 2>/dev/null | cut -d= -f2)
        end_epoch=\$(date -d \"\$enddate\" +%s 2>/dev/null)
        now_epoch=\$(date +%s)
        days=\$(( (end_epoch - now_epoch) / 86400 ))
        if [ \"\$days\" -lt 0 ]; then
            echo '    STATUS: EXPIRED'
        elif [ \"\$days\" -lt 30 ]; then
            echo \"    STATUS: expiring in \${days} days — run: ./g8e security certs rotate\"
        else
            echo \"    STATUS: valid (\${days} days remaining)\"
        fi
    " 2>/dev/null || echo "    (openssl not available — is g8ep running?)"
}

# -----------------------------------------------------------------------------
# Commands
# -----------------------------------------------------------------------------

exec_generate() {
    if _is_running; then
        if docker exec "$CONTAINER" sh -c \
            "test -f /data/ssl/ca.crt && test -f /data/ssl/server.crt" 2>/dev/null; then
            log "Certificates already exist in ${CONTAINER}."
            exec_status
            return
        fi
        log "Certificates missing — restarting ${CONTAINER} to trigger generation..."
        docker compose -f "$COMPOSE_FILE" restart vsodb
        _wait_healthy
    else
        log "Starting ${CONTAINER}..."
        docker compose -f "$COMPOSE_FILE" up -d vsodb
        _wait_healthy
    fi
    log "Certificates generated."
    exec_status
}

exec_rotate() {
    _require_running
    log "Rotating certificates — stopping ${CONTAINER}, wiping ssl/, and restarting..."
    log "WARNING: this invalidates all existing operator mTLS client certificates."
    log "Run './g8e platform rebuild' afterwards to re-embed the new CA in the operator binary."
    echo ""
    docker compose -f "$COMPOSE_FILE" stop vsodb
    docker compose -f "$COMPOSE_FILE" rm -f vsodb
    docker run --rm -v g8es-ssl:/ssl busybox sh -c "rm -rf /ssl/*" \
        || die "Failed to wipe g8es-ssl volume"
    docker compose -f "$COMPOSE_FILE" up -d vsodb
    _wait_healthy
    log "New certificates generated."
    exec_status
}

exec_status() {
    _require_running
    echo ""
    _cert_info "CA cert     (${CERT_SSL_DIR}/ca/ca.crt)" "${CERT_SSL_DIR}/ca/ca.crt"
    echo ""
    _cert_info "Server cert (${CERT_SSL_DIR}/server.crt)" "${CERT_SSL_DIR}/server.crt"
    echo ""
}

# -----------------------------------------------------------------------------
# Trust — install the CA into the host OS certificate store
# -----------------------------------------------------------------------------

# Stream the CA cert PEM from g8ep to stdout. No files written to the host.
_stream_ca_pem() {
    docker exec "$CERT_CONTAINER" cat "${CERT_SSL_DIR}/ca.crt" 2>/dev/null \
        || die "Failed to read CA cert from ${CERT_CONTAINER}. Is the platform running? Run: ./g8e security certs generate"
}

# Trust the CA on this Linux host. Reads PEM from stdin when ca_path is "-",
# otherwise reads from the file path (--ca-file case).
_trust_local_linux() {
    local ca_path="$1"
    log "Trusting CA on this machine (Linux)..."
    if command -v update-ca-certificates >/dev/null 2>&1; then
        log "  Installing system-wide (Debian/Ubuntu)..."
        if [[ "$ca_path" == "-" ]]; then
            _stream_ca_pem | sudo tee /usr/local/share/ca-certificates/g8e-ca.crt >/dev/null
        else
            sudo cp "$ca_path" /usr/local/share/ca-certificates/g8e-ca.crt
        fi
        sudo update-ca-certificates
    elif command -v update-ca-trust >/dev/null 2>&1; then
        log "  Installing system-wide (RHEL/Fedora)..."
        if [[ "$ca_path" == "-" ]]; then
            _stream_ca_pem | sudo tee /etc/pki/ca-trust/source/anchors/g8e-ca.crt >/dev/null
        else
            sudo cp "$ca_path" /etc/pki/ca-trust/source/anchors/g8e-ca.crt
        fi
        sudo update-ca-trust
    else
        warn "No known system trust store tool found — see manual instructions below."
    fi
    if command -v certutil >/dev/null 2>&1; then
        local nssdb="$HOME/.pki/nssdb"
        if [ -d "$nssdb" ]; then
            log "  Updating NSS database (Chrome/Firefox)..."
            certutil -D -d "sql:$nssdb" -n "g8e Operator CA" 2>/dev/null || true
            if [[ "$ca_path" == "-" ]]; then
                local pem
                pem="$(_stream_ca_pem)"
                echo "$pem" | certutil -A -d "sql:$nssdb" -n "g8e Operator CA" -t "CT,," -i /dev/stdin
            else
                certutil -A -d "sql:$nssdb" -n "g8e Operator CA" -t "CT,," -i "$ca_path"
            fi
        fi
    fi
    log "Done. Restart your browser to pick up the new CA."
}

# Trust the CA on this macOS host. Reads PEM from stdin when ca_path is "-".
_trust_local_macos() {
    local ca_path="$1"
    log "Trusting CA on this machine (macOS)..."
    local hashes
    hashes=$(security find-certificate -a -c "g8e Operator CA" -Z \
        /Library/Keychains/System.keychain 2>/dev/null \
        | awk '/SHA-1/{print $NF}')
    if [ -n "$hashes" ]; then
        echo "$hashes" | while read -r h; do
            log "  Removing old cert: $h"
            sudo security delete-certificate -Z "$h" /Library/Keychains/System.keychain
        done
    fi
    if [[ "$ca_path" == "-" ]]; then
        local tmp_file
        tmp_file="$(mktemp "$PROJECT_TMP/g8e-trust-XXXXXX.crt")"
        _stream_ca_pem > "$tmp_file"
        sudo security add-trusted-cert -d -r trustRoot \
            -k /Library/Keychains/System.keychain "$tmp_file"
        rm -f "$tmp_file"
    else
        sudo security add-trusted-cert -d -r trustRoot \
            -k /Library/Keychains/System.keychain "$ca_path"
    fi
    log "Done. Close all browser windows and reload."
}

_print_remote_instructions() {
    local server_host="$1" ws_os="$2"
    local fetch_cmd="ssh ${server_host} \"docker exec g8ep cat /vsodb/ssl/ca.crt\""

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  Step 1 — Fetch the CA cert to your workstation"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "  The cert streams directly from the platform — no file is written"
    echo "  to the server."
    echo ""

    case "$ws_os" in
        macos)
            echo "  Run on your Mac:"
            echo ""
            echo "    ${fetch_cmd} > ~/Downloads/g8e-ca.crt"
            echo ""
            echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
            echo "  Step 2 — Trust the cert on your Mac"
            echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
            echo ""
            echo "  Option A — Terminal (fetch to temp file, then trust):"
            echo ""
            echo "    ${fetch_cmd} > $PROJECT_TMP/g8e-ca.crt && sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain $PROJECT_TMP/g8e-ca.crt && rm $PROJECT_TMP/g8e-ca.crt"
            echo ""
            echo "  Option B — Keychain Access (fetch first, then import):"
            echo "    1. Run the fetch command above to save the file."
            echo "    2. Open Keychain Access and select the System keychain."
            echo "    3. Drag g8e-ca.crt into the keychain, or File > Import Items."
            echo "    4. Double-click the imported g8e Operator CA cert."
            echo "    5. Expand Trust and set 'When using this certificate' to Always Trust."
            ;;
        windows)
            echo "  Option A — One-shot script (recommended):"
            echo ""
            echo "    Copy scripts/security/trust-ca.ps1 to your Windows machine, then"
            echo "    run it in an Administrator PowerShell prompt:"
            echo ""
            echo "    .\\trust-ca.ps1 -Server ${server_host}"
            echo ""
            echo "    The script removes any old g8e CA cert, fetches the new one"
            echo "    via SSH, and installs it — all in one step."
            echo ""
            echo "  Option B — Manual PowerShell (as Administrator):"
            echo ""
            echo "    # Remove old cert"
            echo "    Get-ChildItem Cert:\\LocalMachine\\Root | Where-Object { \$_.Subject -like '*g8e*' } | Remove-Item"
            echo ""
            echo "    # Fetch and save"
            echo "    ssh ${server_host} \"docker exec g8ep cat /vsodb/ssl/ca.crt\" | Out-File -Encoding ascii \$env:USERPROFILE\\Downloads\\g8e-ca.crt"
            echo ""
            echo "    # Import"
            echo "    Import-Certificate -FilePath \"\$env:USERPROFILE\\Downloads\\g8e-ca.crt\" -CertStoreLocation Cert:\\LocalMachine\\Root"
            echo ""
            echo "  Option C — GUI:"
            echo "    1. Run the Option B fetch command to save the file."
            echo "    2. Double-click g8e-ca.crt and click Install Certificate."
            echo "    3. Select Local Machine, click Next."
            echo "    4. Choose 'Place all certificates in the following store' > Browse."
            echo "    5. Select Trusted Root Certification Authorities, click Finish."
            ;;
        linux)
            echo "  Run on your Linux workstation:"
            echo ""
            echo "  Debian / Ubuntu (fetch + trust in one pipeline):"
            echo ""
            echo "    ${fetch_cmd} | sudo tee /usr/local/share/ca-certificates/g8e-ca.crt >/dev/null && sudo update-ca-certificates"
            echo ""
            echo "  RHEL / Fedora:"
            echo ""
            echo "    ${fetch_cmd} | sudo tee /etc/pki/ca-trust/source/anchors/g8e-ca.crt >/dev/null && sudo update-ca-trust"
            ;;
    esac

    echo ""
    echo "  After trusting, close all browser windows and reopen."
    echo ""
    echo "  Tip: You can also download the cert directly from the setup wizard"
    echo "  at https://localhost/setup — no terminal needed."
    echo ""
}

exec_trust() {
    local ca_file_arg=""

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --ca-file) ca_file_arg="$2"; shift 2 ;;
            *) shift ;;
        esac
    done

    local ca_path="-"
    if [[ -n "$ca_file_arg" ]]; then
        [ -f "$ca_file_arg" ] || die "CA cert not found: $ca_file_arg"
        ca_path="$ca_file_arg"
    else
        _require_running
        docker exec "$CERT_CONTAINER" test -f "${CERT_SSL_DIR}/ca.crt" 2>/dev/null \
            || die "CA cert not found at ${CERT_SSL_DIR}/ca.crt in ${CERT_CONTAINER}. Has the platform started and generated certs? Run: ./g8e security certs generate"
    fi

    local is_remote=false
    if [[ -n "${SSH_CLIENT:-}" || -n "${SSH_TTY:-}" || -n "${SSH_CONNECTION:-}" ]]; then
        is_remote=true
    fi

    if [[ "$is_remote" == "false" ]]; then
        local os
        os="$(uname -s)"
        case "$os" in
            Darwin) _trust_local_macos "$ca_path" ;;
            Linux)  _trust_local_linux "$ca_path" ;;
            *)      die "Unsupported OS: $os — see 'manage-ssl.sh status' for manual instructions." ;;
        esac
        return
    fi

    echo "  Detected: remote SSH session."
    echo ""
    echo "  The CA cert lives on this server. To trust it in your browser,"
    echo "  you need to install it on the machine where your browser runs."
    echo ""

    local server_host
    server_host="$(hostname 2>/dev/null || echo "your-server")"

    printf "  Trust on this server too? [y/N] "
    read -r _trust_server
    if [[ "$_trust_server" == "y" || "$_trust_server" == "Y" ]]; then
        local os
        os="$(uname -s)"
        case "$os" in
            Darwin) _trust_local_macos "$ca_path" ;;
            Linux)  _trust_local_linux "$ca_path" ;;
            *)      warn "Unsupported server OS: $os — skipping server-side trust." ;;
        esac
        echo ""
    fi

    echo "  What OS is your workstation (where your browser runs)?"
    echo ""
    echo "    1) macOS"
    echo "    2) Windows"
    echo "    3) Linux"
    echo ""
    printf "  Choice [1/2/3]: "
    read -r _ws_choice

    local ws_os
    case "$_ws_choice" in
        1|macos|mac|darwin)   ws_os="macos"   ;;
        2|windows|win)        ws_os="windows" ;;
        3|linux)              ws_os="linux"   ;;
        *)
            echo ""
            echo "  Unrecognised choice — showing instructions for all platforms."
            _print_remote_instructions "$server_host" "macos"
            _print_remote_instructions "$server_host" "windows"
            _print_remote_instructions "$server_host" "linux"
            return
            ;;
    esac

    _print_remote_instructions "$server_host" "$ws_os"
}

# -----------------------------------------------------------------------------
# Argument parsing
# -----------------------------------------------------------------------------

COMMAND=""
REMAINING=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        generate|rotate|status|trust)
            COMMAND="$1"
            shift
            REMAINING=("$@")
            break
            ;;
        -h|--help)
            sed -n '/^# Usage:/,/^# ====*/{ /^# ===*/d; s/^# \{0,3\}//; p }' "$0"
            exit 0
            ;;
        *)
            die "Unknown command: $1. Run with --help for usage."
            ;;
    esac
done

[[ -n "$COMMAND" ]] || { sed -n '/^# Usage:/,/^# ====*/{ /^# ===*/d; s/^# \{0,3\}//; p }' "$0"; exit 1; }

case "$COMMAND" in
    generate) exec_generate ;;
    rotate)   exec_rotate   ;;
    status)   exec_status   ;;
    trust)    exec_trust "${REMAINING[@]:-}" ;;
esac
