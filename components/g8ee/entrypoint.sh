#!/bin/sh
# g8ee Entrypoint script - waits for operator then starts the application

set -e

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

# operator readiness is gated by docker-compose `depends_on: operator: service_healthy`.
# Execute the main application - bootstrap service handles secret loading
exec uvicorn app.main:app --host 0.0.0.0 --port 8443 \
    --ssl-keyfile "${PKI_DIR}/server.key" \
    --ssl-certfile "${PKI_DIR}/server.crt"
