# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import base64
import hashlib
import json

import pytest
from cryptography.exceptions import InvalidTag

from g8e_evals.evidence import EvidenceEncryptionKey, decrypt_evidence_artifact, encrypt_evidence_artifact
from g8e_evals.schema import EvidenceIndex, EvidenceMediaType, PrivacyClassification
from g8e_evals.stages import EvidenceArtifact

pytestmark = pytest.mark.unit


def _artifact(content: str = '{"secret":"CANARY-SECRET"}') -> EvidenceArtifact:
    digest = hashlib.sha256(content.encode()).hexdigest()
    return EvidenceArtifact(
        index=EvidenceIndex(
            artifact_id="run-1:attempt-1:agent-trail",
            run_id="run-1",
            attempt_id="attempt-1",
            media_type=EvidenceMediaType.APPLICATION_JSON,
            schema_ref="g8e_evals.ChatEvaluationReceipt",
            byte_length=len(content.encode()),
            sha256=digest,
            producer_identity="g8e-evals",
            privacy_classification=PrivacyClassification.RESTRICTED,
            storage_location=f"evidence/{digest}.json",
        ),
        content=content,
    )


def _key() -> EvidenceEncryptionKey:
    return EvidenceEncryptionKey(key_id="test-key-1", key=b"k" * 32)


def test_restricted_artifact_is_authenticated_and_contains_no_plaintext() -> None:
    artifact = _artifact()

    encrypted = encrypt_evidence_artifact(artifact, _key(), nonce=b"n" * 12)

    assert "CANARY-SECRET" not in encrypted.envelope_json
    assert encrypted.index.storage_location.endswith(".json.enc")
    assert encrypted.index.encryption is not None
    assert encrypted.index.encryption.algorithm.value == "aes-256-gcm"
    assert encrypted.index.encryption.key_id == "test-key-1"
    assert encrypted.index.access_control is not None
    assert encrypted.index.access_control.policy.value == "named_key_holders"
    assert decrypt_evidence_artifact(encrypted.envelope_json, encrypted.index, _key()) == artifact.content


def test_artifact_ciphertext_tampering_fails_authentication() -> None:
    encrypted = encrypt_evidence_artifact(_artifact(), _key(), nonce=b"n" * 12)
    envelope = json.loads(encrypted.envelope_json)
    ciphertext = bytearray(base64.b64decode(envelope["ciphertext_b64"], validate=True))
    ciphertext[0] ^= 1
    envelope["ciphertext_b64"] = base64.b64encode(ciphertext).decode()

    with pytest.raises(InvalidTag):
        decrypt_evidence_artifact(json.dumps(envelope), encrypted.index, _key())


def test_artifact_index_tampering_fails_authentication() -> None:
    encrypted = encrypt_evidence_artifact(_artifact(), _key(), nonce=b"n" * 12)
    tampered_index = encrypted.index.model_copy(update={"artifact_id": "other-artifact"})

    with pytest.raises(InvalidTag):
        decrypt_evidence_artifact(encrypted.envelope_json, tampered_index, _key())


def test_restricted_artifact_refuses_plaintext_that_does_not_match_index() -> None:
    artifact = EvidenceArtifact(index=_artifact().index, content="different")

    with pytest.raises(ValueError, match="plaintext does not match"):
        encrypt_evidence_artifact(artifact, _key())


def test_restricted_artifact_refuses_invalid_key_length() -> None:
    with pytest.raises(ValueError, match="32 bytes"):
        EvidenceEncryptionKey(key_id="test-key-1", key=b"short")
