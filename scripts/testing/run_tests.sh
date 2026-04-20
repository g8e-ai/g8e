#!/bin/bash
# g8e Test Runner
#
# Runs inside a dedicated test-runner container (g8ee-test-runner, g8ed-test-runner,
# g8eo-test-runner). The ./g8e CLI handles container selection and docker exec.
# This script is never run on the host.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

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
E2E=false
PARALLEL=""
EXTRA_ARGS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h)
            echo "Usage: run_tests.sh <COMPONENT> [OPTIONS] [-- EXTRA_ARGS]"
            echo ""
            echo "Components: g8ee, g8ed, g8eo"
            echo ""
            echo "Options:"
            echo "  --coverage                Generate coverage reports"
            echo "  --pyright                 Run pyright strict gate (g8ee only)"
            echo "  --ruff                    Run ruff lint check (g8ee only)"
            echo "  --e2e                     Run E2E operator lifecycle tests (g8ee only)"
            echo "  -j, --parallel <N|auto>   Run pytest in parallel via pytest-xdist (g8ee only)"
            echo ""
            echo "LLM/Web Search options are passed as environment variables by the ./g8e CLI."
            exit 0
            ;;
        --coverage) COVERAGE=true; shift ;;
        --pyright)  PYRIGHT=true;  shift ;;
        --ruff)     RUFF=true;     shift ;;
        --e2e)      E2E=true;      shift ;;
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
            if [[ "$1" =~ ^(g8ee|g8ed|g8eo)$ ]]; then
                COMPONENT="$1"
            else
                EXTRA_ARGS+=("$1")
            fi
            shift
            ;;
    esac
done

if [[ -z "$COMPONENT" ]]; then
    log_err "No component specified. Usage: run_tests.sh <g8ee|g8ed|g8eo> [OPTIONS]"
    exit 1
fi

# =============================================================================
# Container Environment Setup
# =============================================================================

_install_ca_cert() {
    local ca_cert="/g8es/ca.crt"
    [[ ! -f "$ca_cert" ]] && ca_cert="/g8es/ca/ca.crt"
    if [[ ! -f "$ca_cert" ]]; then
        log_warn "Platform CA cert not found"
        return
    fi
    export G8E_SSL_CERT_FILE="$ca_cert"
    export NODE_EXTRA_CA_CERTS="$ca_cert"
    log_ok "Platform CA cert set (G8E_SSL_CERT_FILE=$ca_cert)"
}

_load_platform_secrets() {
    local auth_token_file="/g8es/internal_auth_token"
    local session_key_file="/g8es/session_encryption_key"
    if [[ -f "$auth_token_file" ]]; then
        export G8E_INTERNAL_AUTH_TOKEN=$(cat "$auth_token_file")
        log_ok "G8E_INTERNAL_AUTH_TOKEN loaded from /g8es"
    fi
    if [[ -f "$session_key_file" ]]; then
        export G8E_SESSION_ENCRYPTION_KEY=$(cat "$session_key_file")
        log_ok "G8E_SESSION_ENCRYPTION_KEY loaded from /g8es"
    fi
}

_verify_g8es() {
    local ca_cert="/g8es/ca.crt"
    [[ ! -f "$ca_cert" ]] && ca_cert="/g8es/ca/ca.crt"
    local curl_args=()
    if [[ -f "$ca_cert" ]]; then
        curl_args=("--cacert" "$ca_cert")
    else
        curl_args=("-k")
        log_warn "CA cert not found, using insecure connection"
    fi
    if ! curl -sf "${curl_args[@]}" https://g8es:9000/health 2>/dev/null | grep -q '"status":"ok"'; then
        log_err "g8es not accessible at https://g8es:9000/health"
        exit 1
    fi
    log_ok "g8es connected"
}

_show_llm_config() {
    if [[ -n "${TEST_LLM_PROVIDER:-}" ]]; then
        echo ""
        echo -e "${CYAN}  LLM Configuration${NC}"
        echo -e "  Primary Provider:   ${TEST_LLM_PROVIDER}"
        [[ -n "${TEST_LLM_ASSISTANT_PROVIDER:-}" ]] && echo -e "  Assistant Provider: ${TEST_LLM_ASSISTANT_PROVIDER}"
        [[ -n "${TEST_LLM_PRIMARY_MODEL:-}" ]]      && echo -e "  Primary Model:      ${TEST_LLM_PRIMARY_MODEL}"
        [[ -n "${TEST_LLM_ASSISTANT_MODEL:-}" ]]     && echo -e "  Assistant Model:    ${TEST_LLM_ASSISTANT_MODEL}"
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
    log_header "Running g8ee tests (g8ee-test-runner)"
    cd "$PROJECT_ROOT/components/g8ee"
    if [[ "$PYRIGHT" == "true" ]]; then
        python -m pyright --project pyrightconfig.services.json
    fi
    local cov_args=(-rs)
    [[ "$COVERAGE" == "true" ]] && cov_args+=("--cov" "--cov-report=term-missing")
    [[ -n "${TEST_LLM_PROVIDER:-}" ]] && cov_args+=("--log-cli-level=INFO")
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
    pytest "${cov_args[@]}" "${EXTRA_ARGS[@]}"
}

run_e2e() {
    log_header "Running E2E operator lifecycle tests (g8ee-test-runner)"
    cd "$PROJECT_ROOT/components/g8ee"
    pytest -rs -m e2e tests/e2e/ "${EXTRA_ARGS[@]}"
}

run_g8ed() {
    log_header "Running g8ed tests (g8ed-test-runner)"
    cd "$PROJECT_ROOT/components/g8ed"
    local cov_flag=""
    [[ "$COVERAGE" == "true" ]] && cov_flag="--coverage"
    NODE_PATH="./node_modules" npx vitest run --config ./vitest.config.js $cov_flag "${EXTRA_ARGS[@]}"
}

run_g8eo() {
    log_header "Running g8eo tests (g8eo-test-runner)"
    cd "$PROJECT_ROOT/components/g8eo"
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

    if [[ "$COVERAGE" == "true" ]]; then
        gotestsum --format dots-v2 -- -race -timeout 180s -coverprofile=coverage.out "$test_target" "${pass_through_args[@]}"
        local rc=$?
        if [[ -f coverage.out ]]; then
            echo ""
            log_header "g8eo Coverage Report"
            go tool cover -func=coverage.out
        fi
        return $rc
    else
        gotestsum --format dots-v2 -- -race -timeout 180s "$test_target" "${pass_through_args[@]}"
    fi
}

# =============================================================================
# Main
# =============================================================================

export NODE_ENV="test"
_install_ca_cert
_load_platform_secrets
_verify_g8es

log_header "run_tests.sh ${COMPONENT} $*"

if [[ "$COMPONENT" == "g8ee" ]]; then
    _show_llm_config
    _show_web_search_config
fi

if [[ "$E2E" == "true" && "$COMPONENT" == "g8ee" ]]; then
    run_e2e
else
    case "$COMPONENT" in
        g8ee) run_g8ee ;;
        g8ed) run_g8ed ;;
        g8eo) run_g8eo ;;
    esac
fi
