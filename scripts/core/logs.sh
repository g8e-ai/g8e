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
COMPOSE="docker compose -f $SCRIPT_DIR/../../docker-compose.yml"

CORE_SERVICES="vsodb vse vsod g8e-pod"
ALL_SERVICES="vsodb vse vsod g8e-pod"

usage() {
    echo "Usage: ./g8e platform logs [options] [service...]"
    echo ""
    echo "Search and display platform logs across all components in time order."
    echo "Default: last 200 lines from core services (vsodb, vse, vsod), no follow."
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
    echo "  --all                   Include g8e-pod/operator (default: core only)"
    echo ""
    echo "Services (optional, space-separated, overrides defaults):"
    echo "  vsodb  vse  vsod  g8e-pod"
    echo ""
    echo "Examples:"
    echo "  ./g8e platform logs                          # last 200 lines, all core services"
    echo "  ./g8e platform logs --level error            # errors only"
    echo "  ./g8e platform logs --level warn --follow    # stream warnings+"
    echo "  ./g8e platform logs --grep 'operator|investigation'"
    echo "  ./g8e platform logs --since 5m               # last 5 minutes"
    echo "  ./g8e platform logs --since 1h --level error # errors in last hour"
    echo "  ./g8e platform logs --invert 'cache (HIT|MISS)|healthcheck'"
    echo "  ./g8e platform logs vse vsod --tail 50"
    echo "  ./g8e platform logs g8e-pod              # operator process output (via supervisord)"
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
        -l|--level)
            case "${2,,}" in
                error) LEVEL_PATTERN="error" ;;
                warn)  LEVEL_PATTERN="warn" ;;
                info)  LEVEL_PATTERN="info" ;;
                debug) LEVEL_PATTERN="debug" ;;
                *) echo "[g8e] unknown level: '$2' (valid: error, warn, info, debug)" >&2; exit 1 ;;
            esac
            shift 2 ;;
        vsodb|vse|vsod|g8e-pod) SERVICES+=("$1"); shift ;;
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

ARGS=("--timestamps" "--tail=$TAIL")
[[ -n "$SINCE" ]] && ARGS+=("--since=$SINCE")
$FOLLOW && ARGS+=("--follow")

_needs_pipe() {
    [[ -n "$GREP_PATTERN" || -n "$INVERT_PATTERN" || -n "$LEVEL_PATTERN" ]]
}

if _needs_pipe; then
    PIPE_CMD="cat"
    [[ -n "$LEVEL_PATTERN" ]] && PIPE_CMD="$PIPE_CMD | grep -Ei ' - (${LEVEL_PATTERN^^}|${LEVEL_PATTERN}) '"
    [[ -n "$GREP_PATTERN" ]]  && PIPE_CMD="$PIPE_CMD | grep -Ei '$GREP_PATTERN'"
    [[ -n "$INVERT_PATTERN" ]] && PIPE_CMD="$PIPE_CMD | grep -Eiv '$INVERT_PATTERN'"
    eval "$COMPOSE logs ${ARGS[*]} ${SERVICES[*]} | $PIPE_CMD"
else
    $COMPOSE logs "${ARGS[@]}" "${SERVICES[@]}"
fi
