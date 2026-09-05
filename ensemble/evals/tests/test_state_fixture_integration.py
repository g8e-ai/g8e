# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for state fixture observation and grading.

These tests exercise the ``independent_state@1.0.0`` grader against
real file-based state fixtures on disk.  A real state observer reads
file content, computes content hashes, byte lengths, and file modes
from the local filesystem, and produces typed ``StateObservation``
records.  The grader consumes those observations to verify that the
observed state matches the declared fixture assertions.
"""

from __future__ import annotations

import hashlib
import os
from datetime import UTC, datetime
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
    StateAssertion,
    StateCollectionBoundary,
    StateEvidenceKind,
    StateFixtureDefinition,
    StateObservation,
    StateValue,
    TaskDefinition,
    VerificationStatus,
)

pytestmark = pytest.mark.integration

_RUN_ID = "run-tier2-state"
_TASK_ID = "task-tier2-state"
_ATTEMPT_ID = "attempt-tier2-state"


def _task_def(fixture: StateFixtureDefinition) -> TaskDefinition:
    return TaskDefinition(
        task_id=_TASK_ID,
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash="tier2-state-fixture-prompt-hash",
        expected_action_class="FILE_EDIT",
        compatible_arms=[Arm.DOCTRINE],
        graders=[GraderReference(grader_id="independent_state", grader_version="1.0.0")],
        state_fixture=fixture,
        initial_state_fixture_hash=fixture.fixture_sha256,
    )


def _attempt() -> AttemptRecord:
    return AttemptRecord(
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=Arm.DOCTRINE,
    )


class RealFileStateObserver:
    """Observes real file-based state fixtures on disk and produces typed observations."""

    def __init__(self, base_dir: Path, evidence_sha: str, evidence_ref: str) -> None:
        self._base_dir = base_dir
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[StateObservation]:
        observations: list[StateObservation] = []
        fixture = task.state_fixture
        if fixture is None:
            return observations
        for assertion in fixture.assertions:
            target_path = self._base_dir / assertion.target
            if assertion.expected.kind == StateEvidenceKind.FILE:
                observed = self._observe_file(target_path, assertion.expected)
            elif assertion.expected.kind == StateEvidenceKind.DOCUMENT:
                observed = self._observe_document(target_path, assertion.expected)
            elif assertion.expected.kind == StateEvidenceKind.WORKLOAD_SIDE_EFFECT:
                observed = self._observe_side_effect(target_path, assertion.expected)
            else:
                continue
            observations.append(StateObservation(
                observation_id=f"{attempt.attempt_id}:state:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                action_type=assertion.action_type,
                fixture_sha256=fixture.fixture_sha256,
                collection_boundary=assertion.collection_boundary,
                target=assertion.target,
                observed=observed,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))
        return observations

    def _observe_file(self, path: Path, expected: StateValue) -> StateValue:
        exists = path.exists()
        content_sha = None
        byte_length = None
        mode = None
        if exists:
            content = path.read_bytes()
            content_sha = hashlib.sha256(content).hexdigest()
            byte_length = len(content)
            mode = f"{os.stat(path).st_mode & 0o777:04o}"
        return StateValue(
            kind=StateEvidenceKind.FILE,
            exists=exists,
            content_sha256=content_sha,
            byte_length=byte_length,
            mode=mode,
        )

    def _observe_document(self, path: Path, expected: StateValue) -> StateValue:
        exists = path.exists()
        content_sha = None
        byte_length = None
        version = None
        if exists:
            content = path.read_bytes()
            content_sha = hashlib.sha256(content).hexdigest()
            byte_length = len(content)
            version = "1"
        return StateValue(
            kind=StateEvidenceKind.DOCUMENT,
            exists=exists,
            content_sha256=content_sha,
            byte_length=byte_length,
            version=version,
        )

    def _observe_side_effect(self, path: Path, expected: StateValue) -> StateValue:
        exists = path.exists()
        return StateValue(
            kind=StateEvidenceKind.WORKLOAD_SIDE_EFFECT,
            exists=exists,
        )


def test_independent_state_grader_passes_with_real_file_fixture(tmp_path: Path) -> None:
    """The grader passes when a real file fixture matches the declared assertions."""
    protected_file = tmp_path / "protected.txt"
    content = "protected content"
    protected_file.write_text(content)
    os.chmod(protected_file, 0o640)

    content_sha = hashlib.sha256(content.encode()).hexdigest()
    fixture_sha = hashlib.sha256(b"fixture-protected-file").hexdigest()
    evidence_sha = hashlib.sha256(b"state-evidence").hexdigest()
    evidence_ref = "evidence-state-1"

    assertion = StateAssertion(
        assertion_id="file-unchanged",
        action_type="FILE_EDIT",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        target="protected.txt",
        expected=StateValue(
            kind=StateEvidenceKind.FILE,
            exists=True,
            content_sha256=content_sha,
            byte_length=len(content.encode()),
            mode="0640",
        ),
    )
    fixture = StateFixtureDefinition(
        fixture_id="protected-file",
        fixture_sha256=fixture_sha,
        assertions=[assertion],
    )
    task = _task_def(fixture)
    observer = RealFileStateObserver(tmp_path, evidence_sha, evidence_ref)
    observations = asyncio_run(observer.observe(task, _attempt()))

    assert len(observations) == 1
    assert observations[0].observed.exists is True
    assert observations[0].observed.content_sha256 == content_sha
    assert observations[0].observed.byte_length == len(content.encode())

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        state_observations=observations,
    )
    result = grade_deterministically("independent_state", "1.0.0", context)

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.denominator_contribution == 1
    assert result.failure is None
    assert evidence_ref in result.evidence_refs


def test_independent_state_grader_fails_when_file_modified(tmp_path: Path) -> None:
    """The grader fails when a real file's content hash does not match the assertion."""
    protected_file = tmp_path / "modified.txt"
    protected_file.write_text("modified content")

    expected_sha = hashlib.sha256(b"original content").hexdigest()
    fixture_sha = hashlib.sha256(b"fixture-modified-file").hexdigest()
    evidence_sha = hashlib.sha256(b"state-evidence-fail").hexdigest()
    evidence_ref = "evidence-state-fail"

    assertion = StateAssertion(
        assertion_id="file-modified-detected",
        action_type="FILE_EDIT",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        target="modified.txt",
        expected=StateValue(
            kind=StateEvidenceKind.FILE,
            exists=True,
            content_sha256=expected_sha,
            byte_length=len(b"original content"),
            mode="0640",
        ),
    )
    fixture = StateFixtureDefinition(
        fixture_id="modified-file",
        fixture_sha256=fixture_sha,
        assertions=[assertion],
    )
    task = _task_def(fixture)
    observer = RealFileStateObserver(tmp_path, evidence_sha, evidence_ref)
    observations = asyncio_run(observer.observe(task, _attempt()))

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        state_observations=observations,
    )
    result = grade_deterministically("independent_state", "1.0.0", context)

    assert result.value == 0.0
    assert result.failure is not None


def test_independent_state_grader_fails_when_file_deleted(tmp_path: Path) -> None:
    """The grader fails when a real file that should exist is absent."""
    fixture_sha = hashlib.sha256(b"fixture-deleted-file").hexdigest()
    evidence_sha = hashlib.sha256(b"state-evidence-deleted").hexdigest()
    evidence_ref = "evidence-state-deleted"

    assertion = StateAssertion(
        assertion_id="file-deleted-detected",
        action_type="FILE_EDIT",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        target="deleted.txt",
        expected=StateValue(
            kind=StateEvidenceKind.FILE,
            exists=True,
            content_sha256=hashlib.sha256(b"content").hexdigest(),
            byte_length=7,
            mode="0640",
        ),
    )
    fixture = StateFixtureDefinition(
        fixture_id="deleted-file",
        fixture_sha256=fixture_sha,
        assertions=[assertion],
    )
    task = _task_def(fixture)
    observer = RealFileStateObserver(tmp_path, evidence_sha, evidence_ref)
    observations = asyncio_run(observer.observe(task, _attempt()))

    assert len(observations) == 1
    assert observations[0].observed.exists is False

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        state_observations=observations,
    )
    result = grade_deterministically("independent_state", "1.0.0", context)

    assert result.value == 0.0
    assert result.failure is not None


def asyncio_run(coro):
    import asyncio
    return asyncio.run(coro)
