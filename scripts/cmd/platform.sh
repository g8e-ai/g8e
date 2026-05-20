#!/usr/bin/env bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

SUB="${1:-}"
DEV_MODE="${DEV_MODE:-false}"

case "$SUB" in
    -h|--help|"")
        help_file="$SCRIPT_DIR/docs/cli_help.md"
        if [[ -f "$help_file" ]]; then
            awk '/^### platform/,/^### operator/' "$help_file" | head -n -1
        else
            echo "[g8e] Help file not found: $help_file" >&2
            exit 1
        fi
        [[ -z "$SUB" ]] && exit 1 || exit 0
        ;;
    status|start|stop|restart|reset|clean|settings|logs)
        case "$SUB" in
            start)    _banner "platform start";    exec bash "$SCRIPT_DIR/scripts/core/build.sh" $([[ "$DEV_MODE" == true ]] && echo "--dev") up      "${@:2}" ;;
            stop)     _banner "platform stop";     exec bash "$SCRIPT_DIR/scripts/core/build.sh" down    "${@:2}" ;;
            restart)  _banner "platform restart";  exec bash "$SCRIPT_DIR/scripts/core/build.sh" $([[ "$DEV_MODE" == true ]] && echo "--dev") restart "${@:2}" ;;
            status)   _banner "platform status";   exec bash "$SCRIPT_DIR/scripts/core/build.sh" status  "${@:2}" ;;
            reset)
                _banner "platform reset"
                skip_confirm=false
                for arg in "${@:2}"; do
                    if [[ "$arg" == "-y" || "$arg" == "--yes" || "$arg" == "--force" ]]; then
                        skip_confirm=true
                    fi
                done
                if [[ "$skip_confirm" != "true" ]]; then
                    if [[ ! -t 0 ]]; then
                        echo "[g8e] Error: stdin is not a TTY. Interactive confirmation required. Use -y/--yes/--force to bypass." >&2
                        exit 1
                    fi
                    echo ""
                    echo -e "\033[1;31mWARNING: You are about to RESET the g8e platform!\033[0m"
                    echo "This command will:"
                    echo "  1. Stop all running g8e services (Operator and any optional apps)."
                    echo "  2. Wipe the SQLite databases (users, settings, audits, etc.) and bootstrap secrets."
                    echo "  3. Preserve your existing TLS/PKI certificates and keys."
                    echo "  4. Restart the services with a fresh database."
                    echo ""
                    read -p "Are you sure you want to continue? (y/n): " confirm
                    if [[ ! "$confirm" =~ ^[Yy]([Ee][Ss])?$ ]]; then
                        echo "Reset cancelled."
                        exit 0
                    fi
                fi
                exec bash "$SCRIPT_DIR/scripts/core/build.sh" reset   "${@:2}" ;;
            clean)
                _banner "platform clean"
                skip_confirm=false
                for arg in "${@:2}"; do
                    if [[ "$arg" == "-y" || "$arg" == "--yes" || "$arg" == "--force" ]]; then
                        skip_confirm=true
                    fi
                done
                if [[ "$skip_confirm" != "true" ]]; then
                    if [[ ! -t 0 ]]; then
                        echo "[g8e] Error: stdin is not a TTY. Interactive confirmation required. Use -y/--yes/--force to bypass." >&2
                        exit 1
                    fi
                    echo ""
                    echo -e "\033[1;31mWARNING: You are about to CLEAN the g8e platform!\033[0m"
                    echo "This command will:"
                    echo "  1. Stop all running g8e services (Operator and any optional apps)."
                    echo "  2. Completely delete the entire runtime directory ($G8E_RUNTIME_DIR)."
                    echo "  3. Delete all SQLite databases, bootstrap secrets, logs, AND TLS/PKI certificates/keys."
                    echo "  4. Clean Python caches (__pycache__, .pyc, .pyo)."
                    echo "  5. All trust routes and credentials will be permanently destroyed."
                    echo ""
                    read -p "Are you sure you want to continue? (y/n): " confirm
                    if [[ ! "$confirm" =~ ^[Yy]([Ee][Ss])?$ ]]; then
                        echo "Clean cancelled."
                        exit 0
                    fi
                fi
                exec bash "$SCRIPT_DIR/scripts/core/build.sh" clean   "${@:2}" ;;
            settings)
                _banner "platform settings ${@:2}"
                _ensure_operator
                _operator_curl GET "/api/settings" ;;
            logs)
                _banner "platform logs"
                tail -f "$G8E_RUNTIME_DIR/logs/"*.log ;;
        esac ;;
    *)
        echo "[g8e] unknown platform subcommand: '$SUB'" >&2
        echo "  Valid: settings, status, start, stop, restart, reset, clean, logs" >&2
        exit 1 ;;
esac
