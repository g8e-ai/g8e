# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Protocol

from g8e_evals.schema import (
    AttemptRecord,
    FinalStateObservation,
    ReceiptObservation,
    StateAssertionPredicate,
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
    final_state_observations: list[FinalStateObservation] = field(default_factory=list)


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


def observe_receipt_final_state(
    task: TaskDefinition,
    attempt: AttemptRecord,
    receipts: list[ReceiptObservation],
) -> list[FinalStateObservation]:
    observations = []
    for assertion in task.expected_final_state_assertions:
        matching_receipts = [
            receipt for receipt in receipts if receipt.action_type == assertion.action_type
        ]
        receipt = matching_receipts[0] if len(matching_receipts) == 1 else None
        complete = bool(
            receipt
            and receipt.verified
            and receipt.action_receipt.state_root_before
            and receipt.action_receipt.state_root_after
        )
        observations.append(FinalStateObservation(
            observation_id=f"{attempt.attempt_id}:final-state:{assertion.assertion_id}",
            attempt_id=attempt.attempt_id,
            run_id=attempt.run_id,
            task_id=attempt.task_id,
            assertion_id=assertion.assertion_id,
            action_type=assertion.action_type,
            state_root_before=receipt.action_receipt.state_root_before if receipt else None,
            state_root_after=receipt.action_receipt.state_root_after if receipt else None,
            source_receipt_id=receipt.receipt_id if receipt else None,
            verification_status=(
                VerificationStatus.VERIFIED if complete else VerificationStatus.FAILED
            ),
        ))
    return observations


class FinalStateAssertionGrader:
    def grade(self, context: DeterministicGradingContext) -> DeterministicGrade:
        assertions = context.task.expected_final_state_assertions
        if not assertions:
            return self._failed("expected final-state assertions are missing")

        observations_by_assertion: dict[str, list[FinalStateObservation]] = {}
        for observation in context.final_state_observations:
            observations_by_assertion.setdefault(observation.assertion_id, []).append(observation)

        evidence_refs = []
        failed_assertions = []
        for assertion in assertions:
            observations = observations_by_assertion.get(assertion.assertion_id, [])
            if len(observations) != 1:
                return self._failed(
                    f"exactly one final-state observation is required: {assertion.assertion_id}"
                )
            observation = observations[0]
            evidence_refs.append(observation.observation_id)
            if (
                observation.attempt_id != context.attempt.attempt_id
                or observation.run_id != context.attempt.run_id
                or observation.task_id != context.task.task_id
            ):
                return self._failed(
                    f"final-state observation context does not match attempt: {assertion.assertion_id}",
                    evidence_refs,
                )
            if observation.action_type != assertion.action_type:
                return self._failed(
                    f"final-state observation action does not match assertion: {assertion.assertion_id}",
                    evidence_refs,
                )
            if observation.verification_status != VerificationStatus.VERIFIED:
                return self._failed(
                    f"final-state observation is unverified: {assertion.assertion_id}",
                    evidence_refs,
                )
            if not observation.state_root_before or not observation.state_root_after:
                return self._failed(
                    f"final-state observation is incomplete: {assertion.assertion_id}",
                    evidence_refs,
                )

            source_receipts = [
                receipt
                for receipt in context.receipts
                if receipt.receipt_id == observation.source_receipt_id
            ]
            if len(source_receipts) != 1 or not source_receipts[0].verified:
                return self._failed(
                    f"verified source receipt is missing: {assertion.assertion_id}",
                    evidence_refs,
                )
            source_receipt = source_receipts[0]
            if (
                source_receipt.action_type != assertion.action_type
                or source_receipt.action_receipt.state_root_before != observation.state_root_before
                or source_receipt.action_receipt.state_root_after != observation.state_root_after
            ):
                return self._failed(
                    f"final-state observation does not match source receipt: {assertion.assertion_id}",
                    evidence_refs,
                )
            evidence_refs.append(source_receipt.receipt_id)

            changed = observation.state_root_before != observation.state_root_after
            satisfied = (
                changed
                if assertion.predicate == StateAssertionPredicate.STATE_ROOT_CHANGED
                else not changed
            )
            if not satisfied:
                failed_assertions.append(assertion.assertion_id)

        value = (len(assertions) - len(failed_assertions)) / len(assertions)
        return DeterministicGrade(
            value=value,
            verification_status=VerificationStatus.VERIFIED,
            evidence_refs=list(dict.fromkeys(evidence_refs)),
            failure=(
                f"final-state assertion failed: {failed_assertions[0]}"
                if failed_assertions
                else None
            ),
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
    ("final_state_assertions", "1.0.0"): FinalStateAssertionGrader(),
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
    "observe_receipt_final_state",
]
