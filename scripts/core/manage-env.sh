#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Managed environment variable helper for g8e.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
G8E_ENV_FILE="$PROJECT_ROOT/.g8e/.env"

# Ensure .g8e directory exists
mkdir -p "$PROJECT_ROOT/.g8e"

_usage() {
    echo "Usage: g8e vars <command> [options]"
    echo ""
    echo "Commands:"
    echo "  ls, list             List all g8e-related environment variables"
    echo "  set <key> <value>    Set an environment variable in .g8e/.env"
    echo "  get <key>            Get the value of a specific environment variable"
    echo "  unset <key>          Remove an environment variable from .g8e/.env"
    echo "  load                 Print export commands for the current environment"
    echo ""
    echo "Example:"
    echo "  ./g8e vars set G8E_LOG_LEVEL debug"
}

_list() {
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  g8e Environment Variables"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    # Define known variables for better display
    local known_vars=(
        "G8E_PROJECT_ROOT"
        "G8E_RUNTIME_DIR"
        "G8E_PKI_DIR"
        "G8E_SECRETS_DIR"
        "G8E_TRUST_BUNDLE"
        "G8E_DATA_DIR"
        "G8E_INTERNAL_AUTH_TOKEN"
        "G8E_SESSION_ENCRYPTION_KEY"
        "G8E_INTERNAL_HTTP_URL"
        "G8E_INTERNAL_PUBSUB_URL"
        "G8EE_INTERNAL_URL"
        "G8E_LOG_LEVEL"
        "G8E_OPERATOR_API_KEY"
        "TEST_LLM_PROVIDER"
        "TEST_LLM_API_KEY"
    )

    printf "%-30s %s\n" "VARIABLE" "VALUE"
    printf "%-30s %s\n" "--------" "-----"

    for var in "${known_vars[@]}"; do
        local val="${!var:-}"
        if [[ -z "$val" && -f "$G8E_ENV_FILE" ]]; then
            val=$(grep "^export $var=" "$G8E_ENV_FILE" | cut -d'=' -f2- | tr -d '"' || true)
        fi
        
        if [[ -n "$val" ]]; then
            # Mask sensitive values
            if [[ "$var" == *"TOKEN"* || "$var" == *"KEY"* || "$var" == *"SECRET"* ]]; then
                printf "%-30s %s\n" "$var" "******** (set)"
            else
                printf "%-30s %s\n" "$var" "$val"
            fi
        else
            printf "%-30s %s\n" "$var" "(not set)"
        fi
    done
    echo ""
}

_set_var() {
    local key="$1"
    local value="$2"
    if [[ -z "$key" || -z "$value" ]]; then
        _usage
        exit 1
    fi

    touch "$G8E_ENV_FILE"
    if grep -q "^export $key=" "$G8E_ENV_FILE"; then
        # Update existing
        sed -i "s|^export $key=.*|export $key=\"$value\"|" "$G8E_ENV_FILE"
    else
        # Append new
        echo "export $key=\"$value\"" >> "$G8E_ENV_FILE"
    fi
    echo "[g8e] Set $key in $G8E_ENV_FILE"
}

_get_var() {
    local key="$1"
    if [[ -z "$key" ]]; then
        _usage
        exit 1
    fi
    
    local val="${!key:-}"
    if [[ -z "$val" && -f "$G8E_ENV_FILE" ]]; then
        val=$(grep "^export $key=" "$G8E_ENV_FILE" | cut -d'=' -f2- | tr -d '"' || true)
    fi
    echo "$val"
}

_unset_var() {
    local key="$1"
    if [[ -z "$key" ]]; then
        _usage
        exit 1
    fi
    
    if [[ -f "$G8E_ENV_FILE" ]]; then
        sed -i "/^export $key=/d" "$G8E_ENV_FILE"
        echo "[g8e] Unset $key in $G8E_ENV_FILE"
    fi
}

_load() {
    if [[ -f "$G8E_ENV_FILE" ]]; then
        cat "$G8E_ENV_FILE"
    fi
}

case "$1" in
    ls|list)  _list ;;
    set)      _set_var "$2" "$3" ;;
    get)      _get_var "$2" ;;
    unset)    _unset_var "$2" ;;
    load)     _load ;;
    *)        _usage; exit 1 ;;
esac
