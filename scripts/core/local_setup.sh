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

set -euo pipefail

# Use canonical root detection
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
G8E_PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
export G8E_PROJECT_ROOT

# Source common helpers if needed
source "$G8E_PROJECT_ROOT/scripts/cmd/common.sh"

_banner() {
    clear
    echo -e "\033[1;34m  __ _   ___   ___  \033[0m"
    echo -e "\033[1;34m / _\` | ( _ ) / _ \\ \033[0m"
    echo -e "\033[1;34m| (_| | / _ \\|  __/ \033[0m"
    echo -e "\033[1;34m \\__, | \\___/ \\___| \033[0m"
    echo -e "\033[1;34m  ___/              \033[0m"
    echo ""
    echo -e "\033[1;32m   g8e Environment Setup\033[0m"
    echo ""
}

_quick_start() {
    _banner
    echo "Quick Start Options:"
    echo "1) Protocol + Local Operator (Gateway Mode)"
    echo "   - Lightest possible setup."
    echo "   - Ideal for BYO clients, MCP/A2A translators, and custom agents."
    echo "   - Runs the Local Operator (g8eg) in --gateway mode."
    echo ""
    echo "2) Protocol + Local Operator + Apps (Substrate + Engine)"
    echo "   - Full g8e experience."
    echo "   - Includes the reference g8e-Compliant Agentic Ensemble (g8ee)."
    echo "   - Ideal for evaluation and direct use of g8e agents."
    echo ""
    echo "0) Back"
    echo ""
    read -p "Choose a mode [0-2]: " mode
    case $mode in
        1)
            echo "Configuring Protocol + Local Operator (Gateway Mode)..."
            _update_env "G8E_WITH_G8EE=0"
            echo -e "\033[1;32m✓ Environment updated.\033[0m"
            echo ""
            echo "Next steps:"
            echo "1. Run './g8e platform start'"
            echo "2. Run './g8e login' to authenticate"
            ;;
        2)
            echo "Configuring Protocol + Local Operator + g8e-Compliant Agentic Ensemble mode..."
            _update_env "G8E_WITH_G8EE=1"
            echo -e "\033[1;32m✓ Environment updated.\033[0m"
            echo ""
            echo "Next steps:"
            echo "1. Run './g8e platform start'"
            echo "2. Run './g8e apps start g8ee'"
            echo "3. Open https://localhost:8443 in your browser (Reference UI)"
            ;;
        0) return ;;
        *) echo "Invalid option"; sleep 1; _quick_start ;;
    esac
}

_advanced_setup() {
    _banner
    echo "Advanced Setup:"
    echo "Configure custom paths and platform settings."
    echo ""
    
    # Check current mode
    local apps_enabled=$(grep "G8E_WITH_G8EE=1" "$G8E_PROJECT_ROOT/.g8e/.env" 2>/dev/null || echo "")
    
    read -p "Custom Runtime Directory? [$G8E_PROJECT_ROOT/.g8e]: " custom_runtime
    if [[ -n "$custom_runtime" ]]; then
        _update_env "G8E_RUNTIME_DIR=$custom_runtime"
    fi

    if [[ -n "$apps_enabled" ]]; then
        echo ""
        echo "Apps are enabled. Configuring external providers:"
        read -p "Configure LLM provider? [y/N]: " conf_llm
        if [[ "$conf_llm" =~ ^[Yy]$ ]]; then
            bash "$G8E_PROJECT_ROOT/scripts/tools/setup-llm.sh"
        fi
        
        read -p "Configure Search provider? [y/N]: " conf_search
        if [[ "$conf_search" =~ ^[Yy]$ ]]; then
            bash "$G8E_PROJECT_ROOT/scripts/tools/setup-search.sh"
        fi
    fi

    echo ""
    read -p "Configure SSH? [y/N]: " conf_ssh
    if [[ "$conf_ssh" =~ ^[Yy]$ ]]; then
        bash "$G8E_PROJECT_ROOT/scripts/tools/setup-ssh.sh"
    fi

    echo ""
    echo -e "\033[1;32m✓ Advanced setup complete.\033[0m"
}

_update_env() {
    local entry="$1"
    local key="${entry%%=*}"
    local env_file="$G8E_PROJECT_ROOT/.g8e/.env"
    
    mkdir -p "$(dirname "$env_file")"
    touch "$env_file"
    
    if grep -q "^export $key=" "$env_file"; then
        sed -i "s|^export $key=.*|export $entry|" "$env_file"
    else
        echo "export $entry" >> "$env_file"
    fi
}

# Main
if [[ $# -gt 0 && "$1" == "--advanced" ]]; then
    _advanced_setup
elif [[ $# -gt 0 && ("$1" == "--quick" || "$1" == "quick") ]]; then
    _quick_start
else
    while true; do
        _banner
        echo "1) Quick Start"
        echo "2) Advanced Setup"
        echo "0) Back"
        echo ""
        read -p "Choose an option [0-2]: " opt
        case $opt in
            1) _quick_start ;;
            2) _advanced_setup ;;
            0) exit 0 ;;
            *) echo "Invalid option"; sleep 1 ;;
        esac
    done
fi
