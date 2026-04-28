#!/bin/bash
# g8e node Entrypoint
#
# supervisord is exec'd as PID 1. It manages the operator process as a
# supervised service so its output appears in Docker logs and it restarts
# automatically on failure. g8ed persists the operator API key to the
# platform_settings document in g8es, then signals supervisor to start the
# operator. The operator command fetches its API key from g8es at startup.
#
# The operator binary must be built explicitly via: ./g8e operator build

set -euo pipefail

SSL_DIR="${G8E_SSL_DIR:-/g8es}"

# Load Internal Auth Token (required for supervisor inet_http_server password)
if [ -f "${SSL_DIR}/internal_auth_token" ]; then
    export G8E_INTERNAL_AUTH_TOKEN=$(cat "${SSL_DIR}/internal_auth_token" | tr -d '\n\r')
    echo "[g8ep] Loaded G8E_INTERNAL_AUTH_TOKEN from volume"
fi

# Load Session Encryption Key
if [ -f "${SSL_DIR}/session_encryption_key" ]; then
    export G8E_SESSION_ENCRYPTION_KEY=$(cat "${SSL_DIR}/session_encryption_key" | tr -d '\n\r')
    echo "[g8ep] Loaded G8E_SESSION_ENCRYPTION_KEY from volume"
fi

# CA cert paths (G8E_PUBSUB_CA_CERT, G8E_SSL_CERT_FILE) are set by docker-compose.

# Default the supervisor port for the static config's %(ENV_G8E_SUPERVISOR_PORT)s.
export G8E_SUPERVISOR_PORT="${G8E_SUPERVISOR_PORT:-443}"

echo "[g8ep] Starting supervisord"
exec /usr/bin/supervisord -c /app/components/g8ep/scripts/supervisord.conf
