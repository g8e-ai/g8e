# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 byte-determinism tests for the canonical analysis CLI path.

Verifies that running the same frozen fixture through ``_run_suite``
twice produces byte-identical canonical analysis artifacts
(``analysis.json``, ``analysis.md``, ``analysis.html``, ``analysis.txt``)
when non-deterministic inputs (``run_id``, timestamps) are held constant.

The test mocks the SUT to return a fixed answer, the receipt collector to
return no receipts, the gateway posture to a fixed value, and patches
``uuid.uuid4`` and ``datetime.now`` in the ``cli`` module so that every
run produces the same ``run_id``, ``started_at``, and ``ended_at`` values.
The canonical analysis is a pure function of the ``AnalysisInputRecord``,
so identical inputs must produce identical output bytes.
"""

from __future__ import annotations

import uuid
from datetime import UTC, datetime
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.models.model_telemetry import ModelCallTelemetry
from g8e_evals import cli
from g8e_evals.arms import Arm, GovernancePosture
from g8e_evals.auth_bridge import CLIAuthContext
from g8e_evals.evidence import EvidenceEncryptionKey
from g8e_evals.harness import BindingType, LLMRoleConfig, Response, SUTConfig, Task
from g8e_evals.models import ScoreDetails

pytestmark = pytest.mark.unit

_FIXED_DATETIME = datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC)
_FIXED_UUID = uuid.UUID("00000000-0000-0000-0000-000000000001")


def _evidence_key() -> EvidenceEncryptionKey:
    return EvidenceEncryptionKey(key_id="test-key", key=b"k" * 32)


def _auth_context() -> CLIAuthContext:
    return CLIAuthContext(
        operator_session_id="op-session",
        cli_session_id="cli-session",
        user_id="user-1",
        operator_id="op-1",
        client_cert="/runtime/cli.crt",
        client_key="/runtime/cli.key",
    )


def _task() -> Task:
    return Task(id="1001", prompt="Write a sentence without commas.")


def _receipt(terminal_event: str | None = None):
    from g8e_evals.sut.g8ee_chat import AgentTrailEvent, ChatEvaluationReceipt

    return ChatEvaluationReceipt(
        case_id="case-1",
        investigation_id="inv-1",
        terminal_event=terminal_event,
        answer_chars=0,
        event_count=1,
        event_counts_by_type={"x": 1},
        agent_trail=[AgentTrailEvent(id=1, event_type="x", payload={})],
    )


def _score(passed: bool = True, model_calls: list[ModelCallTelemetry] | None = None):
    from g8e_evals.harness import Score

    return Score(task_id="1001", passed=passed, details=ScoreDetails(), model_calls=model_calls or [])


def _patch_loader(monkeypatch, tasks: list[Task]) -> None:
    class _StubLoader:
        def __init__(self, path):
            pass

        def load(self):
            yield from tasks

    monkeypatch.setattr(cli, "IFEvalLoader", _StubLoader)


def _patch_provenance(monkeypatch) -> None:
    from g8e_evals.benchmarks.ifeval.provenance import (
        DatasetOutput,
        DatasetProvenance,
        DatasetSource,
        DatasetTransformation,
    )

    provenance = DatasetProvenance(
        schema_version=1,
        benchmark="ifeval_subset",
        source=DatasetSource(
            url="https://example.com",
            revision="rev",
            license_spdx="Apache-2.0",
            license_url="https://example.com",
            sha256="0" * 64,
        ),
        selected_keys=[1001],
        transformation=DatasetTransformation(
            description="stub",
            code_path="stub",
            code_sha256="0" * 64,
            fixture_path="stub",
            fixture_sha256="0" * 64,
        ),
        output=DatasetOutput(path="input_data.jsonl", rows=1, sha256="0" * 64),
        partition="development",
        domain_strata=["utility"],
    )
    monkeypatch.setattr(cli, "load_provenance", lambda _path: provenance)


def _patch_sut(monkeypatch, *, settings=None, answer_response=None) -> MagicMock:
    sut = MagicMock()
    sut.check_settings = AsyncMock(return_value=settings)
    if answer_response is not None:
        sut.get_answer = AsyncMock(return_value=answer_response)
    else:
        sut.get_answer = AsyncMock(return_value=Response(
            answer="hello world", model="test",
            chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
            binding=BindingType.UNBOUND, unbound_reason="answer-only turn",
        ))
    monkeypatch.setattr(cli, "G8eeChatSUT", lambda *a, **kw: sut)
    return sut


def _patch_collector(monkeypatch) -> MagicMock:
    collector = MagicMock()
    collector.collect_receipt = AsyncMock(return_value=None)
    collector.collect_receipt_for_investigation = AsyncMock(return_value=None)
    monkeypatch.setattr(cli, "ReceiptCollector", lambda *a, **kw: collector)
    return collector


def _patch_verifier(
    monkeypatch,
    passed: bool = True,
    model_calls: list[ModelCallTelemetry] | None = None,
) -> MagicMock:
    verifier = MagicMock()
    verifier.verify.return_value = _score(passed, model_calls)
    monkeypatch.setattr(cli, "IFEvalVerifier", lambda: verifier)
    return verifier


def _patch_posture(monkeypatch, posture: GovernancePosture | None = GovernancePosture.L1_DOCTRINE) -> AsyncMock:
    mock = AsyncMock(return_value=posture)
    monkeypatch.setattr(cli, "observe_gateway_posture", mock)
    monkeypatch.setattr(cli.AuthContext, "from_env", MagicMock(return_value=MagicMock()))
    return mock


class _FakeDateTime(datetime):
    """Datetime subclass that returns a fixed value from ``now()``."""

    @classmethod
    def now(cls, tz=None):
        return _FIXED_DATETIME


def _patch_determinism(monkeypatch) -> None:
    """Patch non-deterministic sources in the ``cli`` module.

    Patches ``uuid.uuid4`` to return a fixed UUID and ``datetime`` to a
    subclass whose ``now()`` returns a fixed timestamp. Only the ``cli``
    module is patched because the ifeval path only calls ``datetime.now``
    in ``cli.py`` (for ``started_at``/``ended_at`` on attempts and the
    report directory timestamp).
    """
    monkeypatch.setattr(cli, "uuid", SimpleNamespace(uuid4=lambda: _FIXED_UUID))
    monkeypatch.setattr(cli, "datetime", _FakeDateTime)


def _config() -> SUTConfig:
    return SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )


def _setup_and_run(monkeypatch, tmp_path) -> None:
    """Set up deterministic mocks for the ifeval suite.

    This is a sync helper that sets up the mocks; the caller is responsible
    for running the async ``_run_suite`` coroutine.
    """
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(
        monkeypatch,
        settings=MagicMock(llm=MagicMock(primary_model="m")),
        answer_response=Response(
            answer="A valid answer.", model="test",
            chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
            binding=BindingType.UNBOUND, unbound_reason="answer-only turn",
        ),
    )
    _patch_collector(monkeypatch)
    _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)
    _patch_determinism(monkeypatch)


async def _run_and_read(tmp_path) -> dict[str, bytes]:
    await cli._run_suite(
        "ifeval_subset", _config(), None, tmp_path, limit=1,
        evidence_key=_evidence_key(),
    )
    report_dirs = [p for p in tmp_path.iterdir() if p.is_dir()]
    assert len(report_dirs) == 1, f"expected one report dir, got {report_dirs}"
    report_dir = report_dirs[0]
    artifacts = {}
    for name in ("analysis.json", "analysis.md", "analysis.html", "analysis.txt"):
        path = report_dir / name
        assert path.exists(), f"{name} must be written by the release path"
        artifacts[name] = path.read_bytes()
    return artifacts


class TestCanonicalAnalysisDeterminism:
    """Byte-determinism tests for the canonical analysis CLI path."""

    @pytest.mark.asyncio
    async def test_repeated_run_produces_byte_identical_analysis_artifacts(self, tmp_path, monkeypatch) -> None:
        """Running the same frozen fixture twice produces byte-identical analysis artifacts.

        The canonical analysis is a pure function of the AnalysisInputRecord.
        When run_id and timestamps are held constant, two runs must produce
        identical analysis.json, analysis.md, analysis.html, and analysis.txt
        bytes. Any difference indicates a hidden non-deterministic input or a
        renderer that depends on mutable global state.
        """
        first_dir = tmp_path / "first"
        first_dir.mkdir()
        _setup_and_run(monkeypatch, first_dir)
        first = await _run_and_read(first_dir)

        second_dir = tmp_path / "second"
        second_dir.mkdir()
        _setup_and_run(monkeypatch, second_dir)
        second = await _run_and_read(second_dir)

        for name in ("analysis.json", "analysis.md", "analysis.html", "analysis.txt"):
            assert first[name] == second[name], (
                f"{name} differs across two runs of the same frozen fixture; "
                f"canonical analysis is not byte-deterministic"
            )
