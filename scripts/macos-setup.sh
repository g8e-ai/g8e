#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# g8e macOS dev setup: validate toolchain, build evaluation-explorer, make build,
# and add the repository root to PATH.
# See docs/architecture/scripts.md and docs/guides/getting_started.md

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/dev-setup-common.sh
source "${SCRIPT_DIR}/lib/dev-setup-common.sh"

g8e_macos_brew_install() {
    local packages=("$@")
    if ! command -v brew >/dev/null 2>&1; then
        echo "FATAL: Homebrew is required on macOS. Install it from https://brew.sh"
        exit 1
    fi
    brew install "${packages[@]}"
}

g8e_macos_install_missing() {
    local missing=("$@")
    local brew_packages=()

    for item in "${missing[@]}"; do
        case "$item" in
            git) brew_packages+=("git") ;;
            make) brew_packages+=("make") ;;
            go) brew_packages+=("go") ;;
            node) brew_packages+=("node") ;;
        esac
    done

    if [[ ${#brew_packages[@]} -eq 0 ]]; then
        return 0
    fi

    echo "They can be installed via: brew install ${brew_packages[*]}"
    if g8e_setup_confirm "Install with Homebrew now? [y/N] "; then
        g8e_macos_brew_install "${brew_packages[@]}"
    fi
}

g8e_setup_init "$@"

echo -e "\n[SETUP] g8e macOS dev environment setup\n"
echo "[STEP 1/4] Checking prerequisites (git, make, go >= ${G8E_GO_MIN}, node >= ${G8E_NODE_MIN_MAJOR})..."

MISSING=()
g8e_check_git || MISSING+=("git")
g8e_check_make || MISSING+=("make")
g8e_check_go || MISSING+=("go")
g8e_check_node || MISSING+=("node")

if [[ ${#MISSING[@]} -gt 0 ]]; then
    echo
    echo "Missing or outdated tooling: ${MISSING[*]}"
    g8e_macos_install_missing "${MISSING[@]}"
    echo
    echo "Re-checking prerequisites..."
    MISSING=()
    g8e_check_git || MISSING+=("git")
    g8e_check_make || MISSING+=("make")
    g8e_check_go || MISSING+=("go")
    g8e_check_node || MISSING+=("node")
    if [[ ${#MISSING[@]} -gt 0 ]]; then
        echo "FATAL: still missing: ${MISSING[*]}"
        echo "Install the remaining tools, then rerun: bash scripts/macos-setup.sh"
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
