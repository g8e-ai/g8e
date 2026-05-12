#!/bin/bash
# validate-platform-security.sh - Validate g8e platform security configuration

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}Starting Platform Security Validation...${NC}"

check_file_exists() {
    local container=$1
    local file_path=$2
    if docker exec "$container" ls "$file_path" > /dev/null 2>&1; then
        echo -e "  [${GREEN}OK${NC}] $container: $file_path exists"
        return 0
    else
        echo -e "  [${RED}FAIL${NC}] $container: $file_path MISSING"
        return 1
    fi
}

check_env_var() {
    local container=$1
    local var_name=$2
    local expected_file=$3
    
    # Read from /proc/1/environ to get the environment of the main process
    # This bypasses the issue where 'docker exec' starts a new shell session without the exported vars
    local actual_val=$(docker exec "$container" sh -c "cat /proc/1/environ" | tr '\0' '\n' | grep "^${var_name}=" | cut -d'=' -f2- | tr -d '\n\r')
    local expected_val=$(docker exec "$container" cat "$expected_file" 2>/dev/null | tr -d '\n\r' || echo "")
    
    if [ -z "$actual_val" ]; then
        echo -e "  [${RED}FAIL${NC}] $container: $var_name is EMPTY"
        return 1
    fi
    
    if [ "$actual_val" == "$expected_val" ]; then
        echo -e "  [${GREEN}OK${NC}] $container: $var_name matches $expected_file"
        return 0
    else
        echo -e "  [${RED}FAIL${NC}] $container: $var_name DOES NOT MATCH $expected_file"
        echo -e "         Actual:   [${actual_val}]"
        echo -e "         Expected: [${expected_val}]"
        return 1
    fi
}

# 2. Host security checks
echo -e "\n${YELLOW}2. Checking host security files...${NC}"
FAILED=0
G8E_PKI_DIR="${G8E_PKI_DIR:-$PROJECT_ROOT/.g8e/pki}"
G8E_SECRETS_DIR="${G8E_SECRETS_DIR:-$PROJECT_ROOT/.g8e/secrets}"

if [ ! -f "$G8E_PKI_DIR/root/root_ca.crt" ]; then echo -e "${RED}FAILED: $G8E_PKI_DIR/root/root_ca.crt missing${NC}"; FAILED=1; fi
if [ ! -f "$G8E_PKI_DIR/trust/hub-bundle.pem" ]; then echo -e "${RED}FAILED: $G8E_PKI_DIR/trust/hub-bundle.pem missing${NC}"; FAILED=1; fi
if [ ! -f "$G8E_SECRETS_DIR/internal_auth_token" ]; then echo -e "${RED}FAILED: $G8E_SECRETS_DIR/internal_auth_token missing${NC}"; FAILED=1; fi
if [ ! -f "$G8E_SECRETS_DIR/session_encryption_key" ]; then echo -e "${RED}FAILED: $G8E_SECRETS_DIR/session_encryption_key missing${NC}"; FAILED=1; fi
if [ ! -f "$G8E_SECRETS_DIR/bootstrap_digest.json" ]; then echo -e "${RED}FAILED: $G8E_SECRETS_DIR/bootstrap_digest.json missing${NC}"; FAILED=1; fi

# 3. Process and Port checks
echo -e "\n${YELLOW}3. Checking host processes and ports...${NC}"
check_port() {
    local port=$1
    if ! lsof -i :$port -sTCP:LISTEN -t >/dev/null; then
        echo -e "${RED}FAILED: Port $port is not listening${NC}"
        return 1
    fi
    return 0
}

check_port 9000 || FAILED=1 # g8eo (Operator --listen)
check_port 9001 || FAILED=1 # g8eo (WSS)
check_port 443 || FAILED=1  # g8ed/g8ee

if [ $FAILED -eq 0 ]; then
    echo -e "\n${GREEN}Platform Security Validation PASSED!${NC}"
    exit 0
else
    echo -e "\n${RED}Platform Security Validation FAILED!${NC}"
    exit 1
fi
