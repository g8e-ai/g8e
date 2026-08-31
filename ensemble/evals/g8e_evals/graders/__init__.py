# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol

from g8e_evals.schema import (
    AttemptRecord,
    ReceiptObservation,
    StageKind,
    StageObservation,
    TaskDefinition,
    VerificationStatus,
)


class UnsupportedGraderError(ValueError):
    pass


@dataclass(frozen=True)
class DeterministicGradingContext:
    task: TaskDefinition
    attempt: AttemptRecord
    receipts: list[ReceiptObservation]
    stages: list[StageObservation]


@dataclass(frozen=True)
class DeterministicGrade:
    value: float
    verification_status: VerificationStatus
    evidence_refs: list[str]
    failure: str | None = None


class DeterministicGrader(Protocol):
    def grade(self, context: DeterministicGradingContext) -> DeterministicGrade: ...


class ReceiptIntegrityGrader:
    def grade(self, context: DeterministicGradingContext) -> DeterministicGrade:
        expected_action = context.task.expected_action_class
        primary_receipts = [receipt for receipt in context.receipts if receipt.primary]
        if not expected_action:
            return self._failed("expected action class is missing")
        if len(primary_receipts) != 1:
            return self._failed("exactly one primary receipt is required")

        primary = primary_receipts[0]
        if primary.action_type != expected_action:
            return self._failed("primary receipt action does not match the expected action class")
        if not primary.verified:
            return self._failed("primary receipt signature verification failed", [primary.receipt_id])

        persistence_stages = [
            stage
            for stage in context.stages
            if stage.kind == StageKind.RECEIPT_PERSISTENCE
            and stage.transaction_id == primary.transaction_id
            and stage.decision == "verified"
        ]
        if len(persistence_stages) != 1:
            return self._failed("verified final-persistence evidence is missing", [primary.receipt_id])

        return DeterministicGrade(
            value=1.0,
            verification_status=VerificationStatus.VERIFIED,
            evidence_refs=[primary.receipt_id, persistence_stages[0].stage_id],
        )

    @staticmethod
    def _failed(failure: str, evidence_refs: list[str] | None = None) -> DeterministicGrade:
        return DeterministicGrade(
            value=0.0,
            verification_status=VerificationStatus.FAILED,
            evidence_refs=evidence_refs or [],
            failure=failure,
        )


_GRADERS: dict[tuple[str, str], DeterministicGrader] = {
    ("receipt_integrity", "1.0.0"): ReceiptIntegrityGrader(),
}


def grade_deterministically(
    grader_id: str,
    grader_version: str,
    context: DeterministicGradingContext,
) -> DeterministicGrade:
    grader = _GRADERS.get((grader_id, grader_version))
    if grader is None:
        raise UnsupportedGraderError(f"unsupported deterministic grader: {grader_id}@{grader_version}")
    return grader.grade(context)


__all__ = [
    "DeterministicGrade",
    "DeterministicGradingContext",
    "UnsupportedGraderError",
    "grade_deterministically",
]
