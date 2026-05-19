#!/usr/bin/env bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
source "$(dirname "${BASH_SOURCE[0]}")/app_helpers.sh"

TOP="$1"
SUB="${2:-}"

case "$TOP" in
    vars)
        _banner "vars ${SUB} ${@:3}"
        exec bash "$SCRIPT_DIR/scripts/core/manage-env.sh" "$SUB" "${@:3}" ;;

    login)
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
                --email)   ((i++)); _login_email="${_args[$i]}" ;;
                --email=*) _login_email="${_args[$i]#--email=}" ;;
                --count)   ((i++)); _dl_count="${_args[$i]}" ;;
                --count=*) _dl_count="${_args[$i]#--count=}" ;;
                --ttl)     ((i++)); _dl_ttl="${_args[$i]}" ;;
                --ttl=*)   _dl_ttl="${_args[$i]#--ttl=}" ;;
            esac
            ((i++))
        done

        if [[ -z "$_login_email" ]]; then
            echo "[g8e] Usage: ./g8e login --email <email> [--count <count>] [--ttl <ttl_seconds>]" >&2
            exit 1
        fi

        _trust_bundle="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR_HOST/trust/hub-bundle.pem}"
        _bootstrap_port="${OPERATOR_LISTEN_PUBLIC_PORT:-$G8E_PORT_OPERATOR_PUBLIC}"
        _bootstrap_url="${G8E_BOOTSTRAP_URL:-https://localhost:$_bootstrap_port}"

        if [[ ! -f "$_trust_bundle" ]]; then
            echo "[g8e] Trust bundle not found at $_trust_bundle - start the platform first: ./g8e platform start" >&2
            exit 1
        fi

        # 1. Request a device-link token via bootstrap port (unauthenticated)
        echo "[g8e] Requesting device-link token..."
        _fingerprint=$(echo "g8e-cli-$(hostname)-$(whoami)" | sha256sum | awk '{print $1}')
        _dl_body=$(python3 -c "import json,sys; print(json.dumps({'email':sys.argv[1],'name':'cli-'+__import__('socket').gethostname(),'max_uses':int(sys.argv[2]),'ttl_seconds':int(sys.argv[3])}))" "$_login_email" "$_dl_count" "$_dl_ttl")
        _dl_resp=$( curl -sS --cacert "$_trust_bundle" \
            -X POST -H "${G8E_HEADER_CONTENT_TYPE}: application/json" \
            -d "$_dl_body" \
            "$_bootstrap_url/api/auth/device-link/request" 2>&1 )
        _dl_token=$(echo "$_dl_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('token',''))" 2>/dev/null)
        _login_user_id=$(echo "$_dl_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('user_id',''))" 2>/dev/null)
        if [[ -z "$_dl_token" ]]; then
            echo "[g8e] Failed to create device-link: $_dl_resp" >&2
            exit 1
        fi
        echo "[g8e] Device-link token obtained: $_dl_token (count=$_dl_count, ttl=$_dl_ttl s)"

        # 3. Generate ECDSA private keys + CSRs
        echo "[g8e] Generating keys and CSRs..."
        _tmp_dir=$(mktemp -d)
        trap 'rm -rf "$_tmp_dir"' EXIT
        if ! _generate_workload_csrs "$_tmp_dir"; then
            echo "[g8e] CSR generation failed" >&2
            exit 1
        fi

        # 4. Register via bootstrap port (no mTLS required on this route)
        echo "[g8e] Registering with operator..."
        _reg_body=$(python3 -c "
import json, socket, os
print(json.dumps({
    'system_fingerprint': '${_fingerprint}',
    'hostname': socket.gethostname(),
    'os': 'linux',
    'arch': os.uname().machine,
    'username': os.environ.get('USER', os.environ.get('LOGNAME', 'unknown')),
    'csr_pem': $(python3 -c "import sys,json; print(json.dumps(sys.stdin.read()))" <<< "$_op_csr_pem"),
    'cli_csr_pem': $(python3 -c "import sys,json; print(json.dumps(sys.stdin.read()))" <<< "$_cli_csr_pem"),
}))")
        _reg_resp=$( curl -sS --cacert "$_trust_bundle" \
            -X POST -H "${G8E_HEADER_CONTENT_TYPE}: application/json" \
            -H "${G8E_HEADER_DEVICE_TOKEN}: $_dl_token" \
            -d "$_reg_body" \
            "$_bootstrap_url/api/auth/device-link/register" 2>&1 )

        _reg_error=$(echo "$_reg_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('error',''))" 2>/dev/null)
        if [[ -n "$_reg_error" ]]; then
            echo "[g8e] Registration failed: $_reg_error" >&2
            exit 1
        fi

        # 5. Extract and save results.
        # cli_session_id is the disjoint BYO/CLI routing namespace minted at
        # login alongside operator_session_id. The CLI must NEVER reuse the
        # operator_session_id as a cli session - those are first-class disjoint
        # session types.
        _session_id=$(echo "$_reg_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('operator_session_id',''))" 2>/dev/null)
        _cli_session_id=$(echo "$_reg_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('cli_session_id',''))" 2>/dev/null)
        _operator_id=$(echo "$_reg_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('operator_id',''))" 2>/dev/null)
        _op_cert_pem=$(echo "$_reg_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('operator_cert',''))" 2>/dev/null)
        _op_chain_pem=$(echo "$_reg_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('operator_cert_chain',''))" 2>/dev/null)
        _cli_cert_pem=$(echo "$_reg_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('cli_cert',''))" 2>/dev/null)
        _cli_chain_pem=$(echo "$_reg_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('cli_cert_chain',''))" 2>/dev/null)
        _hub_bundle=$(echo "$_reg_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('hub_trust_bundle',''))" 2>/dev/null)

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
        echo "[g8e] Authenticated - operator_id=$_operator_id operator_session=${_session_id:0:8}... cli_session=${_cli_session_id:0:8}..."
        echo "[g8e] Credentials saved to $G8E_CREDENTIALS_FILE"
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
                help_file="$SCRIPT_DIR/docs/cli_help.md"
                if [[ -f "$help_file" ]]; then
                    awk '/^### ssh/,/^### aws/' "$help_file" | head -n -1
                else
                    echo "[g8e] Help file not found: $help_file" >&2; exit 1
                fi
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            setup)
                _banner "ssh setup"; exec bash "$SCRIPT_DIR/scripts/tools/setup-ssh.sh" "${@:3}" ;;
            *)
                echo "[g8e] unknown ssh subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    aws)
        case "$SUB" in
            -h|--help|"")
                help_file="$SCRIPT_DIR/docs/cli_help.md"
                if [[ -f "$help_file" ]]; then
                    awk '/^### aws/,/^### demo/' "$help_file" | head -n -1
                else
                    echo "[g8e] Help file not found: $help_file" >&2; exit 1
                fi
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            setup)
                _banner "aws setup"; exec bash "$SCRIPT_DIR/scripts/tools/setup-aws.sh" "${@:3}" ;;
            *)
                echo "[g8e] unknown aws subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    search)
        case "$SUB" in
            -h|--help|"")
                help_file="$SCRIPT_DIR/docs/cli_help.md"
                if [[ -f "$help_file" ]]; then
                    awk '/^### search/,/^### ssh/' "$help_file" | head -n -1
                else
                    echo "[g8e] Help file not found: $help_file" >&2; exit 1
                fi
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            setup)   _banner "search setup"; exec bash "$SCRIPT_DIR/scripts/tools/setup-search.sh" "${@:3}" ;;
            disable) _banner "search disable"; exec bash "$SCRIPT_DIR/scripts/tools/setup-search.sh" --disable "${@:3}" ;;
            *)       echo "[g8e] unknown search subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    llm)
        case "$SUB" in
            -h|--help|"")
                help_file="$SCRIPT_DIR/docs/cli_help.md"
                if [[ -f "$help_file" ]]; then
                    awk '/^### llm/,/^### mcp/' "$help_file" | head -n -1
                else
                    echo "[g8e] Help file not found: $help_file" >&2; exit 1
                fi
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            setup|show|get|set|restart)
                exec bash "$SCRIPT_DIR/scripts/tools/setup-llm.sh" "$SUB" "${@:3}" ;;
            *)
                echo "[g8e] unknown llm subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    security)
        case "$SUB" in
            -h|--help|"")
                help_file="$SCRIPT_DIR/docs/cli_help.md"
                if [[ -f "$help_file" ]]; then
                    awk '/^### security/,/^### data/' "$help_file" | head -n -1
                else
                    echo "[g8e] Help file not found: $help_file" >&2; exit 1
                fi
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
                help_file="$SCRIPT_DIR/docs/cli_help.md"
                if [[ -f "$help_file" ]]; then
                    awk '/^### data/,/^### demo/' "$help_file" | head -n -1
                else
                    echo "[g8e] Help file not found: $help_file" >&2; exit 1
                fi
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
                _banner "data audit ${@:3}"; _ensure_operator
                _run_host_script python3 "$SCRIPT_DIR/scripts/data/manage-lfaa.py" "${@:3}" ;;
            *)
                echo "[g8e] unknown data subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    mcp)
        case "$SUB" in
            -h|--help|"")
                help_file="$SCRIPT_DIR/docs/cli_help.md"
                if [[ -f "$help_file" ]]; then
                    awk '/^### mcp/,/^### llm/' "$help_file" | head -n -1
                else
                    echo "[g8e] Help file not found: $help_file" >&2; exit 1
                fi
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            config|test|status)
                _banner "mcp $SUB"; _ensure_operator; _moved_to_operator_protocol "mcp $SUB" ;;
            *)
                echo "[g8e] unknown mcp subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;
esac
