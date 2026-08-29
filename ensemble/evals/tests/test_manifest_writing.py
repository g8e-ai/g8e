# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for manifest writing and refusal semantics.

Verifies that the CLI writes ``manifest.json`` before execution begins,
refuses to run when required model identities are unavailable (direct
arm), and produces schema-valid ``tasks.jsonl`` and ``attempts.jsonl``
records.
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
from g8e_evals.schema import AttemptRecord, RunManifest, TaskDefinition
from g8e_evals.sut.g8ee_chat import AgentTrailEvent, ChatEvaluationReceipt

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
    monkeypatch.setattr(cli, "ReceiptCollector", lambda *a, **kw: collector)
    return collector


def _patch_verifier(monkeypatch, passed: bool = True) -> MagicMock:
    verifier = MagicMock()
    verifier.verify.return_value = _score(passed)
    monkeypatch.setattr(cli, "IFEvalVerifier", lambda: verifier)
    return verifier


@pytest.mark.asyncio
async def test_manifest_written_before_execution(tmp_path, monkeypatch):
    """manifest.json must exist in the report directory after a successful run."""
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")),
               answer_response=Response(
                   answer="A valid answer.", model="test",
                   chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
                   binding=BindingType.UNBOUND, unbound_reason="answer-only turn",
               ))
    _patch_collector(monkeypatch)

    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )

    await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1)

    report_dirs = [p for p in tmp_path.iterdir() if p.is_dir()]
    assert len(report_dirs) == 1
    manifest_path = report_dirs[0] / "manifest.json"
    assert manifest_path.exists(), "manifest.json must be written before execution"

    manifest_data = json.loads(manifest_path.read_text())
    manifest = RunManifest.model_validate(manifest_data)
    assert manifest.suite_id == "ifeval_subset"
    assert len(manifest.arms) == 1
    assert manifest.arms[0].arm_id == Arm.DOCTRINE
    assert manifest.dataset_hash is not None
    assert manifest.prompt_bundle_hash is not None
    assert manifest.grader_bundle_hash is not None


@pytest.mark.asyncio
async def test_tasks_jsonl_written_with_schema_valid_records(tmp_path, monkeypatch):
    """tasks.jsonl must contain schema-valid TaskDefinition records."""
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")),
               answer_response=Response(
                   answer="A valid answer.", model="test",
                   chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
                   binding=BindingType.UNBOUND, unbound_reason="answer-only turn",
               ))
    _patch_collector(monkeypatch)

    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )

    await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1)

    report_dirs = [p for p in tmp_path.iterdir() if p.is_dir()]
    tasks_path = report_dirs[0] / "tasks.jsonl"
    assert tasks_path.exists()

    lines = tasks_path.read_text().splitlines()
    assert len(lines) == 1
    td = TaskDefinition.model_validate_json(lines[0])
    assert td.task_id == "1001"
    assert td.prompt_hash is not None
    assert len(td.prompt_hash) == 64
    assert td.grader_ids == ["ifeval_subset_verifier"]


@pytest.mark.asyncio
async def test_attempts_jsonl_written_with_schema_valid_records(tmp_path, monkeypatch):
    """attempts.jsonl must contain schema-valid AttemptRecord records."""
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")),
               answer_response=Response(
                   answer="A valid answer.", model="test",
                   chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
                   binding=BindingType.UNBOUND, unbound_reason="answer-only turn",
               ))
    _patch_collector(monkeypatch)

    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )

    await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1)

    report_dirs = [p for p in tmp_path.iterdir() if p.is_dir()]
    attempts_path = report_dirs[0] / "attempts.jsonl"
    assert attempts_path.exists()

    lines = attempts_path.read_text().splitlines()
    assert len(lines) == 1
    ar = AttemptRecord.model_validate_json(lines[0])
    assert ar.task_id == "1001"
    assert ar.arm_id == Arm.DOCTRINE
    assert ar.posture.requested_posture.value == "l1_doctrine"


@pytest.mark.asyncio
async def test_direct_arm_refuses_without_primary_model_identity(tmp_path, monkeypatch):
    """The direct arm must refuse to run when the primary model identity is unavailable."""
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)

    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(),
        arm=Arm.DIRECT,
    )

    with pytest.raises(cli.EvaluationRunError, match="direct arm requires a primary model identity"):
        await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1)

    # No report directory should be created for a refusal.
    report_dirs = [p for p in tmp_path.iterdir() if p.is_dir()]
    assert report_dirs == []


@pytest.mark.asyncio
async def test_manifest_records_arm_and_posture(tmp_path, monkeypatch):
    """The manifest must record the correct arm and requested posture."""
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")),
               answer_response=Response(
                   answer="A valid answer.", model="test",
                   chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
                   binding=BindingType.UNBOUND, unbound_reason="answer-only turn",
               ))
    _patch_collector(monkeypatch)

    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.CONSENSUS,
    )

    await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1)

    report_dirs = [p for p in tmp_path.iterdir() if p.is_dir()]
    manifest_data = json.loads((report_dirs[0] / "manifest.json").read_text())
    manifest = RunManifest.model_validate(manifest_data)
    assert manifest.arms[0].arm_id == Arm.CONSENSUS
    assert manifest.arms[0].requested_posture.value == "l2_consensus"
    assert manifest.arms[0].receipt_binding is True


@pytest.mark.asyncio
async def test_attempt_records_posture_observation(tmp_path, monkeypatch):
    """Each attempt record must capture requested and observed effective posture."""
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")),
               answer_response=Response(
                   answer="A valid answer.", model="test",
                   chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
                   binding=BindingType.UNBOUND, unbound_reason="answer-only turn",
               ))
    _patch_collector(monkeypatch)

    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.NOTARY,
    )

    await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1)

    report_dirs = [p for p in tmp_path.iterdir() if p.is_dir()]
    attempts_path = report_dirs[0] / "attempts.jsonl"
    lines = attempts_path.read_text().splitlines()
    ar = AttemptRecord.model_validate_json(lines[0])
    assert ar.posture.requested_posture.value == "l3_notary"
    assert ar.posture.observed_posture is not None
    assert ar.posture.posture_match is True
