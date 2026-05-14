#!/usr/bin/env bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

# --- Eval Fleet Helpers ---
_eval_fleet_container_ids() {
    COMPOSE_PROJECT_NAME=evals docker compose -f "$EVALS_COMPOSE" ps -q
}

_eval_fleet_running() {
    local container_id
    while IFS= read -r container_id; do
        [[ -z "$container_id" ]] && continue
        [[ "$(docker inspect -f '{{.State.Running}}' "$container_id" 2>/dev/null || true)" == "true" ]] && return 0
    done < <(_eval_fleet_container_ids)
    return 1
}

_eval_fleet_device_token() {
    local container_id token
    while IFS= read -r container_id; do
        [[ -z "$container_id" ]] && continue
        [[ "$(docker inspect -f '{{.State.Running}}' "$container_id" 2>/dev/null || true)" == "true" ]] || continue
        token="$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$container_id" 2>/dev/null | awk -F= '$1 == "DEVICE_TOKEN" {print substr($0, index($0, "=") + 1); exit}')"
        [[ -n "$token" ]] && printf '%s\n' "$token" && return 0
    done < <(_eval_fleet_container_ids)
    return 1
}

_eval_gold_set_path_for_host() {
    local gold_set="$1"
    if [[ "$gold_set" == /* ]]; then
        printf '%s\n' "$gold_set"
    elif [[ "$gold_set" == components/g8ee/* ]]; then
        printf '%s/%s\n' "$SCRIPT_DIR" "$gold_set"
    elif [[ -f "$SCRIPT_DIR/components/g8ee/$gold_set" ]]; then
        printf '%s/components/g8ee/%s\n' "$SCRIPT_DIR" "$gold_set"
    else
        printf '%s\n' "$gold_set"
    fi
}

TOP="$1"
SUB="${2:-}"

case "$TOP" in
    demo)
        DEMO_DIR="$SCRIPT_DIR/demo"
        PROFILES_DIR="$DEMO_DIR/profiles"
        if [[ ! -d "$DEMO_DIR" ]]; then
            echo "[g8e] demo directory not found: $DEMO_DIR" >&2
            exit 1
        fi
        if [[ "$SUB" == "profile" ]]; then
            mkdir -p "$PROFILES_DIR"
            _PROF_CMD="${3:-}"
            _PROF_NAME="${4:-}"
            _ACTIVE_FILE="$PROFILES_DIR/.active"
            case "$_PROF_CMD" in
                list)   exec make -C "$DEMO_DIR" list ;;
                switch) [[ -z "$_PROF_NAME" ]] && { echo "[g8e] usage: ./g8e demo profile switch <name>"; exit 1; }; exec make -C "$DEMO_DIR" switch P="$_PROF_NAME" ;;
                *)      echo "[g8e] unknown demo profile subcommand: '$_PROF_CMD'" >&2; echo "  Valid: list, switch" >&2; exit 1 ;;
            esac
        fi
        demo_args=("${@:3}")
        device_token=""
        nodes=""
        i=0
        while [[ $i -lt ${#demo_args[@]} ]]; do
            arg="${demo_args[$i]}"
            if [[ "$arg" == "-d" ]]; then
                if [[ $((i + 1)) -lt ${#demo_args[@]} ]]; then
                    device_token="${demo_args[$((i + 1))]}"
                    unset "demo_args[$i]" "demo_args[$((i + 1))]"
                    demo_args=("${demo_args[@]}")
                    i=0; continue
                else
                    echo "[g8e] -d flag requires a token value" >&2; exit 1
                fi
            fi
            i=$((i + 1))
        done
        i=0
        while [[ $i -lt ${#demo_args[@]} ]]; do
            arg="${demo_args[$i]}"
            if [[ "$arg" == "-n" ]]; then
                if [[ $((i + 1)) -lt ${#demo_args[@]} ]]; then
                    nodes="${demo_args[$((i + 1))]}"
                    unset "demo_args[$i]" "demo_args[$((i + 1))]"
                    demo_args=("${demo_args[@]}")
                    i=0; continue
                else
                    echo "[g8e] -n flag requires a node count value" >&2; exit 1
                fi
            fi
            i=$((i + 1))
        done
        if [[ -z "$device_token" && -n "$DEVICE_TOKEN" ]]; then
            device_token="$DEVICE_TOKEN"
        fi
        if [[ -n "$device_token" ]]; then
            demo_args=("DEVICE_TOKEN=$device_token" "${demo_args[@]}")
        fi
        if [[ -n "$nodes" ]]; then
            demo_args=("NODES=$nodes" "N=$nodes" "${demo_args[@]}")
        fi
        case "$SUB" in
            -h|--help|"")
                help_file="$SCRIPT_DIR/docs/g8e-help.md"
                if [[ -f "$help_file" ]]; then
                    awk '/^### demo/,/^### evals/' "$help_file" | head -n -1
                else
                    echo "[g8e] Help file not found: $help_file" >&2; exit 1
                fi
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            up|down|status|clean|health|nginx-check|operators|logs|discover-hosts|stream|vanish|dashboard|devices|broken)
                _banner "demo $SUB"; exec make -C "$DEMO_DIR" "$SUB" "${demo_args[@]}" ;;
            deploy)
                _banner "demo deploy"
                if ! make -C "$DEMO_DIR" status | grep -q "devices running: [1-9]"; then
                    echo "[g8e] Demo fleet not running. Starting it now..."
                    make -C "$DEMO_DIR" up "${demo_args[@]}"
                else
                    exec make -C "$DEMO_DIR" deploy "${demo_args[@]}"
                fi
                exit 0 ;;
            shell)
                _banner "demo shell"; exec make -C "$DEMO_DIR" shell "${demo_args[@]}" ;;
            *)
                echo "[g8e] unknown demo subcommand: '$SUB'" >&2
                echo "  Valid: up, down, status, clean, health, nginx-check, operators, logs, shell, deploy, discover-hosts, stream, vanish, dashboard, devices, broken, profile" >&2
                exit 1 ;;
        esac
        ;;
    evals)
        EVALS_DIR="$SCRIPT_DIR/components/g8ee/evals"
        EVALS_COMPOSE="$EVALS_DIR/docker-compose.evals.yml"
        if [[ ! -f "$EVALS_COMPOSE" ]]; then
            echo "[g8e] evals compose file not found: $EVALS_COMPOSE" >&2; exit 1
        fi
        eval_llm_args=()
        filtered_eval_args=()
        i=1
        _eval_args=("${@:2}")
        while [[ $i -le ${#_eval_args[@]} ]]; do
            arg="${_eval_args[$((i-1))]}"
            case "$arg" in
                -p|--llm-provider)      eval_llm_args+=("-p" "${_eval_args[$i]}"); i=$((i+2)) ;;
                -m|--primary-model)     eval_llm_args+=("-m" "${_eval_args[$i]}"); i=$((i+2)) ;;
                -a|--assistant-model)   eval_llm_args+=("-a" "${_eval_args[$i]}"); i=$((i+2)) ;;
                -l|--lite-model)        eval_llm_args+=("-l" "${_eval_args[$i]}"); i=$((i+2)) ;;
                -e|--llm-endpoint-url)  eval_llm_args+=("-e" "${_eval_args[$i]}"); i=$((i+2)) ;;
                -k|--llm-api-key)       eval_llm_args+=("-k" "${_eval_args[$i]}"); i=$((i+2)) ;;
                *)                      filtered_eval_args+=("$arg"); i=$((i+1)) ;;
            esac
        done
        SUB="${filtered_eval_args[0]:-}"
        REMAINING_ARGS=("${filtered_eval_args[@]:1}")
        case "$SUB" in
            -h|--help|"")
                help_file="$SCRIPT_DIR/docs/g8e-help.md"
                if [[ -f "$help_file" ]]; then
                    awk '/^### evals/,/^## DETAILED HELP/' "$help_file" | head -n -1
                else
                    echo "[g8e] Help file not found: $help_file" >&2; exit 1
                fi
                [[ -z "$SUB" ]] && exit 1 || exit 0 ;;
            run)
                _args=("${REMAINING_ARGS[@]}")
                [[ ${#_args[@]} -eq 0 ]] && { echo "Usage: ./g8e evals run --gold-set <path>" >&2; exit 1; }
                run_args=("${eval_llm_args[@]}")
                gold_set=""; operator_session_id=""
                set -- "${_args[@]}"
                while [[ "$#" -gt 0 ]]; do
                    case "$1" in
                        -d|--device-token)      echo "[g8e] evals run no longer accepts device tokens" >&2; exit 1 ;;
                        --operator-session-id)  operator_session_id="$2"; run_args+=("--operator-session-id" "$operator_session_id"); shift 2 ;;
                        --gold-set)             gold_set="$(_eval_gold_set_path_for_host "$2")"; run_args+=("--gold-set" "$gold_set"); shift 2 ;;
                        --dry-run)              run_args+=("--dry-run"); shift 1 ;;
                        *)                      run_args+=("$1"); shift 1 ;;
                    esac
                done
                if ! _eval_fleet_running; then
                    echo "[g8e] eval fleet is not running" >&2; exit 1
                fi
                if [[ -z "$operator_session_id" ]]; then
                    echo "[g8e] Error: --operator-session-id is required (auto-discovery removed with g8ed)" >&2
                    exit 1
                fi
                if [[ "$operator_session_id" == dlk_* ]]; then
                    echo "[g8e] Error: evals run requires a bound operator session id, not a device link token" >&2; exit 1
                fi
                if [[ ${#eval_llm_args[@]} -gt 0 ]]; then
                    echo "[g8e] Warning: LLM settings via evals run is temporarily disabled (g8ed removed)." >&2
                    echo "[g8e] Set LLM settings via the Operator API or g8ee adapter directly." >&2
                fi
                _banner "evals run"
                _venv="$SCRIPT_DIR/components/g8ee/.venv"
                [ ! -x "$_venv/bin/python" ] && { echo "[g8e] g8ee virtualenv missing" >&2; exit 1; }
                (
                    cd "$SCRIPT_DIR/components/g8ee"; export PYTHONPATH="$SCRIPT_DIR/components/g8ee:$SCRIPT_DIR/shared${PYTHONPATH:+:$PYTHONPATH}"
                    export G8E_SHARED_DIR="$SCRIPT_DIR/shared"; export G8E_PKI_DIR="$G8E_PKI_DIR_HOST"; export G8E_SECRETS_DIR="$G8E_SECRETS_DIR_HOST"
                    export G8E_TRUST_BUNDLE="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR_HOST/trust/hub-bundle.pem}"
                    export G8E_INTERNAL_HTTP_URL="$OPERATOR_HTTP_URL"; export G8EE_URL="$G8EE_URL"
                    "$_venv/bin/python" -m app.evals.runner.cli run "${run_args[@]}"
                )
                ;;
            deploy)
                _banner "evals deploy"; _nodes=3; _device_token=""
                while [[ $# -gt 2 ]]; do
                    case "${3:-}" in
                        --nodes) _nodes="${4:-3}"; shift 2 ;;
                        -d|--device-token) _device_token="${4:-}"; shift 2 ;;
                        *) echo "[g8e] unknown option: '${3}'"; exit 1 ;;
                    esac
                done
                [[ -z "$_device_token" ]] && { echo "[g8e] evals deploy requires a device link token" >&2; exit 1; }
                _trust_bundle="${G8E_TRUST_BUNDLE:-$G8E_PKI_DIR_HOST/trust/hub-bundle.pem}"
                [[ ! -f "$_trust_bundle" ]] && { echo "[g8e] trust bundle missing at $_trust_bundle" >&2; exit 1; }
                _ca_cert="$(cat "$_trust_bundle" 2>/dev/null || true)"
                DEVICE_TOKEN="$_device_token" G8E_CA_CERT="$_ca_cert" COMPOSE_PROJECT_NAME=evals docker compose -f "$EVALS_COMPOSE" up -d --build --scale eval-node="$_nodes"
                exit 0 ;;
            down)   _banner "evals down"; COMPOSE_PROJECT_NAME=evals docker compose -f "$EVALS_COMPOSE" down; exit 0 ;;
            status) _banner "evals status"; COMPOSE_PROJECT_NAME=evals docker compose -f "$EVALS_COMPOSE" ps; exit 0 ;;
            logs)   [[ -z "${3:-}" ]] && { echo "[g8e] evals logs requires a node name" >&2; exit 1; }; docker logs -f "${3}"; exit 0 ;;
            list)
                _banner "evals list"; _venv="$SCRIPT_DIR/components/g8ee/.venv"
                [ ! -x "$_venv/bin/python" ] && { echo "[g8e] g8ee virtualenv missing" >&2; exit 1; }
                (
                    cd "$SCRIPT_DIR/components/g8ee"; export PYTHONPATH="$SCRIPT_DIR/components/g8ee:$SCRIPT_DIR/shared${PYTHONPATH:+:$PYTHONPATH}"
                    export G8E_SHARED_DIR="$SCRIPT_DIR/shared"; "$_venv/bin/python" -m app.evals.runner.cli list
                )
                ;;
            *) echo "[g8e] unknown evals subcommand: '$SUB'" >&2; exit 1 ;;
        esac
        ;;
esac
