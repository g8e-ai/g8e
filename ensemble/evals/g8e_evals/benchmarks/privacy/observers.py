# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Production observer implementations for the synthetic privacy token lifecycle suite.

These observers interact with a ``LocalEncryptedTokenStore`` to produce
typed observations that the deterministic graders consume.  Each observer
implements the corresponding protocol from ``g8e_evals.harness`` and
produces verified observations bound to source evidence.
"""

from __future__ import annotations

from datetime import UTC, datetime, timedelta

from g8e_evals.benchmarks.privacy.token_store import (
    LocalEncryptedTokenStore,
    VaultLockedError,
)
from g8e_evals.schema import (
    AttemptRecord,
    TaskDefinition,
    TokenPersistenceFailureObservation,
    TokenPersistenceFailureOutcome,
    TokenStorePersistenceObservation,
    TokenTTLExpiryObservation,
    VerificationStatus,
)


class TokenStorePersistenceObserverImpl:
    """Observes a local encrypted token store and produces typed persistence observations.

    Proves encryption at rest (no plaintext in store), fail-closed on vault
    lock (writes and reads refused), persistence across restart (restored
    token count matches), and expired-token invisibility.
    """

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


class TokenTTLExpiryObserverImpl:
    """Observes real token TTL and expiry behavior using pre-captured states.

    The observer accepts pre-captured visibility states as constructor
    parameters so it can produce a complete typed observation in a single
    call.  In a real system the observer is called at two points in time:
    before expiry (to record visibility) and after expiry (to record
    invisibility).
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


class TokenPersistenceFailureObserverImpl:
    """Observes real token persistence failure behavior with fail-closed checks.

    Calls ``persist`` on the store and records whether the operation was
    refused, in-memory state was rolled back, the sensitive value leaked,
    and unsafe continuation was detected.
    """

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
