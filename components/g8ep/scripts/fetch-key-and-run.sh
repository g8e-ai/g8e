#!/bin/bash
# Fetch Operator API Key from g8es and Launch Operator
#
# Called by supervisord as the operator program command.
# Retries the g8es fetch with exponential backoff to handle
# transient unavailability during container restarts.

set -euo pipefail

SSL_DIR="${G8E_SSL_DIR:-/g8es}"
AUTH_TOKEN="${G8E_INTERNAL_AUTH_TOKEN:-}"
ENDPOINT="${G8E_OPERATOR_ENDPOINT:-${G8E_GATEWAY_OPERATOR_ENDPOINT:-g8e.local}}"
LOG_LEVEL="${G8E_LOG_LEVEL:-info}"
PUBSUB_URL="${G8E_OPERATOR_PUBSUB_URL:-}"
G8ES_URL="https://g8es:9000/db/settings/platform_settings"
OPERATOR_BINARY="/home/g8e/g8e.operator"
BLOB_URL="https://g8es:9000/blob/operator-binary"

MAX_RETRIES=5
RETRY_DELAY=2

_detect_arch() {
    case "$(uname -m)" in
        x86_64)  echo "amd64" ;;
        aarch64) echo "arm64" ;;
        i?86)    echo "386" ;;
        *)       echo "amd64" ;;
    esac
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
}

for attempt in $(seq 1 "$MAX_RETRIES"); do
    echo "[g8ep] Fetching platform settings from g8es (attempt ${attempt}/${MAX_RETRIES})..." >&2
    response=$(curl -sf \
        -H "X-Internal-Auth: ${AUTH_TOKEN}" \
        --cacert "${SSL_DIR}/ca.crt" \
        "${G8ES_URL}" 2>/dev/null) || {
            echo "[g8ep] g8es not reachable, retrying in ${RETRY_DELAY}s..." >&2
            sleep "$RETRY_DELAY"
            RETRY_DELAY=$((RETRY_DELAY * 2))
            continue
        }

    # Document fetched, now try to extract the key
    G8E_OPERATOR_API_KEY=$(echo "$response" | python3 -c "import sys,json; data=json.load(sys.stdin); print(data.get('settings', {}).get('g8ep_operator_api_key', ''))")

    if [ -n "$G8E_OPERATOR_API_KEY" ]; then
        break
    fi

    if [ "$attempt" -eq "$MAX_RETRIES" ]; then
        echo "[g8ep] Failed to obtain Operator API key from platform_settings after ${MAX_RETRIES} attempts" >&2
        exit 1
    fi

    echo "[g8ep] Operator API key not yet available in platform_settings, retrying in ${RETRY_DELAY}s..." >&2
    sleep "$RETRY_DELAY"
    RETRY_DELAY=$((RETRY_DELAY * 2))
done

if [ ! -x "${OPERATOR_BINARY}" ]; then
    _fetch_binary
fi

export G8E_OPERATOR_API_KEY

OPERATOR_FLAGS=(--endpoint "$ENDPOINT" --working-dir /home/g8e --no-git --log "$LOG_LEVEL" --cloud --provider g8ep)

if [ -n "$PUBSUB_URL" ]; then
    export G8E_OPERATOR_PUBSUB_URL="$PUBSUB_URL"
fi

exec "$OPERATOR_BINARY" "${OPERATOR_FLAGS[@]}"
