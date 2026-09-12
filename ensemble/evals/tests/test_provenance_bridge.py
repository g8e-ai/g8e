# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the g8e CLI build-provenance bridge.

``load_cli_build_provenance`` shells out to ``g8e version --json`` and
validates the stamped provenance. The bridge returns ``None`` when the
channel cannot supply provenance (missing binary, non-zero exit,
unstamped development build) and raises ``ProvenanceBridgeError`` only
when a reachable channel returns a malformed payload.
"""

from __future__ import annotations

import json
import subprocess

import pytest

from g8e_evals.provenance_bridge import (
    ProvenanceBridgeError,
    load_cli_build_provenance,
)

pytestmark = pytest.mark.unit

_VALID_HASH = "b" * 64


def _version_payload(**overrides: object) -> dict[str, object]:
    payload: dict[str, object] = {
        "version": "v2.1.8",
        "build_id": "4ca29cf0c",
        "build_time": "2026-09-12T00:00:00Z",
        "platform": "linux_amd64",
        "source_revision": "4ca29cf0c472f093cf5c54c38888a9f094411a2c",
        "source_tree_state_hash": _VALID_HASH,
        "source_tree_modified": True,
    }
    payload.update(overrides)
    return payload


def _completed(payload: dict[str, object]) -> subprocess.CompletedProcess[str]:
    return subprocess.CompletedProcess(["./g8e"], 0, stdout=json.dumps(payload), stderr="")


def test_load_cli_build_provenance_reads_stamped_binary(monkeypatch: pytest.MonkeyPatch):
    calls: list[list[str]] = []

    def run(command: list[str], **kwargs: object) -> subprocess.CompletedProcess[str]:
        calls.append(command)
        return _completed(_version_payload())

    monkeypatch.setattr(subprocess, "run", run)

    provenance = load_cli_build_provenance("./g8e")

    assert provenance is not None
    assert provenance.source_revision == "4ca29cf0c472f093cf5c54c38888a9f094411a2c"
    assert provenance.source_tree_state_hash == _VALID_HASH
    assert provenance.build_id == "4ca29cf0c"
    assert provenance.build_system == "g8e-cli"
    assert calls == [["./g8e", "version", "--json"]]


def test_load_cli_build_provenance_omits_unknown_build_id(monkeypatch: pytest.MonkeyPatch):
    monkeypatch.setattr(
        subprocess,
        "run",
        lambda *args, **kwargs: _completed(_version_payload(build_id="unknown")),
    )

    provenance = load_cli_build_provenance("./g8e")

    assert provenance is not None
    assert provenance.build_id == ""


@pytest.mark.parametrize(
    "payload",
    [
        _version_payload(source_revision="", source_tree_state_hash=_VALID_HASH),
        _version_payload(source_revision="abc123", source_tree_state_hash=""),
        {"version": "dev"},
    ],
    ids=["missing-revision", "missing-tree-hash", "unstamped-dev-build"],
)
def test_load_cli_build_provenance_returns_none_when_unstamped(
    monkeypatch: pytest.MonkeyPatch,
    payload: dict[str, object],
):
    monkeypatch.setattr(subprocess, "run", lambda *args, **kwargs: _completed(payload))

    assert load_cli_build_provenance("./g8e") is None


def test_load_cli_build_provenance_returns_none_on_nonzero_exit(monkeypatch: pytest.MonkeyPatch):
    monkeypatch.setattr(
        subprocess,
        "run",
        lambda *args, **kwargs: subprocess.CompletedProcess(["./g8e"], 1, stdout="", stderr="unknown flag --json"),
    )

    assert load_cli_build_provenance("./g8e") is None


def test_load_cli_build_provenance_returns_none_when_binary_missing(monkeypatch: pytest.MonkeyPatch):
    def run(*args: object, **kwargs: object) -> subprocess.CompletedProcess[str]:
        raise FileNotFoundError("./g8e")

    monkeypatch.setattr(subprocess, "run", run)

    assert load_cli_build_provenance("./g8e") is None


@pytest.mark.parametrize("stdout", ["not-json", ""], ids=["non-json", "empty"])
def test_load_cli_build_provenance_raises_on_malformed_json(
    monkeypatch: pytest.MonkeyPatch,
    stdout: str,
):
    monkeypatch.setattr(
        subprocess,
        "run",
        lambda *args, **kwargs: subprocess.CompletedProcess(["./g8e"], 0, stdout=stdout, stderr=""),
    )

    with pytest.raises(ProvenanceBridgeError, match="invalid JSON"):
        load_cli_build_provenance("./g8e")


def test_load_cli_build_provenance_raises_on_contract_violation(monkeypatch: pytest.MonkeyPatch):
    monkeypatch.setattr(
        subprocess,
        "run",
        lambda *args, **kwargs: _completed(_version_payload(unexpected_field="x")),
    )

    with pytest.raises(ProvenanceBridgeError, match="invalid JSON"):
        load_cli_build_provenance("./g8e")


def test_load_cli_build_provenance_raises_on_invalid_hash_shape(monkeypatch: pytest.MonkeyPatch):
    monkeypatch.setattr(
        subprocess,
        "run",
        lambda *args, **kwargs: _completed(_version_payload(source_tree_state_hash="not-a-hash")),
    )

    with pytest.raises(ProvenanceBridgeError, match="invalid provenance stamp"):
        load_cli_build_provenance("./g8e")
