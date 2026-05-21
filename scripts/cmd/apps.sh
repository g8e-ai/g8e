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

SUB="${1:-}"
_APP_ACTION="$SUB"
_APP_TARGET="${2:-g8ee}"

case "$_APP_ACTION" in
    -h|--help|"")
        echo "Usage: ./g8e apps {start|stop|restart|status|build} [g8ee]"
        echo ""
        echo "Optional reference ensembles are application-layer adapters and are not part of the default platform lifecycle."
        [[ -z "$_APP_ACTION" ]] && exit 1 || exit 0
        ;;
esac

case "$_APP_TARGET" in
    g8ee) _APP_FLAGS=(--with-g8ee) ;;
    *) echo "[g8e] unknown app target: '$_APP_TARGET'" >&2; echo "  Valid: g8ee" >&2; exit 1 ;;
esac

case "$_APP_ACTION" in
    start)
        _banner "apps start $_APP_TARGET"
        exec bash "$SCRIPT_DIR/scripts/core/build.sh" up "${_APP_FLAGS[@]}" ;;
    stop)
        _banner "apps stop $_APP_TARGET"
        case "$_APP_TARGET" in
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
            g8ee)
                _banner "apps build g8ee"
                python3 -m venv "$SCRIPT_DIR/services/g8ee/.venv"
                "$SCRIPT_DIR/services/g8ee/.venv/bin/pip" install --upgrade pip
                exec "$SCRIPT_DIR/services/g8ee/.venv/bin/pip" install -r "$SCRIPT_DIR/services/g8ee/requirements.txt" ;;
        esac ;;
    *)
        echo "[g8e] unknown apps subcommand: '$_APP_ACTION'" >&2
        echo "  Valid: start, stop, restart, status, build" >&2
        exit 1 ;;
esac
