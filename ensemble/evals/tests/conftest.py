# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Shared pytest configuration for the standalone eval package."""

import pytest


TIER_MARKERS = ("unit", "integration", "e2e")


def pytest_collection_modifyitems(items: list[pytest.Item]) -> None:
    for item in items:
        tiers = [marker for marker in TIER_MARKERS if item.get_closest_marker(marker)]
        if len(tiers) != 1:
            raise pytest.UsageError(f"{item.nodeid} must declare exactly one eval test tier")
