# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Shared fixture builders for campaign-set plans and replacement rules.

Used by ``test_replacement_rule.py`` and
``test_campaign_run_identity_provenance.py`` to construct typed
``CampaignSetPlan`` and ``ReplacementManifestRule`` instances bound to
each other.
"""

from __future__ import annotations

from g8e_evals.campaign_set import (
    CHILD_COUNT,
    REPETITION_COUNT,
    TOTAL_IFEVAL_TASKS,
    CampaignSetPlan,
    build_campaign_set_plan,
)
from g8e_evals.replacement_rule import (
    DEAD_EVIDENCE_DISPOSITION,
    REPLACEMENT_CHILD_DERIVATION_RULE,
    REPLACEMENT_RULE_SCHEMA_VERSION,
    ReplacementManifestRule,
    compute_replacement_rule_hash,
)

_VALID_HASH = "a" * 64
_PARENT_CAMPAIGN_ID = "generative-campaign-v1"


def make_campaign_set_plan(
    *,
    parent_campaign_id: str = _PARENT_CAMPAIGN_ID,
) -> CampaignSetPlan:
    population_task_ids = [f"task-{i:04d}" for i in range(TOTAL_IFEVAL_TASKS)]
    return build_campaign_set_plan(
        set_id="p12-ifeval-expanded-v1",
        parent_campaign_id=parent_campaign_id,
        parent_campaign_revision="1",
        population_task_ids=population_task_ids,
        population_hash=_VALID_HASH,
        model_registry_hash=_VALID_HASH,
        campaign_profile_hash=_VALID_HASH,
        repetition_ids=[f"{i}" for i in range(1, REPETITION_COUNT + 1)],
        seed=42,
        retry_policy_hash=_VALID_HASH,
        budget_authority_hash=_VALID_HASH,
        instrumentation_policy_hash=_VALID_HASH,
        expected_record_policy_hash=_VALID_HASH,
        orchestrator_environment_scope="linux/amd64/cpu",
        provider_environment_scope="unavailable",
        child_revisions=[f"rev-{i + 1}" for i in range(CHILD_COUNT)],
    )


def make_replacement_rule(
    *,
    plan: CampaignSetPlan,
    rule_id: str = "p12-replacement-rule-v1",
    max_attempts: int = 3,
    replaceable_child_ids: list[str] | None = None,
    set_plan_hash: str | None = None,
    set_id: str | None = None,
) -> ReplacementManifestRule:
    if replaceable_child_ids is None:
        replaceable_child_ids = [cp.child_id for cp in plan.child_plans]
    fields = {
        "rule_id": rule_id,
        "rule_version": REPLACEMENT_RULE_SCHEMA_VERSION,
        "purpose": "Authorize deterministic replacement runs for interrupted P12 children.",
        "created_at": "2026-09-12T00:00:00Z",
        "set_id": set_id or plan.set_id,
        "set_plan_hash": set_plan_hash or plan.content_hash,
        "replaceable_child_ids": list(replaceable_child_ids),
        "max_attempts_per_child": max_attempts,
        "derivation_rule": REPLACEMENT_CHILD_DERIVATION_RULE,
        "fresh_report_root_required": True,
        "interrupted_report_disposition": DEAD_EVIDENCE_DISPOSITION,
        "approval_note": "Owner approval of the execution packet recording this rule's content_hash.",
    }
    temp = ReplacementManifestRule.model_construct(**fields, content_hash="0" * 64)
    return ReplacementManifestRule(
        **fields,
        content_hash=compute_replacement_rule_hash(temp),
    )
