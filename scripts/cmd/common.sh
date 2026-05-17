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

_operator_bootstrap() {
    local email="${G8E_BOOTSTRAP_EMAIL:-superadmin@g8e.local}"
    local name="${G8E_BOOTSTRAP_NAME:-Superadmin}"
    local public_port="${OPERATOR_LISTEN_PUBLIC_PORT:-9003}"
    local public_url="https://localhost:$public_port"
    local trust_bundle="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR_HOST/trust/hub-bundle.pem}"

    if [[ ! -f "$trust_bundle" ]]; then
        return 1
    fi

    # Check if already bootstrapped
    local status_resp
    status_resp=$(curl -sSk --cacert "$trust_bundle" "$public_url/api/auth/bootstrap/status" 2>/dev/null)
    if [[ $(echo "$status_resp" | jq -r '.bootstrapped' 2>/dev/null) == "true" ]]; then
        return 0
    fi

    _banner "auto-bootstrapping platform..."
    local bootstrap_body
    bootstrap_body=$(jq -n --arg email "$email" --arg name "$name" '{email: $email, name: $name}')
    
    # Perform bootstrap and capture cookies in memory
    local resp_headers
    resp_headers=$(curl -sS -i -k --cacert "$trust_bundle" \
        -X POST -H "Content-Type: application/json" \
        -d "$bootstrap_body" \
        "$public_url/api/auth/bootstrap" 2>/dev/null)

    local session_id
    session_id=$(echo "$resp_headers" | grep -i "Set-Cookie:" | grep "g8e_session=" | sed 's/.*g8e_session=\([^;]*\).*/\1/')

    if [[ -z "$session_id" ]]; then
        echo "[g8e] bootstrap failed: no session cookie returned" >&2
        echo "$resp_headers" >&2
        return 1
    fi

    # Save to credentials for persistent CLI auth, but avoid raw cookie jars
    local user_id
    user_id=$(echo "$resp_headers" | grep -v '^[A-Z]' | jq -r '.user.id' 2>/dev/null)
    _save_credentials "$session_id" "$user_id" "bootstrap"
    
    _banner "platform bootstrapped successfully (user: $email)"
    return 0
}

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
    else
        local cert_name
        cert_name=$(jq -r '.g8ee.cert_name // "g8ee"' "$SCRIPT_DIR/protocol/constants/paths.json" 2>/dev/null || echo "g8ee")
        if [[ -f "$G8E_PKI_DIR_HOST/issued/apps/${cert_name}.crt" && -f "$G8E_PKI_DIR_HOST/issued/apps/${cert_name}.key" ]]; then
            args+=(--cert "$G8E_PKI_DIR_HOST/issued/apps/${cert_name}.crt" --key "$G8E_PKI_DIR_HOST/issued/apps/${cert_name}.key")
        else
            echo "[g8e] No mTLS client certificate available — run: ./g8e login" >&2
            return 1
        fi
    fi

    if [[ -n "$OPERATOR_SESSION_ID" ]]; then
        args+=(-H "X-G8E-Operator-Session-ID: $OPERATOR_SESSION_ID")
        # Also send as a cookie for web-authenticated routes
        args+=(--cookie "g8e_session=$OPERATOR_SESSION_ID")
    fi

    args+=(-H "Content-Type: application/json")
    [[ -n "$body" ]] && args+=(-d "$body")
    curl "${args[@]}" "$OPERATOR_HTTP_URL$path"
}

_g8ee_url() {
    printf '%s' "${G8EE_URL:-https://localhost:8443}"
}

# Build curl args for an mTLS-authenticated request to either Operator or g8ee.
# Usage: _build_protocol_curl_args <out_array_name>
# Required env: G8E_PKI_DIR_HOST, G8E_CLI_CERT_FILE/G8E_CLI_KEY_FILE or app cert fallback.
_build_protocol_curl_args() {
    local _outvar="$1"
    local trust_bundle="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR_HOST/trust/hub-bundle.pem}"
    if [[ ! -f "$trust_bundle" ]]; then
        echo "[g8e] trust bundle not found at $trust_bundle — recreate runtime PKI: ./g8e platform clean && ./g8e platform start" >&2
        return 1
    fi
    local _args=(-sS --cacert "$trust_bundle")

    local cli_cert="${G8E_CLI_CERT:-$G8E_CLI_CERT_FILE}"
    local cli_key="${G8E_CLI_KEY:-$G8E_CLI_KEY_FILE}"
    if [[ -f "$cli_cert" && -f "$cli_key" ]]; then
        _args+=(--cert "$cli_cert" --key "$cli_key")
    else
        local cert_name
        cert_name=$(jq -r '.g8ee.cert_name // "g8ee"' "$SCRIPT_DIR/protocol/constants/paths.json" 2>/dev/null || echo "g8ee")
        if [[ -f "$G8E_PKI_DIR_HOST/issued/apps/${cert_name}.crt" && -f "$G8E_PKI_DIR_HOST/issued/apps/${cert_name}.key" ]]; then
            _args+=(--cert "$G8E_PKI_DIR_HOST/issued/apps/${cert_name}.crt" --key "$G8E_PKI_DIR_HOST/issued/apps/${cert_name}.key")
        else
            echo "[g8e] no mTLS client certificate available — run: ./g8e login" >&2
            return 1
        fi
    fi

    eval "$_outvar=(\"\${_args[@]}\")"
}

# Append the standard g8e HTTP context headers to a curl args array.
# Required env: OPERATOR_SESSION_ID, USER_ID. Optional: G8E_CASE_ID,
# G8E_INVESTIGATION_ID, G8E_BOUND_OPERATORS, G8E_TASK_ID.
# NOTE: G8E_NEW_CASE is deprecated - use resource_creation in request body.
# Usage: _append_g8e_context_headers <array_name>
_append_g8e_context_headers() {
    local _outvar="$1"
    eval "local _args=(\"\${${_outvar}[@]}\")"
    _args+=(-H "Content-Type: application/json")
    _args+=(-H "X-G8E-Source-Component: client")
    if [[ -n "${OPERATOR_SESSION_ID:-}" ]]; then
        _args+=(-H "X-G8E-Operator-Session-ID: $OPERATOR_SESSION_ID")
        _args+=(-H "X-G8E-WebSession-ID: $OPERATOR_SESSION_ID")
        _args+=(--cookie "g8e_session=$OPERATOR_SESSION_ID")
    fi
    [[ -n "${USER_ID:-}" ]]              && _args+=(-H "X-G8E-User-ID: $USER_ID")
    [[ -n "${G8E_CASE_ID:-}" ]]          && _args+=(-H "X-G8E-Case-ID: $G8E_CASE_ID")
    [[ -n "${G8E_INVESTIGATION_ID:-}" ]] && _args+=(-H "X-G8E-Investigation-ID: $G8E_INVESTIGATION_ID")
    [[ -n "${G8E_BOUND_OPERATORS:-}" ]]  && _args+=(-H "X-G8E-Bound-Operators: $G8E_BOUND_OPERATORS")
    [[ -n "${G8E_TASK_ID:-}" ]]          && _args+=(-H "X-G8E-Task-ID: $G8E_TASK_ID")
    eval "$_outvar=(\"\${_args[@]}\")"
}

# POST/GET to g8ee using mTLS + g8e context headers.
# Usage: _g8ee_curl <method> <path> [body]
_g8ee_curl() {
    local method="$1" path="$2" body="${3:-}"
    local args=()
    _build_protocol_curl_args args || return 1
    args+=(-X "$method")
    _append_g8e_context_headers args
    [[ -n "$body" ]] && args+=(-d "$body")
    curl "${args[@]}" "$(_g8ee_url)$path"
}

_run_host_script() {
    export G8E_PKI_DIR="$G8E_PKI_DIR_HOST"
    export G8E_SECRETS_DIR="$G8E_SECRETS_DIR_HOST"
    export G8E_INTERNAL_HTTP_URL="$OPERATOR_HTTP_URL"
    export PYTHONPATH="$SCRIPT_DIR/scripts:$SCRIPT_DIR/protocol${PYTHONPATH:+:$PYTHONPATH}"
    [[ -n "${OPERATOR_SESSION_ID:-}" ]] && export OPERATOR_SESSION_ID
    exec "$@"
}

_operator_bin() {
    local bin="${G8E_OPERATOR_BIN:-$SCRIPT_DIR/services/g8eo/build/linux-amd64/g8e.operator}"
    if [ ! -x "$bin" ]; then
        echo "[g8e] Operator binary missing at $bin — building..." >&2
        (cd "$SCRIPT_DIR/services/g8eo" && make build-local) >&2
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
    # Ensure directory has restricted permissions
    chmod 700 "$G8E_CREDENTIALS_DIR"
    cat > "$G8E_CREDENTIALS_FILE" <<EOF
export OPERATOR_SESSION_ID="$operator_session_id"
export USER_ID="$user_id"
export OPERATOR_ID="$operator_id"
export G8E_AUTH_TIMESTAMP="$(date +%s)"
export G8E_CLI_CERT="$G8E_CLI_CERT_FILE"
export G8E_CLI_KEY="$G8E_CLI_KEY_FILE"
EOF
    # Restrict file permissions to user-only
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
        # Validate the saved CLI certificate against the *current* trust bundle.
        # `./g8e platform clean` regenerates the runtime PKI but leaves the
        # per-user credentials in $HOME/.g8e untouched, so without this check
        # the CLI presents a cert signed by a previous Root CA and every mTLS
        # handshake fails with `x509: ECDSA verification failure`. We refuse to
        # paper over that — clear the stale credentials and require re-login so
        # the cause is obvious.
        local trust_bundle="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR_HOST/trust/hub-bundle.pem}"
        local cert_file="${G8E_CLI_CERT:-$G8E_CLI_CERT_FILE}"
        local key_file="${G8E_CLI_KEY:-$G8E_CLI_KEY_FILE}"
        if [[ ! -f "$trust_bundle" ]]; then
            echo "[g8e] Trust bundle not found at $trust_bundle — start the platform first: ./g8e platform start" >&2
            exit 1
        fi
        if [[ ! -f "$cert_file" || ! -f "$key_file" ]]; then
            echo "[g8e] CLI certificate is missing — re-authenticate: ./g8e login" >&2
            _clear_credentials
            exit 1
        fi
        if ! openssl verify -CAfile "$trust_bundle" "$cert_file" >/dev/null 2>&1; then
            echo "[g8e] CLI certificate is no longer trusted by the current Operator CA (likely after ./g8e platform clean). Re-authenticate: ./g8e login" >&2
            _clear_credentials
            exit 1
        fi
        export OPERATOR_SESSION_ID USER_ID OPERATOR_ID G8E_CLI_CERT G8E_CLI_KEY
        return 0
    fi
    echo "[g8e] Not authenticated. Run: ./g8e login" >&2
    exit 1
}

_help() {
    local help_file="$SCRIPT_DIR/docs/general/cli_help.md"
    if [[ -f "$help_file" ]]; then
        cat "$help_file"
    else
        echo "[g8e] Help file not found: $help_file" >&2
        exit 1
    fi
}
