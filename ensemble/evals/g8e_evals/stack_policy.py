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

Each selector binds to a typed ``SelectionCriterion`` (the exact
metric used to rank variants) and ``SelectionDirection`` (maximize or
minimize). No selector description uses an unspecified ``best``,
``top``, ``approximately``, or manual override. The lineage metadata
source and deduplication rule are frozen in the policy so identical
accepted evidence produces byte-identical ordered outputs.

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

    ``ACCURACY``: Select the variant with the highest macro_average
    for each role.
    ``EFFICIENCY``: Select the variant with the lowest
    median_warm_latency_ms for each role.
    ``SMALLEST``: Select the variant with the lowest parameter_count
    for each role.
    ``TOOL_CALLING``: Select the variant with the highest
    tool_calling_category_score for each role.
    ``PRIVACY``: Select the variant with the highest
    privacy_category_score for each role.
    ``THROUGHPUT``: Select the variant with the highest
    throughput_tokens_per_second for each role.
    ``LINEAGE_DIVERSE_ACCURACY``: Select a lineage-diverse stack
    maximizing macro_average across roles.
    ``LINEAGE_DIVERSE_EFFICIENCY``: Select a lineage-diverse stack
    minimizing median_warm_latency_ms across roles.
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


class SelectionCriterion(StrEnum):
    """The exact metric each D14 selector uses to rank variants.

    Each criterion names a specific, observable measurement. No
    selector uses an unspecified ``best`` or ``top``; the criterion
    and direction fully define the ranking input.

    ``MACRO_AVERAGE``: The D8 category macro-average score from the
    Phase B selection policy (``VariantRoleScore.macro_average``).
    ``MEDIAN_WARM_LATENCY_MS``: Median warm latency in milliseconds
    (``VariantRoleScore.median_warm_latency_ms``).
    ``PARAMETER_COUNT``: The model parameter count from the frozen
    model registry.
    ``TOOL_CALLING_CATEGORY_SCORE``: The macro-average of the
    ``tool_selection`` and ``tool_arguments`` category scores.
    ``PRIVACY_CATEGORY_SCORE``: The ``security_policy`` category
    score from the Phase B selection policy.
    ``THROUGHPUT_TOKENS_PER_SECOND``: Observed throughput in tokens
    per second from resource observations.
    """

    MACRO_AVERAGE = "macro_average"
    MEDIAN_WARM_LATENCY_MS = "median_warm_latency_ms"
    PARAMETER_COUNT = "parameter_count"
    TOOL_CALLING_CATEGORY_SCORE = "tool_calling_category_score"
    PRIVACY_CATEGORY_SCORE = "privacy_category_score"
    THROUGHPUT_TOKENS_PER_SECOND = "throughput_tokens_per_second"


class SelectorEligibilityRule(StrEnum):
    PHASE_B_FINALIST = "phase_b_finalist"


class HomogeneousFamilyEligibilityRule(StrEnum):
    ELIGIBLE_IN_ALL_ROLES = "eligible_in_all_roles"


class SelectionDirection(StrEnum):
    """Whether a selector maximizes or minimizes its criterion.

    ``MAXIMIZE``: Higher criterion values rank higher (accuracy,
    tool calling, privacy, throughput).
    ``MINIMIZE``: Lower criterion values rank higher (efficiency,
    smallest).
    """

    MAXIMIZE = "maximize"
    MINIMIZE = "minimize"


class LineageMetadataSource(StrEnum):
    """The frozen source of family/lineage metadata for diversity checks.

    ``MODEL_REGISTRY_FAMILY_ID``: The ``family_id`` field from the
    frozen model registry entry for each variant. This is the
    canonical lineage identifier used by lineage-diverse selectors
    and the D20 homogeneous family rule.
    """

    MODEL_REGISTRY_FAMILY_ID = "model_registry_family_id"


class DeduplicationRule(StrEnum):
    """How identical role combinations across selectors are handled.

    ``KEEP_SELECTOR_LABEL``: When two selectors produce the same
    role-variant combination, both entries are retained with their
    distinct selector labels. The combination is not collapsed;
    each selector's result remains independently traceable.
    """

    KEEP_SELECTOR_LABEL = "keep_selector_label"


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

    Binds the selector ID, description, selection criterion (the exact
    metric used to rank variants), selection direction (maximize or
    minimize), whether the selector requires lineage diversity, and
    the ordered tie-breaker keys. No description uses an unspecified
    ``best`` or ``top``; the criterion and direction fully define the
    ranking input.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    selector_id: StackSelectorId = Field(description="Stack selector identifier.")
    description: str = Field(min_length=1, description="Human-readable selector description.")
    selection_criterion: SelectionCriterion = Field(
        description="The exact metric this selector uses to rank variants.",
    )
    selection_direction: SelectionDirection = Field(
        description="Whether to maximize or minimize the selection criterion.",
    )
    requires_lineage_diversity: bool = Field(
        description="Whether this selector requires lineage diversity across roles.",
    )
    eligibility_rule: SelectorEligibilityRule = Field(
        default=SelectorEligibilityRule.PHASE_B_FINALIST,
        description="Frozen eligibility threshold requiring accepted Phase B finalist status.",
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
    eligibility_rule: HomogeneousFamilyEligibilityRule = Field(
        description="Typed condition for adding the homogeneous stack.",
    )


class StackPolicy(BaseModel):
    """Frozen D14/D20 Phase D stack selector policy.

    Binds the policy version, selector definitions, lineage metadata
    source, deduplication rule, homogeneous family rule, and content
    hash. The policy is frozen before Phase B ranking and is the
    policy authority; applying it waits for accepted EF8-2 results.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    policy_id: str = Field(min_length=1, description="Unique policy identifier.")
    policy_version: str = Field(min_length=1, description="Policy schema version.")
    selectors: list[StackSelectorDefinition] = Field(
        min_length=1,
        description="Frozen D14 stack selector definitions.",
    )
    lineage_metadata_source: LineageMetadataSource = Field(
        description="Frozen source of family/lineage metadata for diversity checks.",
    )
    deduplication_rule: DeduplicationRule = Field(
        description="How identical role combinations across selectors are handled.",
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
            lineage_metadata_source=self.lineage_metadata_source,
            deduplication_rule=self.deduplication_rule,
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
    lineage_metadata_source: LineageMetadataSource,
    deduplication_rule: DeduplicationRule,
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
            "lineage_metadata_source": lineage_metadata_source.value,
            "deduplication_rule": deduplication_rule.value,
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
        criterion, direction = _SELECTOR_CRITERIA[selector]
        defs.append(
            StackSelectorDefinition(
                selector_id=selector,
                description=_SELECTOR_DESCRIPTIONS[selector],
                selection_criterion=criterion,
                selection_direction=direction,
                requires_lineage_diversity=requires_diversity,
                tie_breaker_order=tie_breakers,
            )
        )
    return defs


_SELECTOR_DESCRIPTIONS: dict[StackSelectorId, str] = {
    StackSelectorId.ACCURACY: "Select the variant with the highest macro_average for each role.",
    StackSelectorId.EFFICIENCY: "Select the variant with the lowest median_warm_latency_ms for each role.",
    StackSelectorId.SMALLEST: "Select the variant with the lowest parameter_count for each role.",
    StackSelectorId.TOOL_CALLING: "Select the variant with the highest tool_calling_category_score for each role.",
    StackSelectorId.PRIVACY: "Select the variant with the highest privacy_category_score for each role.",
    StackSelectorId.THROUGHPUT: "Select the variant with the highest throughput_tokens_per_second for each role.",
    StackSelectorId.LINEAGE_DIVERSE_ACCURACY: "Select a lineage-diverse stack maximizing macro_average across roles.",
    StackSelectorId.LINEAGE_DIVERSE_EFFICIENCY: "Select a lineage-diverse stack minimizing median_warm_latency_ms across roles.",
}

_SELECTOR_CRITERIA: dict[StackSelectorId, tuple[SelectionCriterion, SelectionDirection]] = {
    StackSelectorId.ACCURACY: (SelectionCriterion.MACRO_AVERAGE, SelectionDirection.MAXIMIZE),
    StackSelectorId.EFFICIENCY: (SelectionCriterion.MEDIAN_WARM_LATENCY_MS, SelectionDirection.MINIMIZE),
    StackSelectorId.SMALLEST: (SelectionCriterion.PARAMETER_COUNT, SelectionDirection.MINIMIZE),
    StackSelectorId.TOOL_CALLING: (SelectionCriterion.TOOL_CALLING_CATEGORY_SCORE, SelectionDirection.MAXIMIZE),
    StackSelectorId.PRIVACY: (SelectionCriterion.PRIVACY_CATEGORY_SCORE, SelectionDirection.MAXIMIZE),
    StackSelectorId.THROUGHPUT: (SelectionCriterion.THROUGHPUT_TOKENS_PER_SECOND, SelectionDirection.MAXIMIZE),
    StackSelectorId.LINEAGE_DIVERSE_ACCURACY: (SelectionCriterion.MACRO_AVERAGE, SelectionDirection.MAXIMIZE),
    StackSelectorId.LINEAGE_DIVERSE_EFFICIENCY: (SelectionCriterion.MEDIAN_WARM_LATENCY_MS, SelectionDirection.MINIMIZE),
}


def build_stack_policy(
    *,
    policy_id: str = "d14-d20-stack-policy",
) -> StackPolicy:
    """Build the frozen D14/D20 Phase D stack selector policy.

    The policy is frozen with:
    - D14: eight selectors (accuracy, efficiency, smallest, tool
      calling, privacy, throughput, lineage-diverse accuracy,
      lineage-diverse efficiency), each bound to a typed
      ``SelectionCriterion`` and ``SelectionDirection``.
    - D20: homogeneous family fallback preferring Qwen, then Granite,
      then lexicographic family ID.
    - Lineage metadata source: ``model_registry_family_id``.
    - Deduplication rule: ``keep_selector_label``.
    """
    selectors = _build_selector_definitions()
    homogeneous_rule = HomogeneousFamilyRule(
        preference_order=[
            FamilyPreference.QWEN,
            FamilyPreference.GRANITE,
            FamilyPreference.LEXICOGRAPHIC,
        ],
        eligibility_rule=HomogeneousFamilyEligibilityRule.ELIGIBLE_IN_ALL_ROLES,
    )
    content_hash = compute_stack_policy_hash(
        policy_id=policy_id,
        policy_version=STACK_POLICY_VERSION,
        selectors=selectors,
        lineage_metadata_source=LineageMetadataSource.MODEL_REGISTRY_FAMILY_ID,
        deduplication_rule=DeduplicationRule.KEEP_SELECTOR_LABEL,
        homogeneous_family_rule=homogeneous_rule,
    )
    return StackPolicy(
        policy_id=policy_id,
        policy_version=STACK_POLICY_VERSION,
        selectors=selectors,
        lineage_metadata_source=LineageMetadataSource.MODEL_REGISTRY_FAMILY_ID,
        deduplication_rule=DeduplicationRule.KEEP_SELECTOR_LABEL,
        homogeneous_family_rule=homogeneous_rule,
        content_hash=content_hash,
    )


__all__ = [
    "STACK_POLICY_VERSION",
    "DeduplicationRule",
    "FamilyPreference",
    "HomogeneousFamilyEligibilityRule",
    "HomogeneousFamilyRule",
    "LineageMetadataSource",
    "SelectionCriterion",
    "SelectionDirection",
    "SelectorEligibilityRule",
    "StackPolicy",
    "StackSelectorDefinition",
    "StackSelectorId",
    "StackTieBreakerKey",
    "build_stack_policy",
    "compute_stack_policy_hash",
]
