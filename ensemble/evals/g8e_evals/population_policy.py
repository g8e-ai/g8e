# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""D16 result-blind hash-stratified IFEval task selection policy.

Selects exactly four IFEval tasks from the frozen 120-task population
using only frozen task metadata (task key and instruction_id_list).
No model outcomes are inspected during selection. The selection is
deterministic and reproducible from the gold set alone.

The algorithm groups tasks by primary instruction family, orders
families by SHA-256(family_name), orders tasks within each family by
SHA-256(canonical_json(key, sorted(instruction_id_list))), and selects
four tasks by round-robin across families in sorted family order.

The resulting D16 population is the 21 framework scenario tasks plus
the four selected IFEval tasks (25 tasks total). The population
selection hash binds the selection rule, selected task IDs, IFEval
population hash, and framework scenario count so any change to the
selection creates a new identity.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum

from pydantic import BaseModel, ConfigDict, Field, model_validator
from typing import Self


D16_POPULATION_POLICY_VERSION = "1.0.0"

D16_SELECTED_TASK_COUNT = 4

D16_IFEVAL_POPULATION_HASH = "080cab7f47bbcd86badeb11ad3e836985292173fc8dec4228af11307c49f676a"
D16_SELECTED_IFEVAL_TASK_IDS = ("1019", "136", "16", "32")
D16_POPULATION_SELECTION_HASH = "71c2e71ceacf2f1a4b7b035c314558584b4343903c0fd1422136a6c568645c50"

FRAMEWORK_SCENARIO_COUNT = 21

FRAMEWORK_SUITE_IDS: tuple[str, ...] = (
    "final_response",
    "recovery",
    "routing_delegation",
    "security_policy",
    "technical_analysis",
    "tool_arguments",
    "tool_selection",
    "verification",
)


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class D16SelectionRule(StrEnum):
    """The frozen selection rule for D16 IFEval task selection.

    ``HASH_STRATIFIED_ROUND_ROBIN``: Group tasks by primary instruction
    family, order families by SHA-256(family_name) ascending, order
    tasks within each family by
    SHA-256(canonical_json(key, sorted(instruction_id_list))) ascending,
    and select four tasks by round-robin across families.
    """

    HASH_STRATIFIED_ROUND_ROBIN = "hash-stratified-round-robin"


class D16PopulationSelection(BaseModel):
    """Frozen D16 population selection record.

    Binds the selection rule, selected IFEval task IDs, IFEval population
    hash, framework scenario count, framework suite IDs, and a computed
    content hash. The selection is result-blind: it uses only frozen
    task metadata and does not inspect model outcomes.

    The ``content_hash`` is SHA-256 over canonical JSON of the selection.
    Changing any field changes the hash and creates a new D16 population
    identity.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    selection_id: str = Field(min_length=1, description="Unique selection identifier.")
    selection_version: str = Field(min_length=1, description="Selection schema version.")
    rule: D16SelectionRule = Field(description="Frozen selection rule.")
    selected_ifeval_task_ids: list[str] = Field(
        min_length=D16_SELECTED_TASK_COUNT,
        max_length=D16_SELECTED_TASK_COUNT,
        description="Exactly four selected IFEval task IDs (sorted).",
    )
    ifeval_population_hash: str = Field(
        min_length=64, max_length=64,
        description="Content hash of the source IFEval benchmark population.",
    )
    framework_scenario_count: int = Field(
        ge=1,
        description="Number of framework scenario tasks in the D16 population.",
    )
    framework_suite_ids: list[str] = Field(
        min_length=1,
        description="Sorted framework suite IDs contributing scenario tasks.",
    )
    total_population: int = Field(
        ge=1,
        description="Total D16 population size (framework scenarios + selected IFEval tasks).",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the selection.",
    )

    @model_validator(mode="after")
    def _validate_selection(self) -> Self:
        if self.selected_ifeval_task_ids != sorted(self.selected_ifeval_task_ids):
            raise ValueError(
                f"selected_ifeval_task_ids must be sorted: {self.selected_ifeval_task_ids}"
            )
        if len(self.selected_ifeval_task_ids) != len(set(self.selected_ifeval_task_ids)):
            raise ValueError(
                f"selected_ifeval_task_ids must not contain duplicates: {self.selected_ifeval_task_ids}"
            )
        if self.framework_suite_ids != sorted(self.framework_suite_ids):
            raise ValueError(
                f"framework_suite_ids must be sorted: {self.framework_suite_ids}"
            )
        expected_total = self.framework_scenario_count + len(self.selected_ifeval_task_ids)
        if self.total_population != expected_total:
            raise ValueError(
                f"total_population ({self.total_population}) != "
                f"framework_scenario_count ({self.framework_scenario_count}) + "
                f"selected_ifeval_task_count ({len(self.selected_ifeval_task_ids)})"
            )
        expected = compute_d16_selection_hash(
            selection_id=self.selection_id,
            selection_version=self.selection_version,
            rule=self.rule,
            selected_ifeval_task_ids=self.selected_ifeval_task_ids,
            ifeval_population_hash=self.ifeval_population_hash,
            framework_scenario_count=self.framework_scenario_count,
            framework_suite_ids=self.framework_suite_ids,
            total_population=self.total_population,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"D16 population selection content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


def compute_d16_selection_hash(
    *,
    selection_id: str,
    selection_version: str,
    rule: D16SelectionRule,
    selected_ifeval_task_ids: list[str],
    ifeval_population_hash: str,
    framework_scenario_count: int,
    framework_suite_ids: list[str],
    total_population: int,
) -> str:
    """Compute the content hash for a D16 population selection."""
    payload = json.dumps(
        {
            "selection_id": selection_id,
            "selection_version": selection_version,
            "rule": rule.value,
            "selected_ifeval_task_ids": sorted(selected_ifeval_task_ids),
            "ifeval_population_hash": ifeval_population_hash,
            "framework_scenario_count": framework_scenario_count,
            "framework_suite_ids": sorted(framework_suite_ids),
            "total_population": total_population,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def _canonical_task_payload(key: str, instruction_id_list: list[str]) -> str:
    return json.dumps(
        {"key": key, "instruction_id_list": sorted(instruction_id_list)},
        sort_keys=True,
        separators=(",", ":"),
        ensure_ascii=False,
    )


def _task_selection_hash(key: str, instruction_id_list: list[str]) -> str:
    return _sha256(_canonical_task_payload(key, instruction_id_list))


def _primary_family(instruction_id_list: list[str]) -> str:
    if not instruction_id_list:
        return "unknown"
    return sorted(instruction_id_list)[0].split(":")[0]


def select_d16_ifeval_tasks(
    ifeval_rows: list[dict[str, object]],
) -> list[str]:
    """Select exactly four IFEval task IDs using the D16 hash-stratified rule.

    Groups tasks by primary instruction family, orders families by
    SHA-256(family_name) ascending, orders tasks within each family by
    SHA-256(canonical_json(key, sorted(instruction_id_list))) ascending,
    and selects four tasks by round-robin across families in sorted
    family order.

    Args:
        ifeval_rows: List of IFEval task dicts, each with ``key`` and
            ``instruction_id_list`` fields.

    Returns:
        Sorted list of exactly four selected task ID strings.

    Raises:
        ValueError: If fewer than four tasks are provided.
    """
    if len(ifeval_rows) < D16_SELECTED_TASK_COUNT:
        raise ValueError(
            f"need at least {D16_SELECTED_TASK_COUNT} tasks for D16 selection: "
            f"got {len(ifeval_rows)}"
        )

    from collections import defaultdict

    strata: dict[str, list[tuple[str, str]]] = defaultdict(list)
    for row in ifeval_rows:
        key = str(row["key"])
        raw_iid = row.get("instruction_id_list", [])
        iid_list: list[str] = list(raw_iid) if isinstance(raw_iid, list) else []
        fam = _primary_family(iid_list)
        h = _task_selection_hash(key, iid_list)
        strata[fam].append((key, h))

    sorted_families = sorted(strata.keys(), key=_sha256)

    for fam in sorted_families:
        strata[fam].sort(key=lambda t: t[1])

    selected: list[str] = []
    family_idx = 0
    task_idx_in_family = [0] * len(sorted_families)
    while len(selected) < D16_SELECTED_TASK_COUNT:
        idx = family_idx % len(sorted_families)
        fam = sorted_families[idx]
        ti = task_idx_in_family[idx]
        if ti < len(strata[fam]):
            task_id = strata[fam][ti][0]
            if task_id not in selected:
                selected.append(task_id)
            task_idx_in_family[idx] += 1
        family_idx += 1
        if family_idx > 10000:
            raise ValueError(
                "D16 selection exceeded safety bound; check for empty strata"
            )

    return sorted(selected)


def build_d16_population_selection(
    *,
    ifeval_rows: list[dict[str, object]],
    ifeval_population_hash: str,
    selection_id: str = "d16-population-selection",
) -> D16PopulationSelection:
    """Build a frozen D16 population selection from IFEval task rows.

    Selects four IFEval tasks using the hash-stratified rule, binds the
    selection to the IFEval population hash and framework scenario
    count, and computes the content hash.

    Args:
        ifeval_rows: List of IFEval task dicts with ``key`` and
            ``instruction_id_list`` fields.
        ifeval_population_hash: Content hash of the source IFEval
            benchmark population.
        selection_id: Unique selection identifier.

    Returns:
        Frozen ``D16PopulationSelection`` record.
    """
    selected_ids = select_d16_ifeval_tasks(ifeval_rows)
    total = FRAMEWORK_SCENARIO_COUNT + len(selected_ids)
    content_hash = compute_d16_selection_hash(
        selection_id=selection_id,
        selection_version=D16_POPULATION_POLICY_VERSION,
        rule=D16SelectionRule.HASH_STRATIFIED_ROUND_ROBIN,
        selected_ifeval_task_ids=selected_ids,
        ifeval_population_hash=ifeval_population_hash,
        framework_scenario_count=FRAMEWORK_SCENARIO_COUNT,
        framework_suite_ids=list(FRAMEWORK_SUITE_IDS),
        total_population=total,
    )
    return D16PopulationSelection(
        selection_id=selection_id,
        selection_version=D16_POPULATION_POLICY_VERSION,
        rule=D16SelectionRule.HASH_STRATIFIED_ROUND_ROBIN,
        selected_ifeval_task_ids=selected_ids,
        ifeval_population_hash=ifeval_population_hash,
        framework_scenario_count=FRAMEWORK_SCENARIO_COUNT,
        framework_suite_ids=list(FRAMEWORK_SUITE_IDS),
        total_population=total,
        content_hash=content_hash,
    )


def build_published_d16_population_selection() -> D16PopulationSelection:
    return D16PopulationSelection(
        selection_id="d16-population-selection",
        selection_version=D16_POPULATION_POLICY_VERSION,
        rule=D16SelectionRule.HASH_STRATIFIED_ROUND_ROBIN,
        selected_ifeval_task_ids=list(D16_SELECTED_IFEVAL_TASK_IDS),
        ifeval_population_hash=D16_IFEVAL_POPULATION_HASH,
        framework_scenario_count=FRAMEWORK_SCENARIO_COUNT,
        framework_suite_ids=list(FRAMEWORK_SUITE_IDS),
        total_population=FRAMEWORK_SCENARIO_COUNT + D16_SELECTED_TASK_COUNT,
        content_hash=D16_POPULATION_SELECTION_HASH,
    )


__all__ = [
    "D16_IFEVAL_POPULATION_HASH",
    "D16_POPULATION_POLICY_VERSION",
    "D16_POPULATION_SELECTION_HASH",
    "D16_SELECTED_IFEVAL_TASK_IDS",
    "D16_SELECTED_TASK_COUNT",
    "FRAMEWORK_SCENARIO_COUNT",
    "FRAMEWORK_SUITE_IDS",
    "D16PopulationSelection",
    "D16SelectionRule",
    "build_d16_population_selection",
    "build_published_d16_population_selection",
    "compute_d16_selection_hash",
    "select_d16_ifeval_tasks",
]
