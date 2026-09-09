# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version  2.0.

"""Tier 1 typed executable conformance cases for the privacy-boundary graders.

Covers model_boundary_raw_secret_rate, exact_local_rehydration,
secret_detection_precision, secret_detection_recall, and artifact_leakage.
"""

from __future__ import annotations

import pytest

from conformance.cases import ConformanceCase, case_id
from conformance.registry import cases_for_grader
from conformance.runner import assert_case_passes

pytestmark = pytest.mark.unit

_PRIVACY_BOUNDARY_GRADERS = (
    "model_boundary_raw_secret_rate",
    "exact_local_rehydration",
    "secret_detection_precision",
    "secret_detection_recall",
    "artifact_leakage",
)


def _privacy_boundary_cases() -> list[ConformanceCase]:
    cases: list[ConformanceCase] = []
    for grader_id in _PRIVACY_BOUNDARY_GRADERS:
        cases.extend(cases_for_grader(grader_id))
    return cases


_CASES = _privacy_boundary_cases()


@pytest.mark.parametrize("case", _CASES, ids=case_id)
def test_privacy_boundary_grader_conforms_to_required_category(
    case: ConformanceCase,
) -> None:
    assert_case_passes(case)
