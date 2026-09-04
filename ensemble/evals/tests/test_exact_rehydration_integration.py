# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for exact local rehydration grading.

These tests exercise the ``exact_local_rehydration@1.0.0`` grader
against a real local rehydration artifact on disk.  The rehydrator
serializes token mappings to a JSON artifact, computes content hashes,
and deserializes them back.  Observations are produced by a real
observer that interacts with the artifact, and the grader consumes
those observations to verify exact restoration.
"""

from __future__ import annotations

import hashlib
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from g8e_evals.graders import (
    DeterministicGradingContext,
    grade_deterministically,
)
from g8e_evals.schema import (
    Arm,
    AttemptRecord,
    GraderReference,
    RehydrationAssertion,
    RehydrationBoundary,
    RehydrationObservation,
    TaskDefinition,
    VerificationStatus,
)
from local_encrypted_token_store import LocalRehydrationArtifact, TokenEntry

pytestmark = pytest.mark.integration

_RUN_ID = "run-tier2-rehydration"
_TASK_ID = "task-tier2-rehydration"
_ATTEMPT_ID = "attempt-tier2-rehydration"


def _task_def(assertions: list[RehydrationAssertion]) -> TaskDefinition:
    return TaskDefinition(
        task_id=_TASK_ID,
        suite_id="privacy",
        suite_version="1.0.0",
        prompt_hash="tier2-rehydration-prompt-hash",
        expected_action_class="REHYDRATION_PROBE",
        compatible_arms=[Arm.DOCTRINE],
        graders=[GraderReference(grader_id="exact_local_rehydration", grader_version="1.0.0")],
        rehydration_assertions=assertions,
    )


def _attempt() -> AttemptRecord:
    return AttemptRecord(
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=Arm.DOCTRINE,
    )


class RealRehydrationObserver:
    """Observes a real local rehydration artifact and produces typed observations."""

    def __init__(self, artifact: LocalRehydrationArtifact, evidence_sha: str, evidence_ref: str) -> None:
        self._artifact = artifact
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[RehydrationObservation]:
        observations: list[RehydrationObservation] = []
        for assertion in task.rehydration_assertions:
            restored, unresolved = self._artifact.rehydrate()
            restored_types = sorted({e.sensitive_type for e in restored})
            unresolved_types = sorted(unresolved)
            observations.append(RehydrationObservation(
                observation_id=f"{attempt.attempt_id}:rehydration:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                source=assertion.source,
                input_artifact_sha256=assertion.input_artifact_sha256,
                output_artifact_sha256=self._artifact.output_sha256(),
                rehydrator_version=LocalRehydrationArtifact.REHYDRATOR_VERSION,
                execution_boundary=RehydrationBoundary.LOCAL_RUNTIME,
                collected_at=datetime.now(UTC),
                restored_token_count=len(restored),
                unresolved_token_count=len(unresolved),
                restored_sensitive_types=restored_types,
                unresolved_sensitive_types=unresolved_types,
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))
        return observations


def test_exact_rehydration_grader_passes_with_real_artifact(tmp_path: Path) -> None:
    """The grader passes when a real rehydration artifact restores all tokens
    with matching content hashes, token counts, and sensitive types."""
    artifact_path = tmp_path / "rehydration-artifact.json"
    artifact = LocalRehydrationArtifact(artifact_path)

    now = datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)
    tokens = [
        TokenEntry("token-1", "alice@example.com", "email", now, now + timedelta(seconds=3600)),
        TokenEntry("token-2", "bob@example.com", "email", now, now + timedelta(seconds=3600)),
        TokenEntry("token-3", "sk-1234567890", "api_key", now, now + timedelta(seconds=3600)),
    ]
    content = artifact.serialize(tokens)
    input_sha = hashlib.sha256(content.encode()).hexdigest()
    output_sha = artifact.output_sha256()
    assert input_sha == output_sha, "input and output hashes must match for exact rehydration"

    evidence_sha = hashlib.sha256(artifact_path.read_bytes()).hexdigest()
    evidence_ref = "evidence-rehydration-1"

    assertion = RehydrationAssertion(
        assertion_id="rehydration-1",
        source="assistant_response",
        input_artifact_sha256=input_sha,
        expected_output_artifact_sha256=output_sha,
        expected_token_count=3,
        expected_sensitive_types=["api_key", "email"],
    )
    task = _task_def([assertion])
    observer = RealRehydrationObserver(artifact, evidence_sha, evidence_ref)
    observations = asyncio_run(observer.observe(task, _attempt()))

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        rehydration_observations=observations,
    )
    result = grade_deterministically("exact_local_rehydration", "1.0.0", context)

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.denominator_contribution == 1
    assert result.failure is None
    assert evidence_ref in result.evidence_refs


def test_exact_rehydration_grader_fails_when_token_count_mismatches(tmp_path: Path) -> None:
    """The grader fails when the restored token count does not match the assertion."""
    artifact_path = tmp_path / "rehydration-artifact-mismatch.json"
    artifact = LocalRehydrationArtifact(artifact_path)

    now = datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)
    tokens = [
        TokenEntry("token-1", "alice@example.com", "email", now, now + timedelta(seconds=3600)),
    ]
    content = artifact.serialize(tokens)
    input_sha = hashlib.sha256(content.encode()).hexdigest()
    output_sha = artifact.output_sha256()

    evidence_sha = hashlib.sha256(artifact_path.read_bytes()).hexdigest()
    evidence_ref = "evidence-rehydration-mismatch"

    assertion = RehydrationAssertion(
        assertion_id="rehydration-mismatch-1",
        source="assistant_response",
        input_artifact_sha256=input_sha,
        expected_output_artifact_sha256=output_sha,
        expected_token_count=5,
        expected_sensitive_types=["email"],
    )
    task = _task_def([assertion])
    observer = RealRehydrationObserver(artifact, evidence_sha, evidence_ref)
    observations = asyncio_run(observer.observe(task, _attempt()))

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        rehydration_observations=observations,
    )
    result = grade_deterministically("exact_local_rehydration", "1.0.0", context)

    assert result.value == 0.0
    assert result.failure is not None


def test_exact_rehydration_grader_fails_when_sensitive_types_mismatch(tmp_path: Path) -> None:
    """The grader fails when the restored sensitive types do not match the assertion."""
    artifact_path = tmp_path / "rehydration-artifact-types.json"
    artifact = LocalRehydrationArtifact(artifact_path)

    now = datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)
    tokens = [
        TokenEntry("token-1", "alice@example.com", "email", now, now + timedelta(seconds=3600)),
    ]
    content = artifact.serialize(tokens)
    input_sha = hashlib.sha256(content.encode()).hexdigest()
    output_sha = artifact.output_sha256()

    evidence_sha = hashlib.sha256(artifact_path.read_bytes()).hexdigest()
    evidence_ref = "evidence-rehydration-types"

    assertion = RehydrationAssertion(
        assertion_id="rehydration-types-1",
        source="assistant_response",
        input_artifact_sha256=input_sha,
        expected_output_artifact_sha256=output_sha,
        expected_token_count=1,
        expected_sensitive_types=["api_key"],
    )
    task = _task_def([assertion])
    observer = RealRehydrationObserver(artifact, evidence_sha, evidence_ref)
    observations = asyncio_run(observer.observe(task, _attempt()))

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        rehydration_observations=observations,
    )
    result = grade_deterministically("exact_local_rehydration", "1.0.0", context)

    assert result.value == 0.0
    assert result.failure is not None


def asyncio_run(coro):
    import asyncio
    return asyncio.run(coro)
