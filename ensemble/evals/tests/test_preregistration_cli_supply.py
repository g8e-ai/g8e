# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the preregistration config supply path in the CLI (P3-01).

Verifies that the CLI can load a preregistration configuration from a
JSON file, compute a stable content hash, pass the hash to preflight,
and pass the config through to the ``AnalysisInputRecord`` so that
paired comparisons and Holm correction run on the release matrix
output.
"""

from __future__ import annotations

import hashlib
import dataclasses
import json
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import pytest

from g8e_evals import cli
from g8e_evals.analysis.canonical import (
    PreregistrationConfig,
    canonical_model_json,
)
from g8e_evals.arms import Arm, GovernancePosture
from g8e_evals.auth_bridge import CLIAuthContext
from g8e_evals.benchmarks.ifeval.provenance import (
    DatasetOutput,
    DatasetProvenance,
    DatasetSource,
    DatasetTransformation,
)
from g8e_evals.evidence import EvidenceEncryptionKey
from g8e_evals.harness import BindingType, LLMRoleConfig, Response, SUTConfig
from g8e_evals.suites import SUITE_REGISTRY

pytestmark = pytest.mark.integration


# ---------------------------------------------------------------------------
# Shared fixtures
# ---------------------------------------------------------------------------


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


def _make_preregistration() -> PreregistrationConfig:
    return PreregistrationConfig(
        config_id="release-matrix-v2.1.8",
        config_version="1.0.0",
        baseline_arm_id=Arm.DOCTRINE.value,
        comparison_arm_ids=[Arm.ENSEMBLE_UNGOVERNED.value],
        model_cohort_ids=["ollama/qwen3:8b", "ollama/granite3.3:8b"],
        task_assignment_id="ifeval-subset-dev-v1",
        initial_state_assignment_id="no_initial_state",
        required_replicate_ids=["replicate-1", "replicate-2"],
        required_replicate_count=2,
        primary_metric_ids=["ifeval_subset_verifier"],
        secondary_families=[],
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=0,
        significance_level=0.05,
    )


def _patch_loader(monkeypatch, tasks) -> None:
    monkeypatch.setitem(SUITE_REGISTRY, "ifeval_subset", dataclasses.replace(SUITE_REGISTRY["ifeval_subset"], loader_factory=lambda _path: SimpleNamespace(load=lambda: iter(tasks))))


def _patch_provenance(monkeypatch) -> None:
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
    monkeypatch.setitem(SUITE_REGISTRY, "ifeval_subset", dataclasses.replace(SUITE_REGISTRY["ifeval_subset"], provenance_loader=lambda _path: provenance))


def _patch_source_build_provenance_env(monkeypatch) -> None:
    monkeypatch.setenv("G8E_EVALS_SOURCE_REVISION", "test-rev")
    monkeypatch.setenv("G8E_EVALS_SOURCE_TREE_STATE_HASH", "a" * 64)


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


def _patch_verifier(monkeypatch) -> MagicMock:
    from g8e_evals.harness import Score
    from g8e_evals.models import ScoreDetails

    verifier = MagicMock()
    verifier.grader_id = "ifeval_subset_verifier"
    verifier.grader_version = "1.0.0"
    score = Score(
        task_id="1001", passed=True, details=ScoreDetails(), model_calls=[],
    )
    verifier.verify.return_value = score
    verifier.grade.return_value = score
    monkeypatch.setitem(SUITE_REGISTRY, "ifeval_subset", dataclasses.replace(SUITE_REGISTRY["ifeval_subset"], grader_factory=lambda: verifier))
    return verifier


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


def _patch_posture(monkeypatch, posture: GovernancePosture | None = GovernancePosture.L1_DOCTRINE) -> AsyncMock:
    mock = AsyncMock(return_value=posture)
    monkeypatch.setattr(cli, "observe_gateway_posture", mock)
    monkeypatch.setattr(cli.AuthContext, "from_env", MagicMock(return_value=MagicMock()))
    return mock


def _patch_analysis(monkeypatch, prereg: PreregistrationConfig | None = None) -> None:
    """Mock ``compute_canonical_analysis_from_record`` to return a minimal valid analysis.

    The preregistration supply tests prove loading, hashing, preflight
    threading, and analysis-input persistence. They do not run a full
    multi-arm campaign, so the analysis engine's arm-completeness
    validation would reject a single-arm run against a two-arm
    preregistration. This patch returns a minimal canonical analysis
    carrying the preregistration so the test asserts threading, not
    campaign completion.
    """
    from g8e_evals.analysis.canonical import (
        AnalysisInputSummary,
        CanonicalEvalAnalysis,
        MissingnessBreakdown,
        ReceiptCoverageAnalysis,
    )

    def _fake_compute(_record):
        return CanonicalEvalAnalysis(
            analysis_schema_version="1.0.0",
            analysis_computation_version="1.0.0",
            release_version="v2.1.8",
            run_id="test-run",
            input_summary=AnalysisInputSummary(
                task_count=1, attempt_count=1, observation_count=0,
                receipt_count=0, stage_count=0, metric_observation_count=0,
                input_content_hash="0" * 64,
            ),
            missingness=MissingnessBreakdown(
                completed=1, model_failed=0, governance_rejected=0,
                human_denied=0, timed_out=0, infrastructure_failed=0, invalid_evidence=0,
            ),
            receipt_coverage=ReceiptCoverageAnalysis(
                eligible_attempt_count=0, receipt_bound_count=0, receipt_verified_count=0,
            ),
            arm_ids=[Arm.DOCTRINE.value],
            preregistration=prereg,
        )

    monkeypatch.setattr(cli, "compute_canonical_analysis_from_record", _fake_compute)


def _task():
    from g8e_evals.harness import Task
    return Task(id="1001", prompt="Write a sentence without commas.")


# ---------------------------------------------------------------------------
# load_preregistration
# ---------------------------------------------------------------------------


class TestLoadPreregistration:
    """Tests for ``cli.load_preregistration`` — loading a JSON file as a PreregistrationConfig."""

    def test_loads_valid_json_file(self, tmp_path: Path) -> None:
        config = _make_preregistration()
        path = tmp_path / "prereg.json"
        path.write_text(config.model_dump_json(indent=2))

        loaded = cli.load_preregistration(path)
        assert loaded == config

    def test_fails_on_missing_file(self, tmp_path: Path) -> None:
        path = tmp_path / "nonexistent.json"
        with pytest.raises(cli.EvaluationRunError, match="could not read"):
            cli.load_preregistration(path)

    def test_fails_on_invalid_json(self, tmp_path: Path) -> None:
        path = tmp_path / "prereg.json"
        path.write_text("{not valid json")
        with pytest.raises(cli.EvaluationRunError, match="invalid JSON"):
            cli.load_preregistration(path)

    def test_fails_on_invalid_schema(self, tmp_path: Path) -> None:
        path = tmp_path / "prereg.json"
        path.write_text(json.dumps({"config_id": "x"}))
        with pytest.raises(cli.EvaluationRunError, match="invalid schema"):
            cli.load_preregistration(path)


# ---------------------------------------------------------------------------
# compute_preregistration_hash
# ---------------------------------------------------------------------------


class TestComputePreregistrationHash:
    """Tests for ``cli.compute_preregistration_hash`` — stable content hash."""

    def test_returns_64_char_hex(self) -> None:
        config = _make_preregistration()
        h = cli.compute_preregistration_hash(config)
        assert len(h) == 64
        assert all(c in "0123456789abcdef" for c in h)

    def test_deterministic(self) -> None:
        config = _make_preregistration()
        assert cli.compute_preregistration_hash(config) == cli.compute_preregistration_hash(config)

    def test_matches_sha256_of_canonical_json(self) -> None:
        config = _make_preregistration()
        expected = hashlib.sha256(canonical_model_json(config).encode()).hexdigest()
        assert cli.compute_preregistration_hash(config) == expected

    def test_changes_when_config_changes(self) -> None:
        config_a = _make_preregistration()
        config_b = config_a.model_copy(update={"config_id": "release-matrix-v2.1.8-b"})
        assert cli.compute_preregistration_hash(config_a) != cli.compute_preregistration_hash(config_b)


# ---------------------------------------------------------------------------
# _run_suite preregistration supply
# ---------------------------------------------------------------------------


class TestRunSuitePreregistrationSupply:
    """Tests that ``_run_suite`` threads preregistration into the analysis input and preflight."""

    @pytest.mark.asyncio
    async def test_preregistration_written_to_analysis_input(self, tmp_path: Path, monkeypatch) -> None:
        _patch_loader(monkeypatch, [_task()])
        _patch_provenance(monkeypatch)
        _patch_source_build_provenance_env(monkeypatch)
        _patch_verifier(monkeypatch)
        _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")))
        _patch_collector(monkeypatch)
        _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)
        prereg = _make_preregistration()
        _patch_analysis(monkeypatch, prereg)

        config = SUTConfig(
            g8ee_url="http://g8ee:8000",
            primary=LLMRoleConfig(provider="ollama", model="test-model"),
            operator_url="https://gateway:8443",
            operator_session_id="op-session",
            auth_context=_auth_context(),
            arm=Arm.DOCTRINE,
        )

        await cli._run_suite(
            "ifeval_subset", config, None, tmp_path, limit=1,
            evidence_key=_evidence_key(), preregistration=prereg,
        )

        report_dirs = [p for p in tmp_path.iterdir() if p.is_dir()]
        assert len(report_dirs) == 1
        analysis_input_path = report_dirs[0] / "analysis-input.json"
        assert analysis_input_path.exists()
        analysis_input = json.loads(analysis_input_path.read_text())
        assert analysis_input["preregistration"] is not None
        assert analysis_input["preregistration"]["config_id"] == prereg.config_id

    @pytest.mark.asyncio
    async def test_preregistration_appears_in_analysis(self, tmp_path: Path, monkeypatch) -> None:
        _patch_loader(monkeypatch, [_task()])
        _patch_provenance(monkeypatch)
        _patch_source_build_provenance_env(monkeypatch)
        _patch_verifier(monkeypatch)
        _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")))
        _patch_collector(monkeypatch)
        _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)
        prereg = _make_preregistration()
        _patch_analysis(monkeypatch, prereg)

        config = SUTConfig(
            g8ee_url="http://g8ee:8000",
            primary=LLMRoleConfig(provider="ollama", model="test-model"),
            operator_url="https://gateway:8443",
            operator_session_id="op-session",
            auth_context=_auth_context(),
            arm=Arm.DOCTRINE,
        )

        await cli._run_suite(
            "ifeval_subset", config, None, tmp_path, limit=1,
            evidence_key=_evidence_key(), preregistration=prereg,
        )

        report_dirs = [p for p in tmp_path.iterdir() if p.is_dir()]
        assert len(report_dirs) == 1
        analysis_path = report_dirs[0] / "analysis.json"
        assert analysis_path.exists()
        analysis = json.loads(analysis_path.read_text())
        assert analysis["preregistration"] is not None
        assert analysis["preregistration"]["config_id"] == prereg.config_id

    @pytest.mark.asyncio
    async def test_no_preregistration_leaves_field_null(self, tmp_path: Path, monkeypatch) -> None:
        _patch_loader(monkeypatch, [_task()])
        _patch_provenance(monkeypatch)
        _patch_source_build_provenance_env(monkeypatch)
        _patch_verifier(monkeypatch)
        _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")))
        _patch_collector(monkeypatch)
        _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)
        _patch_analysis(monkeypatch, prereg=None)

        config = SUTConfig(
            g8ee_url="http://g8ee:8000",
            primary=LLMRoleConfig(provider="ollama", model="test-model"),
            operator_url="https://gateway:8443",
            operator_session_id="op-session",
            auth_context=_auth_context(),
            arm=Arm.DOCTRINE,
        )

        await cli._run_suite(
            "ifeval_subset", config, None, tmp_path, limit=1,
            evidence_key=_evidence_key(),
        )

        report_dirs = [p for p in tmp_path.iterdir() if p.is_dir()]
        assert len(report_dirs) == 1
        analysis_input_path = report_dirs[0] / "analysis-input.json"
        analysis_input = json.loads(analysis_input_path.read_text())
        assert analysis_input["preregistration"] is None

    @pytest.mark.asyncio
    async def test_preregistration_hash_passed_to_preflight(self, tmp_path: Path, monkeypatch) -> None:
        _patch_loader(monkeypatch, [_task()])
        _patch_provenance(monkeypatch)
        _patch_source_build_provenance_env(monkeypatch)
        _patch_verifier(monkeypatch)
        _patch_sut(monkeypatch, settings=MagicMock(llm=MagicMock(primary_model="m")))
        _patch_collector(monkeypatch)
        _patch_posture(monkeypatch, GovernancePosture.L1_DOCTRINE)
        prereg = _make_preregistration()
        _patch_analysis(monkeypatch, prereg)

        captured_requests: list = []

        original_preflight = cli._run_preflight

        def _capture_preflight(request):
            captured_requests.append(request)
            original_preflight(request)

        monkeypatch.setattr(cli, "_run_preflight", _capture_preflight)

        config = SUTConfig(
            g8ee_url="http://g8ee:8000",
            primary=LLMRoleConfig(provider="ollama", model="test-model"),
            operator_url="https://gateway:8443",
            operator_session_id="op-session",
            auth_context=_auth_context(),
            arm=Arm.DOCTRINE,
        )
        expected_hash = cli.compute_preregistration_hash(prereg)

        await cli._run_suite(
            "ifeval_subset", config, None, tmp_path, limit=1,
            evidence_key=_evidence_key(), preregistration=prereg,
        )

        assert len(captured_requests) == 1
        assert captured_requests[0].preregistration_hash == expected_hash
