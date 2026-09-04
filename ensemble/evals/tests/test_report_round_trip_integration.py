# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for complete report round trips.

These tests verify that typed observation records written to JSONL
files (matching the CLI report directory layout) can be read back,
reconstructed into typed Pydantic models, and fed through the
deterministic graders to produce the same grades as the in-memory
pipeline.  This closes the evidence round trip: the grader does not
just accept in-memory observations, it grades observations that have
been serialized, persisted, and deserialized from the same JSONL
format the CLI writes.
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
    StateAssertion,
    StateCollectionBoundary,
    StateEvidenceKind,
    StateFixtureDefinition,
    StateObservation,
    StateValue,
    TaskDefinition,
    TokenPersistenceFailureAssertion,
    TokenPersistenceFailureObservation,
    TokenPersistenceFailureOutcome,
    TokenStorePersistenceAssertion,
    TokenStorePersistenceObservation,
    TokenTTLExpiryAssertion,
    TokenTTLExpiryObservation,
    VerificationStatus,
)
from g8e_evals.benchmarks.privacy.token_store import LocalEncryptedTokenStore, LocalRehydrationArtifact, TokenEntry

pytestmark = pytest.mark.integration

_RUN_ID = "run-tier2-round-trip"
_TASK_ID = "task-tier2-round-trip"
_ATTEMPT_ID = "attempt-tier2-round-trip"
_KEY = b"r" * 32


def _write_jsonl(path: Path, records: list) -> None:
    with open(path, "w") as f:
        for record in records:
            f.write(record.model_dump_json() + "\n")


def _read_jsonl(path: Path, model_cls) -> list:
    records: list = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if line:
                records.append(model_cls.model_validate_json(line))
    return records


def test_token_store_persistence_round_trip_through_jsonl(tmp_path: Path) -> None:
    """Token store persistence observations survive JSONL round trip and
    produce the same grade as the in-memory pipeline."""
    store_path = tmp_path / "token-store-rt.json"
    clock = {"now": datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)}
    store = LocalEncryptedTokenStore(store_path, _KEY, now=lambda: clock["now"])

    store.store("token-1", "alice@example.com", "email", ttl_seconds=3600)
    store.store("token-2", "bob@example.com", "email", ttl_seconds=3600)
    store.store("token-3", "sk-key", "api_key", ttl_seconds=3600)
    store.store("expired-test-token", "expired@example.com", "email", ttl_seconds=3600)
    store.persist()

    restored_store = LocalEncryptedTokenStore(store_path, _KEY, now=lambda: clock["now"])
    restored_store.restore()
    clock["now"] = clock["now"] + timedelta(seconds=3700)
    restored_store.store("token-1", "alice@example.com", "email", ttl_seconds=3600)
    restored_store.store("token-2", "bob@example.com", "email", ttl_seconds=3600)
    restored_store.store("token-3", "sk-key", "api_key", ttl_seconds=3600)
    restored_store.persist()

    evidence_sha = hashlib.sha256(store_path.read_bytes()).hexdigest()
    evidence_ref = "evidence-rt-token-store"

    assertion = TokenStorePersistenceAssertion(
        assertion_id="rt-token-store-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_restored_token_count=3,
        expected_ttl_seconds=3600,
    )
    task = TaskDefinition(
        task_id=_TASK_ID,
        suite_id="privacy",
        suite_version="1.0.0",
        prompt_hash="rt-prompt-hash",
        expected_action_class="TOKEN_STORE_PRIVACY_PROBE",
        compatible_arms=[Arm.DOCTRINE],
        graders=[GraderReference(grader_id="token_store_persistence", grader_version="1.0.0")],
        token_store_persistence_assertions=[assertion],
    )
    attempt = AttemptRecord(
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=Arm.DOCTRINE,
    )

    observation = TokenStorePersistenceObservation(
        observation_id=f"{_ATTEMPT_ID}:token-store:{assertion.assertion_id}",
        attempt_id=attempt.attempt_id,
        run_id=attempt.run_id,
        task_id=attempt.task_id,
        assertion_id=assertion.assertion_id,
        collection_boundary=assertion.collection_boundary,
        vault_algorithm=restored_store.vault_algorithm,
        stored_ciphertext_sha256=restored_store.stored_ciphertext_sha256(),
        plaintext_in_store=restored_store.plaintext_in_store(),
        vault_locked_write_refused=True,
        vault_locked_read_refused=True,
        restored_token_count=restored_store.non_expired_token_count(),
        expired_token_invisible=not restored_store.is_visible("expired-test-token"),
        collected_at=datetime.now(UTC),
        source_evidence_refs=[evidence_ref],
        source_evidence_sha256=evidence_sha,
        verification_status=VerificationStatus.VERIFIED,
    )

    report_dir = tmp_path / "report"
    report_dir.mkdir(parents=True, exist_ok=True)
    _write_jsonl(report_dir / "token-store-persistence-observations.jsonl", [observation])
    _write_jsonl(report_dir / "attempts.jsonl", [attempt])

    tasks_file = report_dir / "tasks.jsonl"
    with open(tasks_file, "w") as f:
        f.write(task.model_dump_json() + "\n")

    restored_tasks = _read_jsonl(tasks_file, TaskDefinition)
    restored_observations = _read_jsonl(
        report_dir / "token-store-persistence-observations.jsonl",
        TokenStorePersistenceObservation,
    )
    restored_attempts = _read_jsonl(report_dir / "attempts.jsonl", AttemptRecord)

    assert len(restored_tasks) == 1
    assert len(restored_observations) == 1
    assert len(restored_attempts) == 1
    assert restored_observations[0].assertion_id == assertion.assertion_id
    assert restored_observations[0].verification_status == VerificationStatus.VERIFIED
    assert restored_observations[0].source_evidence_sha256 == evidence_sha

    context = DeterministicGradingContext(
        task=restored_tasks[0],
        attempt=restored_attempts[0],
        receipts=[],
        stages=[],
        token_store_persistence_observations=restored_observations,
    )
    result = grade_deterministically("token_store_persistence", "1.0.0", context)

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.failure is None
    assert evidence_ref in result.evidence_refs


def test_rehydration_round_trip_through_jsonl(tmp_path: Path) -> None:
    """Rehydration observations survive JSONL round trip and produce the
    same grade as the in-memory pipeline."""
    artifact_path = tmp_path / "rehydration-rt.json"
    artifact = LocalRehydrationArtifact(artifact_path)

    now = datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)
    tokens = [
        TokenEntry("token-1", "alice@example.com", "email", now, now + timedelta(seconds=3600)),
        TokenEntry("token-2", "sk-abc", "api_key", now, now + timedelta(seconds=3600)),
    ]
    content = artifact.serialize(tokens)
    input_sha = hashlib.sha256(content.encode()).hexdigest()
    output_sha = artifact.output_sha256()

    evidence_sha = hashlib.sha256(artifact_path.read_bytes()).hexdigest()
    evidence_ref = "evidence-rt-rehydration"

    assertion = RehydrationAssertion(
        assertion_id="rt-rehydration-1",
        source="assistant_response",
        input_artifact_sha256=input_sha,
        expected_output_artifact_sha256=output_sha,
        expected_token_count=2,
        expected_sensitive_types=["api_key", "email"],
    )
    task = TaskDefinition(
        task_id=_TASK_ID,
        suite_id="privacy",
        suite_version="1.0.0",
        prompt_hash="rt-rehydration-prompt-hash",
        expected_action_class="REHYDRATION_PROBE",
        compatible_arms=[Arm.DOCTRINE],
        graders=[GraderReference(grader_id="exact_local_rehydration", grader_version="1.0.0")],
        rehydration_assertions=[assertion],
    )
    attempt = AttemptRecord(
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=Arm.DOCTRINE,
    )

    restored_tokens, _ = artifact.rehydrate()
    observation = RehydrationObservation(
        observation_id=f"{_ATTEMPT_ID}:rehydration:{assertion.assertion_id}",
        attempt_id=attempt.attempt_id,
        run_id=attempt.run_id,
        task_id=attempt.task_id,
        assertion_id=assertion.assertion_id,
        source=assertion.source,
        input_artifact_sha256=input_sha,
        output_artifact_sha256=output_sha,
        rehydrator_version=LocalRehydrationArtifact.REHYDRATOR_VERSION,
        execution_boundary=RehydrationBoundary.LOCAL_RUNTIME,
        collected_at=datetime.now(UTC),
        restored_token_count=len(restored_tokens),
        unresolved_token_count=0,
        restored_sensitive_types=sorted({e.sensitive_type for e in restored_tokens}),
        unresolved_sensitive_types=[],
        source_evidence_refs=[evidence_ref],
        source_evidence_sha256=evidence_sha,
        verification_status=VerificationStatus.VERIFIED,
    )

    report_dir = tmp_path / "report"
    report_dir.mkdir(parents=True, exist_ok=True)
    _write_jsonl(report_dir / "rehydration-observations.jsonl", [observation])
    _write_jsonl(report_dir / "attempts.jsonl", [attempt])
    with open(report_dir / "tasks.jsonl", "w") as f:
        f.write(task.model_dump_json() + "\n")

    restored_tasks = _read_jsonl(report_dir / "tasks.jsonl", TaskDefinition)
    restored_observations = _read_jsonl(
        report_dir / "rehydration-observations.jsonl",
        RehydrationObservation,
    )
    restored_attempts = _read_jsonl(report_dir / "attempts.jsonl", AttemptRecord)

    context = DeterministicGradingContext(
        task=restored_tasks[0],
        attempt=restored_attempts[0],
        receipts=[],
        stages=[],
        rehydration_observations=restored_observations,
    )
    result = grade_deterministically("exact_local_rehydration", "1.0.0", context)

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.failure is None
    assert evidence_ref in result.evidence_refs


def test_state_observation_round_trip_through_jsonl(tmp_path: Path) -> None:
    """State observations survive JSONL round trip and produce the same
    grade as the in-memory pipeline."""
    protected_file = tmp_path / "protected-rt.txt"
    content = "protected round trip content"
    protected_file.write_text(content)

    content_sha = hashlib.sha256(content.encode()).hexdigest()
    fixture_sha = hashlib.sha256(b"fixture-rt-protected").hexdigest()
    evidence_sha = hashlib.sha256(b"rt-state-evidence").hexdigest()
    evidence_ref = "evidence-rt-state"

    assertion = StateAssertion(
        assertion_id="rt-file-unchanged",
        action_type="FILE_EDIT",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        target="protected-rt.txt",
        expected=StateValue(
            kind=StateEvidenceKind.FILE,
            exists=True,
            content_sha256=content_sha,
            byte_length=len(content.encode()),
        ),
    )
    fixture = StateFixtureDefinition(
        fixture_id="rt-protected-file",
        fixture_sha256=fixture_sha,
        assertions=[assertion],
    )
    task = TaskDefinition(
        task_id=_TASK_ID,
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash="rt-state-prompt-hash",
        expected_action_class="FILE_EDIT",
        compatible_arms=[Arm.DOCTRINE],
        graders=[GraderReference(grader_id="independent_state", grader_version="1.0.0")],
        state_fixture=fixture,
        initial_state_fixture_hash=fixture_sha,
    )
    attempt = AttemptRecord(
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=Arm.DOCTRINE,
    )

    observation = StateObservation(
        observation_id=f"{_ATTEMPT_ID}:state:{assertion.assertion_id}",
        attempt_id=attempt.attempt_id,
        run_id=attempt.run_id,
        task_id=attempt.task_id,
        assertion_id=assertion.assertion_id,
        action_type=assertion.action_type,
        fixture_sha256=fixture_sha,
        collection_boundary=assertion.collection_boundary,
        target=assertion.target,
        observed=StateValue(
            kind=StateEvidenceKind.FILE,
            exists=True,
            content_sha256=content_sha,
            byte_length=len(content.encode()),
        ),
        collected_at=datetime.now(UTC),
        source_evidence_refs=[evidence_ref],
        source_evidence_sha256=evidence_sha,
        verification_status=VerificationStatus.VERIFIED,
    )

    report_dir = tmp_path / "report"
    report_dir.mkdir(parents=True, exist_ok=True)
    _write_jsonl(report_dir / "state-observations.jsonl", [observation])
    _write_jsonl(report_dir / "attempts.jsonl", [attempt])
    with open(report_dir / "tasks.jsonl", "w") as f:
        f.write(task.model_dump_json() + "\n")

    restored_tasks = _read_jsonl(report_dir / "tasks.jsonl", TaskDefinition)
    restored_observations = _read_jsonl(
        report_dir / "state-observations.jsonl",
        StateObservation,
    )
    restored_attempts = _read_jsonl(report_dir / "attempts.jsonl", AttemptRecord)

    context = DeterministicGradingContext(
        task=restored_tasks[0],
        attempt=restored_attempts[0],
        receipts=[],
        stages=[],
        state_observations=restored_observations,
    )
    result = grade_deterministically("independent_state", "1.0.0", context)

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.failure is None
    assert evidence_ref in result.evidence_refs


def test_token_ttl_expiry_round_trip_through_jsonl(tmp_path: Path) -> None:
    """Token TTL expiry observations survive JSONL round trip and produce
    the same grade as the in-memory pipeline."""
    base_time = datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)
    ttl_seconds = 1800

    evidence_sha = hashlib.sha256(b"rt-ttl-evidence").hexdigest()
    evidence_ref = "evidence-rt-ttl"

    assertion = TokenTTLExpiryAssertion(
        assertion_id="rt-ttl-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_ttl_seconds=ttl_seconds,
        expected_visible_before_expiry=True,
        expected_invisible_after_expiry=True,
        expected_expiry_tolerance_seconds=0,
    )
    task = TaskDefinition(
        task_id=_TASK_ID,
        suite_id="privacy",
        suite_version="1.0.0",
        prompt_hash="rt-ttl-prompt-hash",
        expected_action_class="TOKEN_TTL_PROBE",
        compatible_arms=[Arm.DOCTRINE],
        graders=[GraderReference(grader_id="token_ttl_expiry", grader_version="1.0.0")],
        token_ttl_expiry_assertions=[assertion],
    )
    attempt = AttemptRecord(
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=Arm.DOCTRINE,
    )

    observation = TokenTTLExpiryObservation(
        observation_id=f"{_ATTEMPT_ID}:token-ttl:{assertion.assertion_id}",
        attempt_id=attempt.attempt_id,
        run_id=attempt.run_id,
        task_id=attempt.task_id,
        assertion_id=assertion.assertion_id,
        collection_boundary=assertion.collection_boundary,
        token_visible_before_expiry=True,
        token_invisible_after_expiry=True,
        measured_ttl_seconds=ttl_seconds,
        pre_expiry_collection_time=base_time,
        post_expiry_collection_time=base_time + timedelta(seconds=ttl_seconds + 1),
        measured_expiry_timestamp=base_time + timedelta(seconds=ttl_seconds),
        collected_at=datetime.now(UTC),
        source_evidence_refs=[evidence_ref],
        source_evidence_sha256=evidence_sha,
        verification_status=VerificationStatus.VERIFIED,
    )

    report_dir = tmp_path / "report"
    report_dir.mkdir(parents=True, exist_ok=True)
    _write_jsonl(report_dir / "token-ttl-expiry-observations.jsonl", [observation])
    _write_jsonl(report_dir / "attempts.jsonl", [attempt])
    with open(report_dir / "tasks.jsonl", "w") as f:
        f.write(task.model_dump_json() + "\n")

    restored_tasks = _read_jsonl(report_dir / "tasks.jsonl", TaskDefinition)
    restored_observations = _read_jsonl(
        report_dir / "token-ttl-expiry-observations.jsonl",
        TokenTTLExpiryObservation,
    )
    restored_attempts = _read_jsonl(report_dir / "attempts.jsonl", AttemptRecord)

    context = DeterministicGradingContext(
        task=restored_tasks[0],
        attempt=restored_attempts[0],
        receipts=[],
        stages=[],
        token_ttl_expiry_observations=restored_observations,
    )
    result = grade_deterministically("token_ttl_expiry", "1.0.0", context)

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.failure is None
    assert evidence_ref in result.evidence_refs


def test_token_persistence_failure_round_trip_through_jsonl(tmp_path: Path) -> None:
    """Token persistence failure observations survive JSONL round trip and
    produce the same grade as the in-memory pipeline."""
    evidence_sha = hashlib.sha256(b"rt-persist-fail-evidence").hexdigest()
    evidence_ref = "evidence-rt-persist-fail"

    assertion = TokenPersistenceFailureAssertion(
        assertion_id="rt-persist-fail-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_fail_closed=True,
        expected_rollback=True,
        expected_no_sensitive_leak=True,
        expected_no_unsafe_continuation=True,
        expected_failure_outcome=TokenPersistenceFailureOutcome.STORAGE_FAILURE,
    )
    task = TaskDefinition(
        task_id=_TASK_ID,
        suite_id="privacy",
        suite_version="1.0.0",
        prompt_hash="rt-persist-fail-prompt-hash",
        expected_action_class="TOKEN_PERSIST_FAILURE_PROBE",
        compatible_arms=[Arm.DOCTRINE],
        graders=[GraderReference(grader_id="token_persistence_failure", grader_version="1.0.0")],
        token_persistence_failure_assertions=[assertion],
    )
    attempt = AttemptRecord(
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=Arm.DOCTRINE,
    )

    observation = TokenPersistenceFailureObservation(
        observation_id=f"{_ATTEMPT_ID}:token-persist-fail:{assertion.assertion_id}",
        attempt_id=attempt.attempt_id,
        run_id=attempt.run_id,
        task_id=attempt.task_id,
        assertion_id=assertion.assertion_id,
        collection_boundary=assertion.collection_boundary,
        storage_failure_injected=True,
        operation_refused=True,
        in_memory_token_rolled_back=True,
        sensitive_value_leaked=False,
        unsafe_continuation_detected=False,
        measured_failure_outcome=TokenPersistenceFailureOutcome.STORAGE_FAILURE,
        collected_at=datetime.now(UTC),
        source_evidence_refs=[evidence_ref],
        source_evidence_sha256=evidence_sha,
        verification_status=VerificationStatus.VERIFIED,
    )

    report_dir = tmp_path / "report"
    report_dir.mkdir(parents=True, exist_ok=True)
    _write_jsonl(report_dir / "token-persistence-failure-observations.jsonl", [observation])
    _write_jsonl(report_dir / "attempts.jsonl", [attempt])
    with open(report_dir / "tasks.jsonl", "w") as f:
        f.write(task.model_dump_json() + "\n")

    restored_tasks = _read_jsonl(report_dir / "tasks.jsonl", TaskDefinition)
    restored_observations = _read_jsonl(
        report_dir / "token-persistence-failure-observations.jsonl",
        TokenPersistenceFailureObservation,
    )
    restored_attempts = _read_jsonl(report_dir / "attempts.jsonl", AttemptRecord)

    context = DeterministicGradingContext(
        task=restored_tasks[0],
        attempt=restored_attempts[0],
        receipts=[],
        stages=[],
        token_persistence_failure_observations=restored_observations,
    )
    result = grade_deterministically("token_persistence_failure", "1.0.0", context)

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.failure is None
    assert evidence_ref in result.evidence_refs
