# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for trust bundle resolution.

These guard against the silent ``verify=False`` regression that defeats the
harness's receipt-binding guarantee.
"""

import os
import inspect
from pathlib import Path

import pytest

from g8e_evals.tls import (
    GATEWAY_TRUST_BUNDLE_NAME,
    TRUST_DIRECTORY_NAME,
    RuntimeIdentity,
    resolve_trust_bundle,
)
from g8e_evals.receipts import collector as collector_mod

pytestmark = pytest.mark.integration


@pytest.fixture
def clean_env(monkeypatch):
    monkeypatch.delenv("G8E_APP_TRUST_BUNDLE", raising=False)
    monkeypatch.delenv("G8E_GATEWAY_TRUST_BUNDLE", raising=False)
    monkeypatch.delenv("G8E_APP_PKI_DIR", raising=False)
    monkeypatch.delenv("G8E_GATEWAY_PKI_DIR", raising=False)


def test_explicit_trust_bundle_env(tmp_path, clean_env, monkeypatch):
    bundle = tmp_path / "hub-bundle.pem"
    bundle.write_text("-----BEGIN CERTIFICATE-----\n")
    monkeypatch.setenv("G8E_APP_TRUST_BUNDLE", str(bundle))
    assert resolve_trust_bundle(RuntimeIdentity.APP) == str(bundle)


def test_explicit_trust_bundle_missing_raises(tmp_path, clean_env, monkeypatch):
    monkeypatch.setenv("G8E_APP_TRUST_BUNDLE", str(tmp_path / "missing.pem"))
    with pytest.raises(FileNotFoundError):
        resolve_trust_bundle(RuntimeIdentity.APP)


def test_pki_dir_default(tmp_path, clean_env, monkeypatch):
    pki = tmp_path / "pki"
    (pki / "trust").mkdir(parents=True)
    bundle = pki / "trust" / "hub-bundle.pem"
    bundle.write_text("-----BEGIN CERTIFICATE-----\n")
    monkeypatch.setenv("G8E_APP_PKI_DIR", str(pki))
    assert resolve_trust_bundle(RuntimeIdentity.APP) == str(bundle)


def test_no_bundle_raises(tmp_path, clean_env, monkeypatch):
    monkeypatch.setenv("G8E_APP_PKI_DIR", str(tmp_path / "nope"))
    with pytest.raises(FileNotFoundError):
        resolve_trust_bundle(RuntimeIdentity.APP)


def test_gateway_identity_uses_gateway_bundle(tmp_path, clean_env, monkeypatch):
    pki = tmp_path / "pki"
    (pki / TRUST_DIRECTORY_NAME).mkdir(parents=True)
    bundle = pki / TRUST_DIRECTORY_NAME / GATEWAY_TRUST_BUNDLE_NAME
    bundle.write_text("-----BEGIN CERTIFICATE-----\n")
    monkeypatch.setenv("G8E_GATEWAY_PKI_DIR", str(pki))
    assert resolve_trust_bundle(RuntimeIdentity.GATEWAY) == str(bundle)


def test_runtime_identities_use_separate_pki_roots(tmp_path, clean_env, monkeypatch):
    app_pki = tmp_path / "app-pki"
    gateway_pki = tmp_path / "gateway-pki"
    (app_pki / TRUST_DIRECTORY_NAME).mkdir(parents=True)
    (gateway_pki / TRUST_DIRECTORY_NAME).mkdir(parents=True)
    app_bundle = app_pki / TRUST_DIRECTORY_NAME / "hub-bundle.pem"
    gateway_bundle = gateway_pki / TRUST_DIRECTORY_NAME / GATEWAY_TRUST_BUNDLE_NAME
    app_bundle.write_text("-----BEGIN CERTIFICATE-----\n")
    gateway_bundle.write_text("-----BEGIN CERTIFICATE-----\n")
    monkeypatch.setenv("G8E_APP_PKI_DIR", str(app_pki))
    monkeypatch.setenv("G8E_GATEWAY_PKI_DIR", str(gateway_pki))

    assert resolve_trust_bundle(RuntimeIdentity.APP) == str(app_bundle)
    assert resolve_trust_bundle(RuntimeIdentity.GATEWAY) == str(gateway_bundle)


def test_no_verify_false_in_clients():
    """The harness must never disable TLS verification."""
    for mod in (collector_mod,):
        src = inspect.getsource(mod)
        assert "verify=False" not in src, (
            f"{mod.__name__} disables TLS verification; use resolve_trust_bundle()"
        )
