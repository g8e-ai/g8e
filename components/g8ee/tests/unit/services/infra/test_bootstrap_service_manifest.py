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
manifest file on the SSL volume.
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


def _write_manifest(volume: Path, secrets: dict[str, str]) -> None:
    (volume / BOOTSTRAP_DIGEST_MANIFEST_FILE).write_text(
        json.dumps(
            {
                "version": 1,
                "updated_at": "2026-04-17T00:00:00Z",
                "secrets": {name: {"sha256": _sha256(value)} for name, value in secrets.items()},
            }
        )
    )


@pytest.fixture
def volume(tmp_path: Path) -> Path:
    return tmp_path


@pytest.fixture
def bootstrap(volume: Path) -> BootstrapService:
    return BootstrapService(volume_path=str(volume))


def test_verify_passes_when_digest_matches(volume: Path, bootstrap: BootstrapService) -> None:
    token = "matching-token"
    _write_manifest(volume, {"internal_auth_token": token})

    # Should not raise.
    bootstrap.verify_against_manifest("internal_auth_token", token)


def test_verify_raises_when_digest_mismatches(volume: Path, bootstrap: BootstrapService) -> None:
    _write_manifest(volume, {"internal_auth_token": "db-authoritative"})

    with pytest.raises(BootstrapSecretTamperError, match="failed tamper-evidence check"):
        bootstrap.verify_against_manifest("internal_auth_token", "tampered-volume-value")


def test_verify_noop_for_empty_value(volume: Path, bootstrap: BootstrapService) -> None:
    _write_manifest(volume, {"internal_auth_token": "anything"})

    # Empty/None value: nothing to compare; the caller already decided not
    # to authenticate with it. Must not raise.
    bootstrap.verify_against_manifest("internal_auth_token", "")
    bootstrap.verify_against_manifest("internal_auth_token", None)


def test_verify_skips_when_manifest_missing(volume: Path, bootstrap: BootstrapService) -> None:
    # Transitional window: old g8eo that never wrote a manifest. Log+skip,
    # do not hard-fail, otherwise upgrade order becomes a deploy footgun.
    bootstrap.verify_against_manifest("internal_auth_token", "any-value")


def test_verify_skips_when_manifest_has_no_entry(volume: Path, bootstrap: BootstrapService) -> None:
    _write_manifest(volume, {"session_encryption_key": "k"})

    bootstrap.verify_against_manifest("internal_auth_token", "any-value")


def test_verify_raises_on_malformed_manifest(volume: Path, bootstrap: BootstrapService) -> None:
    (volume / BOOTSTRAP_DIGEST_MANIFEST_FILE).write_text("{not valid json")

    with pytest.raises(BootstrapSecretTamperError, match="unreadable or malformed"):
        bootstrap.verify_against_manifest("internal_auth_token", "any-value")


def test_verify_session_key_independently(volume: Path, bootstrap: BootstrapService) -> None:
    key = "a" * 64
    _write_manifest(
        volume,
        {"internal_auth_token": "tok", "session_encryption_key": key},
    )

    bootstrap.verify_against_manifest("session_encryption_key", key)
    with pytest.raises(BootstrapSecretTamperError):
        bootstrap.verify_against_manifest("session_encryption_key", "b" * 64)
