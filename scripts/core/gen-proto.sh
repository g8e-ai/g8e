#!/bin/bash
# Protobuf code generation for g8e platform components.
#
# Generates:
#   - Go code for g8eo
#   - Python code for g8ee
#   - JavaScript/TypeScript code for g8ed
#
# Usage: ./scripts/core/gen-proto.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PROTO_SRC_DIR="$PROJECT_ROOT/shared/proto"

# Output directories
GO_OUT_DIR="$PROJECT_ROOT/components/g8eo/shared/proto"
PY_OUT_DIR="$PROJECT_ROOT/components/g8ee/app/proto"
JS_OUT_DIR="$PROJECT_ROOT/components/g8ed/shared/proto"

mkdir -p "$GO_OUT_DIR" "$PY_OUT_DIR" "$JS_OUT_DIR"

echo "Generating Protobuf code..."

# We use the g8eo-test-runner container as it has the Go/Python/JS environments
# installed.
# We will use docker compose run to execute the protoc commands.

# Go generation
docker compose run --rm -u root -v "$PROTO_SRC_DIR:/proto_src" -v "$GO_OUT_DIR:/go_out" g8eo-test-runner sh -c "\
  protoc -I=/proto_src -I=/usr/include \
  --go_out=/go_out --go_opt=module=github.com/g8e-ai/g8e/components/g8eo/shared/proto \
  --go-grpc_out=/go_out --go-grpc_opt=module=github.com/g8e-ai/g8e/components/g8eo/shared/proto \
  /proto_src/*.proto"

# Python generation
docker compose run --rm -u root -v "$PROTO_SRC_DIR:/proto_src" -v "$PY_OUT_DIR:/py_out" g8eo-test-runner sh -c "\
  python3 -m grpc_tools.protoc -I=/proto_src -I=/usr/include \
  --python_out=/py_out \
  --grpc_python_out=/py_out \
  /proto_src/*.proto"

# JS generation
docker compose run --rm -u root -v "$PROTO_SRC_DIR:/proto_src" -v "$JS_OUT_DIR:/js_out" g8eo-test-runner sh -c "\
  grpc_tools_node_protoc -I=/proto_src -I=/usr/include \
  --js_out=import_style=commonjs,binary:/js_out \
  --grpc-web_out=import_style=commonjs,mode=grpcwebtext:/js_out \
  /proto_src/*.proto"

# Fix ownership (since we ran as root in the container)
docker compose run --rm -u root -v "$GO_OUT_DIR:/go_out" -v "$PY_OUT_DIR:/py_out" -v "$JS_OUT_DIR:/js_out" g8eo-test-runner sh -c "\
  chown -R $(id -u):$(id -g) /go_out /py_out /js_out"

echo "Protobuf generation complete (placeholder)."
echo "NOTE: Ensure protoc and plugins are installed in the build environment."
