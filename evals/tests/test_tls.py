"""Regression tests for trust bundle resolution.

These guard against the silent ``verify=False`` regression that defeats the
harness's receipt-binding guarantee.
"""

import os
import inspect
from pathlib import Path

import pytest

from g8e_evals.tls import resolve_trust_bundle
from g8e_evals.receipts import collector as collector_mod
from g8e_evals.sut import answer_only as answer_only_mod


@pytest.fixture
def clean_env(monkeypatch):
    monkeypatch.delenv("G8E_TRUST_BUNDLE", raising=False)
    monkeypatch.delenv("G8E_PKI_DIR", raising=False)


def test_explicit_trust_bundle_env(tmp_path, clean_env, monkeypatch):
    bundle = tmp_path / "hub-bundle.pem"
    bundle.write_text("-----BEGIN CERTIFICATE-----\n")
    monkeypatch.setenv("G8E_TRUST_BUNDLE", str(bundle))
    assert resolve_trust_bundle() == str(bundle)


def test_explicit_trust_bundle_missing_raises(tmp_path, clean_env, monkeypatch):
    monkeypatch.setenv("G8E_TRUST_BUNDLE", str(tmp_path / "missing.pem"))
    with pytest.raises(FileNotFoundError):
        resolve_trust_bundle()


def test_pki_dir_default(tmp_path, clean_env, monkeypatch):
    pki = tmp_path / "pki"
    (pki / "trust").mkdir(parents=True)
    bundle = pki / "trust" / "hub-bundle.pem"
    bundle.write_text("-----BEGIN CERTIFICATE-----\n")
    monkeypatch.setenv("G8E_PKI_DIR", str(pki))
    assert resolve_trust_bundle() == str(bundle)


def test_no_bundle_raises(tmp_path, clean_env, monkeypatch):
    monkeypatch.setenv("G8E_PKI_DIR", str(tmp_path / "nope"))
    with pytest.raises(FileNotFoundError):
        resolve_trust_bundle()


def test_no_verify_false_in_clients():
    """The harness must never disable TLS verification."""
    for mod in (collector_mod, answer_only_mod):
        src = inspect.getsource(mod)
        assert "verify=False" not in src, (
            f"{mod.__name__} disables TLS verification; use resolve_trust_bundle()"
        )
