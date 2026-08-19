# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import json

import pytest

from app.constants import paths as paths_module

pytestmark = [pytest.mark.unit]

load_paths = paths_module.__dict__["_load_paths"]


def _configure_protocol_paths(monkeypatch: pytest.MonkeyPatch, tmp_path):
    protocol_dir = tmp_path / "runner" / "work" / "g8e" / "g8e" / "protocol"
    constants_dir = protocol_dir / "constants"
    constants_dir.mkdir(parents=True)
    path_file = constants_dir / "paths.json"
    path_file.write_text(
        json.dumps(
            {
                "infra": {
                    "db_path": ".g8e/data/g8e.db",
                    "ca_cert_path": ".g8e/pki/trust/hub-bundle.pem",
                    "app_cert_dir": ".g8e/pki/issued/apps",
                    "pki_dir": ".g8e/pki",
                    "docs_dir": "/docs",
                    "protocol_dir": "/app/protocol",
                    "protocol_constants_dir": "/app/protocol/constants",
                    "protocol_models_dir": "/app/protocol/models",
                    "ssh_config_path": "/etc/g8e/ssh_config",
                },
                "g8ee": {
                    "cert_name": "g8ee",
                },
            }
        )
    )
    monkeypatch.setenv("G8E_PROTOCOL_DIR", str(protocol_dir))
    # Clear the paths cache to force re-resolution with new environment
    paths_module.reload_paths()
    return protocol_dir


def test_load_paths_prefers_explicit_host_pki_dir(monkeypatch: pytest.MonkeyPatch, tmp_path):
    _configure_protocol_paths(monkeypatch, tmp_path)
    pki_dir = tmp_path / "runner" / "work" / "g8e" / "g8e" / ".g8e" / "pki"
    monkeypatch.setenv("G8E_PKI_DIR", str(pki_dir))
    monkeypatch.delenv("G8E_RUNTIME_DIR", raising=False)
    # Clear cache to pick up new PKI_DIR
    paths_module.reload_paths()

    paths = load_paths()

    assert paths["infra"]["pki_dir"] == str(pki_dir)
    assert paths["infra"]["ca_cert_path"] == str(pki_dir / "trust" / "hub-bundle.pem")
    assert paths["infra"]["app_cert_dir"] == str(pki_dir / "issued" / "apps")
    assert paths["g8ee"]["cert_name"] == "g8ee"


def test_load_paths_uses_host_runtime_dir_when_pki_dir_unset(
    monkeypatch: pytest.MonkeyPatch, tmp_path
):
    _configure_protocol_paths(monkeypatch, tmp_path)
    runtime_dir = tmp_path / "runner" / "work" / "g8e" / "g8e" / ".g8e"
    monkeypatch.delenv("G8E_PKI_DIR", raising=False)
    monkeypatch.setenv("G8E_RUNTIME_DIR", str(runtime_dir))
    # Clear cache to pick up new RUNTIME_DIR
    paths_module.reload_paths()

    paths = load_paths()

    assert paths["infra"]["pki_dir"] == str(runtime_dir / "pki")
    assert paths["infra"]["ca_cert_path"] == str(runtime_dir / "pki" / "trust" / "hub-bundle.pem")
    assert paths["infra"]["app_cert_dir"] == str(runtime_dir / "pki" / "issued" / "apps")
    assert paths["g8ee"]["cert_name"] == "g8ee"


def test_load_paths_ca_cert_path_env_var_overrides_default(
    monkeypatch: pytest.MonkeyPatch, tmp_path
):
    """G8E_CA_CERT_PATH overrides the default hub-bundle.pem path so the ensemble
    can use the gateway's g8eg-ca-bundle.pem trust bundle in the docker-compose
    setup where the gateway's PKI dir is mounted read-only."""
    _configure_protocol_paths(monkeypatch, tmp_path)
    pki_dir = tmp_path / "runner" / "work" / "g8e" / "g8e" / ".g8e" / "pki"
    monkeypatch.setenv("G8E_PKI_DIR", str(pki_dir))
    monkeypatch.delenv("G8E_RUNTIME_DIR", raising=False)
    override = str(pki_dir / "trust" / "g8eg-ca-bundle.pem")
    monkeypatch.setenv("G8E_CA_CERT_PATH", override)
    paths_module.reload_paths()

    paths = load_paths()

    assert paths["infra"]["ca_cert_path"] == override
    # pki_dir and app_cert_dir are unaffected by the CA cert override
    assert paths["infra"]["pki_dir"] == str(pki_dir)
    assert paths["infra"]["app_cert_dir"] == str(pki_dir / "issued" / "apps")


def test_load_paths_ca_cert_path_defaults_to_hub_bundle_when_env_unset(
    monkeypatch: pytest.MonkeyPatch, tmp_path
):
    """When G8E_CA_CERT_PATH is unset, ca_cert_path falls back to the
    pki_dir/trust/hub-bundle.pem default for backward compatibility."""
    _configure_protocol_paths(monkeypatch, tmp_path)
    pki_dir = tmp_path / "runner" / "work" / "g8e" / "g8e" / ".g8e" / "pki"
    monkeypatch.setenv("G8E_PKI_DIR", str(pki_dir))
    monkeypatch.delenv("G8E_RUNTIME_DIR", raising=False)
    monkeypatch.delenv("G8E_CA_CERT_PATH", raising=False)
    paths_module.reload_paths()

    paths = load_paths()

    assert paths["infra"]["ca_cert_path"] == str(pki_dir / "trust" / "hub-bundle.pem")
