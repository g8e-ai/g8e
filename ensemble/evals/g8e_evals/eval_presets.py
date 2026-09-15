# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Repository-owned presets for approved development shapes.

A preset supplies policy defaults (suite, task population, arm,
repetition, sampling, budget ceilings, stop conditions). It never
supplies authority, owner approval, evidence keys, provider endpoints,
model identities, report roots, or operation IDs. A preset is a policy
template, not live authority and not provider authorization.

The draft command consumes a named preset and explicit concise overrides
to produce a new config. The user supplies identity and authority fields;
the preset supplies the operational shape.
"""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field

from g8e_evals.operation_config import (
    BudgetCeilings,
    SamplingPolicy,
    StopConditions,
)


class DiagnosticPreset(BaseModel):
    """Policy defaults for a diagnostic operation. The user supplies
    ``operation_id``, ``revision``, ``report_root``, ``gold_set``,
    ``evidence_key``, ``provider_endpoint``, ``model_variant_id``, and
    optional overrides for any preset field.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    name: str = Field(min_length=1)
    description: str = Field(min_length=1)
    suite: str = Field(min_length=1)
    arm: str = Field(min_length=1)
    seed: int = Field(ge=0)
    task_limit: int | None = Field(default=None, gt=0)
    task_offset: int = Field(default=0, ge=0)
    budget: BudgetCeilings
    stop_conditions: StopConditions
    sampling: SamplingPolicy = Field(default_factory=SamplingPolicy)


class CampaignPreset(BaseModel):
    """Policy defaults for a campaign operation. The user supplies
    ``operation_id``, ``revision``, ``report_root``, ``gold_set``,
    ``evidence_key``, ``provider_endpoint``, ``campaign_id``,
    ``release_version``, ``preregistration``, ``profile``,
    ``model_registry``, ``arms``, ``cohort_ids``, and optional overrides
    for any preset field.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    name: str = Field(min_length=1)
    description: str = Field(min_length=1)
    suite: str = Field(min_length=1)
    seed: int = Field(ge=0)
    arms: list[str] = Field(min_length=1)
    repetitions: int = Field(default=1, gt=0)
    task_limit: int | None = Field(default=None, gt=0)
    task_offset: int = Field(default=0, ge=0)
    budget: BudgetCeilings
    stop_conditions: StopConditions
    publication_eligible: bool = True


# Repository-owned preset registry. Each preset is an approved
# development shape. Adding a preset requires updating this registry and
# the contract test that asserts the registry is exhaustive.

_DIAGNOSTIC_PRESETS: dict[str, DiagnosticPreset] = {
    "ifeval-five-task": DiagnosticPreset(
        name="ifeval-five-task",
        description=(
            "Five-task ifeval_subset diagnostic against one model and one "
            "arm. Suitable for quick smoke validation of a single model "
            "through the ensemble_ungoverned arm with a local Ollama "
            "provider. Budget: 30 requests, 491520 tokens, 0 USD."
        ),
        suite="ifeval_subset",
        arm="ensemble_ungoverned",
        seed=42,
        task_limit=5,
        budget=BudgetCeilings(
            max_requests=30,
            max_tokens=491520,
            max_usd=0,
            concurrency=1,
        ),
        stop_conditions=StopConditions(idle_timeout_s=180.0),
    ),
}

_CAMPAIGN_PRESETS: dict[str, CampaignPreset] = {
    "all-roles-governed": CampaignPreset(
        name="all-roles-governed",
        description=(
            "Five-task campaign running every declared model-role cohort "
            "through the full g8ee stack under the doctrine arm. Three "
            "repetitions, concurrency one. Budget: 20000 requests, "
            "200000000 tokens, 0 USD. Publication eligible."
        ),
        suite="ifeval_subset",
        seed=42,
        arms=["doctrine"],
        repetitions=3,
        task_limit=5,
        budget=BudgetCeilings(
            max_requests=20000,
            max_tokens=200000000,
            max_usd=0,
            concurrency=1,
            min_free_disk_gb=5,
        ),
        stop_conditions=StopConditions(
            idle_timeout_s=300.0,
            max_duration_s=86400.0,
        ),
        publication_eligible=True,
    ),
    "all-roles-governed-smoke": CampaignPreset(
        name="all-roles-governed-smoke",
        description=(
            "One-task campaign validating role-scoped cohorts through the "
            "full g8ee stack under the doctrine arm. One repetition, "
            "concurrency one. Budget: 100 requests, 2000000 tokens, 0 USD."
        ),
        suite="ifeval_subset",
        seed=42,
        arms=["doctrine"],
        repetitions=1,
        task_limit=1,
        budget=BudgetCeilings(
            max_requests=100,
            max_tokens=2000000,
            max_usd=0,
            concurrency=1,
            min_free_disk_gb=1,
        ),
        stop_conditions=StopConditions(
            idle_timeout_s=300.0,
            max_duration_s=3600.0,
        ),
        publication_eligible=False,
    ),
    "opendevops-development": CampaignPreset(
        name="opendevops-development",
        description=(
            "Development campaign over the ifeval_subset benchmark with "
            "direct and ensemble_ungoverned arms. Three repetitions per "
            "cohort, concurrency one, local Ollama provider. Budget: 100 "
            "requests, 1000000 tokens, 0 USD. Publication eligible."
        ),
        suite="ifeval_subset",
        seed=42,
        arms=["direct", "ensemble_ungoverned"],
        repetitions=3,
        budget=BudgetCeilings(
            max_requests=100,
            max_tokens=1000000,
            max_usd=0,
            concurrency=1,
        ),
        stop_conditions=StopConditions(idle_timeout_s=180.0),
        publication_eligible=True,
    ),
    "ifeval-full-120": CampaignPreset(
        name="ifeval-full-120",
        description=(
            "Full 120-task ifeval_subset campaign across all 25 instruction "
            "types with direct and ensemble_ungoverned arms. Three "
            "repetitions per cohort, concurrency one. Budget: 500 requests, "
            "5000000 tokens, 0 USD. Publication eligible."
        ),
        suite="ifeval_subset",
        seed=42,
        arms=["direct", "ensemble_ungoverned"],
        repetitions=3,
        budget=BudgetCeilings(
            max_requests=500,
            max_tokens=5000000,
            max_usd=0,
            concurrency=1,
        ),
        stop_conditions=StopConditions(idle_timeout_s=180.0),
        publication_eligible=True,
    ),
}


def get_diagnostic_preset(name: str) -> DiagnosticPreset:
    """Return the diagnostic preset with the given name. Raises
    ``KeyError`` if the name is not registered.
    """
    if name not in _DIAGNOSTIC_PRESETS:
        raise KeyError(f"unknown diagnostic preset: {name!r}; available: {list(_DIAGNOSTIC_PRESETS)}")
    return _DIAGNOSTIC_PRESETS[name]


def get_campaign_preset(name: str) -> CampaignPreset:
    """Return the campaign preset with the given name. Raises
    ``KeyError`` if the name is not registered.
    """
    if name not in _CAMPAIGN_PRESETS:
        raise KeyError(f"unknown campaign preset: {name!r}; available: {list(_CAMPAIGN_PRESETS)}")
    return _CAMPAIGN_PRESETS[name]


def list_diagnostic_presets() -> list[str]:
    """Return the sorted names of all registered diagnostic presets."""
    return sorted(_DIAGNOSTIC_PRESETS)


def list_campaign_presets() -> list[str]:
    """Return the sorted names of all registered campaign presets."""
    return sorted(_CAMPAIGN_PRESETS)
