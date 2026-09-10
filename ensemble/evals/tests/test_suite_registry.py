# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the typed suite registry and execution-path classification.

Verifies that the suite registry covers every suite, classifies execution
paths correctly, rejects deterministic-simulation suites from the
model-comparison set, rejects real-model suites from the bench-synthetic
command, and that CLI --suite choices match the registry.
"""

from __future__ import annotations

import pytest

from g8e_evals.arms import ALL_ARMS, Arm, GOVERNED_ARMS
from g8e_evals.suites import (
    SUITE_REGISTRY,
    SuiteExecutionClass,
    assert_model_comparison_eligible,
    assert_simulation_eligible,
    get_model_comparison_suites,
    get_simulation_suites,
    get_suite,
    get_suite_ids_by_class,
    register_suite,
)

pytestmark = pytest.mark.unit

_EXPECTED_SUITE_IDS = frozenset({
    "ifeval_subset",
    "privacy_token_lifecycle",
    "privacy_boundary_leakage",
    "governance_adversarial",
    "policy_attack",
    "benign_overblock",
    "tool_sequence",
    "factual_qa",
    "citation_backed",
    "partial_milestone",
    "final_state",
    "ledger_consistency",
    "reliability",
    "economics_performance",
})

_EXPECTED_MODEL_COMPARISON = frozenset({"ifeval_subset"})

_EXPECTED_SIMULATION = frozenset({
    "privacy_token_lifecycle",
    "privacy_boundary_leakage",
    "governance_adversarial",
    "policy_attack",
    "benign_overblock",
    "tool_sequence",
    "factual_qa",
    "citation_backed",
    "partial_milestone",
    "final_state",
    "ledger_consistency",
    "reliability",
    "economics_performance",
})


class TestSuiteRegistryCompleteness:
    def test_registry_covers_all_expected_suite_ids(self):
        assert set(SUITE_REGISTRY.keys()) == _EXPECTED_SUITE_IDS

    def test_registry_has_no_extra_suites(self):
        for suite_id in SUITE_REGISTRY:
            assert suite_id in _EXPECTED_SUITE_IDS, f"unexpected suite: {suite_id}"

    def test_every_suite_spec_has_nonempty_suite_id(self):
        for spec in SUITE_REGISTRY.values():
            assert spec.suite_id, "suite_id must not be empty"
            assert isinstance(spec.suite_id, str)

    def test_every_suite_spec_has_nonempty_description(self):
        for spec in SUITE_REGISTRY.values():
            assert spec.description, f"suite '{spec.suite_id}': description must not be empty"

    def test_every_suite_spec_has_valid_execution_class(self):
        for spec in SUITE_REGISTRY.values():
            assert isinstance(spec.execution_class, SuiteExecutionClass), (
                f"suite '{spec.suite_id}': execution_class is not a SuiteExecutionClass"
            )

    def test_every_suite_spec_has_compatible_arms(self):
        for spec in SUITE_REGISTRY.values():
            assert len(spec.compatible_arms) > 0, (
                f"suite '{spec.suite_id}': compatible_arms must not be empty"
            )
            for arm in spec.compatible_arms:
                assert isinstance(arm, Arm), (
                    f"suite '{spec.suite_id}': compatible_arms contains non-Arm: {arm}"
                )

    def test_every_suite_spec_has_loader_factory(self):
        for spec in SUITE_REGISTRY.values():
            assert callable(spec.loader_factory), (
                f"suite '{spec.suite_id}': loader_factory is not callable"
            )

    def test_every_suite_spec_has_provenance_loader(self):
        for spec in SUITE_REGISTRY.values():
            assert callable(spec.provenance_loader), (
                f"suite '{spec.suite_id}': provenance_loader is not callable"
            )

    def test_get_suite_returns_registered_spec(self):
        for suite_id in _EXPECTED_SUITE_IDS:
            spec = get_suite(suite_id)
            assert spec.suite_id == suite_id

    def test_get_suite_raises_for_unknown_suite(self):
        with pytest.raises(KeyError):
            get_suite("nonexistent_suite")


class TestExecutionClassClassification:
    def test_model_comparison_suites_match_expected(self):
        actual = frozenset(s.suite_id for s in get_model_comparison_suites())
        assert actual == _EXPECTED_MODEL_COMPARISON

    def test_simulation_suites_match_expected(self):
        actual = frozenset(s.suite_id for s in get_simulation_suites())
        assert actual == _EXPECTED_SIMULATION

    def test_model_comparison_and_simulation_are_disjoint(self):
        mc = frozenset(s.suite_id for s in get_model_comparison_suites())
        sim = frozenset(s.suite_id for s in get_simulation_suites())
        assert mc.isdisjoint(sim)

    def test_every_suite_is_either_model_comparison_or_simulation(self):
        mc = frozenset(s.suite_id for s in get_model_comparison_suites())
        sim = frozenset(s.suite_id for s in get_simulation_suites())
        assert mc | sim == set(SUITE_REGISTRY.keys())

    def test_get_suite_ids_by_class_returns_sorted(self):
        ids = get_suite_ids_by_class(SuiteExecutionClass.DETERMINISTIC_SIMULATION)
        assert ids == sorted(ids)

    def test_ifeval_subset_is_real_model(self):
        spec = get_suite("ifeval_subset")
        assert spec.execution_class is SuiteExecutionClass.REAL_MODEL

    def test_ifeval_subset_is_model_comparison(self):
        spec = get_suite("ifeval_subset")
        assert spec.is_model_comparison

    def test_synthetic_suites_are_not_model_comparison(self):
        for spec in get_simulation_suites():
            assert not spec.is_model_comparison, (
                f"suite '{spec.suite_id}' should not be model comparison"
            )


class TestModelComparisonEligibilityGate:
    def test_accepts_real_model_suite(self):
        spec = assert_model_comparison_eligible("ifeval_subset")
        assert spec.suite_id == "ifeval_subset"

    def test_rejects_deterministic_simulation_suite(self):
        for suite_id in _EXPECTED_SIMULATION:
            with pytest.raises(ValueError, match="cannot be used for model comparisons"):
                assert_model_comparison_eligible(suite_id)

    def test_rejects_unknown_suite(self):
        with pytest.raises(ValueError, match="unknown suite"):
            assert_model_comparison_eligible("nonexistent_suite")

    def test_rejects_privacy_token_lifecycle_from_model_comparison(self):
        with pytest.raises(ValueError, match="deterministic_simulation"):
            assert_model_comparison_eligible("privacy_token_lifecycle")

    def test_rejects_governance_adversarial_from_model_comparison(self):
        with pytest.raises(ValueError, match="deterministic_simulation"):
            assert_model_comparison_eligible("governance_adversarial")


class TestSimulationEligibilityGate:
    def test_accepts_deterministic_simulation_suite(self):
        for suite_id in _EXPECTED_SIMULATION:
            spec = assert_simulation_eligible(suite_id)
            assert spec.suite_id == suite_id

    def test_rejects_real_model_suite(self):
        with pytest.raises(ValueError, match="cannot be used with bench-synthetic"):
            assert_simulation_eligible("ifeval_subset")

    def test_rejects_unknown_suite(self):
        with pytest.raises(ValueError, match="unknown suite"):
            assert_simulation_eligible("nonexistent_suite")


class TestDuplicateRegistrationRejected:
    def test_register_suite_rejects_duplicate(self):
        spec = SUITE_REGISTRY["ifeval_subset"]
        with pytest.raises(ValueError, match="duplicate suite registration"):
            register_suite(spec)


class TestCliSuiteChoicesMatchRegistry:
    def test_run_command_suite_choices_match_model_comparison(self):
        from g8e_evals.cli import _MODEL_COMPARISON_SUITE_CHOICES
        assert set(_MODEL_COMPARISON_SUITE_CHOICES) == _EXPECTED_MODEL_COMPARISON

    def test_campaign_command_suite_choices_match_model_comparison(self):
        from g8e_evals.cli import _MODEL_COMPARISON_SUITE_CHOICES
        assert set(_MODEL_COMPARISON_SUITE_CHOICES) == _EXPECTED_MODEL_COMPARISON

    def test_bench_synthetic_suite_choices_match_simulation(self):
        from g8e_evals.cli import _SIMULATION_SUITE_CHOICES
        assert set(_SIMULATION_SUITE_CHOICES) == _EXPECTED_SIMULATION

    def test_synthetic_suite_choices_alias_matches_simulation(self):
        from g8e_evals.cli import _SYNTHETIC_SUITE_CHOICES
        assert set(_SYNTHETIC_SUITE_CHOICES) == _EXPECTED_SIMULATION

    def test_model_comparison_and_synthetic_choices_are_disjoint(self):
        from g8e_evals.cli import _MODEL_COMPARISON_SUITE_CHOICES, _SYNTHETIC_SUITE_CHOICES
        assert set(_MODEL_COMPARISON_SUITE_CHOICES).isdisjoint(set(_SYNTHETIC_SUITE_CHOICES))


class TestGraderFactoryContract:
    def test_real_model_suite_has_grader_factory(self):
        for spec in get_model_comparison_suites():
            assert spec.grader_factory is not None, (
                f"suite '{spec.suite_id}': model-comparison suite must have a grader_factory"
            )
            assert callable(spec.grader_factory)

    def test_simulation_suites_have_no_grader_factory(self):
        for spec in get_simulation_suites():
            assert spec.grader_factory is None, (
                f"suite '{spec.suite_id}': simulation suite must not have a grader_factory"
            )

    def test_ifeval_subset_grader_factory_returns_verifier(self):
        spec = get_suite("ifeval_subset")
        assert spec.grader_factory is not None
        grader = spec.grader_factory()
        assert grader is not None
        assert hasattr(grader, "verify")


class TestRequiredObserversContract:
    def test_real_model_suite_has_no_required_observers(self):
        for spec in get_model_comparison_suites():
            assert spec.required_observers == (), (
                f"suite '{spec.suite_id}': real-model suite should not require observers"
            )

    def test_simulation_suites_have_observer_or_cross_cutting(self):
        cross_cutting = {"governance_adversarial", "benign_overblock"}
        for spec in get_simulation_suites():
            if spec.suite_id in cross_cutting:
                continue
            assert len(spec.required_observers) > 0, (
                f"suite '{spec.suite_id}': simulation suite should have at least one observer"
            )


class TestCompatibleArmsContract:
    def test_ifeval_subset_supports_all_arms(self):
        spec = get_suite("ifeval_subset")
        assert set(spec.compatible_arms) == set(ALL_ARMS)

    def test_simulation_suites_support_direct_and_governed(self):
        expected = frozenset({Arm.DIRECT, *GOVERNED_ARMS})
        for spec in get_simulation_suites():
            assert set(spec.compatible_arms) == expected, (
                f"suite '{spec.suite_id}': compatible_arms mismatch"
            )
