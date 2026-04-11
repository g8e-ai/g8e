#!/bin/bash
# g8e Web Search Setup
#
# Interactive Vertex AI Search configuration. Writes settings to the platform DB.
# Enables the search_web AI tool, which lets the AI search the web
# during investigations using a Vertex AI Search (Discovery Engine) app.
#
# Prerequisites (one-time GCP setup):
#   1. Enable the Discovery Engine API in your GCP project
#   2. Create a Website data store and index your domains
#   3. Create a Search App connected to that data store
#   4. Create an API key restricted to the Discovery Engine API
#   IDs for steps 2-3 are in the GCP console URL for the search app.
#
# Non-interactive usage (all flags):
#   --project-id   GCP project ID
#   --engine-id    Vertex AI Search engine/app ID
#   --api-key      GCP API key with Discovery Engine API access
#                  (falls back to GEMINI_API_KEY env var if not set)
#   --location     data store location (default: global)
#   --disable      remove Vertex Search config and disable the feature

set -euo pipefail

_footer() {
    local rc=$?
    [[ $rc -eq 0 ]] || return
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  setup-search.sh done"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}
trap _footer EXIT

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  setup-search.sh $*"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# =============================================================================
# CLI argument parsing
# =============================================================================

ARG_PROJECT_ID=""
ARG_ENGINE_ID=""
ARG_API_KEY=""
ARG_LOCATION=""
ARG_DISABLE=false
NON_INTERACTIVE=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --project-id)  ARG_PROJECT_ID="$2"; shift 2 ;;
        --engine-id)   ARG_ENGINE_ID="$2";  shift 2 ;;
        --api-key)     ARG_API_KEY="$2";    shift 2 ;;
        --location)    ARG_LOCATION="$2";   shift 2 ;;
        --disable)     ARG_DISABLE=true;    shift ;;
        --help|-h)
            echo "Usage: setup-search.sh [options]"
            echo ""
            echo "Options:"
            echo "  --project-id   GCP project ID"
            echo "  --engine-id    Vertex AI Search engine/app ID"
            echo "  --api-key      GCP API key with Discovery Engine API access"
            echo "                 (falls back to GEMINI_API_KEY env var if not set)"
            echo "  --location     data store location (default: global)"
            echo "  --disable      remove Vertex Search config and disable the feature"
            exit 0 ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1 ;;
    esac
done

if [[ -n "$ARG_PROJECT_ID" || -n "$ARG_ENGINE_ID" || -n "$ARG_API_KEY" || "$ARG_DISABLE" == true ]]; then
    NON_INTERACTIVE=true
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

RED=$'\033[0;31m'
GREEN=$'\033[0;32m'
BLUE=$'\033[0;34m'
YELLOW=$'\033[1;33m'
CYAN=$'\033[0;36m'
BOLD=$'\033[1m'
NC=$'\033[0m'

_header() { echo -e "\n${BLUE}${BOLD}$1${NC}"; }
_ok()     { echo -e "  ${GREEN}[OK]${NC} $1"; }
_warn()   { echo -e "  ${YELLOW}[WARN]${NC} $1"; }
_err()    { echo -e "  ${RED}[ERROR]${NC} $1" >&2; }
_info()   { echo -e "  ${CYAN}$1${NC}"; }

_prompt_secret() {
    local label="$1"
    local result
    printf "  %s: " "$label" >&2
    IFS= read -rs result
    echo >&2
    echo "$result"
}

OS="$(uname -s)"

_read_env() {
    local key="$1"
    echo "${!key:-}"
}

_write_to_db() {
    local db_args=("$@")
    if docker ps --filter "name=^g8ep$" --filter "status=running" --format "{{.Names}}" 2>/dev/null | grep -q "^g8ep$"; then
        if docker exec g8ep python3 /app/scripts/data/manage-g8es.py settings set "${db_args[@]}" 2>/dev/null; then
            _ok "Settings written to DB"
        else
            _warn "Could not write to DB — settings will apply after next platform restart"
        fi
    else
        _warn "Platform not running — DB write skipped (run ./g8e platform start)"
    fi
}

# =============================================================================
# --disable path
# =============================================================================

if [[ "$ARG_DISABLE" == true ]]; then
    _header "Disabling Vertex AI Search"
    echo
    _write_to_db "vertex_search_enabled=false"
    _ok "Vertex AI Search disabled"
    _info "Run: ./g8e platform restart   to apply"
    exit 0
fi

# =============================================================================
# Read existing values from environment (set by docker-compose or DB injection)
# =============================================================================

_current_project_id="$(_read_env VERTEX_SEARCH_PROJECT_ID)"
_current_engine_id="$(_read_env VERTEX_SEARCH_ENGINE_ID)"
_current_api_key="$(_read_env VERTEX_SEARCH_API_KEY)"
_current_location="$(_read_env VERTEX_SEARCH_LOCATION)"
_current_enabled="$(_read_env VERTEX_SEARCH_ENABLED)"

_is_configured=false
if [[ "$_current_enabled" == "true" && -n "$_current_project_id" && -n "$_current_engine_id" && -n "$_current_api_key" ]]; then
    _is_configured=true
fi

# =============================================================================
# Interactive header
# =============================================================================

_header "g8e Web Search Setup"
echo
_info "Enables the search_web tool — lets the AI search the web during investigations."
_info "Requires a Vertex AI Search app (Discovery Engine) in your GCP project."
echo

if [[ "$_is_configured" == true ]]; then
    _ok "Web search is currently enabled"
    _info "Project: $_current_project_id"
    _info "Engine:  $_current_engine_id"
    _info "Location: ${_current_location:-global}"
    echo
fi

# =============================================================================
# GCP setup instructions (interactive only)
# =============================================================================

if [[ "$NON_INTERACTIVE" == false && "$_is_configured" == false ]]; then
    _info "One-time GCP setup required before continuing:"
    echo
    echo "  ── Step 1: Create an API key ────────────────────────────────────────────"
    echo ""
    echo "    Go to (replace YOUR_PROJECT_ID with your GCP project):"
    echo "      https://console.cloud.google.com/apis/credentials"
    echo ""
    echo "    Click 'Create credentials' -> 'API key'."
    echo ""
    echo "    Recommendation: keep it simple — restrict the key to these two APIs only:"
    echo "      - Gemini for Google Cloud API  (used by the AI engine)"
    echo "      - Discovery Engine API          (used by web search)"
    echo ""
    echo "    This single key is used for both GEMINI_API_KEY and VERTEX_SEARCH_API_KEY."
    echo "    Note: it may take up to 5 minutes after saving for the key to take effect."
    echo ""
    echo "  ── Step 2: Enable the Discovery Engine API ──────────────────────────────"
    echo ""
    echo "    https://console.cloud.google.com/apis/library/discoveryengine.googleapis.com"
    echo ""
    echo "  ── Step 3: Create a search app ──────────────────────────────────────────"
    echo ""
    echo "    Go to (replace YOUR_PROJECT_ID with your GCP project):"
    echo "      https://console.cloud.google.com/gen-app-builder/engines"
    echo ""
    echo "    Click 'Create app' -> 'Custom search (general)' -> follow the prompts."
    echo "    When asked to create a data store, choose 'Website' and add your domains."
    echo "    Note the App ID shown after creation — you will need it below."
    echo ""
fi

# =============================================================================
# Collect values
# =============================================================================

if [[ "$NON_INTERACTIVE" == true ]]; then
    if [[ -z "$ARG_API_KEY" && -n "${GEMINI_API_KEY:-}" ]]; then
        ARG_API_KEY="$GEMINI_API_KEY"
    fi
    if [[ -z "$ARG_PROJECT_ID" || -z "$ARG_ENGINE_ID" || -z "$ARG_API_KEY" ]]; then
        _err "--project-id, --engine-id, and --api-key (or GEMINI_API_KEY) are all required"
        exit 1
    fi
    VS_PROJECT_ID="$ARG_PROJECT_ID"
    VS_ENGINE_ID="$ARG_ENGINE_ID"
    VS_API_KEY="$ARG_API_KEY"
    VS_LOCATION="${ARG_LOCATION:-global}"
else
    _header "GCP Project ID"
    _info "Found in the GCP console URL: console.cloud.google.com/... — the project selector at the top."
    echo
    printf "  Project ID [%s]: " "${_current_project_id:-your-gcp-project-id}" >&2
    IFS= read -r _input
    VS_PROJECT_ID="${_input:-$_current_project_id}"
    if [[ -z "$VS_PROJECT_ID" ]]; then
        _err "Project ID is required."
        exit 1
    fi

    _header "Vertex AI Search Engine ID"
    _info "The App ID shown in the Search App details page."
    _info "https://console.cloud.google.com/gen-app-builder/engines?project=${VS_PROJECT_ID}"
    echo
    printf "  Engine ID [%s]: " "${_current_engine_id:-your-engine-id}" >&2
    IFS= read -r _input
    VS_ENGINE_ID="${_input:-$_current_engine_id}"
    if [[ -z "$VS_ENGINE_ID" ]]; then
        _err "Engine ID is required."
        exit 1
    fi

    _header "Data Store Location"
    _info "Must match the location you selected when creating the data store."
    _info "Most data stores use 'global'. Only change this if you chose a region."
    echo
    printf "  Location [%s]: " "${_current_location:-global}" >&2
    IFS= read -r _input
    VS_LOCATION="${_input:-${_current_location:-global}}"

    _header "GCP API Key"
    _info "A single API key scoped to both Discovery Engine API and Gemini for Google Cloud API."
    _info "This is the same key used for your Gemini LLM provider (GEMINI_API_KEY)."
    _info "https://console.cloud.google.com/apis/credentials?project=${VS_PROJECT_ID}"
    echo
    _gemini_key_fallback="$(_read_env GEMINI_API_KEY)"
    _api_key_default="${_current_api_key:-$_gemini_key_fallback}"
    if [[ -n "$_api_key_default" ]]; then
        if [[ -n "$_current_api_key" ]]; then
            _info "An existing API key is set. Leave blank to keep it."
        else
            _info "GEMINI_API_KEY is set — leave blank to use the same key."
        fi
        VS_API_KEY="$(_prompt_secret "API key")"
        [[ -z "$VS_API_KEY" ]] && VS_API_KEY="$_api_key_default"
    else
        VS_API_KEY="$(_prompt_secret "API key")"
    fi
    if [[ -z "$VS_API_KEY" ]]; then
        _err "API key is required."
        exit 1
    fi
fi

# =============================================================================
# Validate using the exact same code path as the running platform
# =============================================================================

_header "Validating API key"
echo
_info "Testing connectivity to Vertex AI Search via g8ep..."
echo

_CONTAINER="g8ep"
_validate_output=""
_validate_exit=0

if docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${_CONTAINER}$"; then
    _validate_output="$(docker exec -i "$_CONTAINER" \
        /opt/venv/bin/python3 -c "
import asyncio, sys
sys.path.insert(0, '/app/components/g8ee')
from app.services.ai.grounding.web_search_provider import WebSearchProvider
async def run():
    p = WebSearchProvider(
        project_id='${VS_PROJECT_ID}',
        engine_id='${VS_ENGINE_ID}',
        api_key='${VS_API_KEY}',
        location='${VS_LOCATION}',
    )
    r = await p.search(query='test', num=1)
    if r.success:
        print('OK')
    else:
        print('FAIL:' + (r.error or 'unknown error'))
asyncio.run(run())
" 2>&1)" || _validate_exit=$?

    if echo "$_validate_output" | grep -q "^OK"; then
        _ok "API key validated successfully"
    elif echo "$_validate_output" | grep -q "^FAIL:"; then
        _err_msg="$(echo "$_validate_output" | grep "^FAIL:" | sed 's/^FAIL://')"
        _warn "Search returned an error: ${_err_msg}"
        _info "Saving config anyway — check your project ID and engine ID."
    else
        _warn "Unexpected validation output. Saving config anyway."
        _info "$_validate_output"
    fi
else
    _warn "g8ep container is not running — skipping live validation."
    _info "Run './g8e platform start' then './g8e search setup' to validate."
fi

# =============================================================================
# Write to DB (if platform is running)
# =============================================================================

_header "Writing configuration"
echo

_write_to_db \
    "vertex_search_enabled=true" \
    "vertex_search_project_id=$VS_PROJECT_ID" \
    "vertex_search_engine_id=$VS_ENGINE_ID" \
    "vertex_search_location=$VS_LOCATION" \
    "vertex_search_api_key=$VS_API_KEY"

_ok "VERTEX_SEARCH_ENABLED    = true"
_ok "VERTEX_SEARCH_PROJECT_ID = $VS_PROJECT_ID"
_ok "VERTEX_SEARCH_ENGINE_ID  = $VS_ENGINE_ID"
_ok "VERTEX_SEARCH_LOCATION   = $VS_LOCATION"
_ok "VERTEX_SEARCH_API_KEY    = (set)"

# =============================================================================
# Summary
# =============================================================================

_header "Web Search Setup Complete"
echo
_info "Settings written to DB"
echo
_info "If you use Gemini as your LLM provider, set GEMINI_API_KEY to the same key:"
echo "    ./g8e llm setup   — re-run LLM setup to enter the key for Gemini"
echo
echo "    ./g8e platform restart   — apply without full rebuild"
echo "    ./g8e search disable     — remove web search configuration"
echo
