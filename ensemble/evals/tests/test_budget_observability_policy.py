# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for BudgetObservabilityPolicy and usage extraction.

These tests verify the budget observability policy contract: which
ceilings are enforceable, unobservable-ceiling rejection, hash stability,
and the usage extraction helper. The tests import from ``g8e_evals.runner``
which transitively imports ``g8e_evals.harness`` (and the ``app`` package),
so they run as integration tests under ``uv run``.
"""

from __future__ import annotations

import pytest

pytestmark = pytest.mark.integration

from g8e_evals.arms import Arm
from g8e_evals.harness import Response
from g8e_evals.runner import (
    BudgetCeiling,
    BudgetObservabilityPolicy,
    BudgetTracker,
    CampaignRunnerError,
    DEFAULT_BUDGET_OBSERVABILITY_POLICY,
    compute_budget_observability_policy_hash,
    _extract_usage_from_response,
)
from g8e_evals.schema import ProviderBudget
from g8e_evals.sut.direct_provider import DirectCallEvidence, _DirectEvidenceWrapper


# ---------------------------------------------------------------------------
# BudgetObservabilityPolicy: is_ceiling_enforced
# ---------------------------------------------------------------------------


class TestIsCeilingEnforced:
    """is_ceiling_enforced returns True only for observable, non-excluded ceilings."""

    def test_max_requests_always_enforced_when_not_excluded(self):
        policy = BudgetObservabilityPolicy(
            tokens_observable=False,
            usd_observable=False,
            excluded_ceilings=frozenset(),
        )
        assert policy.is_ceiling_enforced(BudgetCeiling.MAX_REQUESTS) is True

    def test_max_requests_not_enforced_when_excluded(self):
        policy = BudgetObservabilityPolicy(
            excluded_ceilings=frozenset({BudgetCeiling.MAX_REQUESTS}),
        )
        assert policy.is_ceiling_enforced(BudgetCeiling.MAX_REQUESTS) is False

    def test_max_tokens_enforced_when_observable_and_not_excluded(self):
        policy = BudgetObservabilityPolicy(
            tokens_observable=True,
            excluded_ceilings=frozenset(),
        )
        assert policy.is_ceiling_enforced(BudgetCeiling.MAX_TOKENS) is True

    def test_max_tokens_not_enforced_when_not_observable(self):
        policy = BudgetObservabilityPolicy(
            tokens_observable=False,
            excluded_ceilings=frozenset(),
        )
        assert policy.is_ceiling_enforced(BudgetCeiling.MAX_TOKENS) is False

    def test_max_tokens_not_enforced_when_excluded_even_if_observable(self):
        policy = BudgetObservabilityPolicy(
            tokens_observable=True,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_TOKENS}),
        )
        assert policy.is_ceiling_enforced(BudgetCeiling.MAX_TOKENS) is False

    def test_max_usd_enforced_when_observable_and_not_excluded(self):
        policy = BudgetObservabilityPolicy(
            usd_observable=True,
            excluded_ceilings=frozenset(),
        )
        assert policy.is_ceiling_enforced(BudgetCeiling.MAX_USD) is True

    def test_max_usd_not_enforced_when_not_observable(self):
        policy = BudgetObservabilityPolicy(
            usd_observable=False,
            excluded_ceilings=frozenset(),
        )
        assert policy.is_ceiling_enforced(BudgetCeiling.MAX_USD) is False

    def test_max_usd_not_enforced_when_excluded_even_if_observable(self):
        policy = BudgetObservabilityPolicy(
            usd_observable=True,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        assert policy.is_ceiling_enforced(BudgetCeiling.MAX_USD) is False


# ---------------------------------------------------------------------------
# BudgetObservabilityPolicy: validate_budget
# ---------------------------------------------------------------------------


class TestValidateBudget:
    """validate_budget rejects declared ceilings that are unobservable and not excluded."""

    def test_none_budget_passes(self):
        policy = BudgetObservabilityPolicy()
        policy.validate_budget(None)

    def test_default_policy_allows_budget_with_only_max_requests(self):
        policy = DEFAULT_BUDGET_OBSERVABILITY_POLICY
        budget = ProviderBudget(max_usd=1000.0, max_requests=100)
        policy.validate_budget(budget)

    def test_unobservable_max_tokens_rejected(self):
        policy = BudgetObservabilityPolicy(
            tokens_observable=False,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        budget = ProviderBudget(max_usd=1000.0, max_tokens=100, max_requests=100)
        with pytest.raises(CampaignRunnerError, match="max_tokens"):
            policy.validate_budget(budget)

    def test_unobservable_max_usd_rejected(self):
        policy = BudgetObservabilityPolicy(
            usd_observable=False,
            excluded_ceilings=frozenset(),
        )
        budget = ProviderBudget(max_usd=1000.0, max_requests=100)
        with pytest.raises(CampaignRunnerError, match="max_usd"):
            policy.validate_budget(budget)

    def test_observable_max_tokens_passes(self):
        policy = BudgetObservabilityPolicy(
            tokens_observable=True,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        budget = ProviderBudget(max_usd=1000.0, max_tokens=100, max_requests=100)
        policy.validate_budget(budget)

    def test_excluded_max_tokens_passes_even_when_not_observable(self):
        policy = BudgetObservabilityPolicy(
            tokens_observable=False,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD, BudgetCeiling.MAX_TOKENS}),
        )
        budget = ProviderBudget(max_usd=1000.0, max_tokens=100, max_requests=100)
        policy.validate_budget(budget)

    def test_excluded_max_usd_passes_even_when_not_observable(self):
        policy = BudgetObservabilityPolicy(
            usd_observable=False,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        budget = ProviderBudget(max_usd=1000.0, max_requests=100)
        policy.validate_budget(budget)

    def test_observable_usd_passes(self):
        policy = BudgetObservabilityPolicy(
            usd_observable=True,
            excluded_ceilings=frozenset(),
        )
        budget = ProviderBudget(max_usd=1000.0, max_requests=100)
        policy.validate_budget(budget)


# ---------------------------------------------------------------------------
# BudgetObservabilityPolicy: hash stability
# ---------------------------------------------------------------------------


class TestBudgetObservabilityPolicyHash:
    """compute_budget_observability_policy_hash is stable and distinguishes policies."""

    def test_same_policy_same_hash(self):
        p1 = BudgetObservabilityPolicy(tokens_observable=True)
        p2 = BudgetObservabilityPolicy(tokens_observable=True)
        assert compute_budget_observability_policy_hash(p1) == compute_budget_observability_policy_hash(p2)

    def test_tokens_observable_changes_hash(self):
        p1 = BudgetObservabilityPolicy(tokens_observable=False)
        p2 = BudgetObservabilityPolicy(tokens_observable=True)
        assert compute_budget_observability_policy_hash(p1) != compute_budget_observability_policy_hash(p2)

    def test_usd_observable_changes_hash(self):
        p1 = BudgetObservabilityPolicy(usd_observable=False)
        p2 = BudgetObservabilityPolicy(usd_observable=True)
        assert compute_budget_observability_policy_hash(p1) != compute_budget_observability_policy_hash(p2)

    def test_excluded_ceilings_change_hash(self):
        p1 = BudgetObservabilityPolicy(excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}))
        p2 = BudgetObservabilityPolicy(excluded_ceilings=frozenset({BudgetCeiling.MAX_TOKENS}))
        assert compute_budget_observability_policy_hash(p1) != compute_budget_observability_policy_hash(p2)

    def test_hash_is_64_char_hex(self):
        h = compute_budget_observability_policy_hash(BudgetObservabilityPolicy())
        assert len(h) == 64
        assert all(c in "0123456789abcdef" for c in h)


# ---------------------------------------------------------------------------
# BudgetTracker with policy
# ---------------------------------------------------------------------------


class TestBudgetTrackerWithPolicy:
    """BudgetTracker respects the observability policy for token/USD ceilings."""

    def test_token_ceiling_enforced_when_observable(self):
        budget = ProviderBudget(max_usd=1000.0, max_tokens=100, max_requests=100)
        policy = BudgetObservabilityPolicy(
            tokens_observable=True,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        tracker = BudgetTracker(budget, policy=policy)
        tracker.record_usage(tokens=50, usd=None)
        tracker.check_usage_budgets()
        tracker.record_usage(tokens=60, usd=None)
        from g8e_evals.runner import BudgetExhausted
        with pytest.raises(BudgetExhausted, match="token ceiling"):
            tracker.check_usage_budgets()

    def test_token_ceiling_not_checked_when_excluded(self):
        budget = ProviderBudget(max_usd=1000.0, max_tokens=10, max_requests=100)
        policy = BudgetObservabilityPolicy(
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD, BudgetCeiling.MAX_TOKENS}),
        )
        tracker = BudgetTracker(budget, policy=policy)
        tracker.record_usage(tokens=1000, usd=None)
        tracker.check_usage_budgets()

    def test_usd_ceiling_enforced_when_observable(self):
        budget = ProviderBudget(max_usd=10.0, max_requests=100)
        policy = BudgetObservabilityPolicy(
            usd_observable=True,
            excluded_ceilings=frozenset(),
        )
        tracker = BudgetTracker(budget, policy=policy)
        tracker.record_usage(tokens=None, usd=5.0)
        tracker.check_usage_budgets()
        tracker.record_usage(tokens=None, usd=6.0)
        from g8e_evals.runner import BudgetExhausted
        with pytest.raises(BudgetExhausted, match="usd ceiling"):
            tracker.check_usage_budgets()

    def test_usd_ceiling_not_checked_when_excluded(self):
        budget = ProviderBudget(max_usd=10.0, max_requests=100)
        policy = BudgetObservabilityPolicy(
            usd_observable=False,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        tracker = BudgetTracker(budget, policy=policy)
        tracker.record_usage(tokens=None, usd=1000.0)
        tracker.check_usage_budgets()

    def test_request_ceiling_always_enforced(self):
        budget = ProviderBudget(max_usd=1000.0, max_requests=2)
        tracker = BudgetTracker(budget)
        tracker.record_provider_call()
        tracker.check_request_budget()
        tracker.record_provider_call()
        from g8e_evals.runner import BudgetExhausted
        with pytest.raises(BudgetExhausted, match="request ceiling"):
            tracker.check_request_budget()

    def test_default_policy_does_not_enforce_tokens_or_usd(self):
        budget = ProviderBudget(max_usd=10.0, max_tokens=10, max_requests=100)
        tracker = BudgetTracker(budget)
        tracker.record_usage(tokens=1000, usd=1000.0)
        tracker.check_usage_budgets()


# ---------------------------------------------------------------------------
# _extract_usage_from_response
# ---------------------------------------------------------------------------


class TestExtractUsageFromResponse:
    """_extract_usage_from_response reads token usage from DirectCallEvidence."""

    def test_returns_none_none_when_no_evidence(self):
        response = Response(answer="test", model="qwen3:8b", arm=Arm.DIRECT)
        tokens, usd = _extract_usage_from_response(response)
        assert tokens is None
        assert usd is None

    def test_returns_tokens_when_usage_reported(self):
        evidence = DirectCallEvidence(
            provider="ollama",
            model="qwen3:8b",
            total_token_count=150,
            usage_reported=True,
        )
        response = Response(
            answer="test",
            model="qwen3:8b",
            arm=Arm.DIRECT,
            chat_evidence=_DirectEvidenceWrapper(evidence),
        )
        tokens, usd = _extract_usage_from_response(response)
        assert tokens == 150
        assert usd is None

    def test_returns_none_when_usage_not_reported(self):
        evidence = DirectCallEvidence(
            provider="ollama",
            model="qwen3:8b",
            total_token_count=150,
            usage_reported=False,
        )
        response = Response(
            answer="test",
            model="qwen3:8b",
            arm=Arm.DIRECT,
            chat_evidence=_DirectEvidenceWrapper(evidence),
        )
        tokens, usd = _extract_usage_from_response(response)
        assert tokens is None
        assert usd is None

    def test_returns_none_when_tokens_zero(self):
        evidence = DirectCallEvidence(
            provider="ollama",
            model="qwen3:8b",
            total_token_count=0,
            usage_reported=True,
        )
        response = Response(
            answer="test",
            model="qwen3:8b",
            arm=Arm.DIRECT,
            chat_evidence=_DirectEvidenceWrapper(evidence),
        )
        tokens, usd = _extract_usage_from_response(response)
        assert tokens is None
        assert usd is None

    def test_usd_always_none_without_pricing_authority(self):
        evidence = DirectCallEvidence(
            provider="ollama",
            model="qwen3:8b",
            total_token_count=100,
            usage_reported=True,
        )
        response = Response(
            answer="test",
            model="qwen3:8b",
            arm=Arm.DIRECT,
            chat_evidence=_DirectEvidenceWrapper(evidence),
        )
        tokens, usd = _extract_usage_from_response(response)
        assert tokens == 100
        assert usd is None
