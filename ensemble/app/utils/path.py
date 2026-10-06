# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from pathlib import Path


def resolve_project_root() -> Path:
    """
    Resolves the project root directory.
    """
    root = Path(__file__).resolve().parent.parent.parent
    if (root.parent / ".g8e").exists() or (root.parent / "go.mod").exists():
        return root.parent
    return root
