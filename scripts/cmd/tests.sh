#!/usr/bin/env bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

_TEST_COMPONENT="${1:-}"
if [[ -z "$_TEST_COMPONENT" || "$_TEST_COMPONENT" == "-h" || "$_TEST_COMPONENT" == "--help" ]]; then
    if [[ -z "$_TEST_COMPONENT" ]]; then
        _TEST_COMPONENT="g8eo"
        set -- "$_TEST_COMPONENT" "${@:2}"
    else
        help_file="$SCRIPT_DIR/docs/cli_help.md"
        if [[ -f "$help_file" ]]; then
            awk '/^### test/,/^### security/' "$help_file" | head -n -1
        else
            echo "[g8e] Help file not found: $help_file" >&2
            exit 1
        fi
        exit 0
    fi
fi
if [[ "$_TEST_COMPONENT" == "-h" || "$_TEST_COMPONENT" == "--help" ]]; then
    help_file="$SCRIPT_DIR/docs/cli_help.md"
    if [[ -f "$help_file" ]]; then
        awk '/^### test/,/^### security/' "$help_file" | head -n -1
    else
        echo "[g8e] Help file not found: $help_file" >&2
        exit 1
    fi
    exit 0
fi
if [[ "$_TEST_COMPONENT" != "g8ee" && "$_TEST_COMPONENT" != "g8eo" && "$_TEST_COMPONENT" != "chaos" ]]; then
    echo "[g8e] Unknown test component: '$_TEST_COMPONENT'" >&2
    echo "  Valid: g8ee, g8eo, chaos" >&2
    exit 1
fi

_TEST_PASSTHROUGH=()
_args=("${@:2}")
_require_value() {
    if [[ ${#_args[@]} -lt 2 || "${_args[1]}" == -* || "${_args[1]}" == "--" ]]; then
        echo "[g8e] $1 requires a value" >&2
        exit 1
    fi
}
while [[ ${#_args[@]} -gt 0 ]]; do
    case "${_args[0]}" in
        -p|--llm-provider)
            _require_value "-p/--llm-provider"
            export TEST_LLM_PROVIDER="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -m|--primary-model)
            _require_value "-m/--primary-model"
            export TEST_LLM_PRIMARY_MODEL="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -a|--assistant-model)
            _require_value "-a/--assistant-model"
            export TEST_LLM_ASSISTANT_MODEL="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -l|--lite-model)
            _require_value "-l/--lite-model"
            export TEST_LLM_LITE_MODEL="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -e|--llm-endpoint-url)
            _require_value "-e/--llm-endpoint-url"
            export TEST_LLM_ENDPOINT_URL="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -k|--llm-api-key)
            _require_value "-k/--llm-api-key"
            export TEST_LLM_API_KEY="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -P|--web-search-project)
            _require_value "-P/--web-search-project"
            export TEST_WEB_SEARCH_PROJECT_ID="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -E|--web-search-engine)
            _require_value "-E/--web-search-engine"
            export TEST_WEB_SEARCH_ENGINE_ID="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -K|--web-search-api-key)
            _require_value "-K/--web-search-api-key"
            export TEST_WEB_SEARCH_API_KEY="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -L|--web-search-location)
            _require_value "-L/--web-search-location"
            export TEST_WEB_SEARCH_LOCATION="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -d|--device-token)
            _require_value "-d/--device-token"
            export DEVICE_TOKEN="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -j|--parallel)
            if [[ ${#_args[@]} -ge 2 && ( "${_args[1]}" == "auto" || "${_args[1]}" =~ ^[0-9]+$ ) ]]; then
                _TEST_PASSTHROUGH+=("--parallel" "${_args[1]}")
                _args=("${_args[@]:2}")
            else
                _TEST_PASSTHROUGH+=("--parallel" "auto")
                _args=("${_args[@]:1}")
            fi ;;
        --ruff)
            _TEST_PASSTHROUGH+=("--ruff")
            _args=("${_args[@]:1}") ;;
        --ruff-fix)
            _TEST_PASSTHROUGH+=("--ruff-fix")
            _args=("${_args[@]:1}") ;;
        --)
            _TEST_PASSTHROUGH+=("${_args[@]}")
            break ;;
        *)
            _TEST_PASSTHROUGH+=("${_args[0]}")
            _args=("${_args[@]:1}") ;;
    esac
done
_TEST_PASSTHROUGH=("$_TEST_COMPONENT" "${_TEST_PASSTHROUGH[@]}")

exec "$SCRIPT_DIR/scripts/testing/run_tests.sh" "${_TEST_PASSTHROUGH[@]}"
