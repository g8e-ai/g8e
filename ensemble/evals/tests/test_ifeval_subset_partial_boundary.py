# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Phase 0.4 tests for the ifeval_subset explicit partial-evaluation boundary.

Verifies that the ifeval_subset suite is explicitly partial: the subset
size, pinned upstream revision, supported and unsupported instruction
types, and the complete-import follow-on declaration are all typed and
verified. Unsupported instruction types fail closed rather than becoming
implicit passes. The complete upstream evaluator and dataset import is
documented as a separately scoped follow-on task.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from g8e_evals.benchmarks.ifeval.import_subset import SELECTED_KEYS, UPSTREAM_REVISION
from g8e_evals.benchmarks.ifeval.partial_boundary import (
    IFEVAL_SUBSET_BOUNDARY,
    IFEvalSubsetBoundary,
)
from g8e_evals.benchmarks.ifeval.verifier import IFEvalVerifier


_GOLD_SETS_DIR = Path(__file__).resolve().parents[1] / "gold_sets"


@pytest.mark.unit
def test_ifeval_subset_boundary_declares_subset_size_matching_selected_keys():
    assert IFEVAL_SUBSET_BOUNDARY.subset_size == len(SELECTED_KEYS)
    assert IFEVAL_SUBSET_BOUNDARY.subset_size == len(IFEVAL_SUBSET_BOUNDARY.selected_keys)


@pytest.mark.unit
def test_ifeval_subset_boundary_declares_pinned_upstream_revision():
    assert IFEVAL_SUBSET_BOUNDARY.upstream_revision == UPSTREAM_REVISION
    assert len(IFEVAL_SUBSET_BOUNDARY.upstream_revision) > 0


@pytest.mark.unit
def test_ifeval_subset_boundary_declares_selected_keys_matching_import_subset():
    assert IFEVAL_SUBSET_BOUNDARY.selected_keys == list(SELECTED_KEYS)


@pytest.mark.unit
def test_ifeval_subset_boundary_declares_complete_import_as_separately_scoped():
    assert IFEVAL_SUBSET_BOUNDARY.complete_import_is_separately_scoped is True
    assert "Follow-on milestone A" in IFEVAL_SUBSET_BOUNDARY.complete_import_follow_on


@pytest.mark.unit
def test_ifeval_subset_boundary_has_at_least_one_supported_instruction_type():
    assert len(IFEVAL_SUBSET_BOUNDARY.supported_types) >= 1


@pytest.mark.unit
def test_ifeval_subset_boundary_has_at_least_one_unsupported_instruction_type():
    assert len(IFEVAL_SUBSET_BOUNDARY.unsupported_types) >= 1


@pytest.mark.unit
def test_ifeval_subset_boundary_supported_and_unsupported_types_are_disjoint():
    assert IFEVAL_SUBSET_BOUNDARY.supported_types.isdisjoint(
        IFEVAL_SUBSET_BOUNDARY.unsupported_types
    )


@pytest.mark.unit
def test_ifeval_subset_boundary_instruction_types_are_unique():
    types = [it.instruction_type for it in IFEVAL_SUBSET_BOUNDARY.instruction_types]
    assert len(types) == len(set(types)), "duplicate instruction types in boundary"


@pytest.mark.unit
def test_ifeval_subset_boundary_is_frozen():
    boundary = IFEVAL_SUBSET_BOUNDARY
    with pytest.raises((TypeError, ValueError)):
        boundary.subset_size = 999  # type: ignore[misc]


@pytest.mark.unit
def test_ifeval_subset_boundary_rejects_extra_fields():
    with pytest.raises((TypeError, ValueError)):
        IFEvalSubsetBoundary(
            suite_id="ifeval_subset",
            subset_size=5,
            upstream_revision="rev",
            selected_keys=[1],
            instruction_types=[],
            extra_field="not allowed",  # type: ignore[call-arg]
        )


@pytest.mark.unit
def test_ifeval_verifier_supported_types_match_boundary():
    """The verifier's implemented instruction checks must match the
    boundary's supported types. This prevents silent drift between the
    verifier implementation and the boundary declaration."""
    verifier = IFEvalVerifier()
    supported_in_boundary = IFEVAL_SUBSET_BOUNDARY.supported_types

    test_cases = {
        "punctuation:no_comma": ("no comma here", [{}]),
        "keywords:forbidden_words": ("no forbidden words", [{"forbidden_words": ["apple"]}]),
        "keywords:existence": ("the keyword appears", [{"keywords": ["keyword"]}]),
        "detectable_format:json_format": ('{"valid": true}', [{}]),
        "length_constraints:number_words": ("one two three four five", [{"relation": "at least", "num_words": 3}]),
        "change_case:english_capital": ("ALL CAPS", [{}]),
        "change_case:english_lowercase": ("all lowercase", [{}]),
    }

    for inst_type, (answer, kwargs) in test_cases.items():
        assert inst_type in supported_in_boundary, (
            f"verifier implements '{inst_type}' but boundary does not declare it as supported"
        )
        score = verifier.verify("test", "prompt", answer, [inst_type], kwargs)
        assert score.passed, (
            f"verifier failed for supported instruction '{inst_type}' with a valid answer"
        )


@pytest.mark.unit
def test_ifeval_verifier_unsupported_types_fail_closed():
    """Every instruction type declared as unsupported in the boundary
    must fail closed (return False) in the verifier."""
    verifier = IFEvalVerifier()
    for inst_type in IFEVAL_SUBSET_BOUNDARY.unsupported_types:
        score = verifier.verify("test", "prompt", "nonempty answer", [inst_type], [{}])
        assert not score.passed, (
            f"unsupported instruction '{inst_type}' did not fail closed"
        )


@pytest.mark.unit
def test_ifeval_verifier_unknown_instruction_type_fails_closed():
    verifier = IFEvalVerifier()
    score = verifier.verify("test", "prompt", "nonempty", ["unknown:unknown_type"], [{}])
    assert not score.passed


@pytest.mark.integration
def test_ifeval_subset_dataset_keys_match_boundary_selected_keys():
    data_path = _GOLD_SETS_DIR / "ifeval_subset" / "input_data.jsonl"
    rows = [json.loads(line) for line in data_path.read_text().splitlines() if line.strip()]
    actual_keys = [row["key"] for row in rows]
    assert actual_keys == IFEVAL_SUBSET_BOUNDARY.selected_keys


@pytest.mark.integration
def test_ifeval_subset_dataset_instruction_types_are_within_supported_set():
    """Every instruction type used in the subset dataset must be in the
    boundary's supported set. This prevents the subset from containing
    tasks that the partial verifier cannot evaluate."""
    data_path = _GOLD_SETS_DIR / "ifeval_subset" / "input_data.jsonl"
    rows = [json.loads(line) for line in data_path.read_text().splitlines() if line.strip()]
    supported = IFEVAL_SUBSET_BOUNDARY.supported_types
    for row in rows:
        for inst_id in row["instruction_id_list"]:
            assert inst_id in supported, (
                f"dataset task {row['key']} uses instruction '{inst_id}' "
                f"which is not in the boundary's supported set"
            )
