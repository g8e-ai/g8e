# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed bootstrap settings replace the G8E_* platform-configuration env vars (INV-ENV-04)."""

from pathlib import Path

import pytest

from app.constants import paths as paths_module
from app.constants.bootstrap import (
    BootstrapSettings,
    configure_bootstrap,
    get_bootstrap,
    reset_bootstrap,
)
from app.services.infra.app_enrollment_service import AppEnrollmentService


@pytest.fixture(autouse=True)
def _isolated_bootstrap():
    reset_bootstrap()
    yield
    reset_bootstrap()


def test_defaults_leave_every_setting_unset():
    assert get_bootstrap() == BootstrapSettings()


def test_configure_bootstrap_replaces_current_settings():
    configure_bootstrap(BootstrapSettings(gateway_http_url="http://g8e.local:8080"))

    assert get_bootstrap().gateway_http_url == "http://g8e.local:8080"


def test_configure_bootstrap_reloads_paths(tmp_path: Path):
    runtime_dir = tmp_path / "runtime"

    configure_bootstrap(BootstrapSettings(runtime_dir=str(runtime_dir)))

    assert paths_module.PATHS["infra"]["pki_dir"] == str(runtime_dir / "pki")
    assert paths_module.PATHS["infra"]["secrets_dir"] == str(runtime_dir / "secrets")


def test_explicit_secrets_and_pki_dirs_override_runtime_dir(tmp_path: Path):
    configure_bootstrap(
        BootstrapSettings(
            runtime_dir=str(tmp_path / "runtime"),
            pki_dir=str(tmp_path / "pki"),
            secrets_dir=str(tmp_path / "operator-state" / "secrets"),
        )
    )

    assert paths_module.PATHS["infra"]["pki_dir"] == str(tmp_path / "pki")
    assert paths_module.PATHS["infra"]["secrets_dir"] == str(tmp_path / "operator-state" / "secrets")


def test_explicit_ca_cert_path_overrides_default(tmp_path: Path):
    bundle = tmp_path / "custom-bundle.pem"

    configure_bootstrap(BootstrapSettings(pki_dir=str(tmp_path / "pki"), ca_cert_path=str(bundle)))

    assert paths_module.PATHS["infra"]["ca_cert_path"] == str(bundle)


def test_gateway_http_url_comes_from_bootstrap_not_the_environment(monkeypatch: pytest.MonkeyPatch):
    # The retired env var must not influence enrollment.
    monkeypatch.setenv("G8E_GATEWAY_HTTP_URL", "http://from-env:1")
    configure_bootstrap(BootstrapSettings(gateway_http_url="http://g8e.local:8080/"))

    assert AppEnrollmentService()._resolve_gateway_http_url() == "http://g8e.local:8080"


def test_retired_platform_env_vars_are_ignored(monkeypatch: pytest.MonkeyPatch, tmp_path: Path):
    monkeypatch.setenv("G8E_RUNTIME_DIR", str(tmp_path / "from-env"))
    monkeypatch.setenv("G8E_PKI_DIR", str(tmp_path / "pki-from-env"))
    monkeypatch.setenv("G8E_SECRETS_DIR", str(tmp_path / "secrets-from-env"))
    monkeypatch.setenv("G8E_CA_CERT_PATH", str(tmp_path / "ca-from-env.pem"))
    paths_module.reload_paths()

    infra = paths_module.PATHS["infra"]

    assert "from-env" not in infra["pki_dir"]
    assert "from-env" not in infra["secrets_dir"]
    assert "from-env" not in infra["ca_cert_path"]
