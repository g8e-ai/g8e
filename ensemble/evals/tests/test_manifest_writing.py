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
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.models.model_telemetry import ModelCallTelemetry
from g8e.operator.v1.operator_pb2 import ActionReceipt
from g8e_evals import cli
from g8e_evals.arms import Arm, GovernancePosture
from g8e_evals.auth_bridge import CLIAuthContext
from g8e_evals.evidence import EvidenceEncryptionKey, decrypt_evidence_artifact
from g8e_evals.harness import BindingType, LLMRoleConfig, Response, SUTConfig, Task
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.schema import AttemptRecord, EvidenceIndex, MetricObservation, RunManifest, StageObservation, TaskDefinition
from g8e_evals.sut.g8ee_chat import AgentTrailEvent, ChatEvaluationReceipt

pytestmark = pytest.mark.unit


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


def _patch_posture(monkeypatch, posture: GovernancePosture | None = GovernancePosture.L3_NOTARY) -> AsyncMock:
    """Patch the gateway posture observation path.

    Patches both ``observe_gateway_posture`` (the async call) and
    ``AuthContext.from_env`` (which runs before it to build the gateway
    transport context) so governed-arm tests don't require real mTLS cert
    files on disk.
    """
    mock = AsyncMock(return_value=posture)
    monkeypatch.setattr(cli, "observe_gateway_posture", mock)
    monkeypatch.setattr(cli.AuthContext, "from_env", MagicMock(return_value=MagicMock()))
    return mock


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
    _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)

    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )

    await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key())

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
async def test_governed_attempt_resolves_receipt_from_investigation_and_action_correlation(tmp_path, monkeypatch):
    task = Task(
        id="1001",
        prompt="Write a sentence without commas.",
        metadata=TaskMetadata(expected_action_class="FILE_EDIT"),
    )
    _patch_loader(monkeypatch, [task])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    _patch_sut(
        monkeypatch,
        settings=MagicMock(llm=MagicMock(primary_model="m")),
        answer_response=Response(
            answer="A valid answer.",
            model="test",
            chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
            binding=BindingType.UNBOUND,
            unbound_reason="answer-only turn",
        ),
    )
    collector = _patch_collector(monkeypatch)
    collector.collect_receipt_for_investigation.return_value = ActionReceipt(
        transaction_id="tx-correlated",
        transaction_hash="hash-correlated",
    )
    _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)
    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )

    await cli._run_suite(
        "ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key()
    )

    collector.collect_receipt_for_investigation.assert_awaited_once_with("inv-1", "FILE_EDIT")
    report_dir = next(path for path in tmp_path.iterdir() if path.is_dir())
    attempt = AttemptRecord.model_validate_json(
        (report_dir / "attempts.jsonl").read_text().splitlines()[0]
    )
    assert attempt.correlation_ids["transaction_id"] == "tx-correlated"


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
    _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)

    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )

    await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key())

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
    _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)

    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )

    await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key())

    report_dirs = [p for p in tmp_path.iterdir() if p.is_dir()]
    attempts_path = report_dirs[0] / "attempts.jsonl"
    assert attempts_path.exists()

    lines = attempts_path.read_text().splitlines()
    assert len(lines) == 1
    ar = AttemptRecord.model_validate_json(lines[0])
    assert ar.task_id == "1001"
    assert ar.arm_id == Arm.DOCTRINE
    assert ar.posture.requested_posture.value == "l1_doctrine"
    assert ar.usage_reconciliation is not None

    stages = [StageObservation.model_validate_json(line) for line in (report_dirs[0] / "stages.jsonl").read_text().splitlines()]
    metrics = [MetricObservation.model_validate_json(line) for line in (report_dirs[0] / "metrics.jsonl").read_text().splitlines()]
    evidence = [EvidenceIndex.model_validate_json(line) for line in (report_dirs[0] / "evidence-index.jsonl").read_text().splitlines()]
    assert stages == []
    assert metrics[0].metric_id == "stage_usage_reconciled"
    assert metrics[0].evidence_refs == [evidence[0].artifact_id]
    assert len(evidence) == 1
    assert evidence[0].encryption is not None
    assert evidence[0].access_control is not None
    artifact_path = report_dirs[0] / evidence[0].storage_location
    assert artifact_path.exists()
    encrypted_content = artifact_path.read_text()
    assert "agent_trail" not in encrypted_content
    assert "agent_trail" in decrypt_evidence_artifact(encrypted_content, evidence[0], _evidence_key())
    legacy_result = json.loads((report_dirs[0] / "results.jsonl").read_text())
    assert "prompt" not in legacy_result
    assert "answer" not in legacy_result
    assert "chat_evidence" not in legacy_result
    assert legacy_result["chat_evidence_ref"] == evidence[0].artifact_id
    assert legacy_result["chat_evidence_sha256"] == evidence[0].sha256


@pytest.mark.asyncio
async def test_run_suite_attaches_eval_judge_calls_to_attempt_reconciliation(tmp_path, monkeypatch):
    judge_call = ModelCallTelemetry(
        agent_role="judge",
        provider="OllamaProvider",
        model="judge-model",
        monotonic_start=10.0,
        monotonic_end=11.0,
        input_tokens=8,
        output_tokens=2,
        total_tokens=10,
        usage_reported=True,
        input_artifact_hash="judge-input",
        output_artifact_hash="judge-output",
    )
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch, model_calls=[judge_call])
    _patch_sut(
        monkeypatch,
        settings=MagicMock(llm=MagicMock(primary_model="m")),
        answer_response=Response(
            answer="A valid answer.",
            model="test",
            chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
            binding=BindingType.UNBOUND,
            unbound_reason="answer-only turn",
        ),
    )
    _patch_collector(monkeypatch)
    _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)
    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )

    await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key())

    report_dir = next(path for path in tmp_path.iterdir() if path.is_dir())
    stages = [StageObservation.model_validate_json(line) for line in (report_dir / "stages.jsonl").read_text().splitlines()]
    attempt = AttemptRecord.model_validate_json((report_dir / "attempts.jsonl").read_text())
    assert len(stages) == 1
    assert stages[0].kind.value == "grading"
    assert stages[0].agent_role == "judge"
    assert stages[0].input_artifact_hash == "judge-input"
    assert attempt.usage_reconciliation is not None
    assert attempt.usage_reconciliation.expected_call_count == 1
    assert attempt.usage_reconciliation.observed_call_count == 1
    assert attempt.usage_reconciliation.reconciled is True


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
        await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key())

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
    _patch_posture(monkeypatch, GovernancePosture.L2_CONSENSUS)

    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.CONSENSUS,
    )

    await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key())

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
    _patch_posture(monkeypatch, GovernancePosture.L3_NOTARY)

    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.NOTARY,
    )

    await cli._run_suite("ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key())

    report_dirs = [p for p in tmp_path.iterdir() if p.is_dir()]
    attempts_path = report_dirs[0] / "attempts.jsonl"
    lines = attempts_path.read_text().splitlines()
    ar = AttemptRecord.model_validate_json(lines[0])
    assert ar.posture.requested_posture.value == "l3_notary"
    assert ar.posture.observed_posture is not None
    assert ar.posture.observed_posture == GovernancePosture.L3_NOTARY
    assert ar.posture.observation_source == "gateway_health_endpoint"
    assert ar.posture.posture_match is True


@pytest.mark.asyncio
async def test_keyless_fake_provider_passes_preflight(tmp_path, monkeypatch):
    _patch_loader(monkeypatch, [_task()])
    _patch_provenance(monkeypatch)
    _patch_verifier(monkeypatch)
    sut = _patch_sut(
        monkeypatch,
        settings=SimpleNamespace(
            llm=SimpleNamespace(
                primary_model="fake-model",
                assistant_model=None,
                lite_model=None,
                primary_api_key=None,
                openai_api_key=None,
                anthropic_api_key=None,
                gemini_api_key=None,
            )
        ),
        answer_response=Response(
            answer="A valid answer.",
            model="fake-model",
            chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
            binding=BindingType.UNBOUND,
            unbound_reason="answer-only turn",
        ),
    )
    _patch_collector(monkeypatch)
    _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)

    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="fake", model="fake-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )

    await cli._run_suite(
        "ifeval_subset",
        config,
        None,
        tmp_path,
        limit=1,
        evidence_key=_evidence_key(),
    )

    sut.get_answer.assert_awaited_once()
