# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 and Tier 2 tests for the EF3 tool calling scorecard.

Verifies that:
- ``ToolCallScorecard`` is frozen with ``extra="forbid"`` and carries
  all 10 scorecard dimensions bound to agent persona, model variant,
  tool name, and task.
- ``ToolCallScorecardDimension`` enumerates exactly the 10 dimensions.
- ``validate_tool_call_scorecards`` rejects duplicate (run, attempt,
  tool, call_index) tuples.
- Verified scorecards require source evidence.
- The campaign verifier has a ``tool_call_scorecard`` layer that
  validates scorecards when present, passes when absent, rejects
  malformed scorecards, rejects duplicates, and rejects scorecards
  bound to unknown attempts.
- The 10 tool scorecard metrics are registered in the metric registry
  with correct grader class, direction, and release domain.
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with extra fields and
# invalid evidence to verify pydantic validation rejects them.

from __future__ import annotations

import asyncio
import hashlib
import json
from dataclasses import dataclass
from pathlib import Path

import pytest
from pydantic import ValidationError

from g8e_evals.analysis.canonical import (
    ClaimPolicy,
    ContinuousTestPolicy,
    PreregistrationConfig,
)
from g8e_evals.arms import Arm
from g8e_evals.campaign import (
    InitialStateAssignmentManifest,
    ModelCohort,
    RetryPolicy,
    RoleModelBinding,
    SamplingSettings,
    TaskAssignmentManifest,
    compute_initial_state_hash,
    compute_model_cohort_hash,
    compute_task_assignment_hash,
)
from g8e_evals.campaign_verify import verify_campaign
from g8e_evals.constants import TOOL_CALL_SCORECARDS_JSONL
from g8e_evals.harness import Response, Score, Task
from g8e_evals.metrics import (
    DEFAULT_METRIC_REGISTRY,
    GraderClass,
    MetricDirection,
)
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.release_metric_set import MetricDomain, RELEASE_METRIC_SET
from g8e_evals.runner import CampaignRunner, CampaignSpec
from g8e_evals.schema import (
    ToolCallScorecard,
    ToolCallScorecardDimension,
    VerificationStatus,
    validate_tool_call_scorecards,
)


_VALID_HASH = "a" * 64

_CAMPAIGN_ID = "v2.1.8-ifeval-pipeline-integrity"
_RELEASE_VERSION = "v2.1.8"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_ID = "cohort-qwen3-8b"
_REPLICATE_IDS = ["replicate-1"]
_INITIAL_STATE_ID = "no-initial-state-v1"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64


# --- Tier 1: Model validation ---


class TestToolCallScorecardModel:
    pytestmark = pytest.mark.unit

    def test_scorecard_is_frozen(self):
        sc = _make_scorecard()
        with pytest.raises((TypeError, ValueError)):
            sc.recognition = False  # type: ignore[misc]

    def test_scorecard_rejects_extra_fields(self):
        with pytest.raises(ValidationError):
            ToolCallScorecard(
                scorecard_id="sc-1",
                attempt_id="att-1",
                run_id="run-1",
                task_id="task-1",
                agent_persona="sage",
                model_variant_id="qwen3-8b-q4_0",
                tool_name="http_status",
                call_index=0,
                recognition=True,
                selection=True,
                schema_valid=True,
                semantics=True,
                permission=True,
                interpretation=True,
                follow_up=True,
                unnecessary=True,
                looping=True,
                recovery=True,
                extra_field="bad",
            )

    def test_scorecard_has_all_ten_dimensions(self):
        sc = _make_scorecard()
        assert sc.recognition is True
        assert sc.selection is True
        assert sc.schema_valid is True
        assert sc.semantics is True
        assert sc.permission is True
        assert sc.interpretation is True
        assert sc.follow_up is True
        assert sc.unnecessary is True
        assert sc.looping is True
        assert sc.recovery is True

    def test_dimensions_property_returns_all_ten(self):
        sc = _make_scorecard()
        dims = sc.dimensions
        assert len(dims) == 10
        expected = {d.value for d in ToolCallScorecardDimension}
        assert set(dims.keys()) == expected

    def test_dimension_enum_has_exactly_ten_values(self):
        assert len(list(ToolCallScorecardDimension)) == 10
        values = {d.value for d in ToolCallScorecardDimension}
        assert values == {
            "recognition", "selection", "schema", "semantics",
            "permission", "interpretation", "follow_up",
            "unnecessary", "looping", "recovery",
        }

    def test_scorecard_binds_persona_model_tool_task(self):
        sc = _make_scorecard(
            agent_persona="dash",
            model_variant_id="qwen3-32b-q4_0",
            tool_name="logs_search",
            task_id="TS-002",
        )
        assert sc.agent_persona == "dash"
        assert sc.model_variant_id == "qwen3-32b-q4_0"
        assert sc.tool_name == "logs_search"
        assert sc.task_id == "TS-002"

    def test_call_index_must_be_non_negative(self):
        with pytest.raises(ValidationError):
            _make_scorecard(call_index=-1)

    def test_empty_agent_persona_rejected(self):
        with pytest.raises(ValidationError):
            _make_scorecard(agent_persona="")

    def test_empty_model_variant_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_scorecard(model_variant_id="")

    def test_empty_tool_name_rejected(self):
        with pytest.raises(ValidationError):
            _make_scorecard(tool_name="")

    def test_empty_task_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_scorecard(task_id="")

    def test_empty_scorecard_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_scorecard(scorecard_id="")

    def test_empty_attempt_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_scorecard(attempt_id="")

    def test_empty_run_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_scorecard(run_id="")

    def test_verified_scorecard_requires_evidence_refs(self):
        with pytest.raises(ValidationError, match="source evidence"):
            _make_scorecard(verification_status=VerificationStatus.VERIFIED)

    def test_verified_scorecard_requires_evidence_sha256(self):
        with pytest.raises(ValidationError, match="source evidence"):
            _make_scorecard(
                verification_status=VerificationStatus.VERIFIED,
                source_evidence_refs=["ev-1"],
            )

    def test_verified_scorecard_with_evidence_passes(self):
        sc = _make_scorecard(
            verification_status=VerificationStatus.VERIFIED,
            source_evidence_refs=["ev-1"],
            source_evidence_sha256=_VALID_HASH,
        )
        assert sc.verification_status == VerificationStatus.VERIFIED

    def test_duplicate_evidence_refs_rejected(self):
        with pytest.raises(ValidationError, match="unique"):
            _make_scorecard(source_evidence_refs=["ev-1", "ev-1"])

    def test_invalid_evidence_sha256_rejected(self):
        with pytest.raises(ValidationError):
            _make_scorecard(source_evidence_sha256="not-a-hash")

    def test_pending_scorecard_without_evidence_passes(self):
        sc = _make_scorecard()
        assert sc.verification_status == VerificationStatus.PENDING
        assert sc.source_evidence_refs == []
        assert sc.source_evidence_sha256 is None

    def test_scorecard_serialization_roundtrip(self):
        sc = _make_scorecard()
        data = json.loads(sc.model_dump_json())
        assert data["scorecard_id"] == "sc-1"
        assert data["recognition"] is True
        restored = ToolCallScorecard.model_validate(data)
        assert restored == sc


class TestValidateToolCallScorecards:
    pytestmark = pytest.mark.unit

    def test_valid_scorecards_pass(self):
        sc1 = _make_scorecard(scorecard_id="sc-1", call_index=0)
        sc2 = _make_scorecard(scorecard_id="sc-2", call_index=1)
        validate_tool_call_scorecards([sc1, sc2])

    def test_duplicate_run_attempt_tool_call_index_rejected(self):
        sc1 = _make_scorecard(scorecard_id="sc-1", call_index=0)
        sc2 = _make_scorecard(scorecard_id="sc-2", call_index=0)
        with pytest.raises(ValueError, match="duplicate"):
            validate_tool_call_scorecards([sc1, sc2])

    def test_same_call_index_different_tool_passes(self):
        sc1 = _make_scorecard(scorecard_id="sc-1", tool_name="http_status", call_index=0)
        sc2 = _make_scorecard(scorecard_id="sc-2", tool_name="logs_search", call_index=0)
        validate_tool_call_scorecards([sc1, sc2])

    def test_same_call_index_different_attempt_passes(self):
        sc1 = _make_scorecard(scorecard_id="sc-1", attempt_id="att-1", call_index=0)
        sc2 = _make_scorecard(scorecard_id="sc-2", attempt_id="att-2", call_index=0)
        validate_tool_call_scorecards([sc1, sc2])

    def test_empty_list_passes(self):
        validate_tool_call_scorecards([])


# --- Tier 1: Metric registry ---


class TestToolCallScorecardMetrics:
    pytestmark = pytest.mark.unit

    _SCORECARD_METRIC_IDS = frozenset({
        "tool_call_recognition",
        "tool_call_selection",
        "tool_call_schema",
        "tool_call_semantics",
        "tool_call_permission",
        "tool_call_interpretation",
        "tool_call_follow_up",
        "tool_call_unnecessary",
        "tool_call_looping",
        "tool_call_recovery",
    })

    def test_all_ten_scorecard_metrics_registered(self):
        for metric_id in self._SCORECARD_METRIC_IDS:
            assert DEFAULT_METRIC_REGISTRY.is_registered(metric_id, "1.0.0"), (
                f"metric {metric_id} is not registered"
            )

    def test_scorecard_metrics_are_analysis_class(self):
        for metric_id in self._SCORECARD_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.grader_class == GraderClass.ANALYSIS, (
                f"metric {metric_id} has grader_class {definition.grader_class}, expected ANALYSIS"
            )

    def test_scorecard_metrics_are_higher_is_better(self):
        for metric_id in self._SCORECARD_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.direction == MetricDirection.HIGHER_IS_BETTER, (
                f"metric {metric_id} has direction {definition.direction}, expected HIGHER_IS_BETTER"
            )

    def test_scorecard_metrics_have_proportion_aggregation(self):
        for metric_id in self._SCORECARD_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.aggregation.value == "proportion", (
                f"metric {metric_id} has aggregation {definition.aggregation}, expected proportion"
            )

    def test_scorecard_metrics_have_tool_call_scorecard_evidence(self):
        for metric_id in self._SCORECARD_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert "tool_call_scorecard" in definition.evidence_requirements, (
                f"metric {metric_id} does not require tool_call_scorecard evidence"
            )

    def test_scorecard_metrics_have_non_inferiority_margin(self):
        for metric_id in self._SCORECARD_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.non_inferiority_margin is not None, (
                f"metric {metric_id} has no non-inferiority margin"
            )

    def test_scorecard_metrics_in_release_set(self):
        release_ids = {m.metric_id for m in RELEASE_METRIC_SET.metrics}
        for metric_id in self._SCORECARD_METRIC_IDS:
            assert metric_id in release_ids, (
                f"metric {metric_id} is not in the release set"
            )

    def test_scorecard_metrics_have_domain_mapping(self):
        domain_map = {m.metric_id: m.domain for m in RELEASE_METRIC_SET.metrics}
        for metric_id in self._SCORECARD_METRIC_IDS:
            assert metric_id in domain_map, (
                f"metric {metric_id} has no domain mapping"
            )
            assert domain_map[metric_id] in (MetricDomain.UTILITY, MetricDomain.GOVERNANCE), (
                f"metric {metric_id} has domain {domain_map[metric_id]}, expected UTILITY or GOVERNANCE"
            )


# --- Tier 2: Campaign verifier integration ---


@dataclass
class _FakeSUT:
    model_id: str
    answer: str = "This is a test answer with no commas."

    async def get_answer(self, task: Task) -> Response:
        return Response(answer=self.answer, model=self.model_id, arm=Arm.DIRECT)


@dataclass
class _FakeGrader:
    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        return Score(task_id=task.id, passed=True, details=ScoreDetails())


def _make_spec() -> CampaignSpec:
    cohort = ModelCohort(
        cohort_id=_COHORT_ID,
        role_bindings=[RoleModelBinding(
            role="primary",
            model_id="qwen3:8b",
            provider="ollama",
            endpoint="http://192.168.1.2:11434",
            sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
            timeout_seconds=120.0,
            seed_capable=True,
        )],
        content_hash=compute_model_cohort_hash(_COHORT_ID, [RoleModelBinding(
            role="primary",
            model_id="qwen3:8b",
            provider="ollama",
            endpoint="http://192.168.1.2:11434",
            sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
            timeout_seconds=120.0,
            seed_capable=True,
        )]),
    )
    task_assignment = TaskAssignmentManifest(
        task_assignment_id="task-assignment-v1",
        suite_id="ifeval_subset",
        dataset_hash=_DATASET_HASH,
        task_ids=_TASK_IDS,
        content_hash=compute_task_assignment_hash("task-assignment-v1", "ifeval_subset", _DATASET_HASH, _TASK_IDS),
    )
    initial_state = InitialStateAssignmentManifest(
        initial_state_assignment_id=_INITIAL_STATE_ID,
        state_type="no_initial_state",
        snapshot_hash=_NO_STATE_HASH,
        content_hash=compute_initial_state_hash(_INITIAL_STATE_ID, "no_initial_state", _NO_STATE_HASH),
    )
    prereg = PreregistrationConfig(
        config_id="test-config-1",
        config_version="1.0.0",
        baseline_arm_id="direct",
        comparison_arm_ids=["ensemble_ungoverned"],
        model_cohort_ids=[_COHORT_ID],
        task_assignment_id="task-assignment-v1",
        initial_state_assignment_id=_INITIAL_STATE_ID,
        required_replicate_ids=_REPLICATE_IDS,
        required_replicate_count=1,
        primary_metric_ids=["ifeval_subset_verifier"],
        continuous_test_policy=ContinuousTestPolicy.PAIRED_T,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=0,
        significance_level=0.05,
        claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY,
    )
    prompt_bundle = "\n".join(f"Prompt for {tid}" for tid in _TASK_IDS).encode()
    return CampaignSpec(
        campaign_id=_CAMPAIGN_ID,
        release_version=_RELEASE_VERSION,
        suite="ifeval_subset",
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        dataset_hash=_DATASET_HASH,
        prompt_bundle_hash=hashlib.sha256(prompt_bundle).hexdigest(),
        grader_bundle_hash="g" * 64,
        preregistration=prereg,
        cohorts=[cohort],
        task_assignment=task_assignment,
        initial_state=initial_state,
        retry_policy=RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"]),
        randomization_seed=42,
    )


def _make_tasks() -> list[Task]:
    tasks = []
    for tid in _TASK_IDS:
        tasks.append(Task(
            id=tid,
            prompt=f"Prompt for {tid}",
            metadata=TaskMetadata(
                benchmark="ifeval_subset",
                instruction_id_list=["punctuation:no_comma"],
                kwargs=[{"no_comma": True}],
            ),
        ))
    return tasks


def _fake_sut_factory(cohort: ModelCohort, arm: Arm):
    return _FakeSUT(model_id=cohort.role_bindings[0].model_id)


def _run_campaign(tmp_path: Path) -> Path:
    spec = _make_spec()
    runner = CampaignRunner(
        spec=spec,
        sut_factory=_fake_sut_factory,
        tasks=_make_tasks(),
        grader=_FakeGrader(),
        output_dir=tmp_path,
    )
    result = asyncio.run(runner.run())
    return result.report_dir


def _first_attempt_id(report_dir: Path) -> str:
    """Read the first attempt_id from the attempts JSONL file."""
    from g8e_evals.constants import ATTEMPTS_JSONL
    attempts_path = report_dir / ATTEMPTS_JSONL
    lines = attempts_path.read_text().strip().splitlines()
    if not lines:
        pytest.skip("no attempts in campaign")
    first = json.loads(lines[0])
    return first["attempt_id"]


class TestCampaignVerifierToolCallScorecardLayer:
    pytestmark = pytest.mark.integration

    def test_verifier_has_tool_call_scorecard_layer(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert "tool_call_scorecard" in result.checked_layers

    def test_verifier_passes_without_scorecards(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert result.ok, f"campaign without scorecards should pass: {result.failures}"
        assert "tool_call_scorecard" in result.checked_layers

    def test_verifier_validates_scorecards_when_present(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        attempt_id = _first_attempt_id(report_dir)
        sc_path = report_dir / TOOL_CALL_SCORECARDS_JSONL
        sc_data = _make_scorecard_dict(attempt_id=attempt_id)
        sc_path.write_text(json.dumps(sc_data) + "\n")
        result = verify_campaign(report_dir)
        assert result.ok, f"valid scorecard should pass: {result.failures}"

    def test_verifier_rejects_malformed_scorecards(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        sc_path = report_dir / TOOL_CALL_SCORECARDS_JSONL
        bad_sc = _make_scorecard_dict()
        del bad_sc["run_id"]
        sc_path.write_text(json.dumps(bad_sc) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("tool call scorecard" in f.lower() for f in result.failures)

    def test_verifier_rejects_duplicate_scorecards(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        attempt_id = _first_attempt_id(report_dir)
        sc_path = report_dir / TOOL_CALL_SCORECARDS_JSONL
        sc_data = _make_scorecard_dict(attempt_id=attempt_id)
        sc_path.write_text(json.dumps(sc_data) + "\n" + json.dumps(sc_data) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("tool call scorecard" in f.lower() and "duplicate" in f.lower() for f in result.failures)

    def test_verifier_rejects_scorecard_bound_to_unknown_attempt(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        sc_path = report_dir / TOOL_CALL_SCORECARDS_JSONL
        sc_data = _make_scorecard_dict(attempt_id="nonexistent-attempt")
        sc_path.write_text(json.dumps(sc_data) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("unknown attempt" in f.lower() for f in result.failures)

    def test_verifier_rejects_symlinked_scorecards(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        attempt_id = _first_attempt_id(report_dir)
        sc_path = report_dir / TOOL_CALL_SCORECARDS_JSONL
        target = report_dir / "real-scorecards.jsonl"
        target.write_text(json.dumps(_make_scorecard_dict(attempt_id=attempt_id)) + "\n")
        sc_path.symlink_to(target)
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("symlink" in f.lower() and "scorecard" in f.lower() for f in result.failures)


# --- Helpers ---


def _make_scorecard(
    *,
    scorecard_id: str = "sc-1",
    attempt_id: str = "att-1",
    run_id: str = "run-1",
    task_id: str = "task-1",
    agent_persona: str = "sage",
    model_variant_id: str = "qwen3-8b-q4_0",
    tool_name: str = "http_status",
    call_index: int = 0,
    recognition: bool = True,
    selection: bool = True,
    schema_valid: bool = True,
    semantics: bool = True,
    permission: bool = True,
    interpretation: bool = True,
    follow_up: bool = True,
    unnecessary: bool = True,
    looping: bool = True,
    recovery: bool = True,
    source_evidence_refs: list[str] | None = None,
    source_evidence_sha256: str | None = None,
    verification_status: VerificationStatus = VerificationStatus.PENDING,
) -> ToolCallScorecard:
    return ToolCallScorecard(
        scorecard_id=scorecard_id,
        attempt_id=attempt_id,
        run_id=run_id,
        task_id=task_id,
        agent_persona=agent_persona,
        model_variant_id=model_variant_id,
        tool_name=tool_name,
        call_index=call_index,
        recognition=recognition,
        selection=selection,
        schema_valid=schema_valid,
        semantics=semantics,
        permission=permission,
        interpretation=interpretation,
        follow_up=follow_up,
        unnecessary=unnecessary,
        looping=looping,
        recovery=recovery,
        source_evidence_refs=source_evidence_refs or [],
        source_evidence_sha256=source_evidence_sha256,
        verification_status=verification_status,
    )


def _make_scorecard_dict(
    *,
    attempt_id: str = "att-1",
    run_id: str = "run-1",
    task_id: str = "task-1",
    tool_name: str = "http_status",
    call_index: int = 0,
    scorecard_id: str = "sc-1",
) -> dict:
    sc = _make_scorecard(
        scorecard_id=scorecard_id,
        attempt_id=attempt_id,
        run_id=run_id,
        task_id=task_id,
        tool_name=tool_name,
        call_index=call_index,
    )
    return json.loads(sc.model_dump_json())
