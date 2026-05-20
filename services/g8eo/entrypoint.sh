#!/bin/sh
# g8eo Entrypoint script - starts the Operator Agent (g8e.operator)
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
. "${SCRIPT_DIR}/../../scripts/core/path_utils.sh"
G8E_PROJECT_ROOT="$(resolve_g8e_root)"
export G8E_PROJECT_ROOT

OPERATOR_BIN="${G8E_PROJECT_ROOT}/services/g8eo/build/linux-amd64/g8e.operator"

# Fallback to local build if binary not present (useful in dev/test)
if [ ! -f "$OPERATOR_BIN" ]; then
    echo "Operator binary not found, compiling locally..."
    (cd "${G8E_PROJECT_ROOT}/services/g8eo" && make build-local)
fi

echo "Starting g8eo Operator Agent..."
exec "$OPERATOR_BIN" "$@"
