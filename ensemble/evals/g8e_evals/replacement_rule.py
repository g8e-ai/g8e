# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed replacement-manifest rule for campaign-set child restarts (D24).

When a campaign-set child is interrupted mid-run, the interrupted report
is dead evidence: it is never resumed or retrofitted. The release owner
pre-approves a single frozen ``ReplacementManifestRule`` so the Live
Operations worker can self-serve a restart without per-interruption
approval.

The rule is content-addressed and bound to one frozen
``CampaignSetPlan`` by ``set_id`` and ``set_plan_hash``. A replacement
run identifies itself by a deterministic replacement child ID derived
from the rule identity, the original plan ``child_id``, and the attempt
number::

    replacement_child_id = sha256(canonical_json({
        "rule_id": rule.rule_id,
        "original_child_id": original_child_id,
        "attempt_number": attempt_number,
    }))

A replacement child ID is not a plan ``child_id`` and never collides
with one: plan child IDs derive from ``(parent_campaign_id,
partition_index, partition_task_ids)`` under ``compute_child_campaign_id``,
a different payload shape. The runner preflight accepts a replacement ID
only when the rule is bound, the original child is in both the plan and
``replaceable_child_ids``, and the attempt number is within
``max_attempts_per_child``. The replacement runs the original child's
exact partition; the run manifest's ``CampaignBinding`` records the
replacement lineage (``supersedes_child_id``, ``replacement_attempt``,
``replacement_rule_hash``) so aggregate verification can prove the
derivation.

Owner signature is the release owner's approval of the execution packet
recording this rule's ``content_hash``; the artifact itself carries no
mutable approval state, so its hash is stable before and after signing.
"""

from __future__ import annotations

import hashlib
import json
from typing import TYPE_CHECKING, Literal, Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

if TYPE_CHECKING:
    from g8e_evals.campaign_set import CampaignChildPlan, CampaignSetPlan


REPLACEMENT_RULE_SCHEMA_VERSION = "1.0.0"

REPLACEMENT_CHILD_DERIVATION_RULE = (
    "sha256(canonical_json({rule_id, original_child_id, attempt_number}))"
)

DEAD_EVIDENCE_DISPOSITION = "dead_evidence"


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


def compute_replacement_child_id(
    rule_id: str,
    original_child_id: str,
    attempt_number: int,
) -> str:
    """Derive the deterministic replacement child campaign ID.

    The same ``(rule_id, original_child_id, attempt_number)`` triple
    always produces the same replacement ID. The payload shape differs
    from ``compute_child_campaign_id`` so a replacement ID can never
    collide with a plan child ID.
    """
    payload = json.dumps(
        {
            "attempt_number": attempt_number,
            "original_child_id": original_child_id,
            "rule_id": rule_id,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def compute_replacement_rule_hash(rule: ReplacementManifestRule) -> str:
    """Compute the content hash for a replacement-manifest rule.

    Serializes every material field (excluding ``content_hash`` itself)
    into canonical JSON and returns SHA-256. The same function is used
    by the model validator and by rule builders.
    """
    data = rule.model_dump(mode="json", by_alias=True)
    data.pop("content_hash", None)
    payload = json.dumps(
        data,
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


class ReplacementManifestRule(BaseModel):
    """Frozen typed rule authorizing campaign-set child replacement runs.

    Binds the frozen ``CampaignSetPlan`` by ``set_id`` and
    ``set_plan_hash``, the replaceable plan children, the per-child
    attempt ceiling, and the disposition of the interrupted report.
    Changing any bound field changes ``content_hash`` and creates a new
    rule identity requiring fresh owner approval.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    rule_id: str = Field(
        min_length=1,
        description="Replacement rule identity.",
    )
    rule_version: str = Field(
        min_length=1,
        description="Rule schema version.",
    )
    purpose: str = Field(
        min_length=1,
        description="Human-readable statement of what this rule authorizes.",
    )
    created_at: str = Field(
        min_length=1,
        description="ISO-8601 creation timestamp.",
    )
    set_id: str = Field(
        min_length=1,
        description="Campaign-set identity this rule applies to.",
    )
    set_plan_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 content hash of the frozen CampaignSetPlan.",
    )
    replaceable_child_ids: list[str] = Field(
        min_length=1,
        description="Plan child IDs this rule may replace (must be a subset of the plan's children).",
    )
    max_attempts_per_child: int = Field(
        ge=1,
        description="Maximum replacement attempts authorized per child.",
    )
    derivation_rule: str = Field(
        min_length=1,
        description="Canonical replacement child-ID derivation rule.",
    )
    fresh_report_root_required: bool = Field(
        description="Every replacement run must allocate a fresh report root; the interrupted report is never resumed.",
    )
    interrupted_report_disposition: Literal["dead_evidence"] = Field(
        description="The interrupted report is preserved as dead evidence and never retrofitted.",
    )
    approval_note: str = Field(
        min_length=1,
        description="How this rule is signed: owner approval of the execution packet recording this rule's content_hash.",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the rule.",
    )

    @model_validator(mode="after")
    def _validate_rule(self) -> Self:
        if self.derivation_rule != REPLACEMENT_CHILD_DERIVATION_RULE:
            raise ValueError(
                f"derivation_rule must be {REPLACEMENT_CHILD_DERIVATION_RULE!r}: "
                f"got {self.derivation_rule!r}"
            )
        if not self.fresh_report_root_required:
            raise ValueError("fresh_report_root_required must be true")
        if len(self.replaceable_child_ids) != len(set(self.replaceable_child_ids)):
            raise ValueError(
                f"duplicate child IDs in replaceable_child_ids: "
                f"{self.replaceable_child_ids}"
            )
        expected = compute_replacement_rule_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"replacement rule content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


def validate_replacement_rule_plan_binding(
    rule: ReplacementManifestRule,
    plan: CampaignSetPlan,
) -> list[str]:
    """Check a replacement rule's declared binding to a campaign-set plan.

    Returns a list of human-readable violations; empty when the rule is
    correctly bound. A correct binding requires the rule's ``set_id`` to
    equal the plan's ``set_id``, ``set_plan_hash`` to equal the plan's
    ``content_hash``, and every ``replaceable_child_ids`` entry to name
    an existing plan child.
    """
    failures: list[str] = []
    if rule.set_id != plan.set_id:
        failures.append(
            f"replacement rule set_id {rule.set_id!r} does not match "
            f"campaign-set plan set_id {plan.set_id!r}"
        )
    if rule.set_plan_hash != plan.content_hash:
        failures.append(
            f"replacement rule set_plan_hash {rule.set_plan_hash!r} does not "
            f"match campaign-set plan content_hash {plan.content_hash!r}"
        )
    plan_child_ids = {cp.child_id for cp in plan.child_plans}
    unknown = sorted(
        cid for cid in rule.replaceable_child_ids if cid not in plan_child_ids
    )
    if unknown:
        failures.append(
            f"replacement rule declares replaceable child IDs not present "
            f"in the campaign-set plan: {unknown}"
        )
    return failures


def resolve_replacement_child_id(
    rule: ReplacementManifestRule,
    plan: CampaignSetPlan,
    campaign_id: str,
) -> tuple[CampaignChildPlan, int] | None:
    """Resolve a replacement ``campaign_id`` to its original plan child.

    Returns ``(original_child_plan, attempt_number)`` when ``campaign_id``
    equals a rule-derived replacement ID for a plan child that the rule
    lists as replaceable, with the attempt number inside
    ``max_attempts_per_child``. Returns ``None`` when the ID is not a
    valid replacement under this rule and plan.
    """
    for cp in plan.child_plans:
        if cp.child_id not in rule.replaceable_child_ids:
            continue
        for attempt in range(1, rule.max_attempts_per_child + 1):
            if compute_replacement_child_id(rule.rule_id, cp.child_id, attempt) == campaign_id:
                return cp, attempt
    return None
