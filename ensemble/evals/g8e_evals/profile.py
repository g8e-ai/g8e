# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed campaign profile for the 46-model comparison campaign.

The campaign profile records the campaign ID, revision, schema version,
purpose, creation time, lifecycle status, exact generative model-variant
IDs, benchmark IDs, dataset hashes, grader hashes, prompt serialization
hash, task IDs, repetitions, track-to-arm assignments, model-to-tier
assignments, fixed baseline mappings and routing policy, fully effective
sampling/context/timeout/retry settings, warm-up and concurrency
policies, both orchestrator-host and provider-host environment scopes,
primary metrics, unit of analysis, claim boundary, the frozen model
registry hash, and the frozen expected-record policy hash.

The profile is frozen and hashed before the first measured run. Any
material change creates a new campaign revision; it does not mutate
completed evidence.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
import random
from enum import StrEnum
from typing import TYPE_CHECKING, Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

if TYPE_CHECKING:
    from g8e_evals.registry import ModelRegistry

from g8e_evals.schema import TrackArmAssignment


CAMPAIGN_PROFILE_VERSION = "1.0.0"

_VALID_TIER_NAMES = frozenset({"primary", "assistant", "lite"})
_VALID_ARM_IDS = frozenset({"direct", "ensemble_ungoverned", "doctrine"})


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class CampaignLifecycleStatus(StrEnum):
    """Lifecycle status of a campaign profile.

    ``DRAFT``: The profile is being authored and may change.
    ``FROZEN``: The profile is frozen and hashed; no further changes
    are permitted without creating a new revision.
    ``ACTIVE``: The campaign is executing under this profile.
    ``COMPLETED``: The campaign has finished execution.
    ``SUPERSEDED``: A newer revision has replaced this profile.
    """

    DRAFT = "draft"
    FROZEN = "frozen"
    ACTIVE = "active"
    COMPLETED = "completed"
    SUPERSEDED = "superseded"


class ClaimBoundary(StrEnum):
    """Statistical claim boundary for the campaign.

    ``DESCRIPTIVE_ONLY``: The benchmark population is too small or
    incomplete for confirmatory inference. Results are descriptive
    measurements without superiority claims.
    ``CONFIRMATORY``: The preregistered paired analysis with multiplicity
    control supports confirmatory pairwise or winner claims.
    """

    DESCRIPTIVE_ONLY = "descriptive_only"
    CONFIRMATORY = "confirmatory"


class ModelTierAssignment(BaseModel):
    """One model-to-tier assignment for the tier-fitness track.

    Binds a model variant to the g8ee model tier it replaces in the
    tier-fitness track. Each variant appears at most once.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    variant_id: str = Field(min_length=1, description="Model variant ID from the registry.")
    target_tier: str = Field(min_length=1, description="Target g8ee tier (primary, assistant, or lite).")


class CampaignProfile(BaseModel):
    """Frozen typed campaign profile.

    Binds the campaign identity, revision, schema version, purpose,
    lifecycle status, generative variant IDs, benchmark and dataset
    identities, task population, repetitions, track-to-arm and
    model-to-tier assignments, baseline mappings, routing policy,
    effective sampling/context/timeout/retry settings, warm-up and
    concurrency policies, orchestrator-host and provider-host
    environment scopes, primary metrics, unit of analysis, claim
    boundary, model registry hash, and expected-record policy hash.

    The ``content_hash`` is SHA-256 over canonical JSON of the profile.
    Changing any field changes the hash and invalidates downstream
    campaign manifest hashes.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_revision: str = Field(min_length=1, description="Campaign revision identifier.")
    schema_version: str = Field(min_length=1, description="Profile schema version.")
    purpose: str = Field(min_length=1, description="Human-readable campaign purpose.")
    created_at: str = Field(min_length=1, description="ISO 8601 creation timestamp.")
    lifecycle_status: CampaignLifecycleStatus = Field(description="Lifecycle status of the profile.")

    generative_variant_ids: list[str] = Field(
        min_length=1,
        description="Exact generative model-variant IDs from the registry.",
    )
    benchmark_ids: list[str] = Field(
        min_length=1,
        description="Benchmark suite IDs (e.g. ifeval_subset).",
    )
    dataset_hashes: list[str] = Field(
        min_length=1,
        description="SHA-256 hashes of the benchmark datasets.",
    )
    grader_hashes: list[str] = Field(
        min_length=1,
        description="SHA-256 hashes of the grader implementations.",
    )
    prompt_serialization_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the prompt serialization format.",
    )
    task_ids: list[str] = Field(
        min_length=1,
        description="Exact task IDs in the campaign population.",
    )
    repetitions: int = Field(ge=1, description="Number of repetitions per assignment.")

    track_arm_assignments: list[TrackArmAssignment] = Field(
        min_length=1,
        description="Track-to-arm assignments. Each track appears at most once.",
    )
    model_tier_assignments: list[ModelTierAssignment] = Field(
        default_factory=list,
        description="Model-to-tier assignments for the tier-fitness track. Each variant appears at most once.",
    )
    baseline_tier_mappings: dict[str, str] = Field(
        default_factory=dict,
        description="Fixed baseline model mappings for non-target tiers (tier -> model tag).",
    )
    routing_policy: str = Field(
        default="default",
        description="Routing policy label for non-target tier mappings.",
    )

    temperature: float = Field(ge=0.0, le=2.0, description="Sampling temperature.")
    top_p: float = Field(ge=0.0, le=1.0, description="Nucleus sampling threshold.")
    max_tokens: int = Field(ge=1, description="Maximum output tokens.")
    seed: int = Field(ge=0, description="Deterministic sampling seed.")
    context_limit: int = Field(ge=1, description="Maximum context length in tokens.")
    timeout_seconds: float = Field(gt=0.0, description="Request timeout in seconds.")
    max_retries: int = Field(ge=0, description="Maximum infrastructure retries per assignment.")

    warmup_excluded: bool = Field(default=True, description="Whether warm-up requests are excluded from measured cells.")
    concurrency: int = Field(default=1, ge=1, description="Concurrency level for campaign execution.")

    hardware_identity: str = Field(
        min_length=1,
        description="Orchestrator-host hardware identity (e.g. linux/amd64/rtx-4090). This describes the local machine running the campaign runner, not the remote provider hardware.",
    )
    environment_stratum: str = Field(
        min_length=1,
        description="Orchestrator-host environment stratum label (e.g. single-machine, multi-machine).",
    )
    provider_hardware_identity: str = Field(
        default="unavailable",
        description="Provider-host hardware identity. Attested when the remote provider exposes hardware metadata; 'unavailable' when the remote boundary does not expose it. Never copied from the orchestrator-host identity.",
    )
    provider_environment_stratum: str = Field(
        default="unavailable",
        description="Provider-host environment stratum label. Attested when the remote provider exposes environment metadata; 'unavailable' when the remote boundary does not expose it.",
    )

    primary_metrics: list[str] = Field(
        min_length=1,
        description="Primary metric IDs for the campaign.",
    )
    unit_of_analysis: str = Field(
        min_length=1,
        description="Primary independent sampling unit (e.g. task).",
    )
    claim_boundary: ClaimBoundary = Field(description="Statistical claim boundary.")

    model_registry_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the frozen model registry.",
    )
    required_record_policy_hash: str = Field(
        default="34b598bec81e407060fdcf3e3d9fc35c3ba6634487481fc53cf11cae20b031f6",
        min_length=64, max_length=64,
        description="SHA-256 of the frozen expected-record policy that declares applicability and exact cardinality or derivation rules by suite/scenario/attempt/inference. The default is the hash of 'no_required_record_policy_v1' for Phase 1; Beacon's Phase 2 ExpectedRecordPolicy replaces it.",
    )
    instrumentation_policy_hash: str = Field(
        default="ae002706076404cef6e22ebfbae087ff430436094849bdf4e8e5d074f0bab1dd",
        min_length=64, max_length=64,
        description="SHA-256 of the frozen instrumentation policy that declares what the runner and SUT instrument per inference. The default is the hash of 'no_instrumentation_policy_v1' for Phase 1; Helix's Phase 2 instrumentation policy replaces it.",
    )

    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the profile.",
    )

    @model_validator(mode="after")
    def _validate_profile(self) -> Self:
        if len(self.generative_variant_ids) != len(set(self.generative_variant_ids)):
            seen: set[str] = set()
            dupes: list[str] = []
            for vid in self.generative_variant_ids:
                if vid in seen:
                    dupes.append(vid)
                seen.add(vid)
            raise ValueError(f"duplicate generative_variant_id: {sorted(set(dupes))}")

        if len(self.benchmark_ids) != len(set(self.benchmark_ids)):
            seen_b: set[str] = set()
            dupes_b: list[str] = []
            for bid in self.benchmark_ids:
                if bid in seen_b:
                    dupes_b.append(bid)
                seen_b.add(bid)
            raise ValueError(f"duplicate benchmark_id: {sorted(set(dupes_b))}")

        if len(self.dataset_hashes) != len(set(self.dataset_hashes)):
            seen_d: set[str] = set()
            dupes_d: list[str] = []
            for dh in self.dataset_hashes:
                if dh in seen_d:
                    dupes_d.append(dh)
                seen_d.add(dh)
            raise ValueError(f"duplicate dataset_hash: {sorted(set(dupes_d))}")

        if len(self.grader_hashes) != len(set(self.grader_hashes)):
            seen_g: set[str] = set()
            dupes_g: list[str] = []
            for gh in self.grader_hashes:
                if gh in seen_g:
                    dupes_g.append(gh)
                seen_g.add(gh)
            raise ValueError(f"duplicate grader_hash: {sorted(set(dupes_g))}")

        if len(self.task_ids) != len(set(self.task_ids)):
            seen_t: set[str] = set()
            dupes_t: list[str] = []
            for tid in self.task_ids:
                if tid in seen_t:
                    dupes_t.append(tid)
                seen_t.add(tid)
            raise ValueError(f"duplicate task_id: {sorted(set(dupes_t))}")

        track_keys = [a.track for a in self.track_arm_assignments]
        if len(track_keys) != len(set(track_keys)):
            raise ValueError(
                f"duplicate track in track_arm_assignments: {track_keys}"
            )

        for assignment in self.track_arm_assignments:
            if assignment.arm_id not in _VALID_ARM_IDS:
                raise ValueError(
                    f"unknown arm_id in track_arm_assignments: {assignment.arm_id!r}"
                )

        tier_variant_ids = [a.variant_id for a in self.model_tier_assignments]
        if len(tier_variant_ids) != len(set(tier_variant_ids)):
            raise ValueError(
                f"duplicate variant_id in model_tier_assignments: {tier_variant_ids}"
            )

        for assignment in self.model_tier_assignments:
            if assignment.target_tier not in _VALID_TIER_NAMES:
                raise ValueError(
                    f"target_tier must be one of {sorted(_VALID_TIER_NAMES)}: "
                    f"got {assignment.target_tier!r}"
                )

        expected = compute_campaign_profile_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"campaign profile content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected!r}"
            )
        return self

    def validate_against_registry(self, registry: ModelRegistry) -> None:
        """Validate that this profile is consistent with the given registry.

        Checks that the model registry hash matches, every generative
        variant ID exists in the registry, and every model-to-tier
        assignment references a variant in the registry. Raises
        ``ValueError`` on any inconsistency.
        """
        if self.model_registry_hash != registry.content_hash:
            raise ValueError(
                f"model_registry_hash mismatch: profile declares {self.model_registry_hash!r}, "
                f"registry has {registry.content_hash!r}"
            )

        registry_variant_ids = {v.variant_id for v in registry.variants}

        for vid in self.generative_variant_ids:
            if vid not in registry_variant_ids:
                raise ValueError(
                    f"generative_variant_id {vid!r} not found in registry"
                )

        for assignment in self.model_tier_assignments:
            if assignment.variant_id not in registry_variant_ids:
                raise ValueError(
                    f"model_tier_assignment variant_id {assignment.variant_id!r} "
                    f"not found in registry"
                )
            if assignment.variant_id not in set(self.generative_variant_ids):
                raise ValueError(
                    f"model_tier_assignment variant_id {assignment.variant_id!r} "
                    f"not in generative_variant_ids"
                )


def compute_campaign_profile_hash(profile: CampaignProfile) -> str:
    """Compute the content hash for a campaign profile from all material fields.

    Serializes every material field of the profile (excluding
    ``content_hash`` itself) into canonical JSON and returns SHA-256.
    The same function is used by the model validator (validation) and
    by profile builders (construction), so a field added to
    ``CampaignProfile`` is automatically included in the hash without
    a separate update site.

    The profile instance may be constructed via ``model_construct()``
    with a placeholder ``content_hash``; this function ignores that
    field during serialization.
    """
    data = profile.model_dump(mode="json", by_alias=True)
    data.pop("content_hash", None)
    payload = json.dumps(
        data,
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def compute_profile_schedule(
    profile: CampaignProfile,
    measured_variant_ids: list[str],
) -> list[str]:
    """Compute the deterministic Fisher-Yates schedule from the profile's seed.

    Builds the Cartesian product of (variant, task, arm, repetition),
    shuffles the canonical slot order with a Fisher-Yates shuffle seeded
    by ``profile.seed``, and returns the ordered assignment IDs. The same
    seed always produces the same order; a different seed produces a
    different order.

    Each assignment ID is a deterministic SHA-256 over canonical JSON of
    (campaign_id, variant_id, task_id, arm_id, repetition).
    """
    canonical_slots: list[tuple[str, str, str, int]] = []
    for variant_id in sorted(measured_variant_ids):
        for task_id in profile.task_ids:
            for assignment in profile.track_arm_assignments:
                for rep in range(1, profile.repetitions + 1):
                    canonical_slots.append((variant_id, task_id, assignment.arm_id, rep))

    rng = random.Random(profile.seed)
    indices = list(range(len(canonical_slots)))
    rng.shuffle(indices)

    ordered_ids: list[str] = []
    for idx in indices:
        variant_id, task_id, arm_id, rep = canonical_slots[idx]
        payload = json.dumps(
            {
                "campaign_id": profile.campaign_id,
                "variant_id": variant_id,
                "task_id": task_id,
                "arm_id": arm_id,
                "repetition": rep,
            },
            allow_nan=False,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        ordered_ids.append(_sha256(payload))

    return ordered_ids


__all__ = [
    "CAMPAIGN_PROFILE_VERSION",
    "CampaignLifecycleStatus",
    "CampaignProfile",
    "ClaimBoundary",
    "ModelTierAssignment",
    "TrackArmAssignment",
    "compute_campaign_profile_hash",
    "compute_profile_schedule",
]
