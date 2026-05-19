#!/usr/bin/env bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

EVALS_PROJECT_DIR="$SCRIPT_DIR/evals"
# The new harness reuses the g8ee venv so it can import the canonical
# g8e LLM provider stack (app.llm.*) without duplicating provider deps.
EVALS_VENV="$SCRIPT_DIR/services/g8ee/.venv"

_ensure_evals_venv() {
    if [[ ! -x "$EVALS_VENV/bin/python" ]]; then
        echo "[evals] g8ee virtualenv missing at $EVALS_VENV — run './g8e platform start' or 'make -C services/g8ee venv' first" >&2
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
        help_file="$SCRIPT_DIR/docs/cli_help.md"
        if [[ -f "$help_file" ]]; then
            awk '/^### evals/,/^## Detailed Help/' "$help_file" | head -n -1
        else
            echo "[g8e] Help file not found: $help_file" >&2; exit 1
        fi
        [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
    bench)
        _ensure_evals_venv
        _banner "evals bench"
        # The bench drives the *full* g8ee chat pipeline (Triage → Dash/Sage →
        # Tribunal → Warden) via /api/internal/chat, so the platform must be
        # running and the caller must be authenticated.
        if ! _operator_running; then
            echo "[g8e] Operator listen mode is not running — start it: ./g8e platform start" >&2
            exit 1
        fi
        if ! _g8ee_running; then
            echo "[g8e] g8ee Engine is not running — start it: ./g8e apps start g8ee" >&2
            exit 1
        fi
        if ! _load_credentials; then
            _operator_bootstrap
            _load_credentials || {
                echo "[g8e] no cached credentials — run: ./g8e login" >&2
                exit 1
            }
        fi
        export "$G8E_ENV_OPERATOR_SESSION_ID"="$G8E_OPERATOR_SESSION_ID"
        export "$G8E_ENV_USER_ID"="$G8E_USER_ID"
        export "$G8E_ENV_OPERATOR_ID"="$G8E_OPERATOR_ID"
        export G8E_CLI_CERT="${G8E_CLI_CERT:-$G8E_CLI_CERT_FILE}"
        export G8E_CLI_KEY="${G8E_CLI_KEY:-$G8E_CLI_KEY_FILE}"
        export "$G8E_ENV_G8EE_URL"="${G8E_G8EE_URL:-https://localhost:$G8E_PORT_G8EE_HTTP}"
        export "$G8E_ENV_INTERNAL_HTTP_URL"="${G8E_INTERNAL_HTTP_URL:-$OPERATOR_HTTP_URL}"
        export G8E_TRUST_BUNDLE="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR_HOST/trust/hub-bundle.pem}"
        cd "$EVALS_PROJECT_DIR"
        export "$G8E_ENV_PKIDir"="${G8E_PKI_DIR:-$G8E_PKI_DIR_HOST}"
        export "$G8E_ENV_PROTOCOL_DIR"="$SCRIPT_DIR/protocol"
        export PYTHONPATH="$EVALS_PYTHONPATH"
        exec "$EVALS_VENV/bin/python" -m g8e_evals.cli run "${REMAINING_ARGS[@]}"
        ;;
    verify-receipts)
        _ensure_evals_venv
        _banner "evals verify-receipts"
        cd "$EVALS_PROJECT_DIR"
        export "$G8E_ENV_PKIDir"="${G8E_PKI_DIR:-$G8E_PKI_DIR_HOST}"
        export "$G8E_ENV_PROTOCOL_DIR"="$SCRIPT_DIR/protocol"
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
    *) echo "[g8e] unknown evals subcommand: '$SUB'" >&2
       echo "  Valid: bench, verify-receipts, list" >&2
       exit 1 ;;
esac
