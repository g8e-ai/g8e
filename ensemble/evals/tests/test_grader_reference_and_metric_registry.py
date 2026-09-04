# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 conformance tests for typed grader references and the metric registry.

Verifies that GraderReference binds grader ID, version, and class together,
that TaskDefinition rejects duplicate grader references, that the MetricRegistry
rejects unregistered metrics, unit mismatches, and grader-class mismatches,
and that the default registry covers every metric produced by the current
evaluation pipeline.
"""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.arms import Arm
from g8e_evals.metrics import (
    DEFAULT_METRIC_REGISTRY,
    DuplicateMetricError,
    MetricDefinition,
    MetricDirection,
    MetricGraderClassMismatchError,
    MetricRegistry,
    MetricUnitMismatchError,
    MissingValuePolicy,
    UnregisteredMetricError,
)
from g8e_evals.schema import (
    GraderClass,
    GraderReference,
    MetricObservation,
    TaskDefinition,
    VerificationStatus,
)

pytestmark = pytest.mark.unit


# ---------------------------------------------------------------------------
# GraderReference
# ---------------------------------------------------------------------------


def test_grader_reference_binds_id_version_and_class():
    ref = GraderReference(
        grader_id="receipt_integrity",
        grader_version="1.0.0",
        grader_class=GraderClass.DETERMINISTIC,
    )
    assert ref.grader_id == "receipt_integrity"
    assert ref.grader_version == "1.0.0"
    assert ref.grader_class == GraderClass.DETERMINISTIC


def test_grader_reference_defaults_to_deterministic_class():
    ref = GraderReference(grader_id="canary_scrubbing", grader_version="1.0.0")
    assert ref.grader_class == GraderClass.DETERMINISTIC


def test_grader_reference_rejects_empty_id():
    with pytest.raises(ValidationError):
        GraderReference(grader_id="", grader_version="1.0.0")


def test_grader_reference_rejects_empty_version():
    with pytest.raises(ValidationError):
        GraderReference(grader_id="receipt_integrity", grader_version="")


def test_grader_reference_rejects_extra_fields():
    with pytest.raises(ValidationError):
        GraderReference.model_validate({
            "grader_id": "receipt_integrity",
            "grader_version": "1.0.0",
            "extra_field": "forbidden",
        })


# ---------------------------------------------------------------------------
# TaskDefinition grader references
# ---------------------------------------------------------------------------


def _minimal_task(graders: list[GraderReference]) -> TaskDefinition:
    return TaskDefinition(
        task_id="task-1",
        suite_id="suite",
        suite_version="1.0.0",
        prompt_hash="a" * 64,
        graders=graders,
    )


def test_task_definition_accepts_typed_grader_references():
    task = _minimal_task([
        GraderReference(grader_id="receipt_integrity", grader_version="1.0.0"),
        GraderReference(grader_id="protocol_chain", grader_version="1.0.0"),
    ])
    assert len(task.graders) == 2
    assert task.graders[0].grader_id == "receipt_integrity"
    assert task.graders[1].grader_id == "protocol_chain"


def test_task_definition_rejects_duplicate_grader_references():
    with pytest.raises(ValidationError, match="grader references must be unique"):
        _minimal_task([
            GraderReference(grader_id="receipt_integrity", grader_version="1.0.0"),
            GraderReference(grader_id="receipt_integrity", grader_version="1.0.0"),
        ])


def test_task_definition_accepts_different_versions_of_same_grader():
    task = _minimal_task([
        GraderReference(grader_id="receipt_integrity", grader_version="1.0.0"),
        GraderReference(grader_id="receipt_integrity", grader_version="2.0.0"),
    ])
    assert len(task.graders) == 2


def test_task_definition_round_trips_grader_references_through_json():
    task = _minimal_task([
        GraderReference(grader_id="receipt_integrity", grader_version="1.0.0"),
    ])
    restored = TaskDefinition.model_validate_json(task.model_dump_json())
    assert restored.graders == task.graders


# ---------------------------------------------------------------------------
# MetricDefinition
# ---------------------------------------------------------------------------


def _minimal_definition(**overrides) -> MetricDefinition:
    defaults: dict[str, object] = {
        "metric_id": "test_metric",
        "metric_version": "1.0.0",
        "definition": "A test metric for conformance testing.",
        "unit": "boolean",
        "direction": MetricDirection.BINARY_PASS_FAIL,
        "grader_class": GraderClass.DETERMINISTIC,
        "eligible_population": "All test attempts.",
        "denominator": "Total test attempts.",
        "missing_value_policy": MissingValuePolicy.EXCLUDE,
        "aggregation": "boolean_fraction",
        "uncertainty_method": "Bootstrap interval.",
        "evidence_requirements": ["test_evidence"],
    }
    defaults.update(overrides)
    return MetricDefinition(**defaults)  # type: ignore[arg-type]


def test_metric_definition_is_frozen():
    definition = _minimal_definition()
    with pytest.raises(ValidationError):
        definition.metric_id = "changed"


def test_metric_definition_rejects_extra_fields():
    with pytest.raises(ValidationError):
        MetricDefinition.model_validate({
            "metric_id": "test_metric",
            "metric_version": "1.0.0",
            "definition": "A test metric.",
            "unit": "boolean",
            "direction": "binary_pass_fail",
            "grader_class": "deterministic",
            "eligible_population": "All attempts.",
            "denominator": "Total attempts.",
            "missing_value_policy": "exclude",
            "aggregation": "boolean_fraction",
            "uncertainty_method": "Bootstrap.",
            "evidence_requirements": ["evidence"],
            "extra_field": "forbidden",
        })


def test_metric_definition_requires_evidence_requirements():
    with pytest.raises(ValidationError):
        _minimal_definition(evidence_requirements=[])


# ---------------------------------------------------------------------------
# MetricRegistry
# ---------------------------------------------------------------------------


def test_registry_rejects_unregistered_metric():
    registry = MetricRegistry([])
    with pytest.raises(UnregisteredMetricError, match="unregistered metric"):
        registry.get("nonexistent", "1.0.0")


def test_registry_rejects_duplicate_registration():
    definition = _minimal_definition()
    registry = MetricRegistry([definition])
    with pytest.raises(DuplicateMetricError, match="already registered"):
        registry.register(definition)


def test_registry_is_registered_returns_false_for_unknown():
    registry = MetricRegistry([])
    assert not registry.is_registered("unknown", "1.0.0")


def test_registry_validate_rejects_unit_mismatch():
    definition = _minimal_definition(unit="boolean")
    registry = MetricRegistry([definition])
    observation = MetricObservation(
        metric_id="test_metric",
        metric_version="1.0.0",
        attempt_id="a1",
        run_id="r1",
        arm_id=Arm.DOCTRINE,
        task_id="t1",
        value=1.0,
        unit="proportion",
        verification_status=VerificationStatus.VERIFIED,
        grader_class=GraderClass.DETERMINISTIC,
    )
    with pytest.raises(MetricUnitMismatchError, match="unit mismatch"):
        registry.validate(observation)


def test_registry_validate_rejects_grader_class_mismatch():
    definition = _minimal_definition(grader_class=GraderClass.DETERMINISTIC)
    registry = MetricRegistry([definition])
    observation = MetricObservation(
        metric_id="test_metric",
        metric_version="1.0.0",
        attempt_id="a1",
        run_id="r1",
        arm_id=Arm.DOCTRINE,
        task_id="t1",
        value=1.0,
        unit="boolean",
        verification_status=VerificationStatus.VERIFIED,
        grader_class=GraderClass.LLM_JUDGE,
    )
    with pytest.raises(MetricGraderClassMismatchError, match="grader class mismatch"):
        registry.validate(observation)


def test_registry_validate_accepts_matching_observation():
    definition = _minimal_definition()
    registry = MetricRegistry([definition])
    observation = MetricObservation(
        metric_id="test_metric",
        metric_version="1.0.0",
        attempt_id="a1",
        run_id="r1",
        arm_id=Arm.DOCTRINE,
        task_id="t1",
        value=1.0,
        unit="boolean",
        verification_status=VerificationStatus.VERIFIED,
        grader_class=GraderClass.DETERMINISTIC,
    )
    registry.validate(observation)


# ---------------------------------------------------------------------------
# DEFAULT_METRIC_REGISTRY coverage
# ---------------------------------------------------------------------------


_CURRENT_METRIC_IDS = [
    "ifeval_subset_verifier",
    "eval_judge",
    "receipt_integrity",
    "protocol_chain",
    "canary_scrubbing",
    "model_boundary_raw_secret_rate",
    "exact_local_rehydration",
    "secret_detection_precision",
    "secret_detection_recall",
    "final_state_accuracy",
    "independent_state_accuracy",
    "policy_outcome",
    "stage_usage_reconciled",
    "unauthorized_mutation",
    "token_store_persistence",
    "token_ttl_expiry",
]


@pytest.mark.parametrize("metric_id", _CURRENT_METRIC_IDS)
def test_default_registry_covers_current_metric(metric_id):
    assert DEFAULT_METRIC_REGISTRY.is_registered(metric_id, "1.0.0")


def test_default_registry_has_no_duplicate_definitions():
    definitions = DEFAULT_METRIC_REGISTRY.all_definitions()
    keys = [(d.metric_id, d.metric_version) for d in definitions]
    assert len(keys) == len(set(keys))


def test_default_registry_definitions_have_complete_contracts():
    for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
        assert definition.metric_id
        assert definition.metric_version
        assert definition.definition
        assert definition.unit
        assert definition.direction
        assert definition.grader_class
        assert definition.eligible_population
        assert definition.denominator
        assert definition.missing_value_policy
        assert definition.aggregation
        assert definition.uncertainty_method
        assert len(definition.evidence_requirements) >= 1


def test_default_registry_eval_judge_uses_llm_judge_class():
    definition = DEFAULT_METRIC_REGISTRY.get("eval_judge", "1.0.0")
    assert definition.grader_class == GraderClass.LLM_JUDGE


def test_default_registry_deterministic_metrics_use_deterministic_class():
    for metric_id in _CURRENT_METRIC_IDS:
        if metric_id == "eval_judge":
            continue
        definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
        assert definition.grader_class == GraderClass.DETERMINISTIC, (
            f"metric {metric_id} should be deterministic"
        )


def test_default_registry_model_boundary_is_lower_is_better():
    definition = DEFAULT_METRIC_REGISTRY.get("model_boundary_raw_secret_rate", "1.0.0")
    assert definition.direction == MetricDirection.LOWER_IS_BETTER


def test_default_registry_stage_usage_reconciled_has_no_grader_ref():
    definition = DEFAULT_METRIC_REGISTRY.get("stage_usage_reconciled", "1.0.0")
    assert definition.grader_ref is None
