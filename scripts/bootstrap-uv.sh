#!/usr/bin/env bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Used by make ensemble-env / dev-python; no Go tools or OS packages installed.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/dev-setup-common.sh"
cd "${SCRIPT_DIR}/.."
export PATH="$HOME/.local/bin:$PATH"
if command -v uv >/dev/null 2>&1; then
    exit 0
fi
G8E_UV_VERSION="$(g8e_make_var UV_VERSION)"
if [[ -z "$G8E_UV_VERSION" ]]; then
    echo "ERROR: UV_VERSION is missing from the Makefile" >&2
    exit 1
fi
if ! g8e_install_uv || ! command -v uv >/dev/null 2>&1; then
    echo "ERROR: could not install uv; install it manually and retry: https://docs.astral.sh/uv/" >&2
    exit 1
fi
