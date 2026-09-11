# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for observation and event record verifier bindings.

Verifies that the strengthened campaign verifier layers 13-19 enforce:

- Expected-record policy: required files must exist, not-applicable files
  must not exist, exact cardinality is enforced.
- Identity cross-binding: every resource observation and event record
  must match the campaign's known campaign_id, run_id, assignment_id,
  attempt_id, and task_id.
- Orphan detection: records referencing unknown attempts are rejected.
- Evidence binding: VERIFIED records must have source evidence references
  and a source evidence hash.
- Resource observation count: completed attempts must have at least one
  resource observation.
- Verification report persistence: the canonical child verification report
  is written to the report directory.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
from dataclasses import dataclass
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

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
from g8e_evals.constants import (
    ATTEMPTS_JSONL,
    CAMPAIGN_MANIFEST_JSON,
    CAMPAIGN_VERIFICATION_REPORT_JSON,
    CORRELATED_ERRORS_JSONL,
    EVIDENCE_INDEX_JSONL,
    ESCALATION_RECORDS_JSONL,
    EXPECTED_RECORD_POLICY_JSON,
    RESOURCE_OBSERVATIONS_JSONL,
    SECURITY_EVENTS_JSONL,
    TOOL_CALL_SCORECARDS_JSONL,
)
from g8e_evals.expected_record_policy import (
    CardinalityRule,
    ExpectedRecordEntry,
    ExpectedRecordPolicy,
    RecordApplicability,
    compute_expected_record_policy_hash,
)
from g8e_evals.harness import Response, Score, Task
from g8e_evals.index import (
    MeasurementAvailability,
    MeasurementScope,
    ModelRole,
    ResourceObservation,
    UnavailableMeasurement,
)
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.runner import CampaignRunner, CampaignSpec
from g8e_evals.schema import (
    CorrelatedErrorRecord,
    ErrorClassLabel,
    EscalationOutcome,
    EscalationRecord,
    EvidenceIndex,
    EvidenceMediaType,
    SecurityEventRecord,
    StackCompositionType,
    ToolCallScorecard,
    VerificationStatus,
)

_CAMPAIGN_ID = "v2.1.8-ifeval-pipeline-integrity"
_RELEASE_VERSION = "v2.1.8"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_ID = "cohort-qwen3-8b"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64
_VALID_HASH = "a" * 64

_MEASUREMENT_FIELDS = (
    "model_load_time_seconds",
    "peak_resident_memory_bytes",
    "peak_accelerator_memory_bytes",
    "artifact_bytes",
    "measured_energy_joules",
    "end_to_end_latency_seconds",
    "provider_call_latency_seconds",
    "output_throughput_tokens_per_second",
    "hidden_reasoning_throughput_tokens_per_second",
    "time_to_first_token_seconds",
    "generation_duration_seconds",
    "accelerator_memory_before_bytes",
    "gpu_utilization_percent",
    "gpu_temperature_celsius",
    "gpu_power_draw_watts",
    "gpu_clock_mhz",
)


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
        initial_state_assignment_id="no-initial-state-v1",
        state_type="no_initial_state",
        snapshot_hash=_NO_STATE_HASH,
        content_hash=compute_initial_state_hash("no-initial-state-v1", "no_initial_state", _NO_STATE_HASH),
    )
    prereg = PreregistrationConfig(
        config_id="test-config-1",
        config_version="1.0.0",
        baseline_arm_id="direct",
        comparison_arm_ids=["ensemble_ungoverned"],
        model_cohort_ids=[_COHORT_ID],
        task_assignment_id="task-assignment-v1",
        initial_state_assignment_id="no-initial-state-v1",
        required_replicate_ids=["replicate-1"],
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
    """Run a fake-provider campaign and return the report directory."""
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


def _campaign_identity(report_dir: Path) -> dict:
    """Read the actual campaign identity from the report directory."""
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


def _all_attempt_identities(report_dir: Path) -> list[dict]:
    """Read all attempt identities from the report directory."""
    cm = json.loads((report_dir / CAMPAIGN_MANIFEST_JSON).read_text())
    attempts_path = report_dir / ATTEMPTS_JSONL
    lines = attempts_path.read_text().strip().splitlines()
    identities = []
    for line in lines:
        a = json.loads(line)
        identities.append({
            "campaign_id": cm["campaign_id"],
            "child_id": cm["campaign_id"],
            "run_id": a["run_id"],
            "task_id": a["task_id"],
            "assignment_id": a.get("assignment_id", ""),
            "attempt_id": a["attempt_id"],
        })
    return identities


def _all_unavailable() -> list[UnavailableMeasurement]:
    return [
        UnavailableMeasurement(
            field_name=f,
            availability=MeasurementAvailability.UNAVAILABLE,
            scope=MeasurementScope.PROVIDER_REMOTE,
            reason=f"{f} not available",
        )
        for f in _MEASUREMENT_FIELDS
    ]


def _write_observations_for_all_attempts(report_dir: Path) -> None:
    """Write one valid resource observation per completed attempt.

    Also writes a matching evidence-index.jsonl entry so VERIFIED
    observations pass the strict evidence index resolution check.
    """
    idents = _all_attempt_identities(report_dir)
    lines = []
    for i, ident in enumerate(idents):
        obs = _make_observation_dict(ident={**ident, "inference_id": f"inf-{i}"})
        lines.append(json.dumps(obs))
    (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text("\n".join(lines) + "\n")
    if idents:
        entry = EvidenceIndex(
            artifact_id="evidence/test.json",
            run_id=idents[0]["run_id"],
            attempt_id=None,
            media_type=EvidenceMediaType.APPLICATION_JSON,
            sha256=_VALID_HASH,
        )
        (report_dir / EVIDENCE_INDEX_JSONL).write_text(entry.model_dump_json() + "\n")


def _make_observation_dict(**kwargs) -> dict:
    """Create a valid ResourceObservation dict matching the campaign identity."""
    ident = kwargs.pop("ident", None)
    defaults = {
        "campaign_id": "campaign-1",
        "child_id": "campaign-1",
        "run_id": "run-1",
        "assignment_id": "assignment-1",
        "attempt_id": "att-1",
        "inference_id": "inf-1",
        "stage_id": "att-1:direct:1",
        "role": "primary",
        "model_variant_id": "qwen3:8b",
        "task_id": "task-1",
        "orchestrator_scope": "linux/amd64/cpu",
        "provider_scope": "linux/amd64/rtx-4090",
        "observation_boundary": "provider_call",
        "clock_domain": "monotonic",
        "collection_tool": "psutil-5.9",
        "source_evidence_refs": ["evidence/test.json"],
        "source_evidence_sha256": _VALID_HASH,
        "verification_status": "verified",
        "unavailable_measurements": [u.model_dump() for u in _all_unavailable()],
    }
    for f in _MEASUREMENT_FIELDS:
        defaults[f] = None
    if ident:
        defaults.update(ident)
    defaults.update(kwargs)
    obs = ResourceObservation(**defaults)
    return json.loads(obs.model_dump_json())


def _make_scorecard_dict(**kwargs) -> dict:
    ident = kwargs.pop("ident", None)
    defaults = {
        "scorecard_id": "sc-1",
        "campaign_id": "campaign-1",
        "child_id": "campaign-1",
        "assignment_id": "assignment-1",
        "attempt_id": "att-1",
        "inference_id": "inf-1",
        "run_id": "run-1",
        "task_id": "task-1",
        "role": "primary",
        "agent_persona": "sage",
        "model_variant_id": "qwen3:8b",
        "tool_name": "http_status",
        "call_index": 0,
        "recognition": True,
        "selection": True,
        "schema_valid": True,
        "semantics": True,
        "permission": True,
        "interpretation": True,
        "follow_up": True,
        "unnecessary": True,
        "looping": True,
        "recovery": True,
    }
    if ident:
        defaults.update(ident)
    defaults.update(kwargs)
    sc = ToolCallScorecard(**defaults)
    return json.loads(sc.model_dump_json())


def _make_escalation_dict(**kwargs) -> dict:
    ident = kwargs.pop("ident", None)
    defaults = {
        "record_id": "er-1",
        "campaign_id": "campaign-1",
        "child_id": "campaign-1",
        "assignment_id": "assignment-1",
        "attempt_id": "att-1",
        "inference_id": None,
        "run_id": "run-1",
        "task_id": "task-1",
        "agent_persona": "triage",
        "model_variant_id": "smollm2-360m-q4_0",
        "expected_role": "lite",
        "ground_truth_complexity": "light",
        "routed_to_role": "lite",
        "outcome": EscalationOutcome.CORRECT_AUTONOMOUS,
        "task_succeeded": True,
    }
    if ident:
        defaults.update(ident)
    defaults.update(kwargs)
    er = EscalationRecord(**defaults)
    return json.loads(er.model_dump_json())


def _make_security_event_dict(**kwargs) -> dict:
    ident = kwargs.pop("ident", None)
    defaults = {
        "record_id": "se-1",
        "campaign_id": "campaign-1",
        "child_id": "campaign-1",
        "assignment_id": "assignment-1",
        "attempt_id": "att-1",
        "inference_id": None,
        "run_id": "run-1",
        "task_id": "task-1",
        "role": "primary",
        "agent_persona": "warden",
        "model_variant_id": "qwen3:8b",
        "governance_layer": "policy",
        "sensitive_data_present": True,
        "sensitive_data_required": False,
        "sensitive_data_sent_externally": False,
        "unnecessary_data_sent_externally": False,
        "policy_prevented_disclosure": False,
        "model_attempted_unauthorized_access": False,
        "tool_attempted_unauthorized_operation": False,
        "authorization_correctly_enforced": True,
        "audit_record_complete": True,
        "audit_record_tampered": False,
        "secret_redaction_successful": True,
    }
    if ident:
        defaults.update(ident)
    defaults.update(kwargs)
    se = SecurityEventRecord(**defaults)
    return json.loads(se.model_dump_json())


def _make_correlated_error_dict(**kwargs) -> dict:
    ident = kwargs.pop("ident", None)
    defaults = {
        "record_id": "ce-1",
        "campaign_id": "campaign-1",
        "child_id": "campaign-1",
        "assignment_id": "assignment-1",
        "attempt_id": "att-1",
        "inference_id": "inf-1",
        "run_id": "run-1",
        "task_id": "task-1",
        "agent_persona": "sage",
        "model_variant_id": "qwen3:8b",
        "stage_role": ModelRole.PRIMARY,
        "error_class": ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM,
        "stack_id": "stack-1",
        "stack_composition_type": StackCompositionType.HETEROGENEOUS,
    }
    if ident:
        defaults.update(ident)
    defaults.update(kwargs)
    ce = CorrelatedErrorRecord(**defaults)
    return json.loads(ce.model_dump_json())


def _make_policy(
    *,
    entries: list[dict] | None = None,
    policy_id: str = "test-policy-1",
) -> ExpectedRecordPolicy:
    """Build a valid ExpectedRecordPolicy with the given entries."""
    if entries is None:
        entries = [
            {"file_name": RESOURCE_OBSERVATIONS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": TOOL_CALL_SCORECARDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": ESCALATION_RECORDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": SECURITY_EVENTS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": CORRELATED_ERRORS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
        ]
    # Normalize entries to include all 5 keys for hash computation
    normalized = [
        {
            "file_name": e["file_name"],
            "applicability": e["applicability"],
            "cardinality_rule": e["cardinality_rule"],
            "expected_count": e.get("expected_count"),
            "derivation_rule": e.get("derivation_rule", ""),
        }
        for e in entries
    ]
    content_hash = compute_expected_record_policy_hash(
        schema_version="1.0.0",
        policy_id=policy_id,
        policy_version="1.0.0",
        suite_id="ifeval_subset",
        entries=normalized,
    )
    return ExpectedRecordPolicy(
        schema_version="1.0.0",
        policy_id=policy_id,
        policy_version="1.0.0",
        suite_id="ifeval_subset",
        entries=[
            ExpectedRecordEntry(
                file_name=e["file_name"],
                applicability=RecordApplicability(e["applicability"]),
                cardinality_rule=CardinalityRule(e["cardinality_rule"]),
                expected_count=e.get("expected_count"),
                derivation_rule=e.get("derivation_rule", ""),
            )
            for e in entries
        ],
        content_hash=content_hash,
    )


def _write_policy(report_dir: Path, policy: ExpectedRecordPolicy) -> None:
    """Write an expected record policy to the report directory."""
    (report_dir / EXPECTED_RECORD_POLICY_JSON).write_text(
        policy.model_dump_json()
    )


class TestExpectedRecordPolicyEnforcement:
    """Layer 13: expected record policy is loaded and enforced."""

    def test_verifier_has_expected_record_policy_layer(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert "expected_record_policy" in result.checked_layers

    def test_valid_policy_passes(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        _write_policy(report_dir, _make_policy())
        result = verify_campaign(report_dir)
        assert result.ok, f"valid policy should pass: {result.failures}"

    def test_required_file_missing_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        # Delete the file to make it actually missing
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).unlink()
        policy = _make_policy(entries=[
            {"file_name": RESOURCE_OBSERVATIONS_JSONL, "applicability": "required", "cardinality_rule": "exact", "expected_count": 0},
            {"file_name": TOOL_CALL_SCORECARDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": ESCALATION_RECORDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": SECURITY_EVENTS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": CORRELATED_ERRORS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
        ])
        _write_policy(report_dir, policy)
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("required record file missing" in f.lower() for f in result.failures)

    def test_required_file_with_exact_count_zero_passes(self, tmp_path: Path):
        """A required file with expected_count=0 must exist and be empty."""
        report_dir = _run_campaign(tmp_path)
        policy = _make_policy(entries=[
            {"file_name": RESOURCE_OBSERVATIONS_JSONL, "applicability": "required", "cardinality_rule": "exact", "expected_count": 0},
            {"file_name": TOOL_CALL_SCORECARDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": ESCALATION_RECORDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": SECURITY_EVENTS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": CORRELATED_ERRORS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
        ])
        _write_policy(report_dir, policy)
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text("")
        result = verify_campaign(report_dir)
        assert result.ok, f"required empty file should pass: {result.failures}"

    def test_required_file_with_wrong_count_fails(self, tmp_path: Path):
        """A required file with expected_count=0 but containing records fails."""
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        policy = _make_policy(entries=[
            {"file_name": RESOURCE_OBSERVATIONS_JSONL, "applicability": "required", "cardinality_rule": "exact", "expected_count": 0},
            {"file_name": TOOL_CALL_SCORECARDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": ESCALATION_RECORDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": SECURITY_EVENTS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": CORRELATED_ERRORS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
        ])
        _write_policy(report_dir, policy)
        obs = _make_observation_dict(ident=ident)
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text(json.dumps(obs) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("cardinality mismatch" in f.lower() for f in result.failures)

    def test_not_applicable_file_present_fails(self, tmp_path: Path):
        """A file declared NOT_APPLICABLE must not exist; a present file is fabricated."""
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        obs = _make_observation_dict(ident=ident)
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text(json.dumps(obs) + "\n")
        policy = _make_policy(entries=[
            {"file_name": RESOURCE_OBSERVATIONS_JSONL, "applicability": "not_applicable", "cardinality_rule": "exact", "expected_count": 0},
            {"file_name": TOOL_CALL_SCORECARDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": ESCALATION_RECORDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": SECURITY_EVENTS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": CORRELATED_ERRORS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
        ])
        _write_policy(report_dir, policy)
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("fabricated" in f.lower() for f in result.failures)


class TestResourceObservationIdentityBinding:
    """Layer 14: resource observations are cross-bound to campaign identities."""

    def test_valid_observation_passes(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        _write_observations_for_all_attempts(report_dir)
        result = verify_campaign(report_dir)
        assert result.ok, f"valid observation should pass: {result.failures}"

    def test_wrong_campaign_id_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        obs = _make_observation_dict(ident={**ident, "campaign_id": "wrong-campaign"})
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text(json.dumps(obs) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("campaign_id mismatch" in f.lower() for f in result.failures)

    def test_wrong_run_id_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        obs = _make_observation_dict(ident={**ident, "run_id": "wrong-run"})
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text(json.dumps(obs) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("unknown run_id" in f.lower() for f in result.failures)

    def test_wrong_attempt_id_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        obs = _make_observation_dict(ident={**ident, "attempt_id": "nonexistent-attempt"})
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text(json.dumps(obs) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("unknown attempt_id" in f.lower() for f in result.failures)

    def test_wrong_task_id_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        obs = _make_observation_dict(ident={**ident, "task_id": "wrong-task"})
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text(json.dumps(obs) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("task_id mismatch" in f.lower() for f in result.failures)

    def test_duplicate_observations_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        obs = _make_observation_dict(ident=ident)
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text(
            json.dumps(obs) + "\n" + json.dumps(obs) + "\n"
        )
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("duplicate" in f.lower() and "observation" in f.lower() for f in result.failures)


class TestToolCallScorecardIdentityBinding:
    """Layer 15: tool call scorecards are cross-bound to campaign identities."""

    def test_valid_scorecard_passes(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        sc = _make_scorecard_dict(ident=ident)
        (report_dir / TOOL_CALL_SCORECARDS_JSONL).write_text(json.dumps(sc) + "\n")
        result = verify_campaign(report_dir)
        assert result.ok, f"valid scorecard should pass: {result.failures}"

    def test_wrong_campaign_id_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        sc = _make_scorecard_dict(ident={**ident, "campaign_id": "wrong-campaign"})
        (report_dir / TOOL_CALL_SCORECARDS_JSONL).write_text(json.dumps(sc) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("campaign_id mismatch" in f.lower() for f in result.failures)

    def test_orphan_scorecard_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        sc = _make_scorecard_dict(ident={**ident, "attempt_id": "nonexistent-attempt"})
        (report_dir / TOOL_CALL_SCORECARDS_JSONL).write_text(json.dumps(sc) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("unknown attempt" in f.lower() for f in result.failures)

    def test_verified_scorecard_without_evidence_fails(self, tmp_path: Path):
        """A VERIFIED scorecard without source evidence is rejected by the verifier.

        The model itself rejects this at construction, so we write raw JSON
        with verification_status=verified and no evidence refs to test the
        verifier's independent check.
        """
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        sc = _make_scorecard_dict(ident=ident)
        # Tamper with the JSON to set verified status without evidence
        sc["verification_status"] = "verified"
        sc["source_evidence_refs"] = []
        sc["source_evidence_sha256"] = None
        (report_dir / TOOL_CALL_SCORECARDS_JSONL).write_text(json.dumps(sc) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("source evidence" in f.lower() for f in result.failures)

    def test_verified_scorecard_with_evidence_passes(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        sc = _make_scorecard_dict(
            ident=ident,
            verification_status=VerificationStatus.VERIFIED,
            source_evidence_refs=["evidence-1"],
            source_evidence_sha256=_VALID_HASH,
        )
        (report_dir / TOOL_CALL_SCORECARDS_JSONL).write_text(json.dumps(sc) + "\n")
        # Write a matching evidence index entry so the VERIFIED scorecard
        # passes the strict evidence index resolution check.
        entry = EvidenceIndex(
            artifact_id="evidence-1",
            run_id=ident["run_id"],
            attempt_id=None,
            media_type=EvidenceMediaType.APPLICATION_JSON,
            sha256=_VALID_HASH,
        )
        (report_dir / EVIDENCE_INDEX_JSONL).write_text(entry.model_dump_json() + "\n")
        result = verify_campaign(report_dir)
        assert result.ok, f"verified scorecard with evidence should pass: {result.failures}"


class TestEscalationRecordIdentityBinding:
    """Layer 16: escalation records are cross-bound to campaign identities."""

    def test_valid_escalation_record_passes(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        er = _make_escalation_dict(ident=ident)
        (report_dir / ESCALATION_RECORDS_JSONL).write_text(json.dumps(er) + "\n")
        result = verify_campaign(report_dir)
        assert result.ok, f"valid escalation record should pass: {result.failures}"

    def test_wrong_campaign_id_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        er = _make_escalation_dict(ident={**ident, "campaign_id": "wrong-campaign"})
        (report_dir / ESCALATION_RECORDS_JSONL).write_text(json.dumps(er) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("campaign_id mismatch" in f.lower() for f in result.failures)

    def test_orphan_escalation_record_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        er = _make_escalation_dict(ident={**ident, "attempt_id": "nonexistent-attempt"})
        (report_dir / ESCALATION_RECORDS_JSONL).write_text(json.dumps(er) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("unknown attempt" in f.lower() for f in result.failures)


class TestSecurityEventIdentityBinding:
    """Layer 17: security event records are cross-bound to campaign identities."""

    def test_valid_security_event_passes(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        se = _make_security_event_dict(ident=ident)
        (report_dir / SECURITY_EVENTS_JSONL).write_text(json.dumps(se) + "\n")
        result = verify_campaign(report_dir)
        assert result.ok, f"valid security event should pass: {result.failures}"

    def test_wrong_campaign_id_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        se = _make_security_event_dict(ident={**ident, "campaign_id": "wrong-campaign"})
        (report_dir / SECURITY_EVENTS_JSONL).write_text(json.dumps(se) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("campaign_id mismatch" in f.lower() for f in result.failures)

    def test_orphan_security_event_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        se = _make_security_event_dict(ident={**ident, "attempt_id": "nonexistent-attempt"})
        (report_dir / SECURITY_EVENTS_JSONL).write_text(json.dumps(se) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("unknown attempt" in f.lower() for f in result.failures)


class TestCorrelatedErrorIdentityBinding:
    """Layer 18: correlated error records are cross-bound to campaign identities."""

    def test_valid_correlated_error_passes(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        ce = _make_correlated_error_dict(ident=ident)
        (report_dir / CORRELATED_ERRORS_JSONL).write_text(json.dumps(ce) + "\n")
        result = verify_campaign(report_dir)
        assert result.ok, f"valid correlated error should pass: {result.failures}"

    def test_wrong_campaign_id_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        ce = _make_correlated_error_dict(ident={**ident, "campaign_id": "wrong-campaign"})
        (report_dir / CORRELATED_ERRORS_JSONL).write_text(json.dumps(ce) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("campaign_id mismatch" in f.lower() for f in result.failures)

    def test_orphan_correlated_error_fails(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        ce = _make_correlated_error_dict(ident={**ident, "attempt_id": "nonexistent-attempt"})
        (report_dir / CORRELATED_ERRORS_JSONL).write_text(json.dumps(ce) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("unknown attempt" in f.lower() for f in result.failures)


class TestResourceObservationCount:
    """Layer 19: resource observation count is verified against attempts."""

    def test_completed_attempt_without_observations_fails(self, tmp_path: Path):
        """A completed attempt with no resource observations is flagged."""
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        # Write observation for a different attempt (second attempt)
        attempts_path = report_dir / ATTEMPTS_JSONL
        lines = attempts_path.read_text().strip().splitlines()
        if len(lines) < 2:
            pytest.skip("need at least 2 attempts")
        second = json.loads(lines[1])
        second_ident = {**ident, "attempt_id": second["attempt_id"], "task_id": second["task_id"]}
        obs = _make_observation_dict(ident=second_ident)
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text(json.dumps(obs) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("no resource observations" in f.lower() for f in result.failures)


class TestVerificationReportPersistence:
    """The canonical child verification report is persisted to the report directory."""

    def test_verification_report_written(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        report_path = report_dir / CAMPAIGN_VERIFICATION_REPORT_JSON
        assert report_path.exists(), "verification report should be persisted"
        persisted = json.loads(report_path.read_text())
        assert persisted["ok"] == result.ok
        assert persisted["campaign_id"] == result.campaign_id
        assert persisted["verified_index_generation_hash"] == result.verified_index_generation_hash

    def test_verification_report_is_canonical_json(self, tmp_path: Path):
        """The persisted report uses canonical JSON (sorted keys, no whitespace)."""
        report_dir = _run_campaign(tmp_path)
        verify_campaign(report_dir)
        report_path = report_dir / CAMPAIGN_VERIFICATION_REPORT_JSON
        raw = report_path.read_text()
        # Canonical JSON has no spaces after separators
        assert ", " not in raw
        assert ": " not in raw
        # Keys are sorted
        parsed = json.loads(raw)
        keys = list(parsed.keys())
        assert keys == sorted(keys)

    def test_verification_report_hash_stable(self, tmp_path: Path):
        """Running verification twice produces byte-identical reports."""
        report_dir = _run_campaign(tmp_path)
        verify_campaign(report_dir)
        first = (report_dir / CAMPAIGN_VERIFICATION_REPORT_JSON).read_text()
        verify_campaign(report_dir)
        second = (report_dir / CAMPAIGN_VERIFICATION_REPORT_JSON).read_text()
        assert first == second
