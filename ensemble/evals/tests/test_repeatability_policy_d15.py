# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the D15 repeatability policy for Phase C
five-repetition analysis.

Verifies that the policy encodes mean pairwise agreement as the
primary statistic, all-five-agree and accuracy as secondary
statistics, task-bootstrap confidence intervals for uncertainty,
correct pairwise agreement computation, repeatability
classification distinguishing consistently wrong from consistently
correct, and content hash validation. No external dependencies (no
files, network, or DB).
"""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.repeatability_policy import (
    REPETITION_COUNT,
    REPEATABILITY_POLICY_VERSION,
    RepeatabilityOutcome,
    RepeatabilityPolicy,
    RepeatabilityStatistic,
    RepeatabilitySummary,
    TaskRepeatabilityRecord,
    build_repeatability_policy,
    build_task_repeatability_record,
    classify_repeatability,
    compute_pairwise_agreement,
)


pytestmark = pytest.mark.unit


class TestRepeatabilityPolicy:
    def test_builds_valid_policy(self):
        policy = build_repeatability_policy()
        assert policy.repetition_count == REPETITION_COUNT
        assert policy.repetition_count == 5
        assert policy.primary_statistic == RepeatabilityStatistic.MEAN_PAIRWISE_AGREEMENT
        assert RepeatabilityStatistic.ALL_FIVE_AGREE in policy.secondary_statistics
        assert RepeatabilityStatistic.ACCURACY in policy.secondary_statistics
        assert RepeatabilityStatistic.TASK_BOOTSTRAP_CI in policy.secondary_statistics
        assert policy.bootstrap_confidence == 0.95

    def test_policy_is_deterministic(self):
        policy_a = build_repeatability_policy()
        policy_b = build_repeatability_policy()
        assert policy_a.content_hash == policy_b.content_hash

    def test_rejects_wrong_repetition_count(self):
        with pytest.raises(ValidationError, match="repetition_count must be 5"):
            RepeatabilityPolicy(
                policy_id="test",
                policy_version=REPEATABILITY_POLICY_VERSION,
                repetition_count=3,
                primary_statistic=RepeatabilityStatistic.MEAN_PAIRWISE_AGREEMENT,
                secondary_statistics=[RepeatabilityStatistic.ALL_FIVE_AGREE],
                bootstrap_iterations=10000,
                bootstrap_seed=42,
                content_hash="0" * 64,
            )

    def test_rejects_wrong_primary_statistic(self):
        with pytest.raises(ValidationError, match="primary_statistic must be"):
            RepeatabilityPolicy(
                policy_id="test",
                policy_version=REPEATABILITY_POLICY_VERSION,
                repetition_count=REPETITION_COUNT,
                primary_statistic=RepeatabilityStatistic.ACCURACY,
                secondary_statistics=[RepeatabilityStatistic.ALL_FIVE_AGREE],
                bootstrap_iterations=10000,
                bootstrap_seed=42,
                content_hash="0" * 64,
            )

    def test_rejects_primary_in_secondary(self):
        with pytest.raises(ValidationError, match="must not also appear"):
            RepeatabilityPolicy(
                policy_id="test",
                policy_version=REPEATABILITY_POLICY_VERSION,
                repetition_count=REPETITION_COUNT,
                primary_statistic=RepeatabilityStatistic.MEAN_PAIRWISE_AGREEMENT,
                secondary_statistics=[
                    RepeatabilityStatistic.MEAN_PAIRWISE_AGREEMENT,
                    RepeatabilityStatistic.ALL_FIVE_AGREE,
                ],
                bootstrap_iterations=10000,
                bootstrap_seed=42,
                content_hash="0" * 64,
            )

    def test_rejects_content_hash_mismatch(self):
        with pytest.raises(ValidationError, match="content_hash mismatch"):
            RepeatabilityPolicy(
                policy_id="test",
                policy_version=REPEATABILITY_POLICY_VERSION,
                repetition_count=REPETITION_COUNT,
                primary_statistic=RepeatabilityStatistic.MEAN_PAIRWISE_AGREEMENT,
                secondary_statistics=[RepeatabilityStatistic.ALL_FIVE_AGREE],
                bootstrap_iterations=10000,
                bootstrap_seed=42,
                content_hash="0" * 64,
            )


class TestPairwiseAgreement:
    def test_all_agree(self):
        agreements, total = compute_pairwise_agreement([True, True, True, True, True])
        assert agreements == 10
        assert total == 10

    def test_all_disagree(self):
        agreements, total = compute_pairwise_agreement([True, False, True, False, True])
        expected = sum(1 for i in range(5) for j in range(i + 1, 5) if [True, False, True, False, True][i] == [True, False, True, False, True][j])
        assert agreements == expected
        assert total == 10

    def test_two_outcomes(self):
        agreements, total = compute_pairwise_agreement([True, False])
        assert agreements == 0
        assert total == 1

    def test_single_outcome(self):
        agreements, total = compute_pairwise_agreement([True])
        assert agreements == 0
        assert total == 0


class TestClassifyRepeatability:
    def test_consistently_correct(self):
        outcome = classify_repeatability([True, True, True, True, True])
        assert outcome == RepeatabilityOutcome.CONSISTENTLY_CORRECT

    def test_consistently_wrong(self):
        outcome = classify_repeatability([False, False, False, False, False])
        assert outcome == RepeatabilityOutcome.CONSISTENTLY_WRONG

    def test_inconsistent(self):
        outcome = classify_repeatability([True, True, True, True, False])
        assert outcome == RepeatabilityOutcome.INCONSISTENT

    def test_insufficient(self):
        outcome = classify_repeatability([True, True, True, True])
        assert outcome == RepeatabilityOutcome.INSUFFICIENT


class TestTaskRepeatabilityRecord:
    def test_builds_consistently_correct(self):
        record = build_task_repeatability_record("task-1", [True, True, True, True, True])
        assert record.outcome == RepeatabilityOutcome.CONSISTENTLY_CORRECT
        assert record.all_agree is True
        assert record.majority_correct is True
        assert record.pairwise_agreements == 10
        assert record.total_pairs == 10

    def test_builds_consistently_wrong(self):
        record = build_task_repeatability_record("task-1", [False, False, False, False, False])
        assert record.outcome == RepeatabilityOutcome.CONSISTENTLY_WRONG
        assert record.all_agree is True
        assert record.majority_correct is False

    def test_builds_inconsistent(self):
        record = build_task_repeatability_record("task-1", [True, True, True, False, False])
        assert record.outcome == RepeatabilityOutcome.INCONSISTENT
        assert record.all_agree is False
        assert record.majority_correct is True

    def test_builds_insufficient(self):
        record = build_task_repeatability_record("task-1", [True, True, True])
        assert record.outcome == RepeatabilityOutcome.INSUFFICIENT

    def test_rejects_too_many_repetitions(self):
        with pytest.raises(ValidationError, match="at most 5"):
            build_task_repeatability_record("task-1", [True] * 6)

    def test_rejects_wrong_total_pairs(self):
        with pytest.raises(ValidationError, match="total_pairs"):
            TaskRepeatabilityRecord(
                task_id="task-1",
                repetition_outcomes=[True, True, True, True, True],
                pairwise_agreements=10,
                total_pairs=99,
                all_agree=True,
                majority_correct=True,
                outcome=RepeatabilityOutcome.CONSISTENTLY_CORRECT,
            )


class TestRepeatabilitySummary:
    def test_builds_valid_summary(self):
        summary = RepeatabilitySummary(
            summary_id="test",
            mean_pairwise_agreement=0.8,
            all_five_agree_rate=0.6,
            accuracy=0.7,
            task_count=10,
            consistently_correct_count=5,
            consistently_wrong_count=1,
            inconsistent_count=3,
            insufficient_count=1,
        )
        assert summary.mean_pairwise_agreement == 0.8
        assert summary.task_count == 10

    def test_rejects_unknown_field(self):
        with pytest.raises(ValidationError):
            RepeatabilitySummary(
                summary_id="test",
                mean_pairwise_agreement=0.8,
                all_five_agree_rate=0.6,
                accuracy=0.7,
                task_count=10,
                consistently_correct_count=5,
                consistently_wrong_count=1,
                inconsistent_count=3,
                insufficient_count=1,
                unknown_field="bad",  # type: ignore
            )
