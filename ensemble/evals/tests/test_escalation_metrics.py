# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 and Tier 2 tests for the EF4 escalation metrics.

Verifies that:
- ``EscalationRecord`` is frozen with ``extra="forbid"`` and carries all
  required bindings (agent persona, model variant, expected role,
  ground-truth complexity, routed-to role, outcome, task success).
- ``EscalationOutcome`` enumerates exactly the 5 outcome classes.
- ``validate_escalation_records`` rejects duplicate (run, attempt, task)
  tuples and rejects the derived ``ESCALATION_EFFICIENCY`` outcome at
  the per-record level.
- Verified records require source evidence.
- The campaign verifier has an ``escalation_records`` layer that
  validates records when present, passes when absent, rejects malformed
  records, rejects duplicates, and rejects records bound to unknown
  attempts.
- The 5 escalation metrics are registered in the metric registry with
  correct grader class, direction, and release domain.
- The derived producers compute correct proportions from
  ``EscalationRecord`` records.
"""

# pyright: reportCallIssue=false, reportArgumentType=false
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
from g8e_evals.analysis.derived import (
    produce_escalation_correct_autonomous_observations,
    produce_escalation_correct_escalation_observations,
    produce_escalation_efficiency_observations,
    produce_escalation_false_escalation_observations,
    produce_escalation_missed_escalation_observations,
)
from g8e_evals.analysis.input import AnalysisInputRecord
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
from g8e_evals.constants import ESCALATION_RECORDS_JSONL
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
    AttemptRecord,
    EscalationOutcome,
    EscalationRecord,
    VerificationStatus,
    validate_escalation_records,
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


class TestEscalationRecordModel:
    pytestmark = pytest.mark.unit

    def test_record_is_frozen(self):
        er = _make_record()
        with pytest.raises((TypeError, ValueError)):
            er.outcome = EscalationOutcome.FALSE_ESCALATION  # type: ignore[misc]

    def test_record_rejects_extra_fields(self):
        with pytest.raises(ValidationError):
            EscalationRecord(
                record_id="er-1",
                campaign_id="campaign-1",
                child_id="campaign-1",
                assignment_id="assignment-1",
                attempt_id="att-1",
                run_id="run-1",
                task_id="task-1",
                agent_persona="triage",
                model_variant_id="smollm2-360m-q4_0",
                expected_role="lite",
                ground_truth_complexity="light",
                routed_to_role="lite",
                outcome=EscalationOutcome.CORRECT_AUTONOMOUS,
                task_succeeded=True,
                extra_field="bad",
            )

    def test_record_has_all_required_fields(self):
        er = _make_record()
        assert er.record_id == "er-1"
        assert er.attempt_id == "att-1"
        assert er.run_id == "run-1"
        assert er.task_id == "task-1"
        assert er.agent_persona == "triage"
        assert er.model_variant_id == "smollm2-360m-q4_0"
        assert er.expected_role == "lite"
        assert er.ground_truth_complexity == "light"
        assert er.routed_to_role == "lite"
        assert er.outcome == EscalationOutcome.CORRECT_AUTONOMOUS
        assert er.task_succeeded is True

    def test_outcome_enum_has_exactly_five_values(self):
        assert len(list(EscalationOutcome)) == 5
        values = {o.value for o in EscalationOutcome}
        assert values == {
            "correct_autonomous", "correct_escalation",
            "false_escalation", "missed_escalation",
            "escalation_efficiency",
        }

    def test_record_binds_persona_model_role_task(self):
        er = _make_record(
            agent_persona="sage",
            model_variant_id="qwen3-8b-q4_0",
            expected_role="primary",
            ground_truth_complexity="primary",
            routed_to_role="primary",
            task_id="TS-002",
        )
        assert er.agent_persona == "sage"
        assert er.model_variant_id == "qwen3-8b-q4_0"
        assert er.expected_role == "primary"
        assert er.ground_truth_complexity == "primary"
        assert er.routed_to_role == "primary"
        assert er.task_id == "TS-002"

    def test_empty_record_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(record_id="")

    def test_empty_attempt_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(attempt_id="")

    def test_empty_run_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(run_id="")

    def test_empty_task_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(task_id="")

    def test_empty_agent_persona_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(agent_persona="")

    def test_empty_model_variant_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(model_variant_id="")

    def test_empty_expected_role_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(expected_role="")

    def test_empty_ground_truth_complexity_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(ground_truth_complexity="")

    def test_empty_routed_to_role_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(routed_to_role="")

    def test_verified_record_requires_evidence_refs(self):
        with pytest.raises(ValidationError, match="source evidence"):
            _make_record(verification_status=VerificationStatus.VERIFIED)

    def test_verified_record_requires_evidence_sha256(self):
        with pytest.raises(ValidationError, match="source evidence"):
            _make_record(
                verification_status=VerificationStatus.VERIFIED,
                source_evidence_refs=["ev-1"],
            )

    def test_verified_record_with_evidence_passes(self):
        er = _make_record(
            verification_status=VerificationStatus.VERIFIED,
            source_evidence_refs=["ev-1"],
            source_evidence_sha256=_VALID_HASH,
        )
        assert er.verification_status == VerificationStatus.VERIFIED

    def test_duplicate_evidence_refs_rejected(self):
        with pytest.raises(ValidationError, match="unique"):
            _make_record(source_evidence_refs=["ev-1", "ev-1"])

    def test_invalid_evidence_sha256_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(source_evidence_sha256="not-a-hash")

    def test_pending_record_without_evidence_passes(self):
        er = _make_record()
        assert er.verification_status == VerificationStatus.PENDING
        assert er.source_evidence_refs == []
        assert er.source_evidence_sha256 is None

    def test_record_serialization_roundtrip(self):
        er = _make_record()
        data = json.loads(er.model_dump_json())
        assert data["record_id"] == "er-1"
        assert data["outcome"] == "correct_autonomous"
        restored = EscalationRecord.model_validate(data)
        assert restored == er


class TestValidateEscalationRecords:
    pytestmark = pytest.mark.unit

    def test_valid_records_pass(self):
        er1 = _make_record(record_id="er-1", task_id="task-1")
        er2 = _make_record(record_id="er-2", task_id="task-2")
        validate_escalation_records([er1, er2])

    def test_duplicate_run_attempt_task_rejected(self):
        er1 = _make_record(record_id="er-1", task_id="task-1")
        er2 = _make_record(record_id="er-2", task_id="task-1")
        with pytest.raises(ValueError, match="duplicate"):
            validate_escalation_records([er1, er2])

    def test_same_task_different_attempt_passes(self):
        er1 = _make_record(record_id="er-1", attempt_id="att-1", task_id="task-1")
        er2 = _make_record(record_id="er-2", attempt_id="att-2", task_id="task-1")
        validate_escalation_records([er1, er2])

    def test_same_task_different_run_passes(self):
        er1 = _make_record(record_id="er-1", run_id="run-1", task_id="task-1")
        er2 = _make_record(record_id="er-2", run_id="run-2", task_id="task-1")
        validate_escalation_records([er1, er2])

    def test_empty_list_passes(self):
        validate_escalation_records([])

    def test_derived_efficiency_outcome_rejected(self):
        er = _make_record(
            record_id="er-1",
            outcome=EscalationOutcome.ESCALATION_EFFICIENCY,
        )
        with pytest.raises(ValueError, match="derived outcome"):
            validate_escalation_records([er])


# --- Tier 1: Metric registry ---


class TestEscalationMetrics:
    pytestmark = pytest.mark.unit

    _ESCALATION_METRIC_IDS = frozenset({
        "escalation_correct_autonomous",
        "escalation_correct_escalation",
        "escalation_false_escalation",
        "escalation_missed_escalation",
        "escalation_efficiency",
    })

    def test_all_five_escalation_metrics_registered(self):
        for metric_id in self._ESCALATION_METRIC_IDS:
            assert DEFAULT_METRIC_REGISTRY.is_registered(metric_id, "1.0.0"), (
                f"metric {metric_id} is not registered"
            )

    def test_escalation_metrics_are_analysis_class(self):
        for metric_id in self._ESCALATION_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.grader_class == GraderClass.ANALYSIS, (
                f"metric {metric_id} has grader_class {definition.grader_class}, expected ANALYSIS"
            )

    def test_escalation_metrics_have_proportion_aggregation(self):
        for metric_id in self._ESCALATION_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.aggregation.value == "proportion", (
                f"metric {metric_id} has aggregation {definition.aggregation}, expected proportion"
            )

    def test_escalation_metrics_have_escalation_record_evidence(self):
        for metric_id in self._ESCALATION_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert "escalation_record" in definition.evidence_requirements, (
                f"metric {metric_id} does not require escalation_record evidence"
            )

    def test_escalation_metrics_have_non_inferiority_margin(self):
        for metric_id in self._ESCALATION_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.non_inferiority_margin is not None, (
                f"metric {metric_id} has no non-inferiority margin"
            )

    def test_escalation_metrics_in_release_set(self):
        release_ids = {m.metric_id for m in RELEASE_METRIC_SET.metrics}
        for metric_id in self._ESCALATION_METRIC_IDS:
            assert metric_id in release_ids, (
                f"metric {metric_id} is not in the release set"
            )

    def test_escalation_metrics_have_domain_mapping(self):
        domain_map = {m.metric_id: m.domain for m in RELEASE_METRIC_SET.metrics}
        for metric_id in self._ESCALATION_METRIC_IDS:
            assert metric_id in domain_map, (
                f"metric {metric_id} has no domain mapping"
            )
            assert domain_map[metric_id] == MetricDomain.UTILITY, (
                f"metric {metric_id} has domain {domain_map[metric_id]}, expected UTILITY"
            )

    def test_correct_autonomous_is_higher_is_better(self):
        definition = DEFAULT_METRIC_REGISTRY.get("escalation_correct_autonomous", "1.0.0")
        assert definition.direction == MetricDirection.HIGHER_IS_BETTER

    def test_correct_escalation_is_higher_is_better(self):
        definition = DEFAULT_METRIC_REGISTRY.get("escalation_correct_escalation", "1.0.0")
        assert definition.direction == MetricDirection.HIGHER_IS_BETTER

    def test_false_escalation_is_lower_is_better(self):
        definition = DEFAULT_METRIC_REGISTRY.get("escalation_false_escalation", "1.0.0")
        assert definition.direction == MetricDirection.LOWER_IS_BETTER

    def test_missed_escalation_is_lower_is_better(self):
        definition = DEFAULT_METRIC_REGISTRY.get("escalation_missed_escalation", "1.0.0")
        assert definition.direction == MetricDirection.LOWER_IS_BETTER

    def test_escalation_efficiency_is_higher_is_better(self):
        definition = DEFAULT_METRIC_REGISTRY.get("escalation_efficiency", "1.0.0")
        assert definition.direction == MetricDirection.HIGHER_IS_BETTER


# --- Tier 1: Derived producers ---


class TestEscalationDerivedProducers:
    pytestmark = pytest.mark.unit

    def test_correct_autonomous_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="er-1", outcome=EscalationOutcome.CORRECT_AUTONOMOUS),
            _make_record(record_id="er-2", outcome=EscalationOutcome.MISSED_ESCALATION),
        ])
        results = produce_escalation_correct_autonomous_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.5
        assert results[0].denominator_contribution == 2

    def test_correct_escalation_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="er-1", outcome=EscalationOutcome.CORRECT_ESCALATION),
            _make_record(record_id="er-2", outcome=EscalationOutcome.CORRECT_ESCALATION),
            _make_record(record_id="er-3", outcome=EscalationOutcome.FALSE_ESCALATION),
        ])
        results = produce_escalation_correct_escalation_observations(record)
        assert len(results) == 1
        assert results[0].value == round(2 / 3, 10)
        assert results[0].denominator_contribution == 3

    def test_false_escalation_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="er-1", outcome=EscalationOutcome.FALSE_ESCALATION),
            _make_record(record_id="er-2", outcome=EscalationOutcome.CORRECT_AUTONOMOUS),
        ])
        results = produce_escalation_false_escalation_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.5
        assert results[0].denominator_contribution == 2

    def test_missed_escalation_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="er-1", outcome=EscalationOutcome.MISSED_ESCALATION),
            _make_record(record_id="er-2", outcome=EscalationOutcome.CORRECT_AUTONOMOUS),
        ])
        results = produce_escalation_missed_escalation_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.5
        assert results[0].denominator_contribution == 2

    def test_efficiency_producer_computes_combined_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="er-1", outcome=EscalationOutcome.CORRECT_AUTONOMOUS),
            _make_record(record_id="er-2", outcome=EscalationOutcome.CORRECT_ESCALATION),
            _make_record(record_id="er-3", outcome=EscalationOutcome.FALSE_ESCALATION),
            _make_record(record_id="er-4", outcome=EscalationOutcome.MISSED_ESCALATION),
        ])
        results = produce_escalation_efficiency_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.5
        assert results[0].denominator_contribution == 4

    def test_no_escalation_records_produces_no_observations(self):
        record = _make_analysis_record([])
        results = produce_escalation_correct_autonomous_observations(record)
        assert results == []
        results = produce_escalation_efficiency_observations(record)
        assert results == []

    def test_multiple_attempts_produce_separate_observations(self):
        record = _make_analysis_record([
            _make_record(record_id="er-1", attempt_id="att-1", task_id="task-1", outcome=EscalationOutcome.CORRECT_AUTONOMOUS),
            _make_record(record_id="er-2", attempt_id="att-2", task_id="task-2", outcome=EscalationOutcome.MISSED_ESCALATION),
        ], attempts=[
            _make_attempt(attempt_id="att-1", task_id="task-1"),
            _make_attempt(attempt_id="att-2", task_id="task-2"),
        ])
        results = produce_escalation_correct_autonomous_observations(record)
        assert len(results) == 2
        att1_result = next(r for r in results if r.attempt_id == "att-1")
        att2_result = next(r for r in results if r.attempt_id == "att-2")
        assert att1_result.value == 1.0
        assert att2_result.value == 0.0

    def test_verified_records_produce_verified_observations(self):
        record = _make_analysis_record([
            _make_record(
                record_id="er-1",
                outcome=EscalationOutcome.CORRECT_AUTONOMOUS,
                verification_status=VerificationStatus.VERIFIED,
                source_evidence_refs=["ev-1"],
                source_evidence_sha256=_VALID_HASH,
            ),
        ])
        results = produce_escalation_correct_autonomous_observations(record)
        assert len(results) == 1
        assert results[0].verification_status == VerificationStatus.VERIFIED

    def test_pending_records_produce_pending_observations(self):
        record = _make_analysis_record([
            _make_record(
                record_id="er-1",
                outcome=EscalationOutcome.CORRECT_AUTONOMOUS,
                verification_status=VerificationStatus.PENDING,
            ),
        ])
        results = produce_escalation_correct_autonomous_observations(record)
        assert len(results) == 1
        assert results[0].verification_status == VerificationStatus.PENDING


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


def _campaign_identity(report_dir: Path) -> dict:
    """Read the actual campaign identity from the report directory."""
    from g8e_evals.constants import ATTEMPTS_JSONL, CAMPAIGN_MANIFEST_JSON
    cm = json.loads((report_dir / CAMPAIGN_MANIFEST_JSON).read_text())
    attempts_path = report_dir / ATTEMPTS_JSONL
    lines = attempts_path.read_text().strip().splitlines()
    if not lines:
        pytest.skip("no attempts in campaign")
    first = json.loads(lines[0])
    return {
        "campaign_id": cm["campaign_id"],
        "child_id": cm["campaign_id"],
        "run_id": first["run_id"],
        "task_id": first["task_id"],
        "assignment_id": first.get("assignment_id", ""),
        "attempt_id": first["attempt_id"],
    }


class TestCampaignVerifierEscalationRecordsLayer:
    pytestmark = pytest.mark.integration

    def test_verifier_has_escalation_records_layer(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert "escalation_records" in result.checked_layers

    def test_verifier_passes_without_escalation_records(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert result.ok, f"campaign without escalation records should pass: {result.failures}"
        assert "escalation_records" in result.checked_layers

    def test_verifier_validates_escalation_records_when_present(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        er_path = report_dir / ESCALATION_RECORDS_JSONL
        er_data = _make_record_dict(**ident)
        er_path.write_text(json.dumps(er_data) + "\n")
        result = verify_campaign(report_dir)
        assert result.ok, f"valid escalation record should pass: {result.failures}"

    def test_verifier_rejects_malformed_escalation_records(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        er_path = report_dir / ESCALATION_RECORDS_JSONL
        bad_er = _make_record_dict()
        del bad_er["run_id"]
        er_path.write_text(json.dumps(bad_er) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("escalation record" in f.lower() for f in result.failures)

    def test_verifier_rejects_duplicate_escalation_records(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        er_path = report_dir / ESCALATION_RECORDS_JSONL
        er_data = _make_record_dict(**ident)
        er_path.write_text(json.dumps(er_data) + "\n" + json.dumps(er_data) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("escalation record" in f.lower() and "duplicate" in f.lower() for f in result.failures)

    def test_verifier_rejects_escalation_record_bound_to_unknown_attempt(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        er_path = report_dir / ESCALATION_RECORDS_JSONL
        er_data = _make_record_dict(attempt_id="nonexistent-attempt")
        er_path.write_text(json.dumps(er_data) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("unknown attempt" in f.lower() for f in result.failures)

    def test_verifier_rejects_symlinked_escalation_records(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        er_path = report_dir / ESCALATION_RECORDS_JSONL
        target = report_dir / "real-escalation-records.jsonl"
        target.write_text(json.dumps(_make_record_dict(**ident)) + "\n")
        er_path.unlink(missing_ok=True)
        er_path.symlink_to(target)
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("symlink" in f.lower() and "escalation" in f.lower() for f in result.failures)


# --- Helpers ---


def _make_record(
    *,
    record_id: str = "er-1",
    campaign_id: str = "campaign-1",
    child_id: str = "campaign-1",
    assignment_id: str = "assignment-1",
    attempt_id: str = "att-1",
    inference_id: str | None = None,
    run_id: str = "run-1",
    task_id: str = "task-1",
    agent_persona: str = "triage",
    model_variant_id: str = "smollm2-360m-q4_0",
    expected_role: str = "lite",
    ground_truth_complexity: str = "light",
    routed_to_role: str = "lite",
    outcome: EscalationOutcome = EscalationOutcome.CORRECT_AUTONOMOUS,
    task_succeeded: bool = True,
    source_evidence_refs: list[str] | None = None,
    source_evidence_sha256: str | None = None,
    verification_status: VerificationStatus = VerificationStatus.PENDING,
) -> EscalationRecord:
    return EscalationRecord(
        record_id=record_id,
        campaign_id=campaign_id,
        child_id=child_id,
        assignment_id=assignment_id,
        attempt_id=attempt_id,
        inference_id=inference_id,
        run_id=run_id,
        task_id=task_id,
        agent_persona=agent_persona,
        model_variant_id=model_variant_id,
        expected_role=expected_role,
        ground_truth_complexity=ground_truth_complexity,
        routed_to_role=routed_to_role,
        outcome=outcome,
        task_succeeded=task_succeeded,
        source_evidence_refs=source_evidence_refs or [],
        source_evidence_sha256=source_evidence_sha256,
        verification_status=verification_status,
    )


def _make_record_dict(
    *,
    attempt_id: str = "att-1",
    run_id: str = "run-1",
    task_id: str = "task-1",
    campaign_id: str = "campaign-1",
    child_id: str = "campaign-1",
    assignment_id: str = "assignment-1",
    record_id: str = "er-1",
) -> dict:
    er = _make_record(
        record_id=record_id,
        campaign_id=campaign_id,
        child_id=child_id,
        assignment_id=assignment_id,
        attempt_id=attempt_id,
        run_id=run_id,
        task_id=task_id,
    )
    return json.loads(er.model_dump_json())


def _make_attempt(
    attempt_id: str = "att-1",
    task_id: str = "task-1",
    arm_id: Arm = Arm.DIRECT,
) -> AttemptRecord:
    return AttemptRecord(
        attempt_id=attempt_id,
        run_id="run-1",
        task_id=task_id,
        arm_id=arm_id,
    )


def _make_analysis_record(
    escalation_records: list[EscalationRecord],
    attempts: list[AttemptRecord] | None = None,
) -> AnalysisInputRecord:
    return AnalysisInputRecord(
        run_id="run-1",
        release_version="v2.1.8",
        tasks=[],
        attempts=attempts or [_make_attempt()],
        escalation_records=escalation_records,
    )
