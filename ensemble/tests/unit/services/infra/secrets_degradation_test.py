# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for secrets degradation behavior.

The ensemble runs without `session_encryption_key` and `auditor_hmac_key` when
its own runtime volume has no secrets directory (the post-enrollment state:
the ensemble has its own `g8e-ensemble-data` volume, not the gateway's
secrets volume). These tests assert the code degrades gracefully — logging
the absence and proceeding with `None` — rather than crashing. If a test
reveals a hard dependency, that is a bug to fix, not a reason to provision
the keys.

Covers WS4 of the v2.0.0 ensemble app enrollment plan.
"""

from __future__ import annotations

import logging
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from app.services.infra.bootstrap_service import BootstrapService
from app.services.infra.settings_service import SettingsService


pytestmark = pytest.mark.unit


class TestPubSubClientAuditorHmacKeyNone:
    """PubSubClient must construct with `auditor_hmac_key=None` and skip signing.

    The PubSubClient stores the key but does not sign pubsub events itself —
    event signing is the Tribunal Auditor stage's responsibility, which reads
    the key from `AIToolService.auditor_hmac_key` (raising `ConfigurationError`
    only on the tribunal path, not at construction). Construction with `None`
    must succeed so the ensemble can boot without the HMAC key provisioned.
    """

    def test_constructs_with_none(self) -> None:
        from app.clients.pubsub_client import PubSubClient

        client = PubSubClient(
            pubsub_url="wss://localhost:443",
            auditor_hmac_key=None,
        )
        assert client._auditor_hmac_key is None

    def test_constructs_with_key(self) -> None:
        from app.clients.pubsub_client import PubSubClient

        client = PubSubClient(
            pubsub_url="wss://localhost:443",
            auditor_hmac_key="test-key-1234",
        )
        assert client._auditor_hmac_key == "test-key-1234"


class TestBootstrapServiceMissingSecretsDir:
    """BootstrapService must degrade gracefully when the secrets directory is
    empty or missing — returning `None` and logging the absence, not raising."""

    def test_load_session_encryption_key_returns_none_when_dir_missing(
        self, tmp_path: Path, caplog: pytest.LogCaptureFixture
    ) -> None:
        # Point at a secrets dir that does not exist.
        bootstrap = BootstrapService(secrets_dir=str(tmp_path / "nonexistent"), pki_dir=str(tmp_path))
        with caplog.at_level(logging.INFO, logger="app.services.infra.bootstrap_service"):
            result = bootstrap.load_session_encryption_key()
        assert result is None

    def test_load_auditor_hmac_key_returns_none_when_dir_missing(
        self, tmp_path: Path, caplog: pytest.LogCaptureFixture
    ) -> None:
        bootstrap = BootstrapService(secrets_dir=str(tmp_path / "nonexistent"), pki_dir=str(tmp_path))
        with caplog.at_level(logging.INFO, logger="app.services.infra.bootstrap_service"):
            result = bootstrap.load_auditor_hmac_key()
        assert result is None

    def test_load_session_encryption_key_returns_none_when_dir_empty(
        self, tmp_path: Path
    ) -> None:
        (tmp_path / "secrets").mkdir()
        bootstrap = BootstrapService(secrets_dir=str(tmp_path / "secrets"), pki_dir=str(tmp_path))
        assert bootstrap.load_session_encryption_key() is None

    def test_load_auditor_hmac_key_returns_none_when_dir_empty(
        self, tmp_path: Path
    ) -> None:
        (tmp_path / "secrets").mkdir()
        bootstrap = BootstrapService(secrets_dir=str(tmp_path / "secrets"), pki_dir=str(tmp_path))
        assert bootstrap.load_auditor_hmac_key() is None

    def test_is_available_false_when_no_secrets(self, tmp_path: Path) -> None:
        (tmp_path / "secrets").mkdir()
        bootstrap = BootstrapService(secrets_dir=str(tmp_path / "secrets"), pki_dir=str(tmp_path))
        assert bootstrap.is_available() is False


class TestSettingsServiceDegradesWithoutSecrets:
    """SettingsService.get_local_settings() must proceed with `None` secrets
    when the bootstrap volume has no secrets, logging the absence rather than
    raising. This is the post-enrollment state: the ensemble's own volume has
    no secrets directory."""

    def test_get_local_settings_succeeds_with_no_secrets(
        self, tmp_path: Path, caplog: pytest.LogCaptureFixture
    ) -> None:
        bootstrap = BootstrapService(
            secrets_dir=str(tmp_path / "nonexistent"),
            pki_dir=str(tmp_path / "nonexistent"),
        )
        service = SettingsService(bootstrap_service=bootstrap)
        with caplog.at_level(logging.INFO, logger="app.services.infra.settings_service"):
            settings = service.get_local_settings()
        assert settings.auth.session_encryption_key is None
        assert settings.auth.auditor_hmac_key is None
        # The service must not raise; reaching this assertion proves graceful
        # degradation. The absence is logged so operators can diagnose why
        # session encryption / HMAC signing is inactive.
        absence_messages = [
            r.message
            for r in caplog.records
            if "not available" in r.message or "not found" in r.message
        ]
        assert absence_messages, "expected at least one 'not available' log message"

    def test_get_local_settings_with_mocked_bootstrap_returning_none(self) -> None:
        bootstrap = MagicMock()
        bootstrap.load_session_encryption_key.return_value = None
        bootstrap.load_auditor_hmac_key.return_value = None
        bootstrap.verify_against_manifest = MagicMock()
        service = SettingsService(bootstrap_service=bootstrap)
        settings = service.get_local_settings()
        assert settings.auth.session_encryption_key is None
        assert settings.auth.auditor_hmac_key is None
        # verify_against_manifest must not be called when the secret is absent
        # (the SettingsService only verifies on the truthy branch).
        bootstrap.verify_against_manifest.assert_not_called()
