# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 conformance tests for the typed grader inventory.

Verifies that the inventory covers every registered deterministic
grader, every metric with a grader reference, every CLI wiring
constant, and every benchmark suite producer path. The inventory is
the single source of truth for grader identity and conformance-case
coverage status.
"""

from __future__ import annotations

import pytest

from g8e_evals.grader_inventory import (
    GRADER_INVENTORY,
    ConformanceCaseCategory,
    InventoryDriftError,
    ProducerPath,
    validate_inventory_against_registries,
)
from g8e_evals.graders import _GRADERS
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY
from g8e_evals.schema import GraderClass

pytestmark = pytest.mark.unit


def test_inventory_covers_every_registered_grader():
    for key in _GRADERS:
        assert key in GRADER_INVENTORY, f"grader {key} missing from inventory"


def test_inventory_partial_external_graders_are_not_in_grader_registry():
    for key, entry in GRADER_INVENTORY.items():
        if entry.producer_path == ProducerPath.PARTIAL_EXTERNAL:
            assert key not in _GRADERS, (
                f"partial_external grader {key} should not be in _GRADERS"
            )


def test_inventory_non_partial_external_graders_are_in_grader_registry():
    for key, entry in GRADER_INVENTORY.items():
        if entry.producer_path != ProducerPath.PARTIAL_EXTERNAL:
            assert key in _GRADERS, (
                f"non-partial_external grader {key} should be in _GRADERS"
            )


def test_inventory_has_34_entries():
    assert len(GRADER_INVENTORY) == 34


def test_inventory_entries_are_deterministic_class():
    for entry in GRADER_INVENTORY.values():
        assert entry.grader_class == GraderClass.DETERMINISTIC


def test_inventory_entries_have_non_empty_metric_ids():
    for entry in GRADER_INVENTORY.values():
        assert len(entry.metric_ids) >= 1
        for metric_id in entry.metric_ids:
            assert metric_id
            assert metric_id.strip()


def test_inventory_metric_ids_are_registered():
    for entry in GRADER_INVENTORY.values():
        for metric_id in entry.metric_ids:
            assert DEFAULT_METRIC_REGISTRY.is_registered(metric_id, entry.grader_version), (
                f"grader {entry.grader_id}: metric {metric_id}@{entry.grader_version} not registered"
            )


def test_inventory_every_metric_with_grader_ref_has_inventory_entry():
    for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
        ref = definition.grader_ref
        if ref is None:
            continue
        if ref.grader_class != GraderClass.DETERMINISTIC:
            continue
        assert (ref.grader_id, ref.grader_version) in GRADER_INVENTORY, (
            f"metric {definition.metric_id} references grader {ref.grader_id}@{ref.grader_version} "
            f"but no inventory entry exists"
        )


def test_inventory_evidence_requirements_match_metric_definitions():
    for entry in GRADER_INVENTORY.values():
        for metric_id in entry.metric_ids:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, entry.grader_version)
            assert set(definition.evidence_requirements) == set(entry.evidence_requirements), (
                f"grader {entry.grader_id}: evidence requirements {entry.evidence_requirements} "
                f"do not match metric {metric_id} definition {definition.evidence_requirements}"
            )


def test_inventory_denominators_match_metric_definitions():
    for entry in GRADER_INVENTORY.values():
        for metric_id in entry.metric_ids:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, entry.grader_version)
            assert entry.denominator == definition.denominator, (
                f"grader {entry.grader_id}: denominator does not match metric {metric_id}"
            )


def test_inventory_cli_constants_exist_in_cli_module():
    import g8e_evals.cli as cli_module

    for entry in GRADER_INVENTORY.values():
        assert hasattr(cli_module, entry.cli_constant), (
            f"grader {entry.grader_id}: CLI constant {entry.cli_constant} not found in cli module"
        )
        constant_value = getattr(cli_module, entry.cli_constant)
        assert constant_value == entry.grader_id, (
            f"grader {entry.grader_id}: CLI constant {entry.cli_constant} = {constant_value!r} "
            f"does not match grader_id"
        )


def test_inventory_assertion_fields_are_valid_task_definition_fields():
    from g8e_evals.schema import TaskDefinition

    valid_fields = set(TaskDefinition.model_fields.keys())
    for entry in GRADER_INVENTORY.values():
        if entry.assertion_field is None:
            continue
        assert entry.assertion_field in valid_fields, (
            f"grader {entry.grader_id}: assertion_field {entry.assertion_field!r} "
            f"is not a TaskDefinition field"
        )


def test_inventory_observation_fields_are_valid_context_fields():
    from g8e_evals.graders import DeterministicGradingContext

    valid_fields = set(DeterministicGradingContext.__dataclass_fields__.keys())
    for entry in GRADER_INVENTORY.values():
        if entry.observation_field is None:
            continue
        assert entry.observation_field in valid_fields, (
            f"grader {entry.grader_id}: observation_field {entry.observation_field!r} "
            f"is not a DeterministicGradingContext field"
        )


def test_inventory_cross_cutting_graders_have_null_assertion_and_observation():
    for entry in GRADER_INVENTORY.values():
        if entry.producer_path == ProducerPath.CROSS_CUTTING:
            assert entry.assertion_field is None, (
                f"cross_cutting grader {entry.grader_id} should have null assertion_field"
            )
            assert entry.observation_field is None, (
                f"cross_cutting grader {entry.grader_id} should have null observation_field"
            )


def test_inventory_partial_external_graders_have_typed_exclusions():
    for entry in GRADER_INVENTORY.values():
        if entry.producer_path == ProducerPath.PARTIAL_EXTERNAL:
            assert len(entry.conformance_exclusions) > 0, (
                f"partial_external grader {entry.grader_id} must have typed exclusions"
            )


def test_inventory_exclusion_categories_are_valid():
    valid_categories = set(ConformanceCaseCategory)
    for entry in GRADER_INVENTORY.values():
        for exclusion in entry.conformance_exclusions:
            assert exclusion.category in valid_categories
            assert exclusion.reason
            assert exclusion.reason.strip()


def test_inventory_exclusions_are_unique_per_grader():
    for entry in GRADER_INVENTORY.values():
        categories = [excl.category for excl in entry.conformance_exclusions]
        assert len(categories) == len(set(categories)), (
            f"grader {entry.grader_id}: duplicate exclusion categories"
        )


def test_inventory_required_categories_are_non_empty_for_every_grader():
    for entry in GRADER_INVENTORY.values():
        required = entry.required_categories()
        assert len(required) > 0, (
            f"grader {entry.grader_id}: must have at least one required conformance category"
        )


def test_inventory_real_provider_graders_are_marked():
    real_provider_graders = [
        entry.grader_id for entry in GRADER_INVENTORY.values()
        if entry.producer_path == ProducerPath.REAL_PROVIDER
    ]
    assert "model_boundary_raw_secret_rate" in real_provider_graders


def test_inventory_producer_suite_ids_reference_real_suites():
    from g8e_evals.benchmarks.economics.loader import EconomicsPerformanceLoader
    from g8e_evals.benchmarks.governance.benign_overblock_loader import BenignOverblockLoader
    from g8e_evals.benchmarks.governance.loader import GovernanceAdversarialLoader
    from g8e_evals.benchmarks.governance.policy_attack_loader import PolicyAttackLoader
    from g8e_evals.benchmarks.ifeval.loader import IFEvalLoader
    from g8e_evals.benchmarks.privacy.loader import (
        PrivacyBoundaryLeakageLoader,
        PrivacyTokenLifecycleLoader,
    )
    from g8e_evals.benchmarks.reliability.loader import ReliabilityLoader
    from g8e_evals.benchmarks.utility.citation_backed_loader import CitationBackedLoader
    from g8e_evals.benchmarks.utility.factual_qa_loader import FactualQALoader
    from g8e_evals.benchmarks.utility.final_state_loader import FinalStateLoader
    from g8e_evals.benchmarks.utility.ledger_consistency_loader import LedgerConsistencyLoader
    from g8e_evals.benchmarks.utility.loader import ToolSequenceLoader
    from g8e_evals.benchmarks.utility.partial_milestone_loader import PartialMilestoneLoader

    suite_loaders = {
        EconomicsPerformanceLoader.SUITE_ID: EconomicsPerformanceLoader,
        BenignOverblockLoader.SUITE_ID: BenignOverblockLoader,
        GovernanceAdversarialLoader.SUITE_ID: GovernanceAdversarialLoader,
        PolicyAttackLoader.SUITE_ID: PolicyAttackLoader,
        IFEvalLoader.SUITE_ID: IFEvalLoader,
        PrivacyBoundaryLeakageLoader.SUITE_ID: PrivacyBoundaryLeakageLoader,
        PrivacyTokenLifecycleLoader.SUITE_ID: PrivacyTokenLifecycleLoader,
        ReliabilityLoader.SUITE_ID: ReliabilityLoader,
        CitationBackedLoader.SUITE_ID: CitationBackedLoader,
        FactualQALoader.SUITE_ID: FactualQALoader,
        FinalStateLoader.SUITE_ID: FinalStateLoader,
        LedgerConsistencyLoader.SUITE_ID: LedgerConsistencyLoader,
        ToolSequenceLoader.SUITE_ID: ToolSequenceLoader,
        PartialMilestoneLoader.SUITE_ID: PartialMilestoneLoader,
    }

    for entry in GRADER_INVENTORY.values():
        for suite_id in entry.producer_suite_ids:
            assert suite_id in suite_loaders, (
                f"grader {entry.grader_id}: producer suite {suite_id!r} has no loader"
            )


def test_validate_inventory_against_registries_passes():
    validate_inventory_against_registries()


def test_validate_inventory_detects_missing_grader_entry(monkeypatch):
    fake_registry = {**_GRADERS, ("phantom_grader", "1.0.0"): object()}
    monkeypatch.setattr("g8e_evals.grader_inventory._GRADERS", fake_registry)
    with pytest.raises(InventoryDriftError, match="phantom_grader"):
        validate_inventory_against_registries()


def test_validate_inventory_detects_unregistered_metric(monkeypatch):
    entry = GRADER_INVENTORY[("receipt_integrity", "1.0.0")]
    fake_entry = entry.model_copy(update={"metric_ids": ["nonexistent_metric"]})
    fake_inventory = {**GRADER_INVENTORY, entry.key: fake_entry}
    monkeypatch.setattr("g8e_evals.grader_inventory.GRADER_INVENTORY", fake_inventory)
    with pytest.raises(InventoryDriftError, match="nonexistent_metric"):
        validate_inventory_against_registries()


def test_inventory_entries_are_frozen():
    entry = GRADER_INVENTORY[("receipt_integrity", "1.0.0")]
    with pytest.raises(Exception, match=r"frozen|immutable|ValidationError"):
        entry.grader_id = "changed"


def test_inventory_has_exclusion_check():
    entry = GRADER_INVENTORY[("receipt_integrity", "1.0.0")]
    assert entry.has_exclusion(ConformanceCaseCategory.NOT_APPLICABLE)
    assert not entry.has_exclusion(ConformanceCaseCategory.PASSING_EVIDENCE)


def test_inventory_excluded_categories():
    entry = GRADER_INVENTORY[("receipt_integrity", "1.0.0")]
    excluded = entry.excluded_categories()
    assert ConformanceCaseCategory.NOT_APPLICABLE in excluded
    assert ConformanceCaseCategory.PASSING_EVIDENCE not in excluded


def test_inventory_required_categories():
    entry = GRADER_INVENTORY[("receipt_integrity", "1.0.0")]
    required = entry.required_categories()
    assert ConformanceCaseCategory.PASSING_EVIDENCE in required
    assert ConformanceCaseCategory.NOT_APPLICABLE not in required
