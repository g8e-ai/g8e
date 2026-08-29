# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Direct regression tests for ``g8e_evals.cli._run_suite`` invalid-evidence
classification and diagnostic-report retention.

The plan requires a direct regression that executes ``_run_suite``, inspects a
physically written failed report, and covers each invalid-evidence
classification:

  - settings HTTP 401/403 (preflight AuthenticationError)
  - chat HTTP 401/403 (unbound_reason contains the status)
  - missing terminal event
  - failure terminal event
  - empty answer

Each failure case must raise ``EvaluationRunError`` (nonzero CLI exit) while
retaining the diagnostic report on disk. A valid response must not raise and
must produce a report with the expected rows.

Preflight failures (settings 401/403) raise before the execution loop and
therefore do not create a report directory — the diagnostic surface is the
command output only. This is the current behavior and these tests pin it.
"""

from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock

import pytest

from g8e_evals import cli
from g8e_evals.arms import Arm
from g8e_evals.auth_bridge import CLIAuthContext
from g8e_evals.harness import BindingType, LLMRoleConfig, Response, SUTConfig, Task
from g8e_evals.models import ScoreDetails
from g8e_evals.sut.g8ee_chat import AgentTrailEvent, ChatEvaluationReceipt, AuthenticationError

pytestmark = pytest.mark.unit


def _auth_context() -> CLIAuthContext:
    return CLIAuthContext(
        operator_session_id="op-session",
        cli_session_id="cli-session",
        user_id="user-1",
        operator_id="op-1",
        client_cert="/runtime/cli.crt",
        client_key="/runtime/cli.key",
    )


def _config() -> SUTConfig:
    return SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )


def _task() -> Task:
    return Task(id="1001", prompt="Write a sentence without commas.")


def _receipt(terminal_event: str | None = None) -> ChatEvaluationReceipt:
    return ChatEvaluationReceipt(
        case_id="case-1",
        investigation_id="inv-1",
        terminal_event=terminal_event,
        answer_chars=0,
        event_count=1,
        event_counts_by_type={"x": 1},
        agent_trail=[AgentTrailEvent(id=1, event_type="x", payload={})],
    )


def _score(passed: bool = True):
    from g8e_evals.harness import Score

    return Score(task_id="1001", passed=passed, details=ScoreDetails())


def _patch_loader(monkeypatch, tasks: list[Task]) -> None:
    """Replace IFEvalLoader with a stub that yields the given tasks."""
    class _StubLoader:
        def __init__(self, path):
            pass

        def load(self):
            yield from tasks

    monkeypatch.setattr(cli, "IFEvalLoader", _StubLoader)


def _patch_provenance(monkeypatch) -> None:
    """Replace load_provenance with a stub returning a minimal provenance."""
    from g8e_evals.benchmarks.ifeval.provenance import DatasetProvenance, DatasetOutput

    provenance = DatasetProvenance(
        schema_version=1,
        benchmark="ifeval_subset",
        source=__import__("g8e_evals.benchmarks.ifeval.provenance", fromlist=["DatasetSource"]).DatasetSource(
            url="https://example.com",
            revision="rev",
            license_spdx="Apache-2.0",
            license_url="https://example.com",
            sha256="0" * 64,
        ),
        selected_keys=[1001],
        transformation=__import__("g8e_evals.benchmarks.ifeval.provenance", fromlist=["DatasetTransformation"]).DatasetTransformation(
            description="stub",
            code_path="stub",
            code_sha256="0" * 64,
            fixture_path="stub",
            fixture_sha256="0" * 64,
        ),
        output=DatasetOutput(path="input_data.jsonl", rows=1, sha256="0" * 64),
    )
    monkeypatch.setattr(cli, "load_provenance", lambda _path: provenance)


def _patch_verifier(monkeypatch, passed: bool = True) -> MagicMock:
    verifier = MagicMock()
    verifier.verify.return_value = _score(passed)
    monkeypatch.setattr(cli, "IFEvalVerifier", lambda: verifier)
    return verifier


def _patch_sut(
    monkeypatch,
    *,
    settings: object = None,
    settings_error: Exception | None = None,
    answer_response: Response | None = None,
) -> MagicMock:
    """Replace G8eeChatSUT with a stub returning controlled settings/responses."""
    sut = MagicMock()
    if settings_error is not None:
        sut.check_settings = AsyncMock(side_effect=settings_error)
    else:
        sut.check_settings = AsyncMock(return_value=settings)
    if answer_response is not None:
        sut.get_answer = AsyncMock(return_value=answer_response)
    else:
        sut.get_answer = AsyncMock(return_value=Response(
            answer="hello world", model="test", chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
            binding=BindingType.UNBOUND, unbound_reason="answer-only turn",
        ))
    monkeypatch.setattr(cli, "G8eeChatSUT", lambda *a, **kw: sut)
    return sut


def _patch_collector(monkeypatch) -> MagicMock:
    collector = MagicMock()
    collector.collect_receipt = AsyncMock(return_value=None)
    monkeypatch.setattr(cli, "ReceiptCollector", lambda *a, **kw: collector)
    return collector


# ---------------------------------------------------------------------------
# Preflight failures (settings 401/403) — no report directory is created.
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_run_suite_settings_401_raises_and_writes_no_report(tmp_path, monkeypatch):
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(monkeypatch, settings_error=AuthenticationError("g8ee settings returned HTTP 401"))
    _patch_collector(monkeypatch)

    with pytest.raises(cli.EvaluationRunError, match="preflight authentication failed"):
        await cli._run_suite("ifeval_subset", _config(), None, tmp_path, limit=1)

    # Preflight failures raise before the execution loop; no report directory
    # is created. The diagnostic surface is the command output only.
    report_dirs = list(tmp_path.iterdir())
    assert report_dirs == [], f"no report directory expected for preflight failure, got {report_dirs}"


@pytest.mark.asyncio
async def test_run_suite_settings_403_raises_and_writes_no_report(tmp_path, monkeypatch):
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(monkeypatch, settings_error=AuthenticationError("g8ee settings returned HTTP 403"))
    _patch_collector(monkeypatch)

    with pytest.raises(cli.EvaluationRunError, match="preflight authentication failed"):
        await cli._run_suite("ifeval_subset", _config(), None, tmp_path, limit=1)

    report_dirs = list(tmp_path.iterdir())
    assert report_dirs == [], f"no report directory expected for preflight failure, got {report_dirs}"


# ---------------------------------------------------------------------------
# Task-level failures — report IS written before EvaluationRunError is raised.
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_run_suite_chat_401_retains_diagnostic_report(tmp_path, monkeypatch):
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")),
               answer_response=Response(
                   answer="", model="test", chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
                   binding=BindingType.UNBOUND, unbound_reason="g8ee chat returned HTTP 401 Unauthorized.",
               ))
    _patch_collector(monkeypatch)

    with pytest.raises(cli.EvaluationRunError, match="invalid evidence"):
        await cli._run_suite("ifeval_subset", _config(), None, tmp_path, limit=1)

    _assert_report_has_results(tmp_path, expected_answer="")


@pytest.mark.asyncio
async def test_run_suite_chat_403_retains_diagnostic_report(tmp_path, monkeypatch):
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")),
               answer_response=Response(
                   answer="", model="test", chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
                   binding=BindingType.UNBOUND, unbound_reason="g8ee chat returned HTTP 403 Forbidden.",
               ))
    _patch_collector(monkeypatch)

    with pytest.raises(cli.EvaluationRunError, match="invalid evidence"):
        await cli._run_suite("ifeval_subset", _config(), None, tmp_path, limit=1)

    _assert_report_has_results(tmp_path, expected_answer="")


@pytest.mark.asyncio
async def test_run_suite_missing_terminal_event_retains_diagnostic_report(tmp_path, monkeypatch):
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")),
               answer_response=Response(
                   answer="partial text", model="test", chat_evidence=_receipt(terminal_event=None),
                   binding=BindingType.UNBOUND, unbound_reason="idle timeout after 180s without terminal event",
               ))
    _patch_collector(monkeypatch)

    with pytest.raises(cli.EvaluationRunError, match="invalid evidence"):
        await cli._run_suite("ifeval_subset", _config(), None, tmp_path, limit=1)

    _assert_report_has_results(tmp_path, expected_answer="partial text")


@pytest.mark.asyncio
async def test_run_suite_failure_terminal_event_retains_diagnostic_report(tmp_path, monkeypatch):
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")),
               answer_response=Response(
                   answer="", model="test",
                   chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.failed"),
                   binding=BindingType.UNBOUND, unbound_reason="chat terminated with g8e.v1.ai.llm.chat.iteration.failed",
               ))
    _patch_collector(monkeypatch)

    with pytest.raises(cli.EvaluationRunError, match="invalid evidence"):
        await cli._run_suite("ifeval_subset", _config(), None, tmp_path, limit=1)

    _assert_report_has_results(tmp_path, expected_answer="")


@pytest.mark.asyncio
async def test_run_suite_empty_answer_retains_diagnostic_report(tmp_path, monkeypatch):
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")),
               answer_response=Response(
                   answer="", model="test",
                   chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
                   binding=BindingType.UNBOUND, unbound_reason="answer-only turn",
               ))
    _patch_collector(monkeypatch)

    with pytest.raises(cli.EvaluationRunError, match="invalid evidence"):
        await cli._run_suite("ifeval_subset", _config(), None, tmp_path, limit=1)

    _assert_report_has_results(tmp_path, expected_answer="")


# ---------------------------------------------------------------------------
# Valid response — no error, report written with expected content.
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_run_suite_valid_response_writes_report_and_does_not_raise(tmp_path, monkeypatch):
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch, passed=True)
    _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")),
               answer_response=Response(
                   answer="A valid answer without commas.", model="test",
                   chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
                   binding=BindingType.UNBOUND, unbound_reason="answer-only turn",
               ))
    _patch_collector(monkeypatch)

    # Must not raise.
    await cli._run_suite("ifeval_subset", _config(), None, tmp_path, limit=1)

    rows = _assert_report_has_results(tmp_path, expected_answer="A valid answer without commas.")
    assert rows[0]["passed"] is True


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _assert_report_has_results(output_dir: Path, expected_answer: str) -> list[dict]:
    """Find the single report directory and verify results.jsonl exists with one row."""
    report_dirs = [p for p in output_dir.iterdir() if p.is_dir()]
    assert len(report_dirs) == 1, f"expected exactly one report dir, got {report_dirs}"
    results_path = report_dirs[0] / "results.jsonl"
    assert results_path.exists(), f"results.jsonl missing at {results_path}"
    lines = results_path.read_text().splitlines()
    assert len(lines) == 1, f"expected one result row, got {len(lines)}"
    row = json.loads(lines[0])
    assert row["answer"] == expected_answer
    assert "chat_evidence" in row
    return [row]
