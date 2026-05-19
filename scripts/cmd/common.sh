#!/usr/bin/env bash

# Common helpers and environment for g8e CLI commands.
# This file is intended to be sourced by specific command scripts.

# Use canonical root detection heuristic
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$SCRIPT_DIR/scripts/core/path_utils.sh"
G8E_PROJECT_ROOT="$(resolve_g8e_root)"
export G8E_PROJECT_ROOT
source "$SCRIPT_DIR/scripts/cmd/headers.sh"
source "$SCRIPT_DIR/scripts/cmd/env_vars.sh"
source "$SCRIPT_DIR/scripts/cmd/paths.sh"
source "$SCRIPT_DIR/scripts/core/config.sh"

# Host-native runtime layout
G8E_RUNTIME_DIR="${G8E_RUNTIME_DIR:-$SCRIPT_DIR/.g8e}"
G8E_PKI_DIR_HOST="${G8E_PKI_DIR:-$SCRIPT_DIR/$G8E_PATH_PKI_DIR}"
G8E_SECRETS_DIR_HOST="${G8E_SECRETS_DIR:-$SCRIPT_DIR/$G8E_PATH_SECRETS_DIR}"
OPERATOR_HTTP_URL="${G8E_INTERNAL_HTTP_URL:-https://localhost:$G8E_PORT_OPERATOR_HTTP}"
_OPERATOR_PID_FILE="$G8E_RUNTIME_DIR/pids/operator-listen.pid"
_G8EE_PID_FILE="$G8E_RUNTIME_DIR/pids/g8ee.pid"

# Local credential store
G8E_CREDENTIALS_DIR="$HOME/.g8e"
G8E_CREDENTIALS_FILE="$G8E_CREDENTIALS_DIR/credentials"
G8E_CLI_CERT_FILE="$G8E_CREDENTIALS_DIR/cli.crt"
G8E_CLI_KEY_FILE="$G8E_CREDENTIALS_DIR/cli.key"
G8E_OPERATOR_CERT_FILE="$G8E_CREDENTIALS_DIR/operator.crt"
G8E_OPERATOR_KEY_FILE="$G8E_CREDENTIALS_DIR/operator.key"

_banner() {
    echo -e "\033[1;34m[g8e]\033[0m $*"
}

_pid_alive() {
    local pid_file="$1"
    [ -f "$pid_file" ] && ps -p "$(cat "$pid_file")" > /dev/null 2>&1
}

_g8ee_running()    { _pid_alive "$_G8EE_PID_FILE"; }
_operator_running() { _pid_alive "$_OPERATOR_PID_FILE"; }

# _generate_workload_csrs generates ECDSA P-256 keys and CSRs for both
# Operator and CLI identities.
# Outputs: sets _op_key_file, _op_csr_pem, _cli_key_file, _cli_csr_pem
_generate_workload_csrs() {
    local tmp_dir="$1"
    _op_key_file="${tmp_dir}/operator.key"
    _cli_key_file="${tmp_dir}/cli.key"
    
    # 1. Operator CSR
    openssl ecparam -name prime256v1 -genkey -noout -out "$_op_key_file" 2>/dev/null
    chmod 600 "$_op_key_file"
    _op_csr_pem=$(openssl req -new -key "$_op_key_file" \
        -subj "/CN=g8e-operator-$(hostname)/O=g8e" 2>/dev/null)
    
    # 2. CLI CSR
    openssl ecparam -name prime256v1 -genkey -noout -out "$_cli_key_file" 2>/dev/null
    chmod 600 "$_cli_key_file"
    _cli_csr_pem=$(openssl req -new -key "$_cli_key_file" \
        -subj "/CN=g8e-cli-$(hostname)/O=g8e" 2>/dev/null)

    if [[ -z "$_op_csr_pem" || -z "$_cli_csr_pem" ]]; then
        echo "[g8e] Failed to generate CSRs" >&2
        return 1
    fi
    return 0
}

_operator_bootstrap() {
    local email="${G8E_BOOTSTRAP_EMAIL:-superadmin@g8e.local}"
    local name="${G8E_BOOTSTRAP_NAME:-Superadmin}"
    local public_port="${OPERATOR_LISTEN_PUBLIC_PORT:-$G8E_PORT_OPERATOR_PUBLIC}"
    local public_url="https://localhost:$public_port"
    local trust_bundle="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR_HOST/trust/hub-bundle.pem}"

    if [[ ! -f "$trust_bundle" ]]; then
        return 1
    fi

    # Check if already bootstrapped AND we have valid local credentials - no rotation on normal start
    local status_resp
    status_resp=$(curl -sS --cacert "$trust_bundle" "$public_url/api/auth/bootstrap/status" 2>/dev/null)
    if [[ $(echo "$status_resp" | jq -r '.bootstrapped' 2>/dev/null) == "true" ]]; then
        # Check if local credentials exist and are valid
        if [[ -f "$G8E_CLI_CERT_FILE" ]] && openssl x509 -noout -in "$G8E_CLI_CERT_FILE" -checkend 0 >/dev/null 2>&1; then
            return 0
        fi
        # If bootstrapped but local cert is missing/invalid, we proceed to re-bootstrap (rotation)
    fi

    _banner "auto-bootstrapping platform..."

    # Generate CSR for CLI mTLS cert (plan §4.3)
    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    if ! _generate_workload_csrs "$tmp_dir"; then
        echo "[g8e] bootstrap failed: CSR generation failed" >&2
        return 1
    fi

    local system_fingerprint
    system_fingerprint=$(echo "g8e-cli-$(hostname)-$(whoami)" | sha256sum | awk '{print $1}')

    # Build bootstrap request with CSRs (plan §4.3)
    local bootstrap_body
    bootstrap_body=$(jq -n \
        --arg email "$email" \
        --arg name "$name" \
        --arg op_csr "$_op_csr_pem" \
        --arg cli_csr "$_cli_csr_pem" \
        --arg fingerprint "$system_fingerprint" \
        '{email: $email, name: $name, csr_pem: $op_csr, cli_csr_pem: $cli_csr, system_fingerprint: $fingerprint}')

    # Perform bootstrap over loopback
    local resp
    resp=$(curl -sS --cacert "$trust_bundle" \
        -X POST -H "${G8E_HEADER_CONTENT_TYPE}: application/json" \
        -d "$bootstrap_body" \
        "$public_url/api/auth/bootstrap")

    local success
    success=$(echo "$resp" | jq -r '.success' 2>/dev/null)
    if [[ "$success" != "true" ]]; then
        echo "[g8e] bootstrap failed: $resp" >&2
        return 1
    fi

    # Extract response fields (plan §4.3).
    # operator_session_id authenticates the host agent (mTLS-bound).
    # cli_session_id is the strictly disjoint routing namespace this CLI uses
    # to receive SessionEvents and embed in outbound request bodies. The
    # substrate refuses to conflate the two session types.
    local operator_id operator_session_id cli_session_id operator_cert operator_cert_chain cli_cert cli_cert_chain hub_trust_bundle
    operator_id=$(echo "$resp" | jq -r '.operator_id' 2>/dev/null)
    operator_session_id=$(echo "$resp" | jq -r '.operator_session_id' 2>/dev/null)
    cli_session_id=$(echo "$resp" | jq -r '.cli_session_id' 2>/dev/null)
    operator_cert=$(echo "$resp" | jq -r '.operator_cert' 2>/dev/null)
    operator_cert_chain=$(echo "$resp" | jq -r '.operator_cert_chain' 2>/dev/null)
    cli_cert=$(echo "$resp" | jq -r '.cli_cert' 2>/dev/null)
    cli_cert_chain=$(echo "$resp" | jq -r '.cli_cert_chain' 2>/dev/null)
    hub_trust_bundle=$(echo "$resp" | jq -r '.hub_trust_bundle' 2>/dev/null)
    local user_id
    user_id=$(echo "$resp" | jq -r '.user.id' 2>/dev/null)

    if [[ -z "$operator_id" || -z "$operator_session_id" || -z "$operator_cert" || -z "$cli_session_id" || "$cli_session_id" == "null" || -z "$cli_cert" ]]; then
        echo "[g8e] bootstrap failed: incomplete response (missing operator_id, operator_session_id, cli_session_id, operator_cert, or cli_cert)" >&2
        echo "$resp" >&2
        return 1
    fi

    # Write CLI cert (chain already includes leaf) to $HOME/.g8e/ (plan §4.3)
    mkdir -p "$G8E_CREDENTIALS_DIR"
    if [[ -n "$cli_cert_chain" ]]; then
        printf '%s\n' "$cli_cert_chain" > "$G8E_CLI_CERT_FILE"
    else
        printf '%s\n' "$cli_cert" > "$G8E_CLI_CERT_FILE"
    fi
    chmod 600 "$G8E_CLI_CERT_FILE"

    # Write CLI key
    cp "$_cli_key_file" "$G8E_CLI_KEY_FILE"
    chmod 600 "$G8E_CLI_KEY_FILE"

    # Write Operator cert
    if [[ -n "$operator_cert_chain" ]]; then
        printf '%s\n' "$operator_cert_chain" > "$G8E_OPERATOR_CERT_FILE"
    else
        printf '%s\n' "$operator_cert" > "$G8E_OPERATOR_CERT_FILE"
    fi
    chmod 600 "$G8E_OPERATOR_CERT_FILE"

    # Write Operator key
    cp "$_op_key_file" "$G8E_OPERATOR_KEY_FILE"
    chmod 600 "$G8E_OPERATOR_KEY_FILE"

    # Write hub trust bundle
    if [[ -n "$hub_trust_bundle" ]]; then
        printf '%s\n' "$hub_trust_bundle" > "$G8E_CREDENTIALS_DIR/hub-bundle.pem"
        chmod 600 "$G8E_CREDENTIALS_DIR/hub-bundle.pem"
    fi

    # Save credentials
    _save_credentials "$operator_session_id" "$user_id" "$operator_id" "$cli_session_id"

    _banner "platform bootstrapped successfully (user: $email, operator_id: $operator_id)"
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

    if [[ -n "$G8E_OPERATOR_SESSION_ID" ]]; then
        args+=(-H "${G8E_HEADER_AUTHORIZATION}: Bearer $G8E_OPERATOR_SESSION_ID")
        # Also send as a cookie for web-authenticated routes
        args+=(--cookie "g8e_session=$G8E_OPERATOR_SESSION_ID")
    fi

    args+=(-H "${G8E_HEADER_CONTENT_TYPE}: application/json")
    [[ -n "$body" ]] && args+=(-d "$body")
    curl "${args[@]}" "$OPERATOR_HTTP_URL$path"
}

_g8ee_url() {
    printf '%s' "${G8E_G8EE_URL:-https://localhost:$G8E_PORT_G8EE_HTTP}"
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

# Append the minimal g8e HTTP substrate auth headers to a curl args array.
# Required env: OPERATOR_SESSION_ID or CLI_SESSION_ID.
# Usage: _append_g8e_auth_headers <array_name>
_append_g8e_auth_headers() {
    local _outvar="$1"
    eval "local __auth_args=(\"\${${_outvar}[@]}\")"
    __auth_args+=(-H "${G8E_HEADER_CONTENT_TYPE}: application/json")
    if [[ -n "${G8E_OPERATOR_SESSION_ID:-}" ]]; then
        # Substrate uses Authorization: Bearer <token>.
        __auth_args+=(-H "${G8E_HEADER_AUTHORIZATION}: Bearer $G8E_OPERATOR_SESSION_ID")
        __auth_args+=(--cookie "g8e_session=$G8E_OPERATOR_SESSION_ID")
    fi
    if [[ -n "${G8E_CLI_SESSION_ID:-}" ]]; then
        __auth_args+=(-H "${G8E_HEADER_X_SESSION_ID}: $G8E_CLI_SESSION_ID")
    fi
    eval "$_outvar=(\"\${__auth_args[@]}\")"
}


# POST/GET to g8ee using mTLS + g8e auth headers.
# Usage: _g8ee_curl <method> <path> [body]
_g8ee_curl() {
    local method="$1" path="$2" body="${3:-}"
    local args=()
    _build_protocol_curl_args args || return 1
    args+=(-X "$method")
    _append_g8e_auth_headers args
    [[ -n "$body" ]] && args+=(-d "$body")
    curl "${args[@]}" "$(_g8ee_url)$path"
}

_run_host_script() {
    export G8E_PKI_DIR="$G8E_PKI_DIR_HOST"
    export G8E_SECRETS_DIR="$G8E_SECRETS_DIR_HOST"
    export G8E_INTERNAL_HTTP_URL="$OPERATOR_HTTP_URL"
    export PYTHONPATH="$SCRIPT_DIR/scripts:$SCRIPT_DIR/protocol${PYTHONPATH:+:$PYTHONPATH}"
    [[ -n "${G8E_OPERATOR_SESSION_ID:-}" ]] && export G8E_OPERATOR_SESSION_ID
    exec "$@"
}

_operator_bin() {
    local bin="${G8E_OPERATOR_BIN:-$SCRIPT_DIR/services/g8eo/build/linux-amd64/g8e.operator}"
    # Always run make build-local - Go's build caching makes this cheap when nothing changed
    (cd "$SCRIPT_DIR/services/g8eo" && make build-local) >&2
    printf '%s' "$bin"
}

_load_credentials() {
    if [[ -f "$G8E_CREDENTIALS_FILE" ]]; then
        source "$G8E_CREDENTIALS_FILE"
        if [[ -n "$OPERATOR_SESSION_ID" ]]; then
            # Credentials loaded, operator session available
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
    local cli_session_id="$4"
    mkdir -p "$G8E_CREDENTIALS_DIR"
    # Ensure directory has restricted permissions
    chmod 700 "$G8E_CREDENTIALS_DIR"
    cat > "$G8E_CREDENTIALS_FILE" <<EOF
export $G8E_ENV_OPERATOR_SESSION_ID="$operator_session_id"
export $G8E_ENV_CLI_SESSION_ID="$cli_session_id"
export $G8E_ENV_USER_ID="$user_id"
export $G8E_ENV_OPERATOR_ID="$operator_id"
export G8E_AUTH_TIMESTAMP="$(date +%s)"
export G8E_CLI_CERT="$G8E_CLI_CERT_FILE"
export G8E_CLI_KEY="$G8E_CLI_KEY_FILE"
export G8E_OPERATOR_CERT="$G8E_OPERATOR_CERT_FILE"
export G8E_OPERATOR_KEY="$G8E_OPERATOR_KEY_FILE"
EOF
    # Restrict file permissions to user-only
    chmod 600 "$G8E_CREDENTIALS_FILE"
}

_clear_credentials() {
    if [[ -f "$G8E_CREDENTIALS_FILE" ]]; then
        rm -f "$G8E_CREDENTIALS_FILE"
    fi
    rm -f "$G8E_CLI_CERT_FILE" "$G8E_CLI_KEY_FILE" "$G8E_OPERATOR_CERT_FILE" "$G8E_OPERATOR_KEY_FILE"
    unset $G8E_ENV_OPERATOR_SESSION_ID $G8E_ENV_CLI_SESSION_ID $G8E_ENV_USER_ID $G8E_ENV_OPERATOR_ID G8E_AUTH_TIMESTAMP G8E_CLI_CERT G8E_CLI_KEY G8E_OPERATOR_CERT G8E_OPERATOR_KEY
}

_check_g8e_error() {
    local resp="$1" label="${2:-request}"
    if [[ -z "$resp" ]]; then
        return 0
    fi
    
    # Check for structured g8e error response
    if echo "$resp" | jq -e '.error' >/dev/null 2>&1; then
        local code message component
        code=$(echo "$resp" | jq -r '.error.code // "UNKNOWN"')
        message=$(echo "$resp" | jq -r '.error.message // "An unexpected error occurred"')
        component=$(echo "$resp" | jq -r '.error.component // "unknown"')
        
        echo -e "\033[1;31m[g8e error]\033[0m \033[1m$code\033[0m ($component): $message" >&2
        
        # If there are remediation steps, show them
        if echo "$resp" | jq -e '.error.remediation_steps | length > 0' >/dev/null 2>&1; then
            echo -e "\033[1;34mRemediation:\033[0m" >&2
            echo "$resp" | jq -r '.error.remediation_steps[]' | sed 's/^/  - /' >&2
        fi
        
        exit 1
    fi
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
        local trust_bundle="${G8E_TRUST_BUNDLE:-}"
        if [[ -z "$trust_bundle" ]]; then
            if [[ -f "$G8E_CREDENTIALS_DIR/hub-bundle.pem" ]]; then
                trust_bundle="$G8E_CREDENTIALS_DIR/hub-bundle.pem"
            else
                trust_bundle="$G8E_PKI_DIR_HOST/trust/hub-bundle.pem"
            fi
        fi
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
        if ! openssl verify -CAfile "$trust_bundle" "$cert_file"; then
            echo "[g8e] CLI certificate is no longer trusted by the current Operator CA (likely after ./g8e platform clean). Re-authenticate: ./g8e login" >&2
            _clear_credentials
            exit 1
        fi
        export $G8E_ENV_OPERATOR_SESSION_ID $G8E_ENV_USER_ID $G8E_ENV_OPERATOR_ID G8E_CLI_CERT G8E_CLI_KEY G8E_OPERATOR_CERT G8E_OPERATOR_KEY
        return 0
    fi
    echo "[g8e] Not authenticated. Run: ./g8e login" >&2
    exit 1
}

_help() {
    local help_file="$SCRIPT_DIR/docs/cli_help.md"
    if [[ -f "$help_file" ]]; then
        cat "$help_file"
    else
        echo "[g8e] Help file not found: $help_file" >&2
        exit 1
    fi
}
