#!/usr/bin/env bash
set -e

_footer() {
    local rc=$?
    [[ $rc -eq 0 ]] || return
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  logs.sh done"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
}
trap _footer EXIT

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  logs.sh $*"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
G8E_RUNTIME_DIR="${G8E_RUNTIME_DIR:-$PROJECT_ROOT/.g8e}"
LOG_DIR="$G8E_RUNTIME_DIR/logs"

CORE_SERVICES="operator g8ee g8ed"
ALL_SERVICES="operator g8ee g8ed"

usage() {
    echo "Usage: ./g8e platform logs [options] [service...]"
    echo ""
    echo "Search and display platform logs across all components in time order."
    echo "Default: last 200 lines from core services (operator, g8ee, g8ed), no follow."
    echo ""
    echo "Filter options:"
    echo "  -g, --grep <pattern>    Include lines matching pattern (grep -Ei, case-insensitive)"
    echo "  -v, --invert <pattern>  Exclude lines matching pattern (grep -Eiv)"
    echo "  -l, --level <level>     Filter by log level: error, warn, info, debug"
    echo "  -s, --since <duration>  Show logs since duration (e.g. 5m, 1h, 30s) or timestamp"
    echo ""
    echo "Output options:"
    echo "  -n, --tail <N>          Lines from end per service (default: 200; use 'all' for all)"
    echo "  -f, --follow            Stream new log lines (default: off)"
    echo "  --all                   Include all services (default: core only)"
    echo ""
    echo "Services (optional, space-separated, overrides defaults):"
    echo "  operator  g8ee  g8ed"
    echo ""
    echo "Examples:"
    echo "  ./g8e platform logs                          # last 200 lines, all core services"
    echo "  ./g8e platform logs --level error            # errors only"
    echo "  ./g8e platform logs --level warn --follow    # stream warnings+"
    echo "  ./g8e platform logs --grep 'operator|investigation'"
    echo "  ./g8e platform logs --since 5m               # last 5 minutes"
    echo "  ./g8e platform logs --since 1h --level error # errors in last hour"
    echo "  ./g8e platform logs --invert 'cache (HIT|MISS)|healthcheck'"
    echo "  ./g8e platform logs g8ee g8ed --tail 50"
    echo "  ./g8e platform logs operator                # operator listen mode logs"
    exit 0
}

FOLLOW=false
TAIL=200
INCLUDE_ALL=false
SERVICES=()
GREP_PATTERN=""
INVERT_PATTERN=""
LEVEL_PATTERN=""
SINCE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help) usage ;;
        -f|--follow) FOLLOW=true; shift ;;
        -n|--tail) TAIL="$2"; shift 2 ;;
        --all) INCLUDE_ALL=true; shift ;;
        -g|--grep) GREP_PATTERN="$2"; shift 2 ;;
        -v|--invert) INVERT_PATTERN="$2"; shift 2 ;;
        -s|--since) SINCE="$2"; shift 2 ;;
        -l, --level)
            case "${2,,}" in
                error) LEVEL_PATTERN="error" ;;
                warn)  LEVEL_PATTERN="warn" ;;
                info)  LEVEL_PATTERN="info" ;;
                debug) LEVEL_PATTERN="debug" ;;
                *) echo "[g8e] unknown level: '$2' (valid: error, warn, info, debug)" >&2; exit 1 ;;
            esac
            shift 2 ;;
        operator|g8ee|g8ed) SERVICES+=("$1"); shift ;;
        *) echo "[g8e] unknown logs option: '$1'" >&2; exit 1 ;;
    esac
done

if [[ ${#SERVICES[@]} -eq 0 ]]; then
    if $INCLUDE_ALL; then
        read -ra SERVICES <<< "$ALL_SERVICES"
    else
        read -ra SERVICES <<< "$CORE_SERVICES"
    fi
fi

# Map service names to log file paths
_service_log_file() {
    case "$1" in
        operator) echo "$LOG_DIR/operator-listen.log" ;;
        g8ee)     echo "$LOG_DIR/g8ee.log" ;;
        g8ed)     echo "$LOG_DIR/g8ed.log" ;;
        *)        echo "" ;;
    esac
}

_needs_pipe() {
    [[ -n "$GREP_PATTERN" || -n "$INVERT_PATTERN" || -n "$LEVEL_PATTERN" ]]
}

# Build the tail command
TAIL_CMD="tail"
[[ "$TAIL" == "all" ]] || TAIL_CMD="tail -n $TAIL"

# Build the grep pipeline
PIPE_CMD=""
if _needs_pipe; then
    PIPE_CMD="cat"
    [[ -n "$LEVEL_PATTERN" ]] && PIPE_CMD="$PIPE_CMD | grep -Ei ' - (${LEVEL_PATTERN^^}|${LEVEL_PATTERN}) '"
    [[ -n "$GREP_PATTERN" ]]  && PIPE_CMD="$PIPE_CMD | grep -Ei '$GREP_PATTERN'"
    [[ -n "$INVERT_PATTERN" ]] && PIPE_CMD="$PIPE_CMD | grep -Eiv '$INVERT_PATTERN'"
fi

# Process each service's log file
for service in "${SERVICES[@]}"; do
    log_file="$(_service_log_file "$service")"
    if [[ -z "$log_file" ]]; then
        echo "[g8e] unknown service: '$service'" >&2
        continue
    fi

    if [[ ! -f "$log_file" ]]; then
        echo "[g8e] log file not found: $log_file" >&2
        continue
    fi

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  $service: $log_file"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    if $FOLLOW; then
        # Follow mode with filtering
        if [[ -n "$PIPE_CMD" ]]; then
            tail -f "$log_file" | eval "$PIPE_CMD"
        else
            tail -f "$log_file"
        fi
    else
        # Non-follow mode with filtering
        if [[ -n "$PIPE_CMD" ]]; then
            eval "$TAIL_CMD $log_file | $PIPE_CMD"
        else
            $TAIL_CMD "$log_file"
        fi
    fi
done
