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

_check_go() {
    if ! command -v go >/dev/null 2>&1; then
        echo -e "\033[0;31m✗ Go 1.26+ is required but not found.\033[0m"
        echo "  Install Go from https://go.dev/dl"
        return 1
    fi

    local go_version=$(go version | awk '{print $3}' | sed 's/go//')
    local major=$(echo "$go_version" | cut -d. -f1)
    local minor=$(echo "$go_version" | cut -d. -f2)

    if [[ "$major" -lt 1 ]] || [[ "$major" -eq 1 && "$minor" -lt 26 ]]; then
        echo -e "\033[0;31m✗ Go 1.26+ is required but found $go_version.\033[0m"
        echo "  Upgrade Go from https://go.dev/dl"
        return 1
    fi

    if [[ -z "$GOPATH" ]]; then
        echo -e "\033[0;33m⚠ GOPATH not set. Setting GOPATH=\$HOME/go\033[0m"
        export GOPATH="$HOME/go"
        export PATH="$GOPATH/bin:$PATH"
        echo "  Add to your shell profile (~/.bashrc or ~/.zshrc):"
        echo "  export GOPATH=\$HOME/go"
        echo "  export PATH=\$GOPATH/bin:\$PATH"
    fi

    echo -e "\033[1;32m✓ Go $go_version found\033[0m"
    return 0
}

_check_python() {
    # Check for pyenv Python first
    if [[ -f "$HOME/.pyenv/versions/3.14.0/bin/python" ]]; then
        export PYENV_ROOT="$HOME/.pyenv"
        export PATH="$PYENV_ROOT/bin:$PATH"
        eval "$(pyenv init -)"
        local py_version=$("$HOME/.pyenv/versions/3.14.0/bin/python" --version | awk '{print $2}')
        echo -e "\033[1;32m✓ Python $py_version found (via pyenv)\033[0m"
        return 0
    fi

    if ! command -v python3 >/dev/null 2>&1; then
        echo -e "\033[0;31m✗ Python 3.14+ is required but not found.\033[0m"
        echo "  Install Python 3.14+ from https://www.python.org/downloads/"
        return 1
    fi

    local py_version=$(python3 --version | awk '{print $2}')
    local major=$(echo "$py_version" | cut -d. -f1)
    local minor=$(echo "$py_version" | cut -d. -f2)

    if [[ "$major" -lt 3 ]] || [[ "$major" -eq 3 && "$minor" -lt 14 ]]; then
        echo -e "\033[0;31m✗ Python 3.14+ is required but found $py_version.\033[0m"
        echo ""
        read -p "Install Python 3.14 via pyenv? [y/N]: " install_pyenv
        if [[ "$install_pyenv" =~ ^[Yy]$ ]]; then
            _install_python_via_pyenv || return 1
        else
            echo "  Install Python 3.14+ from https://www.python.org/downloads/"
            return 1
        fi
    fi

    echo -e "\033[1;32m✓ Python $py_version found\033[0m"
    return 0
}

_install_python_via_pyenv() {
    echo "Installing Python 3.14 via pyenv..."

    if ! command -v pyenv >/dev/null 2>&1; then
        echo "Installing pyenv..."
        curl https://pyenv.run | bash || {
            echo -e "\033[0;31m✗ Failed to install pyenv\033[0m"
            return 1
        }

        export PATH="$HOME/.pyenv/bin:$PATH"
        eval "$(pyenv init -)"

        echo "Add to your shell profile (~/.bashrc or ~/.zshrc):"
        echo 'export PATH="$HOME/.pyenv/bin:$PATH"'
        echo 'eval "$(pyenv init -)"'
    fi

    echo "Installing Python 3.14..."
    pyenv install 3.14.0 || {
        echo -e "\033[0;31m✗ Failed to install Python 3.14\033[0m"
        echo "  This may be due to missing system dependencies."
        echo "  On Ubuntu/Debian, install: sudo apt-get install -y build-essential libssl-dev zlib1g-dev libbz2-dev libreadline-dev libsqlite3-dev curl"
        echo "  On macOS with Homebrew: brew install openssl readline sqlite3 xz zlib"
        return 1
    }

    pyenv global 3.14.0
    export PATH="$HOME/.pyenv/bin:$PATH"
    eval "$(pyenv init -)"

    # Export for child processes
    export PYENV_ROOT="$HOME/.pyenv"
    export PATH="$PYENV_ROOT/bin:$PATH"

    local py_version=$("$HOME/.pyenv/versions/3.14.0/bin/python" --version | awk '{print $2}')
    echo -e "\033[1;32m✓ Python $py_version installed via pyenv\033[0m"
    echo "  Note: The setup script will use the pyenv Python for this session."
    echo "  To make it permanent, add the pyenv init to your shell profile."
    return 0
}

_generate_protocol() {
    echo ""
    echo "Generating protocol artifacts..."
    cd "$G8E_PROJECT_ROOT"

    if ! make generate; then
        echo -e "\033[0;31m✗ Protocol generation failed\033[0m"
        return 1
    fi

    echo -e "\033[1;32m✓ Protocol artifacts generated\033[0m"
    return 0
}

_setup_python_venv() {
    echo ""
    echo "Setting up Python venv for g8ee..."
    cd "$G8E_PROJECT_ROOT"

    # Determine which Python to use
    local python_cmd="python3"
    if [[ -f "$HOME/.pyenv/versions/3.14.0/bin/python" ]]; then
        python_cmd="$HOME/.pyenv/versions/3.14.0/bin/python"
        echo "Using pyenv Python 3.14.0"
    fi

    if [[ ! -d ".venv" ]]; then
        "$python_cmd" -m venv .venv
        echo -e "\033[1;32m✓ Python venv created\033[0m"
    else
        echo -e "\033[1;32m✓ Python venv already exists\033[0m"
    fi

    source .venv/bin/activate

    if ! pip install --upgrade pip; then
        echo -e "\033[0;31m✗ Failed to upgrade pip\033[0m"
        return 1
    fi

    cd services/g8ee
    if ! pip install -r requirements.txt; then
        echo -e "\033[0;31m✗ Failed to install g8ee dependencies\033[0m"
        return 1
    fi
    if ! pip install -e .; then
        echo -e "\033[0;31m✗ Failed to install g8ee package\033[0m"
        return 1
    fi

    cd "$G8E_PROJECT_ROOT"
    echo -e "\033[1;32m✓ Python dependencies installed\033[0m"
    return 0
}

_build_services() {
    echo ""
    echo "Building g8e services..."
    cd "$G8E_PROJECT_ROOT"

    if ! make build-g8eo; then
        echo -e "\033[0;31m✗ Failed to build g8eo\033[0m"
        return 1
    fi
    echo -e "\033[1;32m✓ g8eo built\033[0m"

    if ! make build-g8eg; then
        echo -e "\033[0;31m✗ Failed to build g8eg\033[0m"
        return 1
    fi
    echo -e "\033[1;32m✓ g8eg built\033[0m"

    return 0
}

_quick_start() {
    _banner
    echo "Quick Start Options:"
    echo "1) Protocol + Local Operator (Gateway Mode)"
    echo "   - Lightest possible setup."
    echo "   - Ideal for BYO clients, MCP/A2A translators, and custom agents."
    echo "   - Runs the Governance Gateway (g8eg) in Gateway mode."
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
            echo ""
            echo "=== Setting up Protocol + Local Operator (Gateway Mode) ==="
            echo ""

            _check_go || return 1
            _generate_protocol || return 1
            _build_services || return 1

            _update_env "G8E_WITH_G8EE=0"
            echo -e "\033[1;32m✓ Environment configured for Gateway Mode\033[0m"
            echo ""
            echo "Next steps:"
            echo "1. Run './g8e platform start'"
            echo "2. Run './g8e login' to authenticate"
            echo ""
            echo "Your development environment is ready!"
            ;;
        2)
            echo ""
            echo "=== Setting up Protocol + Local Operator + g8e-Compliant Agentic Ensemble ==="
            echo ""

            _check_go || return 1
            _check_python || return 1
            _setup_python_venv || return 1
            _generate_protocol || return 1
            _build_services || return 1

            _update_env "G8E_WITH_G8EE=1"
            echo -e "\033[1;32m✓ Environment configured for Full Mode\033[0m"
            echo ""
            echo "Next steps:"
            echo "1. Run './g8e platform start'"
            echo "2. Run './g8e apps start g8ee'"
            echo "3. Open https://localhost:8443 in your browser (Reference UI)"
            echo ""
            echo "Your development environment is ready!"
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
    read -p "Configure AWS? [y/N]: " conf_aws
    if [[ "$conf_aws" =~ ^[Yy]$ ]]; then
        bash "$G8E_PROJECT_ROOT/scripts/tools/setup-aws.sh" 2>/dev/null || echo "AWS setup script not found. Run './g8e aws setup' instead."
    fi

    echo ""
    read -p "Configure MCP servers? [y/N]: " conf_mcp
    if [[ "$conf_mcp" =~ ^[Yy]$ ]]; then
        bash "$G8E_PROJECT_ROOT/scripts/tools/setup-mcp.sh" 2>/dev/null || echo "MCP setup script not found. Run './g8e mcp config' instead."
    fi

    echo ""
    echo -e "\033[1;32m✓ Advanced setup complete.\033[0m"
}

_complete_setup() {
    _banner
    echo "Complete Developer Setup:"
    echo "This will run all checks and setup steps for a full development environment."
    echo ""

    echo "=== Step 1: Check Dependencies ==="
    _check_go || return 1
    _check_python || return 1

    echo ""
    echo "=== Step 2: Setup Python Environment ==="
    _setup_python_venv || return 1

    echo ""
    echo "=== Step 3: Generate Protocol Artifacts ==="
    _generate_protocol || return 1

    echo ""
    echo "=== Step 4: Build Services ==="
    _build_services || return 1

    echo ""
    echo "=== Step 5: Configure Environment ==="
    read -p "Enable g8ee (Agentic Ensemble)? [y/N]: " enable_apps
    if [[ "$enable_apps" =~ ^[Yy]$ ]]; then
        _update_env "G8E_WITH_G8EE=1"
        echo -e "\033[1;32m✓ g8ee enabled\033[0m"
    else
        _update_env "G8E_WITH_G8EE=0"
        echo -e "\033[1;32m✓ Gateway mode configured\033[0m"
    fi

    echo ""
    echo "=== Step 6: Verify Setup ==="
    echo "Running substrate tests to verify installation..."
    cd "$G8E_PROJECT_ROOT"
    if ./g8e test g8eo -- services/pubsub >/dev/null 2>&1; then
        echo -e "\033[1;32m✓ Substrate tests passed\033[0m"
    else
        echo -e "\033[0;33m⚠ Substrate tests skipped or failed (may be expected if no mTLS certs)\033[0m"
    fi

    echo ""
    echo -e "\033[1;32m✓ Complete setup finished!\033[0m"
    echo ""
    echo "Next steps:"
    echo "1. Run './g8e platform start'"
    echo "2. Run './g8e login' to authenticate"
    if [[ "$enable_apps" =~ ^[Yy]$ ]]; then
        echo "3. Run './g8e apps start g8ee'"
        echo "4. Open https://localhost:8443 in your browser"
    fi
    echo ""
    echo "For development commands, run './g8e --help'"
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
elif [[ $# -gt 0 && ("$1" == "--complete" || "$1" == "complete") ]]; then
    _complete_setup
else
    while true; do
        _banner
        echo "1) Quick Start"
        echo "2) Advanced Setup"
        echo "3) Complete Developer Setup"
        echo "0) Back"
        echo ""
        read -p "Choose an option [0-3]: " opt
        case $opt in
            1) _quick_start ;;
            2) _advanced_setup ;;
            3) _complete_setup ;;
            0) exit 0 ;;
            *) echo "Invalid option"; sleep 1 ;;
        esac
    done
fi
