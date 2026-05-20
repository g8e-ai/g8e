#!/bin/sh
# g8eg Entrypoint script - starts the Governance Gateway (g8e.gateway) in listen mode
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SCRIPT_DIR}/../../scripts/core/path_utils.sh"
G8E_PROJECT_ROOT="$(resolve_g8e_root)"
export G8E_PROJECT_ROOT

G8E_RUNTIME_DIR="${G8E_RUNTIME_DIR:-${G8E_PROJECT_ROOT}/.g8e}"
DATA_DIR="${OPERATOR_LISTEN_DATA_DIR:-${G8E_RUNTIME_DIR}/data}"
PKI_DIR="${OPERATOR_LISTEN_PKI_DIR:-${G8E_RUNTIME_DIR}/pki}"
SECRETS_DIR="${OPERATOR_LISTEN_SECRETS_DIR:-${G8E_RUNTIME_DIR}/secrets}"

GATEWAY_BIN="${G8E_PROJECT_ROOT}/services/g8eo/build/linux-amd64/g8e.gateway"

# Fallback to local build if binary not present (useful in dev/test)
if [ ! -f "$GATEWAY_BIN" ]; then
    echo "Gateway binary not found, compiling locally..."
    (cd "${G8E_PROJECT_ROOT}/services/g8eo" && make build-local)
fi

# Ensure ports match values from paths.json constants
HTTP_PORT=$(jq -r '.ports.operator_http // "9000"' "${G8E_PROJECT_ROOT}/protocol/constants/paths.json" 2>/dev/null || echo "9000")
WSS_PORT=$(jq -r '.ports.operator_wss // "9001"' "${G8E_PROJECT_ROOT}/protocol/constants/paths.json" 2>/dev/null || echo "9001")
BOOTSTRAP_PORT=$(jq -r '.ports.operator_bootstrap // "9002"' "${G8E_PROJECT_ROOT}/protocol/constants/paths.json" 2>/dev/null || echo "9002")
PUBLIC_PORT=$(jq -r '.ports.operator_public // "9003"' "${G8E_PROJECT_ROOT}/protocol/constants/paths.json" 2>/dev/null || echo "9003")

echo "Starting g8eg Governance Gateway (listen mode) on ports HTTP:${HTTP_PORT}, WSS:${WSS_PORT}..."
exec "$GATEWAY_BIN" --listen \
    --data-dir "$DATA_DIR" \
    --pki-dir "$PKI_DIR" \
    --secrets-dir "$SECRETS_DIR" \
    --http-listen-port "$HTTP_PORT" \
    --wss-listen-port "$WSS_PORT" \
    --bootstrap-listen-port "$BOOTSTRAP_PORT" \
    --public-listen-port "$PUBLIC_PORT" "$@"
