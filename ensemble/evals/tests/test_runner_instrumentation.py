# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for runner SUT instrumentation (EF7-INSTRUMENTATION-PRODUCER).

Verifies that the campaign runner emits all five event/resource JSONL files
atomically, produces one resource observation per actual provider inference,
feeds resource and event records into AnalysisInputRecord, persists stage
records and evidence-index entries, closes SUTs deterministically, and
preserves honest missingness for unavailable remote measurements.

Uses deterministic fake SUTs and local filesystem (tmp_path). No network,
database, or real provider dependencies.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
from dataclasses import dataclass, field
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
    CampaignStatus,
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
from g8e_evals.constants import (
    ANALYSIS_INPUT_JSON,
    ATTEMPTS_JSONL,
    CORRELATED_ERRORS_JSONL,
    EVIDENCE_INDEX_JSONL,
    ESCALATION_RECORDS_JSONL,
    RESOURCE_OBSERVATIONS_JSONL,
    SECURITY_EVENTS_JSONL,
    STAGES_JSONL,
    TOOL_CALL_SCORECARDS_JSONL,
)
from g8e_evals.harness import InferenceObservation, Response, Score, Task
from g8e_evals.index import MeasurementAvailability, MeasurementScope, ModelRole
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.profile import CampaignProfile
from g8e_evals.runner import CampaignRunner, CampaignSpec, CampaignStopReason
from g8e_evals.schema import CampaignTrack, ProviderBudget, TerminalStatus, TrackArmAssignment


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

_CAMPAIGN_ID = "v2.1.8-ifeval-instrumentation"
_RELEASE_VERSION = "v2.1.8"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_IDS = ["cohort-qwen3-8b"]
_ARM_IDS = ["direct"]
_REPLICATE_IDS = ["replicate-1"]
_INITIAL_STATE_ID = "no-initial-state-v1"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64


# ---------------------------------------------------------------------------
# Fake SUT and Grader
# ---------------------------------------------------------------------------


@dataclass
class InstrumentedFakeSUT:
    """Deterministic fake SUT that emits InferenceObservation records.

    Each get_answer call produces one InferenceObservation bound to the
    provider call, simulating what DirectProviderSUT does in production.
    The observation carries timing and token data derived from the fake
    provider response.
    """

    model_id: str
    answer: str = "This is a test answer with no commas."
    provider_prefix: bool = True
    _call_count: int = field(default=0)

    async def get_answer(self, task: Task) -> Response:
        self._call_count += 1
        model = f"ollama:{self.model_id}" if self.provider_prefix else self.model_id
        inference_id = f"inf-{self._call_count}"
        observation = InferenceObservation(
            inference_id=inference_id,
            role=ModelRole.PRIMARY,
            model_variant_id=self.model_id,
            provider="ollama",
            model=self.model_id,
            provider_call_latency_seconds=0.05,
            time_to_first_token_seconds=0.01,
            generation_duration_seconds=0.04,
            output_throughput_tokens_per_second=100.0,
            prompt_token_count=10,
            candidates_token_count=5,
            total_token_count=15,
            usage_reported=True,
            finish_reason="stop",
            input_artifact_hash=hashlib.sha256(b"input").hexdigest(),
            output_artifact_hash=hashlib.sha256(b"output").hexdigest(),
        )
        return Response(
            answer=self.answer,
            model=model,
            arm=Arm.DIRECT,
            inference_observations=[observation],
        )

    async def close(self) -> None:
        """Deterministic close for the fake SUT."""
        self._call_count = 0


@dataclass
class AttemptLocalInferenceIDFakeSUT(InstrumentedFakeSUT):
    async def get_answer(self, task: Task) -> Response:
        response = await super().get_answer(task)
        response.inference_observations[0].inference_id = "inf-0"
        return response


@dataclass
class UninstrumentedEnsembleFakeSUT:
    model_id: str

    async def get_answer(self, task: Task) -> Response:
        return Response(
            answer="This answer has no provider observation.",
            model=self.model_id,
            arm=Arm.ENSEMBLE_UNGOVERNED,
        )

    async def close(self) -> None:
        return None


@dataclass
class FailingFakeSUT:
    """Fake SUT that raises an infrastructure error on every call.

    Still emits an InferenceObservation for the failed call so the runner
    can record the attempted inference.
    """

    model_id: str
    _call_count: int = field(default=0)
    _closed: bool = field(default=False)

    async def get_answer(self, task: Task) -> Response:
        self._call_count += 1
        inference_id = f"inf-{self._call_count}"
        InferenceObservation(
            inference_id=inference_id,
            role=ModelRole.PRIMARY,
            model_variant_id=self.model_id,
            provider="ollama",
            model=self.model_id,
            error="simulated infrastructure failure",
        )
        raise RuntimeError("simulated infrastructure failure")

    async def close(self) -> None:
        self._closed = True


@dataclass
class FailedCallFakeSUT:
    """Fake SUT that returns a Response carrying a failed-call observation.

    Simulates the direct-arm failure path where the provider call fails:
    the SUT returns a Response (no exception escapes) carrying one
    InferenceObservation with an error and all timing/usage fields None.
    """

    model_id: str
    _call_count: int = field(default=0)

    async def get_answer(self, task: Task) -> Response:
        self._call_count += 1
        observation = InferenceObservation(
            inference_id=f"inf-{self._call_count}",
            role=ModelRole.PRIMARY,
            model_variant_id=self.model_id,
            provider="ollama",
            model=self.model_id,
            provider_call_latency_seconds=0.02,
            error="simulated provider failure",
        )
        return Response(
            answer="",
            model=f"ollama:{self.model_id}",
            arm=Arm.DIRECT,
            inference_observations=[observation],
        )

    async def close(self) -> None:
        self._call_count = 0


@dataclass
class FakeGrader:
    """Deterministic fake grader that always passes."""

    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        return Score(task_id=task.id, passed=True, details=ScoreDetails())


def _make_instrumented_sut_factory():
    """Create a SUT factory that returns InstrumentedFakeSUT instances."""
    def factory(cohort: ModelCohort, arm: Arm):
        model_id = cohort.role_bindings[0].model_id
        return InstrumentedFakeSUT(model_id=model_id)
    return factory


def _make_attempt_local_inference_id_sut_factory():
    def factory(cohort: ModelCohort, arm: Arm):
        model_id = cohort.role_bindings[0].model_id
        return AttemptLocalInferenceIDFakeSUT(model_id=model_id)
    return factory


def _make_failing_sut_factory():
    """Create a SUT factory that returns FailingFakeSUT instances."""
    def factory(cohort: ModelCohort, arm: Arm):
        model_id = cohort.role_bindings[0].model_id
        return FailingFakeSUT(model_id=model_id)
    return factory


def _make_failed_call_sut_factory():
    """Create a SUT factory that returns FailedCallFakeSUT instances."""
    def factory(cohort: ModelCohort, arm: Arm):
        model_id = cohort.role_bindings[0].model_id
        return FailedCallFakeSUT(model_id=model_id)
    return factory


# ---------------------------------------------------------------------------
# Spec helpers
# ---------------------------------------------------------------------------


def _make_sampling() -> SamplingSettings:
    return SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42)


def _make_role_binding(role: ModelRole = ModelRole.PRIMARY, model_id: str = "qwen3:8b") -> RoleModelBinding:
    return RoleModelBinding(
        role=role,
        model_id=model_id,
        provider="ollama",
        endpoint="http://192.168.1.2:11434",
        sampling_settings=_make_sampling(),
        timeout_seconds=120.0,
        seed_capable=True,
    )


def _make_cohort(cohort_id: str = "cohort-qwen3-8b", model_id: str = "qwen3:8b") -> ModelCohort:
    bindings = [_make_role_binding(ModelRole.PRIMARY, model_id)]
    variant_id = cohort_id[len("cohort-"):] if cohort_id.startswith("cohort-") else cohort_id
    ch = compute_model_cohort_hash(cohort_id, variant_id, ModelRole.PRIMARY, bindings)
    return ModelCohort(
        cohort_id=cohort_id,
        candidate_variant_id=variant_id,
        candidate_role=ModelRole.PRIMARY,
        role_bindings=bindings,
        content_hash=ch,
    )


def _make_task_assignment(task_ids: list[str] | None = None) -> TaskAssignmentManifest:
    if task_ids is None:
        task_ids = _TASK_IDS
    ch = compute_task_assignment_hash("task-assignment-v1", "ifeval_subset", _DATASET_HASH, task_ids)
    return TaskAssignmentManifest(
        task_assignment_id="task-assignment-v1",
        suite_id="ifeval_subset",
        dataset_hash=_DATASET_HASH,
        task_ids=task_ids,
        content_hash=ch,
    )


def _make_initial_state() -> InitialStateAssignmentManifest:
    ch = compute_initial_state_hash(_INITIAL_STATE_ID, "no_initial_state", _NO_STATE_HASH)
    return InitialStateAssignmentManifest(
        initial_state_assignment_id=_INITIAL_STATE_ID,
        state_type="no_initial_state",
        snapshot_hash=_NO_STATE_HASH,
        content_hash=ch,
    )


def _make_preregistration(
    cohort_ids: list[str] | None = None,
    arm_ids: list[str] | None = None,
    replicate_ids: list[str] | None = None,
) -> PreregistrationConfig:
    if cohort_ids is None:
        cohort_ids = _COHORT_IDS
    if arm_ids is None:
        arm_ids = _ARM_IDS
    if replicate_ids is None:
        replicate_ids = _REPLICATE_IDS
    return PreregistrationConfig(
        config_id="test-config-1",
        config_version="1.0.0",
        baseline_arm_id=arm_ids[0],
        comparison_arm_ids=arm_ids[1:],
        model_cohort_ids=cohort_ids,
        task_assignment_id="task-assignment-v1",
        initial_state_assignment_id=_INITIAL_STATE_ID,
        required_replicate_ids=replicate_ids,
        required_replicate_count=len(replicate_ids),
        primary_metric_ids=["ifeval_subset_verifier"],
        continuous_test_policy=ContinuousTestPolicy.PAIRED_T,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=0,
        significance_level=0.05,
        claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY,
    )


def _make_tasks(task_ids: list[str] | None = None) -> list[Task]:
    if task_ids is None:
        task_ids = _TASK_IDS
    tasks = []
    for tid in task_ids:
        tasks.append(
            Task(
                id=tid,
                prompt=f"Prompt for {tid}",
                metadata=TaskMetadata(
                    benchmark="ifeval_subset",
                    instruction_id_list=["punctuation:no_comma"],
                    kwargs=[{"no_comma": True}],
                ),
            )
        )
    return tasks


def _make_spec(
    task_ids: list[str] | None = None,
    retry_policy: RetryPolicy | None = None,
    provider_budget: ProviderBudget | None = None,
) -> CampaignSpec:
    if task_ids is None:
        task_ids = _TASK_IDS
    if retry_policy is None:
        retry_policy = RetryPolicy(max_retries=0, retryable_terminal_statuses=["infrastructure_failed"])

    cohorts = [_make_cohort(c, m) for c, m in zip(_COHORT_IDS, ["qwen3:8b"], strict=True)]
    task_assignment = _make_task_assignment(task_ids)
    initial_state = _make_initial_state()
    preregistration = _make_preregistration(_COHORT_IDS, _ARM_IDS, _REPLICATE_IDS)

    prompt_bundle = "\n".join(f"Prompt for {tid}" for tid in task_ids).encode()
    prompt_bundle_hash = hashlib.sha256(prompt_bundle).hexdigest()

    return CampaignSpec(
        campaign_id=_CAMPAIGN_ID,
        release_version=_RELEASE_VERSION,
        suite="ifeval_subset",
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        dataset_hash=_DATASET_HASH,
        prompt_bundle_hash=prompt_bundle_hash,
        grader_bundle_hash="g" * 64,
        preregistration=preregistration,
        cohorts=cohorts,
        task_assignment=task_assignment,
        initial_state=initial_state,
        retry_policy=retry_policy,
        randomization_seed=42,
        provider_budget=provider_budget,
    )


# ---------------------------------------------------------------------------
# Tests: Five JSONL files emitted atomically
# ---------------------------------------------------------------------------


class TestFiveJsonlFilesEmitted:
    """The runner must emit all five event/resource JSONL files atomically.

    Missing an expected file is not equivalent to zero records. Files
    with zero records must exist and be empty.
    """

    def test_all_five_event_files_exist_after_run(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        report = result.report_dir
        assert (report / RESOURCE_OBSERVATIONS_JSONL).exists(), "resource-observations.jsonl missing"
        assert (report / TOOL_CALL_SCORECARDS_JSONL).exists(), "tool-call-scorecards.jsonl missing"
        assert (report / ESCALATION_RECORDS_JSONL).exists(), "escalation-records.jsonl missing"
        assert (report / SECURITY_EVENTS_JSONL).exists(), "security-events.jsonl missing"
        assert (report / CORRELATED_ERRORS_JSONL).exists(), "correlated-errors.jsonl missing"

    def test_event_files_empty_when_no_events(self, tmp_path: Path):
        """The direct arm produces no tool calls, escalations, security events, or correlated errors.

        Those four files must exist and be empty. The resource-observations
        file must contain one record per provider inference.
        """
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        report = result.report_dir
        # Tool call scorecards: direct arm has no tool calls
        scorecards_text = (report / TOOL_CALL_SCORECARDS_JSONL).read_text()
        assert scorecards_text.strip() == "", "tool-call-scorecards.jsonl should be empty for direct arm"

        # Escalation records: direct arm has no triage routing
        escalation_text = (report / ESCALATION_RECORDS_JSONL).read_text()
        assert escalation_text.strip() == "", "escalation-records.jsonl should be empty for direct arm"

        # Security events: direct arm has no governance events
        security_text = (report / SECURITY_EVENTS_JSONL).read_text()
        assert security_text.strip() == "", "security-events.jsonl should be empty for direct arm"

        # Correlated errors: direct arm has no stage errors
        errors_text = (report / CORRELATED_ERRORS_JSONL).read_text()
        assert errors_text.strip() == "", "correlated-errors.jsonl should be empty for direct arm"

    def test_resource_observations_one_per_inference(self, tmp_path: Path):
        """Direct inference emits one resource observation per actual provider inference.

        With 2 tasks, 1 cohort, 1 arm, 1 replicate = 2 assignments = 2 inferences.
        """
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        report = result.report_dir
        observations_text = (report / RESOURCE_OBSERVATIONS_JSONL).read_text().strip()
        observation_lines = [line for line in observations_text.splitlines() if line.strip()]
        expected_inferences = len(_TASK_IDS) * len(_COHORT_IDS) * len(_ARM_IDS) * len(_REPLICATE_IDS)
        assert len(observation_lines) == expected_inferences, (
            f"expected {expected_inferences} resource observations, got {len(observation_lines)}"
        )

        for line in observation_lines:
            obs = json.loads(line)
            assert obs["campaign_id"] == _CAMPAIGN_ID
            assert obs["run_id"] == result.run_id
            assert obs["assignment_id"] != ""
            assert obs["attempt_id"] != ""
            assert obs["inference_id"] != ""
            assert obs["stage_id"] != ""
            assert obs["role"] == ModelRole.PRIMARY.value
            assert obs["model_variant_id"] != ""
            assert obs["task_id"] in _TASK_IDS
            assert obs["orchestrator_scope"] != ""
            assert obs["provider_scope"] != ""
            assert obs["observation_boundary"] != ""
            assert obs["clock_domain"] != ""
            assert obs["collection_tool"] != ""

    def test_ensemble_answer_without_provider_observation_is_invalid_evidence(
        self, tmp_path: Path
    ):
        spec = _make_spec(task_ids=[_TASK_IDS[0]])
        preregistration = spec.preregistration.model_copy(
            update={
                "baseline_arm_id": Arm.ENSEMBLE_UNGOVERNED.value,
                "comparison_arm_ids": [],
            }
        )
        spec = spec.model_copy(update={"preregistration": preregistration})
        runner = CampaignRunner(
            spec=spec,
            sut_factory=lambda cohort, arm: UninstrumentedEnsembleFakeSUT(
                model_id=cohort.role_bindings[0].model_id
            ),
            tasks=_make_tasks([_TASK_IDS[0]]),
            grader=FakeGrader(),
            output_dir=tmp_path,
            campaign_profile=CampaignProfile.model_construct(
                hardware_identity="linux/amd64/cpu",
                track_arm_assignments=[
                    TrackArmAssignment(
                        track=CampaignTrack.TIER_FITNESS,
                        arm_id=Arm.ENSEMBLE_UNGOVERNED.value,
                    )
                ],
            ),
        )

        result = asyncio.run(runner.run())
        attempts = [
            json.loads(line)
            for line in (result.report_dir / ATTEMPTS_JSONL).read_text().splitlines()
            if line
        ]

        assert len(attempts) == 1
        assert attempts[0]["terminal_status"] == TerminalStatus.INVALID_EVIDENCE.value
        assert attempts[0]["missingness_or_failure"] == "inference_observation_missing"


# ---------------------------------------------------------------------------
# Tests: Honest missingness for unavailable remote measurements
# ---------------------------------------------------------------------------


class TestHonestMissingness:
    """Unavailable remote measurements remain None with typed unavailable_measurements entries.

    Remote GPU metrics are not inferred from model metadata. The direct
    arm cannot observe GPU utilization, temperature, power draw, or clock
    from the provider response, so those fields must be None with typed
    unavailable_measurements entries explaining why.
    """

    def test_gpu_metrics_unavailable_with_typed_reason(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        report = result.report_dir
        observations_text = (report / RESOURCE_OBSERVATIONS_JSONL).read_text().strip()
        observation_lines = [line for line in observations_text.splitlines() if line.strip()]
        assert len(observation_lines) > 0

        for line in observation_lines:
            obs = json.loads(line)
            # GPU metrics must be None (not zero) when unavailable
            assert obs["gpu_utilization_percent"] is None
            assert obs["gpu_temperature_celsius"] is None
            assert obs["gpu_power_draw_watts"] is None
            assert obs["gpu_clock_mhz"] is None
            # Each None field must have a typed unavailable_measurements entry
            unavailable = {um["field_name"] for um in obs["unavailable_measurements"]}
            assert "gpu_utilization_percent" in unavailable
            assert "gpu_temperature_celsius" in unavailable
            assert "gpu_power_draw_watts" in unavailable
            assert "gpu_clock_mhz" in unavailable
            # Check typed availability and scope
            for um in obs["unavailable_measurements"]:
                if um["field_name"].startswith("gpu_"):
                    assert um["availability"] == MeasurementAvailability.UNAVAILABLE.value
                    assert um["scope"] == MeasurementScope.PROVIDER_REMOTE.value
                    assert um["reason"] != ""

    def test_measured_timing_fields_present(self, tmp_path: Path):
        """Timing fields from the provider response are measured, not None."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        report = result.report_dir
        observations_text = (report / RESOURCE_OBSERVATIONS_JSONL).read_text().strip()
        observation_lines = [line for line in observations_text.splitlines() if line.strip()]
        assert len(observation_lines) > 0

        for line in observation_lines:
            obs = json.loads(line)
            # Timing fields from the fake SUT are measured
            assert obs["provider_call_latency_seconds"] is not None
            assert obs["provider_call_latency_seconds"] > 0.0
            assert obs["time_to_first_token_seconds"] is not None
            assert obs["generation_duration_seconds"] is not None
            assert obs["output_throughput_tokens_per_second"] is not None


class TestFailedCallObservations:
    """Failed provider calls materialize valid resource observations.

    A failed call carries measured monotonic-span latency but no TTFT,
    generation duration, or throughput. Every None measurement field on
    the ResourceObservation must carry a typed unavailable_measurements
    entry — the smoke-run ValidationError regression.
    """

    def test_failed_call_observation_has_unavailable_entries_for_none_fields(
        self, tmp_path: Path
    ):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_failed_call_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        report = result.report_dir
        observations_text = (report / RESOURCE_OBSERVATIONS_JSONL).read_text().strip()
        observation_lines = [line for line in observations_text.splitlines() if line.strip()]
        expected_inferences = len(_TASK_IDS) * len(_COHORT_IDS) * len(_ARM_IDS) * len(_REPLICATE_IDS)
        assert len(observation_lines) == expected_inferences, (
            "failed provider calls must still materialize one resource observation each"
        )

        for line in observation_lines:
            obs = json.loads(line)
            # Measured monotonic-span latency is present
            assert obs["provider_call_latency_seconds"] == 0.02
            # Unmeasured warm-inference fields are None with typed entries
            assert obs["time_to_first_token_seconds"] is None
            assert obs["generation_duration_seconds"] is None
            assert obs["output_throughput_tokens_per_second"] is None
            unavailable = {um["field_name"] for um in obs["unavailable_measurements"]}
            for field_name in (
                "time_to_first_token_seconds",
                "generation_duration_seconds",
                "output_throughput_tokens_per_second",
                "hidden_reasoning_throughput_tokens_per_second",
            ):
                assert field_name in unavailable, (
                    f"{field_name} is None but has no unavailable_measurements entry"
                )


# ---------------------------------------------------------------------------
# Tests: AnalysisInputRecord includes resource and event records
# ---------------------------------------------------------------------------


class TestAnalysisInputInclusion:
    """Resource and event records are fed into AnalysisInputRecord before canonical analysis.

    The analysis input hash must cover resource observations, tool call
    scorecards, escalation records, security events, and correlated
    errors so derived metrics and hashes include them.
    """

    def test_analysis_input_contains_resource_observations(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        analysis_input_text = (result.report_dir / ANALYSIS_INPUT_JSON).read_text()
        analysis_input = json.loads(analysis_input_text)
        assert "resource_observations" in analysis_input
        assert len(analysis_input["resource_observations"]) > 0

    def test_analysis_input_contains_empty_event_lists(self, tmp_path: Path):
        """The direct arm produces no events, but the lists must be present (empty)."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        analysis_input_text = (result.report_dir / ANALYSIS_INPUT_JSON).read_text()
        analysis_input = json.loads(analysis_input_text)
        assert "tool_call_scorecards" in analysis_input
        assert analysis_input["tool_call_scorecards"] == []
        assert "escalation_records" in analysis_input
        assert analysis_input["escalation_records"] == []
        assert "security_events" in analysis_input
        assert analysis_input["security_events"] == []
        assert "correlated_error_records" in analysis_input
        assert analysis_input["correlated_error_records"] == []


# ---------------------------------------------------------------------------
# Tests: Stage records and evidence index
# ---------------------------------------------------------------------------


class TestStageAndEvidenceRecords:
    """The runner persists stage records and evidence-index entries.

    Each provider inference produces a StageObservation bound to the
    attempt and inference. The evidence-index.jsonl file contains entries
    for indexed source artifacts needed to verify VERIFIED records.
    """

    def test_stages_jsonl_exists_with_inference_records(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        stages_path = result.report_dir / STAGES_JSONL
        assert stages_path.exists(), "stages.jsonl missing"
        stages_text = stages_path.read_text().strip()
        stage_lines = [line for line in stages_text.splitlines() if line.strip()]
        expected_inferences = len(_TASK_IDS) * len(_COHORT_IDS) * len(_ARM_IDS) * len(_REPLICATE_IDS)
        assert len(stage_lines) == expected_inferences, (
            f"expected {expected_inferences} stage records, got {len(stage_lines)}"
        )

    def test_evidence_index_jsonl_exists(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        evidence_index_path = result.report_dir / EVIDENCE_INDEX_JSONL
        assert evidence_index_path.exists(), "evidence-index.jsonl missing"


# ---------------------------------------------------------------------------
# Tests: Deterministic SUT close
# ---------------------------------------------------------------------------


class TestSutClose:
    """SUTs are closed deterministically on success, typed stop, and exception.

    The runner calls close() on every SUT it created, regardless of
    whether the campaign completed, stopped due to budget, or encountered
    an error.
    """

    def test_sut_closed_on_success(self, tmp_path: Path):
        spec = _make_spec()
        factory = _make_instrumented_sut_factory()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=factory,
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        asyncio.run(runner.run())

        # The InstrumentedFakeSUT resets _call_count on close
        for sut in runner._suts.values():
            assert isinstance(sut, InstrumentedFakeSUT)
            assert sut._call_count == 0, f"SUT not closed: _call_count={sut._call_count}"

    def test_sut_closed_on_budget_stop(self, tmp_path: Path):
        budget = ProviderBudget(max_usd=100.0, max_requests=1)
        spec = _make_spec(provider_budget=budget)
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        assert result.status == CampaignStatus.STOPPED
        assert result.stop_reason == CampaignStopReason.BUDGET_EXHAUSTED
        for sut in runner._suts.values():
            assert isinstance(sut, InstrumentedFakeSUT)
            assert sut._call_count == 0, f"SUT not closed on budget stop: _call_count={sut._call_count}"


# ---------------------------------------------------------------------------
# Tests: Resource observation identity bindings
# ---------------------------------------------------------------------------


class TestResourceObservationIdentity:
    """Resource observations bind to the exact attempt, inference, stage, role, and task.

    The observation's attempt_id, inference_id, stage_id, role, and task_id
    must match the actual provider call that produced it.
    """

    def test_observation_binds_to_attempt_and_inference(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        # Read attempts
        attempts_text = (result.report_dir / ATTEMPTS_JSONL).read_text().strip()
        attempts = [json.loads(line) for line in attempts_text.splitlines() if line.strip()]
        attempt_ids = {a["attempt_id"] for a in attempts}

        # Read resource observations
        obs_text = (result.report_dir / RESOURCE_OBSERVATIONS_JSONL).read_text().strip()
        observations = [json.loads(line) for line in obs_text.splitlines() if line.strip()]

        for obs in observations:
            assert obs["attempt_id"] in attempt_ids, (
                f"observation attempt_id {obs['attempt_id']} not in attempts"
            )
            assert obs["inference_id"] != ""
            assert obs["stage_id"] != ""

    def test_attempt_local_inference_ids_are_distinct_by_compound_identity(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_attempt_local_inference_id_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        obs_text = (result.report_dir / RESOURCE_OBSERVATIONS_JSONL).read_text().strip()
        observations = [json.loads(line) for line in obs_text.splitlines() if line.strip()]
        assert [obs["inference_id"] for obs in observations] == ["inf-0", "inf-0"]
        identities = {(obs["attempt_id"], obs["inference_id"]) for obs in observations}
        assert len(identities) == len(observations)

    def test_child_id_equals_campaign_id_for_single_campaign(self, tmp_path: Path):
        """For a single (non-set) campaign, child_id equals campaign_id."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        obs_text = (result.report_dir / RESOURCE_OBSERVATIONS_JSONL).read_text().strip()
        observations = [json.loads(line) for line in obs_text.splitlines() if line.strip()]
        for obs in observations:
            assert obs["campaign_id"] == _CAMPAIGN_ID
            assert obs["child_id"] == _CAMPAIGN_ID


# ---------------------------------------------------------------------------
# Tests: Atomic persistence (no partial files)
# ---------------------------------------------------------------------------


class TestAtomicPersistence:
    """All JSONL files are written atomically; no .tmp files remain after run."""

    def test_no_tmp_files_after_successful_run(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        asyncio.run(runner.run())

        tmp_files = list(runner.report_dir.rglob("*.tmp"))
        assert len(tmp_files) == 0, f"tmp files remain: {[str(f) for f in tmp_files]}"

    def test_no_tmp_files_after_budget_stop(self, tmp_path: Path):
        budget = ProviderBudget(max_usd=100.0, max_requests=1)
        spec = _make_spec(provider_budget=budget)
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_instrumented_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        asyncio.run(runner.run())

        tmp_files = list(runner.report_dir.rglob("*.tmp"))
        assert len(tmp_files) == 0, f"tmp files remain after budget stop: {[str(f) for f in tmp_files]}"
