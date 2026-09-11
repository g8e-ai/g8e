# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for CampaignBinding population from
CampaignProfile and ModelRegistry.

Verifies that the CampaignRunner populates RunManifest.campaign_binding
with correct backend artifact identity, tokenizer template identity,
and campaign-level hashes when a CampaignProfile and ModelRegistry are
provided. Also verifies that the campaign verifier rejects
artifact-identity mismatches where the provider telemetry model string
matches but the immutable artifact or settings identity does not.
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
from g8e_evals.constants import MANIFEST_JSON
from g8e_evals.harness import Response, Score, Task
from g8e_evals.index import CampaignVerificationReport
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
from g8e_evals.runner import CampaignRunner, CampaignSpec
from g8e_evals.schema import (
    CampaignTrack,
    RunManifest,
)


_CAMPAIGN_ID = "v2.1.8-ifeval-pipeline-integrity"
_CAMPAIGN_REVISION = "1"
_RELEASE_VERSION = "v2.1.8"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_ID = "cohort-qwen3-8b"
_VARIANT_ID = "qwen3-8b-q4_0"
_ARM_IDS = ["direct", "ensemble_ungoverned"]
_REPLICATE_IDS = ["replicate-1"]
_INITIAL_STATE_ID = "no-initial-state-v1"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64
_VALID_HASH = "a" * 64
_BACKEND_ARTIFACT_DIGEST = "b" * 64
_TOKENIZER_DIGEST = "c" * 64
_CHAT_TEMPLATE_HASH = "d" * 64


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
        chat_template_hash=_CHAT_TEMPLATE_HASH,
        tokenizer_digest=_TOKENIZER_DIGEST,
        weight_class=WeightClass.HEAVY_SLM,
        backend_name="ollama",
        backend_version="0.1.48",
        served_model_tag="qwen3:8b",
        artifact_digest=_BACKEND_ARTIFACT_DIGEST,
        artifact_bytes=8_000_000_000,
        tensor_format="gguf",
        hidden_reasoning_tokens=False,
    )


def _make_registry(variant: ModelVariant | None = None) -> ModelRegistry:
    v = variant or _make_variant()
    variants = [v]
    qual_records: list = []
    content_hash = compute_model_registry_hash(
        "registry-v1", "1", variants, qual_records,
    )
    return ModelRegistry(
        registry_id="registry-v1",
        registry_version="1",
        schema_version=MODEL_REGISTRY_VERSION,
        created_at="2026-09-01T00:00:00Z",
        variants=variants,
        qualification_records=qual_records,
        content_hash=content_hash,
    )


def _make_profile(registry_hash: str) -> CampaignProfile:
    track_arm_assignments = [
        TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
        TrackArmAssignment(track=CampaignTrack.TIER_FITNESS, arm_id="ensemble_ungoverned"),
    ]
    temp = CampaignProfile.model_construct(
        campaign_id=_CAMPAIGN_ID,
        campaign_revision=_CAMPAIGN_REVISION,
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="Pipeline integrity campaign",
        created_at="2026-09-01T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
        generative_variant_ids=[_VARIANT_ID],
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_DATASET_HASH],
        grader_hashes=["g" * 64],
        prompt_serialization_hash=_VALID_HASH,
        task_ids=_TASK_IDS,
        repetitions=1,
        track_arm_assignments=track_arm_assignments,
        model_tier_assignments=[],
        baseline_tier_mappings={},
        routing_policy="default",
        temperature=0.0,
        top_p=1.0,
        max_tokens=4096,
        seed=42,
        context_limit=32768,
        timeout_seconds=120.0,
        max_retries=1,
        warmup_excluded=True,
        concurrency=1,
        hardware_identity="linux/amd64/rtx-4090",
        environment_stratum="single-machine",
        primary_metrics=["ifeval_subset_verifier"],
        unit_of_analysis="task",
        claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
        model_registry_hash=registry_hash,
        content_hash="0" * 64,
    )
    content_hash = compute_campaign_profile_hash(temp)
    return CampaignProfile(
        campaign_id=_CAMPAIGN_ID,
        campaign_revision=_CAMPAIGN_REVISION,
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="Pipeline integrity campaign",
        created_at="2026-09-01T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
        generative_variant_ids=[_VARIANT_ID],
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_DATASET_HASH],
        grader_hashes=["g" * 64],
        prompt_serialization_hash=_VALID_HASH,
        task_ids=_TASK_IDS,
        repetitions=1,
        track_arm_assignments=track_arm_assignments,
        model_tier_assignments=[],
        temperature=0.0,
        top_p=1.0,
        max_tokens=4096,
        seed=42,
        context_limit=32768,
        timeout_seconds=120.0,
        max_retries=1,
        warmup_excluded=True,
        concurrency=1,
        hardware_identity="linux/amd64/rtx-4090",
        environment_stratum="single-machine",
        primary_metrics=["ifeval_subset_verifier"],
        unit_of_analysis="task",
        claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
        model_registry_hash=registry_hash,
        content_hash=content_hash,
    )


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


def _run_campaign_with_binding(tmp_path: Path) -> Path:
    """Run a fake-provider campaign with profile+registry and return the report directory."""
    spec = _make_spec()
    registry = _make_registry()
    profile = _make_profile(registry.content_hash)
    runner = CampaignRunner(
        spec=spec,
        sut_factory=_fake_sut_factory,
        tasks=_make_tasks(),
        grader=_FakeGrader(),
        output_dir=tmp_path,
        campaign_profile=profile,
        model_registry=registry,
        cohort_variant_map={_COHORT_ID: _VARIANT_ID},
    )
    result = asyncio.run(runner.run())
    return result.report_dir


class TestCampaignBindingPopulation:
    def test_run_manifest_has_campaign_binding(self, tmp_path: Path):
        """RunManifest carries a populated CampaignBinding when profile+registry are provided."""
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest_path = report_dir / MANIFEST_JSON
        manifest = RunManifest.model_validate_json(manifest_path.read_text())
        assert manifest.campaign_binding is not None

    def test_campaign_binding_has_correct_campaign_id(self, tmp_path: Path):
        """CampaignBinding carries the campaign ID from the profile."""
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest = RunManifest.model_validate_json(
            (report_dir / MANIFEST_JSON).read_text()
        )
        assert manifest.campaign_binding is not None
        assert manifest.campaign_binding.campaign_id == _CAMPAIGN_ID

    def test_campaign_binding_has_correct_revision(self, tmp_path: Path):
        """CampaignBinding carries the campaign revision from the profile."""
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest = RunManifest.model_validate_json(
            (report_dir / MANIFEST_JSON).read_text()
        )
        assert manifest.campaign_binding is not None
        assert manifest.campaign_binding.campaign_revision == _CAMPAIGN_REVISION

    def test_campaign_binding_has_correct_profile_hash(self, tmp_path: Path):
        """CampaignBinding carries the frozen campaign profile hash."""
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest = RunManifest.model_validate_json(
            (report_dir / MANIFEST_JSON).read_text()
        )
        assert manifest.campaign_binding is not None
        registry = _make_registry()
        profile = _make_profile(registry.content_hash)
        assert manifest.campaign_binding.campaign_profile_hash == profile.content_hash

    def test_campaign_binding_has_correct_registry_hash(self, tmp_path: Path):
        """CampaignBinding carries the frozen model registry hash."""
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest = RunManifest.model_validate_json(
            (report_dir / MANIFEST_JSON).read_text()
        )
        assert manifest.campaign_binding is not None
        registry = _make_registry()
        assert manifest.campaign_binding.model_registry_hash == registry.content_hash

    def test_campaign_binding_has_correct_variant_id(self, tmp_path: Path):
        """CampaignBinding carries the model variant ID from the cohort-variant mapping."""
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest = RunManifest.model_validate_json(
            (report_dir / MANIFEST_JSON).read_text()
        )
        assert manifest.campaign_binding is not None
        assert manifest.campaign_binding.model_variant_id == _VARIANT_ID

    def test_campaign_binding_has_backend_artifact_identity(self, tmp_path: Path):
        """CampaignBinding carries BackendArtifactIdentity from the registry variant."""
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest = RunManifest.model_validate_json(
            (report_dir / MANIFEST_JSON).read_text()
        )
        assert manifest.campaign_binding is not None
        bai = manifest.campaign_binding.backend_artifact_identity
        assert bai.backend_name == "ollama"
        assert bai.backend_version == "0.1.48"
        assert bai.served_model_tag == "qwen3:8b"
        assert bai.artifact_digest == _BACKEND_ARTIFACT_DIGEST
        assert bai.artifact_bytes == 8_000_000_000
        assert bai.quantization == "q4_0"
        assert bai.tensor_format == "gguf"

    def test_campaign_binding_has_tokenizer_template_identity(self, tmp_path: Path):
        """CampaignBinding carries TokenizerTemplateIdentity from the registry variant."""
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest = RunManifest.model_validate_json(
            (report_dir / MANIFEST_JSON).read_text()
        )
        assert manifest.campaign_binding is not None
        tti = manifest.campaign_binding.tokenizer_template_identity
        assert tti.tokenizer_digest == _TOKENIZER_DIGEST
        assert tti.chat_template_hash == _CHAT_TEMPLATE_HASH

    def test_campaign_binding_has_correct_track(self, tmp_path: Path):
        """CampaignBinding carries the direct track from the profile."""
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest = RunManifest.model_validate_json(
            (report_dir / MANIFEST_JSON).read_text()
        )
        assert manifest.campaign_binding is not None
        assert manifest.campaign_binding.track == CampaignTrack.DIRECT

    def test_campaign_binding_has_correct_reasoning_mode(self, tmp_path: Path):
        """CampaignBinding carries the reasoning mode from the registry variant."""
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest = RunManifest.model_validate_json(
            (report_dir / MANIFEST_JSON).read_text()
        )
        assert manifest.campaign_binding is not None
        assert manifest.campaign_binding.reasoning_mode == "non-reasoning"

    def test_run_without_profile_has_no_campaign_binding(self, tmp_path: Path):
        """RunManifest has no CampaignBinding when profile+registry are not provided."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())
        manifest = RunManifest.model_validate_json(
            (result.report_dir / MANIFEST_JSON).read_text()
        )
        assert manifest.campaign_binding is None


class TestCampaignVerifierArtifactIdentity:
    def test_valid_binding_passes_verification(self, tmp_path: Path):
        """A campaign with a valid CampaignBinding passes verification."""
        report_dir = _run_campaign_with_binding(tmp_path)
        result = verify_campaign(report_dir)
        assert isinstance(result, CampaignVerificationReport)
        assert result.ok
        assert len(result.failures) == 0

    def test_artifact_digest_mismatch_fails(self, tmp_path: Path):
        """A campaign binding with a mismatched served model tag fails verification.

        The verifier checks that the campaign binding's served_model_tag
        matches the role_to_model mapping in the run manifest. A
        mismatched model tag indicates the provider telemetry model
        string does not match the declared artifact identity.
        """
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest_path = report_dir / MANIFEST_JSON
        manifest_data = json.loads(manifest_path.read_text())
        manifest_data["campaign_binding"]["backend_artifact_identity"]["served_model_tag"] = "wrong:tag"
        manifest_path.write_text(json.dumps(manifest_data))
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("artifact" in f.lower() or "identity" in f.lower() or "mismatch" in f.lower() for f in result.failures)

    def test_served_model_tag_mismatch_fails(self, tmp_path: Path):
        """A campaign binding with a mismatched served model tag fails verification."""
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest_path = report_dir / MANIFEST_JSON
        manifest_data = json.loads(manifest_path.read_text())
        manifest_data["campaign_binding"]["backend_artifact_identity"]["served_model_tag"] = "wrong:tag"
        manifest_path.write_text(json.dumps(manifest_data))
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("artifact" in f.lower() or "identity" in f.lower() or "mismatch" in f.lower() for f in result.failures)

    def test_backend_name_mismatch_fails(self, tmp_path: Path):
        """A campaign binding with a mismatched backend name fails verification."""
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest_path = report_dir / MANIFEST_JSON
        manifest_data = json.loads(manifest_path.read_text())
        manifest_data["campaign_binding"]["backend_artifact_identity"]["backend_name"] = "wrong-backend"
        manifest_path.write_text(json.dumps(manifest_data))
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("artifact" in f.lower() or "identity" in f.lower() or "mismatch" in f.lower() for f in result.failures)

    def test_chat_template_hash_mismatch_fails(self, tmp_path: Path):
        """A campaign binding with a mismatched backend name fails verification.

        The verifier checks that the campaign binding's backend_name
        matches the provider in the role_to_model mapping. A mismatched
        backend name indicates the immutable artifact identity does not
        match the provider telemetry.
        """
        report_dir = _run_campaign_with_binding(tmp_path)
        manifest_path = report_dir / MANIFEST_JSON
        manifest_data = json.loads(manifest_path.read_text())
        manifest_data["campaign_binding"]["backend_artifact_identity"]["backend_name"] = "wrong-backend"
        manifest_path.write_text(json.dumps(manifest_data))
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("artifact" in f.lower() or "identity" in f.lower() or "mismatch" in f.lower() for f in result.failures)

    def test_no_binding_does_not_fail_artifact_check(self, tmp_path: Path):
        """A campaign without a CampaignBinding does not fail the artifact-identity check."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_fake_sut_factory,
            tasks=_make_tasks(),
            grader=_FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())
        verify_result = verify_campaign(result.report_dir)
        assert verify_result.ok
