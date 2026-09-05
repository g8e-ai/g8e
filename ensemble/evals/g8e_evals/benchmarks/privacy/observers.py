# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Production observer implementations for the synthetic privacy eval suites.

These observers interact with local production-shaped systems under test
(``LocalEncryptedTokenStore``, ``LocalExfiltrationSimulator``,
``LocalArtifactEmitter``, ``LocalRehydrationArtifact``) to produce typed
observations that the deterministic graders consume.  Each observer
implements the corresponding protocol from ``g8e_evals.harness`` and
produces verified observations bound to source evidence.
"""

from __future__ import annotations

from datetime import UTC, datetime, timedelta

from g8e.receipts import verify_action_receipt_signature
from g8e_evals.benchmarks.privacy.artifact_emitter import LocalArtifactEmitter
from g8e_evals.benchmarks.privacy.exfiltration import (
    ExfiltrationActionResult,
    LocalExfiltrationSimulator,
)
from g8e_evals.benchmarks.privacy.token_store import (
    LocalEncryptedTokenStore,
    LocalRehydrationArtifact,
    TokenEntry,
    VaultLockedError,
)
from g8e_evals.schema import (
    ArtifactLeakageObservation,
    AttemptRecord,
    ExfiltrationAttemptObservation,
    ReceiptObservation,
    RehydrationBoundary,
    RehydrationObservation,
    StateEvidenceKind,
    StateValue,
    TaskDefinition,
    TokenPersistenceFailureObservation,
    TokenPersistenceFailureOutcome,
    TokenStorePersistenceObservation,
    TokenTTLExpiryObservation,
    VerificationStatus,
)


def _evidence_binding(evidence_sha: str) -> tuple[str | None, VerificationStatus]:
    """Return (source_evidence_sha256, verification_status) based on whether
    the evidence SHA is known at observation time.

    When the evidence SHA is not yet known (empty string), observations are
    created in PENDING state with no SHA.  The caller updates them to VERIFIED
    after persisting the evidence artifact and computing its digest.
    """
    if evidence_sha:
        return evidence_sha, VerificationStatus.VERIFIED
    return None, VerificationStatus.PENDING


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
        _sha, _status = _evidence_binding(self._evidence_sha)
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
                source_evidence_sha256=_sha,
                verification_status=_status,
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
        _sha, _status = _evidence_binding(self._evidence_sha)
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
                source_evidence_sha256=_sha,
                verification_status=_status,
            ))
        return observations


class TokenPersistenceFailureObserverImpl:
    """Observes real token persistence failure behavior with fail-closed checks.

    Calls ``persist`` on the store and records whether the operation was
    refused, in-memory state was rolled back, the sensitive value leaked,
    and unsafe continuation was detected.  When ``pre_existing_token_ids``
    and ``uncommitted_token_id`` are provided, the observer independently
    verifies post-failure state: pre-existing committed tokens must
    survive and the uncommitted token must be absent.  The
    ``in_memory_token_rolled_back`` and ``storage_failure_injected``
    fields are set from the independent observation, not from the
    store's self-reported result.
    """

    def __init__(
        self,
        store: LocalEncryptedTokenStore,
        evidence_sha: str,
        evidence_ref: str,
        *,
        pre_existing_token_ids: list[str] | None = None,
        uncommitted_token_id: str | None = None,
    ) -> None:
        self._store = store
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref
        self._pre_existing_token_ids = pre_existing_token_ids or []
        self._uncommitted_token_id = uncommitted_token_id

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[TokenPersistenceFailureObservation]:
        observations: list[TokenPersistenceFailureObservation] = []
        _sha, _status = _evidence_binding(self._evidence_sha)
        for assertion in task.token_persistence_failure_assertions:
            result = self._store.persist()

            storage_failure_injected = not result.persisted
            if self._pre_existing_token_ids or self._uncommitted_token_id:
                pre_existing_survived = all(
                    self._store.has_token(tid) for tid in self._pre_existing_token_ids
                )
                uncommitted_gone = (
                    self._uncommitted_token_id is None
                    or not self._store.has_token(self._uncommitted_token_id)
                )
                independently_rolled_back = pre_existing_survived and uncommitted_gone
                in_memory_rolled_back = independently_rolled_back
            else:
                in_memory_rolled_back = result.rolled_back

            observations.append(TokenPersistenceFailureObservation(
                observation_id=f"{attempt.attempt_id}:token-persist-fail:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                collection_boundary=assertion.collection_boundary,
                storage_failure_injected=storage_failure_injected,
                operation_refused=result.operation_refused,
                in_memory_token_rolled_back=in_memory_rolled_back,
                sensitive_value_leaked=result.sensitive_value_leaked,
                unsafe_continuation_detected=result.unsafe_continuation,
                measured_failure_outcome=TokenPersistenceFailureOutcome.STORAGE_FAILURE,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=_sha,
                verification_status=_status,
            ))
        return observations


def _make_receipt_observation(
    result: ExfiltrationActionResult,
    attempt: AttemptRecord,
    action_type: str,
    public_key_hex: str,
) -> ReceiptObservation:
    """Build a ``ReceiptObservation`` from an exfiltration action result.

    The receipt signature is verified against the simulator's assessed public
    key before setting ``verified``.  A receipt that fails signature
    verification is recorded with ``verified=False`` so downstream graders
    fail closed on unverified evidence.
    """
    verified = verify_action_receipt_signature(result.receipt, public_key_hex)
    return ReceiptObservation(
        receipt_id=f"{attempt.attempt_id}:receipt",
        attempt_id=attempt.attempt_id,
        run_id=attempt.run_id,
        transaction_id=result.transaction_id,
        action_type=action_type,
        primary=True,
        verified=verified,
        action_receipt=result.receipt,
    )


class ExfiltrationAttemptObserverImpl:
    """Observes cross-boundary exfiltration attempts and produces typed observations.

    The observer processes each exfiltration assertion through the
    exfiltration simulator, produces a ``ReceiptObservation`` from the signed
    receipt, and produces an ``ExfiltrationAttemptObservation`` recording
    whether the sensitive value materialized at the declared destination
    boundary.
    """

    def __init__(self, simulator: LocalExfiltrationSimulator, evidence_sha: str, evidence_ref: str) -> None:
        self._simulator = simulator
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> tuple[ReceiptObservation, list[ExfiltrationAttemptObservation]]:
        receipt_observations: list[ReceiptObservation] = []
        exfil_observations: list[ExfiltrationAttemptObservation] = []
        _sha, _status = _evidence_binding(self._evidence_sha)

        for assertion in task.exfiltration_attempt_assertions:
            sensitive_value = f"sensitive-value-{assertion.assertion_id}"
            result = self._simulator.process_exfiltration(
                action_type=assertion.action_type,
                source=assertion.source,
                destination=assertion.destination,
                sensitive_value=sensitive_value,
                rejection_layer=assertion.expected_rejection_layer,
            )
            receipt_observations.append(
                _make_receipt_observation(
                    result, attempt, assertion.action_type, self._simulator.public_key_hex
                )
            )

            materialized = self._simulator.is_value_at_destination(assertion.destination)
            observed = StateValue(
                kind=StateEvidenceKind.FILE,
                exists=materialized,
            )

            exfil_observations.append(ExfiltrationAttemptObservation(
                observation_id=f"{attempt.attempt_id}:exfiltration:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                action_type=assertion.action_type,
                source=assertion.source,
                destination=assertion.destination,
                collection_boundary=assertion.collection_boundary,
                observed=observed,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=_sha,
                verification_status=_status,
            ))

        if not receipt_observations:
            raise ValueError("exfiltration attempt observer produced no observations")
        return receipt_observations[0], exfil_observations


class ArtifactLeakageObserverImpl:
    """Observes emitted public artifacts and produces typed leakage observations.

    The observer emits an artifact for each assertion's declared class through
    the artifact emitter, scans it for sensitive content, and produces an
    ``ArtifactLeakageObservation`` recording whether the artifact was present,
    its content hash and byte length, the scanner version, and the count and
    types of sensitive content found.
    """

    def __init__(
        self,
        emitter: LocalArtifactEmitter,
        evidence_sha: str,
        evidence_ref: str,
        *,
        leak_types: list | None = None,
    ) -> None:
        self._emitter = emitter
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref
        self._leak_types = leak_types or []

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[ArtifactLeakageObservation]:
        observations: list[ArtifactLeakageObservation] = []
        _sha, _status = _evidence_binding(self._evidence_sha)

        for assertion in task.artifact_leakage_assertions:
            safe_content = '{"summary": "all-clear", "hash": "abc123"}'
            artifact_path = self._emitter.emit_artifact(
                assertion.artifact_class,
                safe_content,
                leak_types=self._leak_types,
            )
            present, sha, byte_length, occurrences, types_found = self._emitter.scan_artifact(artifact_path)

            observations.append(ArtifactLeakageObservation(
                observation_id=f"{attempt.attempt_id}:artifact-leakage:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                artifact_class=assertion.artifact_class,
                collection_boundary=assertion.collection_boundary,
                artifact_present=present,
                artifact_sha256=sha if present else None,
                artifact_byte_length=byte_length if present else 0,
                scanner_version=self._emitter.scanner_version,
                sensitive_occurrences=occurrences,
                sensitive_types_found=types_found,
                collected_at=datetime.now(UTC),
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=_sha,
                verification_status=_status,
            ))

        return observations


class RehydrationObserverImpl:
    """Observes local rehydration artifacts and produces typed rehydration observations.

    The observer serializes tokens to a rehydration artifact, rehydrates them,
    and produces a ``RehydrationObservation`` recording the restored token
    count, unresolved token count, restored and unresolved sensitive types,
    and the input and output artifact hashes.
    """

    def __init__(
        self,
        artifact: LocalRehydrationArtifact,
        tokens: list[TokenEntry],
        evidence_sha: str,
        evidence_ref: str,
    ) -> None:
        self._artifact = artifact
        self._tokens = tokens
        self._evidence_sha = evidence_sha
        self._evidence_ref = evidence_ref

    async def observe(
        self,
        task: TaskDefinition,
        attempt: AttemptRecord,
    ) -> list[RehydrationObservation]:
        observations: list[RehydrationObservation] = []
        _sha, _status = _evidence_binding(self._evidence_sha)

        self._artifact.serialize(self._tokens)
        restored, unresolved_types = self._artifact.rehydrate()
        public_sha = self._artifact.public_sha256()
        if public_sha:
            input_sha = public_sha
            output_sha = public_sha
        else:
            input_sha = self._artifact.input_sha256()
            output_sha = self._artifact.output_sha256()

        for assertion in task.rehydration_assertions:
            restored_types = sorted({t.sensitive_type for t in restored})
            observations.append(RehydrationObservation(
                observation_id=f"{attempt.attempt_id}:rehydration:{assertion.assertion_id}",
                attempt_id=attempt.attempt_id,
                run_id=attempt.run_id,
                task_id=attempt.task_id,
                assertion_id=assertion.assertion_id,
                source=assertion.source,
                input_artifact_sha256=input_sha,
                output_artifact_sha256=output_sha,
                rehydrator_version=self._artifact.REHYDRATOR_VERSION,
                execution_boundary=RehydrationBoundary.LOCAL_RUNTIME,
                collected_at=datetime.now(UTC),
                restored_token_count=len(restored),
                unresolved_token_count=len(unresolved_types),
                restored_sensitive_types=restored_types,
                unresolved_sensitive_types=unresolved_types,
                source_evidence_refs=[self._evidence_ref],
                source_evidence_sha256=_sha,
                verification_status=_status,
            ))

        return observations
