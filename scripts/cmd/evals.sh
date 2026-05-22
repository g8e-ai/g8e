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

EVALS_PROJECT_DIR="$SCRIPT_DIR/evals"
# The new harness reuses the g8ee venv so it can import the canonical
# g8e LLM provider stack (app.llm.*) without duplicating provider deps.
EVALS_VENV="$SCRIPT_DIR/services/g8ee/.venv"

_ensure_evals_venv() {
    if [[ ! -x "$EVALS_VENV/bin/python" ]]; then
        echo "[evals] g8ee virtualenv missing at $EVALS_VENV - run './g8e platform start' or 'make -C services/g8ee venv' first" >&2
        exit 1
    fi
    if ! "$EVALS_VENV/bin/python" -c "import g8e_evals" >/dev/null 2>&1; then
        echo "[evals] Installing g8e-evals into g8ee venv..." >&2
        "$EVALS_VENV/bin/pip" install --quiet -e "$EVALS_PROJECT_DIR" >&2
    fi
}

EVALS_PYTHONPATH="$SCRIPT_DIR/protocol/python${PYTHONPATH:+:$PYTHONPATH}"
SUB="${1:-}"
REMAINING_ARGS=("${@:2}")

case "$SUB" in
    -h|--help|"")
        cat << 'EOF'
Usage: ./g8e evals <command> [options]

Commands:
  bench --suite <suite> --mode <baseline|receipt>
      Run a benchmark suite against the new harness.
      Receipt mode requires a running Operator and an authenticated CLI session. 
      Baseline mode runs the SUT without binding.

  verify-receipts <report-dir>
      Re-verify receipt signatures offline.

  list
      List benchmark suites and bundled gold sets.

  deploy [-d <token>] [-n <nodes>]
      Deploy operators to the demo fleet for evaluation.
      If no token is provided, a temporary device link is automatically created.

Workflow (new harness):
  1. ./g8e login (zero-arg in sandbox; mints CLI mTLS cert + session)
  2. ./g8e data device-links create --count 10
  3. ./g8e evals bench --suite ifeval --mode baseline
  3. ./g8e evals bench --suite ifeval --mode receipt
  4. ./g8e evals verify-receipts reports/ifeval-<ts>

Note: 
  G8E_OPERATOR_SESSION_ID is loaded from ~/.g8e/credentials automatically after 
  ./g8e login. Do not pass --operator-session-id unless you are explicitly 
  overriding the cached session.
EOF
        [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
    bench)
        _ensure_evals_venv
        _banner "evals bench"

        # Resolve relative gold-set path before we cd into the evals dir
        _args=("${REMAINING_ARGS[@]}")
        for i in "${!_args[@]}"; do
            if [[ "${_args[$i]}" == "--gold-set" && $((i + 1)) -lt ${#_args[@]} ]]; then
                val="${_args[$((i+1))]}"
                if [[ ! "$val" == /* ]]; then
                    _args[$((i+1))]="$(pwd)/$val"
                fi
            fi
        done
        REMAINING_ARGS=("${_args[@]}")

        # The bench drives the *full* g8ee chat pipeline (Triage → Dash/Sage →
        # Tribunal → Warden) via /api/internal/chat, so the platform must be
        # running and the caller must be authenticated.
        if ! _operator_running; then
            echo "[g8e] Operator gateway mode is not running - start it: ./g8e platform start" >&2
            exit 1
        fi
        if ! _g8ee_running; then
            echo "[g8e] g8ee Ensemble is not running - start it: ./g8e apps start g8ee" >&2
            exit 1
        fi
        if ! _load_credentials; then
            _operator_bootstrap
            _load_credentials || {
                echo "[g8e] no cached credentials - run: ./g8e login" >&2
                exit 1
            }
        fi
        export G8E_OPERATOR_SESSION_ID="$G8E_OPERATOR_SESSION_ID"
        export G8E_CLI_SESSION_ID="$G8E_CLI_SESSION_ID"
        export G8E_USER_ID="$G8E_USER_ID"
        export G8E_OPERATOR_ID="$G8E_OPERATOR_ID"
        # Use operator certificate for Operator endpoints (SSE stream requires operator session auth)
        export G8E_CLI_CERT="${G8E_OPERATOR_CERT:-$G8E_OPERATOR_CERT_FILE}"
        export G8E_CLI_KEY="${G8E_OPERATOR_KEY:-$G8E_OPERATOR_KEY_FILE}"
        export G8E_G8EE_URL="${G8E_G8EE_URL:-https://localhost:$G8E_G8EE_HTTPS_PORT}"
        export G8E_INTERNAL_HTTP_URL="${G8E_INTERNAL_HTTP_URL:-$OPERATOR_HTTPS_URL}"
        export G8E_TRUST_BUNDLE="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR_HOST/trust/hub-bundle.pem}"
        cd "$EVALS_PROJECT_DIR"
        export G8E_PKI_DIR="${G8E_PKI_DIR:-$G8E_PKI_DIR_HOST}"
        export G8E_PROTOCOL_DIR="$SCRIPT_DIR/protocol"
        export PYTHONPATH="$EVALS_PYTHONPATH"
        exec "$EVALS_VENV/bin/python" -m g8e_evals.cli run "${REMAINING_ARGS[@]}"
        ;;
    verify-receipts)
        _ensure_evals_venv
        _banner "evals verify-receipts"
        cd "$EVALS_PROJECT_DIR"
        export G8E_PKI_DIR="${G8E_PKI_DIR:-$G8E_PKI_DIR_HOST}"
        export G8E_PROTOCOL_DIR="$SCRIPT_DIR/protocol"
        export PYTHONPATH="$EVALS_PYTHONPATH"
        exec "$EVALS_VENV/bin/python" -m g8e_evals.cli verify-receipts "${REMAINING_ARGS[@]}"
        ;;
    list)
        _banner "evals list"
        echo "Benchmark suites:"
        echo "  ifeval - Instruction Following Evaluation"
        echo ""
        echo "Gold sets under $EVALS_PROJECT_DIR/gold_sets:"
        if [[ -d "$EVALS_PROJECT_DIR/gold_sets" ]]; then
            ( cd "$EVALS_PROJECT_DIR" && find gold_sets -mindepth 2 -type f \( -name '*.jsonl' -o -name '*.json' \) | sort | sed 's/^/  /' )
        fi
        ;;
    deploy)
        _banner "evals deploy"
        _device_token=""
        _nodes=""
        i=0
        while [[ $i -lt ${#REMAINING_ARGS[@]} ]]; do
            arg="${REMAINING_ARGS[$i]}"
            if [[ "$arg" == "-d" ]]; then
                if [[ $((i + 1)) -lt ${#REMAINING_ARGS[@]} ]]; then
                    _device_token="${REMAINING_ARGS[$((i + 1))]}"
                    unset "REMAINING_ARGS[$i]" "REMAINING_ARGS[$((i + 1))]"
                    REMAINING_ARGS=("${REMAINING_ARGS[@]}")
                    i=0; continue
                else
                    echo "[evals] -d flag requires a token value" >&2; exit 1
                fi
            elif [[ "$arg" == "-n" ]]; then
                if [[ $((i + 1)) -lt ${#REMAINING_ARGS[@]} ]]; then
                    _nodes="${REMAINING_ARGS[$((i + 1))]}"
                    unset "REMAINING_ARGS[$i]" "REMAINING_ARGS[$((i + 1))]"
                    REMAINING_ARGS=("${REMAINING_ARGS[@]}")
                    i=0; continue
                else
                    echo "[evals] -n flag requires a node count value" >&2; exit 1
                fi
            fi
            i=$((i + 1))
        done

        if [[ -z "$_device_token" ]]; then
            if ! _load_credentials; then
                echo "[evals] No cached credentials found. Run './g8e login' first, or provide -d <token> explicitly." >&2
                exit 1
            fi
            if ! _operator_running; then
                echo "[evals] Operator is not running - start it: ./g8e platform start" >&2
                exit 1
            fi
            _dl_output=$(python3 "$SCRIPT_DIR/scripts/data/manage-device-links.py" create --user-id "$G8E_USER_ID" --name "evals-auto-deploy" --max-uses 100 --expires-in-hours 1 2>&1)
            if [[ $? -ne 0 ]]; then
                echo "[evals] Failed to automatically create device link." >&2
                echo "$_dl_output" >&2
                exit 1
            fi
            _device_token=$(echo "$_dl_output" | grep "Token:" | awk '{print $2}')
            if [[ -z "$_device_token" ]]; then
                echo "[evals] Could not extract token from device-link output." >&2
                echo "$_dl_output" >&2
                exit 1
            fi
        fi

        _deploy_args=("DEVICE_TOKEN=$_device_token")
        if [[ -n "$_nodes" ]]; then
            _deploy_args+=("NODES=$_nodes" "N=$_nodes")
        fi

        if ! make -C "$SCRIPT_DIR/demo" status | grep -q "devices running: [1-9]"; then
            make -C "$SCRIPT_DIR/demo" up "${_deploy_args[@]}"
        fi

        exec make -C "$SCRIPT_DIR/demo" deploy "${_deploy_args[@]}"
        ;;
    *) echo "[g8e] unknown evals subcommand: '$SUB'" >&2
       echo "  Valid: bench, verify-receipts, list, deploy" >&2
       exit 1 ;;
esac
