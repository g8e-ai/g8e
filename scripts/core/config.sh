#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Structured configuration loader for g8e CLI scripts.
# This script initializes local variables from G8E_ENV_* environment keys,
# avoiding the need for indirect shell expansion like ${!G8E_ENV_...}.

# Ensure we have the project root
if [[ -z "${G8E_PROJECT_ROOT:-}" ]]; then
    # Fallback if not already set (though path_utils.sh usually handles this)
    G8E_PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
fi

# Configuration directory
G8E_CONFIG_DIR="$G8E_PROJECT_ROOT/.g8e"
G8E_ENV_FILE="$G8E_CONFIG_DIR/.env"

# Load environment file if it exists
if [[ -f "$G8E_ENV_FILE" ]]; then
    # Use a subshell to avoid polluting the current shell if we only want to validate
    # but here we actually want to export them.
    # shellcheck disable=SC1090
    source "$G8E_ENV_FILE"
fi

# Define standard g8e environment variable keys
# These are the keys used in the G8E_ENV_* mapping.
# They are sourced from scripts/cmd/env_vars.sh if available,
# but we define defaults here to ensure the loader works independently.
if [[ -f "$G8E_PROJECT_ROOT/scripts/cmd/env_vars.sh" ]]; then
    # shellcheck disable=SC1090
    source "$G8E_PROJECT_ROOT/scripts/cmd/env_vars.sh"
fi

# Fallback defaults if env_vars.sh is missing or incomplete
export G8E_ENV_OPERATOR_SESSION_ID="${G8E_ENV_OPERATOR_SESSION_ID:-G8E_OPERATOR_SESSION_ID}"
export G8E_ENV_CLI_SESSION_ID="${G8E_ENV_CLI_SESSION_ID:-G8E_CLI_SESSION_ID}"
export G8E_ENV_USER_ID="${G8E_ENV_USER_ID:-G8E_USER_ID}"
export G8E_ENV_OPERATOR_ID="${G8E_ENV_OPERATOR_ID:-G8E_OPERATOR_ID}"
export G8E_ENV_G8EE_URL="${G8E_ENV_G8EE_URL:-G8EE_URL}"
export G8E_ENV_INTERNAL_HTTP_URL="${G8E_ENV_INTERNAL_HTTP_URL:-G8E_INTERNAL_HTTP_URL}"
export G8E_ENV_PKIDir="${G8E_ENV_PKIDir:-G8E_PKI_DIR}"
export G8E_ENV_SECRETS_DIR="${G8E_ENV_SECRETS_DIR:-G8E_SECRETS_DIR}"
export G8E_ENV_PROTOCOL_DIR="${G8E_ENV_PROTOCOL_DIR:-G8E_PROTOCOL_DIR}"

# Initialize structured configuration variables
# These variables should be used instead of indirect expansion.

# Auth / Sessions
G8E_OPERATOR_SESSION_ID="${!G8E_ENV_OPERATOR_SESSION_ID:-}"
G8E_CLI_SESSION_ID="${!G8E_ENV_CLI_SESSION_ID:-}"
G8E_USER_ID="${!G8E_ENV_USER_ID:-}"
G8E_OPERATOR_ID="${!G8E_ENV_OPERATOR_ID:-}"

# URLs
G8E_G8EE_URL="${!G8E_ENV_G8EE_URL:-}"
G8E_INTERNAL_HTTP_URL="${!G8E_ENV_INTERNAL_HTTP_URL:-}"

# Paths
G8E_PKI_DIR="${!G8E_ENV_PKIDir:-}"
G8E_SECRETS_DIR="${!G8E_ENV_SECRETS_DIR:-}"
G8E_PROTOCOL_DIR="${!G8E_ENV_PROTOCOL_DIR:-}"

# Export these so they are available to sub-processes
export G8E_OPERATOR_SESSION_ID
export G8E_CLI_SESSION_ID
export G8E_USER_ID
export G8E_OPERATOR_ID
export G8E_G8EE_URL
export G8E_INTERNAL_HTTP_URL
export G8E_PKI_DIR
export G8E_SECRETS_DIR
export G8E_PROTOCOL_DIR
