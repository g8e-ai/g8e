# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Draft generation for diagnostic and campaign operation configs.

``generate_diagnostic_draft`` and ``generate_campaign_draft`` consume a
repository-owned named preset and explicit concise overrides to produce
a new config at a path that does not exist. Draft generation performs no
provider call, model pull, report-root creation, lease issuance, or
publication.

The generated config has no active lease and cannot be started; lease
lifecycle remains a separate typed record so the content-addressed config
does not mutate during authorization.
"""

from __future__ import annotations

from pathlib import Path

from pydantic import BaseModel, ConfigDict, Field

from g8e_evals.eval_presets import (
    get_campaign_preset,
    get_diagnostic_preset,
)
from g8e_evals.operation_config import (
    AuthorityRef,
    BudgetCeilings,
    CampaignConfig,
    DiagnosticConfig,
    EvidenceKeyRef,
    ProviderEndpointRef,
    SamplingPolicy,
    StopConditions,
    write_operation_config,
)


class DraftOverrides(BaseModel):
    """Concise user overrides applied on top of a preset. All fields are
    optional; the preset supplies defaults for any field not overridden.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    seed: int | None = None
    task_limit: int | None = None
    task_offset: int | None = None
    budget: BudgetCeilings | None = None
    stop_conditions: StopConditions | None = None
    sampling: SamplingPolicy | None = None


class DiagnosticDraftOverrides(DraftOverrides):
    """Overrides for a diagnostic draft. Adds diagnostic-specific fields."""

    arm: str | None = None


class CampaignDraftOverrides(DraftOverrides):
    """Overrides for a campaign draft. Adds campaign-specific fields."""

    arms: list[str] | None = None
    repetitions: int | None = None
    publication_eligible: bool | None = None


class DraftReviewSummary(BaseModel):
    """Bounded review summary printed after draft generation. Contains
    model count, dimensions, budget, report root, endpoint class, and
    every referenced authority hash. No secrets.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    operation_kind: str
    operation_id: str
    revision: str
    preset: str
    suite: str
    report_root: str
    content_hash: str
    dimensions: dict[str, object] = Field(default_factory=dict)
    budget: dict[str, object] = Field(default_factory=dict)
    authority_hashes: dict[str, str] = Field(default_factory=dict)
    endpoint_class: str
    provider: str


def _merge_override(preset_value: object, override: object) -> object:
    """Return the override when not None, otherwise the preset value."""
    return override if override is not None else preset_value


def generate_diagnostic_draft(
    preset_name: str,
    operation_id: str,
    revision: str,
    report_root: str,
    gold_set: AuthorityRef,
    evidence_key: EvidenceKeyRef,
    provider_endpoint: ProviderEndpointRef,
    model_variant_id: str,
    overrides: DiagnosticDraftOverrides | None = None,
    output_path: Path | None = None,
) -> tuple[DiagnosticConfig, DraftReviewSummary]:
    """Generate a diagnostic config from a preset and overrides.

    The config is written to ``output_path`` if supplied. The path must
    not exist; draft generation creates a new config only.

    Returns the finalized config and a bounded review summary.
    """
    preset = get_diagnostic_preset(preset_name)
    ov = overrides or DiagnosticDraftOverrides()

    config = DiagnosticConfig(
        operation_id=operation_id,
        revision=revision,
        suite=preset.suite,
        seed=_merge_override(preset.seed, ov.seed),
        report_root=report_root,
        gold_set=gold_set,
        evidence_key=evidence_key,
        provider_endpoint=provider_endpoint,
        budget=_merge_override(preset.budget, ov.budget),
        stop_conditions=_merge_override(preset.stop_conditions, ov.stop_conditions),
        sampling=_merge_override(preset.sampling, ov.sampling),
        model_variant_id=model_variant_id,
        arm=_merge_override(preset.arm, ov.arm),
        task_limit=_merge_override(preset.task_limit, ov.task_limit),
        task_offset=_merge_override(preset.task_offset, ov.task_offset),
    )
    finalized = config.finalized()

    if output_path is not None:
        if output_path.exists():
            raise FileExistsError(f"draft output path already exists: {output_path}")
        write_operation_config(finalized, output_path)

    summary = DraftReviewSummary(
        operation_kind="diagnostic",
        operation_id=operation_id,
        revision=revision,
        preset=preset_name,
        suite=config.suite,
        report_root=report_root,
        content_hash=finalized.content_hash,
        dimensions={
            "arm": config.arm,
            "model_variant_id": config.model_variant_id,
            "task_limit": config.task_limit,
            "task_offset": config.task_offset,
        },
        budget={
            "max_requests": config.budget.max_requests,
            "max_tokens": config.budget.max_tokens,
            "max_usd": config.budget.max_usd,
            "concurrency": config.budget.concurrency,
        },
        authority_hashes={
            "gold_set": config.gold_set.sha256,
        },
        endpoint_class=config.provider_endpoint.endpoint_class,
        provider=config.provider_endpoint.provider,
    )
    return finalized, summary


def generate_campaign_draft(
    preset_name: str,
    operation_id: str,
    revision: str,
    report_root: str,
    gold_set: AuthorityRef,
    evidence_key: EvidenceKeyRef,
    provider_endpoint: ProviderEndpointRef,
    campaign_id: str,
    release_version: str,
    preregistration: AuthorityRef,
    profile: AuthorityRef,
    model_registry: AuthorityRef,
    cohort_ids: list[str],
    model_tags: AuthorityRef | None = None,
    campaign_set_plan: AuthorityRef | None = None,
    replacement_rule: AuthorityRef | None = None,
    overrides: CampaignDraftOverrides | None = None,
    output_path: Path | None = None,
) -> tuple[CampaignConfig, DraftReviewSummary]:
    """Generate a campaign config from a preset and overrides.

    The config is written to ``output_path`` if supplied. The path must
    not exist; draft generation creates a new config only.

    Returns the finalized config and a bounded review summary.
    """
    preset = get_campaign_preset(preset_name)
    ov = overrides or CampaignDraftOverrides()

    config = CampaignConfig(
        operation_id=operation_id,
        revision=revision,
        suite=preset.suite,
        seed=_merge_override(preset.seed, ov.seed),
        report_root=report_root,
        gold_set=gold_set,
        evidence_key=evidence_key,
        provider_endpoint=provider_endpoint,
        budget=_merge_override(preset.budget, ov.budget),
        stop_conditions=_merge_override(preset.stop_conditions, ov.stop_conditions),
        campaign_id=campaign_id,
        release_version=release_version,
        preregistration=preregistration,
        profile=profile,
        model_registry=model_registry,
        model_tags=model_tags,
        arms=_merge_override(preset.arms, ov.arms),
        cohort_ids=cohort_ids,
        repetitions=_merge_override(preset.repetitions, ov.repetitions),
        task_limit=_merge_override(preset.task_limit, ov.task_limit),
        task_offset=_merge_override(preset.task_offset, ov.task_offset),
        campaign_set_plan=campaign_set_plan,
        replacement_rule=replacement_rule,
        publication_eligible=_merge_override(preset.publication_eligible, ov.publication_eligible),
    )
    finalized = config.finalized()

    if output_path is not None:
        if output_path.exists():
            raise FileExistsError(f"draft output path already exists: {output_path}")
        write_operation_config(finalized, output_path)

    authority_hashes: dict[str, str] = {
        "gold_set": config.gold_set.sha256,
        "preregistration": config.preregistration.sha256,
        "profile": config.profile.sha256,
        "model_registry": config.model_registry.sha256,
    }
    if config.model_tags is not None:
        authority_hashes["model_tags"] = config.model_tags.sha256
    if config.campaign_set_plan is not None:
        authority_hashes["campaign_set_plan"] = config.campaign_set_plan.sha256
    if config.replacement_rule is not None:
        authority_hashes["replacement_rule"] = config.replacement_rule.sha256

    summary = DraftReviewSummary(
        operation_kind="campaign",
        operation_id=operation_id,
        revision=revision,
        preset=preset_name,
        suite=config.suite,
        report_root=report_root,
        content_hash=finalized.content_hash,
        dimensions={
            "arms": config.arms,
            "cohort_count": len(config.cohort_ids),
            "cohort_ids": config.cohort_ids,
            "repetitions": config.repetitions,
            "task_limit": config.task_limit,
            "task_offset": config.task_offset,
            "publication_eligible": config.publication_eligible,
        },
        budget={
            "max_requests": config.budget.max_requests,
            "max_tokens": config.budget.max_tokens,
            "max_usd": config.budget.max_usd,
            "concurrency": config.budget.concurrency,
        },
        authority_hashes=authority_hashes,
        endpoint_class=config.provider_endpoint.endpoint_class,
        provider=config.provider_endpoint.provider,
    )
    return finalized, summary
