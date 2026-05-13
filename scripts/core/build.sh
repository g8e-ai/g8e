#!/bin/bash
# Platform lifecycle management for the local g8e environment.
#
# Service categories:
#   Substrate: Operator listen mode (runs as local operator binary)
#   Optional application layer: g8ee, g8ed (explicit opt-in only)
#   Data volumes:
#     .g8e/data     (Operator listen mode -- SQLite DB, users, settings; wiped by reset)
#     .g8e/pki      (Operator listen mode -- TLS/PKI material; preserved by reset and wipe)
#     .g8e/secrets  (Operator listen mode -- bootstrap secrets; wiped by reset, preserved by wipe)
#     g8ee-data    (g8ee   -- app data; wiped by reset)
#     g8ed-data (g8ed  -- app data; wiped by reset)
#   Excluded from reset: core data services only
#
# Prerequisites:
#   - Go available on host
#   - Node and Python available on host when optional apps are enabled
#
# Invoked via: ./g8e platform <subcommand>

set -e

_footer() {
    local rc=$?
    # Ensure any stale PID files are cleaned up if the process is actually gone
    for pid_file in "$OPERATOR_LISTEN_PID_FILE" "$G8EE_PID_FILE" "$G8ED_PID_FILE"; do
        if [ -f "$pid_file" ]; then
            local pid
            pid=$(cat "$pid_file")
            if ! ps -p "$pid" > /dev/null 2>&1; then
                rm -f "$pid_file"
            fi
        fi
    done
    [[ $rc -eq 0 ]] || return
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  build.sh done"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}
trap _footer EXIT

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  build.sh $*"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "${SCRIPT_DIR}/path_utils.sh"
PROJECT_ROOT="$G8E_PROJECT_ROOT"

G8E_RUNTIME_DIR="${G8E_RUNTIME_DIR:-$PROJECT_ROOT/.g8e}"
OPERATOR_LISTEN_DATA_DIR="${OPERATOR_LISTEN_DATA_DIR:-$G8E_RUNTIME_DIR/data}"
OPERATOR_LISTEN_PKI_DIR="${OPERATOR_LISTEN_PKI_DIR:-$G8E_RUNTIME_DIR/pki}"
OPERATOR_LISTEN_SECRETS_DIR="${OPERATOR_LISTEN_SECRETS_DIR:-$G8E_RUNTIME_DIR/secrets}"
OPERATOR_LISTEN_PID_DIR="${OPERATOR_LISTEN_PID_DIR:-$G8E_RUNTIME_DIR/pids}"
OPERATOR_LISTEN_LOG_DIR="${OPERATOR_LISTEN_LOG_DIR:-$G8E_RUNTIME_DIR/logs}"
OPERATOR_LISTEN_PID_FILE="$OPERATOR_LISTEN_PID_DIR/operator-listen.pid"
OPERATOR_LISTEN_LOG_FILE="$OPERATOR_LISTEN_LOG_DIR/operator-listen.log"
G8ED_PID_FILE="$OPERATOR_LISTEN_PID_DIR/g8ed.pid"
G8ED_LOG_FILE="$OPERATOR_LISTEN_LOG_DIR/g8ed.log"
G8EE_PID_FILE="$OPERATOR_LISTEN_PID_DIR/g8ee.pid"
G8EE_LOG_FILE="$OPERATOR_LISTEN_LOG_DIR/g8ee.log"
OPERATOR_LISTEN_HTTP_PORT="${OPERATOR_LISTEN_HTTP_PORT:-9000}"
OPERATOR_LISTEN_WSS_PORT="${OPERATOR_LISTEN_WSS_PORT:-9001}"
OPERATOR_LISTEN_BOOTSTRAP_PORT="${OPERATOR_LISTEN_BOOTSTRAP_PORT:-8080}"
OPERATOR_LISTEN_LOG_MAX_BACKUPS=5

DEV_MODE=false

MANAGED_SERVICES=(operator)
OPTIONAL_APPS=(g8ee g8ed)
TEST_RUNNER_SERVICES=()
WITH_APPS=false
OPTIONAL_COMPONENTS=()

if [[ "${G8E_WITH_APPS:-}" == "1" || "${G8E_WITH_APPS:-}" == "true" ]]; then
    WITH_APPS=true
    OPTIONAL_COMPONENTS=("${OPTIONAL_APPS[@]}")
fi

_service_volume() {
    case "$1" in
        g8ee)   echo "g8ee-data" ;;
        g8ed) echo "g8ed-data" ;;
    esac
}

_unique_components() {
    local seen=""
    local item
    for item in "$@"; do
        [[ -z "$item" ]] && continue
        if [[ " $seen " != *" $item "* ]]; then
            printf '%s\n' "$item"
            seen+=" $item"
        fi
    done
}

_expand_components() {
    local default_to_operator="$1"
    shift
    local components=("$@")
    if [[ ${#components[@]} -eq 0 && "$default_to_operator" == "true" ]]; then
        components=(operator)
    fi
    if [[ ${#OPTIONAL_COMPONENTS[@]} -gt 0 ]]; then
        components+=("${OPTIONAL_COMPONENTS[@]}")
    fi
    mapfile -t components < <(_unique_components "${components[@]}")
    printf '%s\n' "${components[@]}"
}

_start_optional_app() {
    case "$1" in
        g8ee) _start_g8ee ;;
        g8ed) _start_g8ed ;;
        *)
            echo "Error: Unknown optional app '$1'" >&2
            exit 1
            ;;
    esac
}

_stop_optional_app() {
    case "$1" in
        g8ee) _stop_g8ee ;;
        g8ed) _stop_g8ed ;;
        *)
            echo "Error: Unknown optional app '$1'" >&2
            exit 1
            ;;
    esac
}

_wait_optional_app_healthy() {
    case "$1" in
        g8ee) _wait_service_healthy "g8ee" "https://localhost:8443/health" 10 1 "$G8EE_LOG_FILE" ;;
        g8ed) _wait_service_healthy "g8ed" "https://localhost/health" 10 1 "$G8ED_LOG_FILE" ;;
    esac
}

# PKI volume is never wiped — preserved across reset, wipe, and rebuild.
PKI_VOLUME="$OPERATOR_LISTEN_PKI_DIR"
SECRETS_VOLUME="$OPERATOR_LISTEN_SECRETS_DIR"

usage() {
    cat <<EOF
Usage: $(basename "$0") <command> [options]

Commands:
  status                          Show substrate and optional app process status
  up [component ...] [--with-apps] Start Operator listen mode by default
                                  Default (no components): operator
                                  Valid: operator g8ee g8ed
                                  Optional apps require --with-apps, --with-g8ed, or --with-g8ee
  down                            Stop Operator listen mode and optional apps -- nothing is removed
  rebuild [component ...]         Restart Operator listen mode by default
                                  Default (no components): operator
                                  Valid: operator g8ee g8ed
                                  Optional apps require --with-apps, --with-g8ed, or --with-g8ee
  reset                           Wipe Operator listen-mode data. PKI certs and secrets are preserved.
  wipe                            Clear app data from the Operator database
                                  Operator listen mode stays up; preserves: platform settings, PKI certs, secrets, auth token
  clean                           Nuke runtime processes and data.
  operator-build                  Build linux/amd64 operator binary natively
  operator-build-all              Build all operator architectures natively

Examples:
  $(basename "$0") status                       Show host process status and versions
  $(basename "$0") up                           Start Operator listen mode
  $(basename "$0") up --with-apps               Start Operator plus optional bundled apps
  $(basename "$0") down                         Stop runtime processes
  $(basename "$0") rebuild                      Restart Operator listen mode
  $(basename "$0") rebuild --with-g8ed          Restart Operator plus g8ed
  $(basename "$0") wipe                         Clear app data from the Operator database
  $(basename "$0") reset                        Wipe Operator listen-mode data
  $(basename "$0") clean                        Remove host runtime state
EOF
}

COMMAND=""
REBUILD_COMPONENTS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            usage
            exit 0
            ;;
        --dev)
            DEV_MODE=true
            shift
            ;;
        --with-apps)
            WITH_APPS=true
            OPTIONAL_COMPONENTS=("${OPTIONAL_APPS[@]}")
            shift
            ;;
        --with-g8ed)
            WITH_APPS=true
            OPTIONAL_COMPONENTS+=("g8ed")
            shift
            ;;
        --with-g8ee)
            WITH_APPS=true
            OPTIONAL_COMPONENTS+=("g8ee")
            shift
            ;;
        setup|up|down|restart|reset|wipe|clean|status|operator-build|operator-build-all)
            COMMAND="$1"
            shift
            while [[ $# -gt 0 && ! "$1" =~ ^- ]]; do
                if ! printf '%s\n' operator g8ee g8ed | grep -qx "$1"; then
                    echo "Error: Invalid component '$1'" >&2
                    echo "Valid: operator g8ee g8ed" >&2
                    exit 1
                fi
                REBUILD_COMPONENTS+=("$1")
                shift
            done
            ;;
        rebuild)
            COMMAND="rebuild"
            shift
            while [[ $# -gt 0 && ! "$1" =~ ^- ]]; do
                if ! printf '%s\n' operator g8ee g8ed | grep -qx "$1"; then
                    echo "Error: Invalid component '$1'" >&2
                    echo "Valid: operator g8ee g8ed" >&2
                    exit 1
                fi
                REBUILD_COMPONENTS+=("$1")
                shift
            done
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage >&2
            exit 1
            ;;
    esac
done

if [[ "$WITH_APPS" != "true" ]]; then
    for component in "${REBUILD_COMPONENTS[@]}"; do
        if [[ "$component" != "operator" ]]; then
            echo "Error: optional app '$component' requires --with-apps, --with-$component, or ./g8e apps start $component" >&2
            exit 1
        fi
    done
fi

# ─── Helpers ──────────────────────────────────────────────────────────────────


_operator_listen_running() {
    if [ -f "$OPERATOR_LISTEN_PID_FILE" ]; then
        local pid
        pid=$(cat "$OPERATOR_LISTEN_PID_FILE")
        if ps -p "$pid" > /dev/null 2>&1; then
            return 0
        fi
        rm -f "$OPERATOR_LISTEN_PID_FILE"
    fi
    return 1
}

_g8ee_running() {
    if [ -f "$G8EE_PID_FILE" ]; then
        local pid
        pid=$(cat "$G8EE_PID_FILE")
        if ps -p "$pid" > /dev/null 2>&1; then
            return 0
        fi
        rm -f "$G8EE_PID_FILE"
    fi
    return 1
}

_g8ed_running() {
    if [ -f "$G8ED_PID_FILE" ]; then
        local pid
        pid=$(cat "$G8ED_PID_FILE")
        if ps -p "$pid" > /dev/null 2>&1; then
            return 0
        fi
        rm -f "$G8ED_PID_FILE"
    fi
    return 1
}

_rotate_logs() {
    local log_file="$1"
    if [ -f "$log_file" ]; then
        local max_backups="$OPERATOR_LISTEN_LOG_MAX_BACKUPS"
        for i in $(seq $((max_backups - 1)) -1 1); do
            if [ -f "$log_file.$i" ]; then
                mv "$log_file.$i" "$log_file.$((i + 1))"
            fi
        done
        mv "$log_file" "$log_file.1"
    fi
}

_start_g8ee() {
    if _g8ee_running; then
        echo "  g8ee is already running (PID: $(cat "$G8EE_PID_FILE"))."
        return 0
    fi

    local venv_dir="$PROJECT_ROOT/components/g8ee/.venv"
    if [ ! -d "$venv_dir" ]; then
        echo "  Creating g8ee virtualenv..."
        python3 -m venv "$venv_dir"
        "$venv_dir/bin/pip" install --upgrade pip
        "$venv_dir/bin/pip" install -r "$PROJECT_ROOT/components/g8ee/requirements.txt"
    fi

    echo "  Starting g8ee on port 8443 (HTTPS)..."
    _rotate_logs "$G8EE_LOG_FILE"
    
    # g8ee requires internal auth token and session encryption key from bootstrap
    local auth_token
    auth_token=$(cat "$OPERATOR_LISTEN_SECRETS_DIR/internal_auth_token" 2>/dev/null | tr -d ' \n\r' || true)
    
    (
        cd "$PROJECT_ROOT/components/g8ee"
        export G8E_PKI_DIR="$OPERATOR_LISTEN_PKI_DIR"
        export G8E_SECRETS_DIR="$OPERATOR_LISTEN_SECRETS_DIR"
        export G8E_INTERNAL_AUTH_TOKEN="$auth_token"
        export PYTHONPATH="$PROJECT_ROOT/components/g8ee:$PROJECT_ROOT/shared"
        export G8E_SHARED_DIR="$PROJECT_ROOT/shared"
        export G8E_INTERNAL_HTTP_URL="https://localhost:9000"
        export G8E_INTERNAL_PUBSUB_URL="wss://localhost:9001"
        
        setsid "$venv_dir/bin/uvicorn" app.main:app --host 127.0.0.1 --port 8443 \
            --ssl-keyfile "$OPERATOR_LISTEN_PKI_DIR/server.key" \
            --ssl-certfile "$OPERATOR_LISTEN_PKI_DIR/server.crt" \
            > "$G8EE_LOG_FILE" 2>&1 &
        echo $! > "$G8EE_PID_FILE"
    )

    sleep 2
    if ! _g8ee_running; then
        echo "  Error: g8ee failed to start. See $G8EE_LOG_FILE"
        rm -f "$G8EE_PID_FILE"
        return 1
    fi
}

_ensure_node_capabilities() {
    local node_bin
    node_bin=$(which node)
    if [ -z "$node_bin" ]; then
        echo "  WARN: node binary not found in PATH"
        return 0
    fi

    # Check if node already has cap_net_bind_service
    if getcap "$node_bin" 2>/dev/null | grep -q "cap_net_bind_service"; then
        return 0
    fi

    echo "  Setting cap_net_bind_service=+ep on node binary for privileged ports (80/443)..."
    if sudo setcap cap_net_bind_service=+ep "$node_bin"; then
        echo "  Successfully set capabilities on $node_bin"
    else
        echo "  WARN: Failed to set capabilities. g8ed may not be able to bind to ports 80/443 without running as root."
        echo "  Run manually: sudo setcap cap_net_bind_service=+ep $node_bin"
    fi
}

_start_g8ed() {
    if _g8ed_running; then
        local pid_msg=""
        if [ -f "$G8ED_PID_FILE" ]; then
            pid_msg=" (PID: $(cat "$G8ED_PID_FILE"))"
        fi
        echo "  g8ed is already running${pid_msg}."
        return 0
    fi

    if [ ! -d "$PROJECT_ROOT/components/g8ed/node_modules" ]; then
        echo "  Installing g8ed dependencies..."
        (cd "$PROJECT_ROOT/components/g8ed" && npm install)
    fi

    _ensure_node_capabilities

    echo "  Starting g8ed on port 443 (HTTPS) and 80 (HTTP)..."
    _rotate_logs "$G8ED_LOG_FILE"

    (
        cd "$PROJECT_ROOT/components/g8ed"
        export G8E_PKI_DIR="$OPERATOR_LISTEN_PKI_DIR"
        export G8E_SECRETS_DIR="$OPERATOR_LISTEN_SECRETS_DIR"
        export NODE_EXTRA_CA_CERTS="$OPERATOR_LISTEN_PKI_DIR/ca.crt"
        export G8E_INTERNAL_HTTP_URL="https://localhost:9000"
        export G8E_INTERNAL_PUBSUB_URL="wss://localhost:9001"
        export G8EE_INTERNAL_URL="https://localhost:8443"
        
        setsid node server.js > "$G8ED_LOG_FILE" 2>&1 &
        echo $! > "$G8ED_PID_FILE"
    )

    sleep 2
    if ! _g8ed_running; then
        echo "  Error: g8ed failed to start. See $G8ED_LOG_FILE"
        rm -f "$G8ED_PID_FILE"
        return 1
    fi
}

_stop_g8ee() {
    if [ -f "$G8EE_PID_FILE" ]; then
        local pid=$(cat "$G8EE_PID_FILE")
        echo "  Stopping g8ee (PID: $pid)..."
        kill "$pid" 2>/dev/null || true
        rm -f "$G8EE_PID_FILE"
    fi
}

_stop_g8ed() {
    if [ -f "$G8ED_PID_FILE" ]; then
        local pid=$(cat "$G8ED_PID_FILE")
        echo "  Stopping g8ed (PID: $pid)..."
        kill "$pid" 2>/dev/null || true
        sleep 1
        if ps -p "$pid" > /dev/null 2>&1; then
            kill -9 "$pid" 2>/dev/null || true
        fi
        rm -f "$G8ED_PID_FILE"
    fi
}

_start_operator_listen() {
    if _operator_listen_running; then
        echo "  Operator listen mode is already running (PID: $(cat "$OPERATOR_LISTEN_PID_FILE"))."
        return 0
    fi

    local bin="$PROJECT_ROOT/components/g8eo/build/linux-amd64/g8e.operator"
    local needs_build=false

    if [ ! -f "$bin" ]; then
        echo "  Operator binary not found at $bin. Building it..."
        needs_build=true
    else
        # Check if source files are newer than the binary
        local source_files
        source_files=$(find "$PROJECT_ROOT/components/g8eo" -type f \( -name "*.go" -o -name "Makefile" -o -name "go.mod" -o -name "go.sum" \) -not -path "*/vendor/*" -not -path "*/build/*")
        for f in $source_files; do
            if [ "$f" -nt "$bin" ]; then
                echo "  Operator source changed: $f is newer than binary. Rebuilding..."
                needs_build=true
                break
            fi
        done
    fi

    if [ "$needs_build" = true ]; then
        echo "  Building Operator binary natively..."
        (cd "$PROJECT_ROOT/components/g8eo" && make build-local)
    fi

    echo "  Starting Operator listen mode on port $OPERATOR_LISTEN_HTTP_PORT..."
    mkdir -p "$OPERATOR_LISTEN_DATA_DIR" "$OPERATOR_LISTEN_PKI_DIR" "$OPERATOR_LISTEN_SECRETS_DIR" "$OPERATOR_LISTEN_PID_DIR" "$OPERATOR_LISTEN_LOG_DIR"

    _rotate_logs "$OPERATOR_LISTEN_LOG_FILE"

    local auth_token
    auth_token=$(cat "$OPERATOR_LISTEN_SECRETS_DIR/internal_auth_token" 2>/dev/null | tr -d ' \n\r' || true)

    export G8E_INTERNAL_AUTH_TOKEN="$auth_token"
    export G8E_PKI_DIR="$OPERATOR_LISTEN_PKI_DIR"
    export G8E_SECRETS_DIR="$OPERATOR_LISTEN_SECRETS_DIR"

    setsid "$bin" --listen \
        --data-dir "$OPERATOR_LISTEN_DATA_DIR" \
        --pki-dir "$OPERATOR_LISTEN_PKI_DIR" \
        --secrets-dir "$OPERATOR_LISTEN_SECRETS_DIR" \
        --http-listen-port "$OPERATOR_LISTEN_HTTP_PORT" \
        --wss-listen-port "$OPERATOR_LISTEN_WSS_PORT" \
        > "$OPERATOR_LISTEN_LOG_FILE" 2>&1 &

    local pid=$!
    echo "$pid" > "$OPERATOR_LISTEN_PID_FILE"

    sleep 2
    if ! _operator_listen_running; then
        echo "  Error: Operator listen mode failed to start. See $OPERATOR_LISTEN_LOG_FILE"
        rm -f "$OPERATOR_LISTEN_PID_FILE"
        return 1
    fi
}

_stop_operator_listen() {
    local pid=""
    
    if [ -f "$OPERATOR_LISTEN_PID_FILE" ]; then
        pid=$(cat "$OPERATOR_LISTEN_PID_FILE")
    fi
    
    if [ -n "$pid" ] && ps -p "$pid" > /dev/null 2>&1; then
        echo "  Stopping Operator listen mode (PID: $pid)..."
        kill "$pid" 2>/dev/null || true
        local waited=0
        while ps -p "$pid" > /dev/null 2>&1 && [ $waited -lt 10 ]; do
            sleep 1
            waited=$((waited + 1))
        done
        if ps -p "$pid" > /dev/null 2>&1; then
            echo "  Force stopping Operator listen mode..."
            kill -9 "$pid" 2>/dev/null || true
        fi
        rm -f "$OPERATOR_LISTEN_PID_FILE"
    else
        local found_pid
        found_pid=$(pgrep -f "g8e.operator --listen" | head -1)
        if [ -n "$found_pid" ]; then
            echo "  Stopping Operator listen mode (PID: $found_pid, found via pgrep)..."
            kill "$found_pid" 2>/dev/null || true
            local waited=0
            while ps -p "$found_pid" > /dev/null 2>&1 && [ $waited -lt 10 ]; do
                sleep 1
                waited=$((waited + 1))
            done
            if ps -p "$found_pid" > /dev/null 2>&1; then
                echo "  Force stopping Operator listen mode..."
                kill -9 "$found_pid" 2>/dev/null || true
            fi
        fi
        rm -f "$OPERATOR_LISTEN_PID_FILE"
    fi
}

_sync_operator_binaries() {
    echo "  Syncing operator binaries to blob store..."
    local auth_token
    auth_token=$(cat "$OPERATOR_LISTEN_SECRETS_DIR/internal_auth_token" 2>/dev/null | tr -d ' \n\r' || true)

    if [ -z "$auth_token" ]; then
        echo "  Warning: No internal auth token found, skipping binary sync."
        return 0
    fi

    local bin_dir="$PROJECT_ROOT/components/g8eo/build"
    if [ ! -d "$bin_dir" ]; then
        echo "  No operator binaries found at $bin_dir, skipping sync."
        return 0
    fi
    local trust_bundle="${G8E_TRUST_BUNDLE:-$OPERATOR_LISTEN_PKI_DIR/trust/hub-bundle.pem}"
    if [ ! -f "$trust_bundle" ]; then
        echo "  Warning: Operator trust bundle missing at $trust_bundle, skipping binary sync."
        return 0
    fi

    for arch in amd64 arm64 386; do
        local bin_path="$bin_dir/linux-$arch/g8e.operator"
        if [ -f "$bin_path" ]; then
            echo "    Uploading linux/$arch..."
            curl -sf -o /dev/null \
                -X PUT \
                --cacert "$trust_bundle" \
                -H 'Content-Type: application/octet-stream' \
                -H "X-Internal-Auth: $auth_token" \
                --data-binary "@$bin_path" \
                "https://localhost:$OPERATOR_LISTEN_HTTP_PORT/api/internal/blob/operator-binary/linux-$arch" || echo "      Warning: Failed to upload linux/$arch"
        fi
    done
}

_wait_operator_listen_healthy() {
    local url="$1" timeout_s="$2" interval="${3:-1}"
    local waited=0
    local trust_bundle="${G8E_TRUST_BUNDLE:-$OPERATOR_LISTEN_PKI_DIR/trust/hub-bundle.pem}"
    echo "  Operator listen mode: waiting for $url..."

    until [[ -f "$trust_bundle" ]] && curl -sf --cacert "$trust_bundle" "$url" >/dev/null 2>&1; do
        if (( waited >= timeout_s )); then
            echo -e "  Operator listen mode: \033[0;31mTIMEOUT\033[0m"
            echo "  Operator listen mode did not become healthy within ${timeout_s}s. See $OPERATOR_LISTEN_LOG_FILE"
            tail -n 20 "$OPERATOR_LISTEN_LOG_FILE"
            exit 1
        fi
        sleep "$interval"
        waited=$(( waited + interval ))
    done
    echo -e "  Operator listen mode: \033[0;32mready\033[0m (${waited}s)"
}

_wait_service_healthy() {
    local service_name="$1" url="$2" timeout_s="$3" interval="${4:-1}" log_file="$5"
    local waited=0
    echo "  $service_name: waiting for healthy status..."

    until curl -sfk "$url" >/dev/null 2>&1; do
        if (( waited >= timeout_s )); then
            echo -e "  $service_name: \033[0;31mTIMEOUT\033[0m"
            echo "  $service_name did not become healthy within ${timeout_s}s. See $log_file"
            tail -n 20 "$log_file"
            exit 1
        fi
        sleep "$interval"
        waited=$(( waited + interval ))
    done
    echo -e "  $service_name: \033[0;32mready\033[0m (${waited}s)"
}

_load_env() {
    if [[ -z "${G8E_VERSION:-}" ]]; then
        G8E_VERSION="$(cat "$PROJECT_ROOT/VERSION" 2>/dev/null | tr -d '[:space:]' || echo 'dev')"
        export G8E_VERSION
    fi
}

_preflight() {
    _load_env

    HOST_IPS=""
    if command -v ip >/dev/null 2>&1; then
        HOST_IPS=$(ip -4 addr show scope global | awk '/inet / {split($2,a,"/"); print a[1]}' | grep -v '^172\.' | tr '\n' ',' | sed 's/,$//')
    elif command -v ifconfig >/dev/null 2>&1; then
        HOST_IPS=$(ifconfig | awk '/inet / && !/127\.0\.0\.1/ {print $2}' | sed 's/addr://' | grep -v '^172\.' | tr '\n' ',' | sed 's/,$//')
    fi
    export HOST_IPS
}

# ─── Startup ──────────────────────────────────────────────────────────────────

_load_env

_print_platform_info() {
    echo "  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  g8e Operator/protocol substrate ready"
    echo "  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "  Operator HTTP API: https://localhost:$OPERATOR_LISTEN_HTTP_PORT"
    echo "  Operator Pub/Sub:  wss://localhost:$OPERATOR_LISTEN_WSS_PORT"
    echo "  Runtime dir:       $G8E_RUNTIME_DIR"
    echo "  Protocol schemas:  shared/proto"
    if _g8ee_running || _g8ed_running; then
        echo "  Optional apps:     running (see ./g8e platform status)"
    else
        echo "  Optional apps:     not running (use ./g8e apps start to enable)"
    fi
}

if [[ -z "$COMMAND" ]]; then
    echo "Error: no command specified." >&2
    usage >&2
    exit 1
fi

cd "$PROJECT_ROOT"

# ─── status ───────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "status" ]]; then
    _VER="$(cat "$PROJECT_ROOT/VERSION" 2>/dev/null | tr -d '[:space:]' \
        || git -C "$PROJECT_ROOT" describe --tags --abbrev=0 2>/dev/null \
        || echo 'unknown')"
    [[ "$_VER" != v* ]] && _VER="v$_VER"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Substrate Status"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    if _operator_listen_running; then
        printf "  %-14s  %-10s  (PID: %s)\n" "operator" "RUNNING" "$(cat "$OPERATOR_LISTEN_PID_FILE")"
    else
        printf "  %-14s  %-10s\n" "operator" "STOPPED"
    fi

    echo ""
    echo "Optional Application Layer"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    if _g8ed_running; then
        printf "  %-14s  %-10s  (PID: %s)\n" "g8ed" "RUNNING" "$(cat "$G8ED_PID_FILE")"
    else
        printf "  %-14s  %-10s\n" "g8ed" "NOT RUNNING"
    fi

    if _g8ee_running; then
        printf "  %-14s  %-10s  (PID: %s)\n" "g8ee" "RUNNING" "$(cat "$G8EE_PID_FILE")"
    else
        printf "  %-14s  %-10s\n" "g8ee" "NOT RUNNING"
    fi

    echo ""
    exit 0
fi

# ─── down ─────────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "down" ]]; then
    echo "Stopping Operator listen mode and optional application-layer services..."
    _stop_g8ee
    _stop_g8ed
    _stop_operator_listen
    echo "Done."
    exit 0
fi

# ─── restart ──────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "restart" ]]; then
    _preflight
    mapfile -t RESTART_COMPONENTS < <(_expand_components true "${REBUILD_COMPONENTS[@]}")
    echo "Restarting substrate components..."
    if printf '%s\n' "${RESTART_COMPONENTS[@]}" | grep -qx g8ee; then
        _stop_g8ee
    fi
    if printf '%s\n' "${RESTART_COMPONENTS[@]}" | grep -qx g8ed; then
        _stop_g8ed
    fi
    _stop_operator_listen
    _start_operator_listen
    for svc in "${RESTART_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _start_optional_app "$svc"
    done
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$OPERATOR_LISTEN_BOOTSTRAP_PORT/health" 60 1
    _sync_operator_binaries
    for svc in "${RESTART_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _wait_optional_app_healthy "$svc"
    done

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Restart complete."
    echo ""
    _print_platform_info
    exit 0
fi

# ─── reset ───────────────────────────────────────────────────────────────────
# Wipes DB data volumes and bootstrap secrets. PKI certs are preserved.
# Use 'clean' to remove everything including PKI.

if [[ "$COMMAND" == "reset" ]]; then
    mapfile -t RESET_COMPONENTS < <(_expand_components true "${REBUILD_COMPONENTS[@]}")

    echo "Wiping Operator listen-mode data and secrets — PKI certs preserved..."
    _stop_g8ee
    _stop_g8ed
    _stop_operator_listen
    
    # Wipe host data
    rm -rf "$OPERATOR_LISTEN_DATA_DIR/"* 2>/dev/null || true
    rm -rf "$OPERATOR_LISTEN_SECRETS_DIR/"* 2>/dev/null || true
    rm -rf "$PROJECT_ROOT/components/g8ee/data/"* 2>/dev/null || true
    rm -rf "$PROJECT_ROOT/components/g8ed/data/"* 2>/dev/null || true

    echo ""

    _preflight

    echo "Starting substrate services..."
    _start_operator_listen
    for svc in "${RESET_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _start_optional_app "$svc"
    done
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$OPERATOR_LISTEN_BOOTSTRAP_PORT/health" 300 2
    _sync_operator_binaries

    for svc in "${RESET_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _wait_optional_app_healthy "$svc"
    done

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Reset complete. PKI certs preserved; bootstrap secrets were recreated."
    echo ""
    _print_platform_info
    exit 0
fi

# ─── wipe ─────────────────────────────────────────────────────────────────────
# Clears all app data from the Operator listen-mode database via the HTTP API.
# Preserves: platform settings (components collection), PKI certs, secrets, auth token, LLM data.
# Operator listen mode is restarted to flush in-memory state; no volume wipe, no rebuild.
# Use 'reset' to wipe DB data volumes and rebuild from scratch (PKI still preserved).

if [[ "$COMMAND" == "wipe" ]]; then
    _preflight
    mapfile -t WIPE_COMPONENTS < <(_expand_components true "${REBUILD_COMPONENTS[@]}")

    echo "Stopping Operator listen mode and selected optional apps..."
    if printf '%s\n' "${WIPE_COMPONENTS[@]}" | grep -qx g8ee; then
        _stop_g8ee
    fi
    if printf '%s\n' "${WIPE_COMPONENTS[@]}" | grep -qx g8ed; then
        _stop_g8ed
    fi
    _stop_operator_listen
    echo ""

    echo "Restarting Operator listen mode..."
    _start_operator_listen
    _wait_operator_listen_healthy "https://localhost:$OPERATOR_LISTEN_BOOTSTRAP_PORT/health" 120 2
    _sync_operator_binaries

    echo "Clearing app data from Operator listen mode..."
    _wipe_trust_bundle="${G8E_TRUST_BUNDLE:-$OPERATOR_LISTEN_PKI_DIR/trust/hub-bundle.pem}"
    curl -sf --cacert "$_wipe_trust_bundle" -X POST -H "X-Internal-Auth: $(cat "$OPERATOR_LISTEN_SECRETS_DIR/internal_auth_token" 2>/dev/null | tr -d ' \n\r')" \
        "https://localhost:$OPERATOR_LISTEN_HTTP_PORT/api/internal/store/wipe" || echo "  Warning: wipe endpoint failed"
    echo ""

    for svc in "${WIPE_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _start_optional_app "$svc"
    done
    echo ""
    echo "Waiting for services..."
    for svc in "${WIPE_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _wait_optional_app_healthy "$svc"
    done

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Wipe complete. Platform settings, PKI certs, secrets, and auth token preserved."
    echo ""
    _print_platform_info
    exit 0
fi

# ─── clean ────────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "clean" ]]; then
    echo "Cleaning all host services and runtime data..."

    _stop_g8ee
    _stop_g8ed
    _stop_operator_listen
    rm -rf "$G8E_RUNTIME_DIR" 2>/dev/null || true
    
    echo "Done."
    exit 0
fi

# ─── Preflight (up and rebuild) ───────────────────────────────────────────────

_preflight

# ─── up ───────────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "up" ]]; then
    mapfile -t UP_COMPONENTS < <(_expand_components true "${REBUILD_COMPONENTS[@]}")
    
    if printf '%s\n' "${UP_COMPONENTS[@]}" | grep -qx operator; then
        _start_operator_listen
        UP_COMPONENTS=($(printf '%s\n' "${UP_COMPONENTS[@]}" | grep -vx operator || true))
    fi

    if [[ ${#UP_COMPONENTS[@]} -gt 0 ]]; then
        for svc in "${UP_COMPONENTS[@]}"; do
            case "$svc" in
                g8ee) _start_g8ee ;;
                g8ed) _start_g8ed ;;
                *) _start_optional_app "$svc" ;;
            esac
        done
    fi
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$OPERATOR_LISTEN_BOOTSTRAP_PORT/health" 60 1
    _sync_operator_binaries
    
    for svc in "${UP_COMPONENTS[@]}"; do
        _wait_optional_app_healthy "$svc"
    done
    
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Environment ready."
    echo ""
    _print_platform_info
    exit 0
fi

# ─── setup ───────────────────────────────────────────────────────────────────
# Full first-time setup, then start the platform.
# Does NOT wipe data volumes — safe to run on an existing installation.
# Operator binary builds provide the listen-mode and remote Operator artifacts.

if [[ "$COMMAND" == "setup" ]]; then
    mapfile -t SETUP_COMPONENTS < <(_expand_components true "${REBUILD_COMPONENTS[@]}")
    echo "Stopping all runtime services..."
    _stop_g8ee
    _stop_g8ed
    _stop_operator_listen

    echo "Starting substrate services..."
    _start_operator_listen
    for svc in "${SETUP_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _start_optional_app "$svc"
    done
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$OPERATOR_LISTEN_BOOTSTRAP_PORT/health" 300 2
    _sync_operator_binaries

    for svc in "${SETUP_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _wait_optional_app_healthy "$svc"
    done

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Setup complete."
    echo ""
    _print_platform_info
    exit 0
fi

# ─── rebuild ──────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "rebuild" ]]; then
    mapfile -t REBUILD_COMPONENTS < <(_expand_components true "${REBUILD_COMPONENTS[@]}")

    if printf '%s\n' "${REBUILD_COMPONENTS[@]}" | grep -qx operator; then
        _stop_operator_listen
        _start_operator_listen
        REBUILD_COMPONENTS=($(printf '%s\n' "${REBUILD_COMPONENTS[@]}" | grep -vx operator || true))
    fi

    if [[ ${#REBUILD_COMPONENTS[@]} -gt 0 ]]; then
        for svc in "${REBUILD_COMPONENTS[@]}"; do
            case "$svc" in
                g8ee)
                    _stop_g8ee
                    _start_g8ee
                    ;;
                g8ed)
                    _stop_g8ed
                    _start_g8ed
                    ;;
                *) _start_optional_app "$svc" ;;
            esac
        done
    fi
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$OPERATOR_LISTEN_BOOTSTRAP_PORT/health" 300 2
    _sync_operator_binaries

    for svc in "${REBUILD_COMPONENTS[@]}"; do
        _wait_optional_app_healthy "$svc"
    done

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Rebuild complete."
    echo ""
    _print_platform_info
    exit 0
fi

# ─── operator-build ─────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "operator-build" ]]; then
    echo "Building linux/amd64 operator binary natively..."
    (cd "$PROJECT_ROOT/components/g8eo" && make build-local)
    echo ""
    echo "Operator binary built."
    exit 0
fi

# ─── operator-build-all ─────────────────────────────────────────────────────────

if [[ "$COMMAND" == "operator-build-all" ]]; then
    echo "Building all operator architectures natively..."
    (cd "$PROJECT_ROOT/components/g8eo" && make build-local-all)
    echo ""
    echo "All operator binaries built."
    exit 0
fi
