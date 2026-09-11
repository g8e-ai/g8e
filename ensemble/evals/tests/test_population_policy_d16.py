# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the D16 result-blind hash-stratified IFEval task
selection policy.

Verifies that the selection algorithm is deterministic, result-blind,
selects exactly four tasks, covers multiple instruction families,
rejects invalid inputs, and that the frozen selection record
validates its content hash. No external dependencies (no files,
network, or DB).
"""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.population_policy import (
    D16_POPULATION_POLICY_VERSION,
    D16_SELECTED_TASK_COUNT,
    D16PopulationSelection,
    D16SelectionRule,
    FRAMEWORK_SCENARIO_COUNT,
    FRAMEWORK_SUITE_IDS,
    build_d16_population_selection,
    compute_d16_selection_hash,
    select_d16_ifeval_tasks,
)


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64


def _make_ifeval_rows() -> list[dict[str, object]]:
    return [
        {"key": 13, "instruction_id_list": ["detectable_format:json_format"]},
        {"key": 16, "instruction_id_list": ["detectable_format:title", "combination:repeat_prompt"]},
        {"key": 19, "instruction_id_list": ["length_constraints:number_words", "length_constraints:number_words"]},
        {"key": 24, "instruction_id_list": ["detectable_format:numbered_list"]},
        {"key": 30, "instruction_id_list": ["change_case:capital_word"]},
        {"key": 32, "instruction_id_list": ["punctuation:no_comma"]},
        {"key": 102, "instruction_id_list": ["detectable_format:number_highlight"]},
        {"key": 136, "instruction_id_list": ["detectable_format:title"]},
        {"key": 1019, "instruction_id_list": ["change_case:english_capital"]},
        {"key": 1001, "instruction_id_list": ["keywords:existence"]},
        {"key": 1005, "instruction_id_list": ["detectable_content:number_placeholders"]},
        {"key": 1051, "instruction_id_list": ["length_constraints:number_sentences"]},
    ]


class TestD16SelectionAlgorithm:
    def test_selects_exactly_four_tasks(self):
        rows = _make_ifeval_rows()
        selected = select_d16_ifeval_tasks(rows)
        assert len(selected) == D16_SELECTED_TASK_COUNT

    def test_selection_is_deterministic(self):
        rows = _make_ifeval_rows()
        selected_a = select_d16_ifeval_tasks(rows)
        selected_b = select_d16_ifeval_tasks(rows)
        assert selected_a == selected_b

    def test_selection_is_sorted(self):
        rows = _make_ifeval_rows()
        selected = select_d16_ifeval_tasks(rows)
        assert selected == sorted(selected)

    def test_selection_has_no_duplicates(self):
        rows = _make_ifeval_rows()
        selected = select_d16_ifeval_tasks(rows)
        assert len(selected) == len(set(selected))

    def test_selection_covers_multiple_families(self):
        rows = _make_ifeval_rows()
        selected = select_d16_ifeval_tasks(rows)
        families: set[str] = set()
        for row in rows:
            if str(row["key"]) in selected:
                iids = list(row.get("instruction_id_list", []))
                if iids:
                    families.add(sorted(iids)[0].split(":")[0])
        assert len(families) >= 2, (
            f"expected at least 2 families in selection, got {families}"
        )

    def test_rejects_insufficient_tasks(self):
        rows = [{"key": 1, "instruction_id_list": ["detectable_format:title"]}]
        with pytest.raises(ValueError, match="at least 4 tasks"):
            select_d16_ifeval_tasks(rows)

    def test_result_blind_ignores_outcome_fields(self):
        rows_a = [{"key": 1, "instruction_id_list": ["detectable_format:title"], "outcome": "pass"}]
        rows_b = [{"key": 1, "instruction_id_list": ["detectable_format:title"], "outcome": "fail"}]
        rows_a += _make_ifeval_rows()
        rows_b += _make_ifeval_rows()
        assert select_d16_ifeval_tasks(rows_a) == select_d16_ifeval_tasks(rows_b)

    def test_stable_when_extra_rows_added(self):
        rows = _make_ifeval_rows()
        selected_original = select_d16_ifeval_tasks(rows)
        rows_extended = [*rows, {"key": 9999, "instruction_id_list": ["keywords:existence"]}, {"key": 8888, "instruction_id_list": ["startend:quotation"]}]
        selected_extended = select_d16_ifeval_tasks(rows_extended)
        assert selected_original == selected_extended, (
            "adding tasks should not change the first four selected tasks "
            "because round-robin selects from sorted families"
        )


class TestD16PopulationSelection:
    def test_builds_valid_selection(self):
        rows = _make_ifeval_rows()
        selection = build_d16_population_selection(
            ifeval_rows=rows,
            ifeval_population_hash=_VALID_HASH,
        )
        assert len(selection.selected_ifeval_task_ids) == D16_SELECTED_TASK_COUNT
        assert selection.framework_scenario_count == FRAMEWORK_SCENARIO_COUNT
        assert selection.total_population == FRAMEWORK_SCENARIO_COUNT + D16_SELECTED_TASK_COUNT
        assert selection.rule == D16SelectionRule.HASH_STRATIFIED_ROUND_ROBIN

    def test_content_hash_is_deterministic(self):
        rows = _make_ifeval_rows()
        sel_a = build_d16_population_selection(
            ifeval_rows=rows,
            ifeval_population_hash=_VALID_HASH,
        )
        sel_b = build_d16_population_selection(
            ifeval_rows=rows,
            ifeval_population_hash=_VALID_HASH,
        )
        assert sel_a.content_hash == sel_b.content_hash

    def test_changing_ifeval_hash_changes_selection_hash(self):
        rows = _make_ifeval_rows()
        sel_a = build_d16_population_selection(
            ifeval_rows=rows,
            ifeval_population_hash="a" * 64,
        )
        sel_b = build_d16_population_selection(
            ifeval_rows=rows,
            ifeval_population_hash="b" * 64,
        )
        assert sel_a.content_hash != sel_b.content_hash

    def test_rejects_unsorted_task_ids(self):
        with pytest.raises(ValidationError, match="must be sorted"):
            D16PopulationSelection(
                selection_id="test",
                selection_version=D16_POPULATION_POLICY_VERSION,
                rule=D16SelectionRule.HASH_STRATIFIED_ROUND_ROBIN,
                selected_ifeval_task_ids=["32", "16", "1019", "136"],
                ifeval_population_hash=_VALID_HASH,
                framework_scenario_count=FRAMEWORK_SCENARIO_COUNT,
                framework_suite_ids=list(FRAMEWORK_SUITE_IDS),
                total_population=FRAMEWORK_SCENARIO_COUNT + 4,
                content_hash="0" * 64,
            )

    def test_rejects_duplicate_task_ids(self):
        with pytest.raises(ValidationError, match="must not contain duplicates"):
            D16PopulationSelection(
                selection_id="test",
                selection_version=D16_POPULATION_POLICY_VERSION,
                rule=D16SelectionRule.HASH_STRATIFIED_ROUND_ROBIN,
                selected_ifeval_task_ids=["136", "16", "16", "32"],
                ifeval_population_hash=_VALID_HASH,
                framework_scenario_count=FRAMEWORK_SCENARIO_COUNT,
                framework_suite_ids=list(FRAMEWORK_SUITE_IDS),
                total_population=FRAMEWORK_SCENARIO_COUNT + 4,
                content_hash="0" * 64,
            )

    def test_rejects_wrong_task_count(self):
        with pytest.raises(ValidationError):
            D16PopulationSelection(
                selection_id="test",
                selection_version=D16_POPULATION_POLICY_VERSION,
                rule=D16SelectionRule.HASH_STRATIFIED_ROUND_ROBIN,
                selected_ifeval_task_ids=["16", "32", "136"],
                ifeval_population_hash=_VALID_HASH,
                framework_scenario_count=FRAMEWORK_SCENARIO_COUNT,
                framework_suite_ids=list(FRAMEWORK_SUITE_IDS),
                total_population=FRAMEWORK_SCENARIO_COUNT + 3,
                content_hash="0" * 64,
            )

    def test_rejects_wrong_total_population(self):
        sorted_ids = ["1019", "136", "16", "32"]
        with pytest.raises(ValidationError, match="total_population"):
            D16PopulationSelection(
                selection_id="test",
                selection_version=D16_POPULATION_POLICY_VERSION,
                rule=D16SelectionRule.HASH_STRATIFIED_ROUND_ROBIN,
                selected_ifeval_task_ids=sorted_ids,
                ifeval_population_hash=_VALID_HASH,
                framework_scenario_count=FRAMEWORK_SCENARIO_COUNT,
                framework_suite_ids=list(FRAMEWORK_SUITE_IDS),
                total_population=99,
                content_hash="0" * 64,
            )

    def test_rejects_content_hash_mismatch(self):
        sorted_ids = ["1019", "136", "16", "32"]
        with pytest.raises(ValidationError, match="content_hash mismatch"):
            D16PopulationSelection(
                selection_id="test",
                selection_version=D16_POPULATION_POLICY_VERSION,
                rule=D16SelectionRule.HASH_STRATIFIED_ROUND_ROBIN,
                selected_ifeval_task_ids=sorted_ids,
                ifeval_population_hash=_VALID_HASH,
                framework_scenario_count=FRAMEWORK_SCENARIO_COUNT,
                framework_suite_ids=list(FRAMEWORK_SUITE_IDS),
                total_population=FRAMEWORK_SCENARIO_COUNT + 4,
                content_hash="0" * 64,
            )

    def test_rejects_unknown_field(self):
        sorted_ids = ["1019", "136", "16", "32"]
        with pytest.raises(ValidationError):
            D16PopulationSelection(
                selection_id="test",
                selection_version=D16_POPULATION_POLICY_VERSION,
                rule=D16SelectionRule.HASH_STRATIFIED_ROUND_ROBIN,
                selected_ifeval_task_ids=sorted_ids,
                ifeval_population_hash=_VALID_HASH,
                framework_scenario_count=FRAMEWORK_SCENARIO_COUNT,
                framework_suite_ids=list(FRAMEWORK_SUITE_IDS),
                total_population=FRAMEWORK_SCENARIO_COUNT + 4,
                content_hash=compute_d16_selection_hash(
                    selection_id="test",
                    selection_version=D16_POPULATION_POLICY_VERSION,
                    rule=D16SelectionRule.HASH_STRATIFIED_ROUND_ROBIN,
                    selected_ifeval_task_ids=sorted_ids,
                    ifeval_population_hash=_VALID_HASH,
                    framework_scenario_count=FRAMEWORK_SCENARIO_COUNT,
                    framework_suite_ids=list(FRAMEWORK_SUITE_IDS),
                    total_population=FRAMEWORK_SCENARIO_COUNT + 4,
                ),
                unknown_field="bad",
            )
