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

_TEST_COMPONENT="${1:-}"
if [[ -z "$_TEST_COMPONENT" || "$_TEST_COMPONENT" == "-h" || "$_TEST_COMPONENT" == "--help" ]]; then
    if [[ -z "$_TEST_COMPONENT" ]]; then
        _TEST_COMPONENT="g8eo"
        set -- "$_TEST_COMPONENT" "${@:2}"
    else
        cat <<'EOF'
### test (Unit tests)
Testing and evaluation tools for the substrate and applications.

Subcommands:
  g8eo [path]  Remote Operator tests with race detection. This is the default when no component is provided.
  g8ee [path]  Optional Python Ensemble adapter tests with LLM provider support.
  ci           Run all CI workflow steps locally (proto verify, lint, vulncheck, Operator tests, app tests).
  chaos [options]  Run the g8eo Chaos Tester against the local audit stack.
EOF
        exit 0
    fi
fi
if [[ "$_TEST_COMPONENT" == "-h" || "$_TEST_COMPONENT" == "--help" ]]; then
    cat <<'EOF'
### test (Unit tests)
Testing and evaluation tools for the substrate and applications.

Subcommands:
  g8eo [path]  Remote Operator tests with race detection. This is the default when no component is provided.
  g8ee [path]  Optional Python Ensemble adapter tests with LLM provider support.
  ci           Run all CI workflow steps locally (proto verify, lint, vulncheck, Operator tests, app tests).
  chaos [options]  Run the g8eo Chaos Tester against the local audit stack.
EOF
    exit 0
fi
if [[ "$_TEST_COMPONENT" != "g8ee" && "$_TEST_COMPONENT" != "g8eo" && "$_TEST_COMPONENT" != "chaos" && "$_TEST_COMPONENT" != "ci" ]]; then
    echo "[g8e] Unknown test component: '$_TEST_COMPONENT'" >&2
    echo "  Valid: g8ee, g8eo, chaos, ci" >&2
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
            export G8E_TEST_LLM_PRIMARY_PROVIDER="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        --assistant-provider)
            _require_value "--assistant-provider"
            export G8E_TEST_LLM_ASSISTANT_PROVIDER="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        --lite-provider)
            _require_value "--lite-provider"
            export G8E_TEST_LLM_LITE_PROVIDER="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -m|--primary-model)
            _require_value "-m/--primary-model"
            export G8E_TEST_LLM_PRIMARY_MODEL="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -a|--assistant-model)
            _require_value "-a/--assistant-model"
            export G8E_TEST_LLM_ASSISTANT_MODEL="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -l|--lite-model)
            _require_value "-l/--lite-model"
            export G8E_TEST_LLM_LITE_MODEL="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -e|--llm-endpoint-url)
            _require_value "-e/--llm-endpoint-url"
            export G8E_TEST_LLM_PRIMARY_ENDPOINT_URL="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        --assistant-endpoint)
            _require_value "--assistant-endpoint"
            export G8E_TEST_LLM_ASSISTANT_ENDPOINT_URL="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        --lite-endpoint)
            _require_value "--lite-endpoint"
            export G8E_TEST_LLM_LITE_ENDPOINT_URL="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -k|--llm-api-key)
            _require_value "-k/--llm-api-key"
            export G8E_TEST_LLM_PRIMARY_API_KEY="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        --assistant-api-key)
            _require_value "--assistant-api-key"
            export G8E_TEST_LLM_ASSISTANT_API_KEY="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        --lite-api-key)
            _require_value "--lite-api-key"
            export G8E_TEST_LLM_LITE_API_KEY="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -P|--web-search-project)
            _require_value "-P/--web-search-project"
            export G8E_TEST_WEB_SEARCH_PROJECT_ID="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -E|--web-search-engine)
            _require_value "-E/--web-search-engine"
            export G8E_TEST_WEB_SEARCH_ENGINE_ID="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -K|--web-search-api-key)
            _require_value "-K/--web-search-api-key"
            export G8E_TEST_WEB_SEARCH_API_KEY="${_args[1]}"
            _args=("${_args[@]:2}") ;;
        -L|--web-search-location)
            _require_value "-L/--web-search-location"
            export G8E_TEST_WEB_SEARCH_LOCATION="${_args[1]}"
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
