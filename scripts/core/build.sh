#!/bin/bash
# Platform lifecycle management for the local g8e environment.
#
# Service categories:
#   Managed:  g8ee, g8ed  (in scope for up/rebuild/clean)
#   Host:     Operator listen mode (runs as local operator binary)
#   Data volumes:
#     .g8e/data     (Operator listen mode -- SQLite DB, users, settings; wiped by reset)
#     .g8e/ssl      (Operator listen mode -- TLS certs; NEVER wiped by reset or wipe)
#     g8ee-data    (g8ee   -- app data; wiped by reset)
#     g8ed-data (g8ed  -- app data; wiped by reset)
#   Excluded from reset: core data services only
#
# Prerequisites:
#   - Go, Node, Python available on host
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
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

G8E_RUNTIME_DIR="${G8E_RUNTIME_DIR:-$PROJECT_ROOT/.g8e}"
OPERATOR_LISTEN_DATA_DIR="${OPERATOR_LISTEN_DATA_DIR:-$G8E_RUNTIME_DIR/data}"
OPERATOR_LISTEN_SSL_DIR="${OPERATOR_LISTEN_SSL_DIR:-$G8E_RUNTIME_DIR/ssl}"
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
OPERATOR_LISTEN_LOG_MAX_BACKUPS=5

DEV_MODE=false

MANAGED_SERVICES=(g8ee g8ed)
TEST_RUNNER_SERVICES=()

_service_volume() {
    case "$1" in
        g8ee)   echo "g8ee-data" ;;
        g8ed) echo "g8ed-data" ;;
    esac
}

# SSL volume is never wiped — preserved across reset, wipe, and rebuild.
SSL_VOLUME="$OPERATOR_LISTEN_SSL_DIR"

usage() {
    cat <<EOF
Usage: $(basename "$0") <command> [options]

Commands:
  status                          Show host process status and component versions
  up [component ...]              Start managed services and Operator listen mode
                                  Default (no components): g8ee g8ed
                                  Valid: operator g8ee g8ed
  down                            Stop managed services and Operator listen mode -- nothing is removed
  rebuild [component ...]         Restart managed services
                                  Default (no components): g8ee g8ed
                                  Valid: operator g8ee g8ed
  reset                           Wipe managed service data. SSL certs are preserved.
                                  Removes: g8ee, g8ed data and Operator listen-mode data; SSL certs preserved
  wipe                            Clear app data from the database (all collections except platform settings)
                                  Operator listen mode stays up; preserves: platform settings, SSL certs, auth token
  clean                           Nuke all managed processes and data.
  operator-build                  Build linux/amd64 operator binary natively
  operator-build-all              Build all operator architectures natively

Examples:
  $(basename "$0") status                       Show host process status and versions
  $(basename "$0") up                           Start the environment (no build)
  $(basename "$0") up g8ee                      Start only g8ee
  $(basename "$0") down                         Stop managed services and host services
  $(basename "$0") rebuild                      Restart g8ee and g8ed
  $(basename "$0") rebuild g8ee g8ed            Restart g8ee and g8ed only
  $(basename "$0") rebuild g8ee                 Restart g8ee
  $(basename "$0") wipe                         Clear app data from the database; restart g8ee/g8ed
  $(basename "$0") reset                        Wipe managed service and Operator listen-mode data
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
    # Also check for orphaned node server.js processes
    if pgrep -f "node server.js" > /dev/null 2>&1; then
        return 0
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
    auth_token=$(cat "$OPERATOR_LISTEN_SSL_DIR/internal_auth_token" 2>/dev/null | tr -d ' \n\r' || true)
    
    (
        cd "$PROJECT_ROOT/components/g8ee"
        export G8E_SSL_DIR="$OPERATOR_LISTEN_SSL_DIR"
        export G8E_INTERNAL_AUTH_TOKEN="$auth_token"
        export PYTHONPATH="$PROJECT_ROOT/components/g8ee:$PROJECT_ROOT/shared"
        export G8E_SHARED_DIR="$PROJECT_ROOT/shared"
        export G8E_INTERNAL_HTTP_URL="https://localhost:9000"
        export G8E_INTERNAL_PUBSUB_URL="wss://localhost:9001"
        
        setsid "$venv_dir/bin/uvicorn" app.main:app --host 127.0.0.1 --port 8443 \
            --ssl-keyfile "$OPERATOR_LISTEN_SSL_DIR/server.key" \
            --ssl-certfile "$OPERATOR_LISTEN_SSL_DIR/server.crt" \
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
        export G8E_SSL_DIR="$OPERATOR_LISTEN_SSL_DIR"
        export NODE_EXTRA_CA_CERTS="$OPERATOR_LISTEN_SSL_DIR/ca.crt"
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
        rm -f "$G8ED_PID_FILE"
    else
        if _g8ed_running; then
            echo "  Stopping g8ed via pkill..."
            pkill -f "node server.js" || true
            rm -f "$G8ED_PID_FILE"
        fi
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
    mkdir -p "$OPERATOR_LISTEN_DATA_DIR" "$OPERATOR_LISTEN_SSL_DIR" "$OPERATOR_LISTEN_PID_DIR" "$OPERATOR_LISTEN_LOG_DIR"

    _rotate_logs "$OPERATOR_LISTEN_LOG_FILE"

    local auth_token
    auth_token=$(cat "$OPERATOR_LISTEN_SSL_DIR/internal_auth_token" 2>/dev/null | tr -d ' \n\r' || true)

    export G8E_INTERNAL_AUTH_TOKEN="$auth_token"
    export G8E_SSL_DIR="$OPERATOR_LISTEN_SSL_DIR"

    setsid "$bin" --listen \
        --data-dir "$OPERATOR_LISTEN_DATA_DIR" \
        --ssl-dir "$OPERATOR_LISTEN_SSL_DIR" \
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
    auth_token=$(cat "$OPERATOR_LISTEN_SSL_DIR/internal_auth_token" 2>/dev/null | tr -d ' \n\r' || true)

    if [ -z "$auth_token" ]; then
        echo "  Warning: No internal auth token found, skipping binary sync."
        return 0
    fi

    local bin_dir="$PROJECT_ROOT/components/g8eo/build"
    if [ ! -d "$bin_dir" ]; then
        echo "  No operator binaries found at $bin_dir, skipping sync."
        return 0
    fi

    for arch in amd64 arm64 386; do
        local bin_path="$bin_dir/linux-$arch/g8e.operator"
        if [ -f "$bin_path" ]; then
            echo "    Uploading linux/$arch..."
            curl -sf -o /dev/null \
                -X PUT \
                --cacert "$OPERATOR_LISTEN_SSL_DIR/ca.crt" \
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
    echo "  Operator listen mode: waiting for $url..."
    
    until curl -sfk "$url" >/dev/null 2>&1; do
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
    local _https_port="${HTTPS_PORT:-443}"
    local _app_url="${APP_URL:-}"
    local _dashboard_url
    local _ssl_setup_url
    local _host_ip
    local _primary_ip

    if [[ -n "$_app_url" ]]; then
        _dashboard_url="$_app_url"
        _ssl_setup_url="${_app_url}/trust"
    elif [[ "$_https_port" == "443" ]]; then
        _dashboard_url="https://localhost"
        _ssl_setup_url="http://localhost"
    else
        _dashboard_url="https://localhost"
        _ssl_setup_url="http://localhost"
    fi

    if [[ -n "$HOST_IPS" ]]; then
        IFS=',' read -ra IPS <<< "$HOST_IPS"
        _primary_ip="${IPS[0]}"
    else
        _primary_ip="127.0.0.1"
    fi

    echo "  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  g8e requires secure web connections (TLS 1.3 over a locally generated"
    echo "  ECDSA P-384 private CA). Your device must trust this certificate before"
    echo "  you can use the g8e GUI in your browser."
    echo "  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "  Trusting the certificate adds g8e's self-signed CA to your browser's"
    echo "  trusted store, allowing your browser to connect to https://localhost"
    echo "  or https://g8e.local without security warnings."
    echo ""
    echo "  Note: For operator systems, add g8e.local to your DNS or /etc/hosts"
    echo "  file pointing to this server's IP address (192.168.1.62)."
    echo ""
    echo ""
    echo "  STEP 1: Trust the certificate"
    echo ""
    echo "    If g8e is running on your local workstation:"
    echo "      Visit http://localhost to download and trust the certificate"
    echo ""
    echo "    If g8e is running on a remote system:"
    echo "      Visit http://$_primary_ip to download and trust the certificate"
    echo ""
    echo "    Or run the certificate trust script:"
    echo "      macOS/Linux:  curl -fsSL http://$_primary_ip/trust | sudo sh"
    echo "      Windows:      irm http://$_primary_ip/trust | iex"
    echo ""
    echo "    After trusting, restart your browser."
    echo ""
    echo ""
    echo "  STEP 2: Access the dashboard"
    echo ""
    echo "    If g8e is running on your local workstation:"
    echo "      Dashboard: https://localhost"
    echo ""
    echo "    If g8e is running on a remote system (DNS or hosts file required for https):"
    echo "      Dashboard: https://g8e.local"
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
    echo "Component Status"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    if _operator_listen_running; then
        printf "  %-14s  %-10s  (PID: %s)\n" "operator" "RUNNING" "$(cat "$OPERATOR_LISTEN_PID_FILE")"
    else
        printf "  %-14s  %-10s\n" "operator" "STOPPED"
    fi

    if _g8ee_running; then
        printf "  %-14s  %-10s  (PID: %s)\n" "g8ee" "RUNNING" "$(cat "$G8EE_PID_FILE")"
    else
        printf "  %-14s  %-10s\n" "g8ee" "STOPPED"
    fi

    if _g8ed_running; then
        printf "  %-14s  %-10s  (PID: %s)\n" "g8ed" "RUNNING" "$(cat "$G8ED_PID_FILE")"
    else
        printf "  %-14s  %-10s\n" "g8ed" "STOPPED"
    fi

    echo ""
    exit 0
fi

# ─── down ─────────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "down" ]]; then
    echo "Stopping managed services (g8ee, g8ed) and host services..."
    _stop_g8ee
    _stop_g8ed
    _stop_operator_listen
    echo "Done."
    exit 0
fi

# ─── restart ──────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "restart" ]]; then
    _preflight
    echo "Restarting managed services (g8ee, g8ed) and host services..."
    _stop_g8ee
    _stop_g8ed
    _stop_operator_listen
    _start_operator_listen
    _start_g8ee
    _start_g8ed
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$OPERATOR_LISTEN_HTTP_PORT/health" 60 1
    _sync_operator_binaries
    
    # Update health checks to probe host processes directly
    echo "  g8ee: waiting for healthy status..."
    until curl -sfk "https://localhost:8443/health" >/dev/null 2>&1; do
        sleep 1
    done
    echo -e "  g8ee: \033[0;32mready\033[0m"

    echo "  g8ed: waiting for healthy status..."
    until curl -sfk "https://localhost/health" | grep -q '"status"'; do
        sleep 1
    done
    echo -e "  g8ed: \033[0;32mready\033[0m"

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Restart complete."
    echo ""
    exit 0
fi

# ─── reset ───────────────────────────────────────────────────────────────────
# Wipes DB data volumes and Operator listen-mode data. SSL certs are preserved.
# Use 'clean' to remove everything including SSL.

if [[ "$COMMAND" == "reset" ]]; then
    REBUILD_COMPONENTS=(g8ee g8ed)

    echo "Wiping managed service data and Operator listen-mode data — SSL certs preserved..."
    _stop_g8ee
    _stop_g8ed
    _stop_operator_listen
    
    # Wipe host data
    rm -rf "$OPERATOR_LISTEN_DATA_DIR/"* 2>/dev/null || true
    rm -rf "$PROJECT_ROOT/components/g8ee/data/"* 2>/dev/null || true
    rm -rf "$PROJECT_ROOT/components/g8ed/data/"* 2>/dev/null || true

    echo ""

    _preflight

    # Syncing personas is handled by the app models at runtime
    echo "Starting all services..."
    _start_operator_listen
    _start_g8ee
    _start_g8ed
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$OPERATOR_LISTEN_HTTP_PORT/health" 300 2
    _sync_operator_binaries
    
    # Update health checks to probe host processes directly
    echo "  g8ee: waiting for healthy status..."
    until curl -sfk "https://localhost:8443/health" >/dev/null 2>&1; do
        sleep 1
    done
    echo -e "  g8ee: \033[0;32mready\033[0m"

    echo "  g8ed: waiting for healthy status..."
    until curl -sfk "https://localhost/health" | grep -q '"status"'; do
        sleep 1
    done
    echo -e "  g8ed: \033[0;32mready\033[0m"

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Reset complete. SSL certs preserved — no need to re-trust."
    echo ""
    exit 0
fi

# ─── wipe ─────────────────────────────────────────────────────────────────────
# Clears all app data from the Operator listen-mode database via the HTTP API.
# Preserves: platform settings (components collection), SSL certs, auth token, LLM data.
# Operator listen mode is restarted to flush in-memory state; no volume wipe, no rebuild.
# Use 'reset' to wipe DB data volumes and rebuild from scratch (SSL still preserved).

if [[ "$COMMAND" == "wipe" ]]; then
    _preflight

    echo "Stopping g8ee, g8ed, and Operator listen mode..."
    _stop_g8ee
    _stop_g8ed
    _stop_operator_listen
    echo ""

    echo "Restarting Operator listen mode..."
    _start_operator_listen
    _wait_operator_listen_healthy "https://localhost:$OPERATOR_LISTEN_HTTP_PORT/health" 120 2
    _sync_operator_binaries

    echo "Clearing app data from Operator listen mode..."
    curl -sfk -X POST -H "X-Internal-Auth: $(cat "$OPERATOR_LISTEN_SSL_DIR/internal_auth_token" 2>/dev/null | tr -d ' \n\r')" \
        "https://localhost:$OPERATOR_LISTEN_HTTP_PORT/api/internal/store/wipe" || echo "  Warning: wipe endpoint failed"
    echo ""

    echo "Restarting g8ee and g8ed..."
    _start_g8ee
    _start_g8ed
    echo ""
    echo "Waiting for services..."
    # Update health checks to probe host processes directly
    echo "  g8ee: waiting for healthy status..."
    until curl -sfk "https://localhost:8443/health" >/dev/null 2>&1; do
        sleep 1
    done
    echo -e "  g8ee: \033[0;32mready\033[0m"

    echo "  g8ed: waiting for healthy status..."
    until curl -sfk "https://localhost/health" | grep -q '"status"'; do
        sleep 1
    done
    echo -e "  g8ed: \033[0;32mready\033[0m"

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Wipe complete. Platform settings, SSL certs, and auth token preserved."
    echo ""
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
    UP_COMPONENTS=("${REBUILD_COMPONENTS[@]}")
    if [[ ${#UP_COMPONENTS[@]} -eq 0 ]]; then
        UP_COMPONENTS=(g8ee g8ed)
        _start_operator_listen
    fi
    
    if printf '%s\n' "${UP_COMPONENTS[@]}" | grep -qx operator; then
        _start_operator_listen
        UP_COMPONENTS=($(printf '%s\n' "${UP_COMPONENTS[@]}" | grep -vx operator || true))
    fi

    if [[ ${#UP_COMPONENTS[@]} -gt 0 ]]; then
        for svc in "${UP_COMPONENTS[@]}"; do
            case "$svc" in
                g8ee) _start_g8ee ;;
                g8ed) _start_g8ed ;;
                *)
                    echo "Error: Unknown component '$svc'" >&2
                    exit 1
                    ;;
            esac
        done
    fi
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$OPERATOR_LISTEN_HTTP_PORT/health" 60 1
    _sync_operator_binaries
    
    # Update health checks to probe host processes directly
    if printf '%s\n' "${UP_COMPONENTS[@]}" | grep -qx g8ee; then
        echo "  g8ee: waiting for healthy status..."
        until curl -sfk "https://localhost:8443/health" >/dev/null 2>&1; do
            sleep 1
        done
        echo -e "  g8ee: \033[0;32mready\033[0m"
    fi
    if printf '%s\n' "${UP_COMPONENTS[@]}" | grep -qx g8ed; then
        echo "  g8ed: waiting for healthy status..."
        until curl -sfk "https://localhost/health" | grep -q '"status"'; do
            sleep 1
        done
        echo -e "  g8ed: \033[0;32mready\033[0m"
    fi
    
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
    echo "Stopping all managed services and host services..."
    _stop_g8ee
    _stop_g8ed
    _stop_operator_listen

    # Syncing personas is handled by the app models at runtime
    echo "Starting all services..."
    _start_operator_listen
    _start_g8ee
    _start_g8ed
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$OPERATOR_LISTEN_HTTP_PORT/health" 300 2
    _sync_operator_binaries
    
    # Update health checks to probe host processes directly
    echo "  g8ee: waiting for healthy status..."
    until curl -sfk "https://localhost:8443/health" >/dev/null 2>&1; do
        sleep 1
    done
    echo -e "  g8ee: \033[0;32mready\033[0m"

    echo "  g8ed: waiting for healthy status..."
    until curl -sfk "https://localhost/health" | grep -q '"status"'; do
        sleep 1
    done
    echo -e "  g8ed: \033[0;32mready\033[0m"

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Setup complete."
    echo ""
    _print_platform_info
    exit 0
fi

# ─── rebuild ──────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "rebuild" ]]; then
    if [[ ${#REBUILD_COMPONENTS[@]} -eq 0 ]]; then
        REBUILD_COMPONENTS=(g8ee g8ed)
        _start_operator_listen
    fi

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
                *)
                    echo "Error: Unknown component '$svc'" >&2
                    exit 1
                    ;;
            esac
        done
    fi
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$OPERATOR_LISTEN_HTTP_PORT/health" 300 2
    _sync_operator_binaries

    # Update health checks to probe host processes directly
    if printf '%s\n' "${REBUILD_COMPONENTS[@]}" | grep -qx g8ee; then
        echo "  g8ee: waiting for healthy status..."
        until curl -sfk "https://localhost:443/health" >/dev/null 2>&1; do
            sleep 1
        done
        echo -e "  g8ee: \033[0;32mready\033[0m"
    fi
    if printf '%s\n' "${REBUILD_COMPONENTS[@]}" | grep -qx g8ed; then
        echo "  g8ed: waiting for healthy status..."
        until curl -sfk "https://localhost/health" | grep -q '"status"'; do
            sleep 1
        done
        echo -e "  g8ed: \033[0;32mready\033[0m"
    fi

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
