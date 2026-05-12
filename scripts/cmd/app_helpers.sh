#!/usr/bin/env bash

# Application-layer helpers (g8ed, g8ee, evals)
# These are decoupled from the core Operator substrate.

_ensure_g8ed() {
    if ! _g8ed_running; then
        echo "[g8e] g8ed is not running — start the platform: ./g8e platform start" >&2
        exit 1
    fi
}

_g8ed_curl() {
    local method="$1" path="$2" body="${3:-}"
    local g8ed_url="${G8ED_INTERNAL_URL:-https://localhost}"
    local args=(-sk -X "$method" --cacert "$G8E_SSL_DIR_HOST/ca.crt")
    [[ -n "$OPERATOR_SESSION_ID" ]] && args+=(-H "x-operator-session-id: $OPERATOR_SESSION_ID")
    args+=(-H "Content-Type: application/json")
    [[ -n "$body" ]] && args+=(-d "$body")
    curl "${args[@]}" "$g8ed_url$path"
}

_validate_session() {
    local operator_session_id="$1"
    local response
    response=$(_g8ed_curl POST '/api/auth/operator/validate' \
        "{\"operator_session_id\":\"$operator_session_id\"}" 2>/dev/null || true)
    [[ -z "$response" ]] && return 1
    echo "$response" | grep -q '"valid":true'
}

# --- Eval Fleet Helpers (App-coupled) ---

_eval_bound_operator_session_id() {
    local response
    response=$(_g8ed_curl GET '/api/internal/operators' 2>/dev/null || true)
    [[ -z "$response" ]] && return 1
    local session_id
    session_id=$(python3 -c '
import sys, json
try:
    data = json.loads(sys.argv[1])
    if not data.get("success"):
        sys.exit(1)
    for op in data.get("data", []):
        hb = op.get("latest_heartbeat_snapshot") or {}
        si = hb.get("system_identity") or {}
        hn = si.get("hostname") or ""
        if op.get("status") == "BOUND" and hn.startswith("evals-eval-node-"):
            print(op["operator_session_id"])
            sys.exit(0)
except Exception:
    sys.exit(1)
sys.exit(1)
' "$response" 2>/dev/null)
    if [[ -n "$session_id" ]]; then
        echo "$session_id"
        return 0
    fi
    return 1
}

_eval_operator_user_id() {
    local session_id="$1"
    local response
    response=$(_g8ed_curl GET '/api/internal/operators' 2>/dev/null || true)
    [[ -z "$response" ]] && return 1
    local user_id
    user_id=$(python3 -c '
import sys, json
try:
    data = json.loads(sys.argv[2])
    if not data.get("success"):
        sys.exit(1)
    for op in data.get("data", []):
        if op.get("operator_session_id") == sys.argv[1]:
            print(op["user_id"])
            sys.exit(0)
except Exception:
    sys.exit(1)
sys.exit(1)
' "$session_id" "$response")
    if [[ -n "$user_id" ]]; then
        echo "$user_id"
        return 0
    fi
    return 1
}
