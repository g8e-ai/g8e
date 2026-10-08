#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# g8e macOS dev setup: install the toolchain, build evaluation-explorer, make
# build, install the contributor toolchain that `make ci` needs (Go dev tools,
# Python venv, Node deps), and add the repository root to PATH.
# Pass --build-only to stop after `make build`.
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
    local item

    for item in "${missing[@]}"; do
        case "$item" in
            git) brew_packages+=("git") ;;
            make) brew_packages+=("make") ;;
            go) brew_packages+=("go") ;;
            node) brew_packages+=("node") ;;
            python3) brew_packages+=("python") ;;
            rg) brew_packages+=("ripgrep") ;;
            bc) brew_packages+=("bc") ;;
            cc)
                echo "A C compiler is required by the Go race detector. Install the Xcode Command Line Tools:"
                echo "  xcode-select --install"
                echo "Finish that installer, then rerun: bash scripts/macos-setup.sh"
                ;;
            curl) echo "curl ships with macOS; check that /usr/bin is on PATH." ;;
        esac
    done

    if [[ ${#brew_packages[@]} -gt 0 ]]; then
        echo "They can be installed via: brew install ${brew_packages[*]}"
        if g8e_setup_confirm "Install with Homebrew now? [y/N] "; then
            g8e_macos_brew_install "${brew_packages[@]}"
        fi
    fi

    for item in "${missing[@]}"; do
        if [[ "$item" == "uv" ]]; then
            if g8e_setup_confirm "Install uv ${G8E_UV_VERSION} to ~/.local/bin now? [y/N] "; then
                g8e_install_uv || echo "  uv install failed; see https://docs.astral.sh/uv/getting-started/installation/"
            fi
        fi
    done
}

g8e_setup_init "$@"

if [[ "$G8E_BUILD_ONLY" == true ]]; then
    TOTAL_STEPS=4
    PREREQS="git, make, curl, go >= ${G8E_GO_MIN}, node >= ${G8E_NODE_MIN_MAJOR}"
else
    TOTAL_STEPS=6
    PREREQS="git, make, curl, go >= ${G8E_GO_MIN}, node >= ${G8E_NODE_MIN_MAJOR}, python3, uv, rg, bc, C compiler"
fi

echo -e "\n[SETUP] g8e macOS dev environment setup\n"
echo "[STEP 1/${TOTAL_STEPS}] Checking prerequisites (${PREREQS})..."

g8e_collect_missing

if [[ ${#MISSING[@]} -gt 0 ]]; then
    echo
    echo "Missing or outdated tooling: ${MISSING[*]}"
    g8e_macos_install_missing "${MISSING[@]}"
    echo
    echo "Re-checking prerequisites..."
    g8e_collect_missing
    if [[ ${#MISSING[@]} -gt 0 ]]; then
        echo "FATAL: still missing: ${MISSING[*]}"
        echo "Install the remaining tools, then rerun: bash scripts/macos-setup.sh"
        exit 1
    fi
fi

echo -e "\n[STEP 2/${TOTAL_STEPS}] Building evaluation-explorer assets..."
g8e_build_evaluation_explorer

echo -e "\n[STEP 3/${TOTAL_STEPS}] Building g8e..."
g8e_run_make_build
echo "Build successful."

if [[ "$G8E_BUILD_ONLY" == true ]]; then
    echo -e "\n[STEP 4/${TOTAL_STEPS}] Adding repository root to PATH..."
    g8e_configure_path_unix
    g8e_print_next_steps_unix
    exit 0
fi

echo -e "\n[STEP 4/${TOTAL_STEPS}] Installing the contributor toolchain (Go dev tools, Python venv, Node deps)..."
g8e_run_dev_setup

echo -e "\n[STEP 5/${TOTAL_STEPS}] Adding repository root and dev tool directories to PATH..."
g8e_configure_path_unix

echo -e "\n[STEP 6/${TOTAL_STEPS}] Verifying the toolchain (make dev-check)..."
make dev-check
g8e_print_next_steps_unix

# Keep the checkout's g8e binary on PATH for the remainder of this shell.
case ":$PATH:" in
    *":$PWD:"*) ;;
    *) export PATH="$PATH:$PWD" ;;
esac
