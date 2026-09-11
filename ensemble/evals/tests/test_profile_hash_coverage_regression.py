# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Phase 0 regression tests proving the campaign profile content-hash
defect.

``compute_campaign_profile_hash`` omits material profile fields from
canonical serialization. A frozen profile can therefore change
materially without changing its hash, invalidating downstream campaign
manifest hashes and any evidence bound to the old hash.

These tests are written to FAIL under the current implementation. They
pass once AUTH-1 replaces the hash contract so every material field
participates in canonical serialization and hash computation.

No external dependencies (no files, network, or DB).
"""

from __future__ import annotations

import pytest

from g8e_evals.profile import (
    CAMPAIGN_PROFILE_VERSION,
    CampaignLifecycleStatus,
    CampaignProfile,
    ClaimBoundary,
    ModelTierAssignment,
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
from g8e_evals.schema import CampaignTrack


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64


def _make_variant(variant_id: str = "qwen3-8b-q4_0") -> ModelVariant:
    return ModelVariant(
        variant_id=variant_id,
        canonical_display_name="Qwen3-8B",
        source_list_alias="Qwen3-8B",
        hf_repo="Qwen/Qwen3-8B",
        hf_sha="e966e34a21f2cb17a6c3e8bc2209a2faed9268fb",
        retrieval_date="2026-09-09T00:00:00Z",
        license_id="apache-2.0",
        license_text_hash=None,
        gated=False,
        publication_eligibility=PublicationEligibility.ELIGIBLE,
        parameter_count=8030261248,
        parameter_count_display="8.0B",
        architecture="QwenForCausalLM",
        model_type="qwen2",
        dtype="BF16",
        format="safetensors",
        quantization="q4_0",
        base_model="",
        context_length=32768,
        supported_modalities=["text"],
        reasoning_mode="switchable",
        tool_call_support=True,
        chat_template_family="qwen3",
        chat_template_hash=_VALID_HASH,
        tokenizer_digest=_VALID_HASH,
        weight_class=WeightClass.HEAVY_SLM,
        backend_name="ollama",
        backend_version="0.1.48",
        served_model_tag="qwen3:8b",
        artifact_digest=_VALID_HASH,
        artifact_bytes=4_800_000_000,
        tensor_format="gguf",
        hidden_reasoning_tokens=False,
    )


def _make_registry(variants: list[ModelVariant]) -> ModelRegistry:
    ch = compute_model_registry_hash("reg-1", "1", variants, [])
    return ModelRegistry(
        registry_id="reg-1",
        registry_version="1",
        schema_version=MODEL_REGISTRY_VERSION,
        created_at="2026-09-10T00:00:00Z",
        variants=variants,
        qualification_records=[],
        content_hash=ch,
    )


def _make_profile(
    *,
    task_ids: list[str] | None = None,
    temperature: float = 0.0,
    top_p: float = 1.0,
    max_tokens: int = 4096,
    seed: int = 42,
    context_limit: int = 32768,
    timeout_seconds: float = 120.0,
    max_retries: int = 1,
    warmup_excluded: bool = True,
    concurrency: int = 1,
    hardware_identity: str = "linux/amd64/rtx-4090",
    environment_stratum: str = "single-machine",
    provider_hardware_identity: str = "unavailable",
    provider_environment_stratum: str = "unavailable",
    primary_metrics: list[str] | None = None,
    unit_of_analysis: str = "task",
    claim_boundary: ClaimBoundary = ClaimBoundary.DESCRIPTIVE_ONLY,
    baseline_tier_mappings: dict[str, str] | None = None,
    routing_policy: str = "default",
    prompt_serialization_hash: str = _VALID_HASH,
    generative_variant_ids: list[str] | None = None,
    track_arm_assignments: list[TrackArmAssignment] | None = None,
    model_tier_assignments: list[ModelTierAssignment] | None = None,
    model_registry_hash: str = _VALID_HASH,
    required_record_policy_hash: str = "34b598bec81e407060fdcf3e3d9fc35c3ba6634487481fc53cf11cae20b031f6",
    created_at: str = "2026-09-10T00:00:00Z",
    lifecycle_status: CampaignLifecycleStatus = CampaignLifecycleStatus.DRAFT,
    purpose: str = "46-model generative comparison campaign",
    campaign_id: str = "generative-campaign-v1",
    campaign_revision: str = "1",
    schema_version: str = CAMPAIGN_PROFILE_VERSION,
    benchmark_ids: list[str] | None = None,
    dataset_hashes: list[str] | None = None,
    grader_hashes: list[str] | None = None,
    repetitions: int = 3,
) -> CampaignProfile:
    if generative_variant_ids is None:
        generative_variant_ids = ["qwen3-8b-q4_0"]
    if track_arm_assignments is None:
        track_arm_assignments = [
            TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
            TrackArmAssignment(track=CampaignTrack.TIER_FITNESS, arm_id="ensemble_ungoverned"),
        ]
    if model_tier_assignments is None:
        model_tier_assignments = [
            ModelTierAssignment(variant_id="qwen3-8b-q4_0", target_tier="primary"),
        ]
    if task_ids is None:
        task_ids = ["task-1", "task-2", "task-3"]
    if primary_metrics is None:
        primary_metrics = ["ifeval_subset_verifier"]
    if baseline_tier_mappings is None:
        baseline_tier_mappings = {
            "primary": "qwen3:8b",
            "assistant": "granite3.3:8b",
            "lite": "qwen3:0.6b",
        }
    if benchmark_ids is None:
        benchmark_ids = ["ifeval_subset"]
    if dataset_hashes is None:
        dataset_hashes = [_VALID_HASH]
    if grader_hashes is None:
        grader_hashes = [_VALID_HASH]
    temp = CampaignProfile.model_construct(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        schema_version=schema_version,
        purpose=purpose,
        created_at=created_at,
        lifecycle_status=lifecycle_status,
        generative_variant_ids=generative_variant_ids,
        benchmark_ids=benchmark_ids,
        dataset_hashes=dataset_hashes,
        grader_hashes=grader_hashes,
        prompt_serialization_hash=prompt_serialization_hash,
        task_ids=task_ids,
        repetitions=repetitions,
        track_arm_assignments=track_arm_assignments,
        model_tier_assignments=model_tier_assignments,
        baseline_tier_mappings=baseline_tier_mappings,
        routing_policy=routing_policy,
        temperature=temperature,
        top_p=top_p,
        max_tokens=max_tokens,
        seed=seed,
        context_limit=context_limit,
        timeout_seconds=timeout_seconds,
        max_retries=max_retries,
        warmup_excluded=warmup_excluded,
        concurrency=concurrency,
        hardware_identity=hardware_identity,
        environment_stratum=environment_stratum,
        provider_hardware_identity=provider_hardware_identity,
        provider_environment_stratum=provider_environment_stratum,
        primary_metrics=primary_metrics,
        unit_of_analysis=unit_of_analysis,
        claim_boundary=claim_boundary,
        model_registry_hash=model_registry_hash,
        required_record_policy_hash=required_record_policy_hash,
        content_hash="0" * 64,
    )
    ch = compute_campaign_profile_hash(temp)
    return CampaignProfile(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        schema_version=schema_version,
        purpose=purpose,
        created_at=created_at,
        lifecycle_status=lifecycle_status,
        generative_variant_ids=generative_variant_ids,
        benchmark_ids=benchmark_ids,
        dataset_hashes=dataset_hashes,
        grader_hashes=grader_hashes,
        prompt_serialization_hash=prompt_serialization_hash,
        task_ids=task_ids,
        repetitions=repetitions,
        track_arm_assignments=track_arm_assignments,
        model_tier_assignments=model_tier_assignments,
        baseline_tier_mappings=baseline_tier_mappings,
        routing_policy=routing_policy,
        temperature=temperature,
        top_p=top_p,
        max_tokens=max_tokens,
        seed=seed,
        context_limit=context_limit,
        timeout_seconds=timeout_seconds,
        max_retries=max_retries,
        warmup_excluded=warmup_excluded,
        concurrency=concurrency,
        hardware_identity=hardware_identity,
        environment_stratum=environment_stratum,
        provider_hardware_identity=provider_hardware_identity,
        provider_environment_stratum=provider_environment_stratum,
        primary_metrics=primary_metrics,
        unit_of_analysis=unit_of_analysis,
        claim_boundary=claim_boundary,
        model_registry_hash=model_registry_hash,
        required_record_policy_hash=required_record_policy_hash,
        content_hash=ch,
    )


class TestProfileHashOmitsTaskIds:
    """The hash does not cover task_ids."""

    def test_changing_task_ids_changes_hash(self):
        profile_a = _make_profile(task_ids=["task-1", "task-2", "task-3"])
        profile_b = _make_profile(task_ids=["task-1", "task-2", "task-3", "task-4"])
        assert profile_a.content_hash != profile_b.content_hash, (
            "task_ids must participate in the profile hash"
        )


class TestProfileHashOmitsPromptSerializationHash:
    """The hash does not cover prompt_serialization_hash."""

    def test_changing_prompt_serialization_hash_changes_hash(self):
        profile_a = _make_profile(prompt_serialization_hash="a" * 64)
        profile_b = _make_profile(prompt_serialization_hash="b" * 64)
        assert profile_a.content_hash != profile_b.content_hash, (
            "prompt_serialization_hash must participate in the profile hash"
        )


class TestProfileHashOmitsBaselineTierMappings:
    """The hash does not cover baseline_tier_mappings."""

    def test_changing_baseline_tier_mappings_changes_hash(self):
        profile_a = _make_profile(baseline_tier_mappings={"primary": "qwen3:8b", "assistant": "granite3.3:8b", "lite": "qwen3:0.6b"})
        profile_b = _make_profile(baseline_tier_mappings={"primary": "qwen3:8b", "assistant": "phi4:3b", "lite": "qwen3:0.6b"})
        assert profile_a.content_hash != profile_b.content_hash, (
            "baseline_tier_mappings must participate in the profile hash"
        )


class TestProfileHashOmitsRoutingPolicy:
    """The hash does not cover routing_policy."""

    def test_changing_routing_policy_changes_hash(self):
        profile_a = _make_profile(routing_policy="default")
        profile_b = _make_profile(routing_policy="strict")
        assert profile_a.content_hash != profile_b.content_hash, (
            "routing_policy must participate in the profile hash"
        )


class TestProfileHashOmitsTemperature:
    """The hash does not cover temperature."""

    def test_changing_temperature_changes_hash(self):
        profile_a = _make_profile(temperature=0.0)
        profile_b = _make_profile(temperature=0.7)
        assert profile_a.content_hash != profile_b.content_hash, (
            "temperature must participate in the profile hash"
        )


class TestProfileHashOmitsTopP:
    """The hash does not cover top_p."""

    def test_changing_top_p_changes_hash(self):
        profile_a = _make_profile(top_p=1.0)
        profile_b = _make_profile(top_p=0.9)
        assert profile_a.content_hash != profile_b.content_hash, (
            "top_p must participate in the profile hash"
        )


class TestProfileHashOmitsMaxTokens:
    """The hash does not cover max_tokens."""

    def test_changing_max_tokens_changes_hash(self):
        profile_a = _make_profile(max_tokens=4096)
        profile_b = _make_profile(max_tokens=8192)
        assert profile_a.content_hash != profile_b.content_hash, (
            "max_tokens must participate in the profile hash"
        )


class TestProfileHashOmitsSeed:
    """The hash does not cover seed."""

    def test_changing_seed_changes_hash(self):
        profile_a = _make_profile(seed=42)
        profile_b = _make_profile(seed=99)
        assert profile_a.content_hash != profile_b.content_hash, (
            "seed must participate in the profile hash"
        )


class TestProfileHashOmitsContextLimit:
    """The hash does not cover context_limit."""

    def test_changing_context_limit_changes_hash(self):
        profile_a = _make_profile(context_limit=32768)
        profile_b = _make_profile(context_limit=16384)
        assert profile_a.content_hash != profile_b.content_hash, (
            "context_limit must participate in the profile hash"
        )


class TestProfileHashOmitsTimeoutSeconds:
    """The hash does not cover timeout_seconds."""

    def test_changing_timeout_seconds_changes_hash(self):
        profile_a = _make_profile(timeout_seconds=120.0)
        profile_b = _make_profile(timeout_seconds=300.0)
        assert profile_a.content_hash != profile_b.content_hash, (
            "timeout_seconds must participate in the profile hash"
        )


class TestProfileHashOmitsMaxRetries:
    """The hash does not cover max_retries."""

    def test_changing_max_retries_changes_hash(self):
        profile_a = _make_profile(max_retries=1)
        profile_b = _make_profile(max_retries=3)
        assert profile_a.content_hash != profile_b.content_hash, (
            "max_retries must participate in the profile hash"
        )


class TestProfileHashOmitsWarmupExcluded:
    """The hash does not cover warmup_excluded."""

    def test_changing_warmup_excluded_changes_hash(self):
        profile_a = _make_profile(warmup_excluded=True)
        profile_b = _make_profile(warmup_excluded=False)
        assert profile_a.content_hash != profile_b.content_hash, (
            "warmup_excluded must participate in the profile hash"
        )


class TestProfileHashOmitsConcurrency:
    """The hash does not cover concurrency."""

    def test_changing_concurrency_changes_hash(self):
        profile_a = _make_profile(concurrency=1)
        profile_b = _make_profile(concurrency=4)
        assert profile_a.content_hash != profile_b.content_hash, (
            "concurrency must participate in the profile hash"
        )


class TestProfileHashOmitsHardwareIdentity:
    """The hash does not cover hardware_identity."""

    def test_changing_hardware_identity_changes_hash(self):
        profile_a = _make_profile(hardware_identity="linux/amd64/rtx-4090")
        profile_b = _make_profile(hardware_identity="linux/amd64/cpu")
        assert profile_a.content_hash != profile_b.content_hash, (
            "hardware_identity must participate in the profile hash"
        )


class TestProfileHashOmitsEnvironmentStratum:
    """The hash does not cover environment_stratum."""

    def test_changing_environment_stratum_changes_hash(self):
        profile_a = _make_profile(environment_stratum="single-machine")
        profile_b = _make_profile(environment_stratum="multi-machine")
        assert profile_a.content_hash != profile_b.content_hash, (
            "environment_stratum must participate in the profile hash"
        )


class TestProfileHashOmitsPrimaryMetrics:
    """The hash does not cover primary_metrics."""

    def test_changing_primary_metrics_changes_hash(self):
        profile_a = _make_profile(primary_metrics=["ifeval_subset_verifier"])
        profile_b = _make_profile(primary_metrics=["ifeval_subset_verifier", "latency_p50"])
        assert profile_a.content_hash != profile_b.content_hash, (
            "primary_metrics must participate in the profile hash"
        )


class TestProfileHashOmitsUnitOfAnalysis:
    """The hash does not cover unit_of_analysis."""

    def test_changing_unit_of_analysis_changes_hash(self):
        profile_a = _make_profile(unit_of_analysis="task")
        profile_b = _make_profile(unit_of_analysis="assignment")
        assert profile_a.content_hash != profile_b.content_hash, (
            "unit_of_analysis must participate in the profile hash"
        )


class TestProfileHashOmitsClaimBoundary:
    """The hash does not cover claim_boundary."""

    def test_changing_claim_boundary_changes_hash(self):
        profile_a = _make_profile(claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY)
        profile_b = _make_profile(claim_boundary=ClaimBoundary.CONFIRMATORY)
        assert profile_a.content_hash != profile_b.content_hash, (
            "claim_boundary must participate in the profile hash"
        )


class TestProfileHashOmitsCreatedAt:
    """The hash does not cover created_at."""

    def test_changing_created_at_changes_hash(self):
        profile_a = _make_profile(created_at="2026-09-10T00:00:00Z")
        profile_b = _make_profile(created_at="2026-09-11T00:00:00Z")
        assert profile_a.content_hash != profile_b.content_hash, (
            "created_at must participate in the profile hash"
        )


class TestProfileHashOmitsLifecycleStatus:
    """The hash does not cover lifecycle_status."""

    def test_changing_lifecycle_status_changes_hash(self):
        profile_a = _make_profile(lifecycle_status=CampaignLifecycleStatus.DRAFT)
        profile_b = _make_profile(lifecycle_status=CampaignLifecycleStatus.FROZEN)
        assert profile_a.content_hash != profile_b.content_hash, (
            "lifecycle_status must participate in the profile hash"
        )


class TestProfileHashOmitsPurpose:
    """The hash does not cover purpose."""

    def test_changing_purpose_changes_hash(self):
        profile_a = _make_profile(purpose="46-model generative comparison campaign")
        profile_b = _make_profile(purpose="31-model IFEval expanded campaign")
        assert profile_a.content_hash != profile_b.content_hash, (
            "purpose must participate in the profile hash"
        )


class TestProfileHashOmitsCampaignId:
    """The hash does not cover campaign_id."""

    def test_changing_campaign_id_changes_hash(self):
        profile_a = _make_profile(campaign_id="generative-campaign-v1")
        profile_b = _make_profile(campaign_id="generative-campaign-v2")
        assert profile_a.content_hash != profile_b.content_hash, (
            "campaign_id must participate in the profile hash"
        )


class TestProfileHashOmitsCampaignRevision:
    """The hash does not cover campaign_revision."""

    def test_changing_campaign_revision_changes_hash(self):
        profile_a = _make_profile(campaign_revision="1")
        profile_b = _make_profile(campaign_revision="2")
        assert profile_a.content_hash != profile_b.content_hash, (
            "campaign_revision must participate in the profile hash"
        )


class TestProfileHashOmitsSchemaVersion:
    """The hash does not cover schema_version."""

    def test_changing_schema_version_changes_hash(self):
        profile_a = _make_profile(schema_version="1.0.0")
        profile_b = _make_profile(schema_version="1.1.0")
        assert profile_a.content_hash != profile_b.content_hash, (
            "schema_version must participate in the profile hash"
        )


class TestProfileHashOmitsBenchmarkIds:
    """The hash does not cover benchmark_ids."""

    def test_changing_benchmark_ids_changes_hash(self):
        profile_a = _make_profile(benchmark_ids=["ifeval_subset"])
        profile_b = _make_profile(benchmark_ids=["ifeval_subset", "ifeval_expanded"])
        assert profile_a.content_hash != profile_b.content_hash, (
            "benchmark_ids must participate in the profile hash"
        )


class TestProfileHashOmitsDatasetHashes:
    """The hash does not cover dataset_hashes."""

    def test_changing_dataset_hashes_changes_hash(self):
        profile_a = _make_profile(dataset_hashes=["a" * 64])
        profile_b = _make_profile(dataset_hashes=["b" * 64])
        assert profile_a.content_hash != profile_b.content_hash, (
            "dataset_hashes must participate in the profile hash"
        )


class TestProfileHashOmitsGraderHashes:
    """The hash does not cover grader_hashes."""

    def test_changing_grader_hashes_changes_hash(self):
        profile_a = _make_profile(grader_hashes=["a" * 64])
        profile_b = _make_profile(grader_hashes=["b" * 64])
        assert profile_a.content_hash != profile_b.content_hash, (
            "grader_hashes must participate in the profile hash"
        )


class TestProfileHashOmitsGenerativeVariantIds:
    """The hash does not cover generative_variant_ids."""

    def test_changing_generative_variant_ids_changes_hash(self):
        profile_a = _make_profile(generative_variant_ids=["qwen3-8b-q4_0"])
        profile_b = _make_profile(generative_variant_ids=["qwen3-8b-q4_0", "granite-33-8b-instruct"])
        assert profile_a.content_hash != profile_b.content_hash, (
            "generative_variant_ids must participate in the profile hash"
        )


class TestProfileHashOmitsRepetitions:
    """The hash does not cover repetitions."""

    def test_changing_repetitions_changes_hash(self):
        profile_a = _make_profile(repetitions=3)
        profile_b = _make_profile(repetitions=5)
        assert profile_a.content_hash != profile_b.content_hash, (
            "repetitions must participate in the profile hash"
        )


class TestProfileHashOmitsTrackArmAssignments:
    """The hash does not cover track_arm_assignments."""

    def test_changing_track_arm_assignments_changes_hash(self):
        profile_a = _make_profile(
            track_arm_assignments=[
                TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
                TrackArmAssignment(track=CampaignTrack.TIER_FITNESS, arm_id="ensemble_ungoverned"),
            ]
        )
        profile_b = _make_profile(
            track_arm_assignments=[
                TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
                TrackArmAssignment(track=CampaignTrack.TIER_FITNESS, arm_id="doctrine"),
            ]
        )
        assert profile_a.content_hash != profile_b.content_hash, (
            "track_arm_assignments must participate in the profile hash"
        )


class TestProfileHashOmitsModelTierAssignments:
    """The hash does not cover model_tier_assignments."""

    def test_changing_model_tier_assignments_changes_hash(self):
        profile_a = _make_profile(
            model_tier_assignments=[
                ModelTierAssignment(variant_id="qwen3-8b-q4_0", target_tier="primary"),
            ]
        )
        profile_b = _make_profile(
            model_tier_assignments=[
                ModelTierAssignment(variant_id="qwen3-8b-q4_0", target_tier="assistant"),
            ]
        )
        assert profile_a.content_hash != profile_b.content_hash, (
            "model_tier_assignments must participate in the profile hash"
        )


class TestProfileHashOmitsModelRegistryHash:
    """The hash does not cover model_registry_hash."""

    def test_changing_model_registry_hash_changes_hash(self):
        profile_a = _make_profile(model_registry_hash="a" * 64)
        profile_b = _make_profile(model_registry_hash="b" * 64)
        assert profile_a.content_hash != profile_b.content_hash, (
            "model_registry_hash must participate in the profile hash"
        )


class TestProfileHashOmitsProviderHardwareIdentity:
    """The hash does not cover provider_hardware_identity."""

    def test_changing_provider_hardware_identity_changes_hash(self):
        profile_a = _make_profile(provider_hardware_identity="unavailable")
        profile_b = _make_profile(provider_hardware_identity="linux/amd64/a100")
        assert profile_a.content_hash != profile_b.content_hash, (
            "provider_hardware_identity must participate in the profile hash"
        )


class TestProfileHashOmitsProviderEnvironmentStratum:
    """The hash does not cover provider_environment_stratum."""

    def test_changing_provider_environment_stratum_changes_hash(self):
        profile_a = _make_profile(provider_environment_stratum="unavailable")
        profile_b = _make_profile(provider_environment_stratum="single-gpu")
        assert profile_a.content_hash != profile_b.content_hash, (
            "provider_environment_stratum must participate in the profile hash"
        )


class TestProfileHashOmitsRequiredRecordPolicyHash:
    """The hash does not cover required_record_policy_hash."""

    def test_changing_required_record_policy_hash_changes_hash(self):
        profile_a = _make_profile(required_record_policy_hash="34b598bec81e407060fdcf3e3d9fc35c3ba6634487481fc53cf11cae20b031f6")
        profile_b = _make_profile(required_record_policy_hash="1" * 64)
        assert profile_a.content_hash != profile_b.content_hash, (
            "required_record_policy_hash must participate in the profile hash"
        )


class TestProfileHashDistinctFromOrchestratorScope:
    """Provider and orchestrator hardware identities are distinct scopes.

    The provider hardware identity is never copied from the orchestrator
    identity. A profile that sets both to the same value must produce a
    different hash from one where the provider identity is 'unavailable',
    proving the two fields are independently bound.
    """

    def test_provider_identity_same_as_orchestrator_differs_from_unavailable(self):
        profile_a = _make_profile(
            hardware_identity="linux/amd64/rtx-4090",
            provider_hardware_identity="unavailable",
        )
        profile_b = _make_profile(
            hardware_identity="linux/amd64/rtx-4090",
            provider_hardware_identity="linux/amd64/rtx-4090",
        )
        assert profile_a.content_hash != profile_b.content_hash, (
            "provider_hardware_identity must be independently bound, not copied from hardware_identity"
        )
