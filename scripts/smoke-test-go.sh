#!/bin/bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# See the License for the specific language governing permissions and
# limitations under the License.

# Smoke test: verify the Go module can be imported in a clean project,
# mirroring the README quickstart.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Go smoke test ==="

VERSION=$(cat "$REPO_ROOT/VERSION" | tr -d '\n' | sed 's/^v//')

# Create a temp project that imports the g8e module
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

cd "$TMPDIR"
go mod init smoke-test

# Write a minimal main that imports a public package
cat > main.go <<'EOF'
package main

import (
	"fmt"
	"github.com/g8e-ai/g8e/protocol"
)

func main() {
	_ = protocol.NewWorkloadIdentity()
	fmt.Println("Go SDK import OK")
}
EOF

# Use the local module (replace directive for testing without publishing)
go mod edit -replace github.com/g8e-ai/g8e="$REPO_ROOT"
go mod tidy

# Build the binary
go build -o /dev/null .

echo "Go SDK import verified successfully"
echo "=== Go smoke test PASSED ==="
