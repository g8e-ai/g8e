# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

#!/bin/bash
# g8e Auto-Detect Deploy Script
# Detects OS and architecture automatically and deploys the appropriate g8e binary.
# This script is embedded in the g8e gateway binary and served at:
#   http://<gateway-ip>:8080/g8e-deploy.sh
# Run on remote hosts to download and deploy the g8e binary.

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

GATEWAY_HOST="${GATEWAY_HOST:-{{.GatewayHost}}}"
GATEWAY_PORT="${GATEWAY_PORT:-{{.GatewayPort}}}"

echo -e "${GREEN}Deploying g8e...${NC}"

# Clean up existing certificates
echo -e "${YELLOW}Cleaning up existing certificates...${NC}"
rm -rf "${HOME}/.g8e/pki"

# Detect OS
OS="$(uname -s)"
case "$OS" in
  Linux)
    OS="linux"
    ;;
  Darwin)
    OS="darwin"
    ;;
  *)
    echo -e "${RED}Unsupported OS: $OS${NC}"
    echo "This deploy script supports Linux and macOS. For Windows, use PowerShell:"
    echo "iwr http://${GATEWAY_HOST}:${GATEWAY_PORT}/g8e-deploy.ps1 | iex"
    exit 1
    ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  aarch64|arm64)
    ARCH="arm64"
    ;;
  i386|i686)
    ARCH="386"
    ;;
  *)
    echo -e "${RED}Unsupported architecture: $ARCH${NC}"
    echo "Supported architectures: amd64, arm64, 386"
    exit 1
    ;;
esac

# Construct download URL
BINARY_NAME="g8e-${OS}-${ARCH}"
if [ "$OS" = "windows" ]; then
  BINARY_NAME="${BINARY_NAME}.exe"
fi
DOWNLOAD_URL="http://${GATEWAY_HOST}:${GATEWAY_PORT}/.well-known/g8e/bin/${BINARY_NAME}"
CHECKSUM_URL="${DOWNLOAD_URL}.sha256"

echo -e "${YELLOW}Detected: $OS $ARCH${NC}"
echo -e "${YELLOW}Downloading from: $DOWNLOAD_URL${NC}"

# Download the binary and checksum to temporary files before replacing the
# existing executable.
TEMP_BINARY="g8e.download"
TEMP_CHECKSUM="g8e.download.sha256"
trap 'rm -f "$TEMP_BINARY" "$TEMP_CHECKSUM"' EXIT
if command -v curl &> /dev/null; then
  curl -fsSL "$DOWNLOAD_URL" -o "$TEMP_BINARY"
  curl -fsSL "$CHECKSUM_URL" -o "$TEMP_CHECKSUM"
elif command -v wget &> /dev/null; then
  wget -q "$DOWNLOAD_URL" -O "$TEMP_BINARY"
  wget -q "$CHECKSUM_URL" -O "$TEMP_CHECKSUM"
else
  echo -e "${RED}Neither curl nor wget found. Please install one of them.${NC}"
  exit 1
fi

if command -v sha256sum &> /dev/null; then
  (cd "$(dirname "$TEMP_BINARY")" && sha256sum -c "$(basename "$TEMP_CHECKSUM")")
elif command -v shasum &> /dev/null; then
  EXPECTED="$(awk '{print $1}' "$TEMP_CHECKSUM")"
  ACTUAL="$(shasum -a 256 "$TEMP_BINARY" | awk '{print $1}')"
  [ "$EXPECTED" = "$ACTUAL" ] || { echo -e "${RED}Checksum verification failed.${NC}"; exit 1; }
else
  echo -e "${RED}Neither sha256sum nor shasum found. Cannot verify download.${NC}"
  exit 1
fi

mv "$TEMP_BINARY" g8e
chmod +x g8e

echo -e "${GREEN}g8e deployed successfully!${NC}"

echo "Starting g8e Operator to connect to Gateway at ${GATEWAY_HOST}..."
./g8e operator start -e "${GATEWAY_HOST}"
