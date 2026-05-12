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
        export OPERATOR_SESSION_ID="default-session-id"
        export USER_ID="default-user-id"
        export OPERATOR_ID="default-operator-id"
        _save_credentials "$OPERATOR_SESSION_ID" "$USER_ID" "$OPERATOR_ID"
        echo "[g8e] Session saved to $G8E_CREDENTIALS_FILE"
        exit 0 ;;

    logout)
        _banner "logout"
        if _credentials_exist; then
            _clear_credentials
            echo "[g8e] Session cleared from $G8E_CREDENTIALS_FILE"
        else
            echo "[g8e] No active session found"
        fi
        exit 0 ;;

    ssh)
        case "$SUB" in
            -h|--help|"")
                help_file="$SCRIPT_DIR/docs/g8e-help.md"
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
                help_file="$SCRIPT_DIR/docs/g8e-help.md"
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
                help_file="$SCRIPT_DIR/docs/g8e-help.md"
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
                help_file="$SCRIPT_DIR/docs/g8e-help.md"
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
                help_file="$SCRIPT_DIR/docs/g8e-help.md"
                if [[ -f "$help_file" ]]; then
                    awk '/^### security/,/^### data/' "$help_file" | head -n -1
                else
                    echo "[g8e] Help file not found: $help_file" >&2; exit 1
                fi
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            validate)
                _banner "security validate"
                _run_host_script bash "$SCRIPT_DIR/scripts/security/validate-platform-security.sh" "${@:3}" ;;
            certs)
                echo "[g8e] ERROR: the legacy certificate command has been removed. Certificate management is owned by the Operator PKI subsystem." >&2
                echo "[g8e] Use './g8e pki' commands for PKI operations." >&2
                exit 1 ;;
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
                help_file="$SCRIPT_DIR/docs/g8e-help.md"
                if [[ -f "$help_file" ]]; then
                    awk '/^### data/,/^### demo/' "$help_file" | head -n -1
                else
                    echo "[g8e] Help file not found: $help_file" >&2; exit 1
                fi
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            users|operators|store|settings|audit|device-links)
                _banner "data $SUB ${@:3}"; _ensure_operator; _moved_to_operator_protocol "data $SUB" ;;
            *)
                echo "[g8e] unknown data subcommand: '$SUB'" >&2; exit 1 ;;
        esac ;;

    mcp)
        case "$SUB" in
            -h|--help|"")
                help_file="$SCRIPT_DIR/docs/g8e-help.md"
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
