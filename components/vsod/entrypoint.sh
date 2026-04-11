#!/bin/sh
# VSOD Entrypoint script - waits for VSODB then starts the application

set -e

# Wait for VSODB to be ready and platform_settings to be initialized
echo "[VSOD-ENTRYPOINT] Waiting for VSODB health check and platform_settings..."
MAX_RETRIES=30
RETRY_COUNT=0
SSL_DIR="${G8E_SSL_DIR:-/vsodb}"

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if [ -f "${SSL_DIR}/internal_auth_token" ] && [ -f "${SSL_DIR}/ca.crt" ]; then
        INTERNAL_TOKEN=$(cat "${SSL_DIR}/internal_auth_token" | tr -d '\n\r')
        # Check if VSODB is responding on the health endpoint
        # AND check if platform_settings is initialized (returns 200 instead of 401)
        if curl -s -f --cacert "${SSL_DIR}/ca.crt" -H "X-Internal-Auth: ${INTERNAL_TOKEN}" https://vsodb:9001/db/settings/platform_settings > /dev/null; then
            echo "[VSOD-ENTRYPOINT] VSODB is ready and platform_settings are initialized"
            break
        fi
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "[VSOD-ENTRYPOINT] VSODB not fully ready yet (attempt $RETRY_COUNT/$MAX_RETRIES), waiting 2s..."
    sleep 2
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "[VSOD-ENTRYPOINT] ERROR: VSODB health check failed or platform_settings missing after $MAX_RETRIES attempts"
    exit 1
fi

# Trust the VSODB CA certificate for Node.js built-in fetch (undici)
export NODE_EXTRA_CA_CERTS="${SSL_DIR}/ca.crt"

exec node server.js
