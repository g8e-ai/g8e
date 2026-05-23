#!/usr/bin/env bash
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

set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
source "$(dirname "${BASH_SOURCE[0]}")/app_helpers.sh"

TOP="$1"
SUB="${2:-}"

case "$TOP" in
    vars)
        case "$SUB" in
            -h|--help|"")
                cat <<'EOF'
### vars (Environment variables)
Environment variable management.

Subcommands:
  list, ls  List all g8e environment variables and their current values
  set <key> <value>  Set a variable in .g8e/.env
  get <key>  Display the value of a specific variable
  unset <key>  Remove a variable from .g8e/.env
EOF
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
        esac
        _banner "vars ${SUB} ${@:3}"
        exec bash "$SCRIPT_DIR/scripts/core/manage-env.sh" "$SUB" "${@:3}" ;;

    login)
        case "$SUB" in
            -h|--help|"")
                cat <<'EOF'
### identity (Authentication)
Authentication and session management.

Subcommands:
  login [--email <email>] [--count <n>] [--ttl <seconds>]  Authenticate and save operator session to ~/.g8e/credentials
  logout  Clear local operator session and credentials
EOF
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
        esac
        _banner "login"
        _ensure_operator

        # Parse flags
        _login_email=""
        _dl_count=1
        _dl_ttl=3600
        _args=("${@:2}")
        i=0
        while [[ $i -lt ${#_args[@]} ]]; do
            case "${_args[$i]}" in
                --email)   i=$((i+1)); _login_email="${_args[$i]}" ;;
                --email=*) _login_email="${_args[$i]#--email=}" ;;
                --count)   i=$((i+1)); _dl_count="${_args[$i]}" ;;
                --count=*) _dl_count="${_args[$i]#--count=}" ;;
                --ttl)     i=$((i+1)); _dl_ttl="${_args[$i]}" ;;
                --ttl=*)   _dl_ttl="${_args[$i]#--ttl=}" ;;
            esac
            i=$((i+1))
        done

        # Default to the sandbox bootstrap superuser when --email is omitted.
        # Frictionless onboarding: `./g8e login` works zero-arg in sandbox mode.
        # Override with G8E_BOOTSTRAP_EMAIL or --email <addr> for non-default users.
        if [[ -z "$_login_email" ]]; then
            _login_email="${G8E_BOOTSTRAP_EMAIL:-superadmin@g8e.local}"
        fi

        _trust_bundle="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR_HOST/trust/hub-bundle.pem}"
        _bootstrap_port="${G8E_REMOTE_OPERATOR_BOOTSTRAP_HTTPS_PORT:-$G8E_PORT_OPERATOR_BOOTSTRAP_HTTPS}"
        _bootstrap_url="${G8E_BOOTSTRAP_URL:-http://localhost:$_bootstrap_port}"

        if [[ ! -f "$_trust_bundle" ]]; then
            echo "[g8e] Trust bundle not found at $_trust_bundle - start the platform first: ./g8e platform start" >&2
            exit 1
        fi

        # 1. Request a device-link token via bootstrap port (unauthenticated)
        echo "  Requesting device-link token..."
        _fingerprint=$(echo "g8e-cli-$(hostname)-$(whoami)" | _sha256)
        _dl_body=$(python3 -c "import json, sys; print(json.dumps({'email': sys.argv[1], 'name': sys.argv[2], 'max_uses': int(sys.argv[3]), 'ttl_seconds': int(sys.argv[4])}))" \
            "$_login_email" "cli-$(hostname)" "$_dl_count" "$_dl_ttl")
        _dl_resp=$( curl -sS \
            -X POST -H "${G8E_HEADER_CONTENT_TYPE}: application/json" \
            -d "$_dl_body" \
            "$_bootstrap_url/api/auth/device-link/request" 2>&1 )
        _dl_token=$(echo "$_dl_resp" | python3 "$G8E_PROJECT_ROOT/scripts/core/json_query.py" - token 2>/dev/null)
        _login_user_id=$(echo "$_dl_resp" | python3 "$G8E_PROJECT_ROOT/scripts/core/json_query.py" - user_id 2>/dev/null)
        if [[ -z "$_dl_token" ]]; then
            echo "[g8e] Failed to create device-link: $_dl_resp" >&2
            exit 1
        fi
        echo "  Device-link token obtained: ${_dl_token:0:12}... (count=$_dl_count, ttl=$_dl_ttl s)"

        # 3. Generate ECDSA private keys + CSRs
        echo "  Generating keys and CSRs..."
        _tmp_dir=$(mktemp -d)
        trap 'rm -rf "$_tmp_dir"' EXIT
        if ! _generate_workload_csrs "$_tmp_dir"; then
            echo "[g8e] CSR generation failed" >&2
            exit 1
        fi

        # 4. Register via bootstrap port (no mTLS required on this route)
        echo "  Registering with operator..."
        _reg_body=$(python3 -c "import json, sys; print(json.dumps({
            'system_fingerprint': sys.argv[1],
            'hostname': sys.argv[2],
            'os': 'linux',
            'arch': sys.argv[3],
            'username': sys.argv[4],
            'csr_pem': sys.argv[5],
            'cli_csr_pem': sys.argv[6]
        }))" "$_fingerprint" "$(hostname)" "$(uname -m)" "${USER:-$LOGNAME}" "$_op_csr_pem" "$_cli_csr_pem")
        _reg_resp=$( curl -sS \
            -X POST -H "${G8E_HEADER_CONTENT_TYPE}: application/json" \
            -H "${G8E_HEADER_DEVICE_TOKEN}: $_dl_token" \
            -d "$_reg_body" \
            "$_bootstrap_url/api/auth/device-link/register" 2>&1 )

        _reg_error=$(echo "$_reg_resp" | python3 "$G8E_PROJECT_ROOT/scripts/core/json_query.py" - error 2>/dev/null)
        if [[ -n "$_reg_error" ]]; then
            echo "[g8e] Registration failed: $_reg_error" >&2
            exit 1
        fi

        # 5. Extract and save results.
        # cli_session_id is the disjoint BYO/CLI routing namespace minted at
        # login alongside operator_session_id. The CLI must NEVER reuse the
        # operator_session_id as a cli session - those are first-class disjoint
        # session types.
        _session_id=$(echo "$_reg_resp" | python3 "$G8E_PROJECT_ROOT/scripts/core/json_query.py" - operator_session_id 2>/dev/null)
        _cli_session_id=$(echo "$_reg_resp" | python3 "$G8E_PROJECT_ROOT/scripts/core/json_query.py" - cli_session_id 2>/dev/null)
        _operator_id=$(echo "$_reg_resp" | python3 "$G8E_PROJECT_ROOT/scripts/core/json_query.py" - operator_id 2>/dev/null)
        _op_cert_pem=$(echo "$_reg_resp" | python3 "$G8E_PROJECT_ROOT/scripts/core/json_query.py" - operator_cert 2>/dev/null)
        _op_chain_pem=$(echo "$_reg_resp" | python3 "$G8E_PROJECT_ROOT/scripts/core/json_query.py" - operator_cert_chain 2>/dev/null)
        _cli_cert_pem=$(echo "$_reg_resp" | python3 "$G8E_PROJECT_ROOT/scripts/core/json_query.py" - cli_cert 2>/dev/null)
        _cli_chain_pem=$(echo "$_reg_resp" | python3 "$G8E_PROJECT_ROOT/scripts/core/json_query.py" - cli_cert_chain 2>/dev/null)
        _hub_bundle=$(echo "$_reg_resp" | python3 "$G8E_PROJECT_ROOT/scripts/core/json_query.py" - hub_trust_bundle 2>/dev/null)

        if [[ -z "$_session_id" || -z "$_operator_id" || -z "$_op_cert_pem" || -z "$_cli_session_id" || -z "$_cli_cert_pem" ]]; then
            echo "[g8e] Unexpected registration response (missing operator_session_id, cli_session_id, operator_id, operator_cert, or cli_cert): $_reg_resp" >&2
            exit 1
        fi

        # Write CLI cert (leaf + chain)
        printf '%s\n' "$_cli_cert_pem" > "$G8E_CLI_CERT_FILE"
        if [[ -n "$_cli_chain_pem" ]]; then
            printf '%s\n' "$_cli_chain_pem" >> "$G8E_CLI_CERT_FILE"
        fi
        chmod 600 "$G8E_CLI_CERT_FILE"

        # Write CLI key
        cp "$_cli_key_file" "$G8E_CLI_KEY_FILE"
        chmod 600 "$G8E_CLI_KEY_FILE"

        # Write Operator cert (leaf + chain)
        printf '%s\n' "$_op_cert_pem" > "$G8E_OPERATOR_CERT_FILE"
        if [[ -n "$_op_chain_pem" ]]; then
            printf '%s\n' "$_op_chain_pem" >> "$G8E_OPERATOR_CERT_FILE"
        fi
        chmod 600 "$G8E_OPERATOR_CERT_FILE"

        # Write Operator key
        cp "$_op_key_file" "$G8E_OPERATOR_KEY_FILE"
        chmod 600 "$G8E_OPERATOR_KEY_FILE"

        # Update trust bundle if operator returned a fresher one
        if [[ -n "$_hub_bundle" ]]; then
            printf '%s\n' "$_hub_bundle" > "$G8E_CREDENTIALS_DIR/hub-bundle.pem"
            chmod 600 "$G8E_CREDENTIALS_DIR/hub-bundle.pem"
        fi

        _save_credentials "$_session_id" "$_login_user_id" "$_operator_id" "$_cli_session_id"

        echo -e "\n\033[1;32mAuthenticated as $_login_email\033[0m"
        echo -e "  mTLS cert:   \033[2m$G8E_CLI_CERT_FILE\033[0m"
        echo -e "  Credentials: \033[2m$G8E_CREDENTIALS_FILE\033[0m"
        echo -e "  \033[2mSession + operator IDs are stored locally; no need to copy them into commands.\033[0m"

        echo -e "\n\033[1mDeploy to a remote host (optional):\033[0m"
        echo -e "  \033[1;34m./g8e operator deploy <user@host> --endpoint $(hostname -I | awk '{print $1}') --device-token $_dl_token\033[0m"

        echo -e "\n\033[1mNext steps:\033[0m"
        echo -e "  - Create device links:    \033[1;34m./g8e data device-links create --count 10\033[0m"
        echo -e "  - Run benchmarks:         \033[1;34m./g8e evals bench --suite ifeval --mode receipt\033[0m"
        echo -e "  - Start chatting:         \033[1;34m./g8e chat\033[0m"
        echo -e "  - Check platform status:  \033[1;34m./g8e platform status\033[0m"
        echo -e "  - Explore CLI help:       \033[1;34m./g8e --help\033[0m"
        exit 0 ;;

    logout)
        _banner "logout"
        if _credentials_exist; then
            _clear_credentials
            echo "[g8e] Operator session cleared from $G8E_CREDENTIALS_FILE"
        else
            echo "[g8e] No active operator session found"
        fi
        exit 0 ;;

    ssh)
        case "$SUB" in
            -h|--help|"")
                cat <<'EOF'
### ssh
Manage host SSH key mounts.

Subcommands:
  setup  Configure SSH key mounting for operator fleet access
EOF
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            setup)
                _banner "ssh setup"; exec bash "$SCRIPT_DIR/scripts/tools/setup-ssh.sh" "${@:3}" ;;
            *)
                echo "[g8e] unknown ssh subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    aws)
        case "$SUB" in
            -h|--help|"")
                cat <<'EOF'
### aws
Manage AWS credential mounts.

Subcommands:
  setup  Configure AWS credential mounting for operator fleet access
EOF
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            setup)
                _banner "aws setup"; exec bash "$SCRIPT_DIR/scripts/tools/setup-aws.sh" "${@:3}" ;;
            *)
                echo "[g8e] unknown aws subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    search)
        case "$SUB" in
            -h|--help|"")
                cat <<'EOF'
### search
Vertex AI Search configuration.

Subcommands:
  setup    Configure Vertex AI Search integration
  disable  Disable Vertex AI Search integration
EOF
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            setup)   _banner "search setup"; exec bash "$SCRIPT_DIR/scripts/tools/setup-search.sh" "${@:3}" ;;
            disable) _banner "search disable"; exec bash "$SCRIPT_DIR/scripts/tools/setup-search.sh" --disable "${@:3}" ;;
            *)       echo "[g8e] unknown search subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    llm)
        case "$SUB" in
            -h|--help|"")
                cat <<'EOF'
### llm
LLM configuration.

Subcommands:
  setup    Interactive provider configuration
  show     View current LLM variables
  get      Display a specific LLM variable
  set      Update an LLM variable
  restart  Restart Ensemble to apply settings
EOF
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            setup|show|get|set|restart)
                exec bash "$SCRIPT_DIR/scripts/tools/setup-llm.sh" "$SUB" "${@:3}" ;;
            *)
                echo "[g8e] unknown llm subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    security)
        case "$SUB" in
            -h|--help|"")
                cat <<'EOF'
### security
Security validation.

Subcommands:
  validate              Check TLS integrity and volume permissions
  passkeys              Manage FIDO2/WebAuthn credentials
  mtls-test             Verify mTLS connectivity
  scan-licenses         Scan third-party licenses
  rotate-internal-token Rotate the internal auth token
EOF
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            validate)
                _banner "security validate"
                _run_host_script bash "$SCRIPT_DIR/scripts/security/validate-platform-security.sh" "${@:3}" ;;
            mtls-test)
                _banner "security mtls-test"
                _run_host_script bash "$SCRIPT_DIR/scripts/security/mtls-test.sh" "${@:3}" ;;
            scan-licenses)
                _banner "security scan-licenses"
                _run_host_script bash "$SCRIPT_DIR/scripts/security/scan-licenses.sh" "${@:3}" ;;
            passkeys)
                _banner "security passkeys ${@:3}"; _ensure_operator; _requires_operator_route "/api/security/passkeys" ;;
            rotate-internal-token)
                _banner "security rotate-internal-token"; _ensure_operator; _requires_operator_route "/api/security/internal-token/rotate" ;;
            *)
                echo "[g8e] unknown security subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    data)
        case "$SUB" in
            -h|--help|"")
                cat <<'EOF'
### data
Data management.

Subcommands:
  users         Query or modify user documents
  operators     Query or modify operator documents
  store         Access the SQLite-based blob store
  settings      Low-level platform configuration management
  audit         View LFAA audit logs
  device-links  Manage device link tokens
EOF
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            users)
                _banner "data users ${@:3}"; _ensure_operator
                _run_host_script python3 "$SCRIPT_DIR/scripts/data/manage-users.py" "${@:3}" ;;
            operators)
                _banner "data operators ${@:3}"; _ensure_operator
                _run_host_script python3 "$SCRIPT_DIR/scripts/data/manage-operators.py" "${@:3}" ;;
            store)
                _banner "data store ${@:3}"; _ensure_operator
                _run_host_script python3 "$SCRIPT_DIR/scripts/data/manage-store.py" "${@:3}" ;;
            settings)
                _banner "data settings ${@:3}"; _ensure_operator
                _run_host_script python3 "$SCRIPT_DIR/scripts/data/manage-settings.py" "${@:3}" ;;
            device-links)
                _banner "data device-links ${@:3}"; _ensure_operator
                _run_host_script python3 "$SCRIPT_DIR/scripts/data/manage-device-links.py" "${@:3}" ;;
            audit)
                _banner "data audit ${@:3}"
                # manage-lfaa.py interacts with the SQLite audit vault directly and
                # does not require the Operator API to be running for local summary.
                _run_host_script python3 "$SCRIPT_DIR/scripts/data/manage-lfaa.py" "${@:3}" ;;
            *)
                echo "[g8e] unknown data subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    mcp)
        case "$SUB" in
            -h|--help|"")
                cat <<'EOF'
### mcp
Model Context Protocol - generates configs for and interacts with the Operator MCP translation gateway.

Subcommands:
  config  Generate an IDE-compatible mcpServers configuration block
  status  Check the health of the MCP gateway
  test    Run test tools/list and tools/call requests against the operator
  serve   Start the MCP stdio gateway and proxy requests to the operator via mTLS
EOF
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            config)
                _banner "mcp $SUB"
                echo "{
  \"mcpServers\": {
    \"g8e\": {
      \"command\": \"$G8E_PROJECT_ROOT/g8e\",
      \"args\": [\"mcp\", \"serve\"],
      \"env\": {
        \"G8E_OPERATOR_URL\": \"https://localhost:$G8E_PORT_OPERATOR_PUBLIC_HTTPS\"
      }
    }
  }
}"
                ;;
            status)
                _banner "mcp $SUB"
                _ensure_operator
                _operator_curl GET /health
                echo ""
                ;;
            test)
                _banner "mcp $SUB tools/list"
                _ensure_operator
                _operator_curl POST /api/mcp/v1/tools/list "{}"
                echo ""
                _banner "mcp $SUB tools/call"
                _operator_curl POST /api/mcp/v1/tools/call "{}"
                echo ""
                ;;
            serve)
                _ensure_operator
                exec "$(_operator_bin)" \
                    --mcp-serve \
                    --endpoint "${OPERATOR_HTTPS_URL#https://}" \
                    --pki-dir "$G8E_PKI_DIR_HOST" \
                    --log "error"
                ;;
            *)
                echo "[g8e] unknown mcp subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    approve)
        _banner "approve ${SUB}"
        _ensure_operator
        _require_authenticated
        exec bash "$SCRIPT_DIR/scripts/tools/approve-transaction.sh" "$SUB" "${@:3}" ;;
esac
