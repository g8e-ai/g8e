#!/bin/bash
# g8e Test Runner
#
# Runs substrate tests by default and optional app tests only when requested.
# Supports native Go toolchain for the substrate plus virtualenvs/npm for app targets.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "${SCRIPT_DIR}/../core/path_utils.sh"
PROJECT_ROOT="$G8E_PROJECT_ROOT"

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

while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h)
            echo "Usage: run_tests.sh [COMPONENT] [OPTIONS] [-- EXTRA_ARGS]"
            echo ""
            echo "Components: g8eo (default substrate), g8ee"
            echo ""
            echo "Options:"
            echo "  --coverage                Generate coverage reports"
            echo "  --pyright                 Run pyright strict gate (g8ee only)"
            echo "  --ruff                    Run ruff lint check (g8ee only)"
            echo "  --ruff-fix                Run ruff with --fix to auto-fix issues (g8ee only)"
            echo "  --e2e                     Run E2E operator lifecycle tests (g8ee only)"
            echo "  -j, --parallel <N|auto>   Run pytest in parallel via pytest-xdist (g8ee only)"
            echo ""
            echo "Examples (via ./g8e CLI):"
            echo "  ./g8e test"
            echo "  ./g8e test g8eo services/pubsub"
            echo "  ./g8e test g8ee tests/unit"
            echo "  ./g8e test g8ee --coverage"
            echo "  ./g8e test g8ee --pyright --ruff"
            echo "  ./g8e test g8ee --e2e"
            echo "  ./g8e test g8ee -j auto"
            echo "  ./g8e test g8eo services/listen"
            echo "  ./g8e test g8eo ./cmd/server"
            echo ""
            echo "LLM/Web Search configuration (g8ee only):"
            echo "  ./g8e test g8ee -p anthropic -m claude-3-5-sonnet -k <api-key> tests/unit"
            echo "  ./g8e test g8ee -p openai -m gpt-4 -a gpt-3.5-turbo -k <api-key> --coverage"
            echo ""
            echo "LLM/Web Search options are passed as environment variables by the ./g8e CLI."
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
            if [[ "$1" =~ ^(g8ee|g8eo|chaos)$ ]]; then
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
    # Check for G8E_ prefixed vars and populate standard ones if found
    if [[ -n "${G8E_TEST_LLM_PROVIDER:-}" ]]; then
        export TEST_LLM_PROVIDER="$G8E_TEST_LLM_PROVIDER"
    fi
    if [[ -n "${G8E_TEST_LLM_API_KEY:-}" ]]; then
        export TEST_LLM_API_KEY="$G8E_TEST_LLM_API_KEY"
    fi

    # Skip if already provided via env/flags OR if not interactive
    [[ -n "${TEST_LLM_PROVIDER:-}" ]] && return
    
    echo ""
    log_warn "LLM credentials not set. AI integration tests will be skipped."
    log_warn "To enable them, set G8E_TEST_LLM_PROVIDER and G8E_TEST_LLM_API_KEY."
    echo ""
}

_show_llm_config() {
    if [[ -n "${TEST_LLM_PROVIDER:-}" ]]; then
        local provider="${TEST_LLM_PROVIDER}"
        local primary_model="${TEST_LLM_PRIMARY_MODEL:-}"
        
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
        [[ -n "${TEST_LLM_ASSISTANT_PROVIDER:-}" ]] && echo -e "  Assistant Provider: ${TEST_LLM_ASSISTANT_PROVIDER}"
        [[ -n "${TEST_LLM_LITE_PROVIDER:-}" ]]      && echo -e "  Lite Provider:      ${TEST_LLM_LITE_PROVIDER}"
        echo -e "  Primary Model:      ${primary_model}"
        [[ -n "${TEST_LLM_ASSISTANT_MODEL:-}" ]]     && echo -e "  Assistant Model:    ${TEST_LLM_ASSISTANT_MODEL}"
        [[ -n "${TEST_LLM_LITE_MODEL:-}" ]]          && echo -e "  Lite Model:         ${TEST_LLM_LITE_MODEL}"
        [[ -n "${TEST_LLM_ENDPOINT_URL:-}" ]]        && echo -e "  Primary Endpoint:   ${TEST_LLM_ENDPOINT_URL}"
        [[ -n "${TEST_LLM_API_KEY:-}" ]]             && echo -e "  Primary API Key:    (set)"
        echo ""
    else
        echo ""
        echo -e "  ${YELLOW}No LLM flags provided — ai_integration tests will be skipped.${NC}"
        echo ""
    fi
}

_show_web_search_config() {
    if [[ -n "${TEST_WEB_SEARCH_PROJECT_ID:-}" ]] && [[ -n "${TEST_WEB_SEARCH_ENGINE_ID:-}" ]] && [[ -n "${TEST_WEB_SEARCH_API_KEY:-}" ]]; then
        echo ""
        echo -e "${CYAN}  Web Search Configuration${NC}"
        echo -e "  Project ID:      ${TEST_WEB_SEARCH_PROJECT_ID}"
        echo -e "  Engine ID:       ${TEST_WEB_SEARCH_ENGINE_ID}"
        echo -e "  API Key:         (set)"
        echo ""
    else
        echo ""
        echo -e "  ${YELLOW}No web search flags — requires_web_search tests will be skipped.${NC}"
        echo ""
    fi
}

# =============================================================================
# Component Runners
# =============================================================================

run_g8ee() {
    log_header "Running g8ee tests (host)"
    local venv_dir="$PROJECT_ROOT/services/g8ee/.venv"
    if [[ ! -d "$venv_dir" ]]; then
        log_err "g8ee virtualenv not found at $venv_dir. Run ./g8e platform start first."
        exit 1
    fi
    
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
    local venv_dir="$PROJECT_ROOT/services/g8ee/.venv"
    if [[ ! -d "$venv_dir" ]]; then
        log_err "g8ee virtualenv not found at $venv_dir. Run ./g8e platform start first."
        exit 1
    fi
    export PYTHONPATH="$PROJECT_ROOT/services/g8ee:$PROJECT_ROOT/protocol/python"
    export G8E_PROTOCOL_DIR="$PROJECT_ROOT/protocol"
    cd "$PROJECT_ROOT/services/g8ee"
    "$venv_dir/bin/pytest" -rs -m e2e tests/e2e/ "${EXTRA_ARGS[@]}"
}

run_g8eo() {
    log_header "Running g8eo tests (host)"
    cd "$PROJECT_ROOT/services/g8eo"
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
}

# =============================================================================
# Main
# =============================================================================

export NODE_ENV="test"

log_header "run_tests.sh ${COMPONENT} $*"

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
    esac
fi
