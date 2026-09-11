# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the paired model-comparison authority and engine.

Verifies the D10/D18/D19 frozen comparison preregistration, pairing
by exact ``(task_id, repetition_id)`` identity, majority reduction
of three matched repetitions, missingness policy enforcement, exact
McNemar inference, paired risk difference, deterministic task-cluster
bootstrap intervals, Holm-Bonferroni family isolation, insufficient
population handling, stable ordering, hash mutation detection, and
rejection of unauthorized comparisons.

Golden vectors cover pairing, majority reduction, ties or missing
cells, exact McNemar results, bootstrap determinism, Holm family
isolation, insufficient populations, stable ordering, hash mutation,
and rejection of unauthorized comparisons.

No external dependencies (no files, network, or DB).
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with inconsistent configurations
# to verify validation rejects them.

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.model_comparison import (
    MODEL_COMPARISON_AUTHORITY_VERSION,
    MODEL_COMPARISON_ENGINE_VERSION,
    ClassAnchorBinding,
    ClaimGate,
    ClaimGateStatus,
    ComparisonFamilyKind,
    ComparisonPairKey,
    ComparisonTest,
    CorrectionMethod,
    MissingnessPolicy,
    ModelComparisonAuthorityHashes,
    ModelComparisonOutput,
    ModelComparisonPreregistration,
    RepetitionCell,
    RepetitionReductionPolicy,
    TaskMajorityOutcome,
    compute_code_metric_identity_hash,
    compute_model_comparison,
    compute_model_comparison_hash,
    pair_task_majority_outcomes,
    reduce_repetitions_to_majority,
)
from g8e_evals.registry import WeightClass


pytestmark = pytest.mark.unit

_HASH_A = "a" * 64
_HASH_B = "b" * 64
_HASH_C = "c" * 64
_HASH_D = "d" * 64
_HASH_E = "e" * 64
_HASH_F = "f" * 64
_HASH_G = "g" * 64
_HASH_H = "h" * 64

_GLOBAL_ANCHOR = "qwen3-8b-q4_0"
_HEAVY_ANCHOR = "granite-33-8b-instruct"
_SMALL_ANCHOR = "phi-4-mini-instruct"
_TINY_ANCHOR = "smollm2-360m-instruct"

_GLOBAL_FAMILY = "global-anchor-family"
_HEAVY_FAMILY = "heavy-slm-class-family"
_SMALL_FAMILY = "small-reasoning-class-family"
_TINY_FAMILY = "tiny-generative-class-family"


def _make_class_anchors() -> list[ClassAnchorBinding]:
    return [
        ClassAnchorBinding(
            weight_class=WeightClass.HEAVY_SLM,
            anchor_variant_id=_HEAVY_ANCHOR,
            family_name=_HEAVY_FAMILY,
        ),
        ClassAnchorBinding(
            weight_class=WeightClass.SMALL_REASONING,
            anchor_variant_id=_SMALL_ANCHOR,
            family_name=_SMALL_FAMILY,
        ),
        ClassAnchorBinding(
            weight_class=WeightClass.TINY_GENERATIVE,
            anchor_variant_id=_TINY_ANCHOR,
            family_name=_TINY_FAMILY,
        ),
    ]


def _make_pair_keys() -> list[ComparisonPairKey]:
    return [
        ComparisonPairKey(
            candidate_variant_id="candidate-a",
            anchor_variant_id=_GLOBAL_ANCHOR,
            family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
            family_name=_GLOBAL_FAMILY,
        ),
        ComparisonPairKey(
            candidate_variant_id="candidate-b",
            anchor_variant_id=_GLOBAL_ANCHOR,
            family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
            family_name=_GLOBAL_FAMILY,
        ),
        ComparisonPairKey(
            candidate_variant_id="candidate-c",
            anchor_variant_id=_HEAVY_ANCHOR,
            family_kind=ComparisonFamilyKind.CLASS_ANCHOR,
            family_name=_HEAVY_FAMILY,
            weight_class=WeightClass.HEAVY_SLM,
        ),
    ]


def _make_preregistration(
    pair_keys: list[ComparisonPairKey] | None = None,
    claim_gate: ClaimGate = ClaimGate.DESCRIPTIVE_ONLY,
    missingness_policy: MissingnessPolicy = MissingnessPolicy.REJECT_PAIR,
    minimum_population: int = 5,
    non_inferiority_margin: float | None = None,
    significance_level: float = 0.05,
    repetition_count: int = 3,
) -> ModelComparisonPreregistration:
    pk = pair_keys if pair_keys is not None else _make_pair_keys()
    prereg = ModelComparisonPreregistration.model_construct(
        authority_id="model-comparison-authority-1",
        authority_version=MODEL_COMPARISON_AUTHORITY_VERSION,
        schema_version="1.0.0",
        global_anchor_variant_id=_GLOBAL_ANCHOR,
        global_anchor_family_name=_GLOBAL_FAMILY,
        class_anchors=_make_class_anchors(),
        pair_keys=pk,
        repetition_count=repetition_count,
        repetition_reduction_policy=RepetitionReductionPolicy.MAJORITY_BINARY,
        missingness_policy=missingness_policy,
        minimum_population=minimum_population,
        primary_test=ComparisonTest.EXACT_MCNEMAR,
        correction_method=CorrectionMethod.HOLM_BONFERRONI,
        significance_level=significance_level,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=42,
        non_inferiority_margin=non_inferiority_margin,
        claim_gate=claim_gate,
        environment_stratum="single-machine",
        content_hash="0" * 64,
    )
    expected = compute_model_comparison_hash(prereg)
    return ModelComparisonPreregistration(
        authority_id="model-comparison-authority-1",
        authority_version=MODEL_COMPARISON_AUTHORITY_VERSION,
        schema_version="1.0.0",
        global_anchor_variant_id=_GLOBAL_ANCHOR,
        global_anchor_family_name=_GLOBAL_FAMILY,
        class_anchors=_make_class_anchors(),
        pair_keys=pk,
        repetition_count=repetition_count,
        repetition_reduction_policy=RepetitionReductionPolicy.MAJORITY_BINARY,
        missingness_policy=missingness_policy,
        minimum_population=minimum_population,
        primary_test=ComparisonTest.EXACT_MCNEMAR,
        correction_method=CorrectionMethod.HOLM_BONFERRONI,
        significance_level=significance_level,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=42,
        non_inferiority_margin=non_inferiority_margin,
        claim_gate=claim_gate,
        environment_stratum="single-machine",
        content_hash=expected,
    )


def _make_authority_hashes() -> ModelComparisonAuthorityHashes:
    return ModelComparisonAuthorityHashes(
        aggregate_verification_hash=_HASH_A,
        campaign_set_plan_hash=_HASH_B,
        campaign_set_index_hash=_HASH_C,
        profile_hash=_HASH_D,
        registry_hash=_HASH_E,
        benchmark_population_hash=_HASH_F,
        metric_registry_hash=_HASH_G,
    )


def _make_cell(variant_id: str, task_id: str, rep_id: str, passed: bool | None) -> RepetitionCell:
    return RepetitionCell(
        variant_id=variant_id,
        task_id=task_id,
        repetition_id=rep_id,
        passed=passed,
    )


def _make_cells_for_variant(
    variant_id: str,
    task_ids: list[str],
    outcomes: dict[str, list[bool | None]],
    repetition_ids: list[str],
) -> list[RepetitionCell]:
    cells: list[RepetitionCell] = []
    for task_id in task_ids:
        task_outcomes = outcomes.get(task_id, [])
        for i, rep_id in enumerate(repetition_ids):
            passed = task_outcomes[i] if i < len(task_outcomes) else None
            cells.append(_make_cell(variant_id, task_id, rep_id, passed))
    return cells


class TestPreregistrationModel:
    def test_preregistration_is_frozen_with_extra_forbid(self) -> None:
        prereg = _make_preregistration()
        data = prereg.model_dump()
        data["unknown_field"] = "rejected"
        with pytest.raises(ValidationError, match="extra_forbidden"):
            ModelComparisonPreregistration.model_validate(data)

    def test_content_hash_must_match_computed(self) -> None:
        base = _make_preregistration()
        with pytest.raises(ValueError, match="content_hash mismatch"):
            ModelComparisonPreregistration(
                authority_id=base.authority_id,
                authority_version=base.authority_version,
                schema_version=base.schema_version,
                global_anchor_variant_id=base.global_anchor_variant_id,
                global_anchor_family_name=base.global_anchor_family_name,
                class_anchors=base.class_anchors,
                pair_keys=base.pair_keys,
                repetition_count=base.repetition_count,
                repetition_reduction_policy=base.repetition_reduction_policy,
                missingness_policy=base.missingness_policy,
                minimum_population=base.minimum_population,
                primary_test=base.primary_test,
                correction_method=base.correction_method,
                significance_level=base.significance_level,
                bootstrap_count=base.bootstrap_count,
                bootstrap_confidence=base.bootstrap_confidence,
                bootstrap_seed=base.bootstrap_seed,
                non_inferiority_margin=base.non_inferiority_margin,
                claim_gate=base.claim_gate,
                environment_stratum=base.environment_stratum,
                content_hash="1" * 64,
            )

    def test_duplicate_weight_class_in_class_anchors_rejected(self) -> None:
        anchors = _make_class_anchors()
        anchors.append(ClassAnchorBinding(
            weight_class=WeightClass.HEAVY_SLM,
            anchor_variant_id="other-heavy-anchor",
            family_name="other-heavy-family",
        ))
        base = _make_preregistration(
            pair_keys=[
                ComparisonPairKey(
                    candidate_variant_id="candidate-a",
                    anchor_variant_id=_GLOBAL_ANCHOR,
                    family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
                    family_name=_GLOBAL_FAMILY,
                ),
            ],
        )
        with pytest.raises(ValueError, match="duplicate weight class"):
            ModelComparisonPreregistration(
                authority_id=base.authority_id,
                authority_version=base.authority_version,
                schema_version=base.schema_version,
                global_anchor_variant_id=base.global_anchor_variant_id,
                global_anchor_family_name=base.global_anchor_family_name,
                class_anchors=anchors,
                pair_keys=base.pair_keys,
                repetition_count=base.repetition_count,
                repetition_reduction_policy=base.repetition_reduction_policy,
                missingness_policy=base.missingness_policy,
                minimum_population=base.minimum_population,
                primary_test=base.primary_test,
                correction_method=base.correction_method,
                significance_level=base.significance_level,
                bootstrap_count=base.bootstrap_count,
                bootstrap_confidence=base.bootstrap_confidence,
                bootstrap_seed=base.bootstrap_seed,
                non_inferiority_margin=base.non_inferiority_margin,
                claim_gate=base.claim_gate,
                environment_stratum=base.environment_stratum,
                content_hash="0" * 64,
            )

    def test_global_anchor_cannot_also_be_class_anchor(self) -> None:
        anchors = [
            ClassAnchorBinding(
                weight_class=WeightClass.HEAVY_SLM,
                anchor_variant_id=_GLOBAL_ANCHOR,
                family_name=_HEAVY_FAMILY,
            ),
        ]
        base = _make_preregistration(
            pair_keys=[
                ComparisonPairKey(
                    candidate_variant_id="candidate-a",
                    anchor_variant_id=_GLOBAL_ANCHOR,
                    family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
                    family_name=_GLOBAL_FAMILY,
                ),
            ],
        )
        with pytest.raises(ValueError, match="also a class anchor"):
            ModelComparisonPreregistration(
                authority_id=base.authority_id,
                authority_version=base.authority_version,
                schema_version=base.schema_version,
                global_anchor_variant_id=base.global_anchor_variant_id,
                global_anchor_family_name=base.global_anchor_family_name,
                class_anchors=anchors,
                pair_keys=base.pair_keys,
                repetition_count=base.repetition_count,
                repetition_reduction_policy=base.repetition_reduction_policy,
                missingness_policy=base.missingness_policy,
                minimum_population=base.minimum_population,
                primary_test=base.primary_test,
                correction_method=base.correction_method,
                significance_level=base.significance_level,
                bootstrap_count=base.bootstrap_count,
                bootstrap_confidence=base.bootstrap_confidence,
                bootstrap_seed=base.bootstrap_seed,
                non_inferiority_margin=base.non_inferiority_margin,
                claim_gate=base.claim_gate,
                environment_stratum=base.environment_stratum,
                content_hash="0" * 64,
            )

    def test_global_anchor_pair_key_must_reference_global_anchor(self) -> None:
        bad_pk = ComparisonPairKey(
            candidate_variant_id="candidate-a",
            anchor_variant_id=_HEAVY_ANCHOR,
            family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
            family_name=_GLOBAL_FAMILY,
        )
        with pytest.raises(ValueError, match="non-global anchor"):
            _make_preregistration(pair_keys=[bad_pk])

    def test_class_anchor_pair_key_must_reference_d19_anchor(self) -> None:
        bad_pk = ComparisonPairKey(
            candidate_variant_id="candidate-c",
            anchor_variant_id="wrong-anchor",
            family_kind=ComparisonFamilyKind.CLASS_ANCHOR,
            family_name=_HEAVY_FAMILY,
            weight_class=WeightClass.HEAVY_SLM,
        )
        with pytest.raises(ValueError, match="!= D19 anchor"):
            _make_preregistration(pair_keys=[bad_pk])

    def test_class_anchor_pair_key_requires_weight_class(self) -> None:
        with pytest.raises(ValidationError, match="requires weight_class"):
            ComparisonPairKey(
                candidate_variant_id="candidate-c",
                anchor_variant_id=_HEAVY_ANCHOR,
                family_kind=ComparisonFamilyKind.CLASS_ANCHOR,
                family_name=_HEAVY_FAMILY,
            )

    def test_global_anchor_pair_key_must_not_set_weight_class(self) -> None:
        with pytest.raises(ValidationError, match="must not set weight_class"):
            ComparisonPairKey(
                candidate_variant_id="candidate-a",
                anchor_variant_id=_GLOBAL_ANCHOR,
                family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
                family_name=_GLOBAL_FAMILY,
                weight_class=WeightClass.HEAVY_SLM,
            )

    def test_candidate_cannot_equal_anchor(self) -> None:
        with pytest.raises(ValidationError, match="cannot equal anchor"):
            ComparisonPairKey(
                candidate_variant_id=_GLOBAL_ANCHOR,
                anchor_variant_id=_GLOBAL_ANCHOR,
                family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
                family_name=_GLOBAL_FAMILY,
            )

    def test_duplicate_pair_keys_rejected(self) -> None:
        pk = _make_pair_keys()[0]
        with pytest.raises(ValueError, match="duplicate pair key"):
            _make_preregistration(pair_keys=[pk, pk])

    def test_changed_preregistration_changes_hash(self) -> None:
        base = _make_preregistration()
        modified = _make_preregistration(minimum_population=10)
        assert base.content_hash != modified.content_hash

    def test_global_family_name_cannot_collide_with_class_family(self) -> None:
        anchors = [
            ClassAnchorBinding(
                weight_class=WeightClass.HEAVY_SLM,
                anchor_variant_id=_HEAVY_ANCHOR,
                family_name=_GLOBAL_FAMILY,
            ),
        ]
        base = _make_preregistration(
            pair_keys=[
                ComparisonPairKey(
                    candidate_variant_id="candidate-a",
                    anchor_variant_id=_GLOBAL_ANCHOR,
                    family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
                    family_name=_GLOBAL_FAMILY,
                ),
            ],
        )
        with pytest.raises(ValueError, match="collides with a class anchor"):
            ModelComparisonPreregistration(
                authority_id=base.authority_id,
                authority_version=base.authority_version,
                schema_version=base.schema_version,
                global_anchor_variant_id=base.global_anchor_variant_id,
                global_anchor_family_name=base.global_anchor_family_name,
                class_anchors=anchors,
                pair_keys=base.pair_keys,
                repetition_count=base.repetition_count,
                repetition_reduction_policy=base.repetition_reduction_policy,
                missingness_policy=base.missingness_policy,
                minimum_population=base.minimum_population,
                primary_test=base.primary_test,
                correction_method=base.correction_method,
                significance_level=base.significance_level,
                bootstrap_count=base.bootstrap_count,
                bootstrap_confidence=base.bootstrap_confidence,
                bootstrap_seed=base.bootstrap_seed,
                non_inferiority_margin=base.non_inferiority_margin,
                claim_gate=base.claim_gate,
                environment_stratum=base.environment_stratum,
                content_hash="0" * 64,
            )


class TestMajorityReduction:
    def test_three_passes_majority_passes(self) -> None:
        cells = [
            _make_cell("v1", "t1", "r1", True),
            _make_cell("v1", "t1", "r2", True),
            _make_cell("v1", "t1", "r3", True),
        ]
        outcome = reduce_repetitions_to_majority(cells, 3, MissingnessPolicy.REJECT_PAIR)
        assert outcome.majority_passed is True
        assert outcome.pass_count == 3
        assert outcome.rejected_for_missingness is False

    def test_two_of_three_passes_majority_passes(self) -> None:
        cells = [
            _make_cell("v1", "t1", "r1", True),
            _make_cell("v1", "t1", "r2", True),
            _make_cell("v1", "t1", "r3", False),
        ]
        outcome = reduce_repetitions_to_majority(cells, 3, MissingnessPolicy.REJECT_PAIR)
        assert outcome.majority_passed is True
        assert outcome.pass_count == 2

    def test_one_of_three_fails_majority(self) -> None:
        cells = [
            _make_cell("v1", "t1", "r1", True),
            _make_cell("v1", "t1", "r2", False),
            _make_cell("v1", "t1", "r3", False),
        ]
        outcome = reduce_repetitions_to_majority(cells, 3, MissingnessPolicy.REJECT_PAIR)
        assert outcome.majority_passed is False
        assert outcome.pass_count == 1

    def test_zero_passes_majority_fails(self) -> None:
        cells = [
            _make_cell("v1", "t1", "r1", False),
            _make_cell("v1", "t1", "r2", False),
            _make_cell("v1", "t1", "r3", False),
        ]
        outcome = reduce_repetitions_to_majority(cells, 3, MissingnessPolicy.REJECT_PAIR)
        assert outcome.majority_passed is False
        assert outcome.pass_count == 0

    def test_reject_pair_policy_rejects_missing_cell(self) -> None:
        cells = [
            _make_cell("v1", "t1", "r1", True),
            _make_cell("v1", "t1", "r2", None),
            _make_cell("v1", "t1", "r3", True),
        ]
        outcome = reduce_repetitions_to_majority(cells, 3, MissingnessPolicy.REJECT_PAIR)
        assert outcome.majority_passed is None
        assert outcome.rejected_for_missingness is True
        assert outcome.missing_repetition_count == 1

    def test_treat_as_failure_policy_keeps_pair(self) -> None:
        cells = [
            _make_cell("v1", "t1", "r1", True),
            _make_cell("v1", "t1", "r2", None),
            _make_cell("v1", "t1", "r3", True),
        ]
        outcome = reduce_repetitions_to_majority(cells, 3, MissingnessPolicy.TREAT_AS_FAILURE)
        assert outcome.majority_passed is True
        assert outcome.rejected_for_missingness is False
        assert outcome.pass_count == 2
        assert outcome.missing_repetition_count == 1

    def test_wrong_cell_count_raises(self) -> None:
        cells = [
            _make_cell("v1", "t1", "r1", True),
            _make_cell("v1", "t1", "r2", True),
        ]
        with pytest.raises(ValueError, match="repetition cell count"):
            reduce_repetitions_to_majority(cells, 3, MissingnessPolicy.REJECT_PAIR)


class TestRepetitionIdMatching:
    """Golden vectors for (task_id, repetition_id) identity matching."""

    def test_duplicate_repetition_id_within_task_rejected(self) -> None:
        from g8e_evals.model_comparison import _reduce_cells_by_task
        cells = [
            _make_cell("v1", "t1", "r1", True),
            _make_cell("v1", "t1", "r1", False),
            _make_cell("v1", "t1", "r2", True),
        ]
        with pytest.raises(ValueError, match="duplicate repetition_id"):
            _reduce_cells_by_task(cells, 3, MissingnessPolicy.REJECT_PAIR)

    def test_cross_variant_repetition_id_mismatch_rejected(self) -> None:
        from g8e_evals.model_comparison import _validate_repetition_id_matching
        candidate = [
            _make_cell("c1", "t1", "r1", True),
            _make_cell("c1", "t1", "r2", True),
            _make_cell("c1", "t1", "r3", True),
        ]
        anchor = [
            _make_cell("a1", "t1", "r1", True),
            _make_cell("a1", "t1", "r2", True),
            _make_cell("a1", "t1", "r4", True),
        ]
        with pytest.raises(ValueError, match="repetition_id mismatch"):
            _validate_repetition_id_matching(candidate, anchor)

    def test_matching_repetition_ids_accepted(self) -> None:
        from g8e_evals.model_comparison import _validate_repetition_id_matching
        candidate = [
            _make_cell("c1", "t1", "r1", True),
            _make_cell("c1", "t1", "r2", True),
            _make_cell("c1", "t1", "r3", True),
        ]
        anchor = [
            _make_cell("a1", "t1", "r1", True),
            _make_cell("a1", "t1", "r2", True),
            _make_cell("a1", "t1", "r3", True),
        ]
        _validate_repetition_id_matching(candidate, anchor)

    def test_non_overlapping_tasks_accepted(self) -> None:
        from g8e_evals.model_comparison import _validate_repetition_id_matching
        candidate = [
            _make_cell("c1", "t1", "r1", True),
            _make_cell("c1", "t1", "r2", True),
            _make_cell("c1", "t1", "r3", True),
        ]
        anchor = [
            _make_cell("a1", "t2", "r1", True),
            _make_cell("a1", "t2", "r2", True),
            _make_cell("a1", "t2", "r3", True),
        ]
        _validate_repetition_id_matching(candidate, anchor)

    def test_repetition_id_mismatch_rejected_in_engine(self) -> None:
        prereg = _make_preregistration(minimum_population=3)
        task_ids = ["t1", "t2", "t3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {tid: [True, True, True] for tid in task_ids},
            ["r1", "r2", "r3"],
        )
        anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [True, True, True] for tid in task_ids},
            ["r1", "r2", "r4"],
        )
        cells = candidate_cells + anchor_cells
        with pytest.raises(ValueError, match="repetition_id mismatch"):
            compute_model_comparison(prereg, cells, _make_authority_hashes())


class TestDescriptiveStability:
    """Golden vectors for 0/3 through 3/3 count retention."""

    def test_zero_of_three_count_retained(self) -> None:
        cells = [
            _make_cell("v1", "t1", "r1", False),
            _make_cell("v1", "t1", "r2", False),
            _make_cell("v1", "t1", "r3", False),
        ]
        outcome = reduce_repetitions_to_majority(cells, 3, MissingnessPolicy.REJECT_PAIR)
        assert outcome.pass_count == 0
        assert outcome.total_repetitions == 3
        assert outcome.majority_passed is False

    def test_one_of_three_count_retained(self) -> None:
        cells = [
            _make_cell("v1", "t1", "r1", True),
            _make_cell("v1", "t1", "r2", False),
            _make_cell("v1", "t1", "r3", False),
        ]
        outcome = reduce_repetitions_to_majority(cells, 3, MissingnessPolicy.REJECT_PAIR)
        assert outcome.pass_count == 1
        assert outcome.total_repetitions == 3
        assert outcome.majority_passed is False

    def test_two_of_three_count_retained(self) -> None:
        cells = [
            _make_cell("v1", "t1", "r1", True),
            _make_cell("v1", "t1", "r2", True),
            _make_cell("v1", "t1", "r3", False),
        ]
        outcome = reduce_repetitions_to_majority(cells, 3, MissingnessPolicy.REJECT_PAIR)
        assert outcome.pass_count == 2
        assert outcome.total_repetitions == 3
        assert outcome.majority_passed is True

    def test_three_of_three_count_retained(self) -> None:
        cells = [
            _make_cell("v1", "t1", "r1", True),
            _make_cell("v1", "t1", "r2", True),
            _make_cell("v1", "t1", "r3", True),
        ]
        outcome = reduce_repetitions_to_majority(cells, 3, MissingnessPolicy.REJECT_PAIR)
        assert outcome.pass_count == 3
        assert outcome.total_repetitions == 3
        assert outcome.majority_passed is True

    def test_all_four_counts_visible_in_engine_output(self) -> None:
        prereg = _make_preregistration(
            pair_keys=[ComparisonPairKey(
                candidate_variant_id="candidate-a",
                anchor_variant_id=_GLOBAL_ANCHOR,
                family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
                family_name=_GLOBAL_FAMILY,
            )],
            minimum_population=4,
        )
        task_ids = ["t1", "t2", "t3", "t4"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {
                "t1": [False, False, False],
                "t2": [True, False, False],
                "t3": [True, True, False],
                "t4": [True, True, True],
            },
            rep_ids,
        )
        anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        cells = candidate_cells + anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        result = output.results[0]
        paired_outcomes_by_task = {p.task_id: p for p in result.paired_task_outcomes}
        candidate_outcomes = {
            p.task_id: p for p in result.paired_task_outcomes
        }
        assert len(candidate_outcomes) == 4
        assert paired_outcomes_by_task["t1"].candidate_majority is False
        assert paired_outcomes_by_task["t2"].candidate_majority is False
        assert paired_outcomes_by_task["t3"].candidate_majority is True
        assert paired_outcomes_by_task["t4"].candidate_majority is True


class TestTiesAndConcordantPairs:
    """Golden vectors for tied and concordant paired outcomes."""

    def test_all_concordant_pass_produces_no_discordant_pairs(self) -> None:
        prereg = _make_preregistration(
            pair_keys=[ComparisonPairKey(
                candidate_variant_id="candidate-a",
                anchor_variant_id=_GLOBAL_ANCHOR,
                family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
                family_name=_GLOBAL_FAMILY,
            )],
            claim_gate=ClaimGate.SUPERIORITY,
            minimum_population=5,
        )
        task_ids = ["t1", "t2", "t3", "t4", "t5"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        cells = candidate_cells + anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        result = output.results[0]
        assert result.mcnemar_statistic is None
        assert result.mcnemar_p_value is None
        assert result.paired_risk_difference == 0.0
        assert result.claim_gate_status == ClaimGateStatus.NO_DISCORDANT_PAIRS

    def test_all_concordant_fail_produces_no_discordant_pairs(self) -> None:
        prereg = _make_preregistration(
            pair_keys=[ComparisonPairKey(
                candidate_variant_id="candidate-a",
                anchor_variant_id=_GLOBAL_ANCHOR,
                family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
                family_name=_GLOBAL_FAMILY,
            )],
            claim_gate=ClaimGate.SUPERIORITY,
            minimum_population=5,
        )
        task_ids = ["t1", "t2", "t3", "t4", "t5"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {tid: [False, False, False] for tid in task_ids},
            rep_ids,
        )
        anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [False, False, False] for tid in task_ids},
            rep_ids,
        )
        cells = candidate_cells + anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        result = output.results[0]
        assert result.mcnemar_statistic is None
        assert result.paired_risk_difference == 0.0

    def test_mixed_concordant_and_discordant_pairs(self) -> None:
        prereg = _make_preregistration(
            pair_keys=[ComparisonPairKey(
                candidate_variant_id="candidate-a",
                anchor_variant_id=_GLOBAL_ANCHOR,
                family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
                family_name=_GLOBAL_FAMILY,
            )],
            claim_gate=ClaimGate.DESCRIPTIVE_ONLY,
            minimum_population=3,
        )
        task_ids = ["t1", "t2", "t3", "t4"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {
                "t1": [True, True, True],
                "t2": [True, True, True],
                "t3": [False, False, False],
                "t4": [False, False, False],
            },
            rep_ids,
        )
        anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {
                "t1": [True, True, True],
                "t2": [False, False, False],
                "t3": [False, False, False],
                "t4": [True, True, True],
            },
            rep_ids,
        )
        cells = candidate_cells + anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        result = output.results[0]
        assert result.paired_task_count == 4
        assert result.candidate_pass_count == 2
        assert result.anchor_pass_count == 2
        assert result.paired_risk_difference == 0.0
        assert result.mcnemar_statistic is not None
        assert result.mcnemar_p_value is not None


class TestPairing:
    def test_pairs_match_by_task_id(self) -> None:
        candidate = [
            TaskMajorityOutcome(
                variant_id="c1", task_id="t1", pass_count=3, total_repetitions=3,
                majority_passed=True, rejected_for_missingness=False, missing_repetition_count=0,
            ),
            TaskMajorityOutcome(
                variant_id="c1", task_id="t2", pass_count=0, total_repetitions=3,
                majority_passed=False, rejected_for_missingness=False, missing_repetition_count=0,
            ),
        ]
        anchor = [
            TaskMajorityOutcome(
                variant_id="a1", task_id="t1", pass_count=3, total_repetitions=3,
                majority_passed=True, rejected_for_missingness=False, missing_repetition_count=0,
            ),
            TaskMajorityOutcome(
                variant_id="a1", task_id="t2", pass_count=3, total_repetitions=3,
                majority_passed=True, rejected_for_missingness=False, missing_repetition_count=0,
            ),
        ]
        paired = pair_task_majority_outcomes(candidate, anchor)
        assert len(paired) == 2
        assert paired[0].task_id == "t1"
        assert paired[0].candidate_majority is True
        assert paired[0].anchor_majority is True
        assert paired[1].task_id == "t2"
        assert paired[1].candidate_majority is False
        assert paired[1].anchor_majority is True

    def test_task_in_one_sequence_not_other_not_paired(self) -> None:
        candidate = [
            TaskMajorityOutcome(
                variant_id="c1", task_id="t1", pass_count=3, total_repetitions=3,
                majority_passed=True, rejected_for_missingness=False, missing_repetition_count=0,
            ),
        ]
        anchor = [
            TaskMajorityOutcome(
                variant_id="a1", task_id="t2", pass_count=3, total_repetitions=3,
                majority_passed=True, rejected_for_missingness=False, missing_repetition_count=0,
            ),
        ]
        paired = pair_task_majority_outcomes(candidate, anchor)
        assert len(paired) == 0

    def test_rejected_pair_not_in_inferential_population(self) -> None:
        candidate = [
            TaskMajorityOutcome(
                variant_id="c1", task_id="t1", pass_count=2, total_repetitions=3,
                majority_passed=None, rejected_for_missingness=True, missing_repetition_count=1,
            ),
        ]
        anchor = [
            TaskMajorityOutcome(
                variant_id="a1", task_id="t1", pass_count=3, total_repetitions=3,
                majority_passed=True, rejected_for_missingness=False, missing_repetition_count=0,
            ),
        ]
        paired = pair_task_majority_outcomes(candidate, anchor)
        assert len(paired) == 1
        assert paired[0].in_inferential_population is False
        assert paired[0].candidate_majority is None


class TestExactMcNemar:
    def test_no_discordant_pairs_returns_none(self) -> None:
        from g8e_evals.model_comparison import _exact_mcnemar
        stat, p = _exact_mcnemar([True, True], [True, True])
        assert stat is None
        assert p is None

    def test_all_discordant_same_direction(self) -> None:
        from g8e_evals.model_comparison import _exact_mcnemar
        stat, p = _exact_mcnemar([True, True, True, True], [False, False, False, False])
        assert stat == 4.0
        assert p is not None
        assert 0.0 < p < 1.0

    def test_mixed_discordant(self) -> None:
        from g8e_evals.model_comparison import _exact_mcnemar
        stat, p = _exact_mcnemar(
            [True, False, True, False, True, True],
            [False, True, True, False, False, True],
        )
        assert stat is not None
        assert p is not None

    def test_empty_input_returns_none(self) -> None:
        from g8e_evals.model_comparison import _exact_mcnemar
        stat, p = _exact_mcnemar([], [])
        assert stat is None
        assert p is None


class TestBootstrapDeterminism:
    def test_identical_inputs_produce_identical_ci(self) -> None:
        from g8e_evals.model_comparison import _task_cluster_bootstrap_ci
        candidate = [True, False, True, True, False, True, True, False, True, True]
        anchor = [False, False, True, False, False, True, False, False, True, False]
        ci1 = _task_cluster_bootstrap_ci(candidate, anchor, 1000, 0.95, 42)
        ci2 = _task_cluster_bootstrap_ci(candidate, anchor, 1000, 0.95, 42)
        assert ci1 == ci2

    def test_different_seed_produces_different_ci(self) -> None:
        from g8e_evals.model_comparison import _task_cluster_bootstrap_ci
        candidate = [True, False, True, True, False, True, True, False, True, True,
                     False, True, False, True, True, False, True, False, True, False]
        anchor = [False, False, True, False, False, True, False, False, True, False,
                  True, False, True, False, True, False, True, False, True, False]
        ci1 = _task_cluster_bootstrap_ci(candidate, anchor, 1000, 0.95, 42)
        ci2 = _task_cluster_bootstrap_ci(candidate, anchor, 1000, 0.95, 99)
        assert ci1 != ci2

    def test_too_few_values_returns_none(self) -> None:
        from g8e_evals.model_comparison import _task_cluster_bootstrap_ci
        ci = _task_cluster_bootstrap_ci([True], [False], 1000, 0.95, 42)
        assert ci == (None, None)


class TestComputeModelComparison:
    def test_unauthorized_variant_rejected(self) -> None:
        prereg = _make_preregistration()
        cells = [
            _make_cell("unauthorized-variant", "t1", "r1", True),
            _make_cell("unauthorized-variant", "t1", "r2", True),
            _make_cell("unauthorized-variant", "t1", "r3", True),
        ]
        with pytest.raises(ValueError, match="unauthorized variant"):
            compute_model_comparison(prereg, cells, _make_authority_hashes())

    def test_empty_cells_produces_empty_results(self) -> None:
        prereg = _make_preregistration()
        output = compute_model_comparison(prereg, [], _make_authority_hashes())
        assert len(output.results) == 3
        for result in output.results:
            assert result.paired_task_count == 0
            assert result.claim_gate_status == ClaimGateStatus.INSUFFICIENT_POPULATION

    def test_insufficient_population_status(self) -> None:
        prereg = _make_preregistration(minimum_population=100)
        task_ids = ["t1", "t2", "t3"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {"t1": [True, True, True], "t2": [True, False, True], "t3": [False, False, False]},
            rep_ids,
        )
        anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {"t1": [True, True, True], "t2": [True, True, True], "t3": [False, False, False]},
            rep_ids,
        )
        cells = candidate_cells + anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        global_result = next(r for r in output.results if r.candidate_variant_id == "candidate-a")
        assert global_result.paired_task_count == 3
        assert global_result.claim_gate_status == ClaimGateStatus.INSUFFICIENT_POPULATION

    def test_descriptive_only_gate_never_passes(self) -> None:
        prereg = _make_preregistration(
            claim_gate=ClaimGate.DESCRIPTIVE_ONLY,
            minimum_population=3,
        )
        task_ids = ["t1", "t2", "t3"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {"t1": [True, True, True], "t2": [True, True, True], "t3": [True, True, True]},
            rep_ids,
        )
        anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {"t1": [False, False, False], "t2": [False, False, False], "t3": [False, False, False]},
            rep_ids,
        )
        cells = candidate_cells + anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        result = next(r for r in output.results if r.candidate_variant_id == "candidate-a")
        assert result.claim_gate_status == ClaimGateStatus.DESCRIPTIVE_ONLY

    def test_superiority_gate_pass_when_significant(self) -> None:
        single_pk = [ComparisonPairKey(
            candidate_variant_id="candidate-a",
            anchor_variant_id=_GLOBAL_ANCHOR,
            family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
            family_name=_GLOBAL_FAMILY,
        )]
        prereg = _make_preregistration(
            pair_keys=single_pk,
            claim_gate=ClaimGate.SUPERIORITY,
            minimum_population=5,
        )
        task_ids = ["t1", "t2", "t3", "t4", "t5", "t6"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [False, False, False] for tid in task_ids},
            rep_ids,
        )
        cells = candidate_cells + anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        result = next(r for r in output.results if r.candidate_variant_id == "candidate-a")
        assert result.claim_gate_status == ClaimGateStatus.PASS
        assert result.paired_risk_difference == 1.0

    def test_superiority_gate_fail_when_not_significant(self) -> None:
        single_pk = [ComparisonPairKey(
            candidate_variant_id="candidate-a",
            anchor_variant_id=_GLOBAL_ANCHOR,
            family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
            family_name=_GLOBAL_FAMILY,
        )]
        prereg = _make_preregistration(
            pair_keys=single_pk,
            claim_gate=ClaimGate.SUPERIORITY,
            minimum_population=5,
        )
        task_ids = ["t1", "t2", "t3", "t4", "t5"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        cells = candidate_cells + anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        result = next(r for r in output.results if r.candidate_variant_id == "candidate-a")
        assert result.claim_gate_status == ClaimGateStatus.NO_DISCORDANT_PAIRS

    def test_non_inferiority_gate_pass_within_margin(self) -> None:
        prereg = _make_preregistration(
            claim_gate=ClaimGate.NON_INFERIORITY,
            minimum_population=5,
            non_inferiority_margin=0.2,
        )
        task_ids = ["t1", "t2", "t3", "t4", "t5", "t6"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        cells = candidate_cells + anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        result = next(r for r in output.results if r.candidate_variant_id == "candidate-a")
        assert result.claim_gate_status == ClaimGateStatus.PASS

    def test_non_inferiority_gate_fail_without_margin(self) -> None:
        prereg = _make_preregistration(
            claim_gate=ClaimGate.NON_INFERIORITY,
            minimum_population=5,
            non_inferiority_margin=None,
        )
        task_ids = ["t1", "t2", "t3", "t4", "t5", "t6"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        cells = candidate_cells + anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        result = next(r for r in output.results if r.candidate_variant_id == "candidate-a")
        assert result.claim_gate_status == ClaimGateStatus.FAIL

    def test_missingness_reject_pair_excludes_task(self) -> None:
        prereg = _make_preregistration(
            missingness_policy=MissingnessPolicy.REJECT_PAIR,
            minimum_population=3,
        )
        task_ids = ["t1", "t2", "t3", "t4"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {
                "t1": [True, True, True],
                "t2": [True, None, True],
                "t3": [False, False, False],
                "t4": [True, True, True],
            },
            rep_ids,
        )
        anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        cells = candidate_cells + anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        result = next(r for r in output.results if r.candidate_variant_id == "candidate-a")
        assert result.paired_task_count == 3
        assert result.rejected_task_count == 1
        assert result.total_task_count == 4

    def test_holm_family_isolation_global_vs_class(self) -> None:
        prereg = _make_preregistration(
            claim_gate=ClaimGate.SUPERIORITY,
            minimum_population=3,
        )
        task_ids = ["t1", "t2", "t3", "t4", "t5"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_a_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        candidate_b_cells = _make_cells_for_variant(
            "candidate-b", task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        candidate_c_cells = _make_cells_for_variant(
            "candidate-c", task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        global_anchor_cells = _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [False, False, False] for tid in task_ids},
            rep_ids,
        )
        heavy_anchor_cells = _make_cells_for_variant(
            _HEAVY_ANCHOR, task_ids,
            {tid: [False, False, False] for tid in task_ids},
            rep_ids,
        )
        cells = candidate_a_cells + candidate_b_cells + candidate_c_cells + global_anchor_cells + heavy_anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        global_results = [r for r in output.results if r.family_kind == ComparisonFamilyKind.GLOBAL_ANCHOR]
        class_results = [r for r in output.results if r.family_kind == ComparisonFamilyKind.CLASS_ANCHOR]
        assert len(global_results) == 2
        assert len(class_results) == 1
        global_ranks = [r.holm_rank for r in global_results]
        class_ranks = [r.holm_rank for r in class_results]
        assert global_ranks == [1, 2]
        assert class_ranks == [1]

    def test_results_sorted_by_family_kind_name_candidate_anchor(self) -> None:
        prereg = _make_preregistration()
        output = compute_model_comparison(prereg, [], _make_authority_hashes())
        keys = [
            (r.family_kind.value, r.family_name, r.candidate_variant_id, r.anchor_variant_id)
            for r in output.results
        ]
        assert keys == sorted(keys)

    def test_output_content_hash_changes_on_different_results(self) -> None:
        prereg = _make_preregistration(minimum_population=3)
        task_ids = ["t1", "t2", "t3"]
        rep_ids = ["r1", "r2", "r3"]
        cells1 = _make_cells_for_variant(
            "candidate-a", task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        ) + _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [False, False, False] for tid in task_ids},
            rep_ids,
        )
        cells2 = _make_cells_for_variant(
            "candidate-a", task_ids,
            {tid: [False, False, False] for tid in task_ids},
            rep_ids,
        ) + _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        output1 = compute_model_comparison(prereg, cells1, _make_authority_hashes())
        output2 = compute_model_comparison(prereg, cells2, _make_authority_hashes())
        assert output1.content_hash != output2.content_hash

    def test_output_binds_all_authority_hashes(self) -> None:
        prereg = _make_preregistration()
        hashes = _make_authority_hashes()
        output = compute_model_comparison(prereg, [], hashes)
        assert output.authority_hash == prereg.content_hash
        assert output.aggregate_verification_hash == hashes.aggregate_verification_hash
        assert output.campaign_set_plan_hash == hashes.campaign_set_plan_hash
        assert output.campaign_set_index_hash == hashes.campaign_set_index_hash
        assert output.profile_hash == hashes.profile_hash
        assert output.registry_hash == hashes.registry_hash
        assert output.benchmark_population_hash == hashes.benchmark_population_hash
        assert output.environment_stratum == prereg.environment_stratum

    def test_output_content_hash_must_match_computed(self) -> None:
        prereg = _make_preregistration()
        output = compute_model_comparison(prereg, [], _make_authority_hashes())
        data = output.model_dump()
        data["content_hash"] = "1" * 64
        with pytest.raises(ValueError, match="content_hash mismatch"):
            ModelComparisonOutput.model_validate(data)

    def test_deterministic_output_for_identical_inputs(self) -> None:
        prereg = _make_preregistration(minimum_population=3)
        task_ids = ["t1", "t2", "t3", "t4", "t5"]
        rep_ids = ["r1", "r2", "r3"]
        cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        ) + _make_cells_for_variant(
            _GLOBAL_ANCHOR, task_ids,
            {tid: [False, False, False] for tid in task_ids},
            rep_ids,
        )
        output1 = compute_model_comparison(prereg, cells, _make_authority_hashes())
        output2 = compute_model_comparison(prereg, cells, _make_authority_hashes())
        assert output1.content_hash == output2.content_hash
        assert output1.results == output2.results

    def test_code_metric_identity_hash_binds_engine_and_registry(self) -> None:
        h1 = compute_code_metric_identity_hash("1.0.0", _HASH_A)
        h2 = compute_code_metric_identity_hash("1.0.0", _HASH_B)
        h3 = compute_code_metric_identity_hash("2.0.0", _HASH_A)
        assert h1 != h2
        assert h1 != h3

    def test_engine_version_pinned(self) -> None:
        assert MODEL_COMPARISON_ENGINE_VERSION == "1.0.0"

    def test_all_pairs_comparison_rejected(self) -> None:
        bad_pk = ComparisonPairKey(
            candidate_variant_id="candidate-a",
            anchor_variant_id="candidate-b",
            family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
            family_name=_GLOBAL_FAMILY,
        )
        with pytest.raises(ValueError, match="non-global anchor"):
            _make_preregistration(pair_keys=[bad_pk])

    def test_anchor_substitution_in_engine_rejected(self) -> None:
        prereg = _make_preregistration()
        task_ids = ["t1", "t2", "t3"]
        rep_ids = ["r1", "r2", "r3"]
        candidate_cells = _make_cells_for_variant(
            "candidate-a", task_ids,
            {tid: [True, True, True] for tid in task_ids},
            rep_ids,
        )
        substituted_anchor_cells = _make_cells_for_variant(
            "candidate-b", task_ids,
            {tid: [False, False, False] for tid in task_ids},
            rep_ids,
        )
        cells = candidate_cells + substituted_anchor_cells
        output = compute_model_comparison(prereg, cells, _make_authority_hashes())
        result = next(r for r in output.results if r.candidate_variant_id == "candidate-a")
        assert result.paired_task_count == 0
        assert result.claim_gate_status == ClaimGateStatus.INSUFFICIENT_POPULATION


class TestAuthorityHashes:
    def test_authority_hashes_frozen_with_extra_forbid(self) -> None:
        with pytest.raises(ValidationError, match="extra_forbidden"):
            ModelComparisonAuthorityHashes(
                aggregate_verification_hash=_HASH_A,
                campaign_set_plan_hash=_HASH_B,
                campaign_set_index_hash=_HASH_C,
                profile_hash=_HASH_D,
                registry_hash=_HASH_E,
                benchmark_population_hash=_HASH_F,
                metric_registry_hash=_HASH_G,
                unknown_field="rejected",
            )

    def test_all_hashes_required(self) -> None:
        with pytest.raises(ValidationError, match="aggregate_verification_hash"):
            ModelComparisonAuthorityHashes(
                campaign_set_plan_hash=_HASH_B,
                campaign_set_index_hash=_HASH_C,
                profile_hash=_HASH_D,
                registry_hash=_HASH_E,
                benchmark_population_hash=_HASH_F,
                metric_registry_hash=_HASH_G,
            )
