# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Pytest configuration for evals tests.

Note: g8eo binary acquisition fixtures have been removed.
If you need the g8eo binary for evals, please acquire it manually from GitHub releases:
https://github.com/g8e-ai/g8e/releases
"""
import os
import sys
from pathlib import Path

import pytest

# Set up environment for tests that import from g8ee
G8E_ROOT = Path(__file__).parent.parent.parent
