# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for Ion Phase 2 verifier deepening.

Covers the EF7-INSTRUMENTATION-VERIFIER instructions:

1. Expected-record policy consumption with ONE_PER_INFERENCE and
   ONE_PER_ATTEMPT cardinality enforcement (not just EXACT).
2. Typed campaign assignment parsing and final disposition
   completeness (no missing or extra assignment IDs).
3. Cross-binding of model_variant_id, role, and stage_id against
   assignment records and the stage observation trail.
4. Inference trail count verification from stages.jsonl: one
   resource observation per actual provider inference.
5. Evidence index cross-checks: VERIFIED records' source evidence
   references must resolve to indexed evidence entries.
6. Derived metric recomputation from event records.
7. Layer naming: the resource_observation_count layer verifies
   exact inference-trail counts, not just "at least one".
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
    CAMPAIGN_INDEX_JSONL,
    CAMPAIGN_MANIFEST_JSON,
    CAMPAIGN_VERIFICATION_REPORT_JSON,
    CORRELATED_ERRORS_JSONL,
    EVIDENCE_INDEX_JSONL,
    ESCALATION_RECORDS_JSONL,
    EXPECTED_RECORD_POLICY_JSON,
    MANIFEST_JSON,
    METRICS_JSONL,
    REPORT_CHECKSUM_JSON,
    RESOURCE_OBSERVATIONS_JSONL,
    SECURITY_EVENTS_JSONL,
    STAGES_JSONL,
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
    EscalationOutcome,
    EscalationRecord,
    EvidenceIndex,
    EvidenceMediaType,
    StageKind,
    StageObservation,
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


    def close(self) -> None:
        pass


@dataclass
class _FakeGrader:
    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        return Score(task_id=task.id, passed=True, details=ScoreDetails())


def _make_spec() -> CampaignSpec:
    binding = RoleModelBinding(
        role=ModelRole.PRIMARY,
        model_id="qwen3:8b",
        provider="ollama",
        endpoint="http://192.168.1.2:11434",
        sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
        timeout_seconds=120.0,
        seed_capable=True,
    )
    variant_id = _COHORT_ID[len("cohort-"):] if _COHORT_ID.startswith("cohort-") else _COHORT_ID
    cohort = ModelCohort(
        cohort_id=_COHORT_ID,
        candidate_variant_id=variant_id,
        candidate_role=ModelRole.PRIMARY,
        role_bindings=[binding],
        content_hash=compute_model_cohort_hash(_COHORT_ID, variant_id, ModelRole.PRIMARY, [binding]),
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
        "stage_id": "att-1:model_inference:1",
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


def _write_observations_for_all_attempts(report_dir: Path) -> None:
    """Write one valid resource observation per completed attempt.

    Also writes a matching evidence-index.jsonl entry so VERIFIED
    observations pass the strict evidence index resolution check.
    """
    idents = _all_attempt_identities(report_dir)
    lines = []
    for i, ident in enumerate(idents):
        obs = _make_observation_dict(ident={**ident, "inference_id": f"inf-{i}", "stage_id": f"{ident['attempt_id']}:model_inference:1"})
        lines.append(json.dumps(obs))
    (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text("\n".join(lines) + "\n")
    # Write a matching evidence index entry for VERIFIED observations
    if idents:
        evidence_entries = [
            _make_evidence_index_entry(
                artifact_id="evidence/test.json",
                run_id=idents[0]["run_id"],
                attempt_id=None,
            )
        ]
        _write_evidence_index(report_dir, evidence_entries)


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


def _make_stage_dict(
    *,
    attempt_id: str,
    run_id: str,
    stage_id: str | None = None,
    kind: str = "model_inference",
    agent_role: str = "primary",
    provider: str = "ollama",
    model: str = "qwen3:8b",
) -> dict:
    """Create a valid StageObservation dict."""
    if stage_id is None:
        stage_id = f"{attempt_id}:{kind}:1"
    stage = StageObservation(
        stage_id=stage_id,
        attempt_id=attempt_id,
        run_id=run_id,
        kind=StageKind(kind),
        agent_role=agent_role,
        provider=provider,
        model=model,
    )
    return json.loads(stage.model_dump_json())


def _write_stages_for_all_attempts(report_dir: Path) -> None:
    """Write one model_inference stage per completed attempt."""
    idents = _all_attempt_identities(report_dir)
    lines = []
    for ident in idents:
        stage = _make_stage_dict(
            attempt_id=ident["attempt_id"],
            run_id=ident["run_id"],
            stage_id=f"{ident['attempt_id']}:model_inference:1",
        )
        lines.append(json.dumps(stage))
    (report_dir / STAGES_JSONL).write_text("\n".join(lines) + "\n")


def _make_evidence_index_entry(
    *,
    artifact_id: str,
    run_id: str,
    attempt_id: str | None = None,
    sha256: str | None = None,
) -> dict:
    """Create a valid EvidenceIndex dict."""
    entry = EvidenceIndex(
        artifact_id=artifact_id,
        run_id=run_id,
        attempt_id=attempt_id,
        media_type=EvidenceMediaType.APPLICATION_JSON,
        sha256=sha256 or _VALID_HASH,
    )
    return json.loads(entry.model_dump_json())


def _write_evidence_index(report_dir: Path, entries: list[dict]) -> None:
    """Write evidence index entries to the report directory."""
    lines = [json.dumps(e) for e in entries]
    (report_dir / EVIDENCE_INDEX_JSONL).write_text("\n".join(lines) + "\n")


def _make_escalation_dict(**kwargs) -> dict:
    """Create a valid EscalationRecord dict matching the campaign identity."""
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
        "model_variant_id": "qwen3:8b",
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


# ---------------------------------------------------------------------------
# Instruction 2: Typed campaign assignment parsing and disposition completeness
# ---------------------------------------------------------------------------


class TestTypedAssignmentParsingAndDispositionCompleteness:
    """Layer 6: campaign assignments are parsed as typed models and final
    dispositions contain exactly the expected assignment set."""

    def test_assignments_parse_as_typed_models(self, tmp_path: Path):
        """The verifier parses campaign-assignments.jsonl as CampaignAssignment models."""
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        # A valid campaign with typed assignments should pass cell_coverage
        assert "cell_coverage" in result.checked_layers
        assert result.ok, f"valid campaign should pass: {result.failures}"

    def test_extra_assignment_in_disposition_fails(self, tmp_path: Path):
        """An extra assignment ID in the final disposition that is not in
        the campaign assignments is rejected."""
        report_dir = _run_campaign(tmp_path)
        # Read the final index generation and add a fake assignment disposition
        index_path = report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        assert len(lines) >= 2
        final_gen = json.loads(lines[-1])
        # Add a fake assignment ID that is not in campaign-assignments.jsonl
        fake_disposition = {
            "assignment_id": "fake-assignment-not-in-campaign",
            "disposition": "effective",
        }
        final_gen["assignment_dispositions"].append(fake_disposition)
        # Recompute the content hash for the tampered generation
        from g8e_evals.index import compute_index_generation_hash
        final_gen["content_hash"] = compute_index_generation_hash(
            generation_number=final_gen["generation_number"],
            parent_generation_hash=final_gen["parent_generation_hash"],
            creation_reason=final_gen["creation_reason"],
            report_checksums=final_gen["report_checksums"],
            assignment_dispositions=[
                {"assignment_id": d["assignment_id"], "disposition": d["disposition"]}
                for d in final_gen["assignment_dispositions"]
            ],
        )
        lines[-1] = json.dumps(final_gen)
        index_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("extra assignment" in f.lower() for f in result.failures), (
            f"expected extra assignment failure, got: {result.failures}"
        )

    def test_missing_assignment_in_disposition_fails(self, tmp_path: Path):
        """A missing assignment ID from the final disposition is rejected."""
        report_dir = _run_campaign(tmp_path)
        # Remove one assignment from the final disposition
        index_path = report_dir / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        assert len(lines) >= 2
        final_gen = json.loads(lines[-1])
        # Remove the first assignment disposition
        if len(final_gen["assignment_dispositions"]) > 1:
            final_gen["assignment_dispositions"].pop(0)
        else:
            pytest.skip("need at least 2 assignments")
        from g8e_evals.index import compute_index_generation_hash
        final_gen["content_hash"] = compute_index_generation_hash(
            generation_number=final_gen["generation_number"],
            parent_generation_hash=final_gen["parent_generation_hash"],
            creation_reason=final_gen["creation_reason"],
            report_checksums=final_gen["report_checksums"],
            assignment_dispositions=[
                {"assignment_id": d["assignment_id"], "disposition": d["disposition"]}
                for d in final_gen["assignment_dispositions"]
            ],
        )
        lines[-1] = json.dumps(final_gen)
        index_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("missing" in f.lower() and "assignment" in f.lower() for f in result.failures), (
            f"expected missing assignment failure, got: {result.failures}"
        )


# ---------------------------------------------------------------------------
# Instruction 1: ONE_PER_INFERENCE and ONE_PER_ATTEMPT cardinality enforcement
# ---------------------------------------------------------------------------


class TestOnePerInferenceCardinality:
    """Layer 13/14: ONE_PER_INFERENCE cardinality is enforced when stages.jsonl
    provides the inference trail."""

    def test_one_per_inference_with_matching_stages_passes(self, tmp_path: Path):
        """ONE_PER_INFERENCE cardinality passes when observation count
        matches the model_inference stage count."""
        report_dir = _run_campaign(tmp_path)
        _write_stages_for_all_attempts(report_dir)
        _write_observations_for_all_attempts(report_dir)
        policy = _make_policy(entries=[
            {"file_name": RESOURCE_OBSERVATIONS_JSONL, "applicability": "required", "cardinality_rule": "one_per_inference"},
            {"file_name": TOOL_CALL_SCORECARDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": ESCALATION_RECORDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": SECURITY_EVENTS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": CORRELATED_ERRORS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
        ])
        _write_policy(report_dir, policy)
        result = verify_campaign(report_dir)
        assert result.ok, f"matching one_per_inference should pass: {result.failures}"

    def test_one_per_inference_with_too_few_observations_fails(self, tmp_path: Path):
        """ONE_PER_INFERENCE cardinality fails when there are fewer
        observations than model_inference stages."""
        report_dir = _run_campaign(tmp_path)
        _write_stages_for_all_attempts(report_dir)
        # Write only one observation for the first attempt (but there are multiple attempts)
        idents = _all_attempt_identities(report_dir)
        if len(idents) < 2:
            pytest.skip("need at least 2 attempts")
        ident = idents[0]
        obs = _make_observation_dict(ident={**ident, "inference_id": "inf-0", "stage_id": f"{ident['attempt_id']}:model_inference:1"})
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text(json.dumps(obs) + "\n")
        policy = _make_policy(entries=[
            {"file_name": RESOURCE_OBSERVATIONS_JSONL, "applicability": "required", "cardinality_rule": "one_per_inference"},
            {"file_name": TOOL_CALL_SCORECARDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": ESCALATION_RECORDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": SECURITY_EVENTS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": CORRELATED_ERRORS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
        ])
        _write_policy(report_dir, policy)
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("one_per_inference" in f.lower() or "cardinality" in f.lower() for f in result.failures), (
            f"expected cardinality failure, got: {result.failures}"
        )

    def test_one_per_inference_with_too_many_observations_fails(self, tmp_path: Path):
        """ONE_PER_INFERENCE cardinality fails when there are more
        observations than model_inference stages."""
        report_dir = _run_campaign(tmp_path)
        _write_stages_for_all_attempts(report_dir)
        # Write one extra observation
        idents = _all_attempt_identities(report_dir)
        ident = idents[0]
        obs = _make_observation_dict(ident={**ident, "inference_id": "inf-extra", "stage_id": f"{ident['attempt_id']}:model_inference:1"})
        existing = (report_dir / RESOURCE_OBSERVATIONS_JSONL).read_text()
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text(existing + json.dumps(obs) + "\n")
        policy = _make_policy(entries=[
            {"file_name": RESOURCE_OBSERVATIONS_JSONL, "applicability": "required", "cardinality_rule": "one_per_inference"},
            {"file_name": TOOL_CALL_SCORECARDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": ESCALATION_RECORDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": SECURITY_EVENTS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": CORRELATED_ERRORS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
        ])
        _write_policy(report_dir, policy)
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("one_per_inference" in f.lower() or "cardinality" in f.lower() for f in result.failures), (
            f"expected cardinality failure, got: {result.failures}"
        )


class TestOnePerAttemptCardinality:
    """Layer 13: ONE_PER_ATTEMPT cardinality is enforced."""

    def test_one_per_attempt_with_matching_count_passes(self, tmp_path: Path):
        """ONE_PER_ATTEMPT cardinality passes when record count matches
        the completed attempt count."""
        report_dir = _run_campaign(tmp_path)
        idents = _all_attempt_identities(report_dir)
        # Write one escalation record per attempt
        lines = []
        for ident in idents:
            er = _make_escalation_dict(ident=ident)
            lines.append(json.dumps(er))
        (report_dir / ESCALATION_RECORDS_JSONL).write_text("\n".join(lines) + "\n")
        policy = _make_policy(entries=[
            {"file_name": RESOURCE_OBSERVATIONS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": TOOL_CALL_SCORECARDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": ESCALATION_RECORDS_JSONL, "applicability": "required", "cardinality_rule": "one_per_attempt"},
            {"file_name": SECURITY_EVENTS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": CORRELATED_ERRORS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
        ])
        _write_policy(report_dir, policy)
        result = verify_campaign(report_dir)
        assert result.ok, f"matching one_per_attempt should pass: {result.failures}"

    def test_one_per_attempt_with_wrong_count_fails(self, tmp_path: Path):
        """ONE_PER_ATTEMPT cardinality fails when record count does not
        match the completed attempt count."""
        report_dir = _run_campaign(tmp_path)
        idents = _all_attempt_identities(report_dir)
        if len(idents) < 2:
            pytest.skip("need at least 2 attempts")
        # Write only one record when there are multiple attempts
        er = _make_escalation_dict(ident=idents[0])
        (report_dir / ESCALATION_RECORDS_JSONL).write_text(json.dumps(er) + "\n")
        policy = _make_policy(entries=[
            {"file_name": RESOURCE_OBSERVATIONS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": TOOL_CALL_SCORECARDS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
            {"file_name": ESCALATION_RECORDS_JSONL, "applicability": "required", "cardinality_rule": "one_per_attempt"},
            {"file_name": SECURITY_EVENTS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_attempt"},
            {"file_name": CORRELATED_ERRORS_JSONL, "applicability": "optional", "cardinality_rule": "one_per_inference"},
        ])
        _write_policy(report_dir, policy)
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("one_per_attempt" in f.lower() or "cardinality" in f.lower() for f in result.failures), (
            f"expected cardinality failure, got: {result.failures}"
        )


# ---------------------------------------------------------------------------
# Instruction 3: Cross-binding model_variant_id, role, and stage_id
# ---------------------------------------------------------------------------


class TestModelVariantAndRoleCrossBinding:
    """Layer 14: resource observations are cross-bound to model_variant_id
    and role against assignment records."""

    def test_wrong_model_variant_id_fails(self, tmp_path: Path):
        """A resource observation with a model_variant_id that does not match
        the assignment's cohort is rejected."""
        report_dir = _run_campaign(tmp_path)
        _write_observations_for_all_attempts(report_dir)
        # Tamper one observation's model_variant_id to a wrong value
        obs_path = report_dir / RESOURCE_OBSERVATIONS_JSONL
        lines = obs_path.read_text().strip().splitlines()
        first = json.loads(lines[0])
        first["model_variant_id"] = "wrong-variant-id"
        lines[0] = json.dumps(first)
        obs_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("model_variant_id" in f.lower() for f in result.failures), (
            f"expected model_variant_id failure, got: {result.failures}"
        )

    def test_wrong_role_fails(self, tmp_path: Path):
        """A resource observation with a role that does not match the
        assignment's role is rejected."""
        report_dir = _run_campaign(tmp_path)
        _write_observations_for_all_attempts(report_dir)
        # Tamper one observation's role to a wrong value
        obs_path = report_dir / RESOURCE_OBSERVATIONS_JSONL
        lines = obs_path.read_text().strip().splitlines()
        first = json.loads(lines[0])
        first["role"] = "assistant"
        lines[0] = json.dumps(first)
        obs_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("role" in f.lower() and "mismatch" in f.lower() for f in result.failures), (
            f"expected role mismatch failure, got: {result.failures}"
        )

    def test_tier_fitness_allows_bound_baseline_model_calls(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        _write_stages_for_all_attempts(report_dir)
        _write_observations_for_all_attempts(report_dir)
        manifest_path = report_dir / MANIFEST_JSON
        manifest = json.loads(manifest_path.read_text())
        manifest["campaign_binding"] = {
            "campaign_id": _CAMPAIGN_ID,
            "campaign_revision": "test-revision",
            "report_role": "single",
            "campaign_profile_hash": _VALID_HASH,
            "model_registry_hash": _VALID_HASH,
            "required_record_policy_hash": _VALID_HASH,
            "orchestrator_hardware_identity": "linux/amd64/cpu",
            "orchestrator_environment_stratum": "single-machine",
            "provider_hardware_identity": "unavailable",
            "provider_environment_stratum": "remote-ollama",
            "track_arm_assignments": [
                {"track": "direct", "arm_id": "direct"},
                {"track": "tier_fitness", "arm_id": "ensemble_ungoverned"},
            ],
        }
        manifest_path.write_text(json.dumps(manifest))

        stage_path = report_dir / STAGES_JSONL
        stages = [json.loads(line) for line in stage_path.read_text().splitlines() if line]
        observation_path = report_dir / RESOURCE_OBSERVATIONS_JSONL
        observations = [
            json.loads(line) for line in observation_path.read_text().splitlines() if line
        ]
        attempts = [
            json.loads(line)
            for line in (report_dir / ATTEMPTS_JSONL).read_text().splitlines()
            if line
        ]
        tier_fitness_attempt_id = next(
            attempt["attempt_id"]
            for attempt in attempts
            if attempt["arm_id"] == "ensemble_ungoverned"
        )
        target_observation = next(
            observation
            for observation in observations
            if observation["attempt_id"] == tier_fitness_attempt_id
        )
        baseline_stage_id = f"{target_observation['attempt_id']}:model_inference:baseline"
        stages.append(
            _make_stage_dict(
                attempt_id=target_observation["attempt_id"],
                run_id=target_observation["run_id"],
                stage_id=baseline_stage_id,
                agent_role="lite",
                model="smollm2:360m",
            )
        )
        observations.append(
            _make_observation_dict(
                ident={
                    "campaign_id": target_observation["campaign_id"],
                    "child_id": target_observation["child_id"],
                    "run_id": target_observation["run_id"],
                    "task_id": target_observation["task_id"],
                    "assignment_id": target_observation["assignment_id"],
                    "attempt_id": target_observation["attempt_id"],
                    "inference_id": "inf-baseline",
                    "stage_id": baseline_stage_id,
                },
                role="lite",
                model_variant_id="smollm2:360m",
            )
        )
        stage_path.write_text("\n".join(json.dumps(stage) for stage in stages) + "\n")
        observation_path.write_text(
            "\n".join(json.dumps(observation) for observation in observations) + "\n"
        )

        result = verify_campaign(report_dir)

        assert result.ok, result.failures


class TestStageIdCrossBinding:
    """Layer 14: resource observations' stage_id is cross-bound to the
    stage observation trail in stages.jsonl."""

    def test_stage_id_not_in_stages_fails(self, tmp_path: Path):
        """A resource observation with a stage_id that does not exist in
        stages.jsonl is rejected."""
        report_dir = _run_campaign(tmp_path)
        _write_stages_for_all_attempts(report_dir)
        _write_observations_for_all_attempts(report_dir)
        # Tamper one observation's stage_id to a non-existent value
        obs_path = report_dir / RESOURCE_OBSERVATIONS_JSONL
        lines = obs_path.read_text().strip().splitlines()
        first = json.loads(lines[0])
        first["stage_id"] = "nonexistent-stage-id"
        lines[0] = json.dumps(first)
        obs_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("stage_id" in f.lower() for f in result.failures), (
            f"expected stage_id failure, got: {result.failures}"
        )

    def test_valid_stage_id_passes(self, tmp_path: Path):
        """A resource observation with a stage_id that exists in stages.jsonl passes."""
        report_dir = _run_campaign(tmp_path)
        _write_stages_for_all_attempts(report_dir)
        _write_observations_for_all_attempts(report_dir)
        result = verify_campaign(report_dir)
        assert result.ok, f"valid stage_id should pass: {result.failures}"


# ---------------------------------------------------------------------------
# Instruction 4: Inference trail count verification from stages.jsonl
# ---------------------------------------------------------------------------


class TestInferenceTrailCountVerification:
    """Layer 19: resource observation count is verified against the
    inference trail from stages.jsonl, not just 'at least one per attempt'."""

    def test_observation_count_matches_inference_trail_passes(self, tmp_path: Path):
        """When stages.jsonl has N model_inference stages for an attempt,
        there must be exactly N resource observations for that attempt."""
        report_dir = _run_campaign(tmp_path)
        _write_stages_for_all_attempts(report_dir)
        _write_observations_for_all_attempts(report_dir)
        result = verify_campaign(report_dir)
        assert result.ok, f"matching inference trail should pass: {result.failures}"

    def test_extra_observation_for_attempt_fails(self, tmp_path: Path):
        """An extra resource observation for an attempt (beyond the
        inference trail) is rejected."""
        report_dir = _run_campaign(tmp_path)
        _write_stages_for_all_attempts(report_dir)
        _write_observations_for_all_attempts(report_dir)
        # Add an extra observation for the first attempt
        idents = _all_attempt_identities(report_dir)
        ident = idents[0]
        extra = _make_observation_dict(
            ident={**ident, "inference_id": "inf-extra", "stage_id": f"{ident['attempt_id']}:model_inference:1"},
        )
        existing = (report_dir / RESOURCE_OBSERVATIONS_JSONL).read_text()
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text(existing + json.dumps(extra) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any(
            ("inference" in f.lower() and "count" in f.lower())
            or "duplicate" in f.lower()
            or ("observation" in f.lower() and "stage" in f.lower())
            for f in result.failures
        ), (
            f"expected inference count or duplicate observation failure, got: {result.failures}"
        )

    def test_missing_observation_for_inference_fails(self, tmp_path: Path):
        """A missing resource observation for an inference in the trail is rejected."""
        report_dir = _run_campaign(tmp_path)
        _write_stages_for_all_attempts(report_dir)
        # Write observations for all but the last attempt
        idents = _all_attempt_identities(report_dir)
        if len(idents) < 2:
            pytest.skip("need at least 2 attempts")
        lines = []
        for i, ident in enumerate(idents[:-1]):
            obs = _make_observation_dict(
                ident={**ident, "inference_id": f"inf-{i}", "stage_id": f"{ident['attempt_id']}:model_inference:1"},
            )
            lines.append(json.dumps(obs))
        (report_dir / RESOURCE_OBSERVATIONS_JSONL).write_text("\n".join(lines) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        # Should fail because the last attempt has a stage but no observation
        assert any("no resource observations" in f.lower() or "inference" in f.lower() for f in result.failures), (
            f"expected missing observation failure, got: {result.failures}"
        )


# ---------------------------------------------------------------------------
# Instruction 3 (continued): Evidence index cross-checks
# ---------------------------------------------------------------------------


class TestEvidenceIndexCrossCheck:
    """VERIFIED records' source_evidence_refs must resolve to indexed
    evidence entries in evidence-index.jsonl."""

    def test_verified_observation_with_unindexed_evidence_fails(self, tmp_path: Path):
        """A VERIFIED resource observation whose source_evidence_refs do not
        resolve to evidence-index.jsonl entries is rejected."""
        report_dir = _run_campaign(tmp_path)
        _write_observations_for_all_attempts(report_dir)
        # Write an evidence index that does NOT contain the referenced artifact
        idents = _all_attempt_identities(report_dir)
        evidence_entries = [
            _make_evidence_index_entry(
                artifact_id="evidence/different.json",
                run_id=idents[0]["run_id"],
                attempt_id=idents[0]["attempt_id"],
            )
        ]
        _write_evidence_index(report_dir, evidence_entries)
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("evidence" in f.lower() and "index" in f.lower() for f in result.failures), (
            f"expected evidence index failure, got: {result.failures}"
        )

    def test_verified_observation_with_indexed_evidence_passes(self, tmp_path: Path):
        """A VERIFIED resource observation whose source_evidence_refs resolve
        to evidence-index.jsonl entries passes."""
        report_dir = _run_campaign(tmp_path)
        _write_observations_for_all_attempts(report_dir)
        # Write a single evidence index entry that all observations reference
        idents = _all_attempt_identities(report_dir)
        evidence_entries = [
            _make_evidence_index_entry(
                artifact_id="evidence/test.json",
                run_id=idents[0]["run_id"],
                attempt_id=None,
            )
        ]
        _write_evidence_index(report_dir, evidence_entries)
        result = verify_campaign(report_dir)
        assert result.ok, f"indexed evidence should pass: {result.failures}"

    def test_verified_observation_evidence_sha256_mismatch_fails(self, tmp_path: Path):
        """A VERIFIED resource observation whose source_evidence_sha256 does not
        match the evidence index entry's sha256 is rejected."""
        report_dir = _run_campaign(tmp_path)
        _write_observations_for_all_attempts(report_dir)
        idents = _all_attempt_identities(report_dir)
        # Write evidence index with a different sha256 than the observation's
        evidence_entries = [
            _make_evidence_index_entry(
                artifact_id="evidence/test.json",
                run_id=idents[0]["run_id"],
                attempt_id=idents[0]["attempt_id"],
                sha256="b" * 64,  # Different from _VALID_HASH
            )
        ]
        _write_evidence_index(report_dir, evidence_entries)
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("sha256" in f.lower() and "mismatch" in f.lower() for f in result.failures), (
            f"expected sha256 mismatch failure, got: {result.failures}"
        )


# ---------------------------------------------------------------------------
# Instruction 6: Derived metric recomputation from event records
# ---------------------------------------------------------------------------


class TestDerivedMetricRecomputation:
    """Layer 8+: derived metrics are recomputed from event records and
    metrics with mismatched numerator, denominator, or unit are rejected."""

    def test_metric_with_wrong_denominator_fails(self, tmp_path: Path):
        """A metric observation with a denominator_contribution that does not
        match the recomputed value from event records is rejected."""
        report_dir = _run_campaign(tmp_path)
        # Tamper a metric's denominator_contribution to a wrong value
        metrics_path = report_dir / METRICS_JSONL
        lines = metrics_path.read_text().strip().splitlines()
        first = json.loads(lines[0])
        first["denominator_contribution"] = 99  # Wrong value
        lines[0] = json.dumps(first)
        metrics_path.write_text("\n".join(lines) + "\n")
        # Recompute the report checksum so the standalone validator does not
        # fail on checksum mismatch (we want the denominator check to fire)
        from g8e_evals.report.validate import _compute_report_checksum
        new_checksum = _compute_report_checksum(report_dir)
        checksum_path = report_dir / REPORT_CHECKSUM_JSON
        if checksum_path.exists():
            checksum_data = json.loads(checksum_path.read_text())
            checksum_data["checksum"] = new_checksum
            checksum_path.write_text(json.dumps(checksum_data))
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("denominator" in f.lower() for f in result.failures), (
            f"expected denominator failure, got: {result.failures}"
        )

    def test_metric_with_valid_denominator_passes(self, tmp_path: Path):
        """A metric observation with a correct denominator_contribution passes."""
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert result.ok, f"valid denominator should pass: {result.failures}"


# ---------------------------------------------------------------------------
# Instruction 7: Layer naming reflects the proof
# ---------------------------------------------------------------------------


class TestLayerNaming:
    """The resource_observation_count layer name reflects that it verifies
    exact inference-trail counts, not just 'at least one per attempt'."""

    def test_resource_observation_count_layer_present(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert "resource_observation_count" in result.checked_layers

    def test_layer_name_in_verification_report(self, tmp_path: Path):
        """The verification report includes the resource_observation_count layer."""
        report_dir = _run_campaign(tmp_path)
        verify_campaign(report_dir)
        report_path = report_dir / CAMPAIGN_VERIFICATION_REPORT_JSON
        persisted = json.loads(report_path.read_text())
        assert "resource_observation_count" in persisted["checked_layers"]
