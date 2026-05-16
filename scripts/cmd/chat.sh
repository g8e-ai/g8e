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

Options for `tail`:
  --since <id>                Resume from this SSE event id (default: 0)
  --timeout <seconds>         Stop polling after N idle seconds (default: 0 = forever)

Authentication: requires `./g8e login` (uses mTLS + OPERATOR_SESSION_ID).
The g8ee Engine must be running: `./g8e apps start g8ee`.
EOF
}

case "$SUB" in
    -h|--help|help) _chat_help; exit 0 ;;
esac

_ensure_authenticated

if ! _g8ee_running; then
    echo "[g8e] g8ee Engine is not running — start it: ./g8e apps start g8ee" >&2
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

while [[ $# -gt 0 ]]; do
    case "$1" in
        --new)            NEW_CASE=true; shift ;;
        --case)           CASE_ID="$2"; shift 2 ;;
        --investigation)  INVESTIGATION_ID="$2"; shift 2 ;;
        --bound)          BOUND_OPERATORS="$2"; shift 2 ;;
        --timeout)        TIMEOUT_SECS="$2"; shift 2 ;;
        --since)          SINCE_ID="$2"; shift 2 ;;
        -h|--help)        _chat_help; exit 0 ;;
        --) shift; MESSAGE+="$* "; break ;;
        -*) echo "[chat] unknown flag: $1" >&2; exit 2 ;;
        *)  MESSAGE+="$1 "; shift ;;
    esac
done
MESSAGE="${MESSAGE%% }"

if [[ -z "$CASE_ID" && -z "$INVESTIGATION_ID" && "$SUB" == "send" ]]; then
    NEW_CASE=true
fi

# --- Stream helper: poll the Operator's internal SSE buffer -----------------
_chat_stream_events() {
    local session_id="$1" since="$2" timeout="$3"
    local last_id="$since"
    local idle=0
    local interval=0.5
    while :; do
        local args=()
        _build_protocol_curl_args args || return 1
        args+=(-X GET)
        local resp
        if ! resp=$(curl "${args[@]}" "$OPERATOR_HTTP_URL/api/internal/sse/events?session_id=${session_id}&since_id=${last_id}&limit=200" 2>/dev/null); then
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
                    echo "[chat] no new events in ${timeout}s — stopping" >&2
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
        if [[ "$NEW_CASE" == "true" ]]; then
            body="{\"context\": {\"cli_session_id\": \"${OPERATOR_SESSION_ID:-}\", \"user_id\": \"${USER_ID:-}\", \"case_id\": \"${CASE_ID:-}\", \"investigation_id\": \"${INVESTIGATION_ID:-}\", \"source_component\": \"client\"}, \"message\": \"$MESSAGE\", \"attachments\": [], \"sentinel_mode\": true, \"resource_creation\": {\"create_case\": true}}"
        else
            body="{\"context\": {\"cli_session_id\": \"${OPERATOR_SESSION_ID:-}\", \"user_id\": \"${USER_ID:-}\", \"case_id\": \"${CASE_ID:-}\", \"investigation_id\": \"${INVESTIGATION_ID:-}\", \"source_component\": \"client\"}, \"message\": \"$MESSAGE\", \"attachments\": [], \"sentinel_mode\": true}"
        fi

        _banner "sending chat to g8ee..."
        resp=$(_g8ee_curl POST "/api/internal/chat" "$body") || {
            echo "[chat] g8ee request failed" >&2; exit 1
        }
        echo "$resp" | jq . 2>/dev/null || echo "$resp"

        _banner "streaming events for session ${OPERATOR_SESSION_ID:0:12}... (Ctrl+C to stop)"
        _chat_stream_events "$OPERATOR_SESSION_ID" 0 "$TIMEOUT_SECS"
        ;;
    tail)
        _banner "tailing events for session ${OPERATOR_SESSION_ID:0:12}... (Ctrl+C to stop)"
        _chat_stream_events "$OPERATOR_SESSION_ID" "$SINCE_ID" "$TIMEOUT_SECS"
        ;;
    *)
        echo "[chat] unknown subcommand: $SUB" >&2
        _chat_help
        exit 2 ;;
esac
