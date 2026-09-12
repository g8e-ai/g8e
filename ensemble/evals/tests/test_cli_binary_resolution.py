# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tests for g8e CLI binary resolution (``g8e_evals.cli_binary``).

``resolve_g8e_cli`` implements the evals binary contract: an explicit
``--g8e-cli`` path, the ``auto`` sentinel fetching the binary served by
the running Gateway at ``/.well-known/g8e/bin/``, then implicit
candidates (repo-root ``./g8e``, ``bin/g8e-<platform>``, ``PATH``).
Pure mapping and ordering cases are Tier 1; filesystem and fetch cases
are Tier 2.
"""

from __future__ import annotations

import hashlib
import json
import os
import platform
import stat
import subprocess
import sys
from pathlib import Path

import httpx
import pytest

from g8e_evals import cli_binary
from g8e_evals.cli_binary import (
    CLIBinaryError,
    platform_binary_name,
    resolve_g8e_cli,
)
from g8e_evals.provenance_bridge import load_cli_build_provenance


@pytest.mark.unit
class TestPlatformBinaryName:
    @pytest.mark.parametrize(
        ("sys_platform", "machine", "expected"),
        [
            ("linux", "x86_64", "g8e-linux-amd64"),
            ("linux", "aarch64", "g8e-linux-arm64"),
            ("darwin", "arm64", "g8e-darwin-arm64"),
            ("darwin", "x86_64", "g8e-darwin-amd64"),
            ("win32", "AMD64", "g8e-windows-amd64.exe"),
            ("linux", "i686", "g8e-linux-386"),
        ],
    )
    def test_maps_host_platform_to_artifact_name(
        self, monkeypatch: pytest.MonkeyPatch, sys_platform: str, machine: str, expected: str
    ):
        monkeypatch.setattr(sys, "platform", sys_platform)
        monkeypatch.setattr(platform, "machine", lambda: machine)

        assert platform_binary_name() == expected

    def test_rejects_unsupported_os(self, monkeypatch: pytest.MonkeyPatch):
        monkeypatch.setattr(sys, "platform", "plan9")

        with pytest.raises(CLIBinaryError, match="unsupported host OS"):
            platform_binary_name()

    def test_rejects_unsupported_arch(self, monkeypatch: pytest.MonkeyPatch):
        monkeypatch.setattr(sys, "platform", "linux")
        monkeypatch.setattr(platform, "machine", lambda: "riscv64")

        with pytest.raises(CLIBinaryError, match="unsupported host architecture"):
            platform_binary_name()


@pytest.mark.unit
class TestImplicitCandidateOrdering:
    def test_returns_first_usable_candidate(self, monkeypatch: pytest.MonkeyPatch):
        candidates = [Path("/repo/g8e"), Path("/repo/bin/g8e-linux-amd64"), Path("/usr/bin/g8e")]
        monkeypatch.setattr(cli_binary, "_candidate_paths", lambda: candidates)
        monkeypatch.setattr(cli_binary, "_is_usable_binary", lambda p: p.name == "g8e")

        assert resolve_g8e_cli(None) == "/repo/g8e"

    def test_falls_through_unusable_candidates(self, monkeypatch: pytest.MonkeyPatch):
        candidates = [Path("/repo/g8e"), Path("/repo/bin/g8e-linux-amd64")]
        monkeypatch.setattr(cli_binary, "_candidate_paths", lambda: candidates)
        monkeypatch.setattr(cli_binary, "_is_usable_binary", lambda p: "bin" in p.parts)

        assert resolve_g8e_cli(None) == "/repo/bin/g8e-linux-amd64"

    def test_returns_none_when_no_candidate_exists(self, monkeypatch: pytest.MonkeyPatch):
        monkeypatch.setattr(cli_binary, "_candidate_paths", list)

        assert resolve_g8e_cli(None) is None
        assert resolve_g8e_cli("") is None


@pytest.mark.integration
class TestExplicitPathResolution:
    def test_resolves_existing_executable(self, tmp_path: Path):
        binary = tmp_path / "g8e"
        binary.write_bytes(b"#!/bin/sh\n")
        binary.chmod(binary.stat().st_mode | stat.S_IXUSR)

        assert resolve_g8e_cli(str(binary)) == str(binary)

    def test_rejects_missing_explicit_path(self, tmp_path: Path):
        with pytest.raises(CLIBinaryError, match="not found"):
            resolve_g8e_cli(str(tmp_path / "does-not-exist"))

    @pytest.mark.skipif(sys.platform in ("win32", "cygwin"), reason="POSIX executable-bit check")
    def test_rejects_non_executable_explicit_path(self, tmp_path: Path):
        binary = tmp_path / "g8e"
        binary.write_bytes(b"#!/bin/sh\n")
        binary.chmod(0o644)

        with pytest.raises(CLIBinaryError, match="not executable"):
            resolve_g8e_cli(str(binary))


@pytest.mark.integration
class TestGatewayFetch:
    def _fake_response(self, status: int, content: bytes = b"", headers: dict[str, str] | None = None) -> httpx.Response:
        return httpx.Response(status, content=content, headers=headers or {})

    def test_auto_fetches_and_caches_binary(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
        monkeypatch.setattr(
            cli_binary.httpx,
            "get",
            lambda url, **kwargs: self._fake_response(200, content=b"BINARY-BYTES"),
        )

        resolved = resolve_g8e_cli("auto", gateway_base_url="http://gateway:8080", cache_dir=tmp_path)

        assert resolved is not None
        path = Path(resolved)
        assert path == tmp_path / platform_binary_name()
        assert path.read_bytes() == b"BINARY-BYTES"
        assert os.access(path, os.X_OK)

    def test_auto_reuses_cache_on_304(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
        cached = tmp_path / platform_binary_name()
        cached.write_bytes(b"CACHED")
        requests: list[dict[str, object]] = []

        def fake_get(url: str, **kwargs: object) -> httpx.Response:
            requests.append(kwargs)
            return self._fake_response(304)

        monkeypatch.setattr(cli_binary.httpx, "get", fake_get)

        resolved = resolve_g8e_cli("auto", gateway_base_url="http://gateway:8080", cache_dir=tmp_path)

        assert resolved == str(cached)
        headers = requests[0]["headers"]
        assert isinstance(headers, dict)
        assert "If-Modified-Since" in headers

    def test_auto_fails_on_gateway_error(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
        monkeypatch.setattr(
            cli_binary.httpx,
            "get",
            lambda url, **kwargs: self._fake_response(404),
        )

        with pytest.raises(CLIBinaryError, match="HTTP 404"):
            resolve_g8e_cli("auto", gateway_base_url="http://gateway:8080", cache_dir=tmp_path)

    def test_auto_fails_on_transport_error(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
        def fake_get(url: str, **kwargs: object) -> httpx.Response:
            raise httpx.ConnectError("refused")

        monkeypatch.setattr(cli_binary.httpx, "get", fake_get)

        with pytest.raises(CLIBinaryError, match="could not fetch"):
            resolve_g8e_cli("auto", gateway_base_url="http://gateway:8080", cache_dir=tmp_path)


@pytest.mark.integration
class TestBinaryHashProvenance:
    def test_provenance_records_binary_sha256_and_modified_flag(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ):
        binary = tmp_path / "g8e-resolved"
        content = b"binary-content-for-hashing"
        binary.write_bytes(content)
        binary.chmod(0o755)

        payload = {
            "version": "v2.1.8",
            "build_id": "4ca29cf0c",
            "source_revision": "4ca29cf0c472f093cf5c54c38888a9f094411a2c",
            "source_tree_state_hash": "b" * 64,
            "source_tree_modified": True,
        }
        monkeypatch.setattr(
            subprocess,
            "run",
            lambda *args, **kwargs: subprocess.CompletedProcess(
                [str(binary)], 0, stdout=json.dumps(payload), stderr=""
            ),
        )

        provenance = load_cli_build_provenance(str(binary))

        assert provenance is not None
        assert provenance.binary_sha256 == hashlib.sha256(content).hexdigest()
        assert provenance.source_tree_modified is True
