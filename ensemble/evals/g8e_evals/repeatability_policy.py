# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""D15 repeatability policy for Phase C five-repetition analysis.

Encodes the frozen repeatability policy that measures within-task
variability across five repetitions. The primary statistic is mean
pairwise agreement across all five repetitions. Secondary statistics
are the all-five-agree rate and separate accuracy. Uncertainty is
estimated through task-bootstrap confidence intervals.

Repetitions estimate within-task variability and are not independent
tasks. The task remains the independent inferential unit. Golden
vectors distinguish consistently wrong behavior from consistently
correct behavior.

D15 statistics:
- Primary: mean pairwise agreement across five repetitions.
- Secondary: all-five-agree rate.
- Accuracy: reported separately.
- Uncertainty: task-bootstrap confidence intervals.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator


REPEATABILITY_POLICY_VERSION = "1.0.0"

REPETITION_COUNT = 5


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class RepeatabilityStatistic(StrEnum):
    """The frozen D15 repeatability statistics.

    ``MEAN_PAIRWISE_AGREEMENT``: Primary statistic. The mean of
    pairwise agreement rates across all C(5,2)=10 repetition pairs
    for each task, averaged across tasks.
    ``ALL_FIVE_AGREE``: Secondary statistic. The fraction of tasks
    where all five repetitions produce the same outcome.
    ``ACCURACY``: Reported separately. The fraction of tasks where
    the majority outcome is correct.
    ``TASK_BOOTSTRAP_CI``: Uncertainty estimate. Task-bootstrap
    confidence interval for the primary statistic.
    """

    MEAN_PAIRWISE_AGREEMENT = "mean_pairwise_agreement"
    ALL_FIVE_AGREE = "all_five_agree"
    ACCURACY = "accuracy"
    TASK_BOOTSTRAP_CI = "task_bootstrap_ci"


class RepeatabilityOutcome(StrEnum):
    """Per-task repeatability outcome across five repetitions.

    ``CONSISTENTLY_CORRECT``: All five repetitions agree and the
    agreed outcome is correct.
    ``CONSISTENTLY_WRONG``: All five repetitions agree and the
    agreed outcome is incorrect.
    ``INCONSISTENT``: Not all five repetitions agree.
    ``INSUFFICIENT``: Fewer than five repetitions have terminal
    outcomes for this task.
    """

    CONSISTENTLY_CORRECT = "consistently_correct"
    CONSISTENTLY_WRONG = "consistently_wrong"
    INCONSISTENT = "inconsistent"
    INSUFFICIENT = "insufficient"


class TaskRepeatabilityRecord(BaseModel):
    """Repeatability record for one task across five repetitions.

    Binds the task ID, per-repetition binary outcomes (pass/fail),
    pairwise agreement count, total pairs, all-five-agree flag,
    majority outcome, and typed repeatability outcome. The record
    distinguishes consistently wrong from consistently correct
    behavior.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    task_id: str = Field(min_length=1, description="Task identifier.")
    repetition_outcomes: list[bool] = Field(
        min_length=1,
        description="Per-repetition binary outcomes (True=pass, False=fail).",
    )
    pairwise_agreements: int = Field(
        ge=0,
        description="Number of agreeing pairs out of C(n,2) total pairs.",
    )
    total_pairs: int = Field(
        ge=0,
        description="Total number of pairs C(n,2) for n repetitions.",
    )
    all_agree: bool = Field(
        description="Whether all repetitions produce the same outcome.",
    )
    majority_correct: bool = Field(
        description="Whether the majority outcome is correct (True=pass).",
    )
    outcome: RepeatabilityOutcome = Field(
        description="Typed repeatability outcome.",
    )

    @model_validator(mode="after")
    def _validate_record(self) -> Self:
        if len(self.repetition_outcomes) > REPETITION_COUNT:
            raise ValueError(
                f"repetition_outcomes must have at most {REPETITION_COUNT} entries: "
                f"got {len(self.repetition_outcomes)}"
            )
        n = len(self.repetition_outcomes)
        expected_pairs = n * (n - 1) // 2
        if self.total_pairs != expected_pairs:
            raise ValueError(
                f"total_pairs ({self.total_pairs}) != C({n},2) = {expected_pairs}"
            )
        if self.pairwise_agreements < 0 or self.pairwise_agreements > self.total_pairs:
            raise ValueError(
                f"pairwise_agreements ({self.pairwise_agreements}) out of range "
                f"[0, {self.total_pairs}]"
            )
        return self


class RepeatabilitySummary(BaseModel):
    """Frozen repeatability summary across all tasks.

    Binds the primary statistic (mean pairwise agreement), secondary
    statistics (all-five-agree rate, accuracy), task count, and
    per-task records. The summary is computed from terminal attempts
    only and distinguishes consistent failure from correct
    repeatability.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    summary_id: str = Field(min_length=1, description="Unique summary identifier.")
    mean_pairwise_agreement: float = Field(
        ge=0.0, le=1.0,
        description="Primary D15 statistic: mean pairwise agreement across tasks.",
    )
    all_five_agree_rate: float = Field(
        ge=0.0, le=1.0,
        description="Secondary D15 statistic: fraction of tasks where all five agree.",
    )
    accuracy: float = Field(
        ge=0.0, le=1.0,
        description="Separate D15 statistic: fraction of tasks where majority is correct.",
    )
    task_count: int = Field(
        ge=1,
        description="Number of tasks with sufficient repetitions.",
    )
    consistently_correct_count: int = Field(
        ge=0,
        description="Tasks where all five agree and majority is correct.",
    )
    consistently_wrong_count: int = Field(
        ge=0,
        description="Tasks where all five agree and majority is wrong.",
    )
    inconsistent_count: int = Field(
        ge=0,
        description="Tasks where not all five agree.",
    )
    insufficient_count: int = Field(
        ge=0,
        description="Tasks with fewer than five terminal repetitions.",
    )


class RepeatabilityPolicy(BaseModel):
    """Frozen D15 repeatability policy.

    Binds the policy version, repetition count, primary and secondary
    statistics, bootstrap configuration, and content hash. The policy
    is frozen before Phase C execution and produces byte-identical
    results from identical accepted evidence.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    policy_id: str = Field(min_length=1, description="Unique policy identifier.")
    policy_version: str = Field(min_length=1, description="Policy schema version.")
    repetition_count: int = Field(
        ge=1,
        description="Number of repetitions per task (D15: 5).",
    )
    primary_statistic: RepeatabilityStatistic = Field(
        description="Primary D15 statistic.",
    )
    secondary_statistics: list[RepeatabilityStatistic] = Field(
        min_length=1,
        description="Secondary D15 statistics.",
    )
    bootstrap_iterations: int = Field(
        ge=1,
        description="Number of bootstrap iterations for confidence intervals.",
    )
    bootstrap_seed: int = Field(
        ge=0,
        description="Deterministic seed for bootstrap resampling.",
    )
    bootstrap_confidence: float = Field(
        default=0.95,
        gt=0.0,
        lt=1.0,
        description="Confidence level for task-bootstrap intervals.",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the policy.",
    )

    @model_validator(mode="after")
    def _validate_policy(self) -> Self:
        if self.repetition_count != REPETITION_COUNT:
            raise ValueError(
                f"repetition_count must be {REPETITION_COUNT} (D15): "
                f"got {self.repetition_count}"
            )
        if self.primary_statistic != RepeatabilityStatistic.MEAN_PAIRWISE_AGREEMENT:
            raise ValueError(
                f"primary_statistic must be MEAN_PAIRWISE_AGREEMENT (D15): "
                f"got {self.primary_statistic!r}"
            )
        if RepeatabilityStatistic.MEAN_PAIRWISE_AGREEMENT in self.secondary_statistics:
            raise ValueError(
                "MEAN_PAIRWISE_AGREEMENT is the primary statistic and must not "
                "also appear in secondary_statistics"
            )
        expected = compute_repeatability_policy_hash(
            policy_id=self.policy_id,
            policy_version=self.policy_version,
            repetition_count=self.repetition_count,
            primary_statistic=self.primary_statistic,
            secondary_statistics=self.secondary_statistics,
            bootstrap_iterations=self.bootstrap_iterations,
            bootstrap_seed=self.bootstrap_seed,
            bootstrap_confidence=self.bootstrap_confidence,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"repeatability policy content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


def compute_repeatability_policy_hash(
    *,
    policy_id: str,
    policy_version: str,
    repetition_count: int,
    primary_statistic: RepeatabilityStatistic,
    secondary_statistics: list[RepeatabilityStatistic],
    bootstrap_iterations: int,
    bootstrap_seed: int,
    bootstrap_confidence: float = 0.95,
) -> str:
    """Compute the content hash for a D15 repeatability policy."""
    payload = json.dumps(
        {
            "policy_id": policy_id,
            "policy_version": policy_version,
            "repetition_count": repetition_count,
            "primary_statistic": primary_statistic.value,
            "secondary_statistics": sorted(s.value for s in secondary_statistics),
            "bootstrap_iterations": bootstrap_iterations,
            "bootstrap_seed": bootstrap_seed,
            "bootstrap_confidence": bootstrap_confidence,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def build_repeatability_policy(
    *,
    policy_id: str = "d15-repeatability-policy",
    bootstrap_iterations: int = 10000,
    bootstrap_seed: int = 42,
) -> RepeatabilityPolicy:
    """Build the frozen D15 repeatability policy.

    The policy is frozen with:
    - Primary: mean pairwise agreement across five repetitions.
    - Secondary: all-five-agree rate, accuracy.
    - Uncertainty: task-bootstrap confidence intervals.
    """
    secondary = [
        RepeatabilityStatistic.ALL_FIVE_AGREE,
        RepeatabilityStatistic.ACCURACY,
        RepeatabilityStatistic.TASK_BOOTSTRAP_CI,
    ]
    content_hash = compute_repeatability_policy_hash(
        policy_id=policy_id,
        policy_version=REPEATABILITY_POLICY_VERSION,
        repetition_count=REPETITION_COUNT,
        primary_statistic=RepeatabilityStatistic.MEAN_PAIRWISE_AGREEMENT,
        secondary_statistics=secondary,
        bootstrap_iterations=bootstrap_iterations,
        bootstrap_seed=bootstrap_seed,
    )
    return RepeatabilityPolicy(
        policy_id=policy_id,
        policy_version=REPEATABILITY_POLICY_VERSION,
        repetition_count=REPETITION_COUNT,
        primary_statistic=RepeatabilityStatistic.MEAN_PAIRWISE_AGREEMENT,
        secondary_statistics=secondary,
        bootstrap_iterations=bootstrap_iterations,
        bootstrap_seed=bootstrap_seed,
        content_hash=content_hash,
    )


def compute_pairwise_agreement(outcomes: list[bool]) -> tuple[int, int]:
    """Compute pairwise agreement count and total pairs.

    Returns (agreements, total_pairs) where agreements is the number
    of pairs (i, j) with outcomes[i] == outcomes[j] and total_pairs is
    C(n, 2) for n outcomes.
    """
    n = len(outcomes)
    total = n * (n - 1) // 2
    agreements = 0
    for i in range(n):
        for j in range(i + 1, n):
            if outcomes[i] == outcomes[j]:
                agreements += 1
    return agreements, total


def classify_repeatability(
    outcomes: list[bool],
) -> RepeatabilityOutcome:
    """Classify the repeatability outcome for a task.

    Returns ``INSUFFICIENT`` when fewer than five repetitions have
    terminal outcomes, ``CONSISTENTLY_CORRECT`` when all five agree
    and the majority is correct, ``CONSISTENTLY_WRONG`` when all five
    agree and the majority is wrong, and ``INCONSISTENT`` when not
    all five agree.
    """
    if len(outcomes) < REPETITION_COUNT:
        return RepeatabilityOutcome.INSUFFICIENT
    all_agree = all(o == outcomes[0] for o in outcomes)
    if all_agree:
        if outcomes[0]:
            return RepeatabilityOutcome.CONSISTENTLY_CORRECT
        return RepeatabilityOutcome.CONSISTENTLY_WRONG
    return RepeatabilityOutcome.INCONSISTENT


def build_task_repeatability_record(
    task_id: str,
    outcomes: list[bool],
) -> TaskRepeatabilityRecord:
    """Build a repeatability record for one task.

    Computes pairwise agreement, all-five-agree, majority outcome,
    and typed repeatability outcome from the per-repetition binary
    outcomes.
    """
    agreements, total_pairs = compute_pairwise_agreement(outcomes)
    all_agree = len(outcomes) >= REPETITION_COUNT and all(o == outcomes[0] for o in outcomes)
    majority_correct = sum(outcomes) > len(outcomes) // 2 if outcomes else False
    outcome = classify_repeatability(outcomes)
    return TaskRepeatabilityRecord(
        task_id=task_id,
        repetition_outcomes=outcomes,
        pairwise_agreements=agreements,
        total_pairs=total_pairs,
        all_agree=all_agree,
        majority_correct=majority_correct,
        outcome=outcome,
    )


__all__ = [
    "REPEATABILITY_POLICY_VERSION",
    "REPETITION_COUNT",
    "RepeatabilityOutcome",
    "RepeatabilityPolicy",
    "RepeatabilityStatistic",
    "RepeatabilitySummary",
    "TaskRepeatabilityRecord",
    "build_repeatability_policy",
    "build_task_repeatability_record",
    "classify_repeatability",
    "compute_pairwise_agreement",
    "compute_repeatability_policy_hash",
]
