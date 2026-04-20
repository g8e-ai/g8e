#!/bin/bash
# Fetch Operator API Key from g8es and Launch Operator
#
# Called by supervisord as the operator program command.
# Waits indefinitely with 2-second retry intervals for the API key
# to distinguish dependency readiness from actual crashes in logs.

set -euo pipefail

SSL_DIR="${G8E_SSL_DIR:-/g8es}"
AUTH_TOKEN="${G8E_INTERNAL_AUTH_TOKEN:-}"
ENDPOINT="${G8E_OPERATOR_ENDPOINT:-${G8E_GATEWAY_OPERATOR_ENDPOINT:-g8e.local}}"
LOG_LEVEL="${G8E_LOG_LEVEL:-info}"
PUBSUB_URL="${G8E_OPERATOR_PUBSUB_URL:-}"
G8ES_URL="https://g8es:9000/db/settings/platform_settings"
OPERATOR_BINARY="/home/g8e/g8e.operator"
OPERATOR_META="/home/g8e/g8e.operator.meta"
BLOB_URL="https://g8es:9000/blob/operator-binary"

RETRY_DELAY=2

_detect_arch() {
    case "$(uname -m)" in
        x86_64)  echo "amd64" ;;
        aarch64) echo "arm64" ;;
        i?86)    echo "386" ;;
        *)       echo "amd64" ;;
    esac
}

_fetch_metadata() {
    local arch
    arch=$(_detect_arch)
    local meta_url="${BLOB_URL}/linux-${arch}/meta"

    local response
    response=$(curl -sf \
        -H "X-Internal-Auth: ${AUTH_TOKEN}" \
        --cacert "${SSL_DIR}/ca.crt" \
        "${meta_url}" 2>/dev/null) || {
        echo "[g8ep] Failed to fetch operator binary metadata" >&2
        return 1
    }

    echo "$response"
}

_fetch_binary() {
    local arch
    arch=$(_detect_arch)
    echo "[g8ep] Downloading operator binary (linux/${arch}) from g8es blob store..." >&2

    local http_code
    http_code=$(curl -sf -o "${OPERATOR_BINARY}" -w '%{http_code}' \
        -H "X-Internal-Auth: ${AUTH_TOKEN}" \
        --cacert "${SSL_DIR}/ca.crt" \
        "${BLOB_URL}/linux-${arch}" 2>/dev/null)

    if [ "$http_code" != "200" ]; then
        rm -f "${OPERATOR_BINARY}"
        echo "[g8ep] Failed to download operator binary (HTTP ${http_code})" >&2
        echo "[g8ep] Run './g8e operator build' or './g8e platform setup' to compile the binary" >&2
        exit 1
    fi

    chmod +x "${OPERATOR_BINARY}"
    local size
    size=$(ls -lh "${OPERATOR_BINARY}" | awk '{print $5}')
    echo "[g8ep] Operator binary downloaded (${size})" >&2

    local metadata
    metadata=$(_fetch_metadata)
    if [ -n "$metadata" ]; then
        echo "$metadata" > "${OPERATOR_META}"
        echo "[g8ep] Operator binary metadata saved" >&2
    fi
}

# Readiness gating: wait indefinitely for Operator API key to be available
# This distinguishes dependency waiting from actual crashes in log aggregation
attempt=0
while true; do
    attempt=$((attempt + 1))
    echo "[g8ep] Waiting for Operator API key in platform_settings (attempt ${attempt})..." >&2
    response=$(curl -sf \
        -H "X-Internal-Auth: ${AUTH_TOKEN}" \
        --cacert "${SSL_DIR}/ca.crt" \
        "${G8ES_URL}" 2>/dev/null) || {
            echo "[g8ep] g8es not reachable, waiting ${RETRY_DELAY}s for readiness..." >&2
            sleep "$RETRY_DELAY"
            continue
        }

    # Document fetched, now try to extract the key
    G8E_OPERATOR_API_KEY=$(echo "$response" | python3 -c "import sys,json; data=json.load(sys.stdin); print(data.get('settings', {}).get('g8ep_operator_api_key', ''))")

    if [ -n "$G8E_OPERATOR_API_KEY" ]; then
        echo "[g8ep] Operator API key obtained, proceeding to launch operator" >&2
        break
    fi

    echo "[g8ep] Operator API key not yet available in platform_settings, waiting ${RETRY_DELAY}s for readiness..." >&2
    sleep "$RETRY_DELAY"
done

should_download=false

if [ ! -x "${OPERATOR_BINARY}" ]; then
    echo "[g8ep] Operator binary not found or not executable, downloading..." >&2
    should_download=true
else
    current_metadata=$(_fetch_metadata)
    if [ -z "$current_metadata" ]; then
        echo "[g8ep] Could not fetch current metadata, skipping update check" >&2
    else
        if [ ! -f "${OPERATOR_META}" ]; then
            echo "[g8ep] No local metadata found, downloading to establish baseline..." >&2
            should_download=true
        else
            local_metadata=$(cat "${OPERATOR_META}")
            if [ "$current_metadata" != "$local_metadata" ]; then
                echo "[g8ep] Operator binary metadata changed, re-downloading..." >&2
                should_download=true
            else
                echo "[g8ep] Operator binary up to date" >&2
            fi
        fi
    fi
fi

if [ "$should_download" = true ]; then
    _fetch_binary
fi

export G8E_OPERATOR_API_KEY

OPERATOR_FLAGS=(--endpoint "$ENDPOINT" --working-dir /home/g8e --no-git --log "$LOG_LEVEL" --cloud --provider g8ep)

if [ -n "$PUBSUB_URL" ]; then
    export G8E_OPERATOR_PUBSUB_URL="$PUBSUB_URL"
fi

exec "$OPERATOR_BINARY" "${OPERATOR_FLAGS[@]}"
