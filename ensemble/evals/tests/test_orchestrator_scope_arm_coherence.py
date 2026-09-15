# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for the S2 smoke defects (orchestrator scope
vocabulary and profile/preregistration arm coherence).

DEFECT S2-A: ``_ORCHESTRATOR_SCOPE`` emitted ``platform.machine()``
vocabulary (``x86_64``) while frozen profiles declare GOARCH vocabulary
(``amd64``), producing 31 identical ``orchestrator_scope mismatch``
verifier failures. The producer now canonicalizes the machine token,
emits the bound profile's declared ``hardware_identity`` when a profile
is bound, and rejects an observed/declared prefix mismatch at preflight.

DEFECT S2-B: the runner derived assignment arms solely from the
preregistration, so a direct-arm preregistration bound to a
tier-fitness profile silently executed the direct track. A preflight
subset check now rejects preregistration arms the bound profile does
not declare, and the verifier's ``arm_coherence`` layer fails a report
whose executed arms (run manifest and attempts) are not declared by the
binding's ``track_arm_assignments``.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import platform
import time
from dataclasses import dataclass
from pathlib import Path

import pytest

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
from g8e_evals.constants import ATTEMPTS_JSONL, MANIFEST_JSON, RESOURCE_OBSERVATIONS_JSONL
from g8e_evals.harness import InferenceObservation, Response, Score, Task
from g8e_evals.index import ModelRole, ResourceObservation
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.profile import (
    CAMPAIGN_PROFILE_VERSION,
    CampaignLifecycleStatus,
    CampaignProfile,
    ClaimBoundary,
    TrackArmAssignment,
    compute_campaign_profile_hash,
)
from g8e_evals.registry import (
    MODEL_REGISTRY_VERSION,
    ModelRegistry,
    ModelVariant,
    PublicationEligibility,
    WeightClass,
    compute_model_registry_hash,
)
from g8e_evals.runner import (
    CampaignRunner,
    CampaignRunnerError,
    CampaignSpec,
    _ORCHESTRATOR_SCOPE,
    _build_resource_observation,
    _canonical_machine_token,
    _validate_arm_coherence,
    _validate_orchestrator_scope,
)
from g8e_evals.schema import (
    CampaignTrack,
    ReportRole,
    RunManifest,
)


_CAMPAIGN_ID = "s2-coherence-campaign"
_CAMPAIGN_REVISION = "1"
_TASK_IDS = ["task-1001"]
_COHORT_ID = "cohort-qwen3-8b"
_VARIANT_ID = "qwen3-8b-q4_0"
_REPLICATE_IDS = ["replicate-1"]
_INITIAL_STATE_ID = "no-initial-state-v1"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64
_VALID_HASH = "a" * 64
_DECLARED_HARDWARE = "linux/amd64/rtx-4090"


@dataclass
class _FakeSUT:
    model_id: str
    answer: str = "This is a test answer with no commas."

    async def get_answer(self, task: Task) -> Response:
        start = time.monotonic()
        inference_obs = InferenceObservation(
            inference_id=f"inf-{task.id}-{start}",
            role="primary",
            model_variant_id=self.model_id,
            provider="ollama",
            model=self.model_id,
            provider_call_latency_seconds=1.0,
            time_to_first_token_seconds=0.5,
            generation_duration_seconds=1.0,
            output_throughput_tokens_per_second=10.0,
            prompt_token_count=10,
            candidates_token_count=10,
            total_token_count=20,
            usage_reported=True,
            finish_reason="stop",
            monotonic_start=1.0,
            monotonic_end=2.0,
        )
        return Response(
            answer=self.answer,
            model=self.model_id,
            arm=Arm.DIRECT,
            inference_observations=[inference_obs],
        )


    def close(self) -> None:
        pass


@dataclass
class _FakeGrader:
    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        return Score(task_id=task.id, passed=True, details=ScoreDetails())


def _make_variant() -> ModelVariant:
    return ModelVariant(
        variant_id=_VARIANT_ID,
        canonical_display_name="Qwen3 8B",
        source_list_alias="qwen3:8b",
        hf_repo="Qwen/Qwen3-8B",
        hf_sha="a" * 40,
        retrieval_date="2026-09-01",
        license_id="Apache-2.0",
        license_text_hash=_VALID_HASH,
        gated=False,
        publication_eligibility=PublicationEligibility.ELIGIBLE,
        parameter_count=8_000_000_000,
        parameter_count_display="8.0B",
        architecture="QwenForCausalLM",
        model_type="qwen3",
        dtype="BF16",
        format="gguf",
        quantization="q4_0",
        context_length=32768,
        supported_modalities=["text"],
        reasoning_mode="non-reasoning",
        tool_call_support=True,
        chat_template_family="qwen3",
        chat_template_hash="c" * 64,
        tokenizer_digest="d" * 64,
        weight_class=WeightClass.HEAVY_SLM,
        backend_name="ollama",
        backend_version="0.1.48",
        served_model_tag="qwen3:8b",
        artifact_digest="b" * 64,
        artifact_bytes=8_000_000_000,
        tensor_format="gguf",
        hidden_reasoning_tokens=False,
    )


def _make_registry() -> ModelRegistry:
    variants = [_make_variant()]
    content_hash = compute_model_registry_hash("registry-v1", "1", variants, [])
    return ModelRegistry(
        registry_id="registry-v1",
        registry_version="1",
        schema_version=MODEL_REGISTRY_VERSION,
        created_at="2026-09-01T00:00:00Z",
        variants=variants,
        qualification_records=[],
        content_hash=content_hash,
    )


def _make_profile(
    registry_hash: str,
    *,
    arm_ids: list[str] | None = None,
    hardware_identity: str = _DECLARED_HARDWARE,
) -> CampaignProfile:
    if arm_ids is None:
        arm_ids = ["direct", "ensemble_ungoverned"]
    track_by_arm = {
        "direct": CampaignTrack.DIRECT,
        "ensemble_ungoverned": CampaignTrack.TIER_FITNESS,
        "doctrine": CampaignTrack.GOVERNED,
        "consensus": CampaignTrack.GOVERNED,
        "notary": CampaignTrack.GOVERNED,
    }
    track_arm_assignments = [
        TrackArmAssignment(track=track_by_arm[a], arm_id=a) for a in arm_ids
    ]
    fields = {
        "campaign_id": _CAMPAIGN_ID,
        "campaign_revision": _CAMPAIGN_REVISION,
        "schema_version": CAMPAIGN_PROFILE_VERSION,
        "purpose": "S2 coherence regression campaign",
        "created_at": "2026-09-01T00:00:00Z",
        "lifecycle_status": CampaignLifecycleStatus.FROZEN,
        "generative_variant_ids": [_VARIANT_ID],
        "benchmark_ids": ["ifeval_subset"],
        "dataset_hashes": [_DATASET_HASH],
        "grader_hashes": ["g" * 64],
        "prompt_serialization_hash": _VALID_HASH,
        "task_ids": _TASK_IDS,
        "repetitions": 1,
        "track_arm_assignments": track_arm_assignments,
        "model_tier_assignments": [],
        "baseline_tier_mappings": {},
        "routing_policy": "default",
        "temperature": 0.0,
        "top_p": 1.0,
        "max_tokens": 4096,
        "seed": 42,
        "context_limit": 32768,
        "timeout_seconds": 120.0,
        "max_retries": 1,
        "warmup_excluded": True,
        "concurrency": 1,
        "hardware_identity": hardware_identity,
        "environment_stratum": "single-machine",
        "primary_metrics": ["ifeval_subset_verifier"],
        "unit_of_analysis": "task",
        "claim_boundary": ClaimBoundary.DESCRIPTIVE_ONLY,
        "model_registry_hash": registry_hash,
    }
    temp = CampaignProfile.model_construct(**fields, content_hash="0" * 64)
    return CampaignProfile(**fields, content_hash=compute_campaign_profile_hash(temp))


def _make_spec(*, arm_ids: list[str] | None = None) -> CampaignSpec:
    if arm_ids is None:
        arm_ids = ["direct", "ensemble_ungoverned"]
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
        initial_state_assignment_id=_INITIAL_STATE_ID,
        state_type="no_initial_state",
        snapshot_hash=_NO_STATE_HASH,
        content_hash=compute_initial_state_hash(_INITIAL_STATE_ID, "no_initial_state", _NO_STATE_HASH),
    )
    prereg = PreregistrationConfig(
        config_id="s2-coherence-prereg",
        config_version="1.0.0",
        baseline_arm_id=arm_ids[0],
        comparison_arm_ids=arm_ids[1:],
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
        release_version="v2.1.8",
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
    return [
        Task(
            id=tid,
            prompt=f"Prompt for {tid}",
            metadata=TaskMetadata(
                benchmark="ifeval_subset",
                instruction_id_list=["punctuation:no_comma"],
                kwargs=[{"no_comma": True}],
            ),
        )
        for tid in _TASK_IDS
    ]


def _fake_sut_factory(cohort: ModelCohort, arm: Arm):
    return _FakeSUT(model_id=cohort.role_bindings[0].model_id)


def _make_runner(
    tmp_path: Path,
    *,
    profile: CampaignProfile | None,
    spec: CampaignSpec | None = None,
) -> CampaignRunner:
    registry = _make_registry()
    if profile is None:
        return CampaignRunner(
            spec=spec or _make_spec(),
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
    return CampaignRunner(
        spec=spec or _make_spec(),
        sut_factory=_fake_sut_factory,
        tasks=_make_tasks(),
        grader=_FakeGrader(),
        output_dir=tmp_path,
        campaign_profile=profile,
        model_registry=registry,
    )


@pytest.mark.unit
class TestCanonicalMachineToken:
    """The producer canonicalizes platform.machine() to GOARCH vocabulary."""

    def test_x86_64_canonicalizes_to_amd64(self):
        assert _canonical_machine_token("x86_64") == "amd64"

    def test_aarch64_canonicalizes_to_arm64(self):
        assert _canonical_machine_token("aarch64") == "arm64"

    def test_canonical_tokens_pass_through(self):
        assert _canonical_machine_token("amd64") == "amd64"
        assert _canonical_machine_token("arm64") == "arm64"

    def test_unknown_token_passes_through(self):
        assert _canonical_machine_token("riscv64") == "riscv64"

    def test_canonicalization_is_case_insensitive(self):
        assert _canonical_machine_token("X86_64") == "amd64"
        assert _canonical_machine_token("AMD64") == "amd64"

    def test_orchestrator_scope_uses_canonical_vocabulary(self):
        machine_segment = _ORCHESTRATOR_SCOPE.split("/")[1]
        assert machine_segment == _canonical_machine_token(platform.machine())
        assert machine_segment != "x86_64"


@pytest.mark.unit
class TestValidateOrchestratorScope:
    """The preflight rejects a declared identity the host contradicts."""

    def test_observed_scope_passes(self):
        _validate_orchestrator_scope(_ORCHESTRATOR_SCOPE)

    def test_matching_prefix_with_accelerator_suffix_passes(self):
        # The accelerator suffix is not observable via platform; only
        # the system/machine prefix is compared.
        prefix = "/".join(_ORCHESTRATOR_SCOPE.split("/")[:2])
        _validate_orchestrator_scope(f"{prefix}/rtx-4090")

    def test_mismatched_prefix_rejected(self):
        observed_prefix = "/".join(_ORCHESTRATOR_SCOPE.split("/")[:2])
        with pytest.raises(CampaignRunnerError, match="orchestrator scope mismatch"):
            _validate_orchestrator_scope("other-system/other-machine/cpu")
        # Sanity: the constructed prefix really does differ.
        assert observed_prefix != "other-system/other-machine"

    def test_unparseable_identity_rejected(self):
        with pytest.raises(CampaignRunnerError, match="no system/machine prefix"):
            _validate_orchestrator_scope("unavailable")


@pytest.mark.unit
class TestValidateArmCoherence:
    """Every preregistration arm must be declared by the bound profile."""

    def _prereg(self, baseline: str, comparisons: list[str]) -> PreregistrationConfig:
        return PreregistrationConfig(
            config_id="test",
            config_version="1.0.0",
            baseline_arm_id=baseline,
            comparison_arm_ids=comparisons,
            model_cohort_ids=[_COHORT_ID],
            task_assignment_id="task-assignment-v1",
            initial_state_assignment_id=_INITIAL_STATE_ID,
            required_replicate_ids=_REPLICATE_IDS,
            required_replicate_count=1,
            primary_metric_ids=["ifeval_subset_verifier"],
            bootstrap_count=100,
            bootstrap_confidence=0.95,
            bootstrap_seed=0,
            significance_level=0.05,
            claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY,
        )

    def test_subset_passes(self):
        profile = CampaignProfile.model_construct(
            track_arm_assignments=[
                TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
                TrackArmAssignment(track=CampaignTrack.TIER_FITNESS, arm_id="ensemble_ungoverned"),
            ]
        )
        _validate_arm_coherence(profile, self._prereg("direct", ["ensemble_ungoverned"]))

    def test_strict_subset_passes(self):
        # A profile may declare arms the preregistration does not use.
        profile = CampaignProfile.model_construct(
            track_arm_assignments=[
                TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
                TrackArmAssignment(track=CampaignTrack.TIER_FITNESS, arm_id="ensemble_ungoverned"),
            ]
        )
        _validate_arm_coherence(profile, self._prereg("direct", []))

    def test_undeclared_baseline_arm_rejected(self):
        # The S2-B defect shape: a direct-arm preregistration bound to a
        # tier-fitness-only profile must be rejected, not silently honored.
        profile = CampaignProfile.model_construct(
            track_arm_assignments=[
                TrackArmAssignment(track=CampaignTrack.TIER_FITNESS, arm_id="ensemble_ungoverned"),
            ]
        )
        with pytest.raises(CampaignRunnerError, match="not present in the profile"):
            _validate_arm_coherence(profile, self._prereg("direct", []))

    def test_partial_overlap_rejected(self):
        # One declared arm plus one undeclared comparison arm is rejected;
        # subset semantics, not non-empty intersection.
        profile = CampaignProfile.model_construct(
            track_arm_assignments=[
                TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
            ]
        )
        with pytest.raises(CampaignRunnerError, match="doctrine"):
            _validate_arm_coherence(profile, self._prereg("direct", ["doctrine"]))


@pytest.mark.unit
class TestResourceObservationOrchestratorScope:
    """The producer stamps the scope it is given, not a raw platform value."""

    def test_declared_identity_is_emitted(self):
        obs = InferenceObservation(
            inference_id="inf-1",
            role="primary",
            model_variant_id="qwen3:8b",
            provider="ollama",
            model="qwen3:8b",
            error="connection refused",
        )
        ro = _build_resource_observation(
            obs=obs,
            campaign_id="c1",
            child_id="c1",
            run_id="r1",
            assignment_id="a1",
            attempt_id="at1",
            task_id="t1",
            stage_id="s1",
            orchestrator_scope=_DECLARED_HARDWARE,
        )
        assert isinstance(ro, ResourceObservation)
        assert ro.orchestrator_scope == _DECLARED_HARDWARE


@pytest.mark.integration
class TestRunnerPreflights:
    """Profile-bound preflights fire before the report directory is created."""

    def test_scope_prefix_mismatch_rejects_before_report_dir(self, tmp_path: Path):
        observed_prefix = "/".join(_ORCHESTRATOR_SCOPE.split("/")[:2])
        bad_identity = "other-system/other-machine/cpu"
        assert "/".join(bad_identity.split("/")[:2]) != observed_prefix
        registry = _make_registry()
        profile = _make_profile(registry.content_hash, hardware_identity=bad_identity)
        runner = _make_runner(tmp_path, profile=profile)
        with pytest.raises(CampaignRunnerError, match="orchestrator scope mismatch"):
            asyncio.run(runner.run())
        assert list(tmp_path.iterdir()) == []

    def test_undeclared_prereg_arm_rejects_before_report_dir(self, tmp_path: Path):
        registry = _make_registry()
        profile = _make_profile(registry.content_hash, arm_ids=["direct"])
        spec = _make_spec(arm_ids=["ensemble_ungoverned"])
        runner = _make_runner(tmp_path, profile=profile, spec=spec)
        with pytest.raises(CampaignRunnerError, match="not present in the profile"):
            asyncio.run(runner.run())
        assert list(tmp_path.iterdir()) == []

    def test_unprofiled_run_skips_preflights(self, tmp_path: Path):
        runner = _make_runner(tmp_path, profile=None, spec=_make_spec(arm_ids=["direct"]))
        result = asyncio.run(runner.run())
        assert result.report_dir.exists()


@pytest.mark.integration
class TestDeclaredScopeAndVerifierCoherence:
    """A profile-bound run stamps the declared scope and verifies clean."""

    def _run_bound_campaign(self, tmp_path: Path) -> Path:
        registry = _make_registry()
        profile = _make_profile(registry.content_hash)
        runner = _make_runner(tmp_path, profile=profile)
        result = asyncio.run(runner.run())
        return result.report_dir

    def test_resource_observations_carry_declared_identity(self, tmp_path: Path):
        report_dir = self._run_bound_campaign(tmp_path)
        observations = [
            ResourceObservation.model_validate_json(line)
            for line in (report_dir / RESOURCE_OBSERVATIONS_JSONL).read_text().splitlines()
            if line.strip()
        ]
        assert observations
        for ro in observations:
            assert ro.orchestrator_scope == _DECLARED_HARDWARE

    def test_bound_campaign_verifies_clean(self, tmp_path: Path):
        report_dir = self._run_bound_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert result.ok, result.failures
        assert "arm_coherence" in result.checked_layers

    def test_binding_carries_declared_track_arms(self, tmp_path: Path):
        report_dir = self._run_bound_campaign(tmp_path)
        manifest = RunManifest.model_validate_json(
            (report_dir / MANIFEST_JSON).read_text()
        )
        assert manifest.campaign_binding is not None
        declared = {a.arm_id for a in manifest.campaign_binding.track_arm_assignments}
        assert declared == {"direct", "ensemble_ungoverned"}
        assert manifest.campaign_binding.report_role == ReportRole.SINGLE

    def test_undeclared_manifest_arm_fails_verification(self, tmp_path: Path):
        report_dir = self._run_bound_campaign(tmp_path)
        manifest_path = report_dir / MANIFEST_JSON
        manifest_data = json.loads(manifest_path.read_text())
        manifest_data["campaign_binding"]["track_arm_assignments"] = [
            {"track": "direct", "arm_id": "direct"}
        ]
        manifest_path.write_text(json.dumps(manifest_data, indent=2))
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any(
            "arm coherence" in f and "ensemble_ungoverned" in f
            for f in result.failures
        ), result.failures

    def test_undeclared_attempt_arm_fails_verification(self, tmp_path: Path):
        report_dir = self._run_bound_campaign(tmp_path)
        attempts_path = report_dir / ATTEMPTS_JSONL
        lines = attempts_path.read_text().splitlines()
        first = json.loads(lines[0])
        first["arm_id"] = "doctrine"
        lines[0] = json.dumps(first)
        attempts_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any(
            "arm coherence" in f and "attempt" in f and "doctrine" in f
            for f in result.failures
        ), result.failures
