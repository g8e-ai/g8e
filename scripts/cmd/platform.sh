#!/usr/bin/env bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

SUB="${1:-}"
DEV_MODE="${DEV_MODE:-false}"

case "$SUB" in
    -h|--help|"")
        help_file="$SCRIPT_DIR/docs/general/cli_help.md"
        if [[ -f "$help_file" ]]; then
            awk '/^### platform/,/^### operator/' "$help_file" | head -n -1
        else
            echo "[g8e] Help file not found: $help_file" >&2
            exit 1
        fi
        [[ -z "$SUB" ]] && exit 1 || exit 0
        ;;
    status|start|stop|restart|reset|wipe|clean|settings|logs)
        case "$SUB" in
            start)    _banner "platform start";    exec bash "$SCRIPT_DIR/scripts/core/build.sh" $([[ "$DEV_MODE" == true ]] && echo "--dev") up      "${@:2}" ;;
            stop)     _banner "platform stop";     exec bash "$SCRIPT_DIR/scripts/core/build.sh" down    "${@:2}" ;;
            restart)  _banner "platform restart";  exec bash "$SCRIPT_DIR/scripts/core/build.sh" $([[ "$DEV_MODE" == true ]] && echo "--dev") restart "${@:2}" ;;
            status)   _banner "platform status";   exec bash "$SCRIPT_DIR/scripts/core/build.sh" status  "${@:2}" ;;
            reset)    _banner "platform reset";    exec bash "$SCRIPT_DIR/scripts/core/build.sh" reset   "${@:2}" ;;
            wipe)     _banner "platform wipe";     exec bash "$SCRIPT_DIR/scripts/core/build.sh" wipe    "${@:2}" ;;
            clean)    _banner "platform clean";    exec bash "$SCRIPT_DIR/scripts/core/build.sh" clean   "${@:2}" ;;
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
        echo "  Valid: settings, status, start, stop, restart, reset, wipe, clean, logs" >&2
        exit 1 ;;
esac
