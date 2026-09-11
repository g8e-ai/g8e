# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Package-root pytest configuration for the standalone eval package.

Adds the repository root to ``sys.path`` so that cross-package imports
such as ``scripts.generate_readme`` (the offline README generator) are
resolvable from the evals test environment.  The evals package is
self-contained for its own domain models, but the v5 README loading
regression test imports the standard-library-only generator to verify
that v5 publication artifacts load and render strictly.
"""

import sys
from pathlib import Path

_REPO_ROOT = str(Path(__file__).resolve().parent.parent.parent)
if _REPO_ROOT not in sys.path:
    sys.path.insert(0, _REPO_ROOT)
