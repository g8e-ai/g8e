# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Local encrypted token store for synthetic privacy eval suites.

This module provides a production-shaped encrypted token store that uses
AES-256-GCM to encrypt token mappings at rest on the local filesystem.
It supports vault lock/unlock, persistence across restarts, token TTL
expiry, and storage failure injection.  The store is the system under
test for the synthetic privacy token lifecycle suite: the suite's
observers interact with the store to produce typed observations that
the deterministic graders consume.
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from pathlib import Path
from collections.abc import Callable

from cryptography.hazmat.primitives.ciphers.aead import AESGCM


_NONCE_BYTES = 12
_KEY_BYTES = 32
_VAULT_ALGORITHM = "aes-256-gcm"


class VaultLockedError(Exception):
    """Raised when a read or write is attempted while the vault is locked."""


class StorageError(Exception):
    """Raised when a storage operation fails (injected or real)."""


@dataclass(frozen=True)
class TokenEntry:
    token_id: str
    value: str
    sensitive_type: str
    created_at: datetime
    expires_at: datetime


@dataclass
class PersistResult:
    persisted: bool
    rolled_back: bool
    sensitive_value_leaked: bool
    unsafe_continuation: bool
    operation_refused: bool


class LocalEncryptedTokenStore:
    """A real local encrypted token store using AES-256-GCM at rest.

    The store encrypts every token mapping before writing it to disk and
    decrypts on restore.  When the vault is locked, all reads and writes
    are refused.  Tokens carry a TTL and become invisible after expiry.
    Storage failures can be injected to test fail-closed behavior.

    The store tracks the last successfully committed in-memory snapshot
    and restores it on any persistence failure, so pre-existing committed
    tokens survive while uncommitted tokens are rolled back.  An optional
    writer callable exercises real filesystem write failures rather than
    relying solely on the ``fail_persist`` flag.
    """

    def __init__(
        self,
        store_path: Path,
        key: bytes,
        *,
        locked: bool = False,
        now: Callable[[], datetime] | None = None,
        fail_persist: bool = False,
        writer: Callable[[Path, bytes], None] | None = None,
    ) -> None:
        if len(key) != _KEY_BYTES:
            raise ValueError(f"key must be exactly {_KEY_BYTES} bytes")
        self._store_path = store_path
        self._key = key
        self._locked = locked
        self._now = now or (lambda: datetime.now(UTC))
        self._fail_persist = fail_persist
        self._writer = writer
        self._in_memory: dict[str, TokenEntry] = {}
        self._committed_snapshot: dict[str, TokenEntry] | None = None

    @property
    def vault_algorithm(self) -> str:
        return _VAULT_ALGORITHM

    @property
    def locked(self) -> bool:
        return self._locked

    def lock(self) -> None:
        self._locked = True

    def unlock(self) -> None:
        self._locked = False

    def store(self, token_id: str, value: str, sensitive_type: str, ttl_seconds: int) -> None:
        if self._locked:
            raise VaultLockedError("write refused: vault is locked")
        expires_at = self._now() + timedelta(seconds=ttl_seconds)
        self._in_memory[token_id] = TokenEntry(
            token_id=token_id,
            value=value,
            sensitive_type=sensitive_type,
            created_at=self._now(),
            expires_at=expires_at,
        )

    def retrieve(self, token_id: str) -> str | None:
        if self._locked:
            raise VaultLockedError("read refused: vault is locked")
        entry = self._in_memory.get(token_id)
        if entry is None:
            return None
        if self._now() >= entry.expires_at:
            return None
        return entry.value

    def is_visible(self, token_id: str) -> bool:
        try:
            return self.retrieve(token_id) is not None
        except VaultLockedError:
            return False

    def set_fail_persist(self, fail: bool) -> None:
        """Toggle the ``fail_persist`` flag after construction.

        This allows a caller to establish a committed state with a
        successful persist, then inject a failure for a subsequent
        persist attempt on the same store instance.
        """
        self._fail_persist = fail

    def persist(self) -> PersistResult:
        """Persist all in-memory tokens to disk as an encrypted envelope.

        Returns a ``PersistResult`` describing the outcome.  When a storage
        failure is injected or a real write fails, the in-memory state is
        restored to the last successfully committed snapshot (or emptied if
        no prior commit exists) and the operation is refused.  Pre-existing
        committed tokens survive; uncommitted tokens added after the last
        successful persist are rolled back.
        """
        if self._locked:
            return PersistResult(
                persisted=False,
                rolled_back=False,
                sensitive_value_leaked=False,
                unsafe_continuation=False,
                operation_refused=True,
            )
        if self._fail_persist:
            self._restore_committed_snapshot()
            return PersistResult(
                persisted=False,
                rolled_back=True,
                sensitive_value_leaked=False,
                unsafe_continuation=False,
                operation_refused=True,
            )
        plaintext = json.dumps({
            tid: {
                "token_id": e.token_id,
                "value": e.value,
                "sensitive_type": e.sensitive_type,
                "created_at": e.created_at.isoformat(),
                "expires_at": e.expires_at.isoformat(),
            }
            for tid, e in self._in_memory.items()
        }, sort_keys=True)
        nonce = os.urandom(_NONCE_BYTES)
        ciphertext = AESGCM(self._key).encrypt(nonce, plaintext.encode(), None)
        envelope = {
            "nonce_b64": base64.b64encode(nonce).decode(),
            "ciphertext_b64": base64.b64encode(ciphertext).decode(),
        }
        envelope_bytes = json.dumps(envelope, sort_keys=True).encode()
        try:
            if self._writer is not None:
                self._writer(self._store_path, envelope_bytes)
            else:
                self._store_path.parent.mkdir(parents=True, exist_ok=True)
                self._store_path.write_bytes(envelope_bytes)
        except (OSError, StorageError):
            self._restore_committed_snapshot()
            return PersistResult(
                persisted=False,
                rolled_back=True,
                sensitive_value_leaked=False,
                unsafe_continuation=False,
                operation_refused=True,
            )
        self._committed_snapshot = dict(self._in_memory)
        return PersistResult(
            persisted=True,
            rolled_back=False,
            sensitive_value_leaked=False,
            unsafe_continuation=False,
            operation_refused=False,
        )

    def _restore_committed_snapshot(self) -> None:
        """Restore in-memory state to the last committed snapshot.

        If no prior commit exists, the in-memory state is emptied.  This
        ensures pre-existing committed tokens survive a persistence
        failure while uncommitted tokens are rolled back.
        """
        if self._committed_snapshot is not None:
            self._in_memory = dict(self._committed_snapshot)
        else:
            self._in_memory.clear()

    def restore(self) -> int:
        """Restore tokens from the encrypted on-disk envelope.

        Returns the number of non-expired tokens restored.  Expired tokens
        are loaded but immediately invisible via ``retrieve``.
        """
        if self._locked:
            raise VaultLockedError("read refused: vault is locked")
        if not self._store_path.exists():
            return 0
        envelope = json.loads(self._store_path.read_text())
        nonce = base64.b64decode(envelope["nonce_b64"], validate=True)
        ciphertext = base64.b64decode(envelope["ciphertext_b64"], validate=True)
        plaintext = AESGCM(self._key).decrypt(nonce, ciphertext, None)
        data = json.loads(plaintext.decode())
        self._in_memory = {
            tid: TokenEntry(
                token_id=e["token_id"],
                value=e["value"],
                sensitive_type=e["sensitive_type"],
                created_at=datetime.fromisoformat(e["created_at"]),
                expires_at=datetime.fromisoformat(e["expires_at"]),
            )
            for tid, e in data.items()
        }
        self._committed_snapshot = dict(self._in_memory)
        return sum(1 for e in self._in_memory.values() if self._now() < e.expires_at)

    def stored_ciphertext_sha256(self) -> str:
        if not self._store_path.exists():
            return ""
        return hashlib.sha256(self._store_path.read_bytes()).hexdigest()

    def plaintext_in_store(self) -> bool:
        """Check whether any plaintext token value appears in the store file."""
        if not self._store_path.exists():
            return False
        content = self._store_path.read_bytes()
        for entry in self._in_memory.values():
            if entry.value.encode() in content:
                return True
        return False

    def token_count(self) -> int:
        return len(self._in_memory)

    def non_expired_token_count(self) -> int:
        return sum(1 for e in self._in_memory.values() if self._now() < e.expires_at)

    def token_expiry(self, token_id: str) -> datetime | None:
        entry = self._in_memory.get(token_id)
        return entry.expires_at if entry else None

    def token_created_at(self, token_id: str) -> datetime | None:
        entry = self._in_memory.get(token_id)
        return entry.created_at if entry else None

    def token_sensitive_type(self, token_id: str) -> str | None:
        entry = self._in_memory.get(token_id)
        return entry.sensitive_type if entry else None

    def has_token(self, token_id: str) -> bool:
        """Return ``True`` if the token ID is present in the in-memory store."""
        return token_id in self._in_memory


class LocalRehydrationArtifact:
    """Serializes and deserializes token mappings to a local artifact.

    The artifact supports two serialization modes:

    - **Encrypted** (``key`` provided): the full token mappings including
      raw sensitive values are encrypted with AES-256-GCM and written to
      ``artifact_path``.  The ciphertext is the restricted evidence; the
      key must be stored outside the public report directory.

    - **Hash-safe public projection** (``public_path`` provided): only
      SHA-256 digests of the sensitive values are written, never the raw
      values.  This is the safe public artifact that can appear in the
      report directory without leaking canaries.

    The rehydrator reads the encrypted artifact and restores the tokens,
    measuring how many were successfully restored versus unresolved.
    """

    REHYDRATOR_VERSION = "sentinel-rehydrator@1.0.0"

    def __init__(
        self,
        artifact_path: Path,
        *,
        key: bytes | None = None,
        public_path: Path | None = None,
    ) -> None:
        self._artifact_path = artifact_path
        self._key = key
        self._public_path = public_path

    def serialize(self, tokens: list[TokenEntry]) -> str:
        """Serialize tokens to the artifact.

        When a key is provided, the full token mappings are encrypted at
        rest.  When a public path is provided, a hash-safe projection
        (SHA-256 digests only) is written to the public path.  Returns
        the content of the encrypted (or plaintext) artifact.
        """
        data = {
            "tokens": [
                {
                    "token_id": e.token_id,
                    "value": e.value,
                    "sensitive_type": e.sensitive_type,
                    "created_at": e.created_at.isoformat(),
                    "expires_at": e.expires_at.isoformat(),
                }
                for e in tokens
            ]
        }
        plaintext = json.dumps(data, sort_keys=True)
        self._artifact_path.parent.mkdir(parents=True, exist_ok=True)

        if self._key is not None:
            nonce = os.urandom(_NONCE_BYTES)
            ciphertext = AESGCM(self._key).encrypt(nonce, plaintext.encode(), None)
            envelope = {
                "nonce_b64": base64.b64encode(nonce).decode(),
                "ciphertext_b64": base64.b64encode(ciphertext).decode(),
            }
            content = json.dumps(envelope, sort_keys=True)
            self._artifact_path.write_text(content)
        else:
            self._artifact_path.write_text(plaintext)
            content = plaintext

        if self._public_path is not None:
            public_data = {
                "tokens": [
                    {
                        "token_id": e.token_id,
                        "value_sha256": hashlib.sha256(e.value.encode()).hexdigest(),
                        "sensitive_type": e.sensitive_type,
                        "created_at": e.created_at.isoformat(),
                        "expires_at": e.expires_at.isoformat(),
                    }
                    for e in tokens
                ]
            }
            public_content = json.dumps(public_data, sort_keys=True)
            self._public_path.parent.mkdir(parents=True, exist_ok=True)
            self._public_path.write_text(public_content)

        return content

    def input_sha256(self) -> str:
        if not self._artifact_path.exists():
            return ""
        return hashlib.sha256(self._artifact_path.read_bytes()).hexdigest()

    def rehydrate(self) -> tuple[list[TokenEntry], list[str]]:
        """Rehydrate tokens from the artifact.

        Returns ``(restored, unresolved_types)``.  In the local
        rehydrator every token in the artifact is restored, so the
        unresolved list is empty.  The caller can simulate unresolved
        tokens by corrupting the artifact before calling this method.
        """
        content = self._artifact_path.read_text()
        if self._key is not None:
            envelope = json.loads(content)
            nonce = base64.b64decode(envelope["nonce_b64"], validate=True)
            ciphertext = base64.b64decode(envelope["ciphertext_b64"], validate=True)
            plaintext = AESGCM(self._key).decrypt(nonce, ciphertext, None).decode()
            data = json.loads(plaintext)
        else:
            data = json.loads(content)
        restored: list[TokenEntry] = []
        for entry in data.get("tokens", []):
            restored.append(TokenEntry(
                token_id=entry["token_id"],
                value=entry["value"],
                sensitive_type=entry["sensitive_type"],
                created_at=datetime.fromisoformat(entry["created_at"]),
                expires_at=datetime.fromisoformat(entry["expires_at"]),
            ))
        return restored, []

    def output_sha256(self) -> str:
        if not self._artifact_path.exists():
            return ""
        return hashlib.sha256(self._artifact_path.read_bytes()).hexdigest()

    def public_sha256(self) -> str:
        """Return the SHA-256 of the hash-safe public projection.

        The public projection contains only SHA-256 digests of sensitive
        values, never the raw values themselves.  Its hash is deterministic
        across runs (unlike the encrypted artifact whose nonce varies), so
        it is the canonical hash for rehydration assertions and
        observations.
        """
        if self._public_path is None or not self._public_path.exists():
            return ""
        return hashlib.sha256(self._public_path.read_bytes()).hexdigest()
