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
