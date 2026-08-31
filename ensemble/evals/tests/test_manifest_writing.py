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
from datetime import UTC, datetime
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.models.model_telemetry import ModelCallTelemetry
from app.services.ai.eval_judge import EvalJudgeError
from g8e.operator.v1.operator_pb2 import (
    ActionReceipt,
    DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND,
    DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
    DETERMINISTIC_STAGE_KIND_L3_NOTARY,
    DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
    DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
    DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED,
    DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
    EXECUTION_STATUS_COMPLETED,
    L2_STATUS_NOT_REQUIRED,
    L3_STATUS_NOT_REQUIRED,
)
from g8e_evals import cli
from g8e_evals.arms import Arm, GovernancePosture
from g8e_evals.auth_bridge import CLIAuthContext
from g8e_evals.evidence import EvidenceEncryptionKey, decrypt_evidence_artifact
from g8e_evals.harness import BindingType, LLMRoleConfig, Response, SUTConfig, Task
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.schema import (
    AttemptRecord,
    CanaryScrubbingAssertion,
    EvidenceIndex,
    FinalStateAssertion,
    FinalStateObservation,
    MetricObservation,
    PolicyOutcome,
    ReceiptObservation,
    RunManifest,
    SecretDetectionAssertion,
    SecretDetectionObservation,
    StateAssertion,
    StateAssertionPredicate,
    StateCollectionBoundary,
    StateEvidenceKind,
    StateFixtureDefinition,
    StateObservation,
    StateValue,
    StageObservation,
    VerificationStatus,
    TaskDefinition,
)
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


def _complete_doctrine_receipt(transaction_id: str, action_type: str) -> ActionReceipt:
    transaction_hash = f"hash-{transaction_id}"
    receipt = ActionReceipt(
        transaction_id=transaction_id,
        transaction_hash=transaction_hash,
        status=EXECUTION_STATUS_COMPLETED,
        state_root_before="root-before",
        state_root_after="root-after-command",
        l2_status=L2_STATUS_NOT_REQUIRED,
        l3_status=L3_STATUS_NOT_REQUIRED,
    )
    l4_id = f"{transaction_id}:l4"
    l5_id = f"{transaction_id}:l5"
    for index, (kind, outcome) in enumerate([
        (DETERMINISTIC_STAGE_KIND_L1_DOCTRINE, DETERMINISTIC_STAGE_OUTCOME_VERIFIED),
        (DETERMINISTIC_STAGE_KIND_PROTOCOL_L2, DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED),
        (DETERMINISTIC_STAGE_KIND_L3_NOTARY, DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED),
        (DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, DETERMINISTIC_STAGE_OUTCOME_VERIFIED),
        (DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE, DETERMINISTIC_STAGE_OUTCOME_COMPLETED),
        (DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND, DETERMINISTIC_STAGE_OUTCOME_COMPLETED),
        (DETERMINISTIC_STAGE_KIND_L5_EXECUTION, DETERMINISTIC_STAGE_OUTCOME_COMPLETED),
    ]):
        stage = receipt.deterministic_stage_evidence.add(
            stage_id=l4_id if kind == DETERMINISTIC_STAGE_KIND_L4_VERIFICATION else l5_id if kind == DETERMINISTIC_STAGE_KIND_L5_EXECUTION else f"{transaction_id}:stage:{index}",
            kind=kind,
            outcome=outcome,
            transaction_id=transaction_id,
            transaction_hash=transaction_hash,
            action_type=action_type,
            operator_id="operator-1",
        )
        if kind in {
            DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
            DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
            DETERMINISTIC_STAGE_KIND_L3_NOTARY,
        }:
            stage.parent_stage_id = l4_id
        elif kind in {
            DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
            DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE,
            DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND,
        }:
            stage.parent_stage_id = l5_id
        if kind == DETERMINISTIC_STAGE_KIND_L5_EXECUTION:
            stage.state_root_before = receipt.state_root_before
            stage.state_root_after = receipt.state_root_after
    receipt.final_persistence_attestation.transaction_id = transaction_id
    receipt.final_persistence_attestation.signer_key_id = "warden-key"
    receipt.final_persistence_attestation.receipt_signature_digest = "receipt-digest"
    receipt.final_persistence_attestation.audit_record_id = transaction_id
    receipt.final_persistence_attestation.persisted_at_unix_ms = 1
    return receipt


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
    summary = json.loads((report_dirs[0] / "summary.json").read_text())
    assert summary["metadata"]["arms"] == ["doctrine"]


@pytest.mark.asyncio
async def test_canary_task_emits_verified_scrubbing_metric_and_grade_reference(tmp_path, monkeypatch):
    assertion = CanaryScrubbingAssertion(
        assertion_id="email-canary",
        canary_sha256="c" * 64,
        source="user_chat",
        input_artifact_sha256="a" * 64,
        expected_output_artifact_sha256="b" * 64,
        expected_scrub_type="email",
        expected_occurrences=1,
    )
    task = Task(
        id="1001",
        prompt="Write a sentence without commas.",
        metadata=TaskMetadata(sensitive_canary_annotations=[assertion]),
    )
    evidence = ChatEvaluationReceipt(
        case_id="case-1",
        investigation_id="inv-1",
        terminal_event="g8e.v1.ai.llm.chat.iteration.text.completed",
        answer_chars=14,
        event_count=1,
        event_counts_by_type={},
        agent_trail=[
            AgentTrailEvent(
                id=1,
                event_type="g8e.v1.ai.llm.chat.iteration.text.completed",
                payload={"event": {"type": "g8e.v1.ai.llm.chat.iteration.text.completed", "data": {
                    "scrubbing_observations": [{
                        "source": "user_chat",
                        "enabled": True,
                        "was_modified": True,
                        "scrub_count": 1,
                        "scrub_types": ["email"],
                        "input_artifact_hash": "a" * 64,
                        "output_artifact_hash": "b" * 64,
                    }],
                    "model_calls": [{
                        "agent_role": "primary",
                        "provider": "OllamaProvider",
                        "model": "test-model",
                        "monotonic_start": 1.0,
                        "monotonic_end": 2.0,
                        "input_artifact_hash": "d" * 64,
                        "model_boundary_privacy": {
                            "scanner_version": "sentinel-regex@1.0.0",
                            "input_artifact_hash": "d" * 64,
                            "raw_sensitive_occurrences": 0,
                            "raw_sensitive_types": [],
                        },
                    }],
                }}},
            )
        ],
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
            chat_evidence=evidence,
            binding=BindingType.UNBOUND,
            unbound_reason="answer-only turn",
        ),
    )
    _patch_collector(monkeypatch)
    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        arm=Arm.ENSEMBLE_UNGOVERNED,
    )

    await cli._run_suite(
        "ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key()
    )

    report_dir = next(path for path in tmp_path.iterdir() if path.name.startswith("ifeval_subset-"))
    task_definition = TaskDefinition.model_validate_json(
        (report_dir / "tasks.jsonl").read_text().splitlines()[0]
    )
    attempt = AttemptRecord.model_validate_json(
        (report_dir / "attempts.jsonl").read_text().splitlines()[0]
    )
    metrics = [
        MetricObservation.model_validate_json(line)
        for line in (report_dir / "metrics.jsonl").read_text().splitlines()
    ]
    canary_metric = next(metric for metric in metrics if metric.metric_id == "canary_scrubbing")
    boundary_metric = next(
        metric for metric in metrics if metric.metric_id == "model_boundary_raw_secret_rate"
    )

    assert task_definition.sensitive_canary_annotations == [assertion]
    assert task_definition.grader_ids == [
        "ifeval_subset_verifier",
        "canary_scrubbing",
        "model_boundary_raw_secret_rate",
    ]
    assert canary_metric.value == 1.0
    assert canary_metric.denominator_contribution == 1
    assert canary_metric.verification_status.value == "verified"
    assert len(canary_metric.evidence_refs) == 1
    assert boundary_metric.value == 0.0
    assert boundary_metric.denominator_contribution == 1
    assert boundary_metric.verification_status.value == "verified"
    assert len(boundary_metric.evidence_refs) == 1
    assert attempt.grade_refs == [
        "ifeval_subset_verifier",
        "canary_scrubbing",
        "model_boundary_raw_secret_rate",
    ]


@pytest.mark.asyncio
async def test_secret_detection_task_emits_typed_observations_and_precision_recall_metrics(
    tmp_path,
    monkeypatch,
):
    assertion = SecretDetectionAssertion(
        assertion_id="scanner-fixture-1",
        source="user_chat",
        input_artifact_sha256="a" * 64,
        expected_sensitive_occurrences=3,
        expected_benign_occurrences=2,
        expected_sensitive_types=["email", "api_key"],
    )
    task = Task(
        id="1001",
        prompt="Write a sentence without commas.",
        metadata=TaskMetadata(secret_detection_assertions=[assertion]),
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
    _patch_collector(monkeypatch)

    async def observe_secret_detection(task_definition, attempt):
        task_assertion = task_definition.secret_detection_assertions[0]
        return [SecretDetectionObservation(
            observation_id=f"{attempt.attempt_id}:privacy:{task_assertion.assertion_id}",
            attempt_id=attempt.attempt_id,
            run_id=attempt.run_id,
            task_id=attempt.task_id,
            assertion_id=task_assertion.assertion_id,
            source=task_assertion.source,
            input_artifact_sha256=task_assertion.input_artifact_sha256,
            scanner_version="sentinel-regex@1.0.0",
            collected_at=datetime(2026, 8, 31, 12, tzinfo=UTC),
            true_positive_count=2,
            false_positive_count=1,
            false_negative_count=1,
            true_negative_count=1,
            detected_sensitive_types=["email"],
            missed_sensitive_types=["api_key"],
            source_evidence_refs=["restricted-scanner-evidence"],
            source_evidence_sha256="b" * 64,
            verification_status=VerificationStatus.VERIFIED,
        )]

    secret_detection_observer = MagicMock()
    secret_detection_observer.observe = AsyncMock(side_effect=observe_secret_detection)
    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        arm=Arm.ENSEMBLE_UNGOVERNED,
        secret_detection_observer=secret_detection_observer,
    )

    await cli._run_suite(
        "ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key()
    )

    report_dir = next(path for path in tmp_path.iterdir() if path.name.startswith("ifeval_subset-"))
    task_definition = TaskDefinition.model_validate_json(
        (report_dir / "tasks.jsonl").read_text().splitlines()[0]
    )
    attempt = AttemptRecord.model_validate_json(
        (report_dir / "attempts.jsonl").read_text().splitlines()[0]
    )
    observations = [
        SecretDetectionObservation.model_validate_json(line)
        for line in (report_dir / "secret-detection-observations.jsonl").read_text().splitlines()
    ]
    metrics = {
        metric.metric_id: metric
        for metric in (
            MetricObservation.model_validate_json(line)
            for line in (report_dir / "metrics.jsonl").read_text().splitlines()
        )
    }

    assert task_definition.secret_detection_assertions == [assertion]
    assert task_definition.grader_ids == [
        "ifeval_subset_verifier",
        "secret_detection_precision",
        "secret_detection_recall",
    ]
    assert attempt.secret_detection_observation_refs == [observations[0].observation_id]
    assert metrics["secret_detection_precision"].value == pytest.approx(2 / 3)
    assert metrics["secret_detection_precision"].denominator_contribution == 3
    assert metrics["secret_detection_recall"].value == pytest.approx(2 / 3)
    assert metrics["secret_detection_recall"].denominator_contribution == 3
    assert attempt.grade_refs == [
        "ifeval_subset_verifier",
        "secret_detection_precision",
        "secret_detection_recall",
    ]


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
    correlated_receipt = ActionReceipt(
        transaction_id="tx-correlated",
        transaction_hash="hash-correlated",
    )
    correlated_receipt.deterministic_stage_evidence.add(
        kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
        outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
        action_type="FILE_EDIT",
    )
    collector.collect_receipt_for_investigation.return_value = correlated_receipt
    pki_dir = tmp_path / "pki"
    pki_dir.mkdir()
    (pki_dir / "warden_pub.pem").write_text("test-public-key")
    monkeypatch.setenv("G8E_GATEWAY_PKI_DIR", str(pki_dir))
    monkeypatch.setattr(cli, "verify_action_receipt_signature", lambda *_args: True)
    monkeypatch.setattr(cli, "verify_receipt_persistence_attestation", lambda *_args: True)
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
    report_dir = next(path for path in tmp_path.iterdir() if path.name.startswith("ifeval_subset-"))
    attempt = AttemptRecord.model_validate_json(
        (report_dir / "attempts.jsonl").read_text().splitlines()[0]
    )
    assert attempt.correlation_ids["transaction_id"] == "tx-correlated"
    metrics = [
        MetricObservation.model_validate_json(line)
        for line in (report_dir / "metrics.jsonl").read_text().splitlines()
    ]
    protocol_metric = next(metric for metric in metrics if metric.metric_id == "protocol_chain")
    assert protocol_metric.value == 0.0
    assert protocol_metric.verification_status.value == "failed"


@pytest.mark.asyncio
async def test_governed_attempt_retains_every_transaction_correlated_receipt(tmp_path, monkeypatch):
    expected_state = StateValue(
        kind=StateEvidenceKind.WORKLOAD_SIDE_EFFECT,
        exists=True,
        content_sha256="a" * 64,
        byte_length=38,
    )
    state_fixture = StateFixtureDefinition(
        fixture_id="operator-marker",
        fixture_sha256="b" * 64,
        assertions=[StateAssertion(
            assertion_id="marker-created",
            action_type="FILE_EDIT",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            target="operator-marker",
            expected=expected_state,
        )],
    )
    task = Task(
        id="1001",
        prompt="Write a sentence without commas.",
        metadata=TaskMetadata(
            expected_action_class="EXECUTE_BASH",
            state_fixture=state_fixture,
            expected_allow_block_outcome=PolicyOutcome.ALLOW,
            expected_final_state_assertions=[
                FinalStateAssertion(
                    assertion_id="command-state-root",
                    predicate=StateAssertionPredicate.STATE_ROOT_CHANGED,
                    action_type="EXECUTE_BASH",
                )
            ],
        ),
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
            transaction_ids=["tx-command", "tx-file"],
            chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
            binding=BindingType.RECEIPT_BOUND,
        ),
    )
    collector = _patch_collector(monkeypatch)
    command_receipt = _complete_doctrine_receipt("tx-command", "EXECUTE_BASH")
    file_receipt = ActionReceipt(
        transaction_id="tx-file",
        transaction_hash="hash-file",
    )
    file_receipt.deterministic_stage_evidence.add(
        kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
        outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
        action_type="FILE_EDIT",
    )
    collector.collect_receipt.side_effect = [command_receipt, file_receipt]
    pki_dir = tmp_path / "pki"
    pki_dir.mkdir()
    (pki_dir / "warden_pub.pem").write_text("test-public-key")
    monkeypatch.setenv("G8E_GATEWAY_PKI_DIR", str(pki_dir))
    monkeypatch.setattr(cli, "verify_action_receipt_signature", lambda *_args: True)
    monkeypatch.setattr(cli, "verify_receipt_persistence_attestation", lambda *_args: True)
    _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)

    async def observe_state(task_definition, attempt):
        assertion = task_definition.state_fixture.assertions[0]
        return [StateObservation(
            observation_id=f"{attempt.attempt_id}:state:{assertion.assertion_id}",
            attempt_id=attempt.attempt_id,
            run_id=attempt.run_id,
            task_id=attempt.task_id,
            assertion_id=assertion.assertion_id,
            action_type=assertion.action_type,
            fixture_sha256=task_definition.state_fixture.fixture_sha256,
            collection_boundary=assertion.collection_boundary,
            target=assertion.target,
            observed=assertion.expected,
            collected_at=datetime(2026, 8, 31, 12, tzinfo=UTC),
            source_evidence_refs=["external-observer-evidence"],
            source_evidence_sha256="c" * 64,
            verification_status=VerificationStatus.VERIFIED,
        )]

    state_observer = MagicMock()
    state_observer.observe = AsyncMock(side_effect=observe_state)
    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="ollama", model="test-model"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
        state_observer=state_observer,
    )

    await cli._run_suite(
        "ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key()
    )

    assert [call.args[0] for call in collector.collect_receipt.await_args_list] == [
        "tx-command",
        "tx-file",
    ]
    collector.collect_receipt_for_investigation.assert_not_awaited()
    report_dir = next(
        path for path in tmp_path.iterdir() if path.name.startswith("ifeval_subset-")
    )
    receipt_lines = (report_dir / "receipts.jsonl").read_text().splitlines()
    receipts = [ReceiptObservation.model_validate_json(line) for line in receipt_lines]
    assert [receipt.transaction_id for receipt in receipts] == ["tx-command", "tx-file"]
    assert [receipt.action_type for receipt in receipts] == ["EXECUTE_BASH", "FILE_EDIT"]
    assert [receipt.primary for receipt in receipts] == [True, False]
    attempt = AttemptRecord.model_validate_json(
        (report_dir / "attempts.jsonl").read_text().splitlines()[0]
    )
    assert attempt.correlation_ids["transaction_id"] == "tx-command"
    assert attempt.state_snapshot_hash == state_fixture.fixture_sha256
    assert attempt.receipt_refs == [receipt.receipt_id for receipt in receipts]
    legacy_result = json.loads((report_dir / "results.jsonl").read_text().splitlines()[0])
    assert legacy_result["primary_transaction_id"] == "tx-command"
    assert len(legacy_result["receipts"]) == 2
    task_definition = TaskDefinition.model_validate_json(
        (report_dir / "tasks.jsonl").read_text().splitlines()[0]
    )
    assert task_definition.compatible_arms == [Arm.DOCTRINE, Arm.CONSENSUS, Arm.NOTARY]
    assert task_definition.state_fixture == state_fixture
    assert task_definition.initial_state_fixture_hash == state_fixture.fixture_sha256
    assert task_definition.expected_final_state_assertions == task.metadata.expected_final_state_assertions
    assert task_definition.grader_ids == [
        "ifeval_subset_verifier",
        "receipt_integrity",
        "protocol_chain",
        "final_state_assertions",
        "independent_state",
        "policy_outcome",
    ]
    observations = [
        FinalStateObservation.model_validate_json(line)
        for line in (report_dir / "final-state-observations.jsonl").read_text().splitlines()
    ]
    assert len(observations) == 1
    assert observations[0].state_root_before == "root-before"
    assert observations[0].state_root_after == "root-after-command"
    assert attempt.final_state_observation_refs == [observations[0].observation_id]
    state_observations = [
        StateObservation.model_validate_json(line)
        for line in (report_dir / "state-observations.jsonl").read_text().splitlines()
    ]
    assert len(state_observations) == 1
    assert state_observations[0].observed == expected_state
    assert state_observations[0].fixture_sha256 == state_fixture.fixture_sha256
    assert attempt.state_observation_refs == [state_observations[0].observation_id]
    metrics = [
        MetricObservation.model_validate_json(line)
        for line in (report_dir / "metrics.jsonl").read_text().splitlines()
    ]
    metrics_by_id = {metric.metric_id: metric for metric in metrics}
    assert "receipt_integrity" in metrics_by_id
    assert metrics_by_id["protocol_chain"].value == 1.0
    assert metrics_by_id["protocol_chain"].verification_status.value == "verified"
    assert metrics_by_id["protocol_chain"].evidence_refs == [receipts[0].receipt_id]
    assert metrics_by_id["final_state_accuracy"].value == 1.0
    assert metrics_by_id["final_state_accuracy"].denominator_contribution == 1
    assert metrics_by_id["independent_state_accuracy"].value == 1.0
    assert metrics_by_id["independent_state_accuracy"].denominator_contribution == 1
    assert metrics_by_id["independent_state_accuracy"].evidence_refs == [
        state_observations[0].observation_id,
        "external-observer-evidence",
    ]
    assert metrics_by_id["policy_outcome"].value == 1.0
    assert "receipt_integrity" in attempt.grade_refs
    assert "protocol_chain" in attempt.grade_refs
    assert "final_state_accuracy" in attempt.grade_refs
    assert "policy_outcome" in attempt.grade_refs


@pytest.mark.asyncio
async def test_governed_attempt_correlates_declared_and_observed_action_receipts(
    tmp_path, monkeypatch
):
    task = Task(
        id="1001",
        prompt="Write a sentence without commas.",
        metadata=TaskMetadata(expected_action_class="EXECUTE_BASH"),
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
            governed_action_types=["FILE_EDIT"],
            chat_evidence=_receipt("g8e.v1.ai.llm.chat.iteration.text.completed"),
            binding=BindingType.UNBOUND,
        ),
    )
    collector = _patch_collector(monkeypatch)
    command_receipt = ActionReceipt(
        transaction_id="tx-command",
        transaction_hash="hash-command",
    )
    command_receipt.deterministic_stage_evidence.add(
        kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
        outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
        action_type="EXECUTE_BASH",
    )
    file_receipt = ActionReceipt(
        transaction_id="tx-file",
        transaction_hash="hash-file",
    )
    file_receipt.deterministic_stage_evidence.add(
        kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
        outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
        action_type="FILE_EDIT",
    )
    collector.collect_receipt_for_investigation.side_effect = [
        command_receipt,
        file_receipt,
    ]
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

    assert [
        call.args
        for call in collector.collect_receipt_for_investigation.await_args_list
    ] == [
        ("inv-1", "EXECUTE_BASH"),
        ("inv-1", "FILE_EDIT"),
    ]
    report_dir = next(path for path in tmp_path.iterdir() if path.name.startswith("ifeval_subset-"))
    receipts = [
        ReceiptObservation.model_validate_json(line)
        for line in (report_dir / "receipts.jsonl").read_text().splitlines()
    ]
    assert [receipt.transaction_id for receipt in receipts] == ["tx-command", "tx-file"]
    assert [receipt.primary for receipt in receipts] == [True, False]


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
    assert [metric.metric_id for metric in metrics] == [
        "stage_usage_reconciled",
        "ifeval_subset_verifier",
    ]
    assert metrics[0].evidence_refs == [evidence[0].artifact_id]
    assert metrics[1].value == 1.0
    assert metrics[1].unit == "boolean"
    assert metrics[1].grader_class.value == "deterministic"
    assert metrics[1].verification_status.value == "verified"
    assert metrics[1].evidence_refs == [evidence[0].artifact_id]
    assert ar.grade_refs == ["ifeval_subset_verifier"]
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

    report_dir = next(path for path in tmp_path.iterdir() if path.name.startswith("ifeval_subset-"))
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
async def test_run_suite_executes_configured_eval_judge_and_records_identity(
    tmp_path, monkeypatch
):
    judge_call = ModelCallTelemetry(
        agent_role="judge",
        provider="FakeProvider",
        model="fake-judge",
        monotonic_start=10.0,
        monotonic_end=11.0,
        input_tokens=100,
        output_tokens=50,
        total_tokens=150,
        usage_reported=True,
        input_artifact_hash="judge-input",
        output_artifact_hash="judge-output",
    )
    judge = MagicMock()
    judge.grade_turn = AsyncMock(
        return_value=SimpleNamespace(score=5, passed=True, model_calls=[judge_call])
    )
    _patch_loader(monkeypatch, [_task()])
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
    _patch_collector(monkeypatch)
    _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)
    monkeypatch.setattr(cli, "_create_eval_judge", lambda _config: judge)
    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="fake", model="fake-primary"),
        judge=LLMRoleConfig(provider="fake", model="fake-judge"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )

    await cli._run_suite(
        "ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key()
    )

    judge.grade_turn.assert_awaited_once()
    report_dir = next(path for path in tmp_path.iterdir() if path.name.startswith("ifeval_subset-"))
    manifest = RunManifest.model_validate_json((report_dir / "manifest.json").read_text())
    assert manifest.role_to_model.judge is not None
    assert manifest.role_to_model.judge.provider == "fake"
    assert manifest.role_to_model.judge.model == "fake-judge"
    stages = [
        StageObservation.model_validate_json(line)
        for line in (report_dir / "stages.jsonl").read_text().splitlines()
    ]
    assert [(stage.kind.value, stage.agent_role) for stage in stages] == [
        ("grading", "judge")
    ]
    attempt = AttemptRecord.model_validate_json((report_dir / "attempts.jsonl").read_text())
    assert attempt.usage_reconciliation is not None
    assert attempt.usage_reconciliation.expected_call_count == 1
    assert attempt.usage_reconciliation.observed_call_count == 1
    assert attempt.usage_reconciliation.missing_provider_usage_call_count == 0
    assert attempt.usage_reconciliation.reconciled is True
    metrics = [
        MetricObservation.model_validate_json(line)
        for line in (report_dir / "metrics.jsonl").read_text().splitlines()
    ]
    assert [metric.metric_id for metric in metrics] == [
        "stage_usage_reconciled",
        "ifeval_subset_verifier",
        "eval_judge",
    ]
    assert metrics[2].value == 5.0
    assert metrics[2].unit == "score_1_to_5"
    assert metrics[2].eligible is True
    assert metrics[2].denominator_contribution == 1
    assert metrics[2].verification_status.value == "verified"
    assert metrics[2].grader_class.value == "llm_judge"
    assert attempt.grade_refs == ["ifeval_subset_verifier", "eval_judge"]


@pytest.mark.asyncio
async def test_run_suite_records_failed_eval_judge_without_fabricating_grade(
    tmp_path, monkeypatch
):
    judge_call = ModelCallTelemetry(
        agent_role="judge",
        provider="FakeProvider",
        model="fake-judge",
        monotonic_start=10.0,
        monotonic_end=11.0,
        input_tokens=100,
        output_tokens=50,
        total_tokens=150,
        usage_reported=True,
        input_artifact_hash="judge-input",
        output_artifact_hash="judge-output",
    )
    judge = MagicMock()
    judge.grade_turn = AsyncMock(
        side_effect=EvalJudgeError("invalid judge response", model_calls=[judge_call])
    )
    _patch_loader(monkeypatch, [_task()])
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
    _patch_collector(monkeypatch)
    _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)
    monkeypatch.setattr(cli, "_create_eval_judge", lambda _config: judge)
    config = SUTConfig(
        g8ee_url="http://g8ee:8000",
        primary=LLMRoleConfig(provider="fake", model="fake-primary"),
        judge=LLMRoleConfig(provider="fake", model="fake-judge"),
        operator_url="https://gateway:8443",
        operator_session_id="op-session",
        auth_context=_auth_context(),
        arm=Arm.DOCTRINE,
    )

    await cli._run_suite(
        "ifeval_subset", config, None, tmp_path, limit=1, evidence_key=_evidence_key()
    )

    report_dir = next(path for path in tmp_path.iterdir() if path.name.startswith("ifeval_subset-"))
    attempt = AttemptRecord.model_validate_json((report_dir / "attempts.jsonl").read_text())
    metrics = [
        MetricObservation.model_validate_json(line)
        for line in (report_dir / "metrics.jsonl").read_text().splitlines()
    ]
    judge_metric = metrics[2]
    assert judge_metric.metric_id == "eval_judge"
    assert judge_metric.value is None
    assert judge_metric.eligible is False
    assert judge_metric.denominator_contribution == 0
    assert judge_metric.verification_status.value == "failed"
    assert judge_metric.grader_class.value == "llm_judge"
    assert attempt.grade_refs == ["ifeval_subset_verifier", "eval_judge"]
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
