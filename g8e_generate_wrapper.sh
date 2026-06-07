#!/bin/bash
# g8e_generate_wrapper.sh
# Wrapper script to fix path separators for protobuf generation and bypass Makefile syntax errors

echo "--- Running Protobuf Generation ---"

if ! command -v buf &> /dev/null || [ ! -f "./buf" ]; then
    echo "Error: Buf not found or executable. Please run 'make proto' first, or ensure Buf is installed." >&2
    exit 1
fi

# Find all *.proto files and pass the list to buf generate.
# The find command itself handles OS path separators correctly for the output string.
PROTO_FILES=$(find protocol/proto -type f -name "*.proto" | sort);

if [ -z "$$PROTO_FILES" ]; then \
    echo "Warning: No .proto files found in protocol/proto directory."; \
else \
    # This single command is designed to be robust against path separators.
    $$($$(buf) generate $$PROTO_FILES); \
fi

echo "Protobuf generation attempt complete."