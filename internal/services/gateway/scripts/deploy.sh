#!/bin/bash
# g8e Auto-Detect Deploy Script
# Detects OS and architecture automatically and deploys the appropriate g8e binary.
# This script is embedded in the g8e gateway binary and served at:
#   http://<gateway-ip>:8080/deploy.sh
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
    echo "iwr http://${GATEWAY_HOST}:${GATEWAY_PORT}/deploy.ps1 -UseBasicParsing | iex"
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

echo -e "${YELLOW}Detected: $OS $ARCH${NC}"
echo -e "${YELLOW}Downloading from: $DOWNLOAD_URL${NC}"

# Download binary
if command -v curl &> /dev/null; then
  curl -fsSL "$DOWNLOAD_URL" -o g8e
elif command -v wget &> /dev/null; then
  wget -q "$DOWNLOAD_URL" -O g8e
else
  echo -e "${RED}Neither curl nor wget found. Please install one of them.${NC}"
  exit 1
fi

# Make executable
chmod +x g8e

echo -e "${GREEN}g8e deployed successfully!${NC}"

# Run PKI enrollment
echo -e "${YELLOW}Enrolling with Gateway at ${GATEWAY_HOST}...${NC}"
ENROLLMENT_OUTPUT=$(./g8e security pki enroll --endpoint "${GATEWAY_HOST}")
echo "$ENROLLMENT_OUTPUT"

# Extract operator session ID from enrollment output
OPERATOR_SESSION_ID=$(echo "$ENROLLMENT_OUTPUT" | grep "Operator Session ID:" | awk '{print $4}')

if [ -z "$OPERATOR_SESSION_ID" ]; then
  echo -e "${RED}Failed to extract operator session ID from enrollment output${NC}"
  exit 1
fi

echo -e "${GREEN}Enrollment complete!${NC}"
echo ""
echo "Starting g8e Operator to connect to Gateway at ${GATEWAY_HOST}..."
export G8E_OPERATOR_SESSION_ID="$OPERATOR_SESSION_ID"
./g8e -e "${GATEWAY_HOST}" -k ".g8e/pki/operator.key" --cert ".g8e/pki/operator.crt"
