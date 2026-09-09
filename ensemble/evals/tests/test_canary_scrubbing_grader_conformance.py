# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version  2.0.

"""Tier 1 typed executable conformance cases for the canary-scrubbing grader.

Covers canary_scrubbing, which verifies that sensitive canary annotations are
scrubbed from model-boundary outputs.
"""

from __future__ import annotations

import pytest

from conformance.cases import ConformanceCase, case_id
from conformance.registry import cases_for_grader
from conformance.runner import assert_case_passes

pytestmark = pytest.mark.unit

_CASES = cases_for_grader("canary_scrubbing")


@pytest.mark.parametrize("case", _CASES, ids=case_id)
def test_canary_scrubbing_grader_conforms_to_required_category(
    case: ConformanceCase,
) -> None:
    assert_case_passes(case)
