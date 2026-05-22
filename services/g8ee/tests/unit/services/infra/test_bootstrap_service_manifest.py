# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Unit tests for BootstrapService.verify_against_manifest.

These assert the tamper-evidence contract between g8eo SecretManager
(writer) and g8ee BootstrapService (reader) when using the digest
manifest file on the secrets volume.

g8eo encrypts secrets with AES-256-GCM before writing to disk. The manifest
contains SHA-256 digests of the encrypted file content, not plaintext secrets.
g8ee reads the encrypted file content directly and hashes it for verification.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

import pytest

from app.services.infra.bootstrap_service import (
    BOOTSTRAP_DIGEST_MANIFEST_FILE,
    BootstrapSecretTamperError,
    BootstrapService,
)


def _sha256(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def _sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _write_encrypted_secret(volume: Path, name: str, content: str) -> None:
    """Simulate g8eo writing an encrypted secret file."""
    (volume / name).write_text(content)


def _write_manifest(volume: Path, secrets: dict[str, str]) -> None:
    """Write manifest with digests of the encrypted file content."""
    digests = {}
    for name, content in secrets.items():
        digests[name] = {"sha256": _sha256_bytes(content.encode("utf-8"))}
    (volume / BOOTSTRAP_DIGEST_MANIFEST_FILE).write_text(
        json.dumps(
            {
                "version": 1,
                "updated_at": "2026-04-17T00:00:00Z",
                "secrets": digests,
            }
        )
    )


@pytest.fixture
def volume(tmp_path: Path) -> Path:
    return tmp_path


@pytest.fixture
def bootstrap(volume: Path) -> BootstrapService:
    return BootstrapService(secrets_dir=str(volume), pki_dir=str(volume))


def test_verify_passes_when_digest_matches(volume: Path, bootstrap: BootstrapService) -> None:
    encrypted_content = '{"version":1,"nonce":"abc","ciphertext":"encrypted-data"}'
    _write_encrypted_secret(volume, "session_encryption_key", encrypted_content)
    _write_manifest(volume, {"session_encryption_key": encrypted_content})

    bootstrap.verify_against_manifest("session_encryption_key", None)


def test_verify_raises_when_digest_mismatches(volume: Path, bootstrap: BootstrapService) -> None:
    original_encrypted = '{"version":1,"nonce":"abc","ciphertext":"original"}'
    tampered_encrypted = '{"version":1,"nonce":"xyz","ciphertext":"tampered"}'
    _write_encrypted_secret(volume, "session_encryption_key", tampered_encrypted)
    _write_manifest(volume, {"session_encryption_key": original_encrypted})

    with pytest.raises(BootstrapSecretTamperError, match="failed tamper-evidence check"):
        bootstrap.verify_against_manifest("session_encryption_key", None)


def test_verify_skips_when_manifest_missing(volume: Path, bootstrap: BootstrapService) -> None:
    _write_encrypted_secret(volume, "session_encryption_key", "any-encrypted")
    bootstrap.verify_against_manifest("session_encryption_key", None)


def test_verify_skips_when_manifest_has_no_entry(volume: Path, bootstrap: BootstrapService) -> None:
    _write_encrypted_secret(volume, "session_encryption_key", "encrypted")
    _write_manifest(volume, {"auditor_hmac_key": "encrypted-hmac"})

    bootstrap.verify_against_manifest("session_encryption_key", None)


def test_verify_raises_on_malformed_manifest(volume: Path, bootstrap: BootstrapService) -> None:
    (volume / BOOTSTRAP_DIGEST_MANIFEST_FILE).write_text("{not valid json")

    with pytest.raises(BootstrapSecretTamperError, match="unreadable or malformed"):
        bootstrap.verify_against_manifest("session_encryption_key", None)


def test_verify_raises_when_secret_file_missing(volume: Path, bootstrap: BootstrapService) -> None:
    _write_manifest(volume, {"session_encryption_key": "encrypted-content"})

    with pytest.raises(BootstrapSecretTamperError, match="does not exist"):
        bootstrap.verify_against_manifest("session_encryption_key", None)


def test_verify_session_key_independently(volume: Path, bootstrap: BootstrapService) -> None:
    encrypted_key = '{"version":1,"nonce":"abc","ciphertext":"encrypted-key-a"}'
    _write_encrypted_secret(volume, "session_encryption_key", encrypted_key)
    _write_manifest(volume, {"session_encryption_key": encrypted_key})

    bootstrap.verify_against_manifest("session_encryption_key", None)


def test_verify_auditor_hmac_key_independently(volume: Path, bootstrap: BootstrapService) -> None:
    # GDD §14.4 Artifact B: the auditor HMAC key must have the same
    # tamper-evidence guarantees as the other bootstrap secrets. A
    # divergent key would silently poison the reputation commitment
    # chain with signatures that look valid to g8ee but cannot be
    # reproduced from the DB-authoritative key.
    encrypted_session = '{"version":1,"nonce":"abc","ciphertext":"encrypted-session"}'
    encrypted_hmac = '{"version":1,"nonce":"def","ciphertext":"encrypted-hmac-d"}'
    _write_encrypted_secret(volume, "session_encryption_key", encrypted_session)
    _write_encrypted_secret(volume, "auditor_hmac_key", encrypted_hmac)
    _write_manifest(
        volume,
        {
            "session_encryption_key": encrypted_session,
            "auditor_hmac_key": encrypted_hmac,
        },
    )

    bootstrap.verify_against_manifest("auditor_hmac_key", None)


def test_load_auditor_hmac_key_reads_file_and_caches(volume: Path, bootstrap: BootstrapService) -> None:
    # Note: In production, g8eo writes encrypted files. For testing, we write
    # plaintext since g8ee can't decrypt without OS key store access.
    (volume / "auditor_hmac_key").write_text("hmac-value\n")

    # First read hits disk.
    assert bootstrap.load_auditor_hmac_key() == "hmac-value"

    # Mutating the file after the first successful load must not affect
    # subsequent reads - the cache is the whole point of the loader (one
    # value per process lifetime; rotation requires restart, mirrored on
    # the g8eo side).
    (volume / "auditor_hmac_key").write_text("rotated")
    assert bootstrap.load_auditor_hmac_key() == "hmac-value"


def test_load_auditor_hmac_key_returns_none_when_missing(volume: Path, bootstrap: BootstrapService) -> None:
    assert bootstrap.load_auditor_hmac_key() is None


def test_is_available_true_when_only_auditor_hmac_key_present(volume: Path, bootstrap: BootstrapService) -> None:
    # Reputation-only Phase 2 deployments that boot with only the auditor
    # HMAC key (no API key) must still be reported as bootstrap-available.
    (volume / "auditor_hmac_key").write_text("h")
    assert bootstrap.is_available() is True
