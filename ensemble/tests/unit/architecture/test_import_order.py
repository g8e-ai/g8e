# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Every entry point into the app.errors / app.models / app.llm import cycle must load first.

app.errors imports app.models.errors, which runs app/models/__init__.py, which
imports app.llm and its providers. A provider that imports app.errors at module
level therefore fails with a partially initialized module, but only when
app.errors is the first of the three imported. The container entry point
(app.main) is one such order, and in-process tests are not: by the time a test
runs, pytest has already imported app.models. Each order is checked in a fresh
interpreter so the result does not depend on what was imported before.
"""

from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path

import pytest

pytestmark = pytest.mark.unit

ENSEMBLE_ROOT = Path(__file__).resolve().parents[3]


@pytest.mark.parametrize(
    "module",
    [
        "app.errors",
        "app.models",
        "app.llm",
        "app.llm.providers.anthropic",
        "app.main",
    ],
)
def test_module_imports_first_in_a_fresh_interpreter(module: str) -> None:
    result = subprocess.run(
        [sys.executable, "-c", f"import {module}"],
        cwd=ENSEMBLE_ROOT,
        env={**os.environ, "PYTHONPATH": str(ENSEMBLE_ROOT)},
        capture_output=True,
        text=True,
        timeout=120,
        check=False,
    )
    assert result.returncode == 0, f"`import {module}` failed first-import:\n{result.stderr}"
