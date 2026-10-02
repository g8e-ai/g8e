#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# g8e Linux dev setup: install the toolchain, build evaluation-explorer, make
# build, install the contributor toolchain that `make ci` needs (Go dev tools,
# Python venv, Node deps), and add the repository root to PATH.
# Pass --build-only to stop after `make build`.
# See docs/architecture/scripts.md and docs/guides/getting_started.md

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/dev-setup-common.sh
source "${SCRIPT_DIR}/lib/dev-setup-common.sh"

G8E_PKG=""
SUDO=""

g8e_linux_detect_pkg_manager() {
    local pm
    for pm in apt-get dnf pacman zypper; do
        if command -v "$pm" >/dev/null 2>&1; then
            G8E_PKG="${pm%-get}"
            return 0
        fi
    done
    return 1
}

# Sets SUDO to "" for root and "sudo" otherwise; fails when neither applies.
g8e_linux_init_sudo() {
    if [[ "$(id -u)" -eq 0 ]]; then
        SUDO=""
        return 0
    fi
    if command -v sudo >/dev/null 2>&1; then
        SUDO="sudo"
        return 0
    fi
    return 1
}

# Maps a logical prerequisite name to the package that provides it.
g8e_linux_pkg_name() {
    case "$G8E_PKG:$1" in
        apt:cc) echo "build-essential" ;;
        dnf:cc|zypper:cc) echo "gcc" ;;
        pacman:cc) echo "base-devel" ;;
        pacman:python3) echo "python" ;;
        *:rg) echo "ripgrep" ;;
        *) echo "$1" ;;
    esac
}

g8e_linux_pkg_install() {
    case "$G8E_PKG" in
        apt) $SUDO apt-get update && $SUDO apt-get install -y "$@" ;;
        dnf) $SUDO dnf install -y "$@" ;;
        pacman) $SUDO pacman -S --noconfirm --needed "$@" ;;
        zypper) $SUDO zypper install -y "$@" ;;
        *) return 1 ;;
    esac
}

# Installs the official Go tarball matching go.mod into /usr/local/go. Works on
# every distro and on WSL; the snap alternative needs a running snapd, which WSL
# often lacks, and distro golang packages are usually too old.
g8e_linux_install_go() {
    local goarch tarball tmp want got
    case "$(uname -m)" in
        x86_64) goarch="amd64" ;;
        aarch64|arm64) goarch="arm64" ;;
        *) echo "  no official Go tarball flow for $(uname -m); install Go from https://go.dev/dl/"; return 1 ;;
    esac
    tarball="go${G8E_GO_MIN}.linux-${goarch}.tar.gz"
    tmp="$(mktemp "${TMPDIR:-/tmp}/g8e-go.XXXXXX")"

    echo "Downloading ${tarball}..."
    if ! curl -fSL "https://go.dev/dl/${tarball}" -o "$tmp" ||
        ! want="$(curl -fsSL "https://dl.google.com/go/${tarball}.sha256")"; then
        rm -f "$tmp"
        return 1
    fi
    got="$(g8e_sha256 "$tmp")"
    if [[ "$got" != "$want" ]]; then
        echo "  checksum mismatch for ${tarball} (expected ${want}, got ${got})"
        rm -f "$tmp"
        return 1
    fi

    echo "Installing Go ${G8E_GO_MIN} to /usr/local/go (replaces any existing /usr/local/go)..."
    $SUDO rm -rf /usr/local/go
    $SUDO tar -C /usr/local -xzf "$tmp"
    rm -f "$tmp"
    # Ahead of any older distro Go (e.g. /usr/bin/go) so the re-check sees this one.
    export PATH="/usr/local/go/bin:$PATH"
    hash -r
}

# Installs the newest Node ${G8E_NODE_MIN_MAJOR}.x LTS tarball from nodejs.org
# into /usr/local/lib/nodejs and links node/npm/npx into /usr/local/bin.
# Ubuntu's apt nodejs is older than the required major.
g8e_linux_install_node() {
    local nodearch sums tarball tmp want got dest
    case "$(uname -m)" in
        x86_64) nodearch="x64" ;;
        aarch64|arm64) nodearch="arm64" ;;
        *) echo "  no official Node.js tarball flow for $(uname -m); install Node ${G8E_NODE_MIN_MAJOR}+ from https://nodejs.org/"; return 1 ;;
    esac
    local base="https://nodejs.org/dist/latest-v${G8E_NODE_MIN_MAJOR}.x"
    sums="$(curl -fsSL "${base}/SHASUMS256.txt")" || return 1
    tarball="$(echo "$sums" | awk -v arch="linux-${nodearch}.tar.gz" '$2 ~ arch"$" {print $2; exit}')"
    want="$(echo "$sums" | awk -v t="$tarball" '$2 == t {print $1}')"
    if [[ -z "$tarball" || -z "$want" ]]; then
        echo "  could not find a Node ${G8E_NODE_MIN_MAJOR}.x linux-${nodearch} tarball in ${base}/SHASUMS256.txt"
        return 1
    fi

    tmp="$(mktemp "${TMPDIR:-/tmp}/g8e-node.XXXXXX")"
    echo "Downloading ${tarball}..."
    if ! curl -fSL "${base}/${tarball}" -o "$tmp"; then
        rm -f "$tmp"
        return 1
    fi
    got="$(g8e_sha256 "$tmp")"
    if [[ "$got" != "$want" ]]; then
        echo "  checksum mismatch for ${tarball} (expected ${want}, got ${got})"
        rm -f "$tmp"
        return 1
    fi

    dest="/usr/local/lib/nodejs"
    echo "Installing ${tarball%.tar.gz} to ${dest}..."
    $SUDO mkdir -p "$dest"
    $SUDO tar -C "$dest" -xzf "$tmp"
    rm -f "$tmp"
    for bin in node npm npx corepack; do
        $SUDO ln -sf "${dest}/${tarball%.tar.gz}/bin/${bin}" "/usr/local/bin/${bin}"
    done
    hash -r
}

g8e_linux_install_missing() {
    local item
    local -a pkgs
    pkgs=()

    for item in "$@"; do
        case "$item" in
            git|make|curl|python3|rg|bc|cc) pkgs+=("$(g8e_linux_pkg_name "$item")") ;;
        esac
    done

    if [[ ${#pkgs[@]} -gt 0 ]]; then
        if ! g8e_linux_detect_pkg_manager; then
            echo "No supported package manager (apt, dnf, pacman, zypper) found."
            echo "Install manually: ${pkgs[*]}"
        elif ! g8e_linux_init_sudo; then
            echo "Need root to install packages but 'sudo' is not available."
            echo "As root, run: ${G8E_PKG} install ${pkgs[*]}"
        elif g8e_setup_confirm "Install ${pkgs[*]} with ${G8E_PKG} now? [y/N] "; then
            g8e_linux_pkg_install "${pkgs[@]}" || echo "  package install failed; install ${pkgs[*]} manually"
        fi
    fi

    for item in "$@"; do
        case "$item" in
            go)
                echo "Go must satisfy go.mod (currently $G8E_GO_MIN)."
                if g8e_linux_init_sudo && g8e_have curl &&
                    g8e_setup_confirm "Download the official Go ${G8E_GO_MIN} tarball to /usr/local/go now? [y/N] "; then
                    g8e_linux_install_go || echo "  Go install failed; install it manually from https://go.dev/dl/"
                else
                    echo "  Install Go ${G8E_GO_MIN}+ from https://go.dev/dl/ and rerun this script."
                fi
                ;;
            node)
                echo "Node.js ${G8E_NODE_MIN_MAJOR}+ is required (evaluation explorer, dashboard, protocol/node)."
                if g8e_linux_init_sudo && g8e_have curl &&
                    g8e_setup_confirm "Download the official Node.js ${G8E_NODE_MIN_MAJOR}.x LTS tarball to /usr/local/lib/nodejs now? [y/N] "; then
                    g8e_linux_install_node || echo "  Node.js install failed; install Node ${G8E_NODE_MIN_MAJOR}+ manually from https://nodejs.org/"
                else
                    echo "  Install Node.js ${G8E_NODE_MIN_MAJOR}+ from https://nodejs.org/ and rerun this script."
                fi
                ;;
            uv)
                if g8e_have curl && g8e_setup_confirm "Install uv ${G8E_UV_VERSION} to ~/.local/bin now? [y/N] "; then
                    g8e_install_uv || echo "  uv install failed; see https://docs.astral.sh/uv/getting-started/installation/"
                else
                    echo "  Install uv ${G8E_UV_VERSION} (https://docs.astral.sh/uv/) and rerun this script."
                fi
                ;;
        esac
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

echo -e "\n[SETUP] g8e Linux dev environment setup\n"
echo "[STEP 1/${TOTAL_STEPS}] Checking prerequisites (${PREREQS})..."

g8e_collect_missing

if [[ ${#MISSING[@]} -gt 0 ]]; then
    echo
    echo "Missing or outdated tooling: ${MISSING[*]}"
    g8e_linux_install_missing "${MISSING[@]}"
    echo
    echo "Re-checking prerequisites..."
    g8e_collect_missing
    if [[ ${#MISSING[@]} -gt 0 ]]; then
        echo "FATAL: still missing: ${MISSING[*]}"
        echo "Install the remaining tools, then rerun: bash scripts/linux-setup.sh"
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
