#!/usr/bin/env bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

SUB="${1:-}"
_APP_ACTION="$SUB"
_APP_TARGET="${2:-all}"

case "$_APP_ACTION" in
    -h|--help|"")
        echo "Usage: ./g8e apps {start|stop|restart|status|build} [g8ed|g8ee|all]"
        echo ""
        echo "Optional bundled apps are application-layer adapters and are not part of the default substrate lifecycle."
        [[ -z "$_APP_ACTION" ]] && exit 1 || exit 0
        ;;
esac

case "$_APP_TARGET" in
    all) _APP_FLAGS=(--with-apps) ;;
    g8ed) _APP_FLAGS=(--with-g8ed) ;;
    g8ee) _APP_FLAGS=(--with-g8ee) ;;
    *) echo "[g8e] unknown app target: '$_APP_TARGET'" >&2; echo "  Valid: g8ed, g8ee, all" >&2; exit 1 ;;
esac

case "$_APP_ACTION" in
    start)
        _banner "apps start $_APP_TARGET"
        exec bash "$SCRIPT_DIR/scripts/core/build.sh" up "${_APP_FLAGS[@]}" ;;
    stop)
        _banner "apps stop $_APP_TARGET"
        case "$_APP_TARGET" in
            all) exec bash "$SCRIPT_DIR/scripts/core/build.sh" down ;;
            g8ed)
                if [[ -f "$_G8ED_PID_FILE" ]]; then
                    kill "$(cat "$_G8ED_PID_FILE")" 2>/dev/null || true
                    rm -f "$_G8ED_PID_FILE"
                fi
                exit 0 ;;
            g8ee)
                if [[ -f "$_G8EE_PID_FILE" ]]; then
                    kill "$(cat "$_G8EE_PID_FILE")" 2>/dev/null || true
                    rm -f "$_G8EE_PID_FILE"
                fi
                exit 0 ;;
        esac ;;
    restart)
        _banner "apps restart $_APP_TARGET"
        exec bash "$SCRIPT_DIR/scripts/core/build.sh" rebuild "${_APP_FLAGS[@]}" ;;
    status)
        _banner "apps status"
        exec bash "$SCRIPT_DIR/scripts/core/build.sh" status ;;
    build)
        case "$_APP_TARGET" in
            all)
                _banner "apps build all"
                npm ci --prefix "$SCRIPT_DIR/components/g8ed"
                python3 -m venv "$SCRIPT_DIR/components/g8ee/.venv"
                "$SCRIPT_DIR/components/g8ee/.venv/bin/pip" install --upgrade pip
                "$SCRIPT_DIR/components/g8ee/.venv/bin/pip" install -r "$SCRIPT_DIR/components/g8ee/requirements.txt"
                ;;
            g8ed)
                _banner "apps build g8ed"
                exec npm ci --prefix "$SCRIPT_DIR/components/g8ed" ;;
            g8ee)
                _banner "apps build g8ee"
                python3 -m venv "$SCRIPT_DIR/components/g8ee/.venv"
                "$SCRIPT_DIR/components/g8ee/.venv/bin/pip" install --upgrade pip
                exec "$SCRIPT_DIR/components/g8ee/.venv/bin/pip" install -r "$SCRIPT_DIR/components/g8ee/requirements.txt" ;;
        esac ;;
    *)
        echo "[g8e] unknown apps subcommand: '$_APP_ACTION'" >&2
        echo "  Valid: start, stop, restart, status, build" >&2
        exit 1 ;;
esac
