# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version  2.0.

"""Shared test runner for conformance cases.

Each per-suite conformance test file parametrizes over its subset of cases
and delegates to ``assert_case_passes`` to execute the grader and assert the
declared outcome.
"""

from __future__ import annotations

import pytest

from g8e_evals.graders import VerificationStatus, grade_deterministically

from .cases import ConformanceCase


def assert_case_passes(case: ConformanceCase) -> None:
    """Execute one conformance case and assert the declared outcome."""
    if case.raises is not None:
        with pytest.raises(case.raises):
            grade_deterministically(case.grader_id, "2.0.0", case.build_context())
        return

    result = grade_deterministically(case.grader_id, "1.0.0", case.build_context())

    assert result.verification_status == case.expected_status, (
        f"{case.grader_id}/{case.category.value}: "
        f"status {result.verification_status} != {case.expected_status}"
    )
    if case.expected_value is not None:
        assert result.value == case.expected_value, (
            f"{case.grader_id}/{case.category.value}: "
            f"value {result.value} != {case.expected_value}"
        )
    if case.expected_failure_contains is not None:
        assert result.failure is not None
        assert case.expected_failure_contains in result.failure, (
            f"{case.grader_id}/{case.category.value}: "
            f"{case.expected_failure_contains!r} not in {result.failure!r}"
        )
    elif case.expected_status != VerificationStatus.VERIFIED:
        assert result.failure is not None, (
            f"{case.grader_id}/{case.category.value}: expected a failure message"
        )
    if case.expected_denominator is not None:
        assert result.denominator_contribution == case.expected_denominator, (
            f"{case.grader_id}/{case.category.value}: "
            f"denominator {result.denominator_contribution} != {case.expected_denominator}"
        )
