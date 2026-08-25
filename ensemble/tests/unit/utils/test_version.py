# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
g8ee Version utility tests.

Following g8ee testing patterns:
- Test REAL version parsing logic
- Test file path resolution
- Test fallback behavior
"""

import pytest

from app.utils.version import VersionInfo, get_version, get_version_info

pytestmark = pytest.mark.unit


class TestVersionUtility:
    """Test version utility functions."""

    def test_get_version_returns_string(self):
        """Test get_version returns a non-empty string."""
        get_version.cache_clear()

        result = get_version()

        assert isinstance(result, str)
        assert len(result) > 0

    def test_get_version_starts_with_v(self):
        """Test get_version returns a semver string starting with v."""
        get_version.cache_clear()

        result = get_version()

        assert result.startswith("v")

    def test_get_version_caching(self):
        """Test get_version uses lru_cache for performance."""
        get_version.cache_clear()

        result1 = get_version()
        result2 = get_version()

        assert result1 == result2

        cache_info = get_version.cache_info()
        assert cache_info.hits >= 1

    def test_get_version_info_returns_version_info(self):
        """Test get_version_info returns a VersionInfo model."""
        get_version_info.cache_clear()

        result = get_version_info()

        assert isinstance(result, VersionInfo)
        assert result.version

    def test_get_version_info_caching(self):
        """Test get_version_info uses lru_cache."""
        get_version_info.cache_clear()

        result1 = get_version_info()
        result2 = get_version_info()

        assert result1 == result2

        cache_info = get_version_info.cache_info()
        assert cache_info.hits >= 1

    def test_version_info_consistency(self):
        """Test version_info.version matches get_version()."""
        get_version.cache_clear()
        get_version_info.cache_clear()

        version = get_version()
        info = get_version_info()

        assert info.version == version
