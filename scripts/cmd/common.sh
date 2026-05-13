#!/usr/bin/env bash

# Common helpers and environment for g8e CLI commands.
# This file is intended to be sourced by specific command scripts.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export G8E_PROJECT_ROOT="$SCRIPT_DIR"

# Host-native runtime layout
G8E_RUNTIME_DIR="${G8E_RUNTIME_DIR:-$SCRIPT_DIR/.g8e}"
G8E_PKI_DIR_HOST="${G8E_PKI_DIR:-$G8E_RUNTIME_DIR/pki}"
G8E_SECRETS_DIR_HOST="${G8E_SECRETS_DIR:-$G8E_RUNTIME_DIR/secrets}"
OPERATOR_HTTP_URL="${G8E_INTERNAL_HTTP_URL:-https://localhost:9000}"
_OPERATOR_PID_FILE="$G8E_RUNTIME_DIR/pids/operator-listen.pid"
_G8EE_PID_FILE="$G8E_RUNTIME_DIR/pids/g8ee.pid"

# Local credential store
G8E_CREDENTIALS_DIR="$HOME/.g8e"
G8E_CREDENTIALS_FILE="$G8E_CREDENTIALS_DIR/credentials"
G8E_CLI_CERT_FILE="$G8E_CREDENTIALS_DIR/cli.crt"
G8E_CLI_KEY_FILE="$G8E_CREDENTIALS_DIR/cli.key"

_banner() {
    echo -e "\033[1;34m[g8e]\033[0m $*"
}

_pid_alive() {
    local pid_file="$1"
    [ -f "$pid_file" ] && ps -p "$(cat "$pid_file")" > /dev/null 2>&1
}

_g8ee_running()    { _pid_alive "$_G8EE_PID_FILE"; }
_operator_running() { _pid_alive "$_OPERATOR_PID_FILE"; }

_ensure_operator() {
    if ! _operator_running; then
        echo "[g8e] Operator listen mode is not running — start the platform: ./g8e platform start" >&2
        exit 1
    fi
}

_requires_operator_route() {
    echo "[g8e] command requires Operator-side route not yet implemented: $1" >&2
    exit 1
}

_moved_to_operator_protocol() {
    echo "[g8e] command moved to Operator protocol API and not yet implemented: $1" >&2
    exit 1
}

_operator_curl() {
    local method="$1" path="$2" body="${3:-}"
    local trust_bundle="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR_HOST/trust/hub-bundle.pem}"
    if [[ ! -f "$trust_bundle" ]]; then
        echo "[g8e] Operator trust bundle not found at $trust_bundle — recreate runtime PKI with ./g8e platform clean && ./g8e platform start" >&2
        return 1
    fi
    local args=(-sS --cacert "$trust_bundle" -X "$method")

    # mTLS client certificate: prefer CLI cert, fall back to app cert
    local cli_cert="${G8E_CLI_CERT:-$G8E_CLI_CERT_FILE}"
    local cli_key="${G8E_CLI_KEY:-$G8E_CLI_KEY_FILE}"
    if [[ -f "$cli_cert" && -f "$cli_key" ]]; then
        args+=(--cert "$cli_cert" --key "$cli_key")
    elif [[ -f "$G8E_PKI_DIR_HOST/issued/apps/g8ee.crt" && -f "$G8E_PKI_DIR_HOST/issued/apps/g8ee.key" ]]; then
        args+=(--cert "$G8E_PKI_DIR_HOST/issued/apps/g8ee.crt" --key "$G8E_PKI_DIR_HOST/issued/apps/g8ee.key")
    else
        echo "[g8e] No mTLS client certificate available — run: ./g8e login" >&2
        return 1
    fi

    if [[ -n "$OPERATOR_SESSION_ID" ]]; then
        args+=(-H "X-G8E-Operator-Session-ID: $OPERATOR_SESSION_ID")
    fi

    args+=(-H "Content-Type: application/json")
    [[ -n "$body" ]] && args+=(-d "$body")
    curl "${args[@]}" "$OPERATOR_HTTP_URL$path"
}

_run_host_script() {
    export G8E_PKI_DIR="$G8E_PKI_DIR_HOST"
    export G8E_SECRETS_DIR="$G8E_SECRETS_DIR_HOST"
    export G8E_INTERNAL_HTTP_URL="$OPERATOR_HTTP_URL"
    export PYTHONPATH="$SCRIPT_DIR/scripts:$SCRIPT_DIR/shared${PYTHONPATH:+:$PYTHONPATH}"
    [[ -n "${OPERATOR_SESSION_ID:-}" ]] && export OPERATOR_SESSION_ID
    exec "$@"
}

_operator_bin() {
    local bin="${G8E_OPERATOR_BIN:-$SCRIPT_DIR/components/g8eo/build/linux-amd64/g8e.operator}"
    if [ ! -x "$bin" ]; then
        echo "[g8e] Operator binary missing at $bin — building..." >&2
        (cd "$SCRIPT_DIR/components/g8eo" && make build-local) >&2
    fi
    printf '%s' "$bin"
}

_load_credentials() {
    if [[ -f "$G8E_CREDENTIALS_FILE" ]]; then
        source "$G8E_CREDENTIALS_FILE"
        if [[ -n "$OPERATOR_SESSION_ID" ]]; then
            # Credentials loaded, session available
            :
        fi
        return 0
    fi
    return 1
}

_save_credentials() {
    local operator_session_id="$1"
    local user_id="$2"
    local operator_id="$3"
    mkdir -p "$G8E_CREDENTIALS_DIR"
    cat > "$G8E_CREDENTIALS_FILE" <<EOF
export OPERATOR_SESSION_ID="$operator_session_id"
export USER_ID="$user_id"
export OPERATOR_ID="$operator_id"
export G8E_AUTH_TIMESTAMP="$(date +%s)"
export G8E_CLI_CERT="$G8E_CLI_CERT_FILE"
export G8E_CLI_KEY="$G8E_CLI_KEY_FILE"
EOF
    chmod 600 "$G8E_CREDENTIALS_FILE"
}

_clear_credentials() {
    if [[ -f "$G8E_CREDENTIALS_FILE" ]]; then
        rm -f "$G8E_CREDENTIALS_FILE"
    fi
    rm -f "$G8E_CLI_CERT_FILE" "$G8E_CLI_KEY_FILE"
    unset OPERATOR_SESSION_ID USER_ID OPERATOR_ID G8E_AUTH_TIMESTAMP G8E_CLI_CERT G8E_CLI_KEY
}

_credentials_exist() {
    [[ -f "$G8E_CREDENTIALS_FILE" ]]
}

_ensure_authenticated() {
    if _load_credentials; then
        export OPERATOR_SESSION_ID USER_ID OPERATOR_ID G8E_CLI_CERT G8E_CLI_KEY
        return 0
    fi
    echo "[g8e] Not authenticated. Run: ./g8e login" >&2
    exit 1
}

_help() {
    local help_file="$SCRIPT_DIR/docs/g8e-help.md"
    if [[ -f "$help_file" ]]; then
        cat "$help_file"
    else
        echo "[g8e] Help file not found: $help_file" >&2
        exit 1
    fi
}
