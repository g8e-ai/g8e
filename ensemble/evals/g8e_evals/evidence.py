# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import base64
import binascii
import hashlib
import json
import os
import stat
from dataclasses import dataclass
from pathlib import Path

from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from pydantic import BaseModel, ConfigDict, ValidationError

from g8e_evals.schema import (
    EvidenceAccessControl,
    EvidenceAccessPolicy,
    EvidenceEncryption,
    EvidenceEncryptionAlgorithm,
    EvidenceIndex,
)
from g8e_evals.stages import EvidenceArtifact

_KEY_FILE_VERSION = 1
_ENVELOPE_VERSION = 1
_NONCE_BYTES = 12
_KEY_BYTES = 32
_AUTHORIZATION_SCOPE = "restricted_evaluation_evidence"


@dataclass(frozen=True)
class EvidenceEncryptionKey:
    key_id: str
    key: bytes

    def __post_init__(self) -> None:
        if not self.key_id.strip():
            raise ValueError("evidence encryption key id must not be empty")
        if len(self.key) != _KEY_BYTES:
            raise ValueError("evidence encryption key must be exactly 32 bytes")


class _EvidenceKeyFile(BaseModel):
    model_config = ConfigDict(extra="forbid")

    version: int
    key_id: str
    key_b64: str


class _EncryptedEvidenceEnvelope(BaseModel):
    model_config = ConfigDict(extra="forbid")

    version: int
    algorithm: EvidenceEncryptionAlgorithm
    key_id: str
    nonce_b64: str
    ciphertext_b64: str


@dataclass(frozen=True)
class EncryptedEvidenceArtifact:
    index: EvidenceIndex
    envelope_json: str


def load_evidence_encryption_key(path: Path) -> EvidenceEncryptionKey:
    if path.is_symlink() or not path.is_file():
        raise ValueError("evidence encryption key path must be a regular file")
    mode = stat.S_IMODE(path.stat().st_mode)
    if mode & 0o077:
        raise ValueError("evidence encryption key file must be owner-only")
    try:
        wire = _EvidenceKeyFile.model_validate_json(path.read_text())
        if wire.version != _KEY_FILE_VERSION:
            raise ValueError("unsupported evidence encryption key file version")
        key = base64.b64decode(wire.key_b64, validate=True)
        return EvidenceEncryptionKey(key_id=wire.key_id, key=key)
    except (OSError, ValidationError, ValueError, binascii.Error) as error:
        raise ValueError("invalid evidence encryption key file") from error


def _encrypted_storage_location(index: EvidenceIndex) -> str:
    path = Path(index.storage_location)
    return str(path.with_suffix(path.suffix + ".enc"))


def _aad(index: EvidenceIndex) -> bytes:
    wire = {
        "access_control": index.access_control.model_dump(mode="json") if index.access_control else None,
        "artifact_id": index.artifact_id,
        "attempt_id": index.attempt_id,
        "byte_length": index.byte_length,
        "media_type": index.media_type.value,
        "privacy_classification": index.privacy_classification.value,
        "run_id": index.run_id,
        "schema_ref": index.schema_ref,
        "sha256": index.sha256,
        "storage_location": index.storage_location,
    }
    return json.dumps(wire, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode()


def encrypt_evidence_artifact(
    artifact: EvidenceArtifact,
    key: EvidenceEncryptionKey,
    *,
    nonce: bytes | None = None,
) -> EncryptedEvidenceArtifact:
    if nonce is None:
        nonce = os.urandom(_NONCE_BYTES)
    if len(nonce) != _NONCE_BYTES:
        raise ValueError("AES-256-GCM evidence nonce must be exactly 12 bytes")
    plaintext = artifact.content.encode()
    if hashlib.sha256(plaintext).hexdigest() != artifact.index.sha256 or len(plaintext) != artifact.index.byte_length:
        raise ValueError("evidence plaintext does not match its index")

    access_control = EvidenceAccessControl(
        policy=EvidenceAccessPolicy.NAMED_KEY_HOLDERS,
        authorization_scope=_AUTHORIZATION_SCOPE,
    )
    index = artifact.index.model_copy(update={
        "storage_location": _encrypted_storage_location(artifact.index),
        "access_control": access_control,
    })
    aad = _aad(index)
    ciphertext = AESGCM(key.key).encrypt(nonce, plaintext, aad)
    encryption = EvidenceEncryption(
        algorithm=EvidenceEncryptionAlgorithm.AES_256_GCM,
        key_id=key.key_id,
        aad_sha256=hashlib.sha256(aad).hexdigest(),
        ciphertext_sha256=hashlib.sha256(ciphertext).hexdigest(),
        ciphertext_byte_length=len(ciphertext),
    )
    index = index.model_copy(update={"encryption": encryption})
    envelope = _EncryptedEvidenceEnvelope(
        version=_ENVELOPE_VERSION,
        algorithm=encryption.algorithm,
        key_id=key.key_id,
        nonce_b64=base64.b64encode(nonce).decode(),
        ciphertext_b64=base64.b64encode(ciphertext).decode(),
    )
    return EncryptedEvidenceArtifact(index=index, envelope_json=envelope.model_dump_json())


def decrypt_evidence_artifact(
    envelope_json: str,
    index: EvidenceIndex,
    key: EvidenceEncryptionKey,
) -> str:
    envelope = _EncryptedEvidenceEnvelope.model_validate_json(envelope_json)
    encryption = index.encryption
    if encryption is None or index.access_control is None:
        raise ValueError("evidence index lacks encryption or access-control metadata")
    if envelope.version != _ENVELOPE_VERSION:
        raise ValueError("unsupported encrypted evidence envelope version")
    if envelope.algorithm != encryption.algorithm or envelope.key_id != encryption.key_id:
        raise ValueError("encrypted evidence envelope does not match its index")
    if key.key_id != encryption.key_id:
        raise ValueError("evidence decryption key id does not match the index")

    nonce = base64.b64decode(envelope.nonce_b64, validate=True)
    ciphertext = base64.b64decode(envelope.ciphertext_b64, validate=True)
    if len(nonce) != _NONCE_BYTES:
        raise ValueError("encrypted evidence envelope has an invalid nonce")
    aad = _aad(index)
    plaintext = AESGCM(key.key).decrypt(nonce, ciphertext, aad)
    if hashlib.sha256(aad).hexdigest() != encryption.aad_sha256:
        raise ValueError("evidence associated-data digest does not match the index")
    if hashlib.sha256(ciphertext).hexdigest() != encryption.ciphertext_sha256:
        raise ValueError("evidence ciphertext digest does not match the index")
    if len(ciphertext) != encryption.ciphertext_byte_length:
        raise ValueError("evidence ciphertext length does not match the index")
    if hashlib.sha256(plaintext).hexdigest() != index.sha256 or len(plaintext) != index.byte_length:
        raise ValueError("decrypted evidence does not match the plaintext index")
    return plaintext.decode()
