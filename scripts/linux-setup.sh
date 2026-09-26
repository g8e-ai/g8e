#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# g8e Linux dev setup: validate toolchain, build evaluation-explorer, make build,
# and add the repository root to PATH.
# See docs/architecture/scripts.md and docs/guides/getting_started.md

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/dev-setup-common.sh
source "${SCRIPT_DIR}/lib/dev-setup-common.sh"

g8e_linux_install_make() {
    if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update && sudo apt-get install -y make
        return 0
    fi
    if command -v dnf >/dev/null 2>&1; then
        sudo dnf install -y make
        return 0
    fi
    if command -v pacman >/dev/null 2>&1; then
        sudo pacman -S --noconfirm make
        return 0
    fi
    if command -v zypper >/dev/null 2>&1; then
        sudo zypper install -y make
        return 0
    fi
    return 1
}

g8e_linux_install_go() {
    if command -v snap >/dev/null 2>&1; then
        echo "Installing Go via snap (recommended on Linux; distro golang packages are often too old)..."
        sudo snap install go --classic
        return 0
    fi
    return 1
}

g8e_linux_install_node() {
    if command -v snap >/dev/null 2>&1; then
        echo "Installing Node.js via snap..."
        sudo snap install node --classic --channel=22
        return 0
    fi
    if command -v apt-get >/dev/null 2>&1; then
        echo "Installing Node.js via apt (verify the version is >= ${G8E_NODE_MIN_MAJOR} after install)..."
        sudo apt-get update && sudo apt-get install -y nodejs npm
        return 0
    fi
    if command -v dnf >/dev/null 2>&1; then
        sudo dnf install -y nodejs npm
        return 0
    fi
    return 1
}

g8e_linux_install_git() {
    if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update && sudo apt-get install -y git
        return 0
    fi
    if command -v dnf >/dev/null 2>&1; then
        sudo dnf install -y git
        return 0
    fi
    if command -v pacman >/dev/null 2>&1; then
        sudo pacman -S --noconfirm git
        return 0
    fi
    if command -v zypper >/dev/null 2>&1; then
        sudo zypper install -y git
        return 0
    fi
    return 1
}

g8e_linux_install_missing() {
    local missing=("$@")
    local item

    for item in "${missing[@]}"; do
        case "$item" in
            make)
                if g8e_setup_confirm "Install make now? [y/N] "; then
                    g8e_linux_install_make || echo "  could not install make automatically"
                fi
                ;;
            git)
                if g8e_setup_confirm "Install git now? [y/N] "; then
                    g8e_linux_install_git || echo "  could not install git automatically"
                fi
                ;;
            go)
                echo "Go must satisfy go.mod (currently $G8E_GO_MIN)."
                echo "  Preferred: install from https://go.dev/dl/"
                if g8e_setup_confirm "Try snap install go --classic now? [y/N] "; then
                    g8e_linux_install_go || echo "  snap install failed; install Go manually from https://go.dev/dl/"
                fi
                ;;
            node)
                echo "Node.js ${G8E_NODE_MIN_MAJOR}+ is required to build the embedded evaluation explorer."
                if g8e_setup_confirm "Attempt automatic Node.js install now? [y/N] "; then
                    g8e_linux_install_node || echo "  could not install Node.js automatically; install Node ${G8E_NODE_MIN_MAJOR}+ manually"
                fi
                ;;
        esac
    done
}

g8e_setup_init "$@"

echo -e "\n[SETUP] g8e Linux dev environment setup\n"
echo "[STEP 1/4] Checking prerequisites (git, make, go >= ${G8E_GO_MIN}, node >= ${G8E_NODE_MIN_MAJOR})..."

MISSING=()
g8e_check_git || MISSING+=("git")
g8e_check_make || MISSING+=("make")
g8e_check_go || MISSING+=("go")
g8e_check_node || MISSING+=("node")

if [[ ${#MISSING[@]} -gt 0 ]]; then
    echo
    echo "Missing or outdated tooling: ${MISSING[*]}"
    g8e_linux_install_missing "${MISSING[@]}"
    echo
    echo "Re-checking prerequisites..."
    MISSING=()
    g8e_check_git || MISSING+=("git")
    g8e_check_make || MISSING+=("make")
    g8e_check_go || MISSING+=("go")
    g8e_check_node || MISSING+=("node")
    if [[ ${#MISSING[@]} -gt 0 ]]; then
        echo "FATAL: still missing: ${MISSING[*]}"
        echo "Install the remaining tools, then rerun: bash scripts/linux-setup.sh"
        exit 1
    fi
fi

echo -e "\n[STEP 2/4] Building evaluation-explorer assets..."
g8e_build_evaluation_explorer

echo -e "\n[STEP 3/4] Building g8e..."
g8e_run_make_build
echo "Build successful."

echo -e "\n[STEP 4/4] Adding repository root to PATH..."
g8e_configure_path_unix
g8e_print_next_steps_unix
