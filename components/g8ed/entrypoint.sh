#!/bin/sh
# g8ed Entrypoint script - waits for operator then starts the application

set -e

# Wait for operator to be ready and platform_settings to be initialized
echo "[G8ED-ENTRYPOINT] Waiting for operator health check and platform_settings..."
MAX_RETRIES=30
RETRY_COUNT=0
# Derive project root using shared utility
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SCRIPT_DIR}/../../scripts/core/path_utils.sh"

SECRETS_DIR="${G8E_SECRETS_DIR:-${G8E_PROJECT_ROOT}/.g8e/secrets}"
PKI_DIR="${G8E_PKI_DIR:-${G8E_PROJECT_ROOT}/.g8e/pki}"


# Load security tokens into environment if files exist
if [ -f "${SECRETS_DIR}/internal_auth_token" ]; then
    export G8E_INTERNAL_AUTH_TOKEN=$(cat "${SECRETS_DIR}/internal_auth_token" | tr -d ' \n\r')
fi

if [ -f "${SECRETS_DIR}/session_encryption_key" ]; then
    export G8E_SESSION_ENCRYPTION_KEY=$(cat "${SECRETS_DIR}/session_encryption_key" | tr -d ' \n\r')
fi

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if [ -f "${SECRETS_DIR}/internal_auth_token" ] && [ -f "${PKI_DIR}/ca.crt" ]; then
        INTERNAL_TOKEN=$(cat "${SECRETS_DIR}/internal_auth_token" | tr -d '\n\r')
        # Check if operator is responding on the health endpoint
        # AND check if platform_settings is initialized (returns 200 instead of 401)
        if curl -s -f --cacert "${PKI_DIR}/ca.crt" -H "X-Internal-Auth: ${INTERNAL_TOKEN}" https://localhost:9000/db/settings/platform_settings > /dev/null; then
            echo "[G8ED-ENTRYPOINT] operator is ready and platform_settings are initialized"
            break
        fi
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "[G8ED-ENTRYPOINT] operator not fully ready yet (attempt $RETRY_COUNT/$MAX_RETRIES), waiting 2s..."
    sleep 2
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "[G8ED-ENTRYPOINT] ERROR: operator health check failed or platform_settings missing after $MAX_RETRIES attempts"
    exit 1
fi

# Trust the operator CA certificate for Node.js built-in fetch (undici)
export NODE_EXTRA_CA_CERTS="${PKI_DIR}/ca.crt"

exec node server.js
