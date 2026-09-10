# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the frozen statistical analysis contract and
role-combination track.

Verifies that the statistical analysis contract declares task as the
primary independent sampling unit, paired task-level comparisons, the
exact paired binary procedure for pass/fail outcomes, the paired
task-cluster bootstrap for continuous or repeated outcomes,
preregistered multiple-comparison correction per declared family, paired
effect sizes and uncertainty intervals, Pareto views for quality versus
latency/memory/artifact size, and descriptive labeling when the
population is too small for inferential procedures.

Verifies that the role-combination track defines exact
Primary/Assistant/Lite assignments, freezes a tractable combination
matrix, every benchmark task declares which roles it exercises, each
measured role has matching provider-boundary telemetry, metrics bind
both the exact three-role combination and the individual invoked variant,
and results from different combinations never pool implicitly.

No external dependencies (no files, network, or DB).
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with inconsistent configurations
# to verify validation rejects them.

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.analysis_contract import (
    ComparisonFamily,
    DescriptiveFallbackPolicy,
    EstimandType,
    ParetoDimension,
    ParetoView,
    RoleCombination,
    RoleCombinationId,
    RoleCombinationTrack,
    RoleExerciseDeclaration,
    StatisticalAnalysisContract,
    TaskRoleBinding,
)


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64


class TestStatisticalAnalysisContractModel:
    def test_contract_is_frozen_with_extra_forbid(self):
        """The contract model rejects unknown fields and is frozen."""
        with pytest.raises(ValidationError, match="extra_forbidden"):
            StatisticalAnalysisContract(
                contract_id="contract-1",
                contract_version="1.0.0",
                independent_unit="task",
                comparison_method=ComparisonFamily.PAIRED_TASK_LEVEL,
                binary_test_policy="mcnemar",
                continuous_test_policy="paired_bootstrap",
                bootstrap_count=10000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                correction_family="holm_bonferroni",
                significance_level=0.05,
                estimand=EstimandType.PAIRED_DELTA,
                effect_size_measure="cohens_d",
                pareto_views=[
                    ParetoView(
                        view_id="quality_vs_latency",
                        quality_dimension=ParetoDimension.ACCURACY,
                        cost_dimension=ParetoDimension.LATENCY_SECONDS,
                    ),
                ],
                descriptive_fallback=DescriptiveFallbackPolicy.BELOW_MIN_POPULATION,
                minimum_inferential_population=10,
                content_hash=_VALID_HASH,
                unknown_field="rejected",
            )

    def test_contract_independent_unit_must_be_task(self):
        """The primary independent sampling unit must be 'task'."""
        with pytest.raises(ValidationError, match="independent_unit"):
            StatisticalAnalysisContract(
                contract_id="contract-1",
                contract_version="1.0.0",
                independent_unit="assignment",
                comparison_method=ComparisonFamily.PAIRED_TASK_LEVEL,
                binary_test_policy="mcnemar",
                continuous_test_policy="paired_bootstrap",
                bootstrap_count=10000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                correction_family="holm_bonferroni",
                significance_level=0.05,
                estimand=EstimandType.PAIRED_DELTA,
                effect_size_measure="cohens_d",
                pareto_views=[],
                descriptive_fallback=DescriptiveFallbackPolicy.BELOW_MIN_POPULATION,
                minimum_inferential_population=10,
                content_hash=_VALID_HASH,
            )

    def test_contract_bootstrap_count_must_be_positive(self):
        """Bootstrap count must be at least 1."""
        with pytest.raises(ValidationError, match="bootstrap_count"):
            StatisticalAnalysisContract(
                contract_id="contract-1",
                contract_version="1.0.0",
                independent_unit="task",
                comparison_method=ComparisonFamily.PAIRED_TASK_LEVEL,
                binary_test_policy="mcnemar",
                continuous_test_policy="paired_bootstrap",
                bootstrap_count=0,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                correction_family="holm_bonferroni",
                significance_level=0.05,
                estimand=EstimandType.PAIRED_DELTA,
                effect_size_measure="cohens_d",
                pareto_views=[],
                descriptive_fallback=DescriptiveFallbackPolicy.BELOW_MIN_POPULATION,
                minimum_inferential_population=10,
                content_hash=_VALID_HASH,
            )

    def test_contract_significance_level_must_be_in_range(self):
        """Significance level must be between 0 and 1 (exclusive)."""
        with pytest.raises(ValidationError, match="significance_level"):
            StatisticalAnalysisContract(
                contract_id="contract-1",
                contract_version="1.0.0",
                independent_unit="task",
                comparison_method=ComparisonFamily.PAIRED_TASK_LEVEL,
                binary_test_policy="mcnemar",
                continuous_test_policy="paired_bootstrap",
                bootstrap_count=10000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                correction_family="holm_bonferroni",
                significance_level=0.0,
                estimand=EstimandType.PAIRED_DELTA,
                effect_size_measure="cohens_d",
                pareto_views=[],
                descriptive_fallback=DescriptiveFallbackPolicy.BELOW_MIN_POPULATION,
                minimum_inferential_population=10,
                content_hash=_VALID_HASH,
            )

    def test_contract_content_hash_must_match_computed(self):
        """The content hash must match the computed hash over canonical JSON."""
        from g8e_evals.analysis_contract import compute_contract_hash

        expected = compute_contract_hash(
            contract_id="contract-1",
            contract_version="1.0.0",
            independent_unit="task",
            comparison_method=ComparisonFamily.PAIRED_TASK_LEVEL,
            binary_test_policy="mcnemar",
            continuous_test_policy="paired_bootstrap",
            bootstrap_count=10000,
            bootstrap_confidence=0.95,
            bootstrap_seed=42,
            correction_family="holm_bonferroni",
            significance_level=0.05,
            estimand=EstimandType.PAIRED_DELTA,
            effect_size_measure="cohens_d",
            pareto_views=[
                ParetoView(
                    view_id="quality_vs_latency",
                    quality_dimension=ParetoDimension.ACCURACY,
                    cost_dimension=ParetoDimension.LATENCY_SECONDS,
                ),
            ],
            descriptive_fallback=DescriptiveFallbackPolicy.BELOW_MIN_POPULATION,
            minimum_inferential_population=10,
        )

        with pytest.raises(ValueError, match="content_hash mismatch"):
            StatisticalAnalysisContract(
                contract_id="contract-1",
                contract_version="1.0.0",
                independent_unit="task",
                comparison_method=ComparisonFamily.PAIRED_TASK_LEVEL,
                binary_test_policy="mcnemar",
                continuous_test_policy="paired_bootstrap",
                bootstrap_count=10000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                correction_family="holm_bonferroni",
                significance_level=0.05,
                estimand=EstimandType.PAIRED_DELTA,
                effect_size_measure="cohens_d",
                pareto_views=[
                    ParetoView(
                        view_id="quality_vs_latency",
                        quality_dimension=ParetoDimension.ACCURACY,
                        cost_dimension=ParetoDimension.LATENCY_SECONDS,
                    ),
                ],
                descriptive_fallback=DescriptiveFallbackPolicy.BELOW_MIN_POPULATION,
                minimum_inferential_population=10,
                content_hash="0" * 64,
            )

        contract_valid = StatisticalAnalysisContract(
            contract_id="contract-1",
            contract_version="1.0.0",
            independent_unit="task",
            comparison_method=ComparisonFamily.PAIRED_TASK_LEVEL,
            binary_test_policy="mcnemar",
            continuous_test_policy="paired_bootstrap",
            bootstrap_count=10000,
            bootstrap_confidence=0.95,
            bootstrap_seed=42,
            correction_family="holm_bonferroni",
            significance_level=0.05,
            estimand=EstimandType.PAIRED_DELTA,
            effect_size_measure="cohens_d",
            pareto_views=[
                ParetoView(
                    view_id="quality_vs_latency",
                    quality_dimension=ParetoDimension.ACCURACY,
                    cost_dimension=ParetoDimension.LATENCY_SECONDS,
                ),
            ],
            descriptive_fallback=DescriptiveFallbackPolicy.BELOW_MIN_POPULATION,
            minimum_inferential_population=10,
            content_hash=expected,
        )
        assert contract_valid.content_hash == expected

    def test_contract_pareto_view_dimensions_must_differ(self):
        """A Pareto view's quality and cost dimensions must not be the same."""
        with pytest.raises(ValidationError, match=r"quality_dimension.*cost_dimension"):
            ParetoView(
                view_id="self_pareto",
                quality_dimension=ParetoDimension.ACCURACY,
                cost_dimension=ParetoDimension.ACCURACY,
            )

    def test_contract_pareto_view_ids_must_be_unique(self):
        """Pareto view IDs must be unique within the contract."""
        from g8e_evals.analysis_contract import compute_contract_hash

        view = ParetoView(
            view_id="dup",
            quality_dimension=ParetoDimension.ACCURACY,
            cost_dimension=ParetoDimension.LATENCY_SECONDS,
        )
        pareto_views = [view, view.model_copy(update={"quality_dimension": ParetoDimension.PASS_RATE})]
        ch = compute_contract_hash(
            contract_id="contract-1",
            contract_version="1.0.0",
            independent_unit="task",
            comparison_method=ComparisonFamily.PAIRED_TASK_LEVEL,
            binary_test_policy="mcnemar",
            continuous_test_policy="paired_bootstrap",
            bootstrap_count=10000,
            bootstrap_confidence=0.95,
            bootstrap_seed=42,
            correction_family="holm_bonferroni",
            significance_level=0.05,
            estimand=EstimandType.PAIRED_DELTA,
            effect_size_measure="cohens_d",
            pareto_views=pareto_views,
            descriptive_fallback=DescriptiveFallbackPolicy.BELOW_MIN_POPULATION,
            minimum_inferential_population=10,
        )
        with pytest.raises(ValueError, match=r"duplicate.*pareto.*view_id"):
            StatisticalAnalysisContract(
                contract_id="contract-1",
                contract_version="1.0.0",
                independent_unit="task",
                comparison_method=ComparisonFamily.PAIRED_TASK_LEVEL,
                binary_test_policy="mcnemar",
                continuous_test_policy="paired_bootstrap",
                bootstrap_count=10000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                correction_family="holm_bonferroni",
                significance_level=0.05,
                estimand=EstimandType.PAIRED_DELTA,
                effect_size_measure="cohens_d",
                pareto_views=pareto_views,
                descriptive_fallback=DescriptiveFallbackPolicy.BELOW_MIN_POPULATION,
                minimum_inferential_population=10,
                content_hash=ch,
            )

    def test_contract_round_trip_serialization(self):
        """The contract round-trips through JSON without data loss."""
        import json

        from g8e_evals.analysis_contract import compute_contract_hash

        pareto_views = [
            ParetoView(
                view_id="quality_vs_latency",
                quality_dimension=ParetoDimension.ACCURACY,
                cost_dimension=ParetoDimension.LATENCY_SECONDS,
            ),
            ParetoView(
                view_id="quality_vs_memory",
                quality_dimension=ParetoDimension.PASS_RATE,
                cost_dimension=ParetoDimension.PEAK_MEMORY_BYTES,
            ),
        ]
        ch = compute_contract_hash(
            contract_id="contract-1",
            contract_version="1.0.0",
            independent_unit="task",
            comparison_method=ComparisonFamily.PAIRED_TASK_LEVEL,
            binary_test_policy="mcnemar",
            continuous_test_policy="paired_bootstrap",
            bootstrap_count=10000,
            bootstrap_confidence=0.95,
            bootstrap_seed=42,
            correction_family="holm_bonferroni",
            significance_level=0.05,
            estimand=EstimandType.PAIRED_DELTA,
            effect_size_measure="cohens_d",
            pareto_views=pareto_views,
            descriptive_fallback=DescriptiveFallbackPolicy.BELOW_MIN_POPULATION,
            minimum_inferential_population=10,
        )
        contract = StatisticalAnalysisContract(
            contract_id="contract-1",
            contract_version="1.0.0",
            independent_unit="task",
            comparison_method=ComparisonFamily.PAIRED_TASK_LEVEL,
            binary_test_policy="mcnemar",
            continuous_test_policy="paired_bootstrap",
            bootstrap_count=10000,
            bootstrap_confidence=0.95,
            bootstrap_seed=42,
            correction_family="holm_bonferroni",
            significance_level=0.05,
            estimand=EstimandType.PAIRED_DELTA,
            effect_size_measure="cohens_d",
            pareto_views=pareto_views,
            descriptive_fallback=DescriptiveFallbackPolicy.BELOW_MIN_POPULATION,
            minimum_inferential_population=10,
            content_hash=ch,
        )
        data = json.loads(contract.model_dump_json())
        restored = StatisticalAnalysisContract.model_validate(data)
        assert restored.content_hash == contract.content_hash
        assert restored.pareto_views == contract.pareto_views

    def test_contract_minimum_inferential_population_must_be_positive(self):
        """The minimum inferential population must be at least 1."""
        with pytest.raises(ValidationError, match="minimum_inferential_population"):
            StatisticalAnalysisContract(
                contract_id="contract-1",
                contract_version="1.0.0",
                independent_unit="task",
                comparison_method=ComparisonFamily.PAIRED_TASK_LEVEL,
                binary_test_policy="mcnemar",
                continuous_test_policy="paired_bootstrap",
                bootstrap_count=10000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                correction_family="holm_bonferroni",
                significance_level=0.05,
                estimand=EstimandType.PAIRED_DELTA,
                effect_size_measure="cohens_d",
                pareto_views=[],
                descriptive_fallback=DescriptiveFallbackPolicy.BELOW_MIN_POPULATION,
                minimum_inferential_population=0,
                content_hash=_VALID_HASH,
            )


class TestRoleCombinationTrack:
    def _make_combination(
        self,
        combo_id: str = "combo-primary-qwen3-8b-assistant-granite3.3-8b-lite-qwen3-0.6b",
        primary_variant_id: str = "qwen3-8b-q4_0",
        assistant_variant_id: str = "granite3.3-8b-q4_0",
        lite_variant_id: str = "qwen3-0.6b-q4_0",
    ) -> RoleCombination:
        return RoleCombination(
            combination_id=RoleCombinationId(
                combination_id=combo_id,
                primary_variant_id=primary_variant_id,
                assistant_variant_id=assistant_variant_id,
                lite_variant_id=lite_variant_id,
            ),
            rationale="Tractable combination for smoke validation",
        )

    def test_track_is_frozen_with_extra_forbid(self):
        """The track model rejects unknown fields and is frozen."""
        with pytest.raises(ValidationError, match="extra_forbidden"):
            RoleCombinationTrack(
                track_id="role-combo-track-1",
                track_version="1.0.0",
                description="Exact Primary/Assistant/Lite role-combination track",
                combinations=[self._make_combination()],
                role_exercise_declarations=[
                    RoleExerciseDeclaration(
                        task_id="task-1",
                        exercised_roles=["assistant", "primary"],
                    ),
                ],
                task_role_bindings=[
                    TaskRoleBinding(
                        task_id="task-1",
                        combination_id="combo-primary-qwen3-8b-assistant-granite3.3-8b-lite-qwen3-0.6b",
                        primary_variant_id="qwen3-8b-q4_0",
                        assistant_variant_id="granite3.3-8b-q4_0",
                        lite_variant_id="qwen3-0.6b-q4_0",
                    ),
                ],
                content_hash=_VALID_HASH,
                unknown_field="rejected",
            )

    def test_combination_ids_must_be_unique(self):
        """Each role combination must have a unique combination_id."""
        from g8e_evals.analysis_contract import compute_role_track_hash

        combo = self._make_combination()
        combos = [combo, combo.model_copy(update={})]
        ch = compute_role_track_hash(
            track_id="track-1",
            track_version="1.0.0",
            description="test",
            combinations=combos,
            role_exercise_declarations=[],
            task_role_bindings=[],
        )
        with pytest.raises(ValueError, match=r"duplicate.*combination_id"):
            RoleCombinationTrack(
                track_id="track-1",
                track_version="1.0.0",
                description="test",
                combinations=combos,
                role_exercise_declarations=[],
                task_role_bindings=[],
                content_hash=ch,
            )

    def test_combination_variant_ids_must_differ(self):
        """A role combination's three variant IDs must be distinct."""
        with pytest.raises(ValidationError, match=r"variant_id.*must be distinct"):
            RoleCombinationId(
                combination_id="combo-same",
                primary_variant_id="qwen3-8b-q4_0",
                assistant_variant_id="qwen3-8b-q4_0",
                lite_variant_id="qwen3-0.6b-q4_0",
            )

    def test_role_exercise_declaration_must_use_valid_tiers(self):
        """Role exercise declarations must use valid tier names."""
        with pytest.raises(ValidationError, match="exercised_roles"):
            RoleExerciseDeclaration(
                task_id="task-1",
                exercised_roles=["primary", "unknown_tier"],
            )

    def test_track_content_hash_must_match_computed(self):
        """The track content hash must match the computed hash."""
        from g8e_evals.analysis_contract import compute_role_track_hash

        combo = self._make_combination()
        declarations = [
            RoleExerciseDeclaration(
                task_id="task-1",
                exercised_roles=["assistant", "primary"],
            ),
        ]
        bindings = [
            TaskRoleBinding(
                task_id="task-1",
                combination_id=combo.combination_id.combination_id,
                primary_variant_id=combo.combination_id.primary_variant_id,
                assistant_variant_id=combo.combination_id.assistant_variant_id,
                lite_variant_id=combo.combination_id.lite_variant_id,
            ),
        ]
        ch = compute_role_track_hash(
            track_id="track-1",
            track_version="1.0.0",
            description="test track",
            combinations=[combo],
            role_exercise_declarations=declarations,
            task_role_bindings=bindings,
        )
        with pytest.raises(ValueError, match="content_hash mismatch"):
            RoleCombinationTrack(
                track_id="track-1",
                track_version="1.0.0",
                description="test track",
                combinations=[combo],
                role_exercise_declarations=declarations,
                task_role_bindings=bindings,
                content_hash="0" * 64,
            )

        track = RoleCombinationTrack(
            track_id="track-1",
            track_version="1.0.0",
            description="test track",
            combinations=[combo],
            role_exercise_declarations=declarations,
            task_role_bindings=bindings,
            content_hash=ch,
        )
        assert track.content_hash == ch

    def test_track_rejects_binding_for_unknown_combination(self):
        """A task role binding referencing an unknown combination_id is rejected."""
        from g8e_evals.analysis_contract import compute_role_track_hash

        combo = self._make_combination()
        bindings = [
            TaskRoleBinding(
                task_id="task-1",
                combination_id="nonexistent-combo",
                primary_variant_id="a",
                assistant_variant_id="b",
                lite_variant_id="c",
            ),
        ]
        ch = compute_role_track_hash(
            track_id="track-1",
            track_version="1.0.0",
            description="test",
            combinations=[combo],
            role_exercise_declarations=[],
            task_role_bindings=bindings,
        )
        with pytest.raises(ValueError, match=r"unknown.*combination_id"):
            RoleCombinationTrack(
                track_id="track-1",
                track_version="1.0.0",
                description="test",
                combinations=[combo],
                role_exercise_declarations=[],
                task_role_bindings=bindings,
                content_hash=ch,
            )

    def test_track_rejects_binding_variant_mismatch(self):
        """A task role binding with variant IDs not matching the combination is rejected."""
        from g8e_evals.analysis_contract import compute_role_track_hash

        combo = self._make_combination()
        bindings = [
            TaskRoleBinding(
                task_id="task-1",
                combination_id=combo.combination_id.combination_id,
                primary_variant_id="wrong-primary",
                assistant_variant_id=combo.combination_id.assistant_variant_id,
                lite_variant_id=combo.combination_id.lite_variant_id,
            ),
        ]
        ch = compute_role_track_hash(
            track_id="track-1",
            track_version="1.0.0",
            description="test",
            combinations=[combo],
            role_exercise_declarations=[],
            task_role_bindings=bindings,
        )
        with pytest.raises(ValueError, match=r"variant_id.*mismatch"):
            RoleCombinationTrack(
                track_id="track-1",
                track_version="1.0.0",
                description="test",
                combinations=[combo],
                role_exercise_declarations=[],
                task_role_bindings=bindings,
                content_hash=ch,
            )

    def test_track_round_trip_serialization(self):
        """The track round-trips through JSON without data loss."""
        import json

        from g8e_evals.analysis_contract import compute_role_track_hash

        combo = self._make_combination()
        declarations = [
            RoleExerciseDeclaration(
                task_id="task-1",
                exercised_roles=["assistant", "lite", "primary"],
            ),
            RoleExerciseDeclaration(
                task_id="task-2",
                exercised_roles=["primary"],
            ),
        ]
        bindings = [
            TaskRoleBinding(
                task_id="task-1",
                combination_id=combo.combination_id.combination_id,
                primary_variant_id=combo.combination_id.primary_variant_id,
                assistant_variant_id=combo.combination_id.assistant_variant_id,
                lite_variant_id=combo.combination_id.lite_variant_id,
            ),
        ]
        ch = compute_role_track_hash(
            track_id="track-1",
            track_version="1.0.0",
            description="test track",
            combinations=[combo],
            role_exercise_declarations=declarations,
            task_role_bindings=bindings,
        )
        track = RoleCombinationTrack(
            track_id="track-1",
            track_version="1.0.0",
            description="test track",
            combinations=[combo],
            role_exercise_declarations=declarations,
            task_role_bindings=bindings,
            content_hash=ch,
        )
        data = json.loads(track.model_dump_json())
        restored = RoleCombinationTrack.model_validate(data)
        assert restored.content_hash == track.content_hash
        assert len(restored.combinations) == 1
        assert len(restored.role_exercise_declarations) == 2
        assert len(restored.task_role_bindings) == 1

    def test_track_exercise_declaration_must_have_at_least_one_role(self):
        """A role exercise declaration must declare at least one role."""
        with pytest.raises(ValidationError, match="exercised_roles"):
            RoleExerciseDeclaration(
                task_id="task-1",
                exercised_roles=[],
            )

    def test_track_combination_rationale_required(self):
        """Each combination must have a non-empty rationale."""
        with pytest.raises(ValidationError, match="rationale"):
            RoleCombination(
                combination_id=RoleCombinationId(
                    combination_id="combo-1",
                    primary_variant_id="a",
                    assistant_variant_id="b",
                    lite_variant_id="c",
                ),
                rationale="",
            )
