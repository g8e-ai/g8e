#!/bin/bash
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Smoke test: verify the Python package installs and imports work
# from a clean virtual environment, mirroring the README quickstart.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PY_DIR="$REPO_ROOT/protocol/python"

echo "=== Python smoke test ==="

# Create a clean venv
VENV_DIR="$PY_DIR/.smoke-env"
python3 -m venv "$VENV_DIR"

# Activate venv (POSIX-compatible)
# shellcheck disable=SC1091
source "$VENV_DIR/bin/activate"

pip install --upgrade pip
pip install -e "$PY_DIR"

# Verify imports from README quickstart
python -c "from g8e.constants import EVENTS, STATUS, ComponentName; print('Constants import OK')"
python -c "from g8e.models import RequestContext, PlatformSettings; print('Models import OK')"
python -c "import g8e; print(f'g8e version: {g8e.__version__}')"

# Run example scripts from README
python "$PY_DIR/examples/constants_example.py"
python "$PY_DIR/examples/models_example.py"

# Cleanup
deactivate
rm -rf "$VENV_DIR"

echo "=== Python smoke test PASSED ==="
