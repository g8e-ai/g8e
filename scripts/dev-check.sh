#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Preflight for `make ci`: reports every missing or mismatched tool in one pass
# instead of letting the pipeline die on the first one. Run via `make dev-check`;
# the ci targets depend on it. See docs/architecture/scripts.md.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/dev-setup-common.sh
source "${SCRIPT_DIR}/lib/dev-setup-common.sh"

g8e_setup_init

# When invoked from make, PATH already carries the Go bin dir and ~/.local/bin.
# When run directly, add them so a fresh install is found before the profile is
# sourced.
GO_BIN_DIR="$(go env GOBIN 2>/dev/null)"
if [[ -z "$GO_BIN_DIR" ]] && command -v go >/dev/null 2>&1; then
    GO_BIN_DIR="$(go env GOPATH)/bin"
fi
export PATH="${GO_BIN_DIR:-$HOME/go/bin}:$HOME/.local/bin:$PATH"

FAILED=0

echo -e "\n[DEV-CHECK] Prerequisites (git, make, curl, go >= ${G8E_GO_MIN}, node >= ${G8E_NODE_MIN_MAJOR}, python3, uv, rg, bc, C compiler)"
g8e_collect_missing
if [[ ${#MISSING[@]} -gt 0 ]]; then
    FAILED=1
fi

echo -e "\n[DEV-CHECK] Go dev tools"
g8e_check_go_tools || FAILED=1

echo -e "\n[DEV-CHECK] Python environment (.venv)"
g8e_check_python_env || FAILED=1

echo -e "\n[DEV-CHECK] Node dependencies"
g8e_check_node_deps || FAILED=1

echo
if [[ "$FAILED" -ne 0 ]]; then
    case "$(uname -s)" in
        Darwin) setup="bash scripts/macos-setup.sh" ;;
        Linux) setup="bash scripts/linux-setup.sh" ;;
        *) setup="pwsh scripts/windows-setup.ps1 (or run linux-setup.sh inside WSL)" ;;
    esac
    echo "FATAL: the toolchain 'make ci' needs is incomplete."
    echo "Fix everything above in one step with: $setup"
    echo "(Go tools, the Python venv, and Node deps alone: make dev-setup)"
    exit 1
fi
echo "[DEV-CHECK] OK: everything 'make ci' needs is installed."
