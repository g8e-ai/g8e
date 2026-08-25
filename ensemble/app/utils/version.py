# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Version utility - reads VERSION file.
The VERSION file at the component root contains the platform semver (e.g., v0.1.3).
"""

from functools import lru_cache

from app.models.version import VersionInfo
from app.utils.path import resolve_project_root

__all__ = ["VersionInfo", "get_version", "get_version_info"]


@lru_cache(maxsize=1)
def get_version() -> str:
    """Get the version from the repo root VERSION file.

    Returns:
        Semver version string
    """
    version_path = resolve_project_root() / "VERSION"
    if version_path.exists():
        return version_path.read_text().strip()
    return "v0.0.0"


@lru_cache(maxsize=1)
def get_version_info() -> VersionInfo:
    """Get full version info object.

    Returns:
        VersionInfo with version string
    """
    return VersionInfo(version=get_version())
