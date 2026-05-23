#!/bin/sh
# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# g8eo Entrypoint script - starts the Operator Agent (g8e.operator)
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
G8E_PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
export G8E_PROJECT_ROOT

# Set runtime defaults
export G8E_RUNTIME_DIR="${G8E_RUNTIME_DIR:-$G8E_PROJECT_ROOT/.g8e}"
export G8E_DATA_DIR="${G8E_DATA_DIR:-$G8E_RUNTIME_DIR/data}"
export G8E_PKI_DIR="${G8E_PKI_DIR:-$G8E_RUNTIME_DIR/pki}"
export G8E_SECRETS_DIR="${G8E_SECRETS_DIR:-$G8E_RUNTIME_DIR/secrets}"

# Load environment file if it exists
G8E_ENV_FILE="$G8E_PROJECT_ROOT/.g8e/.env"
if [[ -f "$G8E_ENV_FILE" ]]; then
    set -a
    source "$G8E_ENV_FILE"
    set +a
fi

OPERATOR_BIN="${G8E_PROJECT_ROOT}/services/g8eo/build/linux-amd64/g8e.operator"

# Fallback to local build if binary not present (useful in dev/test)
if [ ! -f "$OPERATOR_BIN" ]; then
    echo "Operator binary not found, compiling locally..."
    (cd "${G8E_PROJECT_ROOT}/services/g8eo" && make build-local)
fi

echo "Starting g8eo Operator Agent..."
exec "$OPERATOR_BIN" "$@"
