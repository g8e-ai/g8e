# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
Version utility - reads VERSION file.
The VERSION file at the component root contains the platform semver (e.g., v0.1.3).
"""

from functools import lru_cache
from pathlib import Path

from app.models.version import VersionInfo

__all__ = ["VersionInfo", "get_version", "get_version_info"]


@lru_cache(maxsize=1)
def get_version() -> str:
    """Get the version from the repo root VERSION file.

    Returns:
        Semver version string
    """
    version_path = Path(__file__).parent.parent.parent.parent / "VERSION"
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
