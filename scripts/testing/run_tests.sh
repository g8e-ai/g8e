#!/bin/bash
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

# g8e Test Runner
#
# Runs substrate tests by default and optional app tests only when requested.
# Supports native Go toolchain for the substrate plus virtualenvs/npm for app targets.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
. "${SCRIPT_DIR}/../core/path_utils.sh"
PROJECT_ROOT="${G8E_PROJECT_ROOT:-$(resolve_g8e_root)}"

# =============================================================================
# Helper functions
# =============================================================================

RED=$'\033[0;31m'
GREEN=$'\033[0;32m'
BLUE=$'\033[0;34m'
YELLOW=$'\033[1;33m'
CYAN=$'\033[0;36m'
NC=$'\033[0m'

log_header() {
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}
log_ok() { echo -e "${GREEN}[OK]${NC} $1"; }
log_err() { echo -e "${RED}[ERROR]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

_footer() {
    local rc=$?
    [[ $rc -eq 0 ]] || return
    echo -e "\n${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}  run_tests.sh complete${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
}
trap _footer EXIT

_ensure_g8ee_venv() {
    local venv_dir="$PROJECT_ROOT/services/g8ee/.venv"
    if [[ ! -d "$venv_dir" ]]; then
        log_warn "g8ee virtualenv not found at $venv_dir. Bootstrapping automatically..."
        python3 -m venv "$venv_dir"
        "$venv_dir/bin/pip" install --upgrade pip
        "$venv_dir/bin/pip" install -r "$PROJECT_ROOT/services/g8ee/requirements.txt"
    fi
}

_check_pki_exists() {
    if [[ ! -d "$PROJECT_ROOT/.g8e/pki" ]]; then
        log_warn "Runtime PKI directory (.g8e/pki) not found. Some mTLS/integration tests may fail."
        log_warn "Run './g8e platform start' to initialize PKI trust material."
    fi
}

# =============================================================================
# Parse arguments
# =============================================================================

COMPONENT=""
COVERAGE=false
PYRIGHT=false
RUFF=false
RUFF_FIX=false
E2E=false
PARALLEL=""
QUIET=false
EXTRA_ARGS=()

DETECTED_COMPONENT=""
for arg in "$@"; do
    if [[ "$arg" =~ ^(g8eo|g8ee|chaos|ci)$ ]]; then
        DETECTED_COMPONENT="$arg"
        break
    fi
done

while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h)
            if [[ "$DETECTED_COMPONENT" == "g8eo" ]]; then
                echo "Usage: ./g8e test g8eo [OPTIONS] [-- EXTRA_ARGS]"
                echo ""
                echo "Go Operator substrate tests (Policy Execution Point & mTLS translation gateway)."
                echo ""
                echo "Options:"
                echo "  --coverage                Generate Go coverage report (coverage.out)"
                echo "  [path]                    Specific Go package or test path to run (relative to services/g8eo)"
                echo "                            e.g. services/pubsub, services/listen, ./cmd/server"
                echo ""
                echo "Go-specific Pass-through Options (via '--' or directly):"
                echo "  -v                        Verbose output (print all test names and logs)"
                echo "  -run <pattern>            Run only tests matching the regular expression pattern"
                echo "  -count <N>                Run each test N times (useful for debugging flaky tests)"
                echo "  -timeout <duration>       Override the default test timeout (default: 180s)"
                echo ""
                echo "Environment Variables:"
                echo "  G8E_STRICT_CONSTANTS_LINT=1  Enforce strict constants validation in internal contracts"
                echo ""
                echo "Examples:"
                echo "  ./g8e test g8eo"
                echo "  ./g8e test g8eo services/pubsub"
                echo "  ./g8e test g8eo --coverage"
                echo "  ./g8e test g8eo -- -run TestRecordEvent"
                echo "  ./g8e test g8eo -- -v -run TestWarden"
            elif [[ "$DETECTED_COMPONENT" == "g8ee" ]]; then
                echo "Usage: ./g8e test g8ee [OPTIONS] [-- EXTRA_ARGS]"
                echo ""
                echo "Python Engine adapter tests (stateless reasoning agent, context delivery, triage)."
                echo ""
                echo "Options:"
                echo "  --coverage                Generate pytest coverage reports"
                echo "  --pyright                 Run pyright strict static typing checks"
                echo "  --ruff                    Run ruff linter check"
                echo "  --ruff-fix                Run ruff linter check and auto-fix issues"
                echo "  --e2e                     Run E2E operator lifecycle tests (-m e2e)"
                echo "  -j, --parallel <N|auto>   Run pytest in parallel via pytest-xdist (defaults to auto)"
                echo "  [path]                    Specific pytest path to run (relative to services/g8ee)"
                echo "                            e.g. tests/unit, tests/unit/test_auth.py"
                echo ""
                echo "LLM Configuration:"
                echo "  -p, --llm-provider <provider>       LLM provider (anthropic, openai, gemini, ollama, llamacpp)"
                echo "  -m, --primary-model <model>         Primary model name"
                echo "  -a, --assistant-model <model>       Assistant model name"
                echo "  -l, --lite-model <model>            Lite model name"
                echo "  -e, --llm-endpoint-url <url>       Custom endpoint URL"
                echo "  -k, --llm-api-key <key>            API key"
                echo ""
                echo "Web Search Configuration:"
                echo "  -P, --web-search-project <id>       Google Cloud Project ID"
                echo "  -E, --web-search-engine <id>        Custom Search Engine ID"
                echo "  -K, --web-search-api-key <key>      Google API key"
                echo "  -L, --web-search-location <loc>     Search location (e.g., 'country=US')"
                echo ""
                echo "Pytest Pass-through Options (via '--' or directly):"
                echo "  -k <pattern>              Run only tests matching the expression"
                echo "  -v, --verbose             Verbose pytest output"
                echo ""
                echo "Environment Variables:"
                echo "  G8E_STRICT_SSE=true       Enforce strict JSON schema validation for SSE envelope structures"
                echo ""
                echo "Examples:"
                echo "  ./g8e test g8ee tests/unit"
                echo "  ./g8e test g8ee --coverage"
                echo "  ./g8e test g8ee --pyright --ruff"
                echo "  ./g8e test g8ee -j auto"
                echo "  ./g8e test g8ee -p gemini -m gemini-3-flash-preview -k \$GEMINI_API_KEY tests/unit"
                echo "  ./g8e test g8ee -p anthropic -m claude-3-5-sonnet -a claude-3-haiku -k \$ANTHROPIC_API_KEY"
            elif [[ "$DETECTED_COMPONENT" == "chaos" ]]; then
                echo "Usage: ./g8e test chaos [OPTIONS]"
                echo ""
                echo "Run the g8eo Chaos Tester against the local audit stack (drives the TransactionVerifier + Warden stack directly in-process)."
                echo ""
                echo "Options:"
                echo "  --count <N>               Number of chaotic payloads to generate and fire (default: 100)"
                echo "  --data-dir <path>         Override path to audit vault data dir (default: .g8e/data)"
                echo "  --pki-dir <path>          Override path to PKI directory (default: .g8e/pki)"
                echo ""
                echo "Examples:"
                echo "  ./g8e test chaos"
                echo "  ./g8e test chaos --count=500"
                echo "  ./g8e test chaos --data-dir=/tmp/g8e-data --pki-dir=/tmp/g8e-pki"
            elif [[ "$DETECTED_COMPONENT" == "ci" ]]; then
                echo "Usage: ./g8e test ci"
                echo ""
                echo "Run the full g8e CI workflow locally to validate changes before pushing."
                echo ""
                echo "Workflow Steps:"
                echo "  1. verify-proto / lint-no-bare-session-id"
                echo "  2. lint-g8eo / lint-protocol (golangci-lint)"
                echo "  3. vulncheck-g8eo (govulncheck)"
                echo "  4. test-g8eo (Substrate unit tests)"
                echo "  5. constants-lint (Verify no raw string literals in Go code where constants exist)"
                echo "  6. apps-g8ee (Start g8ee application and run Pytest unit tests)"
                echo ""
                echo "Examples:"
                echo "  ./g8e test ci"
            else
                echo "Usage: ./g8e test [COMPONENT] [OPTIONS]"
                echo ""
                echo "Components:"
                echo "  g8eo                      Go Operator substrate tests (default component)"
                echo "  g8ee                      Python Engine adapter tests"
                echo "  chaos                     Run local audit stack Chaos Tester"
                echo "  ci                        Run full CI pipeline locally"
                echo ""
                echo "To see help and unique options/flags for a specific component:"
                echo "  ./g8e test g8eo -h"
                echo "  ./g8e test g8ee -h"
                echo "  ./g8e test chaos -h"
                echo "  ./g8e test ci -h"
            fi
            exit 0
            ;;
        --coverage) COVERAGE=true; shift ;;
        --pyright)  PYRIGHT=true;  shift ;;
        --ruff)     RUFF=true;     shift ;;
        --ruff-fix) RUFF=true; RUFF_FIX=true; shift ;;
        --e2e)      E2E=true;      shift ;;
        -q|--quiet)
            QUIET=true
            shift ;;
        -j|--parallel)
            if [[ $# -lt 2 || "$2" == -* || "$2" == "--" ]]; then
                PARALLEL="auto"
                shift
            else
                PARALLEL="$2"
                shift 2
            fi ;;
        --)
            shift
            EXTRA_ARGS=("$@")
            break
            ;;
        *)
            if [[ "$1" =~ ^(g8ee|g8eo|chaos|ci)$ ]]; then
                COMPONENT="$1"
            else
                EXTRA_ARGS+=("$1")
            fi
            shift
            ;;
    esac
done

if [[ -z "$COMPONENT" ]]; then
    COMPONENT="g8eo"
fi

_prompt_llm_config() {
    # Skip if already provided via env/flags OR if not interactive
    [[ -n "${G8E_TEST_LLM_PROVIDER:-}" ]] && return
    
    echo ""
    log_warn "LLM credentials not set. AI integration tests will be skipped."
    log_warn "To enable them, set G8E_TEST_LLM_PROVIDER and G8E_TEST_LLM_API_KEY."
    echo ""
}

_show_llm_config() {
    if [[ -n "${G8E_TEST_LLM_PROVIDER:-}" ]]; then
        local provider="${G8E_TEST_LLM_PROVIDER}"
        local primary_model="${G8E_TEST_LLM_PRIMARY_MODEL:-}"
        
        # Determine default model if not set
        if [[ -z "$primary_model" ]]; then
            case "$provider" in
                openai)    primary_model="gpt-5.4 (default)" ;;
                anthropic) primary_model="claude-opus-4-6 (default)" ;;
                gemini)    primary_model="gemini-3-flash-preview (default)" ;;
                ollama)    primary_model="gemma4:e4b (default)" ;;
                llamacpp)  primary_model="gemma4:e2b (default)" ;;
            esac
        fi

        echo ""
        echo -e "${CYAN}  LLM Configuration${NC}"
        echo -e "  Primary Provider:   ${provider}"
        [[ -n "${G8E_TEST_LLM_ASSISTANT_PROVIDER:-}" ]] && echo -e "  Assistant Provider: ${G8E_TEST_LLM_ASSISTANT_PROVIDER}"
        [[ -n "${G8E_TEST_LLM_LITE_PROVIDER:-}" ]]      && echo -e "  Lite Provider:      ${G8E_TEST_LLM_LITE_PROVIDER}"
        echo -e "  Primary Model:      ${primary_model}"
        [[ -n "${G8E_TEST_LLM_ASSISTANT_MODEL:-}" ]]     && echo -e "  Assistant Model:    ${G8E_TEST_LLM_ASSISTANT_MODEL}"
        [[ -n "${G8E_TEST_LLM_LITE_MODEL:-}" ]]          && echo -e "  Lite Model:         ${G8E_TEST_LLM_LITE_MODEL}"
        [[ -n "${G8E_TEST_LLM_ENDPOINT_URL:-}" ]]        && echo -e "  Primary Endpoint:   ${G8E_TEST_LLM_ENDPOINT_URL}"
        [[ -n "${G8E_TEST_LLM_API_KEY:-}" ]]             && echo -e "  Primary API Key:    (set)"
        echo ""
    else
        echo ""
        echo -e "  ${YELLOW}No LLM flags provided - ai_integration tests will be skipped.${NC}"
        echo ""
    fi
}

_show_web_search_config() {
    if [[ -n "${G8E_TEST_WEB_SEARCH_PROJECT_ID:-}" ]] && [[ -n "${G8E_TEST_WEB_SEARCH_ENGINE_ID:-}" ]] && [[ -n "${G8E_TEST_WEB_SEARCH_API_KEY:-}" ]]; then
        echo ""
        echo -e "${CYAN}  Web Search Configuration${NC}"
        echo -e "  Project ID:      ${G8E_TEST_WEB_SEARCH_PROJECT_ID}"
        echo -e "  Engine ID:       ${G8E_TEST_WEB_SEARCH_ENGINE_ID}"
        echo -e "  API Key:         (set)"
        echo ""
    else
        echo ""
        echo -e "  ${YELLOW}No web search flags - requires_web_search tests will be skipped.${NC}"
        echo ""
    fi
}

# =============================================================================
# Component Runners
# =============================================================================

run_g8ee() {
    log_header "Running g8ee tests (host)"
    _ensure_g8ee_venv
    _check_pki_exists
    local venv_dir="$PROJECT_ROOT/services/g8ee/.venv"
    
    export PYTHONPATH="$PROJECT_ROOT/services/g8ee:$PROJECT_ROOT/protocol/python"
    export G8E_PROTOCOL_DIR="$PROJECT_ROOT/protocol"
    export G8E_PROJECT_ROOT="$PROJECT_ROOT"

    if [[ "$PYRIGHT" == "true" ]]; then
        (set -o pipefail && cd "$PROJECT_ROOT/services/g8ee" && "$venv_dir/bin/python" -m pyright --project pyrightconfig.services.json | sed "s|$PROJECT_ROOT/services/||g")
    fi
    cd "$PROJECT_ROOT/services/g8ee"
    if [[ "$RUFF" == "true" ]]; then
        local ruff_args=(check .)
        [[ "$RUFF_FIX" == "true" ]] && ruff_args+=(--fix)
        "$venv_dir/bin/python" -m ruff "${ruff_args[@]}"
    fi
    local cov_args=(-rs)
    [[ "$COVERAGE" == "true" ]] && cov_args+=("--cov" "--cov-report=term-missing")
    if [[ -n "$PARALLEL" ]]; then
        # -s (capture=no) is incompatible with xdist; drop it when parallelising.
        local filtered=()
        for a in "${cov_args[@]}"; do
            [[ "$a" == "-s" ]] && continue
            filtered+=("$a")
        done
        cov_args=("${filtered[@]}" "-n" "$PARALLEL")
        log_ok "pytest parallelism: -n $PARALLEL"
    fi
    cd "$PROJECT_ROOT/services/g8ee"
    "$venv_dir/bin/pytest" "${cov_args[@]}" "${EXTRA_ARGS[@]}"
}

run_e2e() {
    log_header "Running E2E operator lifecycle tests (host)"
    _ensure_g8ee_venv
    _check_pki_exists
    local venv_dir="$PROJECT_ROOT/services/g8ee/.venv"
    export PYTHONPATH="$PROJECT_ROOT/services/g8ee:$PROJECT_ROOT/protocol/python"
    export G8E_PROTOCOL_DIR="$PROJECT_ROOT/protocol"
    cd "$PROJECT_ROOT/services/g8ee"
    "$venv_dir/bin/pytest" -rs -m e2e tests/e2e/ "${EXTRA_ARGS[@]}"
}

run_g8eo() {
    log_header "Running g8eo tests (host)"
    _check_pki_exists
    cd "$PROJECT_ROOT/services/g8eo"
    
    # Ensure GOPATH/bin is in PATH for Go tools
    local gopath="$(go env GOPATH 2>/dev/null || echo "$HOME/go")"
    export PATH="$gopath/bin:$PATH"
    
    # Run golangci-lint before tests
    log_header "Running golangci-lint"
    if ! command -v golangci-lint >/dev/null 2>&1; then
        log_warn "golangci-lint not found in PATH. Run 'cd services/g8eo && make install-tools' to install."
        log_warn "Skipping golangci-lint for now..."
    elif ! golangci-lint run --path-prefix=services/g8eo; then
        log_err "golangci-lint failed"
        return 1
    fi
    
    local test_target="./..."
    local pass_through_args=()

    for arg in "${EXTRA_ARGS[@]}"; do
        if [[ "$arg" == ./* || "$arg" == */* ]]; then
            if [[ "$arg" != ./* && "$arg" != /* ]]; then
                test_target="./$arg"
            else
                test_target="$arg"
            fi
        else
            pass_through_args+=("$arg")
        fi
    done

    local test_cmd="go test"
    if command -v gotestsum >/dev/null 2>&1; then
        test_cmd="gotestsum --format dots-v2 --"
    fi

    if [[ "$COVERAGE" == "true" ]]; then
        $test_cmd -race -parallel 4 -timeout 180s -coverprofile=coverage.out "$test_target" "${pass_through_args[@]}"
        local rc=$?
        if [[ -f coverage.out ]]; then
            echo ""
            log_header "g8eo Coverage Report"
            go tool cover -func=coverage.out
        fi
        return $rc
    else
        $test_cmd -race -parallel 4 -timeout 180s "$test_target" "${pass_through_args[@]}"
    fi
}

run_chaos() {
    log_header "Running g8eo Chaos Tester (host)"
    cd "$PROJECT_ROOT/services/g8eo"
    
    # Ensure binary is built or run directly with go run
    # 'go run' is simpler for a one-off tool
    go run ./cmd/chaos_tester --data-dir="$PROJECT_ROOT/.g8e/data" --pki-dir="$PROJECT_ROOT/.g8e/pki" "${EXTRA_ARGS[@]}"
    
    # Run the actual audit summary command after chaos test completes
    log_header "Running Audit Summary"
    cd "$PROJECT_ROOT"
    "$PROJECT_ROOT/g8e" data audit --db-path "$PROJECT_ROOT/.g8e/data/g8e.db" summary
}

run_ci() {
    log_header "Running full CI workflow locally"
    cd "$PROJECT_ROOT"
    
    # 1. verify-proto (Hard gates only)
    log_header "CI: verify-proto"
    
    # We skip the git drift check locally because it blocks test execution 
    # and run_tests.sh already synchronized protocols at startup.
    
    make lint-no-bare-session-id
    log_ok "verify-proto passed"

    # 2. lint-g8eo
    log_header "CI: lint-g8eo"
    (cd services/g8eo && golangci-lint run) || log_warn "golangci-lint (g8eo) failed or not installed"
    (cd protocol && golangci-lint run) || log_warn "golangci-lint (protocol) failed or not installed"
    
    # 3. vulncheck-g8eo
    log_header "CI: vulncheck-g8eo"
    if command -v govulncheck >/dev/null 2>&1; then
        (cd services/g8eo && govulncheck ./...)
    else
        log_warn "govulncheck not found, skipping"
    fi

    # 4. test-g8eo & apps-g8ee (requires platform)
    log_header "CI: Running substrate and app tests"
    
    # Skip proto generation in sub-steps since we already did it at start of run_tests.sh
    export G8E_SKIP_PROTO=true
    
    # Ensure platform is stopped before starting a fresh one
    "$PROJECT_ROOT/g8e" platform stop || true
    "$PROJECT_ROOT/g8e" platform start
    
    local test_exit_code=0
    
    # test-g8eo
    log_header "CI: test-g8eo"
    if ! run_g8eo; then
        log_err "test-g8eo failed"
        test_exit_code=1
    fi
    
    # constants-lint
    log_header "CI: constants-lint"
    if ! (cd "$PROJECT_ROOT/services/g8eo" && G8E_STRICT_CONSTANTS_LINT="1" go test -v -run TestNoRawStringLiteralsWhereConstantsExist ./internal/contracts); then
        log_err "constants-lint failed"
        test_exit_code=1
    fi

    # apps-g8ee
    log_header "CI: apps-g8ee"
    "$PROJECT_ROOT/g8e" apps start g8ee
    if ! run_g8ee; then
        log_err "apps-g8ee failed"
        test_exit_code=1
    fi
    
    "$PROJECT_ROOT/g8e" platform stop
    
    if [[ $test_exit_code -ne 0 ]]; then
        log_err "CI workflow failed"
        exit $test_exit_code
    fi
    
    log_ok "Full CI workflow passed"
}

# =============================================================================
# Main
# =============================================================================

export NODE_ENV="test"

log_header "run_tests.sh ${COMPONENT} $*"

# 0. Preparation: ensure proto/constants are up to date (done by default)
if [[ "$COMPONENT" != "chaos" && "${G8E_SKIP_PROTO:-}" != "true" ]]; then
    log_ok "Synchronizing proto and constants..."
    (cd "$PROJECT_ROOT" && make proto > /dev/null && python3 scripts/data/generate_constants.py --all > /dev/null)
fi

if [[ "$COMPONENT" == "g8ee" ]]; then
    _prompt_llm_config
    _show_llm_config
    _show_web_search_config
fi

if [[ "$E2E" == "true" && "$COMPONENT" == "g8ee" ]]; then
    run_e2e
else
    case "$COMPONENT" in
        g8ee) run_g8ee ;;
        g8eo) run_g8eo ;;
        chaos) run_chaos ;;
        ci) run_ci ;;
    esac
fi
