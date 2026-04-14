#!/bin/bash
# g8e LLM management.
#
# Non-interactive setup usage:
#   --provider   gemini | anthropic | openai | ollama | vllm
#   --model      primary model name
#   --asst-model assistant model name (optional)
#   --endpoint   API base URL (openai / vllm / ollama)
#   --api-key    API key (provider-specific; for Gemini use --gemini-key)
#   --gemini-key GEMINI_API_KEY (Gemini only)
#   --anthropic-key ANTHROPIC_API_KEY (Anthropic only)

set -euo pipefail

_footer() {
    local rc=$?
    [[ $rc -eq 0 ]] || return
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  setup-llm.sh done"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}
trap _footer EXIT

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  setup-llm.sh $*"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# =============================================================================
# CLI argument parsing
# =============================================================================

usage() {
    echo "Usage: setup-llm.sh <command> [options]"
    echo ""
    echo "Commands:"
    echo "  setup            Interactive or non-interactive provider setup"
    echo "  show             Show current LLM settings"
    echo "  get <key>        Read a single LLM setting"
    echo "  set              Write one or more LLM settings"
    echo "  restart          Restart LLM-related services"
    echo ""
    echo "Options for 'setup' and 'set':"
    echo "  --user-id            Target user ID (required for UserSettingsDocument)"
    echo "  --provider           LLM provider (gemini, anthropic, openai, ollama, vllm)"
    echo "  --primary-llm        Primary model name (llm_model)"
    echo "  --assistant-llm      Assistant model name (llm_assistant_model)"
    echo "  --endpoint           API base URL (llm_endpoint)"
    echo "  --openai-api-key     OpenAI API key (llm_api_key)"
    echo "  --gemini-api-key     Gemini API key (gemini_api_key)"
    echo "  --anthropic-api-key  Anthropic API key (anthropic_api_key)"
    echo "  --temperature        LLM temperature (llm_temperature)"
    echo "  --max-tokens         Max tokens (llm_max_tokens)"
    echo ""
    echo "Examples:"
    echo "  ./setup-llm.sh setup --provider gemini --gemini-api-key AIza..."
    echo "  ./setup-llm.sh set --primary-llm gpt-4o --temperature 0.5"
    echo "  ./setup-llm.sh show"
    exit 0
}

COMMAND=""
[[ $# -gt 0 ]] && COMMAND="$1" && shift

ARG_USER_ID=""
ARG_PROVIDER=""
ARG_MODEL=""
ARG_ASST_MODEL=""
ARG_ENDPOINT=""
ARG_API_KEY=""
ARG_GEMINI_KEY=""
ARG_ANTHROPIC_KEY=""
ARG_TEMPERATURE=""
ARG_MAX_TOKENS=""
NON_INTERACTIVE=false

# Helper for 'set' and 'get' subcommands
EXT_ARGS=()

case "$COMMAND" in
    setup|set|show|get)
        while [[ $# -gt 0 ]]; do
            case "$1" in
                --user-id=*)           ARG_USER_ID="${1#*=}";           shift 1 ;;
                --user-id)             ARG_USER_ID="$2";                shift 2 ;;
                --provider=*)          ARG_PROVIDER="${1#*=}";          shift 1 ;;
                --provider)            ARG_PROVIDER="$2";               shift 2 ;;
                --primary-llm=*)       ARG_MODEL="${1#*=}";             shift 1 ;;
                --primary-llm)         ARG_MODEL="$2";                  shift 2 ;;
                --model=*)             ARG_MODEL="${1#*=}";             shift 1 ;;
                --model)               ARG_MODEL="$2";                  shift 2 ;;
                --assistant-llm=*)     ARG_ASST_MODEL="${1#*=}";        shift 1 ;;
                --assistant-llm)       ARG_ASST_MODEL="$2";             shift 2 ;;
                --asst-model=*)        ARG_ASST_MODEL="${1#*=}";        shift 1 ;;
                --asst-model)          ARG_ASST_MODEL="$2";             shift 2 ;;
                --endpoint=*)          ARG_ENDPOINT="${1#*=}";          shift 1 ;;
                --endpoint)            ARG_ENDPOINT="$2";               shift 2 ;;
                --openai-api-key=*)    ARG_API_KEY="${1#*=}";           shift 1 ;;
                --openai-api-key)      ARG_API_KEY="$2";                shift 2 ;;
                --gemini-api-key=*)    ARG_GEMINI_KEY="${1#*=}";        shift 1 ;;
                --gemini-api-key)      ARG_GEMINI_KEY="$2";             shift 2 ;;
                --anthropic-api-key=*) ARG_ANTHROPIC_KEY="${1#*=}";      shift 1 ;;
                --anthropic-api-key)   ARG_ANTHROPIC_KEY="$2";          shift 2 ;;
                --temperature=*)       ARG_TEMPERATURE="${1#*=}";       shift 1 ;;
                --temperature)         ARG_TEMPERATURE="$2";            shift 2 ;;
                --max-tokens=*)        ARG_MAX_TOKENS="${1#*=}";        shift 1 ;;
                --max-tokens)          ARG_MAX_TOKENS="$2";             shift 2 ;;
                -h|--help)             usage ;;
                *)                     echo "Unknown option: $1" >&2; exit 1 ;;
            esac
        done
        [[ "$COMMAND" == "setup" && -n "$ARG_PROVIDER" ]] && NON_INTERACTIVE=true
        ;;
    show|restart)
        [[ $# -gt 0 ]] && { echo "Unknown arguments for $COMMAND: $*" >&2; exit 1; }
        ;;
    get|set)
        [[ $# -eq 0 ]] && { echo "Missing arguments for $COMMAND" >&2; exit 1; }
        EXT_ARGS=("$@")
        ;;
    -h|--help|"")
        usage
        ;;
    *)
        echo "Unknown command: $COMMAND" >&2
        usage
        ;;
esac

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

_exec_in_pod() {
    if docker ps --filter "name=^g8ep$" --filter "status=running" --format "{{.Names}}" 2>/dev/null | grep -q "^g8ep$"; then
        docker exec -i g8ep "$@"
    else
        _err "Platform not running — g8ep is required (run ./g8e platform start)"
        exit 1
    fi
}

# =============================================================================
# Subcommand: show, get, set, restart
# =============================================================================

case "$COMMAND" in
    show)
        _header "Current LLM Settings"
        [[ -n "$ARG_USER_ID" ]] && USER_ID_ARG="--user-id=$ARG_USER_ID" || USER_ID_ARG=""
        _exec_in_pod python3 /app/scripts/data/manage-g8es.py settings show --section llm $USER_ID_ARG
        exit 0
        ;;
    get)
        [[ -n "$ARG_USER_ID" ]] && USER_ID_ARG="--user-id=$ARG_USER_ID" || USER_ID_ARG=""
        _exec_in_pod python3 /app/scripts/data/manage-g8es.py settings get "${EXT_ARGS[@]}" $USER_ID_ARG
        exit 0
        ;;
    set)
        _header "Updating LLM Settings"
        SET_ARGS=()
        [[ -n "$ARG_PROVIDER" ]]      && SET_ARGS+=("llm_provider=$ARG_PROVIDER")
        [[ -n "$ARG_MODEL" ]]         && SET_ARGS+=("llm_model=$ARG_MODEL")
        [[ -n "$ARG_ASST_MODEL" ]]    && SET_ARGS+=("llm_assistant_model=$ARG_ASST_MODEL")
        [[ -n "$ARG_ENDPOINT" ]]      && SET_ARGS+=("llm_endpoint=$ARG_ENDPOINT")
        [[ -n "$ARG_API_KEY" ]]       && SET_ARGS+=("llm_api_key=$ARG_API_KEY")
        [[ -n "$ARG_GEMINI_KEY" ]]    && SET_ARGS+=("gemini_api_key=$ARG_GEMINI_KEY")
        [[ -n "$ARG_ANTHROPIC_KEY" ]] && SET_ARGS+=("anthropic_api_key=$ARG_ANTHROPIC_KEY")
        [[ -n "$ARG_TEMPERATURE" ]]   && SET_ARGS+=("llm_temperature=$ARG_TEMPERATURE")
        [[ -n "$ARG_MAX_TOKENS" ]]    && SET_ARGS+=("llm_max_tokens=$ARG_MAX_TOKENS")

        if [[ ${#SET_ARGS[@]} -eq 0 ]]; then
            _err "No settings provided. Use --help to see available options."
            exit 1
        fi

        [[ -n "$ARG_USER_ID" ]] && USER_ID_ARG="--user-id=$ARG_USER_ID" || USER_ID_ARG=""
        _exec_in_pod python3 /app/scripts/data/manage-g8es.py settings set "${SET_ARGS[@]}" $USER_ID_ARG
        
        echo ""
        _header "Effective LLM Settings"
        _exec_in_pod python3 /app/scripts/data/manage-g8es.py settings show --section llm $USER_ID_ARG
        exit 0
        ;;
    restart)
        _header "Restarting LLM Services"
        # Since we are on the host, we can call docker compose if available, 
        # but the standard way in this repo is via build.sh
        bash "$REPO_ROOT/scripts/core/build.sh" restart g8ed g8ee
        exit 0
        ;;
esac

# =============================================================================
# Subcommand: setup (Interactive or Non-interactive)
# =============================================================================

# =============================================================================
# Read existing values (for interactive setup defaults)
# =============================================================================

[[ -n "$ARG_USER_ID" ]] && USER_ID_ARG="--user-id=$ARG_USER_ID" || USER_ID_ARG=""

# Export all settings as JSON for robust extraction
_SETTINGS_JSON="$(_exec_in_pod python3 /app/scripts/data/manage-g8es.py settings export --section llm $USER_ID_ARG 2>/dev/null || echo "{}")"

_get_setting() {
    echo "$_SETTINGS_JSON" | jq -r ".$1 // \"\""
}

_cur_provider="$(_get_setting llm_provider)"
_cur_model="$(_get_setting llm_model)"
_cur_asst_model="$(_get_setting llm_assistant_model)"
_cur_endpoint="$(_get_setting llm_endpoint)"
_cur_api_key=""
_cur_gemini_key=""
_cur_anthropic_key=""

# =============================================================================
# Interactive header
# =============================================================================

_header "g8e LLM Setup"
echo
_info "Configures the AI provider for g8ee (command generation, investigations, assistant)."
echo

if [[ -n "$_cur_provider" ]]; then
    _ok "LLM provider currently set: $_cur_provider"
    [[ -n "$_cur_model" ]]    && _info "Model:    $_cur_model"
    [[ -n "$_cur_endpoint" ]] && _info "Endpoint: $_cur_endpoint"
    echo
fi

# =============================================================================
# Provider selection
# =============================================================================

if [[ "$NON_INTERACTIVE" == true ]]; then
    LLM_PROVIDER="$ARG_PROVIDER"
    case "$LLM_PROVIDER" in
        gemini|anthropic|openai|ollama) ;;
        *)
            _err "Unknown provider: $LLM_PROVIDER. Supported: gemini, anthropic, openai, ollama"
            exit 1 ;;
    esac
else
    _header "Select LLM Provider"
    echo
    echo "  1) Gemini       (Google — recommended, most tested)"
    echo "  2) Anthropic    (Claude)"
    echo "  3) OpenAI       (GPT)"
    echo "  4) Ollama       (remote Ollama server)"
    echo

    _default_choice=""
    case "$_cur_provider" in
        gemini)           _default_choice="1" ;;
        anthropic)        _default_choice="2" ;;
        openai)           _default_choice="3" ;;
        ollama)           _default_choice="4" ;;
    esac

    printf "  Provider [%s]: " "${_default_choice:-1}" >&2
    IFS= read -r _choice
    _choice="${_choice:-${_default_choice:-1}}"

    case "$_choice" in
        1) LLM_PROVIDER="gemini" ;;
        2) LLM_PROVIDER="anthropic" ;;
        3) LLM_PROVIDER="openai" ;;
        4) LLM_PROVIDER="ollama" ;;
        *)
            _err "Invalid choice: $_choice"
            exit 1 ;;
    esac
fi

# =============================================================================
# Provider-specific configuration
# =============================================================================

LLM_API_KEY=""
LLM_ENDPOINT=""
LLM_MODEL=""
LLM_ASST_MODEL=""
GEMINI_KEY=""
ANTHROPIC_KEY=""

case "$LLM_PROVIDER" in

    # ── Gemini ────────────────────────────────────────────────────────────────
    gemini)
        _header "Gemini Configuration"
        echo
        _info "Requires a GCP API key restricted to the Gemini for Google Cloud API."
        _info "https://console.cloud.google.com/apis/credentials"
        echo

        if [[ "$NON_INTERACTIVE" == true ]]; then
            GEMINI_KEY="${ARG_GEMINI_KEY:-${ARG_API_KEY:-}}"
            if [[ -z "$GEMINI_KEY" ]]; then
                _err "--gemini-key or --api-key is required for Gemini"
                exit 1
            fi
            LLM_MODEL="${ARG_MODEL:-gemini-3.1-pro-preview}"
            LLM_ASST_MODEL="${ARG_ASST_MODEL:-gemini-3-flash-preview}"
        else
            if [[ -n "$_cur_gemini_key" ]]; then
                _info "An existing API key is set. Leave blank to keep it."
            fi
            GEMINI_KEY="$(_prompt_secret "Gemini API key")"
            [[ -z "$GEMINI_KEY" ]] && GEMINI_KEY="$_cur_gemini_key"
            if [[ -z "$GEMINI_KEY" ]]; then
                _err "Gemini API key is required."
                exit 1
            fi

            printf "  Primary model [%s]: " "${_cur_model:-gemini-3.1-pro-preview}" >&2
            IFS= read -r _input
            LLM_MODEL="${_input:-${_cur_model:-gemini-3.1-pro-preview}}"

            printf "  Assistant model [%s]: " "${_cur_asst_model:-gemini-3-flash-preview}" >&2
            IFS= read -r _input
            LLM_ASST_MODEL="${_input:-${_cur_asst_model:-gemini-3-flash-preview}}"
        fi

        LLM_PROVIDER_VAL="gemini"
        ;;

    # ── Anthropic ─────────────────────────────────────────────────────────────
    anthropic)
        _header "Anthropic Configuration"
        echo
        _info "Requires an Anthropic API key."
        _info "https://console.anthropic.com/settings/keys"
        echo

        if [[ "$NON_INTERACTIVE" == true ]]; then
            ANTHROPIC_KEY="${ARG_ANTHROPIC_KEY:-${ARG_API_KEY:-}}"
            if [[ -z "$ANTHROPIC_KEY" ]]; then
                _err "--anthropic-key or --api-key is required for Anthropic"
                exit 1
            fi
            LLM_MODEL="${ARG_MODEL:-claude-opus-4-5}"
            LLM_ASST_MODEL="${ARG_ASST_MODEL:-claude-haiku-4-5}"
        else
            if [[ -n "$_cur_anthropic_key" ]]; then
                _info "An existing API key is set. Leave blank to keep it."
            fi
            ANTHROPIC_KEY="$(_prompt_secret "Anthropic API key")"
            [[ -z "$ANTHROPIC_KEY" ]] && ANTHROPIC_KEY="$_cur_anthropic_key"
            if [[ -z "$ANTHROPIC_KEY" ]]; then
                _err "Anthropic API key is required."
                exit 1
            fi

            printf "  Primary model [%s]: " "${_cur_model:-claude-opus-4-5}" >&2
            IFS= read -r _input
            LLM_MODEL="${_input:-${_cur_model:-claude-opus-4-5}}"

            printf "  Assistant model [%s]: " "${_cur_asst_model:-claude-haiku-4-5}" >&2
            IFS= read -r _input
            LLM_ASST_MODEL="${_input:-${_cur_asst_model:-claude-haiku-4-5}}"
        fi

        LLM_PROVIDER_VAL="anthropic"
        ;;

    # ── OpenAI ────────────────────────────────────────────────────────────────
    openai)
        _header "OpenAI Configuration"
        echo
        _info "Requires an OpenAI API key."
        _info "https://platform.openai.com/api_keys"
        echo

        if [[ "$NON_INTERACTIVE" == true ]]; then
            LLM_API_KEY="${ARG_API_KEY:-}"
            if [[ -z "$LLM_API_KEY" ]]; then
                _err "--api-key is required for OpenAI"
                exit 1
            fi
            LLM_MODEL="${ARG_MODEL:-gpt-4o}"
            LLM_ASST_MODEL="${ARG_ASST_MODEL:-gpt-4o-mini}"
            LLM_ENDPOINT="${ARG_ENDPOINT:-https://api.openai.com/v1}"
        else
            if [[ -n "$_cur_api_key" ]]; then
                _info "An existing API key is set. Leave blank to keep it."
            fi
            LLM_API_KEY="$(_prompt_secret "OpenAI API key")"
            [[ -z "$LLM_API_KEY" ]] && LLM_API_KEY="$_cur_api_key"
            if [[ -z "$LLM_API_KEY" ]]; then
                _err "OpenAI API key is required."
                exit 1
            fi

            printf "  Primary model [%s]: " "${_cur_model:-gpt-4o}" >&2
            IFS= read -r _input
            LLM_MODEL="${_input:-${_cur_model:-gpt-4o}}"

            printf "  Assistant model [%s]: " "${_cur_asst_model:-gpt-4o-mini}" >&2
            IFS= read -r _input
            LLM_ASST_MODEL="${_input:-${_cur_asst_model:-gpt-4o-mini}}"

            LLM_ENDPOINT="https://api.openai.com/v1"
        fi

        LLM_PROVIDER_VAL="openai"
        ;;

    # ── Ollama (remote) ────────────────────────────────────────────────────────
    ollama)
        _header "Ollama Configuration"
        echo
        _info "Connects to an existing remote Ollama server."
        echo

        if [[ "$NON_INTERACTIVE" == true ]]; then
            LLM_ENDPOINT="${ARG_ENDPOINT:-}"
            if [[ -z "$LLM_ENDPOINT" ]]; then
                _err "--endpoint is required for Ollama"
                exit 1
            fi
            LLM_MODEL="${ARG_MODEL:-gemma4:e4b}"
            LLM_ASST_MODEL="${ARG_ASST_MODEL:-gemma4:e4b}"
        else
            printf "  Ollama endpoint [%s]: " "${_cur_endpoint:-https://your-ollama-host:11434/v1}" >&2
            IFS= read -r _input
            LLM_ENDPOINT="${_input:-$_cur_endpoint}"
            if [[ -z "$LLM_ENDPOINT" ]]; then
                _err "Endpoint is required."
                exit 1
            fi

            printf "  Primary model [%s]: " "${_cur_model:-gemma4:e4b}" >&2
            IFS= read -r _input
            LLM_MODEL="${_input:-${_cur_model:-gemma4:e4b}}"

            printf "  Assistant model [%s]: " "${_cur_asst_model:-gemma4:e4b}" >&2
            IFS= read -r _input
            LLM_ASST_MODEL="${_input:-${_cur_asst_model:-gemma4:e4b}}"
        fi

        LLM_API_KEY="ollama"
        LLM_PROVIDER_VAL="ollama"
        ;;

esac

# =============================================================================
# Write to DB (if platform is running)
# =============================================================================

_header "Writing configuration"
echo
_ok "LLM_PROVIDER        = $LLM_PROVIDER_VAL"
_ok "LLM_MODEL           = $LLM_MODEL"
_ok "LLM_ASSISTANT_MODEL = $LLM_ASST_MODEL"
[[ -n "$LLM_ENDPOINT" ]]   && _ok "LLM_ENDPOINT        = $LLM_ENDPOINT"
[[ -n "$LLM_API_KEY" ]]    && _ok "LLM_API_KEY         = (set)"
[[ -n "$GEMINI_KEY" ]]     && _ok "GEMINI_API_KEY      = (set)"
[[ -n "$ANTHROPIC_KEY" ]]  && _ok "ANTHROPIC_API_KEY   = (set)"
echo

_build_db_args() {
    DB_ARGS=()
    DB_ARGS+=("llm_provider=$LLM_PROVIDER_VAL")
    DB_ARGS+=("llm_model=$LLM_MODEL")
    DB_ARGS+=("llm_assistant_model=$LLM_ASST_MODEL")
    [[ -n "$LLM_ENDPOINT" ]]   && DB_ARGS+=("llm_endpoint=$LLM_ENDPOINT")
    [[ -n "$LLM_API_KEY" ]]    && DB_ARGS+=("llm_api_key=$LLM_API_KEY")
    [[ -n "$GEMINI_KEY" ]]     && DB_ARGS+=("gemini_api_key=$GEMINI_KEY")
    [[ -n "$ANTHROPIC_KEY" ]]  && DB_ARGS+=("anthropic_api_key=$ANTHROPIC_KEY")

    if [[ -n "$ARG_USER_ID" ]]; then
        DB_ARGS+=("--user-id=$ARG_USER_ID")
    fi
}

_write_to_db() {
    _build_db_args

    if ! docker ps --filter "name=^g8ep$" --filter "status=running" --format "{{.Names}}" 2>/dev/null | grep -q "^g8ep$"; then
        _warn "Platform not running — DB write skipped (run ./g8e platform start)"
        return 1
    fi

    if docker exec g8ep python3 /app/scripts/data/manage-g8es.py settings set "${DB_ARGS[@]}" 2>/dev/null; then
        _ok "LLM settings written to DB (via g8ed)"
        return 0
    fi

    _info "g8ed unavailable — writing directly to g8es"
    if docker exec g8ep python3 /app/scripts/data/manage-g8es.py settings set --direct "${DB_ARGS[@]}" 2>/dev/null; then
        _ok "LLM settings written to DB (direct)"
        return 0
    fi

    _warn "Could not write to DB — settings will apply after next platform restart"
    return 1
}

_write_to_db
_write_rc=$?

if [[ $_write_rc -eq 0 ]]; then
    echo ""
    _header "Effective LLM Settings"
    [[ -n "$ARG_USER_ID" ]] && USER_ID_ARG="--user-id=$ARG_USER_ID" || USER_ID_ARG=""
    _exec_in_pod python3 /app/scripts/data/manage-g8es.py settings show --section llm $USER_ID_ARG 2>/dev/null || true
fi

# =============================================================================
# Summary
# =============================================================================

_header "LLM Setup Complete"
echo
_info "Settings written to DB"
echo
echo "  Apply without a full rebuild:"
echo "    ./g8e platform restart"
echo
echo "  Re-run at any time to change providers:"
echo "    ./g8e llm setup"
echo
