# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version  2.0.

"""Tier 1 typed executable conformance cases for the governance-adversarial graders.

Covers the 12 receipt-proportion graders (unauthorized_mutation,
exfiltration_attempt, replay_attempt, signed_field_tampering,
payload_tampering, stale_state_root, identity_mismatch, nonce_expiration,
signer_defect, l3_proof_transplant, revoked_credential, policy_attack) and
the 3 cross-cutting graders (receipt_integrity, policy_outcome,
protocol_chain) that consume receipts and deterministic stage evidence.
"""

from __future__ import annotations

import pytest

from .conformance.cases import ConformanceCase, case_id
from .conformance.registry import cases_for_grader
from .conformance.runner import assert_case_passes

pytestmark = pytest.mark.unit

_GOVERNANCE_ADVERSARIAL_GRADERS = (
    "unauthorized_mutation",
    "exfiltration_attempt",
    "replay_attempt",
    "signed_field_tampering",
    "payload_tampering",
    "stale_state_root",
    "identity_mismatch",
    "nonce_expiration",
    "signer_defect",
    "l3_proof_transplant",
    "revoked_credential",
    "policy_attack",
    "receipt_integrity",
    "policy_outcome",
    "protocol_chain",
)


def _governance_adversarial_cases() -> list[ConformanceCase]:
    cases: list[ConformanceCase] = []
    for grader_id in _GOVERNANCE_ADVERSARIAL_GRADERS:
        cases.extend(cases_for_grader(grader_id))
    return cases


_CASES = _governance_adversarial_cases()


@pytest.mark.parametrize("case", _CASES, ids=case_id)
def test_governance_adversarial_grader_conforms_to_required_category(
    case: ConformanceCase,
) -> None:
    assert_case_passes(case)
