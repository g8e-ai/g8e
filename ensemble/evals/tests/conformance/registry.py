# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version  2.0.

"""Aggregated conformance case registry.

Combines the proportion-grader cases and the strict-grader cases into a single
list. The per-suite conformance test files filter this registry by grader ID.
"""

from __future__ import annotations

from .cases import ConformanceCase, proportion_cases
from .strict_cases import (
    canary_scrubbing_cases,
    final_state_cases,
    independent_state_cases,
    model_boundary_cases,
    policy_outcome_cases,
    protocol_chain_cases,
    receipt_integrity_cases,
    rehydration_cases,
    secret_detection_cases,
)


def _build_all_cases() -> list[ConformanceCase]:
    cases: list[ConformanceCase] = []
    cases.extend(proportion_cases())
    cases.extend(receipt_integrity_cases())
    cases.extend(canary_scrubbing_cases())
    cases.extend(model_boundary_cases())
    cases.extend(rehydration_cases())
    cases.extend(secret_detection_cases("secret_detection_precision"))
    cases.extend(secret_detection_cases("secret_detection_recall"))
    cases.extend(final_state_cases())
    cases.extend(independent_state_cases())
    cases.extend(policy_outcome_cases())
    cases.extend(protocol_chain_cases())
    return cases


ALL_CASES: list[ConformanceCase] = _build_all_cases()


def cases_for_grader(grader_id: str) -> list[ConformanceCase]:
    """Return all registered conformance cases for a single grader."""
    return [c for c in ALL_CASES if c.grader_id == grader_id]
