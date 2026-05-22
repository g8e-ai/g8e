#!/bin/bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Platform lifecycle management for the local g8e environment.
#
# Service categories:
#   Gateway: Operator listen mode (runs as local operator binary)
#   Optional application layer: g8ee (explicit opt-in only)
#   Data volumes:
#     .g8e/data     (Operator listen mode -- SQLite DB, users, settings; wiped by reset)
#     .g8e/pki      (Operator listen mode -- TLS/PKI material; preserved by reset and wipe)
#     .g8e/secrets  (Operator listen mode -- bootstrap secrets; wiped by reset, preserved by wipe)
#     g8ee-data    (g8ee   -- app data; wiped by reset)
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
    for pid_file in "$G8E_OPERATOR_PID_FILE" "$G8E_G8EE_PID_FILE"; do
        if [ -f "$pid_file" ]; then
            local pid
            pid=$(cat "$pid_file")
            if ! ps -p "$pid" > /dev/null 2>&1; then
                rm -f "$pid_file"
            fi
        fi
    done
    [[ $rc -eq 0 ]] || return
}
trap _footer EXIT


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
. "${SCRIPT_DIR}/config.sh"
PROJECT_ROOT="$G8E_PROJECT_ROOT"

DEV_MODE=false

MANAGED_SERVICES=(operator)
OPTIONAL_APPS=(g8ee)
TEST_RUNNER_SERVICES=()
WITH_APPS=false
OPTIONAL_COMPONENTS=()

if [[ "${G8E_WITH_G8EE:-}" == "1" || "${G8E_WITH_G8EE:-}" == "true" ]]; then
    WITH_APPS=true
    OPTIONAL_COMPONENTS=("${OPTIONAL_APPS[@]}")
fi

_service_volume() {
    case "$1" in
        g8ee)   echo "g8ee-data" ;;
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
        *)
            echo "Error: Unknown optional app '$1'" >&2
            exit 1
            ;;
    esac
}

_stop_optional_app() {
    case "$1" in
        g8ee) _stop_g8ee ;;
        *)
            echo "Error: Unknown optional app '$1'" >&2
            exit 1
            ;;
    esac
}

_wait_optional_app_healthy() {
    case "$1" in
        g8ee) _wait_service_healthy "g8ee" "https://localhost:${G8E_G8EE_HTTPS_PORT}/health" 10 1 "$G8E_G8EE_LOG_FILE" ;;
    esac
}

# PKI volume is never wiped - preserved across reset, wipe, and rebuild.
PKI_VOLUME="$G8E_PKI_DIR"
SECRETS_VOLUME="$G8E_SECRETS_DIR"

usage() {
    cat <<EOF
Usage: $(basename "$0") <command> [options]

Commands:
  status                          Show Gateway and optional app process status
  up [component ...] [-a|--with-apps] Start Operator listen mode by default
                                  Default (no components): operator
                                  Valid: operator g8ee
                                  Optional apps require -a, --with-apps, or --with-g8ee
  down                            Stop Operator listen mode and optional apps -- nothing is removed
  rebuild [component ...]         Restart Operator listen mode by default
                                  Default (no components): operator
                                  Valid: operator g8ee
                                  Optional apps require -a, --with-apps, or --with-g8ee
  reset                           Wipe Operator listen-mode data. PKI certs and secrets are preserved.
  clean                           Nuke runtime processes and data.
  operator-build                  Build linux/amd64 operator binary natively
  operator-build-all              Build all operator architectures natively

Examples:
  $(basename "$0") status                       Show host process status and versions
  $(basename "$0") up                           Start Operator listen mode
  $(basename "$0") up -a                        Start Operator plus optional bundled apps
  $(basename "$0") down                         Stop runtime processes
  $(basename "$0") rebuild                      Restart Operator listen mode
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
        --with-g8ee|-a)
            WITH_APPS=true
            OPTIONAL_COMPONENTS+=("g8ee")
            shift
            ;;
        setup|up|down|restart|reset|clean|status|operator-build|operator-build-all)
            COMMAND="$1"
            shift
            while [[ $# -gt 0 && ! "$1" =~ ^- ]]; do
                if ! printf '%s\n' operator g8ee | grep -qx "$1"; then
                    echo "Error: Invalid component '$1'" >&2
                    echo "Valid: operator g8ee" >&2
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
                if ! printf '%s\n' operator g8ee | grep -qx "$1"; then
                    echo "Error: Invalid component '$1'" >&2
                    echo "Valid: operator g8ee" >&2
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
            echo "Error: optional app '$component' requires --with-g8ee or ./g8e apps start $component" >&2
            exit 1
        fi
    done
fi

# ─── Helpers ──────────────────────────────────────────────────────────────────

_check_port_available() {
    local port="$1"
    local name="$2"
    if ! python3 -c "import socket; s = socket.socket(socket.AF_INET, socket.SOCK_STREAM); s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1); s.settimeout(0.5); s.bind(('127.0.0.1', $port))" 2>/dev/null; then
        local pid_info=""
        if command -v lsof >/dev/null 2>&1; then
            local pids
            pids=$(lsof -i :"$port" -t 2>/dev/null)
            if [ -n "$pids" ]; then
                pid_info=$(echo "$pids" | xargs ps -o pid=,comm= 2>/dev/null | tr '\n' ' ')
            fi
        elif command -v fuser >/dev/null 2>&1; then
            local pids
            pids=$(fuser "$port"/tcp 2>/dev/null)
            if [ -n "$pids" ]; then
                pid_info=$(echo "$pids" | xargs ps -o pid=,comm= 2>/dev/null | tr '\n' ' ')
            fi
        fi
        echo "Error: Port $port ($name) is already in use!" >&2
        if [ -n "$pid_info" ]; then
            echo "       Conflicting process: $pid_info" >&2
        fi
        return 1
    fi
    return 0
}


_operator_listen_running() {
    if [ -f "$G8E_OPERATOR_PID_FILE" ]; then
        local pid
        pid=$(cat "$G8E_OPERATOR_PID_FILE")
        if ps -p "$pid" > /dev/null 2>&1; then
            return 0
        fi
        rm -f "$G8E_OPERATOR_PID_FILE"
    fi
    return 1
}

_g8ee_running() {
    if [ -f "$G8E_G8EE_PID_FILE" ]; then
        local pid
        pid=$(cat "$G8E_G8EE_PID_FILE")
        if ps -p "$pid" > /dev/null 2>&1; then
            return 0
        fi
        rm -f "$G8E_G8EE_PID_FILE"
    fi
    return 1
}

_rotate_logs() {
    local log_file="$1"
    if [ -f "$log_file" ]; then
        local max_backups="$G8E_LOG_MAX_BACKUPS"
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
        echo "  g8ee is already running (PID: $(cat "$G8E_G8EE_PID_FILE"))."
        return 0
    fi

    _check_port_available "${G8E_G8EE_HTTPS_PORT}" "g8ee Ensemble API" || exit 1

    local venv_dir="$PROJECT_ROOT/services/g8ee/.venv"
    if [ ! -d "$venv_dir" ]; then
        echo "  Creating g8ee virtualenv..."
        python3 -m venv "$venv_dir"
        "$venv_dir/bin/pip" install --upgrade pip
        "$venv_dir/bin/pip" install -r "$PROJECT_ROOT/services/g8ee/requirements.txt"
    fi

    echo "  Starting g8ee on port ${G8E_G8EE_HTTPS_PORT} (HTTPS)..."
    _rotate_logs "$G8E_G8EE_LOG_FILE"

    (
        cd "$PROJECT_ROOT/services/g8ee"
        export G8E_PKI_DIR="$G8E_PKI_DIR"
        export G8E_SECRETS_DIR="$G8E_SECRETS_DIR"
        export PYTHONPATH="$PROJECT_ROOT/services/g8ee:$PROJECT_ROOT/protocol/python"
        export G8E_PROTOCOL_DIR="$PROJECT_ROOT/protocol"
        export G8E_INTERNAL_HTTP_URL="https://localhost:${G8E_OPERATOR_HTTPS_PORT}"
        export G8E_INTERNAL_PUBSUB_URL="wss://localhost:${G8E_OPERATOR_PUBLIC_WSS_PORT}"
        export G8E_TEST_LLM_PRIMARY_PROVIDER="${G8E_TEST_LLM_PRIMARY_PROVIDER:-}"
        export G8E_TEST_LLM_PRIMARY_MODEL="${G8E_TEST_LLM_PRIMARY_MODEL:-}"
        export G8E_TEST_LLM_PRIMARY_API_KEY="${G8E_TEST_LLM_PRIMARY_API_KEY:-}"
        export G8E_TEST_LLM_ASSISTANT_PROVIDER="${G8E_TEST_LLM_ASSISTANT_PROVIDER:-}"
        export G8E_TEST_LLM_ASSISTANT_MODEL="${G8E_TEST_LLM_ASSISTANT_MODEL:-}"
        export G8E_TEST_LLM_ASSISTANT_API_KEY="${G8E_TEST_LLM_ASSISTANT_API_KEY:-}"
        export G8E_TEST_LLM_LITE_PROVIDER="${G8E_TEST_LLM_LITE_PROVIDER:-}"
        export G8E_TEST_LLM_LITE_MODEL="${G8E_TEST_LLM_LITE_MODEL:-}"
        export G8E_TEST_LLM_LITE_API_KEY="${G8E_TEST_LLM_LITE_API_KEY:-}"
        export G8E_TEST_LLM_LITE_ENDPOINT="${G8E_TEST_LLM_LITE_ENDPOINT:-}"

        local cert_name="${G8E_PATH_G8EE_CERT_NAME:-g8ee}"
        setsid "$venv_dir/bin/uvicorn" app.main:app --host 0.0.0.0 --port "${G8E_G8EE_HTTPS_PORT}" \
            --ssl-keyfile "$G8E_PKI_DIR/issued/apps/${cert_name}.key" \
            --ssl-certfile "$G8E_PKI_DIR/issued/apps/${cert_name}.crt" \
            > "$G8E_G8EE_LOG_FILE" 2>&1 &
        echo $! > "$G8E_G8EE_PID_FILE"
    )

    sleep 2
    if ! _g8ee_running; then
        echo "  Error: g8ee failed to start. See $G8E_G8EE_LOG_FILE"
        rm -f "$G8E_G8EE_PID_FILE"
        return 1
    fi
}

_stop_g8ee() {
    local pid=""
    if [ -f "$G8E_G8EE_PID_FILE" ]; then
        pid=$(cat "$G8E_G8EE_PID_FILE")
    fi

    if [ -n "$pid" ] && ps -p "$pid" > /dev/null 2>&1; then
        echo "  Stopping g8ee (PID: $pid)..."
        kill "$pid" 2>/dev/null || true
        local waited=0
        while ps -p "$pid" > /dev/null 2>&1 && [ $waited -lt 10 ]; do
            sleep 1
            waited=$((waited + 1))
        done
        if ps -p "$pid" > /dev/null 2>&1; then
            echo "  Force stopping g8ee (PID: $pid)..."
            kill -9 "$pid" 2>/dev/null || true
        fi
    fi

    # Fallback to pgrep for any remaining g8ee processes (uvicorn app.main:app)
    local found_pids
    found_pids=$(pgrep -f "uvicorn app.main:app" 2>/dev/null || true)
    for f_pid in $found_pids; do
        if [[ "$f_pid" != "$pid" ]]; then
            echo "  Stopping g8ee (PID: $f_pid, found via pgrep)..."
            kill "$f_pid" 2>/dev/null || true
            local waited=0
            while ps -p "$f_pid" > /dev/null 2>&1 && [ $waited -lt 10 ]; do
                sleep 1
                waited=$((waited + 1))
            done
            if ps -p "$f_pid" > /dev/null 2>&1; then
                echo "  Force stopping g8ee (PID: $f_pid)..."
                kill -9 "$f_pid" 2>/dev/null || true
            fi
        fi
    done

    rm -f "$G8E_G8EE_PID_FILE"
}

_start_operator_listen() {
    if _operator_listen_running; then
        echo "  Governance Gateway is already running (PID: $(cat "$G8E_OPERATOR_PID_FILE"))."
        return 0
    fi

    _check_port_available "$G8E_OPERATOR_HTTPS_PORT" "Governance Gateway HTTP API" || exit 1
    _check_port_available "$G8E_REMOTE_OPERATOR_BOOTSTRAP_HTTPS_PORT" "Operator Bootstrap" || exit 1
    _check_port_available "$G8E_OPERATOR_PUBLIC_HTTPS_PORT" "Operator Public API" || exit 1

    local host_arch="amd64"
    case "$(uname -m)" in
        x86_64)         host_arch="amd64" ;;
        aarch64|arm64)  host_arch="arm64" ;;
        i386|i686)      host_arch="386" ;;
    esac
    local bin="$PROJECT_ROOT/services/g8eo/build/linux-${host_arch}/g8e.gateway"

    if command -v go >/dev/null 2>&1; then
        echo "  Building Governance Gateway and Operator natively..."
        (cd "$PROJECT_ROOT/services/g8eo" && make build-local) || {
            if [ -f "$bin" ]; then
                echo "  WARNING: Native build failed, but pre-built binary exists. Using pre-built..."
            else
                echo "  Error: Native build failed and no pre-built binary found." >&2
                return 1
            fi
        }
    else
        if [ -f "$bin" ]; then
            echo "  Go toolchain not found. Using pre-compiled binary for ${host_arch}..."
        else
            echo "  Error: Go toolchain not found and no pre-compiled binary found for ${host_arch} at $bin" >&2
            return 1
        fi
    fi

    echo "  Starting Governance Gateway..."
    mkdir -p "$G8E_DATA_DIR" "$G8E_PKI_DIR" "$G8E_SECRETS_DIR" "$G8E_PID_DIR" "$G8E_LOG_DIR"

    _rotate_logs "$G8E_OPERATOR_LOG_FILE"

    export G8E_PKI_DIR="$G8E_PKI_DIR"
    export G8E_SECRETS_DIR="$G8E_SECRETS_DIR"

    setsid "$bin" --listen \
        --data-dir "$G8E_DATA_DIR" \
        --pki-dir "$G8E_PKI_DIR" \
        --secrets-dir "$G8E_SECRETS_DIR" \
        --http-listen-port "$G8E_OPERATOR_HTTPS_PORT" \
        --bootstrap-listen-port "$G8E_REMOTE_OPERATOR_BOOTSTRAP_HTTPS_PORT" \
        --public-listen-port "$G8E_OPERATOR_PUBLIC_HTTPS_PORT" \
        > "$G8E_OPERATOR_LOG_FILE" 2>&1 &

    local pid=$!
    echo "$pid" > "$G8E_OPERATOR_PID_FILE"

    sleep 2
    if ! _operator_listen_running; then
        echo "  Error: Operator listen mode failed to start. See $G8E_OPERATOR_LOG_FILE"
        rm -f "$G8E_OPERATOR_PID_FILE"
        return 1
    fi
}

_stop_operator_listen() {
    local pid=""
    
    if [ -f "$G8E_OPERATOR_PID_FILE" ]; then
        pid=$(cat "$G8E_OPERATOR_PID_FILE")
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
    fi

    # Fallback search for any remaining g8e.gateway or g8e.operator processes
    # We look for both because g8e.gateway is the newer name but g8e.operator may still be used
    # We try to be specific to this project root to avoid killing processes from other workspaces
    local patterns=("g8e.gateway --listen" "g8e.operator --listen")
    for pattern in "${patterns[@]}"; do
        local found_pids
        # Try matching with project root first for specificity
        found_pids=$(pgrep -f "$PROJECT_ROOT/.*$pattern" 2>/dev/null || true)
        # If none found, fall back to global match as a safety measure
        if [ -z "$found_pids" ]; then
            found_pids=$(pgrep -f "$pattern" 2>/dev/null || true)
        fi
        
        for f_pid in $found_pids; do
            if [[ "$f_pid" != "$pid" ]]; then
                echo "  Stopping Operator listen mode (PID: $f_pid, found via pgrep '$pattern')..."
                kill "$f_pid" 2>/dev/null || true
                local waited=0
                while ps -p "$f_pid" > /dev/null 2>&1 && [ $waited -lt 10 ]; do
                    sleep 1
                    waited=$((waited + 1))
                done
                if ps -p "$f_pid" > /dev/null 2>&1; then
                    echo "  Force stopping Operator listen mode (PID: $f_pid)..."
                    kill -9 "$f_pid" 2>/dev/null || true
                fi
            fi
        done
    done

    rm -f "$G8E_OPERATOR_PID_FILE"
}

_wait_operator_listen_healthy() {
    local url="$1" timeout_s="$2" interval="${3:-1}"
    local waited=0
    local trust_bundle="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR/trust/hub-bundle.pem}"
    echo "  Operator listen mode: waiting for $url..."

    until [[ -f "$trust_bundle" ]] && curl -sf --cacert "$trust_bundle" "$url" >/dev/null 2>&1; do
        if (( waited >= timeout_s )); then
            echo -e "  Operator listen mode: \033[0;31mTIMEOUT\033[0m"
            echo "  Operator listen mode did not become healthy within ${timeout_s}s. See $G8E_OPERATOR_LOG_FILE"
            tail -n 20 "$G8E_OPERATOR_LOG_FILE"
            exit 1
        fi
        sleep "$interval"
        waited=$(( waited + interval ))
    done
    echo -e "  Operator listen mode: \033[0;32mready\033[0m (${waited}s)"

    # Auto-bootstrap if needed
    source "$PROJECT_ROOT/scripts/cmd/common.sh"
    _operator_bootstrap
}

_wait_service_healthy() {
    local service_name="$1" url="$2" timeout_s="$3" interval="${4:-1}" log_file="$5"
    local waited=0
    local trust_bundle="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR/trust/hub-bundle.pem}"
    echo "  $service_name: waiting for healthy status..."

    until [[ -f "$trust_bundle" ]] && curl -sf --cacert "$trust_bundle" "$url" >/dev/null 2>&1; do
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
    local op_pid="-"
    if _operator_listen_running; then
        op_pid=$(cat "$G8E_OPERATOR_PID_FILE")
    fi

    local g8ee_pid="-"
    if _g8ee_running; then
        g8ee_pid=$(cat "$G8E_G8EE_PID_FILE")
    fi

    # Gather dynamic data
    local build_hash=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    local version=$(cat "$PROJECT_ROOT/VERSION" 2>/dev/null || echo "unknown")
    
    # Root CA fingerprint
    local root_ca_fingerprint="unavailable"
    local root_ca_path="$G8E_PKI_DIR/trust/root.pem"
    if [[ -f "$root_ca_path" ]]; then
        root_ca_fingerprint=$(openssl x509 -in "$root_ca_path" -noout -fingerprint -sha256 2>/dev/null | sed 's/^.*=//' | tr -d ':' | tr '[:upper:]' '[:lower:]' | head -c 16)
    fi

    # L1 pattern count (count forbidden_patterns extensions in proto files)
    local l1_pattern_count=6
    local l1_count=$(grep -r "forbidden_patterns" "$PROJECT_ROOT/protocol/proto" 2>/dev/null | wc -l)
    if [[ "$l1_count" -gt 0 ]]; then
        l1_pattern_count=$l1_count
    fi

    # L2 signer count (query SQLite or default to 0)
    local l2_signer_count=0
    local db_path="$G8E_DATA_DIR/g8e.db"
    if [[ -f "$db_path" ]]; then
        l2_signer_count=$(sqlite3 "$db_path" "SELECT COUNT(*) FROM trusted_signers;" 2>/dev/null || echo "0")
    fi

    # Operator session ID (from bootstrap status or placeholder)
    local operator_session_id="pending first connection"
    local trust_bundle="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR/trust/hub-bundle.pem}"
    if [[ -f "$trust_bundle" ]]; then
        local status_resp
        status_resp=$(curl -sS --cacert "$trust_bundle" "https://localhost:$G8E_OPERATOR_PUBLIC_HTTPS_PORT/api/auth/bootstrap/status" 2>/dev/null)
        if [[ -n "$status_resp" ]]; then
            local session_id
            session_id=$(echo "$status_resp" | python3 "$PROJECT_ROOT/scripts/core/json_query.py" - operator_session_id --default "" 2>/dev/null)
            if [[ -n "$session_id" ]]; then
                operator_session_id=$session_id
            fi
        fi
    fi

    # State root
    local state_root="0x0"
    if [[ -f "$trust_bundle" ]]; then
        local status_resp
        status_resp=$(curl -sS --cacert "$trust_bundle" "https://localhost:$G8E_OPERATOR_PUBLIC_HTTPS_PORT/api/auth/bootstrap/status" 2>/dev/null)
        if [[ -n "$status_resp" ]]; then
            state_root=$(echo "$status_resp" | python3 "$PROJECT_ROOT/scripts/core/json_query.py" - state_merkle_root --default "0x0")
        fi
    fi

    # Ruleset hash (hash of proto files for version tracking)
    local ruleset_hash=$(find "$PROJECT_ROOT/protocol/proto" -name "*.proto" -exec sha256sum {} \; | sort | sha256sum | head -c 8)

    local title="G8E GOVERNANCE GATEWAY  ·  $version ($build_hash)"
    local title_len=${#title}
    local pad=$(( (78 - title_len) / 2 ))
    local pad_str=$(printf '%*s' "$pad" '')
    local right_pad_len=$(( 78 - title_len - pad ))
    local right_pad_str=$(printf '%*s' "$right_pad_len" '')

    _print_header() {
        local htitle=" $1 "
        local hlen=${#htitle}
        local dash_count=$(( 80 - 1 - hlen ))
        local dashes=""
        for ((i=0; i<dash_count; i++)); do dashes="${dashes}─"; done
        echo "─${htitle}${dashes}"
    }

    local uptime="0s"
    if [[ "$op_pid" != "-" ]]; then
        local etimes
        etimes=$(ps -p "$op_pid" -o etimes= 2>/dev/null | tr -d ' ' || echo "0")
        if [[ -n "$etimes" && "$etimes" =~ ^[0-9]+$ ]]; then
            uptime="${etimes}s"
        fi
    fi

    echo ""
    echo "╔══════════════════════════════════════════════════════════════════════════════╗"
    echo "║${pad_str}${title}${right_pad_str}║"
    echo "╚══════════════════════════════════════════════════════════════════════════════╝"
    echo ""
    _print_header "STATUS"
    printf "  %-7s %s\n" "State" "LISTENING — accepting mutations"
    printf "  %-7s %s\n" "PID" "$op_pid"
    if [[ "$WITH_APPS" == "true" ]]; then
        printf "  %-7s %s\n" "App PID" "$g8ee_pid"
    fi
    printf "  %-7s %s\n" "Uptime" "$uptime"
    _print_header "ENDPOINTS"
    printf "  %-13s %-25s %-10s %s\n" "Public API" "https://localhost:$G8E_OPERATOR_PUBLIC_HTTPS_PORT" "$G8E_OPERATOR_PUBLIC_HTTPS_PORT/tcp" ""
    printf "  %-13s %-25s %-10s %s\n" "WSS Stream" "wss://localhost:$G8E_OPERATOR_PUBLIC_WSS_PORT" "$G8E_OPERATOR_PUBLIC_WSS_PORT/tcp" ""
    printf "  %-13s %-25s %-10s %s\n" "Internal API" "http://127.0.0.1:$G8E_OPERATOR_HTTPS_PORT" "$G8E_OPERATOR_HTTPS_PORT/tcp" "loopback only"
    printf "  %-13s %-25s %-10s %s\n" "Bootstrap" "http://${HOST_IPS%%,*}:$G8E_REMOTE_OPERATOR_BOOTSTRAP_HTTPS_PORT" "$G8E_REMOTE_OPERATOR_BOOTSTRAP_HTTPS_PORT/tcp" "LAN — trust install"
    _print_header "ENFORCEMENT POSTURE"
    printf "  %-17s %s\n" "L1 Doctrine" "$l1_pattern_count patterns"
    printf "  %-17s %s\n" "L2 Consensus" "$l2_signer_count signers enrolled"
    printf "  %-17s %s\n" "L3 Notary" "$l2_signer_count signers enrolled"
    printf "  %-17s %s\n" "Ruleset" "${ruleset_hash:0:8}…"
    if [[ "$l2_signer_count" -eq 0 ]]; then
        printf "  %-17s %s\n" "" "⚠ No L2/L3 signers"
    fi
    _print_header "IDENTITY"
    printf "  %-11s %s\n" "Session" "$operator_session_id"
    printf "  %-11s %s\n" "Root CA" "sha256:${root_ca_fingerprint:0:6}… (fresh)"
    local sr_suffix=""
    if [[ "$state_root" == "0x0" ]]; then sr_suffix=" (genesis)"; fi
    printf "  %-11s %s\n" "State Root" "$state_root$sr_suffix"
    _print_header "TRUST BOOTSTRAP"
    echo "  Install Platform Root CA + provision mTLS certs:"
    echo "    macOS / Linux   curl -fsSL http://${HOST_IPS%%,*}:$G8E_REMOTE_OPERATOR_BOOTSTRAP_HTTPS_PORT/trust | sudo sh"
    echo "    Windows (PS)    irm http://${HOST_IPS%%,*}:$G8E_REMOTE_OPERATOR_BOOTSTRAP_HTTPS_PORT/trust | iex"
    _print_header "NEXT"
    printf "  %-2s %-16s %s\n" "1" "Authenticate" "./g8e login"
    if [[ "$WITH_APPS" == "true" ]]; then
        printf "  %-2s %-16s %s\n" "2" "Use Engine" "https://localhost:$G8E_G8EE_HTTPS_PORT"
    else
        printf "  %-2s %-16s %s\n" "2" "Device links" "./g8e data device-links create --email <email>"
    fi
    printf "  %-2s %-16s %s\n" "3" "Tail logs" "tail -f ${G8E_LOG_DIR/#$HOME/\~}/operator.log"
    echo "────────────────────────────────────────────────────────────────────────────────"
    echo "[g8e] Control plane listening. Mutations governed by active ruleset."
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
    
    printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
    printf "  %-14s %-12s %-8s %-32s %s\n" "Component" "Status" "PID" "Endpoints" "Extra"
    printf "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
    printf "\n  \033[1m[Gateway]\033[0m\n"
    
    if _operator_listen_running; then
        pid=$(cat "$G8E_OPERATOR_PID_FILE")
        
        # Check bootstrap status
        bootstrapped="UNKNOWN"
        trust_bundle="$G8E_PKI_DIR/trust/hub-bundle.pem"
        if [[ -f "$trust_bundle" ]]; then
            status_resp=$(curl -sS --cacert "$trust_bundle" "https://localhost:$G8E_OPERATOR_PUBLIC_HTTPS_PORT/api/auth/bootstrap/status" 2>/dev/null)
            if [[ $(echo "$status_resp" | python3 "$PROJECT_ROOT/scripts/core/json_query.py" - bootstrapped 2>/dev/null) == "true" ]]; then
                bootstrapped="YES"
            else
                bootstrapped="NO"
            fi
        fi
        
        printf "  %-14s \033[1;32m%-12s\033[0m %-8s %-32s %s\n" "operator" "RUNNING" "$pid" "https://localhost:$G8E_OPERATOR_PUBLIC_HTTPS_PORT (API)" "Bootstrapped: $bootstrapped"
        printf "  %-14s %-12s %-8s %-32s %s\n" "" "" "" "wss://localhost:$G8E_OPERATOR_PUBLIC_WSS_PORT (WSS)" ""
        printf "  %-14s %-12s %-8s %-32s %s\n" "" "" "" "http://localhost:$G8E_REMOTE_OPERATOR_BOOTSTRAP_HTTPS_PORT (Bootstrap)" ""
    else
        printf "  %-14s \033[1;31m%-12s\033[0m %-8s %-32s %s\n" "operator" "STOPPED" "-" "-" "-"
    fi

    printf "\n  \033[1m[Optional Application Layer]\033[0m\n"
    if _g8ee_running; then
        printf "  %-14s \033[1;32m%-12s\033[0m %-8s %-32s %s\n" "g8ee" "RUNNING" "$(cat "$G8E_G8EE_PID_FILE")" "https://localhost:$G8E_G8EE_HTTPS_PORT" ""
    else
        printf "  %-14s \033[1;31m%-12s\033[0m %-8s %-32s %s\n" "g8ee" "NOT RUNNING" "-" "-" "-"
    fi

    echo ""
    exit 0
fi

# ─── down ─────────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "down" ]]; then
    echo "Stopping Operator listen mode and optional application-layer services..."
    _stop_g8ee
    _stop_operator_listen
    echo "Done."
    exit 0
fi

# ─── restart ──────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "restart" ]]; then
    _preflight
    mapfile -t RESTART_COMPONENTS < <(_expand_components true "${REBUILD_COMPONENTS[@]}")
    echo "Restarting Gateway components..."
    if printf '%s\n' "${RESTART_COMPONENTS[@]}" | grep -qx g8ee; then
        _stop_g8ee
    fi
    _stop_operator_listen
    _start_operator_listen
    for svc in "${RESTART_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _start_optional_app "$svc"
    done
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$G8E_OPERATOR_PUBLIC_HTTPS_PORT/health" 60 1
    for svc in "${RESTART_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _wait_optional_app_healthy "$svc"
    done

    _print_platform_info
    exit 0
fi

# ─── reset ───────────────────────────────────────────────────────────────────
# Wipes DB data volumes and bootstrap secrets. PKI certs are preserved.
# Use 'clean' to remove everything including PKI.

if [[ "$COMMAND" == "reset" ]]; then
    mapfile -t RESET_COMPONENTS < <(_expand_components true "${REBUILD_COMPONENTS[@]}")

    echo "Wiping Operator listen-mode data and secrets - PKI certs preserved..."
    _stop_g8ee
    _stop_operator_listen
    
    # Wipe host data
    rm -rf "$G8E_DATA_DIR/"* 2>/dev/null || true
    rm -rf "$G8E_SECRETS_DIR/"* 2>/dev/null || true
    rm -rf "$PROJECT_ROOT/services/g8ee/data/"* 2>/dev/null || true

    echo ""

    _preflight

    echo "Starting Gateway services..."
    _start_operator_listen
    for svc in "${RESET_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _start_optional_app "$svc"
    done
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$G8E_OPERATOR_PUBLIC_HTTPS_PORT/health" 300 2

    for svc in "${RESET_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _wait_optional_app_healthy "$svc"
    done

    _print_platform_info
    exit 0
fi

# ─── clean ────────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "clean" ]]; then
    echo "Cleaning all host services and runtime data..."

    _stop_g8ee
    _stop_operator_listen
    rm -rf "$G8E_RUNTIME_DIR" 2>/dev/null || true

    echo "Cleaning Python caches..."
    find "$PROJECT_ROOT" -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
    find "$PROJECT_ROOT" -type f -name "*.pyc" -delete 2>/dev/null || true
    find "$PROJECT_ROOT" -type f -name "*.pyo" -delete 2>/dev/null || true
    find "$PROJECT_ROOT" -type d -name "*.pyc" -exec rm -rf {} + 2>/dev/null || true
    find "$PROJECT_ROOT" -type d -name "*.pyo" -exec rm -rf {} + 2>/dev/null || true
    if [ -d "$PROJECT_ROOT/services/g8ee/.venv" ]; then
        rm -rf "$PROJECT_ROOT/services/g8ee/.venv/__pycache__" 2>/dev/null || true
        find "$PROJECT_ROOT/services/g8ee/.venv" -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
    fi

    echo "Done."
    exit 0
fi

# ─── Preflight (up and rebuild) ───────────────────────────────────────────────

_preflight

# ─── up ───────────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "up" ]]; then
    echo "[g8e] Initializing"
    mapfile -t UP_COMPONENTS < <(_expand_components true "${REBUILD_COMPONENTS[@]}")
    
    if printf '%s\n' "${UP_COMPONENTS[@]}" | grep -qx operator; then
        _start_operator_listen
        UP_COMPONENTS=($(printf '%s\n' "${UP_COMPONENTS[@]}" | grep -vx operator || true))
    fi

    if [[ ${#UP_COMPONENTS[@]} -gt 0 ]]; then
        for svc in "${UP_COMPONENTS[@]}"; do
            case "$svc" in
                g8ee) _start_g8ee ;;
                *) _start_optional_app "$svc" ;;
            esac
        done
    fi
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$G8E_OPERATOR_PUBLIC_HTTPS_PORT/health" 60 1
    
    for svc in "${UP_COMPONENTS[@]}"; do
        _wait_optional_app_healthy "$svc"
    done
    
    _print_platform_info
    exit 0
fi

# ─── setup ───────────────────────────────────────────────────────────────────
# Full first-time setup, then start the platform.
# Does NOT wipe data volumes - safe to run on an existing installation.
# Operator binary builds provide the listen-mode and remote Operator artifacts.

if [[ "$COMMAND" == "setup" ]]; then
    echo "[g8e] Initializing"
    mapfile -t SETUP_COMPONENTS < <(_expand_components true "${REBUILD_COMPONENTS[@]}")
    echo "Stopping all runtime services..."
    _stop_g8ee
    _stop_operator_listen

    echo "Starting Gateway services..."
    _start_operator_listen
    for svc in "${SETUP_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _start_optional_app "$svc"
    done
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$G8E_OPERATOR_PUBLIC_HTTPS_PORT/health" 300 2

    for svc in "${SETUP_COMPONENTS[@]}"; do
        [[ "$svc" == "operator" ]] && continue
        _wait_optional_app_healthy "$svc"
    done

    _print_platform_info
    exit 0
fi

# ─── rebuild ──────────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "rebuild" ]]; then
    echo "[g8e] Initializing"
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
                *) _start_optional_app "$svc" ;;
            esac
        done
    fi
    echo ""
    echo "Waiting for services..."
    _wait_operator_listen_healthy "https://localhost:$G8E_OPERATOR_PUBLIC_HTTPS_PORT/health" 300 2

    for svc in "${REBUILD_COMPONENTS[@]}"; do
        _wait_optional_app_healthy "$svc"
    done

    _print_platform_info
    exit 0
fi

# ─── operator-build ─────────────────────────────────────────────────────────────

if [[ "$COMMAND" == "operator-build" ]]; then
    echo "Building linux/amd64 operator binary natively..."
    (cd "$PROJECT_ROOT/services/g8eo" && make build-local)
    echo ""
    echo "Operator binary built."
    exit 0
fi

# ─── operator-build-all ─────────────────────────────────────────────────────────

if [[ "$COMMAND" == "operator-build-all" ]]; then
    echo "Building all operator architectures natively..."
    (cd "$PROJECT_ROOT/services/g8eo" && make build-local-all)
    echo ""
    echo "All operator binaries built."
    exit 0
fi
