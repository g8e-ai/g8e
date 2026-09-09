# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version  2.0.

"""Tier 1 cross-cutting validation of the typed conformance case registry.

Verifies that every required (grader, category) pair in the inventory has at
least one registered executable case, that no case is registered for an
excluded or unknown category, and that no case targets a partial-external
grader. These checks are independent of the per-suite case execution tests.
"""

from __future__ import annotations

import pytest

from g8e_evals.grader_inventory import (
    GRADER_INVENTORY,
    ProducerPath,
)

from .conformance.registry import ALL_CASES

pytestmark = pytest.mark.unit


def test_every_required_grader_category_pair_has_a_conformance_case() -> None:
    covered: set[tuple[str, str]] = {
        (c.grader_id, c.category.value) for c in ALL_CASES
    }
    gaps: list[str] = []
    for entry in GRADER_INVENTORY.values():
        if entry.producer_path == ProducerPath.PARTIAL_EXTERNAL:
            continue
        for category in entry.required_categories():
            if (entry.grader_id, category.value) not in covered:
                gaps.append(f"{entry.grader_id}:{category.value}")
    assert not gaps, (
        f"missing conformance cases for required (grader, category) pairs:\n{gaps}"
    )


def test_no_conformance_case_targets_an_excluded_category() -> None:
    for case in ALL_CASES:
        entry = GRADER_INVENTORY.get((case.grader_id, "1.0.0"))
        assert entry is not None, f"case references unknown grader: {case.grader_id}"
        assert entry.producer_path != ProducerPath.PARTIAL_EXTERNAL, (
            f"case registered for partial-external grader: {case.grader_id}"
        )
        assert not entry.has_exclusion(case.category), (
            f"case registered for excluded category "
            f"{case.grader_id}:{case.category.value}"
        )


def test_conformance_case_registry_summary_has_zero_gaps() -> None:
    covered: set[tuple[str, str]] = {
        (c.grader_id, c.category.value) for c in ALL_CASES
    }
    lines: list[str] = []
    gap_count = 0
    for entry in GRADER_INVENTORY.values():
        if entry.producer_path == ProducerPath.PARTIAL_EXTERNAL:
            continue
        required = entry.required_categories()
        missing = sorted(c.value for c in required if (entry.grader_id, c.value) not in covered)
        if missing:
            gap_count += len(missing)
            lines.append(f"  {entry.grader_id}: missing {missing}")
    summary = (
        f"conformance matrix: {len(ALL_CASES)} cases, "
        f"{len(GRADER_INVENTORY)} graders, {gap_count} gaps"
    )
    assert gap_count == 0, summary + "\n" + "\n".join(lines)
