#!/usr/bin/env bash
set -e
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

SUB="${1:-send}"
shift || true

_chat_help() {
    cat <<'EOF'
Usage: ./g8e chat [send|tail] [options]

Subcommands:
  send <message>      Send a chat message to the g8ee Engine and stream events
  tail                Stream new SSE events for the current session (no message)

Options for `send`:
  --new                       Start a new case + investigation (default if no IDs given)
  --case <case_id>            Continue an existing case
  --investigation <id>        Continue an existing investigation
  --bound '[{...}]'           JSON array of bound operators (default: [])
  --timeout <seconds>         Stop polling after N idle seconds (default: 60)

LLM Overrides for `send`:
  --primary-provider <p>      LLM provider for primary tier (openai, anthropic, etc)
  --primary-model <m>         Model name for primary tier
  --assistant-provider <p>    LLM provider for assistant tier
  --assistant-model <m>       Model name for assistant tier
  --lite-provider <p>         LLM provider for lite tier
  --lite-model <m>            Model name for lite tier
  --primary-api-key <k>       API key for primary tier
  --primary-endpoint <e>      Endpoint URL for primary tier
  --assistant-api-key <k>     API key for assistant tier
  --assistant-endpoint <e>    Endpoint URL for assistant tier
  --lite-api-key <k>          API key for lite tier
  --lite-endpoint <e>         Endpoint URL for lite tier

Configuration precedence: CLI flags (highest priority) override provider-specific environment variables (G8E_OPENAI_*, G8E_ANTHROPIC_*, etc.), which override primary tier environment variables (G8E_PRIMARY_*, G8E_ASSISTANT_*, G8E_LITE_*), which override default configuration.

Options for `tail`:
  --since <id>                Resume from this SSE event id (default: 0)
  --timeout <seconds>         Stop polling after N idle seconds (default: 0 = forever)

Authentication: requires `./g8e login` (uses mTLS + OPERATOR_SESSION_ID).
SSE event routing uses CLI_SESSION_ID, a strictly disjoint session id minted at
login and persisted in ~/.g8e/credentials.
The g8ee Engine must be running: `./g8e apps start g8ee`.
EOF
}

case "$SUB" in
    -h|--help|help) _chat_help; exit 0 ;;
esac

_ensure_authenticated

if [[ -z "${G8E_CLI_SESSION_ID:-}" ]]; then
    echo "[g8e] CLI_SESSION_ID is missing from credentials - re-run: ./g8e login" >&2
    echo "     (cli_session_id and operator_session_id are strictly disjoint session types;" >&2
    echo "      stale credentials from before the split must be re-issued)" >&2
    exit 1
fi

if ! _g8ee_running; then
    echo "[g8e] g8ee Engine is not running - start it: ./g8e apps start g8ee" >&2
    exit 1
fi

# --- Argument parsing -------------------------------------------------------
MESSAGE=""
NEW_CASE=false
CASE_ID=""
INVESTIGATION_ID=""
BOUND_OPERATORS="[]"
TIMEOUT_SECS=60
SINCE_ID=0

# LLM Overrides
LLM_PRIMARY_PROVIDER=""
LLM_PRIMARY_MODEL=""
LLM_ASSISTANT_PROVIDER=""
LLM_ASSISTANT_MODEL=""
LLM_LITE_PROVIDER=""
LLM_LITE_MODEL=""
LLM_PRIMARY_API_KEY=""
LLM_PRIMARY_ENDPOINT=""
LLM_ASSISTANT_API_KEY=""
LLM_ASSISTANT_ENDPOINT=""
LLM_LITE_API_KEY=""
LLM_LITE_ENDPOINT=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --new)            NEW_CASE=true; shift ;;
        --case)           CASE_ID="$2"; shift 2 ;;
        --investigation)  INVESTIGATION_ID="$2"; shift 2 ;;
        --bound)          BOUND_OPERATORS="$2"; shift 2 ;;
        --timeout)        TIMEOUT_SECS="$2"; shift 2 ;;
        --since)          SINCE_ID="$2"; shift 2 ;;
        --primary-provider)   LLM_PRIMARY_PROVIDER="$2"; shift 2 ;;
        --primary-model)      LLM_PRIMARY_MODEL="$2"; shift 2 ;;
        --assistant-provider) LLM_ASSISTANT_PROVIDER="$2"; shift 2 ;;
        --assistant-model)    LLM_ASSISTANT_MODEL="$2"; shift 2 ;;
        --lite-provider)      LLM_LITE_PROVIDER="$2"; shift 2 ;;
        --lite-model)          LLM_LITE_MODEL="$2"; shift 2 ;;
        --primary-api-key)    LLM_PRIMARY_API_KEY="$2"; shift 2 ;;
        --primary-endpoint)   LLM_PRIMARY_ENDPOINT="$2"; shift 2 ;;
        --assistant-api-key)  LLM_ASSISTANT_API_KEY="$2"; shift 2 ;;
        --assistant-endpoint) LLM_ASSISTANT_ENDPOINT="$2"; shift 2 ;;
        --lite-api-key)       LLM_LITE_API_KEY="$2"; shift 2 ;;
        --lite-endpoint)      LLM_LITE_ENDPOINT="$2"; shift 2 ;;
        -h|--help)        _chat_help; exit 0 ;;
        --) shift; MESSAGE+="$* "; break ;;
        -*) echo "[chat] unknown flag: $1" >&2; exit 2 ;;
        *)  MESSAGE+="$1 "; shift ;;
    esac
done
MESSAGE="${MESSAGE%% }"

# Resolve effective LLM config from environment variables when CLI flags are
# not supplied. Precedence (high to low): CLI flag > G8E_<TIER>_<FIELD> >
# provider-specific G8E_<PROVIDER>_API_KEY / G8E_<PROVIDER>_ENDPOINT.
#
# The Engine's per-request override surface is tier-shaped (primary/assistant/
# lite), but operators typically configure credentials provider-shaped
# (G8E_GEMINI_API_KEY, G8E_OLLAMA_ENDPOINT). This block bridges the two by
# selecting credentials based on the effective tier provider.
_resolve_provider_credential() {
    # $1 = uppercase provider name (e.g. GEMINI), $2 = field (API_KEY|ENDPOINT)
    local provider="$1" field="$2"
    [[ -z "$provider" ]] && return 0
    local varname="G8E_${provider}_${field}"
    printf '%s' "${!varname:-}"
}

_resolve_tier() {
    # $1 = tier name (PRIMARY|ASSISTANT|LITE)
    local tier="$1"
    local flag_provider_var="LLM_${tier}_PROVIDER"
    local flag_model_var="LLM_${tier}_MODEL"
    local flag_key_var="LLM_${tier}_API_KEY"
    local flag_endpoint_var="LLM_${tier}_ENDPOINT"

    local env_provider_var="G8E_${tier}_PROVIDER"
    local env_model_var="G8E_${tier}_MODEL"
    local env_key_var="G8E_${tier}_API_KEY"
    local env_endpoint_var="G8E_${tier}_ENDPOINT"

    # Provider: CLI flag > G8E_<TIER>_PROVIDER
    local provider="${!flag_provider_var:-}"
    [[ -z "$provider" ]] && provider="${!env_provider_var:-}"
    printf -v "$flag_provider_var" '%s' "$provider"

    # Model: CLI flag > G8E_<TIER>_MODEL
    if [[ -z "${!flag_model_var:-}" ]]; then
        printf -v "$flag_model_var" '%s' "${!env_model_var:-}"
    fi

    # API key: CLI flag > G8E_<TIER>_API_KEY > G8E_<PROVIDER>_API_KEY
    if [[ -z "${!flag_key_var:-}" ]]; then
        local tier_key="${!env_key_var:-}"
        if [[ -n "$tier_key" ]]; then
            printf -v "$flag_key_var" '%s' "$tier_key"
        elif [[ -n "$provider" ]]; then
            local prov_upper="${provider^^}"
            printf -v "$flag_key_var" '%s' "$(_resolve_provider_credential "$prov_upper" API_KEY)"
        fi
    fi

    # Endpoint: CLI flag > G8E_<TIER>_ENDPOINT > G8E_<PROVIDER>_ENDPOINT
    if [[ -z "${!flag_endpoint_var:-}" ]]; then
        local tier_endpoint="${!env_endpoint_var:-}"
        if [[ -n "$tier_endpoint" ]]; then
            printf -v "$flag_endpoint_var" '%s' "$tier_endpoint"
        elif [[ -n "$provider" ]]; then
            local prov_upper="${provider^^}"
            printf -v "$flag_endpoint_var" '%s' "$(_resolve_provider_credential "$prov_upper" ENDPOINT)"
        fi
    fi
}

if [[ "$SUB" == "send" ]]; then
    _resolve_tier PRIMARY
    _resolve_tier ASSISTANT
    _resolve_tier LITE
fi

if [[ -z "$CASE_ID" && -z "$INVESTIGATION_ID" && "$SUB" == "send" ]]; then
    NEW_CASE=true
fi

# --- Stream helper: poll the Operator's internal SSE buffer -----------------
_chat_stream_events() {
    local cli_session_id="$1" since="$2" timeout="$3"
    local last_id="$since"
    local idle=0
    local interval=0.5
    while :; do
        local args=()
        _build_protocol_curl_args args || return 1
        args+=(-X GET)
        # Operator auth requires either an Operator-Session-ID header bound to
        # the mTLS URI SAN, or a /app/<id> SAN. CLI certs carry the operator
        # session SAN, so we must include the matching session header for the
        # Operator middleware to accept this request.
        _append_g8e_auth_headers args
        local resp
        # CLI is a first-class BYO session type: poll only the cli namespace so
        # we never accidentally drain a colliding web session id. The substrate
        # refuses to talk about a bare session id - every routing target is
        # tagged at the type level.
        if ! resp=$(curl "${args[@]}" "$OPERATOR_HTTP_URL/api/internal/sse/events?cli_session_id=${cli_session_id}&since_id=${last_id}&limit=200" 2>/dev/null); then
            # curl failed (network issue, or Operator down)
            sleep "$interval"
            continue
        fi
        local count
        count=$(echo "$resp" | jq -r '.count // 0' 2>/dev/null || echo 0)
        if [[ "$count" -gt 0 ]]; then
            idle=0
            # Stream each event to stdout as: [id type] payload
            echo "$resp" | jq -c '.events[]' | while read -r event; do
                etype=$(echo "$event" | jq -r '.event_type')
                payload=$(echo "$event" | jq -r '.payload')
                id=$(echo "$event" | jq -r '.id')
                
                # Only show relevant events to the user
                case "$etype" in
                    "g8e.v1.ai.llm.chat.iteration.text.chunk.received")
                        printf "%s" "$payload" | jq -r '.event.data.content // empty'
                        ;;
                    "g8e.v1.ai.llm.chat.iteration.failed"|"g8e.v1.ai.llm.chat.iteration.stopped")
                        error_msg=$(echo "$payload" | jq -r '.event.data.error // "Unknown error"')
                        printf "\n\033[1;31m[%s]\033[0m %s\n" "$etype" "$error_msg"
                        ;;
                    "g8e.v1.ai.llm.chat.iteration.thinking.started")
                        thinking=$(echo "$payload" | jq -r '.event.data.thinking // empty')
                        action=$(echo "$payload" | jq -r '.event.data.action_type // "UPDATE"')
                        if [[ "$action" == "START" ]]; then
                            printf "\n\033[1;30mThinking...\033[0m "
                        elif [[ -n "$thinking" && "$thinking" != "null" ]]; then
                            printf "\033[1;30m.\033[0m"
                        fi
                        ;;
                    "g8e.v1.ai.llm.chat.iteration.text.completed")
                        printf "\n"
                        ;;
                    "g8e.v1.ai.llm.chat.iteration.started")
                        : # ignore start event
                        ;;
                    *)
                        # Optional: show tool calls or other progress
                        if [[ "$etype" == *"tool"* ]]; then
                            tool_name=$(echo "$payload" | jq -r '.event.data.tool_name // "unknown"')
                            status=$(echo "$payload" | jq -r '.event.data.status // empty')
                            if [[ "$status" == "STARTED" ]]; then
                                printf "\n\033[1;34m[Tool: %s]\033[0m " "$tool_name"
                            fi
                        fi
                        ;;
                esac
                echo "$id" > "$_chat_cursor_file"
            done
            if [[ -s "$_chat_cursor_file" ]]; then
                last_id=$(cat "$_chat_cursor_file")
            fi
            # If we saw a terminal event, exit
            if echo "$resp" | jq -e '.events[] | select(.event_type | test("text\\.completed|failed|stopped"))' >/dev/null 2>&1; then
                return 0
            fi
        else
            idle=$(awk -v i="$idle" -v step="$interval" 'BEGIN{printf "%.1f", i+step}')
            if [[ "$timeout" -gt 0 ]]; then
                if awk -v i="$idle" -v t="$timeout" 'BEGIN{exit !(i+0 >= t+0)}'; then
                    echo "[chat] no new events in ${timeout}s - stopping" >&2
                    return 0
                fi
            fi
        fi
        sleep "$interval"
    done
}

_chat_cursor_file=$(mktemp)
trap 'rm -f "$_chat_cursor_file"' EXIT

case "$SUB" in
    send)
        if [[ -z "$MESSAGE" ]]; then
            echo "[chat] no message provided" >&2
            _chat_help
            exit 2
        fi

        # Build full request body with resource_creation for new cases
        # Pass LLM overrides to g8ee
        # cli_session_id is the disjoint CLI routing namespace minted at login;
        # NEVER use OPERATOR_SESSION_ID here - that authenticates the host
        # agent and conflating the two would let an operator session drain a
        # client's event stream.
        context_json="{\"cli_session_id\": \"${G8E_CLI_SESSION_ID:-}\", \"user_id\": \"${G8E_USER_ID:-}\", \"case_id\": \"${CASE_ID:-}\", \"investigation_id\": \"${INVESTIGATION_ID:-}\", \"source_component\": \"client\"}"
        
        body_obj=$(jq -n \
            --argjson context "$context_json" \
            --arg message "$MESSAGE" \
            --arg sentinel_mode "true" \
            --arg primary_provider "$LLM_PRIMARY_PROVIDER" \
            --arg primary_model "$LLM_PRIMARY_MODEL" \
            --arg assistant_provider "$LLM_ASSISTANT_PROVIDER" \
            --arg assistant_model "$LLM_ASSISTANT_MODEL" \
            --arg lite_provider "$LLM_LITE_PROVIDER" \
            --arg lite_model "$LLM_LITE_MODEL" \
            --arg primary_api_key "$LLM_PRIMARY_API_KEY" \
            --arg primary_endpoint "$LLM_PRIMARY_ENDPOINT" \
            --arg assistant_api_key "$LLM_ASSISTANT_API_KEY" \
            --arg assistant_endpoint "$LLM_ASSISTANT_ENDPOINT" \
            --arg lite_api_key "$LLM_LITE_API_KEY" \
            --arg lite_endpoint "$LLM_LITE_ENDPOINT" \
            '{
                context: $context,
                message: $message,
                attachments: [],
                sentinel_mode: ($sentinel_mode == "true"),
                llm_primary_provider: (if $primary_provider != "" then $primary_provider else null end),
                llm_primary_model: (if $primary_model != "" then $primary_model else null end),
                llm_assistant_provider: (if $assistant_provider != "" then $assistant_provider else null end),
                llm_assistant_model: (if $assistant_model != "" then $assistant_model else null end),
                llm_lite_provider: (if $lite_provider != "" then $lite_provider else null end),
                llm_lite_model: (if $lite_model != "" then $lite_model else null end),
                llm_primary_api_key: (if $primary_api_key != "" then $primary_api_key else null end),
                llm_primary_endpoint: (if $primary_endpoint != "" then $primary_endpoint else null end),
                llm_assistant_api_key: (if $assistant_api_key != "" then $assistant_api_key else null end),
                llm_assistant_endpoint: (if $assistant_endpoint != "" then $assistant_endpoint else null end),
                llm_lite_api_key: (if $lite_api_key != "" then $lite_api_key else null end),
                llm_lite_endpoint: (if $lite_endpoint != "" then $lite_endpoint else null end)
            }')

        if [[ "$NEW_CASE" == "true" ]]; then
            body=$(echo "$body_obj" | jq -c '. + {resource_creation: {create_case: true}}')
        else
            body=$(echo "$body_obj" | jq -c '.')
        fi

        _banner "sending chat to g8ee..."
        resp=$(_g8ee_curl POST "/api/internal/chat" "$body") || {
            echo "[chat] g8ee connection failed" >&2; exit 1
        }
        _check_g8e_error "$resp" "chat"
        echo "$resp" | jq . 2>/dev/null || echo "$resp"

        _banner "streaming events for cli session ${G8E_CLI_SESSION_ID:0:12}... (Ctrl+C to stop)"
        _chat_stream_events "${G8E_CLI_SESSION_ID}" 0 "$TIMEOUT_SECS"
        ;;
    tail)
        _banner "tailing events for cli session ${G8E_CLI_SESSION_ID:0:12}... (Ctrl+C to stop)"
        _chat_stream_events "${G8E_CLI_SESSION_ID}" "$SINCE_ID" "$TIMEOUT_SECS"
        ;;
    *)
        echo "[chat] unknown subcommand: $SUB" >&2
        _chat_help
        exit 2 ;;
esac
