#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Common utility functions for g8e shell scripts
# This file should be sourced by scripts that need these helpers
# Note: This file complements common.sh - it contains additional utilities
# that are not already defined in common.sh

# Display an error message with the g8e prefix
# Usage: _error <message>
_error() {
    echo -e "\033[1;31m[g8e]\033[0m $*" >&2
}

# Display a warning message with the g8e prefix
# Usage: _warn <message>
_warn() {
    echo -e "\033[1;33m[g8e]\033[0m $*" >&2
}

# Display a success message with the g8e prefix
# Usage: _success <message>
_success() {
    echo -e "\033[1;32m[g8e]\033[0m $*"
}

# Display an error for unknown subcommand
# Usage: _unknown_subcommand <subcommand> <valid_commands...>
_unknown_subcommand() {
    local subcmd="$1"
    shift
    local valid_cmds="$*"
    _error "unknown subcommand: '$subcmd'"
    echo "  Valid: $valid_cmds" >&2
    exit 1
}

# Display an error for unknown option
# Usage: _unknown_option <option>
_unknown_option() {
    local opt="$1"
    _error "unknown option: $opt"
    exit 1
}

# Display an error for missing required value
# Usage: _require_value <flag>
_require_value() {
    local flag="$1"
    _error "$flag requires a value"
    exit 1
}

# Confirm a destructive operation with the user
# Usage: _confirm_destructive <operation_name> <warning_text> [args...]
# Returns 0 if confirmed, 1 if cancelled
# Checks args for -y/--yes/--force to bypass confirmation
_confirm_destructive() {
    local operation="$1"
    local warning="$2"
    local skip_confirm=false

    # Check for bypass flags in arguments
    for arg in "${@:3}"; do
        if [[ "$arg" == "-y" || "$arg" == "--yes" || "$arg" == "--force" ]]; then
            skip_confirm=true
            break
        fi
    done

    if [[ "$skip_confirm" == "true" ]]; then
        return 0
    fi

    if [[ ! -t 0 ]]; then
        _error "stdin is not a TTY. Interactive confirmation required. Use -y/--yes/--force to bypass."
        exit 1
    fi

    echo ""
    echo -e "\033[1;31mWARNING: You are about to $operation!\033[0m"
    echo "$warning"
    echo ""
    read -p "Are you sure you want to continue? (y/n): " confirm
    if [[ ! "$confirm" =~ ^[Yy]([Ee][Ss])?$ ]]; then
        echo "$operation cancelled."
        exit 0
    fi
    return 0
}

# Cross-platform sed -i helper
# Usage: _sed_i <sed_args...>
_sed_i() {
    if [[ "${OSTYPE:-}" == "darwin"* ]]; then
        sed -i "" "$@"
    else
        sed -i "$@"
    fi
}

# Check if a command exists
# Usage: _command_exists <command>
_command_exists() {
    command -v "$1" >/dev/null 2>&1
}
