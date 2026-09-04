# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for encrypted token-store privacy grading.

These tests exercise the ``token_store_persistence@1.0.0``,
``token_ttl_expiry@1.0.0``, and ``token_persistence_failure@1.0.0``
graders against a real local encrypted token store on disk.  The store
uses AES-256-GCM for encryption at rest, supports vault lock/unlock,
persists across restarts, and enforces token TTL expiry.  Observations
are produced by real observer implementations that interact with the
store, and the graders consume those observations to produce grades.

This closes the gap between Tier 1 conformance matrices (which use
synthetic observations) and the full evidence round trip: the tests
prove that the graders correctly grade real encrypted-store behavior
including encryption at rest, fail-closed on vault lock, persistence
across restart, expired-token invisibility, TTL visibility windows,
and fail-closed persistence-failure behavior.
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
    StateCollectionBoundary,
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
from local_encrypted_token_store import (
    LocalEncryptedTokenStore,
    VaultLockedError,
)

pytestmark = pytest.mark.integration

_RUN_ID = "run-tier2-token-store"
_TASK_ID = "task-tier2-token-store"
_ATTEMPT_ID = "attempt-tier2-token-store"
_KEY = b"t" * 32


def _task_def(
    *,
    graders: list[GraderReference] | None = None,
    token_store_assertions: list[TokenStorePersistenceAssertion] | None = None,
    ttl_assertions: list[TokenTTLExpiryAssertion] | None = None,
    failure_assertions: list[TokenPersistenceFailureAssertion] | None = None,
) -> TaskDefinition:
    return TaskDefinition(
        task_id=_TASK_ID,
        suite_id="privacy",
        suite_version="1.0.0",
        prompt_hash="tier2-token-store-prompt-hash",
        expected_action_class="TOKEN_STORE_PRIVACY_PROBE",
        compatible_arms=[Arm.DOCTRINE],
        graders=graders or [GraderReference(grader_id="token_store_persistence", grader_version="1.0.0")],
        token_store_persistence_assertions=token_store_assertions or [],
        token_ttl_expiry_assertions=ttl_assertions or [],
        token_persistence_failure_assertions=failure_assertions or [],
    )


def _attempt() -> AttemptRecord:
    return AttemptRecord(
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=Arm.DOCTRINE,
    )


# ---------------------------------------------------------------------------
# Real observer implementations
# ---------------------------------------------------------------------------


class RealTokenStorePersistenceObserver:
    """Observes a real local encrypted token store and produces typed observations."""

    def __init__(self, store: LocalEncryptedTokenStore, evidence_sha: str, evidence_ref: str) -> None:
        self._store = store
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[TokenStorePersistenceObservation]:
        observations: list[TokenStorePersistenceObservation] = []
        for assertion in task.token_store_persistence_assertions:
            # Test vault lock: try write and read while locked
            self._store.lock()
            write_refused = True
            try:
                self._store.store("lock-test", "x", "email", 60)
                write_refused = False
            except VaultLockedError:
                pass
            read_refused = True
            try:
                self._store.retrieve("lock-test")
                read_refused = False
            except VaultLockedError:
                pass
            self._store.unlock()

            observations.append(TokenStorePersistenceObservation(
                observation_id=f"{attempt.attempt_id}:token-store:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                collection_boundary=assertion.collection_boundary,
                vault_algorithm=self._store.vault_algorithm,
                stored_ciphertext_sha256=self._store.stored_ciphertext_sha256(),
                plaintext_in_store=self._store.plaintext_in_store(),
                vault_locked_write_refused=write_refused,
                vault_locked_read_refused=read_refused,
                restored_token_count=self._store.non_expired_token_count(),
                expired_token_invisible=not self._store.is_visible("expired-test-token"),
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))
        return observations


class RealTokenTTLExpiryObserver:
    """Observes real token TTL and expiry behavior using pre-captured states.

    In a real system the observer is called at two points in time: before
    expiry (to record visibility) and after expiry (to record invisibility).
    The observer accepts these pre-captured states as constructor parameters
    so it can produce a complete typed observation in a single call.
    """

    def __init__(
        self,
        store: LocalEncryptedTokenStore,
        evidence_sha: str,
        evidence_ref: str,
        *,
        visible_before_expiry: bool,
        invisible_after_expiry: bool,
    ) -> None:
        self._store = store
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref
        self._visible_before = visible_before_expiry
        self._invisible_after = invisible_after_expiry

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[TokenTTLExpiryObservation]:
        observations: list[TokenTTLExpiryObservation] = []
        for assertion in task.token_ttl_expiry_assertions:
            token_id = f"ttl-test-{assertion.assertion_id}"
            created_at = self._store.token_created_at(token_id)
            expires_at = self._store.token_expiry(token_id)
            if created_at is None or expires_at is None:
                continue
            measured_ttl = int((expires_at - created_at).total_seconds())
            observations.append(TokenTTLExpiryObservation(
                observation_id=f"{attempt.attempt_id}:token-ttl:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                collection_boundary=assertion.collection_boundary,
                token_visible_before_expiry=self._visible_before,
                token_invisible_after_expiry=self._invisible_after,
                measured_ttl_seconds=measured_ttl,
                pre_expiry_collection_time=created_at,
                post_expiry_collection_time=expires_at + timedelta(seconds=1),
                measured_expiry_timestamp=expires_at,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))
        return observations


class RealTokenPersistenceFailureObserver:
    """Observes real token persistence failure behavior with fail-closed checks."""

    def __init__(self, store: LocalEncryptedTokenStore, evidence_sha: str, evidence_ref: str) -> None:
        self._store = store
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[TokenPersistenceFailureObservation]:
        observations: list[TokenPersistenceFailureObservation] = []
        for assertion in task.token_persistence_failure_assertions:
            result = self._store.persist()
            observations.append(TokenPersistenceFailureObservation(
                observation_id=f"{attempt.attempt_id}:token-persist-fail:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                collection_boundary=assertion.collection_boundary,
                storage_failure_injected=True,
                operation_refused=result.operation_refused,
                in_memory_token_rolled_back=result.rolled_back,
                sensitive_value_leaked=result.sensitive_value_leaked,
                unsafe_continuation_detected=result.unsafe_continuation,
                measured_failure_outcome=TokenPersistenceFailureOutcome.STORAGE_FAILURE,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=self._evidence_sha,
                verification_status=VerificationStatus.VERIFIED,
            ))
        return observations


# ---------------------------------------------------------------------------
# Token store persistence grading
# ---------------------------------------------------------------------------


def test_token_store_persistence_grader_passes_with_real_encrypted_store(tmp_path: Path) -> None:
    """The grader passes when a real encrypted store encrypts at rest, fails
    closed on vault lock, persists across restart, and hides expired tokens."""
    store_path = tmp_path / "token-store.json"
    clock = {"now": datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)}
    store = LocalEncryptedTokenStore(store_path, _KEY, now=lambda: clock["now"])

    store.store("token-1", "alice@example.com", "email", ttl_seconds=3600)
    store.store("token-2", "bob@example.com", "email", ttl_seconds=3600)
    store.store("token-3", "secret-key-123", "api_key", ttl_seconds=3600)
    store.store("expired-test-token", "expired@example.com", "email", ttl_seconds=3600)

    store.persist()
    assert not store.plaintext_in_store(), "store must not contain plaintext after persist"

    restored_store = LocalEncryptedTokenStore(store_path, _KEY, now=lambda: clock["now"])
    restored_count = restored_store.restore()
    assert restored_count == 4, f"expected 4 restored tokens, got {restored_count}"

    clock["now"] = clock["now"] + timedelta(seconds=3700)
    assert not restored_store.is_visible("expired-test-token"), "expired token must be invisible"
    restored_store.store("token-1", "alice@example.com", "email", ttl_seconds=3600)
    restored_store.store("token-2", "bob@example.com", "email", ttl_seconds=3600)
    restored_store.store("token-3", "secret-key-123", "api_key", ttl_seconds=3600)
    restored_store.persist()

    evidence_content = store_path.read_bytes()
    evidence_sha = hashlib.sha256(evidence_content).hexdigest()
    evidence_ref = "evidence-token-store-persistence"

    assertion = TokenStorePersistenceAssertion(
        assertion_id="token-store-persist-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_encryption_at_rest=True,
        expected_fail_closed_on_lock=True,
        expected_persistence_across_restart=True,
        expected_ttl_seconds=3600,
        expected_restored_token_count=3,
    )
    task = _task_def(
        graders=[GraderReference(grader_id="token_store_persistence", grader_version="1.0.0")],
        token_store_assertions=[assertion],
    )
    observer = RealTokenStorePersistenceObserver(restored_store, evidence_sha, evidence_ref)
    observations = asyncio_run(observer.observe(task, _attempt()))

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        token_store_persistence_observations=observations,
    )
    result = grade_deterministically("token_store_persistence", "1.0.0", context)

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.denominator_contribution == 1
    assert result.failure is None
    assert evidence_ref in result.evidence_refs


def test_token_store_persistence_grader_fails_when_plaintext_leaks(tmp_path: Path) -> None:
    """The grader fails when plaintext token values appear in the store file."""
    store_path = tmp_path / "token-store-plaintext.json"
    store_path.parent.mkdir(parents=True, exist_ok=True)
    store_path.write_text('{"token-1": "alice@example.com"}')

    store = LocalEncryptedTokenStore(store_path, _KEY)
    store.store("token-1", "alice@example.com", "email", ttl_seconds=3600)

    evidence_sha = hashlib.sha256(store_path.read_bytes()).hexdigest()
    evidence_ref = "evidence-plaintext-leak"

    assertion = TokenStorePersistenceAssertion(
        assertion_id="token-store-plaintext-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_restored_token_count=1,
        expected_ttl_seconds=3600,
    )
    task = _task_def(token_store_assertions=[assertion])
    observer = RealTokenStorePersistenceObserver(store, evidence_sha, evidence_ref)
    observations = asyncio_run(observer.observe(task, _attempt()))

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        token_store_persistence_observations=observations,
    )
    result = grade_deterministically("token_store_persistence", "1.0.0", context)

    assert result.value == 0.0
    assert result.failure is not None


def test_token_store_persistence_grader_fails_when_restored_count_mismatches(tmp_path: Path) -> None:
    """The grader fails when the restored token count does not match the assertion."""
    store_path = tmp_path / "token-store-mismatch.json"
    clock = {"now": datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)}
    store = LocalEncryptedTokenStore(store_path, _KEY, now=lambda: clock["now"])

    store.store("token-1", "alice@example.com", "email", ttl_seconds=3600)
    store.persist()

    evidence_sha = hashlib.sha256(store_path.read_bytes()).hexdigest()
    evidence_ref = "evidence-count-mismatch"

    assertion = TokenStorePersistenceAssertion(
        assertion_id="token-store-mismatch-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_restored_token_count=5,
        expected_ttl_seconds=3600,
    )
    task = _task_def(token_store_assertions=[assertion])
    observer = RealTokenStorePersistenceObserver(store, evidence_sha, evidence_ref)
    observations = asyncio_run(observer.observe(task, _attempt()))

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        token_store_persistence_observations=observations,
    )
    result = grade_deterministically("token_store_persistence", "1.0.0", context)

    assert result.value == 0.0
    assert result.failure is not None


# ---------------------------------------------------------------------------
# Token TTL and expiry grading
# ---------------------------------------------------------------------------


def test_token_ttl_expiry_grader_passes_with_real_ttl_behavior(tmp_path: Path) -> None:
    """The grader passes when a token is visible before expiry, invisible after
    expiry, and the measured TTL matches the declared TTL."""
    store_path = tmp_path / "token-store-ttl.json"
    base_time = datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)
    clock = {"now": base_time}
    store = LocalEncryptedTokenStore(store_path, _KEY, now=lambda: clock["now"])

    ttl_seconds = 1800
    token_id = "ttl-test-token-ttl-1"
    store.store(token_id, "secret@example.com", "email", ttl_seconds=ttl_seconds)

    visible_before = store.is_visible(token_id)
    assert visible_before, "token must be visible before expiry"

    clock["now"] = base_time + timedelta(seconds=ttl_seconds + 1)
    invisible_after = not store.is_visible(token_id)
    assert invisible_after, "token must be invisible after expiry"

    evidence_sha = hashlib.sha256(b"ttl-evidence").hexdigest()
    evidence_ref = "evidence-ttl-expiry"

    assertion = TokenTTLExpiryAssertion(
        assertion_id="token-ttl-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_ttl_seconds=ttl_seconds,
        expected_visible_before_expiry=True,
        expected_invisible_after_expiry=True,
        expected_expiry_tolerance_seconds=0,
    )
    task = _task_def(
        graders=[GraderReference(grader_id="token_ttl_expiry", grader_version="1.0.0")],
        ttl_assertions=[assertion],
    )
    observer = RealTokenTTLExpiryObserver(
        store, evidence_sha, evidence_ref,
        visible_before_expiry=visible_before,
        invisible_after_expiry=invisible_after,
    )
    observations = asyncio_run(observer.observe(task, _attempt()))

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        token_ttl_expiry_observations=observations,
    )
    result = grade_deterministically("token_ttl_expiry", "1.0.0", context)

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.denominator_contribution == 1
    assert result.failure is None


def test_token_ttl_expiry_grader_fails_when_token_visible_after_expiry(tmp_path: Path) -> None:
    """The grader fails when a token is still visible after its TTL expires."""
    store_path = tmp_path / "token-store-ttl-fail.json"
    base_time = datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)
    clock = {"now": base_time}
    store = LocalEncryptedTokenStore(store_path, _KEY, now=lambda: clock["now"])

    ttl_seconds = 100
    token_id = "ttl-test-token-ttl-fail-1"
    store.store(token_id, "secret@example.com", "email", ttl_seconds=ttl_seconds)

    visible_before = store.is_visible(token_id)
    assert visible_before, "token must be visible before expiry"

    clock["now"] = base_time + timedelta(seconds=ttl_seconds + 10)
    invisible_after = not store.is_visible(token_id)
    assert invisible_after, "token should be invisible after expiry"

    evidence_sha = hashlib.sha256(b"ttl-evidence-fail").hexdigest()
    evidence_ref = "evidence-ttl-fail"

    assertion = TokenTTLExpiryAssertion(
        assertion_id="token-ttl-fail-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_ttl_seconds=999,
        expected_visible_before_expiry=True,
        expected_invisible_after_expiry=True,
        expected_expiry_tolerance_seconds=0,
    )
    task = _task_def(
        graders=[GraderReference(grader_id="token_ttl_expiry", grader_version="1.0.0")],
        ttl_assertions=[assertion],
    )
    observer = RealTokenTTLExpiryObserver(
        store, evidence_sha, evidence_ref,
        visible_before_expiry=visible_before,
        invisible_after_expiry=invisible_after,
    )
    observations = asyncio_run(observer.observe(task, _attempt()))

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        token_ttl_expiry_observations=observations,
    )
    result = grade_deterministically("token_ttl_expiry", "1.0.0", context)

    assert result.value == 0.0
    assert result.failure is not None


# ---------------------------------------------------------------------------
# Token persistence failure grading
# ---------------------------------------------------------------------------


def test_token_persistence_failure_grader_passes_with_real_fail_closed(tmp_path: Path) -> None:
    """The grader passes when a storage failure is fail-closed: operation
    refused, in-memory rollback, no sensitive leak, no unsafe continuation."""
    store_path = tmp_path / "token-store-fail-closed.json"
    store = LocalEncryptedTokenStore(store_path, _KEY, fail_persist=True)

    store.store("token-1", "alice@example.com", "email", ttl_seconds=3600)
    store.store("token-2", "bob@example.com", "email", ttl_seconds=3600)

    evidence_sha = hashlib.sha256(b"fail-closed-evidence").hexdigest()
    evidence_ref = "evidence-persist-failure"

    assertion = TokenPersistenceFailureAssertion(
        assertion_id="token-persist-fail-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_fail_closed=True,
        expected_rollback=True,
        expected_no_sensitive_leak=True,
        expected_no_unsafe_continuation=True,
        expected_failure_outcome=TokenPersistenceFailureOutcome.STORAGE_FAILURE,
    )
    task = _task_def(
        graders=[GraderReference(grader_id="token_persistence_failure", grader_version="1.0.0")],
        failure_assertions=[assertion],
    )
    observer = RealTokenPersistenceFailureObserver(store, evidence_sha, evidence_ref)
    observations = asyncio_run(observer.observe(task, _attempt()))

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        token_persistence_failure_observations=observations,
    )
    result = grade_deterministically("token_persistence_failure", "1.0.0", context)

    assert result.value == 1.0
    assert result.verification_status == VerificationStatus.VERIFIED
    assert result.denominator_contribution == 1
    assert result.failure is None
    assert evidence_ref in result.evidence_refs


def test_token_persistence_failure_grader_fails_when_operation_not_refused(tmp_path: Path) -> None:
    """The grader fails when a storage failure does not refuse the operation."""
    store_path = tmp_path / "token-store-no-refuse.json"
    store = LocalEncryptedTokenStore(store_path, _KEY, fail_persist=False)

    store.store("token-1", "alice@example.com", "email", ttl_seconds=3600)

    evidence_sha = hashlib.sha256(b"no-refuse-evidence").hexdigest()
    evidence_ref = "evidence-no-refuse"

    assertion = TokenPersistenceFailureAssertion(
        assertion_id="token-persist-fail-no-refuse-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_fail_closed=True,
        expected_rollback=True,
        expected_no_sensitive_leak=True,
        expected_no_unsafe_continuation=True,
        expected_failure_outcome=TokenPersistenceFailureOutcome.STORAGE_FAILURE,
    )
    task = _task_def(
        graders=[GraderReference(grader_id="token_persistence_failure", grader_version="1.0.0")],
        failure_assertions=[assertion],
    )
    observer = RealTokenPersistenceFailureObserver(store, evidence_sha, evidence_ref)
    observations = asyncio_run(observer.observe(task, _attempt()))

    context = DeterministicGradingContext(
        task=task,
        attempt=_attempt(),
        receipts=[],
        stages=[],
        token_persistence_failure_observations=observations,
    )
    result = grade_deterministically("token_persistence_failure", "1.0.0", context)

    assert result.value == 0.0
    assert result.failure is not None


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def asyncio_run(coro):
    """Run an async coroutine synchronously in a fresh event loop."""
    import asyncio
    return asyncio.run(coro)
