#!/bin/sh
# g8e Dashboard (g8ed) Entrypoint — waits for the gateway HTTP health surface,
# then starts the static SPA host (plain HTTP on port 3000).
#
# The gateway enforces strict port separation (see
# docs/guides/connect_apps_to_gateway.md): HTTP 8080 for unauthenticated
# health checks, HTTPS 8443 for the API surface (mTLS or web session). The
# frontend container has no reason to speak HTTPS to the gateway — the
# browser makes the cross-origin HTTPS calls directly — so the readiness
# probe uses plain HTTP via the container-internal hostname.

set -e

echo "[g8ed-ENTRYPOINT] Waiting for gateway health check..."
MAX_RETRIES=30
RETRY_COUNT=0
GATEWAY_HEALTH_URL="${GATEWAY_HEALTH_URL:-http://g8eg:8080}"
GATEWAY_HEALTH_PATH="${GATEWAY_HEALTH_PATH:-/api/v1/health}"

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if curl -s -f "${GATEWAY_HEALTH_URL}${GATEWAY_HEALTH_PATH}" > /dev/null; then
        echo "[g8ed-ENTRYPOINT] Gateway is ready"
        break
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "[g8ed-ENTRYPOINT] Gateway not ready yet (attempt $RETRY_COUNT/$MAX_RETRIES), waiting 2s..."
    sleep 2
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "[g8ed-ENTRYPOINT] ERROR: Gateway health check failed after $MAX_RETRIES attempts"
    exit 1
fi

exec node server.js
