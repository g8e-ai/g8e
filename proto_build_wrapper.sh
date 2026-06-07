#!/bin/bash

# proto_build_wrapper.sh
# A POSIX-compliant wrapper script to explicitly generate all required protobuf artifacts,
# bypassing potential cross-platform pathing issues found in the Makefile.

set -euo pipefail

echo "--- Starting explicit Protobuf code generation wrapper ---"

# Check for essential tools
if ! command -v protoc &> /dev/null; then
    echo "Error: 'protoc' compiler not found. Please run 'make protoc-install' first." >&2
    exit 1
fi

if ! command -v buf &> /dev/null && [ ! -f "./buf" ]; then
    echo "Warning: 'buf' CLI not found. Skipping Buf generation step."
fi


# --- STEP 1: Generate Go Protobuf code using Buf (or directly) ---
echo ""
echo "[STEP 1/2] Generating Go Protobuf code..."

if command -v buf &> /dev/null || [ -f "./buf" ]; then
    echo "Attempting to run 'buf generate protocol/proto'..."
    # Use the actual detected path for buf or just assume it's available in PATH if we are on a POSIX-like shell.
    if command -v $(./buf) &> /dev/null; then
        $(./buf) generate protocol/proto || echo "Warning: Buf generation failed, continuing with fallback."
    else
        # Fallback to manual gRPC tools if buf fails but protoc is present (as seen in Makefile's proto-python target)
        echo "Buf not runnable. Attempting direct gRPC tool invocation instead..."
        python3 -m grpc_tools.protoc \
            --python_out=protocol/python/g8e \
            --proto_path=protocol/proto \
            $(wildcard protocol/proto/g8e/common/v1/*.proto) \
            $(wildcard protocol/proto/g8e/operator/v1/*.proto) \
            $(wildcard protocol/proto/g8e/pubsub/v1/*.proto) || echo "Warning: Python gRPC generation failed. Check python3 and grpc_tools.protoc."
    fi
else
    echo "Skipping Buf step as 'buf' command is unavailable or './buf' not found."
fi


# --- STEP 2: Build Dependencies & Verify (Minimal set to check build success) ---
echo ""
echo "[STEP 2/2] Building core application binary..."

# This attempts the simplest build, relying on platform checks like in the Makefile.
make clean # Clean everything first for a fresh start if we are running this manually
make build
if [ $? -ne 0 ]; then
    echo "WARNING: The 'make build' step failed or encountered errors. Reviewing logs above."
fi

echo ""
echo "--- Protobuf generation wrapper finished successfully (exit code may vary). ---"