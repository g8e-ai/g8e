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
GO_OUT_DIR="$PROJECT_ROOT/components/g8eo/internal/shared/proto"
PY_OUT_DIR="$PROJECT_ROOT/components/g8ee/app/proto"

mkdir -p "$GO_OUT_DIR" "$PY_OUT_DIR"

echo "Generating Protobuf code..."

# Python generation
echo "Generating Python code..."

# Try to find a suitable python with grpc_tools.protoc
if [ -f "$PROJECT_ROOT/components/g8ee/.venv/bin/python3" ]; then
    PYTHON_CMD="$PROJECT_ROOT/components/g8ee/.venv/bin/python3"
else
    PYTHON_CMD="python3"
fi

if "$PYTHON_CMD" -m grpc_tools.protoc --version >/dev/null 2>&1; then
    echo "Using $PYTHON_CMD -m grpc_tools.protoc..."
    # Generate each proto to ensure correct package structure
    for proto in common operator pubsub; do
        "$PYTHON_CMD" -m grpc_tools.protoc -I "$PROTO_SRC_DIR" --python_out="$PY_OUT_DIR" "$PROTO_SRC_DIR/$proto.proto"
    done
else
    echo "grpc_tools.protoc not found in venv or path, falling back to Docker..."
    docker run --rm -u $(id -u):$(id -g) -v "$PROTO_SRC_DIR:/proto_src" -v "$PY_OUT_DIR:/py_out" namely/protoc-all:latest -i /proto_src -d /proto_src -l python -o /py_out
fi

# Post-process Python files to fix imports for package structure
touch "$PY_OUT_DIR/__init__.py"
sed -i 's/^import \(.*_pb2\)/from . import \1/' "$PY_OUT_DIR"/*_pb2*.py

# Verify the generated code is parseable by the current python
if "$PYTHON_CMD" -c "import grpc_tools" 2>/dev/null; then
    echo "Verifying generated Python modules..."
    for f in "$PY_OUT_DIR"/*_pb2.py; do
        module_name=$(basename "$f" .py)
        echo "Checking $module_name..."
        # We need to test from the components/g8ee directory so 'app.proto' is a valid package
        (cd "$PROJECT_ROOT/components/g8ee" && "$PYTHON_CMD" -c "from app.proto import $module_name") || { echo "Failed to import $module_name!"; exit 1; }
    done
fi

# Go generation
# We generate each package explicitly to ensure correct output paths and package names
echo "Generating Go code..."
for proto in common operator pubsub; do
    echo "Generating $proto.proto..."
    docker run --rm -u $(id -u):$(id -g) -v "$PROJECT_ROOT:/workspace" -w /workspace namely/protoc-all:latest \
        -i shared/proto \
        -f shared/proto/$proto.proto \
        -l go \
        -o components/g8eo/internal \
        --go-module-prefix github.com/g8e-ai/g8e/components/g8eo/internal \
        --go-opt=module=github.com/g8e-ai/g8e/components/g8eo/internal
done

echo "Protobuf generation complete."
