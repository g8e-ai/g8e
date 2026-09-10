# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for duplicate effective assignment detection.

Verifies that exactly one effective valid report exists per publishable
assignment within an index generation. Two effective reports for the
same assignment are rejected. Multiple superseded reports for the same
assignment are allowed (only one effective). Qualification and
unavailable dispositions do not count as effective reports.
"""

from __future__ import annotations

import pytest

from g8e_evals.index import (
    AssignmentDisposition,
    AssignmentDispositionEntry,
    IndexCreationReason,
    IndexGeneration,
    compute_index_generation_hash,
    validate_no_duplicate_effective_assignments,
)


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64
_ZERO_HASH = "0" * 64


def _make_generation(
    *,
    assignment_dispositions: list[dict],
    generation_number: int = 0,
    parent_generation_hash: str = _ZERO_HASH,
    creation_reason: IndexCreationReason = IndexCreationReason.INITIAL,
) -> IndexGeneration:
    report_checksums = [_VALID_HASH] * len({d["assignment_id"] for d in assignment_dispositions})
    content_hash = compute_index_generation_hash(
        generation_number=generation_number,
        parent_generation_hash=parent_generation_hash,
        creation_reason=creation_reason,
        report_checksums=report_checksums,
        assignment_dispositions=assignment_dispositions,
    )
    disposition_entries = [
        AssignmentDispositionEntry(
            assignment_id=d["assignment_id"],
            disposition=AssignmentDisposition(d["disposition"])
            if isinstance(d["disposition"], str)
            else d["disposition"],
        )
        for d in assignment_dispositions
    ]
    return IndexGeneration(
        generation_number=generation_number,
        parent_generation_hash=parent_generation_hash,
        creation_reason=creation_reason,
        report_checksums=report_checksums,
        assignment_dispositions=disposition_entries,
        content_hash=content_hash,
    )


class TestDuplicateEffectiveAssignment:
    def test_single_effective_assignment_passes(self):
        """One effective report for one assignment passes."""
        gen = _make_generation(
            assignment_dispositions=[
                {"assignment_id": "a1", "disposition": AssignmentDisposition.EFFECTIVE},
            ],
        )
        validate_no_duplicate_effective_assignments(gen)

    def test_two_effective_reports_for_same_assignment_rejected(self):
        """Two effective reports for the same assignment are rejected."""
        gen = _make_generation(
            assignment_dispositions=[
                {"assignment_id": "a1", "disposition": AssignmentDisposition.EFFECTIVE},
                {"assignment_id": "a1", "disposition": AssignmentDisposition.EFFECTIVE},
            ],
        )
        with pytest.raises(ValueError, match="duplicate effective"):
            validate_no_duplicate_effective_assignments(gen)

    def test_one_effective_one_superseded_for_same_assignment_passes(self):
        """One effective and one superseded report for the same assignment passes."""
        gen = _make_generation(
            assignment_dispositions=[
                {"assignment_id": "a1", "disposition": AssignmentDisposition.EFFECTIVE},
                {"assignment_id": "a1", "disposition": AssignmentDisposition.SUPERSEDED},
            ],
        )
        validate_no_duplicate_effective_assignments(gen)

    def test_multiple_superseded_for_same_assignment_passes(self):
        """Multiple superseded reports for the same assignment pass (no effective)."""
        gen = _make_generation(
            assignment_dispositions=[
                {"assignment_id": "a1", "disposition": AssignmentDisposition.SUPERSEDED},
                {"assignment_id": "a1", "disposition": AssignmentDisposition.SUPERSEDED},
            ],
        )
        validate_no_duplicate_effective_assignments(gen)

    def test_qualification_disposition_does_not_count_as_effective(self):
        """Qualification disposition does not count as an effective report."""
        gen = _make_generation(
            assignment_dispositions=[
                {"assignment_id": "a1", "disposition": AssignmentDisposition.QUALIFICATION},
                {"assignment_id": "a1", "disposition": AssignmentDisposition.QUALIFICATION},
            ],
        )
        validate_no_duplicate_effective_assignments(gen)

    def test_unavailable_disposition_does_not_count_as_effective(self):
        """Unavailable disposition does not count as an effective report."""
        gen = _make_generation(
            assignment_dispositions=[
                {"assignment_id": "a1", "disposition": AssignmentDisposition.UNAVAILABLE},
                {"assignment_id": "a1", "disposition": AssignmentDisposition.UNAVAILABLE},
            ],
        )
        validate_no_duplicate_effective_assignments(gen)

    def test_effective_reports_for_different_assignments_pass(self):
        """Effective reports for different assignments pass."""
        gen = _make_generation(
            assignment_dispositions=[
                {"assignment_id": "a1", "disposition": AssignmentDisposition.EFFECTIVE},
                {"assignment_id": "a2", "disposition": AssignmentDisposition.EFFECTIVE},
            ],
        )
        validate_no_duplicate_effective_assignments(gen)

    def test_empty_dispositions_passes(self):
        """An index generation with no assignment dispositions passes."""
        gen = _make_generation(assignment_dispositions=[])
        validate_no_duplicate_effective_assignments(gen)
