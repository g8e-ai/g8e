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

# Load Internal Auth Token
if [ -f "${SSL_DIR}/internal_auth_token" ]; then
    export G8E_INTERNAL_AUTH_TOKEN=$(cat "${SSL_DIR}/internal_auth_token" | tr -d '\n\r')
    echo "[g8ep] Loaded G8E_INTERNAL_AUTH_TOKEN from volume"
fi

# Load Session Encryption Key
if [ -f "${SSL_DIR}/session_encryption_key" ]; then
    export G8E_SESSION_ENCRYPTION_KEY=$(cat "${SSL_DIR}/session_encryption_key" | tr -d '\n\r')
    echo "[g8ep] Loaded G8E_SESSION_ENCRYPTION_KEY from volume"
fi

# Load CA Certificate path
if [ -f "${SSL_DIR}/ca.crt" ]; then
    export G8E_PUBSUB_CA_CERT="${SSL_DIR}/ca.crt"
    export G8E_SSL_CERT_FILE="${SSL_DIR}/ca.crt"
    echo "[g8ep] Configured CA certificate paths from ${SSL_DIR}/ca.crt"
fi

SUPERVISOR_CONF=/tmp/g8e.operator.conf

_write_supervisor_conf() {
    local endpoint="${G8E_OPERATOR_ENDPOINT:-${G8E_GATEWAY_OPERATOR_ENDPOINT:-g8e.local}}"
    local auth_token="${G8E_INTERNAL_AUTH_TOKEN:-}"
    local supervisor_port="${G8E_SUPERVISOR_PORT:-443}"
    cat > "${SUPERVISOR_CONF}" <<EOF
[supervisord]
nodaemon=true
logfile=/dev/null
logfile_maxbytes=0
pidfile=/tmp/supervisord.pid
childlogdir=/var/log/supervisor

[unix_http_server]
file=/tmp/supervisor.sock

[inet_http_server]
port = *:${supervisor_port}
username = g8e-internal
password = ${auth_token}

[supervisorctl]
serverurl=unix:///tmp/supervisor.sock

[rpcinterface:supervisor]
supervisor.rpcinterface_factory = supervisor.rpcinterface:make_main_rpcinterface

[program:operator]
command=/app/components/g8ep/scripts/fetch-key-and-run.sh
autostart=true
autorestart=true
startsecs=10
startretries=3
stopwaitsecs=10
stdout_logfile=/dev/fd/1
stdout_logfile_maxbytes=0
stderr_logfile=/dev/fd/1
stderr_logfile_maxbytes=0
EOF
    echo "[g8ep] Supervisor config written (endpoint: ${endpoint})"
}

_write_supervisor_conf

echo "[g8ep] Starting supervisord"
exec /usr/bin/supervisord -c "${SUPERVISOR_CONF}"
