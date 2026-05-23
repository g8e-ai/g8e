#!/bin/bash
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

set -euo pipefail

# Resolve script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

OUTPUT_FILE="${PROJECT_ROOT}/docs/reference/cli.md"

echo "Generating CLI reference documentation..."

# Create output directory if it doesn't exist
mkdir -p "$(dirname "${OUTPUT_FILE}")"

# Start the markdown file
cat > "${OUTPUT_FILE}" << 'EOF'
# CLI Reference

This document is auto-generated from the CLI help output. Do not edit manually.

EOF

# Extract g8e CLI help
if [ -f "${PROJECT_ROOT}/g8e" ]; then
    echo "## g8e Platform Commands" >> "${OUTPUT_FILE}"
    echo "" >> "${OUTPUT_FILE}"
    echo "The \`g8e\` CLI is the primary interface for platform management." >> "${OUTPUT_FILE}"
    echo "" >> "${OUTPUT_FILE}"
    echo '```' >> "${OUTPUT_FILE}"
    "${PROJECT_ROOT}/g8e" --help >> "${OUTPUT_FILE}" 2>&1 || true
    echo '```' >> "${OUTPUT_FILE}"
    echo "" >> "${OUTPUT_FILE}"
else
    echo "Warning: g8e CLI not found at ${PROJECT_ROOT}/g8e" >&2
    echo "Hint: Run 'make build-cli' to build the CLI binary." >&2
fi

# Extract g8eo binary help if it exists
G8EO_BIN="${PROJECT_ROOT}/services/g8eo/build/linux-amd64/g8e.operator"
if [ -f "${G8EO_BIN}" ]; then
    echo "## g8eo Operator Binary" >> "${OUTPUT_FILE}"
    echo "" >> "${OUTPUT_FILE}"
    echo "The \`g8e.operator\` binary is the host-side Policy Execution Point (PEP)." >> "${OUTPUT_FILE}"
    echo "" >> "${OUTPUT_FILE}"
    echo '```' >> "${OUTPUT_FILE}"
    "${G8EO_BIN}" --help >> "${OUTPUT_FILE}" 2>&1 || true
    echo '```' >> "${OUTPUT_FILE}"
    echo "" >> "${OUTPUT_FILE}"
else
    echo "Warning: g8eo binary not found at ${G8EO_BIN}" >&2
    echo "Hint: Run 'make build-g8eo' to build the operator binary." >&2
fi

echo "CLI reference documentation generated at ${OUTPUT_FILE}"
