# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 conformance coverage analysis for all authoritative graders.

Parses the existing conformance test file and maps test function names
to the required conformance-case categories from the grader inventory.
Verifies that every authoritative grader has at least one conformance
case for each required category (categories not excluded by a typed
exclusion). Reports gaps as test failures so missing conformance cases
are detected at test time rather than at release time.
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest

from g8e_evals.grader_inventory import (
    GRADER_INVENTORY,
    ConformanceCaseCategory,
    ProducerPath,
)

pytestmark = pytest.mark.unit

_TEST_FILE = Path(__file__).parent / "test_authoritative_receipt_grader.py"


def _extract_test_names() -> list[str]:
    """Extract all test function names from the authoritative grader test file."""
    content = _TEST_FILE.read_text()
    return re.findall(r"^def test_(\w+)\(", content, re.MULTILINE)


# Grader ID prefixes that differ from the grader_id in the inventory.
_PREFIX_TO_GRADER_ID: dict[str, str] = {
    "model_boundary": "model_boundary_raw_secret_rate",
    "final_state_assertion": "final_state_assertions",
    "receipt_final_state_observer": "final_state_assertions",
    "secret_detection_precision": "secret_detection_precision",
    "secret_detection_recall": "secret_detection_recall",
    "secret_detection_graders": "secret_detection_precision",  # applies to both
    "secret_detection": "secret_detection_precision",
    "deterministic_grader_registry": "receipt_integrity",  # uses receipt_integrity@2.0.0
}


def _grader_id_from_test_name(test_name: str) -> str | None:
    """Map a test function name to a grader ID."""
    name = test_name.lower()

    for prefix, grader_id in _PREFIX_TO_GRADER_ID.items():
        if name.startswith(prefix):
            return grader_id

    match = re.match(r"^([a-z0-9_]+?)_grader", name)
    if match:
        candidate = match.group(1)
        if (candidate, "1.0.0") in GRADER_INVENTORY:
            return candidate

    return None


def _categorize_test(test_name: str) -> set[ConformanceCaseCategory]:
    """Map a test function name to conformance-case categories."""
    name = test_name.lower()
    categories: set[ConformanceCaseCategory] = set()

    # PASSING_EVIDENCE: tests that verify correct behavior on valid evidence
    if any(kw in name for kw in (
        "verifies", "passes", "aggregates", "measures", "supports_every",
        "supports_ledger", "supports_failed", "supports_rejected", "supports_interrupted",
    )):
        categories.add(ConformanceCaseCategory.PASSING_EVIDENCE)

    # MEASURED_FAILURE: tests that verify the grader measures a real failure
    if any(kw in name for kw in (
        "fails_closed", "fails_when", "partial_failure", "mixed_pass",
        "verified_failure", "reports_failed", "measures_verified_raw_secret_leakage",
        "returns_verified_failure", "fail_closed",
    )):
        categories.add(ConformanceCaseCategory.MEASURED_FAILURE)

    # MISSING_EVIDENCE: tests that verify the grader fails on missing evidence
    if any(kw in name for kw in (
        "missing_assertions", "missing_observation", "missing_primary_receipt",
        "missing_source_evidence", "fails_closed_without_observation",
        "fails_closed_without_unique_verified_receipt", "missing",
    )):
        categories.add(ConformanceCaseCategory.MISSING_EVIDENCE)

    # The "invalid_evidence" parametrize case includes both missing and malformed
    if "invalid_evidence" in name:
        categories.add(ConformanceCaseCategory.MISSING_EVIDENCE)
        categories.add(ConformanceCaseCategory.MALFORMED_EVIDENCE)

    # DUPLICATE_EVIDENCE: tests that verify the grader rejects duplicate evidence
    if "duplicate" in name:
        categories.add(ConformanceCaseCategory.DUPLICATE_EVIDENCE)

    # UNSUPPORTED_VERSION: tests that verify the grader rejects unsupported versions
    if "unsupported_version" in name or "rejects_unsupported" in name:
        categories.add(ConformanceCaseCategory.UNSUPPORTED_VERSION)

    # NOT_APPLICABLE: tests that verify the grader returns NOT_APPLICABLE
    if "not_applic" in name:
        categories.add(ConformanceCaseCategory.NOT_APPLICABLE)

    # WRONG_RUN_BINDING: tests that verify the grader rejects evidence from wrong run
    if "cross_run" in name:
        categories.add(ConformanceCaseCategory.WRONG_RUN_BINDING)

    # WRONG_ATTEMPT_BINDING: tests that verify the grader rejects evidence from wrong attempt
    if "cross_attempt" in name:
        categories.add(ConformanceCaseCategory.WRONG_ATTEMPT_BINDING)

    # WRONG_TASK_BINDING: tests that verify the grader rejects evidence from wrong task
    if "cross_task" in name:
        categories.add(ConformanceCaseCategory.WRONG_TASK_BINDING)

    # WRONG_ACTION_BINDING: tests that verify the grader rejects wrong action type
    if any(kw in name for kw in (
        "action_type_mismatch", "action_class_mismatch", "action_does_not_match",
        "action_type_mismatches",
    )):
        categories.add(ConformanceCaseCategory.WRONG_ACTION_BINDING)

    # WRONG_BOUNDARY_BINDING: tests that verify the grader rejects wrong collection boundary
    if "collection_boundary_mismatch" in name or "boundary_mismatch" in name:
        categories.add(ConformanceCaseCategory.WRONG_BOUNDARY_BINDING)

    # WRONG_SOURCE_BINDING: tests that verify the grader rejects wrong source binding
    if any(kw in name for kw in (
        "source_mismatch", "source_binding", "source_unbound",
        "source_evidence", "rejects_source",
    )):
        categories.add(ConformanceCaseCategory.WRONG_SOURCE_BINDING)

    # DENOMINATOR_BEHAVIOR: tests that verify denominator behavior
    if any(kw in name for kw in (
        "denominator", "aggregates_multiple", "counts_each_declared",
    )):
        categories.add(ConformanceCaseCategory.DENOMINATOR_BEHAVIOR)

    # MALFORMED_EVIDENCE: tests that verify the grader rejects evidence that is
    # present but semantically invalid (unverified, unverifiable, ambiguous,
    # invalid outcome, mismatched fields that indicate corruption not just
    # a different binding).
    if any(kw in name for kw in (
        "unverifiable", "unverified_receipt", "unverified_observation",
        "unverified", "invalid_l4_outcome", "ambiguous_failed_layers",
        "verified_l4_with_failed_prerequisite", "mismatched_evidence",
        "malform",
    )):
        categories.add(ConformanceCaseCategory.MALFORMED_EVIDENCE)

    return categories


def _build_coverage_map() -> dict[str, set[ConformanceCaseCategory]]:
    """Build a map of grader_id -> covered conformance categories."""
    coverage: dict[str, set[ConformanceCaseCategory]] = {}
    for test_name in _extract_test_names():
        grader_id = _grader_id_from_test_name(test_name)
        if grader_id is None:
            continue
        categories = _categorize_test(test_name)
        coverage.setdefault(grader_id, set()).update(categories)
        # secret_detection_graders_* tests apply to both precision and recall
        if grader_id == "secret_detection_precision" and test_name.startswith("secret_detection_graders"):
            coverage.setdefault("secret_detection_recall", set()).update(categories)
    return coverage


_COVERAGE = _build_coverage_map()


def test_conformance_test_file_exists():
    assert _TEST_FILE.is_file()


def test_every_authoritative_grader_has_conformance_tests():
    for key, entry in GRADER_INVENTORY.items():
        if entry.producer_path == ProducerPath.PARTIAL_EXTERNAL:
            continue
        assert entry.grader_id in _COVERAGE, (
            f"grader {entry.grader_id}: no conformance tests found in {_TEST_FILE.name}"
        )


@pytest.mark.parametrize("category", list(ConformanceCaseCategory))
def test_every_authoritative_grader_covers_category(category: ConformanceCaseCategory):
    gaps: list[str] = []
    for key, entry in GRADER_INVENTORY.items():
        if entry.producer_path == ProducerPath.PARTIAL_EXTERNAL:
            continue
        required = entry.required_categories()
        if category not in required:
            continue
        covered = _COVERAGE.get(entry.grader_id, set())
        if category not in covered:
            gaps.append(entry.grader_id)
    if gaps:
        pytest.fail(
            f"graders missing {category.value} conformance tests: {gaps}"
        )


def test_conformance_coverage_summary():
    """Print a coverage summary. Fails if any required category is uncovered."""
    gaps: list[str] = []
    for key, entry in GRADER_INVENTORY.items():
        if entry.producer_path == ProducerPath.PARTIAL_EXTERNAL:
            continue
        required = entry.required_categories()
        covered = _COVERAGE.get(entry.grader_id, set())
        missing = required - covered
        if missing:
            missing_names = sorted(c.value for c in missing)
            gaps.append(f"  {entry.grader_id}: missing {missing_names}")
    if gaps:
        pytest.fail(
            "Conformance coverage gaps:\n" + "\n".join(gaps)
            + f"\n(covered graders: {len(_COVERAGE)}, "
            f"required graders: {sum(1 for e in GRADER_INVENTORY.values() if e.producer_path != ProducerPath.PARTIAL_EXTERNAL)})"
        )
