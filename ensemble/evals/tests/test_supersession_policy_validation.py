# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for policy-valid supersession validation.

Verifies that supersession is permitted only for interrupted or
infrastructure-invalid assignments. Superseding a completed valid
assignment is rejected. Superseding a model-failed, governance-rejected,
human-denied, timed-out, or invalid-evidence assignment is also rejected
because those are assignment-scoped failures, not infrastructure
failures. Result-aware discretionary reruns create a new campaign
revision and cannot replace an unfavorable valid result.
"""

from __future__ import annotations

import pytest

from g8e_evals.index import (
    AssignmentDisposition,
    IndexCreationReason,
    SupersessionPolicy,
    validate_supersession_policy,
)


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64
_ZERO_HASH = "0" * 64


class TestSupersessionPolicy:
    def test_supersede_interrupted_assignment_allowed(self):
        """Superseding an interrupted assignment is policy-valid."""
        policy = SupersessionPolicy()
        assert policy.is_supersession_allowed(
            prior_disposition=AssignmentDisposition.SUPERSEDED,
            prior_terminal_status="infrastructure_failed",
        )

    def test_supersede_completed_valid_assignment_rejected(self):
        """Superseding a completed valid assignment is not policy-valid."""
        policy = SupersessionPolicy()
        assert not policy.is_supersession_allowed(
            prior_disposition=AssignmentDisposition.EFFECTIVE,
            prior_terminal_status="completed",
        )

    def test_supersede_model_failed_assignment_rejected(self):
        """Superseding a model-failed assignment is not policy-valid."""
        policy = SupersessionPolicy()
        assert not policy.is_supersession_allowed(
            prior_disposition=AssignmentDisposition.SUPERSEDED,
            prior_terminal_status="model_failed",
        )

    def test_supersede_governance_rejected_assignment_rejected(self):
        """Superseding a governance-rejected assignment is not policy-valid."""
        policy = SupersessionPolicy()
        assert not policy.is_supersession_allowed(
            prior_disposition=AssignmentDisposition.SUPERSEDED,
            prior_terminal_status="governance_rejected",
        )

    def test_supersede_human_denied_assignment_rejected(self):
        """Superseding a human-denied assignment is not policy-valid."""
        policy = SupersessionPolicy()
        assert not policy.is_supersession_allowed(
            prior_disposition=AssignmentDisposition.SUPERSEDED,
            prior_terminal_status="human_denied",
        )

    def test_supersede_timed_out_assignment_rejected(self):
        """Superseding a timed-out assignment is not policy-valid."""
        policy = SupersessionPolicy()
        assert not policy.is_supersession_allowed(
            prior_disposition=AssignmentDisposition.SUPERSEDED,
            prior_terminal_status="timed_out",
        )

    def test_supersede_invalid_evidence_assignment_rejected(self):
        """Superseding an invalid-evidence assignment is not policy-valid."""
        policy = SupersessionPolicy()
        assert not policy.is_supersession_allowed(
            prior_disposition=AssignmentDisposition.SUPERSEDED,
            prior_terminal_status="invalid_evidence",
        )

    def test_supersede_infrastructure_failed_assignment_allowed(self):
        """Superseding an infrastructure-failed assignment is policy-valid."""
        policy = SupersessionPolicy()
        assert policy.is_supersession_allowed(
            prior_disposition=AssignmentDisposition.SUPERSEDED,
            prior_terminal_status="infrastructure_failed",
        )

    def test_supersede_effective_with_infrastructure_failed_still_rejected(self):
        """An effective report cannot be superseded even if the terminal status
        was infrastructure_failed (effective means it was already retried and completed)."""
        policy = SupersessionPolicy()
        assert not policy.is_supersession_allowed(
            prior_disposition=AssignmentDisposition.EFFECTIVE,
            prior_terminal_status="infrastructure_failed",
        )


class TestValidateSupersessionPolicy:
    def test_valid_supersession_of_interrupted_passes(self):
        """A supersession generation replacing an interrupted assignment passes."""
        validate_supersession_policy(
            prior_disposition=AssignmentDisposition.SUPERSEDED,
            prior_terminal_status="infrastructure_failed",
            new_creation_reason=IndexCreationReason.SUPERSESSION,
        )

    def test_invalid_supersession_of_completed_rejected(self):
        """A supersession generation replacing a completed assignment is rejected."""
        with pytest.raises(ValueError, match=r"supersession.*not allowed"):
            validate_supersession_policy(
                prior_disposition=AssignmentDisposition.EFFECTIVE,
                prior_terminal_status="completed",
                new_creation_reason=IndexCreationReason.SUPERSESSION,
            )

    def test_non_supersession_creation_reason_skips_policy_check(self):
        """A non-supersession creation reason (e.g. resume) skips the policy check."""
        validate_supersession_policy(
            prior_disposition=AssignmentDisposition.EFFECTIVE,
            prior_terminal_status="completed",
            new_creation_reason=IndexCreationReason.RESUME,
        )
