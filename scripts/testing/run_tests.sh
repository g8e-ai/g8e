#!/bin/bash
# g8e node g8e node
#
# Runs tests in the g8e-pod container. Infrastructure must already be running (use build.sh).

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
BOLD=$'\033[1m'
NC=$'\033[0m'

log_header() {
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}  [g8e-pod] $1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}
log_ok() { echo -e "${GREEN}[OK]${NC} $1"; }
log_err() { echo -e "${RED}[ERROR]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

in_container() {
    [[ -f /.dockerenv ]] || [[ "${RUNNING_IN_CONTAINER:-}" == "true" ]]
}

_footer() {
    local rc=$?
    [[ $rc -eq 0 ]] || return
    echo -e "\n${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}  [g8e-pod] run_tests.sh complete${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
}
trap _footer EXIT

# =============================================================================
# Parse arguments
# =============================================================================

COMPONENT="all"
COVERAGE=false
PYRIGHT=false
TEST_LLM_PROVIDER="${TEST_LLM_PROVIDER:-}"
TEST_LLM_PRIMARY_MODEL="${TEST_LLM_PRIMARY_MODEL:-}"
TEST_LLM_ASSISTANT_MODEL="${TEST_LLM_ASSISTANT_MODEL:-}"
TEST_LLM_ENDPOINT_URL="${TEST_LLM_ENDPOINT_URL:-}"
TEST_LLM_API_KEY="${TEST_LLM_API_KEY:-}"
TEST_WEB_SEARCH_PROJECT_ID="${TEST_WEB_SEARCH_PROJECT_ID:-}"
TEST_WEB_SEARCH_ENGINE_ID="${TEST_WEB_SEARCH_ENGINE_ID:-}"
TEST_WEB_SEARCH_API_KEY="${TEST_WEB_SEARCH_API_KEY:-}"
TEST_WEB_SEARCH_LOCATION="${TEST_WEB_SEARCH_LOCATION:-}"
EXTRA_ARGS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h)
            echo "Usage: ./run_tests.sh [COMPONENT] [OPTIONS] [-- EXTRA_ARGS]"
            echo ""
            echo "Components: all, g8ee, vsod, vsa, security"
            echo ""
            echo "Options:"
            echo "  --coverage                Generate coverage reports"
            echo "  --pyright                 Run pyright strict gate (g8ee only)"
            echo ""
            echo "LLM Options (enables ai_integration tests):"
            echo "  -p, --llm-provider        LLM provider (gemini, openai, anthropic, ollama)"
            echo "  -m, --primary-model       Primary model name"
            echo "  -a, --assistant-model      Assistant model name"
            echo "  -e, --llm-endpoint-url    API endpoint URL"
            echo "  -k, --llm-api-key         API key"
            echo ""
            echo "Web Search Options (enables requires_web_search tests):"
            echo "  --web-search-project-id   GCP project ID"
            echo "  --web-search-engine-id    Google search engine ID"
            echo "  --web-search-api-key      Google search API key"
            echo "  --web-search-location     Vertex AI location (default: global)"
            echo ""
            exit 0
            ;;
        --coverage)
            COVERAGE=true
            shift
            ;;
        --pyright)
            PYRIGHT=true
            shift
            ;;
        --llm-provider|-p)
            TEST_LLM_PROVIDER="$2"; shift 2 ;;
        --llm-provider=*)
            TEST_LLM_PROVIDER="${1#*=}"; shift ;;
        --primary-model|-m)
            TEST_LLM_PRIMARY_MODEL="$2"; shift 2 ;;
        --primary-model=*)
            TEST_LLM_PRIMARY_MODEL="${1#*=}"; shift ;;
        --assistant-model|-a)
            TEST_LLM_ASSISTANT_MODEL="$2"; shift 2 ;;
        --assistant-model=*)
            TEST_LLM_ASSISTANT_MODEL="${1#*=}"; shift ;;
        --llm-endpoint-url|-e)
            TEST_LLM_ENDPOINT_URL="$2"; shift 2 ;;
        --llm-endpoint-url=*)
            TEST_LLM_ENDPOINT_URL="${1#*=}"; shift ;;
        --llm-api-key|-k)
            TEST_LLM_API_KEY="$2"; shift 2 ;;
        --llm-api-key=*)
            TEST_LLM_API_KEY="${1#*=}"; shift ;;
        --web-search-project-id)
            TEST_WEB_SEARCH_PROJECT_ID="$2"; shift 2 ;;
        --web-search-project-id=*)
            TEST_WEB_SEARCH_PROJECT_ID="${1#*=}"; shift ;;
        --web-search-engine-id)
            TEST_WEB_SEARCH_ENGINE_ID="$2"; shift 2 ;;
        --web-search-engine-id=*)
            TEST_WEB_SEARCH_ENGINE_ID="${1#*=}"; shift ;;
        --web-search-api-key)
            TEST_WEB_SEARCH_API_KEY="$2"; shift 2 ;;
        --web-search-api-key=*)
            TEST_WEB_SEARCH_API_KEY="${1#*=}"; shift ;;
        --web-search-location)
            TEST_WEB_SEARCH_LOCATION="$2"; shift 2 ;;
        --web-search-location=*)
            TEST_WEB_SEARCH_LOCATION="${1#*=}"; shift ;;
        --)
            shift
            EXTRA_ARGS=("$@")
            break
            ;;
        *)
            if [[ "$1" =~ ^(all|g8ee|vsod|vsa|security)$ ]]; then
                COMPONENT="$1"
            else
                EXTRA_ARGS+=("$1")
            fi
            shift
            ;;
    esac
done

# =============================================================================
# Container Environment Setup
# =============================================================================

_settings_script=(/app/scripts/data/manage-vsodb.py settings)

_install_ca_cert() {
    local ca_cert="/vsodb/ca.crt"
    [[ ! -f "$ca_cert" ]] && ca_cert="/vsodb/ca/ca.crt"
    if [[ ! -f "$ca_cert" ]]; then
        log_warn "Platform CA cert not found"
        return
    fi
    export G8E_SSL_CERT_FILE="$ca_cert"
    export REQUESTS_CA_BUNDLE="$ca_cert"
    export NODE_EXTRA_CA_CERTS="$ca_cert"
    log_ok "Platform CA cert set (G8E_SSL_CERT_FILE=$ca_cert)"
}

_load_platform_secrets() {
    local auth_token_file="/vsodb/internal_auth_token"
    local session_key_file="/vsodb/session_encryption_key"
    if [[ -f "$auth_token_file" ]]; then
        export G8E_INTERNAL_AUTH_TOKEN=$(cat "$auth_token_file")
        log_ok "G8E_INTERNAL_AUTH_TOKEN loaded from /vsodb"
    fi
    if [[ -f "$session_key_file" ]]; then
        export G8E_SESSION_ENCRYPTION_KEY=$(cat "$session_key_file")
        log_ok "G8E_SESSION_ENCRYPTION_KEY loaded from /vsodb"
    fi
}

setup_container_environment() {
    export NODE_ENV="test"
    _install_ca_cert
    _load_platform_secrets
}

verify_container_infrastructure() {
    local ca_cert="/vsodb/ca.crt"
    [[ ! -f "$ca_cert" ]] && ca_cert="/vsodb/ca/ca.crt"
    local curl_args=()
    if [[ -f "$ca_cert" ]]; then
        curl_args=("--cacert" "$ca_cert")
    else
        curl_args=("-k")  # Fallback to insecure if CA not found
        log_warn "CA cert not found, using insecure connection"
    fi
    
    if ! curl -sf "${curl_args[@]}" https://vsodb:9000/health 2>/dev/null | grep -q '"status":"ok"'; then
        log_err "VSODB not accessible at https://vsodb:9000/health"
        exit 1
    fi
    log_ok "VSODB connected"
}

# =============================================================================
# Component Runners
# =============================================================================

run_g8ee() {
    log_header "Running g8ee tests on g8e-pod"
    cd "$PROJECT_ROOT/components/g8ee"
    if [[ "$PYRIGHT" == "true" ]]; then
        python -m pyright --project pyrightconfig.services.json
    fi
    local cov_args=(-rs)
    [[ "$COVERAGE" == "true" ]] && cov_args+=("--cov" "--cov-report=term-missing")
    pytest "${cov_args[@]}" "${EXTRA_ARGS[@]}"
}

run_vsod() {
    log_header "Running VSOD tests on g8e-pod"
    cd "$PROJECT_ROOT/components/vsod"
    local cov_flag=""
    [[ "$COVERAGE" == "true" ]] && cov_flag="--coverage"
    NODE_PATH="./node_modules" npx vitest run --config ./vitest.config.js $cov_flag "${EXTRA_ARGS[@]}"
}

run_vsa() {
    log_header "Running VSA tests on g8e-pod"
    cd "$PROJECT_ROOT/components/vsa"
    local test_target="./..."
    local pass_through_args=()
    
    for arg in "${EXTRA_ARGS[@]}"; do
        if [[ "$arg" == ./* || "$arg" == */* ]]; then
            # Ensure path starts with ./ for Go if it doesn't already
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
            log_header "VSA Coverage Report"
            go tool cover -func=coverage.out
        fi
        return $rc
    else
        gotestsum --format dots-v2 -- -race -timeout 180s "$test_target" "${pass_through_args[@]}"
    fi
}

run_component() {
    case "$COMPONENT" in
        g8ee) run_g8ee ;;
        vsod) run_vsod ;;
        vsa) run_vsa ;;
        all) run_g8ee; run_vsod; run_vsa ;;
    esac
}

_show_llm_config() {
    if [[ -n "${TEST_LLM_PROVIDER:-}" ]]; then
        echo ""
        echo -e "${CYAN}  LLM Configuration (from CLI flags)${NC}"
        echo -e "  Provider:        ${TEST_LLM_PROVIDER}"
        [[ -n "${TEST_LLM_PRIMARY_MODEL:-}" ]]   && echo -e "  Primary Model:   ${TEST_LLM_PRIMARY_MODEL}"
        [[ -n "${TEST_LLM_ASSISTANT_MODEL:-}" ]] && echo -e "  Assistant Model: ${TEST_LLM_ASSISTANT_MODEL}"
        [[ -n "${TEST_LLM_ENDPOINT_URL:-}" ]]    && echo -e "  Endpoint:        ${TEST_LLM_ENDPOINT_URL}"
        [[ -n "${TEST_LLM_API_KEY:-}" ]]         && echo -e "  API Key:         (set)"
        echo ""
    else
        echo ""
        echo -e "  ${YELLOW}No LLM flags provided — ai_integration tests will be skipped.${NC}"
        echo -e "  ${YELLOW}Use -p/--llm-provider to enable real LLM tests.${NC}"
        echo ""
    fi
}

_show_web_search_config() {
    if [[ -n "${TEST_WEB_SEARCH_PROJECT_ID:-}" ]] && [[ -n "${TEST_WEB_SEARCH_ENGINE_ID:-}" ]] && [[ -n "${TEST_WEB_SEARCH_API_KEY:-}" ]]; then
        echo ""
        echo -e "${CYAN}  Web Search Configuration (from CLI flags)${NC}"
        echo -e "  Project ID:      ${TEST_WEB_SEARCH_PROJECT_ID}"
        echo -e "  Engine ID:       ${TEST_WEB_SEARCH_ENGINE_ID}"
        echo -e "  API Key:         (set)"
        [[ -n "${TEST_WEB_SEARCH_LOCATION:-}" ]] && echo -e "  Location:        ${TEST_WEB_SEARCH_LOCATION}"
        echo ""
    else
        echo ""
        echo -e "  ${YELLOW}No web search flags provided — requires_web_search tests will be skipped.${NC}"
        echo -e "  ${YELLOW}Use --web-search-* flags to enable real web search tests.${NC}"
        echo ""
    fi
}

# =============================================================================
# Host-side launch
# =============================================================================

run_in_container() {
    if ! docker ps --filter "name=^g8e-pod$" --filter "status=running" -q | grep -q .; then
        log_warn "g8e-pod not running — starting it..."
        docker compose up -d g8e-pod
    fi
    local exec_args=("$COMPONENT")
    [[ "$COVERAGE" == "true" ]] && exec_args+=("--coverage")
    [[ "$PYRIGHT" == "true" ]] && exec_args+=("--pyright")
    [[ ${#EXTRA_ARGS[@]} -gt 0 ]] && exec_args+=("--" "${EXTRA_ARGS[@]}")

    local env_args=(-e RUNNING_IN_CONTAINER=true)
    [[ -n "$TEST_LLM_PROVIDER" ]]        && env_args+=(-e "TEST_LLM_PROVIDER=$TEST_LLM_PROVIDER")
    [[ -n "$TEST_LLM_PRIMARY_MODEL" ]]    && env_args+=(-e "TEST_LLM_PRIMARY_MODEL=$TEST_LLM_PRIMARY_MODEL")
    [[ -n "$TEST_LLM_ASSISTANT_MODEL" ]]  && env_args+=(-e "TEST_LLM_ASSISTANT_MODEL=$TEST_LLM_ASSISTANT_MODEL")
    [[ -n "$TEST_LLM_ENDPOINT_URL" ]]     && env_args+=(-e "TEST_LLM_ENDPOINT_URL=$TEST_LLM_ENDPOINT_URL")
    [[ -n "$TEST_LLM_API_KEY" ]]          && env_args+=(-e "TEST_LLM_API_KEY=$TEST_LLM_API_KEY")

    [[ -n "$TEST_WEB_SEARCH_PROJECT_ID" ]] && env_args+=(-e "TEST_WEB_SEARCH_PROJECT_ID=$TEST_WEB_SEARCH_PROJECT_ID")
    [[ -n "$TEST_WEB_SEARCH_ENGINE_ID" ]]  && env_args+=(-e "TEST_WEB_SEARCH_ENGINE_ID=$TEST_WEB_SEARCH_ENGINE_ID")
    [[ -n "$TEST_WEB_SEARCH_API_KEY" ]]     && env_args+=(-e "TEST_WEB_SEARCH_API_KEY=$TEST_WEB_SEARCH_API_KEY")
    [[ -n "$TEST_WEB_SEARCH_LOCATION" ]]    && env_args+=(-e "TEST_WEB_SEARCH_LOCATION=$TEST_WEB_SEARCH_LOCATION")

    docker exec "${env_args[@]}" g8e-pod /app/scripts/testing/run_tests.sh "${exec_args[@]}"
}

# =============================================================================
# Main Entry Point
# =============================================================================

if in_container; then
    setup_container_environment
    verify_container_infrastructure

    [[ -n "$TEST_LLM_PROVIDER" ]]        && export TEST_LLM_PROVIDER
    [[ -n "$TEST_LLM_PRIMARY_MODEL" ]]   && export TEST_LLM_PRIMARY_MODEL
    [[ -n "$TEST_LLM_ASSISTANT_MODEL" ]] && export TEST_LLM_ASSISTANT_MODEL
    [[ -n "$TEST_LLM_ENDPOINT_URL" ]]    && export TEST_LLM_ENDPOINT_URL
    [[ -n "$TEST_LLM_API_KEY" ]]         && export TEST_LLM_API_KEY

    [[ -n "$TEST_WEB_SEARCH_PROJECT_ID" ]] && export TEST_WEB_SEARCH_PROJECT_ID
    [[ -n "$TEST_WEB_SEARCH_ENGINE_ID" ]]  && export TEST_WEB_SEARCH_ENGINE_ID
    [[ -n "$TEST_WEB_SEARCH_API_KEY" ]]     && export TEST_WEB_SEARCH_API_KEY
    [[ -n "$TEST_WEB_SEARCH_LOCATION" ]]    && export TEST_WEB_SEARCH_LOCATION

    log_header "run_tests.sh $*"
    
    _show_llm_config
    _show_web_search_config
    run_component
else
    # Host-side: ALWAYS launch in g8e-pod. 
    # Direct execution on host is strictly forbidden for tests.
    run_in_container
fi
