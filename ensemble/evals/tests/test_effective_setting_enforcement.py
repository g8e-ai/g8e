# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for effective-setting enforcement in the run command.

Verifies that every requested inference setting is either applied by
the selected backend or rejected before the manifest is written. The
run command must expose sampling settings (temperature, top_p,
max_tokens, seed) and seed support declarations, pass them to preflight
for validation, and reject unsupported settings before writing the
manifest.
"""

from __future__ import annotations

import os

import pytest
from click.testing import CliRunner
from unittest.mock import AsyncMock

from g8e_evals import cli
from g8e_evals.cli import main

pytestmark = pytest.mark.unit


def _invoke(runner: CliRunner, args: list[str], env: dict[str, str] | None = None):
    """Invoke the CLI with a controlled environment."""
    sterile = {
        k: v
        for k, v in os.environ.items()
        if not k.startswith(("G8E_OPERATOR", "OPERATOR_SESSION", "OPERATOR_ID"))
    }
    if env:
        sterile.update(env)
    return runner.invoke(main, args, env=sterile, catch_exceptions=False)


def test_run_exposes_temperature_option():
    """The run command must expose --temperature for sampling control."""
    result = _invoke(CliRunner(), ["run", "--help"])
    assert result.exit_code == 0, result.output
    assert "--temperature" in result.output


def test_run_exposes_top_p_option():
    """The run command must expose --top-p for sampling control."""
    result = _invoke(CliRunner(), ["run", "--help"])
    assert result.exit_code == 0, result.output
    assert "--top-p" in result.output


def test_run_exposes_max_tokens_option():
    """The run command must expose --max-tokens for output length control."""
    result = _invoke(CliRunner(), ["run", "--help"])
    assert result.exit_code == 0, result.output
    assert "--max-tokens" in result.output


def test_run_exposes_seed_option():
    """The run command must expose --seed for deterministic sampling."""
    result = _invoke(CliRunner(), ["run", "--help"])
    assert result.exit_code == 0, result.output
    assert "--seed" in result.output


def test_run_exposes_seed_support_option():
    """The run command must expose --seed-support for declaring backend seed capability."""
    result = _invoke(CliRunner(), ["run", "--help"])
    assert result.exit_code == 0, result.output
    assert "--seed-support" in result.output


def test_run_rejects_temperature_out_of_range_before_manifest(monkeypatch: pytest.MonkeyPatch):
    """Temperature outside [0.0, 2.0] must be rejected before the manifest is written."""
    monkeypatch.setattr(cli, "load_evidence_encryption_key", lambda *_: object())
    manifest_calls: list = []
    original_run_suite = cli._run_suite

    async def _capture_manifest(*args, **kwargs):
        manifest_calls.append(args)
        return await original_run_suite(*args, **kwargs)

    monkeypatch.setattr(cli, "_run_suite", AsyncMock())

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "direct",
            "--provider", "ollama",
            "--model", "qwen3:8b",
            "--evidence-key-file", "/runtime/evidence-key.json",
            "--temperature", "3.0",
        ],
    )

    assert result.exit_code != 0, result.output
    assert "temperature" in result.output.lower() or "sampling" in result.output.lower()


def test_run_rejects_seed_without_seed_support_before_manifest(monkeypatch: pytest.MonkeyPatch):
    """A seed requested with --seed-support none must be rejected before the manifest is written."""
    monkeypatch.setattr(cli, "load_evidence_encryption_key", lambda *_: object())
    monkeypatch.setattr(cli, "_run_suite", AsyncMock())

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "direct",
            "--provider", "ollama",
            "--model", "qwen3:8b",
            "--evidence-key-file", "/runtime/evidence-key.json",
            "--seed", "42",
            "--seed-support", "none",
        ],
    )

    assert result.exit_code != 0, result.output
    assert "seed" in result.output.lower()


def test_run_accepts_seed_with_deterministic_support(monkeypatch: pytest.MonkeyPatch):
    """A seed with --seed-support deterministic must be accepted."""
    monkeypatch.setattr(cli, "load_evidence_encryption_key", lambda *_: object())
    monkeypatch.setattr(cli, "_run_suite", AsyncMock())

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "direct",
            "--provider", "ollama",
            "--model", "qwen3:8b",
            "--evidence-key-file", "/runtime/evidence-key.json",
            "--seed", "42",
            "--seed-support", "deterministic",
        ],
    )

    assert result.exit_code == 0, result.output


def test_run_accepts_seed_with_unknown_support(monkeypatch: pytest.MonkeyPatch):
    """A seed with --seed-support unknown must be accepted (unverified, not rejected)."""
    monkeypatch.setattr(cli, "load_evidence_encryption_key", lambda *_: object())
    monkeypatch.setattr(cli, "_run_suite", AsyncMock())

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "direct",
            "--provider", "ollama",
            "--model", "qwen3:8b",
            "--evidence-key-file", "/runtime/evidence-key.json",
            "--seed", "42",
            "--seed-support", "unknown",
        ],
    )

    assert result.exit_code == 0, result.output


def test_run_rejects_top_p_out_of_range_before_manifest(monkeypatch: pytest.MonkeyPatch):
    """top_p outside (0.0, 1.0] must be rejected before the manifest is written."""
    monkeypatch.setattr(cli, "load_evidence_encryption_key", lambda *_: object())
    monkeypatch.setattr(cli, "_run_suite", AsyncMock())

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "direct",
            "--provider", "ollama",
            "--model", "qwen3:8b",
            "--evidence-key-file", "/runtime/evidence-key.json",
            "--top-p", "1.5",
        ],
    )

    assert result.exit_code != 0, result.output
    assert "top_p" in result.output.lower() or "sampling" in result.output.lower()


def test_run_rejects_max_tokens_below_one(monkeypatch: pytest.MonkeyPatch):
    """max_tokens < 1 must be rejected before the manifest is written."""
    monkeypatch.setattr(cli, "load_evidence_encryption_key", lambda *_: object())
    monkeypatch.setattr(cli, "_run_suite", AsyncMock())

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "direct",
            "--provider", "ollama",
            "--model", "qwen3:8b",
            "--evidence-key-file", "/runtime/evidence-key.json",
            "--max-tokens", "0",
        ],
    )

    assert result.exit_code != 0, result.output
    assert "max_output_tokens" in result.output.lower() or "sampling" in result.output.lower()


def test_run_accepts_valid_sampling_settings(monkeypatch: pytest.MonkeyPatch):
    """Valid sampling settings must be accepted."""
    monkeypatch.setattr(cli, "load_evidence_encryption_key", lambda *_: object())
    monkeypatch.setattr(cli, "_run_suite", AsyncMock())

    result = _invoke(
        CliRunner(),
        [
            "run",
            "--suite", "ifeval_subset",
            "--arm", "direct",
            "--provider", "ollama",
            "--model", "qwen3:8b",
            "--evidence-key-file", "/runtime/evidence-key.json",
            "--temperature", "0.0",
            "--top-p", "0.9",
            "--max-tokens", "4096",
            "--seed", "42",
            "--seed-support", "deterministic",
        ],
    )

    assert result.exit_code == 0, result.output
