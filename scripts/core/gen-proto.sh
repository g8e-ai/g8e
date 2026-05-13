#!/bin/bash
# Protobuf code generation for g8e platform components.
#
# Generates:
#   - Go code for g8eo
#   - Python code for g8ee
#
# Usage: ./scripts/core/gen-proto.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PROTO_SRC_DIR="$PROJECT_ROOT/shared/proto"

# Output directories
GO_OUT_DIR="$PROJECT_ROOT/components/g8eo/shared/proto"
PY_OUT_DIR="$PROJECT_ROOT/components/g8ee/app/proto"

mkdir -p "$GO_OUT_DIR" "$PY_OUT_DIR"

echo "Generating Protobuf code..."

# We use the g8eo-test-runner container as it has the Go/Python/JS environments
# installed.
# We will use docker compose run to execute the protoc commands.

# Go generation
docker run --rm -u $(id -u):$(id -g) -v "$PROJECT_ROOT:/workspace" -w /workspace namely/protoc-all \
    -i shared/proto \
    -d shared/proto \
    -l go \
    -o components/g8eo \
    --go-module-prefix github.com/g8e-ai/g8e/components/g8eo \
    --go-opt=module=github.com/g8e-ai/g8e/components/g8eo

# Python generation
docker run --rm -u $(id -u):$(id -g) -v "$PROTO_SRC_DIR:/proto_src" -v "$PY_OUT_DIR:/py_out" namely/protoc-all -i /proto_src -d /proto_src -l python -o /py_out

# Post-process Python files to fix imports for package structure
touch "$PY_OUT_DIR/__init__.py"
sed -i 's/^import \(.*_pb2\)/from . import \1/' "$PY_OUT_DIR"/*_pb2*.py

echo "Protobuf generation complete."
