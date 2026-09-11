# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""D14/D20 Phase D stack selector policy for heterogeneous-stack evaluation.

Encodes the frozen stack selector definitions that select model
variants for each of the eight Phase D stacks. The policy is frozen
before any Phase B ranking and is the policy authority; applying it
waits for accepted EF8-2 results.

D14 freezes selectors for: accuracy, efficiency, smallest, tool
calling, privacy, throughput, lineage-diverse accuracy, and
lineage-diverse efficiency. A homogeneous baseline is added only
under D20.

D20 homogeneous family fallback: prefer Qwen, then Granite, then
lexicographically ordered family ID among remaining families
eligible in all roles.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator


STACK_POLICY_VERSION = "1.0.0"


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class StackSelectorId(StrEnum):
    """The eight D14 stack selector identifiers.

    Each selector identifies a stack composition strategy that
    selects model variants for the primary, assistant, and lite roles
    based on a specific criterion.

    ``ACCURACY``: Select the best accuracy variant for each role.
    ``EFFICIENCY``: Select the best efficiency (latency/throughput)
    variant for each role.
    ``SMALLEST``: Select the smallest (lowest parameter count) variant
    for each role.
    ``TOOL_CALLING``: Select the best tool-calling variant for each
    role.
    ``PRIVACY``: Select the best privacy-preserving variant for each
    role.
    ``THROUGHPUT``: Select the best throughput variant for each role.
    ``LINEAGE_DIVERSE_ACCURACY``: Select a lineage-diverse stack
    optimized for accuracy.
    ``LINEAGE_DIVERSE_EFFICIENCY``: Select a lineage-diverse stack
    optimized for efficiency.
    """

    ACCURACY = "accuracy"
    EFFICIENCY = "efficiency"
    SMALLEST = "smallest"
    TOOL_CALLING = "tool_calling"
    PRIVACY = "privacy"
    THROUGHPUT = "throughput"
    LINEAGE_DIVERSE_ACCURACY = "lineage_diverse_accuracy"
    LINEAGE_DIVERSE_EFFICIENCY = "lineage_diverse_efficiency"


class FamilyPreference(StrEnum):
    """D20 homogeneous family preference order.

    ``QWEN``: Prefer Qwen family variants.
    ``GRANITE``: Prefer Granite family variants.
    ``LEXICOGRAPHIC``: Prefer lexicographically ordered family ID
    among remaining families.
    """

    QWEN = "qwen"
    GRANITE = "granite"
    LEXICOGRAPHIC = "lexicographic"


class StackTieBreakerKey(StrEnum):
    """Ordered tie-breaker keys for D14 stack selection.

    When multiple variants satisfy a selector's primary criterion,
    ties are broken by: stable variant ID (lexicographic ascending),
    then median warm latency (lower is better), then peak memory
    (lower is better).
    """

    VARIANT_ID = "variant_id"
    MEDIAN_WARM_LATENCY = "median_warm_latency"
    PEAK_MEMORY = "peak_memory"


class StackSelectorDefinition(BaseModel):
    """Frozen definition for one D14 stack selector.

    Binds the selector ID, description, primary criterion, tie-breaker
    order, and whether the selector requires lineage diversity.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    selector_id: StackSelectorId = Field(description="Stack selector identifier.")
    description: str = Field(min_length=1, description="Human-readable selector description.")
    requires_lineage_diversity: bool = Field(
        description="Whether this selector requires lineage diversity across roles.",
    )
    tie_breaker_order: list[StackTieBreakerKey] = Field(
        min_length=1,
        description="Ordered tie-breaker keys for this selector.",
    )


class HomogeneousFamilyRule(BaseModel):
    """Frozen D20 homogeneous family fallback rule.

    Binds the family preference order and the condition under which
    the homogeneous family stack is added. The homogeneous family is
    added only when at least one family has candidates eligible in
    all three roles.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    preference_order: list[FamilyPreference] = Field(
        min_length=1,
        description="Ordered family preference (Qwen, Granite, lexicographic).",
    )
    condition: str = Field(
        min_length=1,
        description="Condition for adding the homogeneous stack (at least one family eligible in all roles).",
    )


class StackPolicy(BaseModel):
    """Frozen D14/D20 Phase D stack selector policy.

    Binds the policy version, selector definitions, homogeneous
    family rule, and content hash. The policy is frozen before Phase
    B ranking and is the policy authority; applying it waits for
    accepted EF8-2 results.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    policy_id: str = Field(min_length=1, description="Unique policy identifier.")
    policy_version: str = Field(min_length=1, description="Policy schema version.")
    selectors: list[StackSelectorDefinition] = Field(
        min_length=1,
        description="Frozen D14 stack selector definitions.",
    )
    homogeneous_family_rule: HomogeneousFamilyRule = Field(
        description="D20 homogeneous family fallback rule.",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the policy.",
    )

    @model_validator(mode="after")
    def _validate_policy(self) -> Self:
        selector_ids = [s.selector_id for s in self.selectors]
        if len(selector_ids) != len(set(selector_ids)):
            raise ValueError(
                f"duplicate selector_id in selectors: {selector_ids}"
            )
        if len(self.selectors) != len(StackSelectorId):
            raise ValueError(
                f"expected {len(StackSelectorId)} selectors: "
                f"got {len(self.selectors)}"
            )
        expected = compute_stack_policy_hash(
            policy_id=self.policy_id,
            policy_version=self.policy_version,
            selectors=self.selectors,
            homogeneous_family_rule=self.homogeneous_family_rule,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"stack policy content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


def compute_stack_policy_hash(
    *,
    policy_id: str,
    policy_version: str,
    selectors: list[StackSelectorDefinition],
    homogeneous_family_rule: HomogeneousFamilyRule,
) -> str:
    """Compute the content hash for a D14/D20 stack policy."""
    payload = json.dumps(
        {
            "policy_id": policy_id,
            "policy_version": policy_version,
            "selectors": [
                json.loads(s.model_dump_json())
                for s in sorted(selectors, key=lambda s: s.selector_id.value)
            ],
            "homogeneous_family_rule": json.loads(
                homogeneous_family_rule.model_dump_json()
            ),
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def _build_selector_definitions() -> list[StackSelectorDefinition]:
    """Build the frozen D14 selector definitions for all eight stacks."""
    tie_breakers = list(StackTieBreakerKey)
    defs = []
    for selector in StackSelectorId:
        requires_diversity = selector in (
            StackSelectorId.LINEAGE_DIVERSE_ACCURACY,
            StackSelectorId.LINEAGE_DIVERSE_EFFICIENCY,
        )
        defs.append(
            StackSelectorDefinition(
                selector_id=selector,
                description=_SELECTOR_DESCRIPTIONS[selector],
                requires_lineage_diversity=requires_diversity,
                tie_breaker_order=tie_breakers,
            )
        )
    return defs


_SELECTOR_DESCRIPTIONS: dict[StackSelectorId, str] = {
    StackSelectorId.ACCURACY: "Select the best accuracy variant for each role.",
    StackSelectorId.EFFICIENCY: "Select the best efficiency variant for each role.",
    StackSelectorId.SMALLEST: "Select the smallest variant for each role.",
    StackSelectorId.TOOL_CALLING: "Select the best tool-calling variant for each role.",
    StackSelectorId.PRIVACY: "Select the best privacy-preserving variant for each role.",
    StackSelectorId.THROUGHPUT: "Select the best throughput variant for each role.",
    StackSelectorId.LINEAGE_DIVERSE_ACCURACY: "Select a lineage-diverse stack optimized for accuracy.",
    StackSelectorId.LINEAGE_DIVERSE_EFFICIENCY: "Select a lineage-diverse stack optimized for efficiency.",
}


def build_stack_policy(
    *,
    policy_id: str = "d14-d20-stack-policy",
) -> StackPolicy:
    """Build the frozen D14/D20 Phase D stack selector policy.

    The policy is frozen with:
    - D14: eight selectors (accuracy, efficiency, smallest, tool
      calling, privacy, throughput, lineage-diverse accuracy,
      lineage-diverse efficiency).
    - D20: homogeneous family fallback preferring Qwen, then Granite,
      then lexicographic family ID.
    """
    selectors = _build_selector_definitions()
    homogeneous_rule = HomogeneousFamilyRule(
        preference_order=[
            FamilyPreference.QWEN,
            FamilyPreference.GRANITE,
            FamilyPreference.LEXICOGRAPHIC,
        ],
        condition="at least one family has candidates eligible in all three roles",
    )
    content_hash = compute_stack_policy_hash(
        policy_id=policy_id,
        policy_version=STACK_POLICY_VERSION,
        selectors=selectors,
        homogeneous_family_rule=homogeneous_rule,
    )
    return StackPolicy(
        policy_id=policy_id,
        policy_version=STACK_POLICY_VERSION,
        selectors=selectors,
        homogeneous_family_rule=homogeneous_rule,
        content_hash=content_hash,
    )


__all__ = [
    "STACK_POLICY_VERSION",
    "FamilyPreference",
    "HomogeneousFamilyRule",
    "StackPolicy",
    "StackSelectorDefinition",
    "StackSelectorId",
    "StackTieBreakerKey",
    "build_stack_policy",
    "compute_stack_policy_hash",
]
