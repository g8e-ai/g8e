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

# 1. vsodb file checks
echo -e "\n${YELLOW}1. Checking vsodb security files...${NC}"
FAILED=0
check_file_exists g8e-data /ssl/ca.crt || FAILED=1
check_file_exists g8e-data /ssl/server.crt || FAILED=1
check_file_exists g8e-data /ssl/server.key || FAILED=1
check_file_exists g8e-data /ssl/internal_auth_token || FAILED=1
check_file_exists g8e-data /ssl/session_encryption_key || FAILED=1

# 2. Volume mount checks
echo -e "\n${YELLOW}2. Checking volume mounts...${NC}"
check_file_exists g8e-dashboard /vsodb/ca.crt || FAILED=1
check_file_exists g8e-dashboard /vsodb/internal_auth_token || FAILED=1
check_file_exists g8e-dashboard /vsodb/session_encryption_key || FAILED=1
check_file_exists g8ee /vsodb/ca.crt || FAILED=1
check_file_exists g8ee /vsodb/internal_auth_token || FAILED=1
check_file_exists g8ee /vsodb/session_encryption_key || FAILED=1

# 3. INTERNAL_AUTH_TOKEN checks
echo -e "\n${YELLOW}3. Checking G8E_INTERNAL_AUTH_TOKEN environment variables...${NC}"
check_env_var g8e-data G8E_INTERNAL_AUTH_TOKEN /ssl/internal_auth_token || FAILED=1
check_env_var g8e-dashboard G8E_INTERNAL_AUTH_TOKEN /vsodb/internal_auth_token || FAILED=1
check_env_var g8ee G8E_INTERNAL_AUTH_TOKEN /vsodb/internal_auth_token || FAILED=1

# 4. SESSION_ENCRYPTION_KEY checks
echo -e "\n${YELLOW}4. Checking G8E_SESSION_ENCRYPTION_KEY environment variables...${NC}"
check_env_var g8e-data G8E_SESSION_ENCRYPTION_KEY /ssl/session_encryption_key || FAILED=1
check_env_var g8e-dashboard G8E_SESSION_ENCRYPTION_KEY /vsodb/session_encryption_key || FAILED=1
check_env_var g8ee G8E_SESSION_ENCRYPTION_KEY /vsodb/session_encryption_key || FAILED=1

if [ $FAILED -eq 0 ]; then
    echo -e "\n${GREEN}Platform Security Validation PASSED!${NC}"
    exit 0
else
    echo -e "\n${RED}Platform Security Validation FAILED!${NC}"
    exit 1
fi
