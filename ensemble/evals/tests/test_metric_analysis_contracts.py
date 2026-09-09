# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import pytest

from g8e_evals.analysis.engine import compute_canonical_analysis
from g8e_evals.arms import Arm
from g8e_evals.metrics import (
    DEFAULT_METRIC_REGISTRY,
    AggregationMethod,
    DenominatorKind,
    EligibilityKind,
    MetricApplicabilityContract,
    MetricDefinition,
    MetricDirection,
    ThresholdOperator,
)
from g8e_evals.release_metric_set import RELEASE_METRIC_SET, ReleaseMetricEntry
from g8e_evals.schema import (
    AttemptRecord,
    CanaryScrubbingAssertion,
    FactualQAAssertion,
    FactualQAMatchType,
    GraderReference,
    L3ProofTransplantAssertion,
    MetricObservation,
    RejectionLayer,
    SecretDetectionAssertion,
    StateCollectionBoundary,
    StateEvidenceKind,
    StateFixtureDefinition,
    StateValue,
    TaskDefinition,
    TerminalStatus,
    UnauthorizedMutationAssertion,
)


pytestmark = pytest.mark.unit

_RUN_ID = "run-contract"


def _canary_assertion(assertion_id: str) -> CanaryScrubbingAssertion:
    return CanaryScrubbingAssertion(
        assertion_id=assertion_id,
        canary_sha256="a" * 64,
        source="prompt",
        input_artifact_sha256="b" * 64,
        expected_output_artifact_sha256="c" * 64,
        expected_scrub_type="api_key",
        expected_occurrences=1,
    )


def test_every_metric_has_typed_applicability_and_denominator_contract() -> None:
    for definition in DEFAULT_METRIC_REGISTRY.all_definitions():
        assert isinstance(definition.applicability.eligibility, EligibilityKind)
        assert isinstance(definition.applicability.denominator, DenominatorKind)


def test_missing_multi_assertion_metric_uses_declared_denominator() -> None:
    task = TaskDefinition(
        task_id="task-canary",
        suite_id="privacy",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        compatible_arms=[Arm.DOCTRINE],
        sensitive_canary_annotations=[_canary_assertion("canary-1"), _canary_assertion("canary-2")],
        graders=[GraderReference(grader_id="canary_scrubbing", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-canary",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.MODEL_FAILED,
    )

    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )

    result = next(result for result in analysis.metric_results if result.metric_id == "canary_scrubbing")
    assert result.eligible_count == 1
    assert result.missing_count == 1
    assert result.denominator == 2


def test_release_blocker_threshold_is_typed() -> None:
    definition = DEFAULT_METRIC_REGISTRY.get("model_boundary_raw_secret_rate", "1.0.0")
    assert definition.practical_threshold is not None
    assert definition.practical_threshold.operator == ThresholdOperator.LESS_THAN_OR_EQUAL
    assert definition.practical_threshold.value == 0.0
    assert definition.practical_threshold.release_blocker is True


def test_uncalibrated_metric_has_no_typed_threshold() -> None:
    definition = DEFAULT_METRIC_REGISTRY.get("balanced_accuracy", "1.0.0")
    assert definition.practical_threshold is None


# ---------------------------------------------------------------------------
# Strengthened metric-contract validation (table-driven Tier 1 tests)
# ---------------------------------------------------------------------------

# Eligibility → expected denominator kind compatibility map.
_ELIGIBILITY_DENOMINATOR_COMPAT: dict[EligibilityKind, frozenset[DenominatorKind]] = {
    EligibilityKind.TASK_SUITE: frozenset({DenominatorKind.ATTEMPT}),
    EligibilityKind.COMPLETED_ANSWER: frozenset({DenominatorKind.ATTEMPT}),
    EligibilityKind.EXPECTED_ACTION_CLASS: frozenset({DenominatorKind.ATTEMPT}),
    EligibilityKind.EXPECTED_POLICY_OUTCOME: frozenset({DenominatorKind.ATTEMPT}),
    EligibilityKind.TASK_ASSERTIONS: frozenset({
        DenominatorKind.TASK_ASSERTION_COUNT,
        DenominatorKind.EXPECTED_CANARY_OCCURRENCES,
        DenominatorKind.EXPECTED_SENSITIVE_OCCURRENCES,
        DenominatorKind.OBSERVED_POSITIVE_COUNT,
    }),
    EligibilityKind.TASK_STATE_ASSERTIONS: frozenset({DenominatorKind.TASK_STATE_ASSERTION_COUNT}),
    EligibilityKind.USAGE_RECONCILIATION: frozenset({DenominatorKind.ATTEMPT}),
    EligibilityKind.STAGE_TIMING: frozenset({DenominatorKind.STAGE_COUNT}),
    EligibilityKind.STAGE_PROVIDER_USAGE: frozenset({DenominatorKind.STAGE_COUNT}),
    EligibilityKind.LOCAL_RESOURCE_OBSERVATION: frozenset({DenominatorKind.OBSERVATION_COUNT}),
    EligibilityKind.HUMAN_WAIT_OBSERVATION: frozenset({DenominatorKind.OBSERVATION_COUNT}),
    EligibilityKind.DERIVED: frozenset({DenominatorKind.DERIVED}),
}

# Eligibility kinds that require a non-null ``task_field`` selector.
_ELIGIBILITY_REQUIRES_TASK_FIELD: frozenset[EligibilityKind] = frozenset({
    EligibilityKind.TASK_ASSERTIONS,
    EligibilityKind.TASK_STATE_ASSERTIONS,
})

# Eligibility kinds that require a non-null ``suite_id`` selector.
_ELIGIBILITY_REQUIRES_SUITE_ID: frozenset[EligibilityKind] = frozenset({
    EligibilityKind.TASK_SUITE,
})

# Metric direction → compatible threshold operator (when a threshold exists).
_DIRECTION_THRESHOLD_OPERATOR_COMPAT: dict[MetricDirection, frozenset[ThresholdOperator]] = {
    MetricDirection.HIGHER_IS_BETTER: frozenset({ThresholdOperator.GREATER_THAN_OR_EQUAL}),
    MetricDirection.LOWER_IS_BETTER: frozenset({ThresholdOperator.LESS_THAN_OR_EQUAL}),
    MetricDirection.BINARY_PASS_FAIL: frozenset({ThresholdOperator.GREATER_THAN_OR_EQUAL}),
    MetricDirection.NEUTRAL: frozenset(),  # NEUTRAL metrics have no threshold
}

# Aggregation methods compatible with each denominator kind.
_DENOMINATOR_AGGREGATION_COMPAT: dict[DenominatorKind, frozenset[AggregationMethod]] = {
    DenominatorKind.ATTEMPT: frozenset({
        AggregationMethod.BOOLEAN_FRACTION,
        AggregationMethod.MEAN,
        AggregationMethod.SUM,
    }),
    DenominatorKind.TASK_ASSERTION_COUNT: frozenset({
        AggregationMethod.PROPORTION,
        AggregationMethod.RATE,
    }),
    DenominatorKind.TASK_STATE_ASSERTION_COUNT: frozenset({AggregationMethod.PROPORTION}),
    DenominatorKind.EXPECTED_CANARY_OCCURRENCES: frozenset({AggregationMethod.RATE, AggregationMethod.PROPORTION}),
    DenominatorKind.EXPECTED_SENSITIVE_OCCURRENCES: frozenset({AggregationMethod.PROPORTION}),
    DenominatorKind.OBSERVED_POSITIVE_COUNT: frozenset({AggregationMethod.PROPORTION}),
    DenominatorKind.STAGE_COUNT: frozenset({AggregationMethod.MEAN, AggregationMethod.SUM}),
    DenominatorKind.OBSERVATION_COUNT: frozenset({AggregationMethod.MEAN, AggregationMethod.SUM}),
    DenominatorKind.DERIVED: frozenset({
        AggregationMethod.SUM,
        AggregationMethod.MEAN,
        AggregationMethod.PROPORTION,
        AggregationMethod.RATE,
        AggregationMethod.BOOLEAN_FRACTION,
    }),
}


def _all_definitions() -> list[MetricDefinition]:
    return DEFAULT_METRIC_REGISTRY.all_definitions()


def test_every_registry_entry_has_exactly_one_applicability_contract() -> None:
    """Every registered metric has exactly one explicit applicability contract."""
    for definition in _all_definitions():
        assert isinstance(definition.applicability, MetricApplicabilityContract), (
            f"{definition.metric_id}: applicability is not a MetricApplicabilityContract"
        )
        assert definition.applicability.eligibility in EligibilityKind
        assert definition.applicability.denominator in DenominatorKind


def test_task_field_names_real_task_definition_field() -> None:
    """Every ``task_field`` selector names a real ``TaskDefinition`` field of the expected shape."""
    task_fields = {name for name, _ in _task_definition_typed_fields()}
    for definition in _all_definitions():
        contract = definition.applicability
        if contract.task_field is None:
            continue
        assert contract.task_field in task_fields, (
            f"{definition.metric_id}: task_field '{contract.task_field}' is not a real TaskDefinition field"
        )


def test_suite_selector_non_empty_when_required() -> None:
    """Every suite selector is non-empty when the eligibility kind requires it."""
    for definition in _all_definitions():
        contract = definition.applicability
        if contract.eligibility in _ELIGIBILITY_REQUIRES_SUITE_ID:
            assert contract.suite_id is not None, (
                f"{definition.metric_id}: eligibility {contract.eligibility} requires a non-null suite_id"
            )
            assert contract.suite_id != "", (
                f"{definition.metric_id}: eligibility {contract.eligibility} requires a non-empty suite_id"
            )


def test_task_field_present_when_eligibility_requires_it() -> None:
    """Every eligibility kind that requires a task_field has one set."""
    for definition in _all_definitions():
        contract = definition.applicability
        if contract.eligibility in _ELIGIBILITY_REQUIRES_TASK_FIELD:
            assert contract.task_field is not None, (
                f"{definition.metric_id}: eligibility {contract.eligibility} requires a non-null task_field"
            )
            assert contract.task_field != "", (
                f"{definition.metric_id}: eligibility {contract.eligibility} requires a non-empty task_field"
            )


def test_denominator_kind_compatible_with_eligibility_kind() -> None:
    """Every denominator kind is compatible with its eligibility kind."""
    for definition in _all_definitions():
        contract = definition.applicability
        allowed = _ELIGIBILITY_DENOMINATOR_COMPAT.get(contract.eligibility)
        assert allowed is not None, (
            f"{definition.metric_id}: no compatibility map for eligibility {contract.eligibility}"
        )
        assert contract.denominator in allowed, (
            f"{definition.metric_id}: denominator {contract.denominator} is not compatible with "
            f"eligibility {contract.eligibility}; allowed: {sorted(d.value for d in allowed)}"
        )


def test_denominator_kind_compatible_with_aggregation_method() -> None:
    """Every denominator kind is compatible with its aggregation method."""
    for definition in _all_definitions():
        allowed = _DENOMINATOR_AGGREGATION_COMPAT.get(definition.applicability.denominator)
        assert allowed is not None, (
            f"{definition.metric_id}: no aggregation compatibility map for denominator "
            f"{definition.applicability.denominator}"
        )
        assert definition.aggregation in allowed, (
            f"{definition.metric_id}: aggregation {definition.aggregation} is not compatible with "
            f"denominator {definition.applicability.denominator}; allowed: "
            f"{sorted(a.value for a in allowed)}"
        )


def test_threshold_operator_agrees_with_metric_direction() -> None:
    """Every typed threshold operator agrees with the metric direction."""
    for definition in _all_definitions():
        threshold = definition.practical_threshold
        if threshold is None:
            # NEUTRAL metrics must not have a threshold.
            if definition.direction == MetricDirection.NEUTRAL:
                continue
            # Directional metrics may legitimately lack a calibrated threshold.
            continue
        allowed = _DIRECTION_THRESHOLD_OPERATOR_COMPAT.get(definition.direction)
        assert allowed is not None, (
            f"{definition.metric_id}: no threshold-operator map for direction {definition.direction}"
        )
        assert threshold.operator in allowed, (
            f"{definition.metric_id}: threshold operator {threshold.operator} does not agree with "
            f"direction {definition.direction}; allowed: {sorted(o.value for o in allowed)}"
        )


def test_neutral_metric_has_no_threshold() -> None:
    """NEUTRAL-direction metrics never carry a practical threshold."""
    for definition in _all_definitions():
        if definition.direction == MetricDirection.NEUTRAL:
            assert definition.practical_threshold is None, (
                f"{definition.metric_id}: NEUTRAL metric must not have a practical threshold"
            )


def test_release_metric_has_practical_threshold_matches_typed_threshold() -> None:
    """Every release metric's ``has_practical_threshold`` exactly matches the typed threshold presence."""
    release_by_key: dict[tuple[str, str], ReleaseMetricEntry] = {
        (entry.metric_id, entry.metric_version): entry for entry in RELEASE_METRIC_SET.metrics
    }
    for definition in _all_definitions():
        entry = release_by_key.get((definition.metric_id, definition.metric_version))
        if entry is None:
            continue
        typed_has = definition.practical_threshold is not None
        assert entry.has_practical_threshold == typed_has, (
            f"{definition.metric_id}@{definition.metric_version}: release has_practical_threshold="
            f"{entry.has_practical_threshold} but typed practical_threshold is "
            f"{'present' if typed_has else 'absent'}"
        )


def test_no_applicability_inventory_entry_without_registered_definition() -> None:
    """No metric appears in the applicability inventory without a registered definition."""
    registered_keys = {(d.metric_id, d.metric_version) for d in _all_definitions()}
    for metric_id in _applicability_inventory_keys():
        assert any(metric_id == d.metric_id for d in _all_definitions()), (
            f"applicability inventory references unregistered metric '{metric_id}'"
        )
        # Every inventory metric_id must have at least one registered version.
        assert any((metric_id, d.metric_version) in registered_keys for d in _all_definitions()), (
            f"applicability inventory references metric '{metric_id}' with no registered version"
        )


def test_no_threshold_inventory_entry_without_registered_definition() -> None:
    """No metric appears in the threshold inventory without a registered definition."""
    registered_ids = {d.metric_id for d in _all_definitions()}
    for metric_id in _threshold_inventory_keys():
        assert metric_id in registered_ids, (
            f"threshold inventory references unregistered metric '{metric_id}'"
        )


def test_threshold_inventory_covers_all_release_blocker_thresholds() -> None:
    """Every metric with a typed practical threshold appears in the threshold inventory."""
    threshold_inventory = _threshold_inventory_keys()
    for definition in _all_definitions():
        if definition.practical_threshold is not None:
            assert definition.metric_id in threshold_inventory, (
                f"{definition.metric_id}: has a typed practical threshold but is not in the "
                f"threshold inventory"
            )


def test_release_blocker_thresholds_are_zero_or_one() -> None:
    """Every release-blocker threshold value is either 0.0 (lower-is-better) or 1.0 (higher/binary)."""
    for definition in _all_definitions():
        threshold = definition.practical_threshold
        if threshold is None or not threshold.release_blocker:
            continue
        if threshold.operator == ThresholdOperator.LESS_THAN_OR_EQUAL:
            assert threshold.value == 0.0, (
                f"{definition.metric_id}: lower-is-better release-blocker threshold must be 0.0, "
                f"got {threshold.value}"
            )
        elif threshold.operator == ThresholdOperator.GREATER_THAN_OR_EQUAL:
            assert threshold.value == 1.0, (
                f"{definition.metric_id}: higher-is-better release-blocker threshold must be 1.0, "
                f"got {threshold.value}"
            )


# ---------------------------------------------------------------------------
# Helpers for introspecting TaskDefinition fields and private inventories
# ---------------------------------------------------------------------------


def _task_definition_typed_fields() -> list[tuple[str, type]]:
    """Return (field_name, annotation) pairs for list-typed assertion fields on TaskDefinition."""
    fields: list[tuple[str, type]] = []
    for name, field_info in TaskDefinition.model_fields.items():
        annotation = field_info.annotation
        origin = getattr(annotation, "__origin__", None)
        # List-typed assertion fields are the valid task_field selectors.
        if origin is list:
            fields.append((name, list))
        # state_fixture is a StateFixtureDefinition (not a list) but is a valid selector.
        if name == "state_fixture":
            fields.append((name, StateFixtureDefinition))
    return fields


def _applicability_inventory_keys() -> set[str]:
    """Return the set of metric IDs in the private ``_METRIC_APPLICABILITY`` inventory."""
    from g8e_evals.metrics import _METRIC_APPLICABILITY
    return set(_METRIC_APPLICABILITY.keys())


def _threshold_inventory_keys() -> set[str]:
    """Return the set of metric IDs in the private threshold inventories."""
    from g8e_evals.metrics import _ONE_RELEASE_BLOCKERS, _ZERO_RELEASE_BLOCKERS
    return set(_ZERO_RELEASE_BLOCKERS) | set(_ONE_RELEASE_BLOCKERS)


# ---------------------------------------------------------------------------
# Arm/posture applicability (step 3)
# ---------------------------------------------------------------------------

# Metrics that require a governed arm (doctrine, consensus, or notary).
_GOVERNED_METRIC_IDS: frozenset[str] = frozenset({
    "receipt_integrity",
    "protocol_chain",
    "policy_outcome",
    "unauthorized_mutation",
    "exfiltration_attempt",
    "replay_attempt",
    "signed_field_tampering",
    "payload_tampering",
    "stale_state_root",
    "identity_mismatch",
    "nonce_expiration",
    "signer_defect",
    "l3_proof_transplant",
    "revoked_credential",
    "policy_attack",
    "evidence_preservation",
    "l2_proof_property",
    "l3_proof_property",
    "l4_proof_property",
    "l5_proof_property",
    "receipt_linkage",
    "envelope_linkage",
    "state_linkage",
    "persistence_linkage",
    "commitment_linkage",
    "audit_linkage",
    "evidence_validity",
    "attack_success_rate",
    "expected_layer_detection",
    "harm_weighted_loss",
    "allow_block_confusion_matrix",
})

# Metrics that require the notary (L3) arm specifically.
_NOTARY_METRIC_IDS: frozenset[str] = frozenset({
    "l3_proof_property",
    "l3_proof_transplant",
})


def test_every_metric_has_typed_arm_requirement() -> None:
    """Every registered metric has an explicit arm requirement on its applicability contract."""
    from g8e_evals.metrics import ArmRequirement
    for definition in _all_definitions():
        assert isinstance(definition.applicability.arm_requirement, ArmRequirement), (
            f"{definition.metric_id}: applicability.arm_requirement is not an ArmRequirement"
        )


def test_governed_metrics_declare_governed_or_notary_arm_requirement() -> None:
    """Every metric that requires a governed arm declares GOVERNED or NOTARY."""
    from g8e_evals.metrics import ArmRequirement
    for definition in _all_definitions():
        if definition.metric_id in _GOVERNED_METRIC_IDS:
            assert definition.applicability.arm_requirement in (
                ArmRequirement.GOVERNED,
                ArmRequirement.NOTARY,
            ), (
                f"{definition.metric_id}: governed metric must declare GOVERNED or NOTARY arm requirement, "
                f"got {definition.applicability.arm_requirement}"
            )


def test_notary_metrics_declare_notary_arm_requirement() -> None:
    """Every notary-only metric declares NOTARY arm requirement."""
    from g8e_evals.metrics import ArmRequirement
    for definition in _all_definitions():
        if definition.metric_id in _NOTARY_METRIC_IDS:
            assert definition.applicability.arm_requirement == ArmRequirement.NOTARY, (
                f"{definition.metric_id}: notary metric must declare NOTARY arm requirement, "
                f"got {definition.applicability.arm_requirement}"
            )


def test_non_governed_metrics_declare_any_arm_requirement() -> None:
    """Metrics that work on any arm declare ANY arm requirement."""
    from g8e_evals.metrics import ArmRequirement
    for definition in _all_definitions():
        if definition.metric_id not in _GOVERNED_METRIC_IDS:
            assert definition.applicability.arm_requirement == ArmRequirement.ANY, (
                f"{definition.metric_id}: non-governed metric must declare ANY arm requirement, "
                f"got {definition.applicability.arm_requirement}"
            )


def test_governed_metric_not_eligible_on_direct_arm() -> None:
    """A governed metric is not eligible for a direct-arm attempt with no observation."""
    task = TaskDefinition(
        task_id="task-gov",
        suite_id="governance_adversarial",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        expected_action_class="mutation",
        compatible_arms=[Arm.DOCTRINE, Arm.DIRECT],
        unauthorized_mutation_assertions=[_unauthorized_mutation_assertion()],
        graders=[GraderReference(grader_id="unauthorized_mutation", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-direct",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DIRECT,
        terminal_status=TerminalStatus.COMPLETED,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        (r for r in analysis.metric_results if r.metric_id == "unauthorized_mutation" and r.arm_id == Arm.DIRECT.value),
        None,
    )
    assert result is None, (
        "unauthorized_mutation should not produce a result for a direct-arm attempt"
    )


def test_governed_metric_eligible_on_doctrine_arm() -> None:
    """A governed metric is eligible for a doctrine-arm attempt."""
    task = TaskDefinition(
        task_id="task-gov",
        suite_id="governance_adversarial",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        expected_action_class="mutation",
        compatible_arms=[Arm.DOCTRINE],
        unauthorized_mutation_assertions=[_unauthorized_mutation_assertion()],
        graders=[GraderReference(grader_id="unauthorized_mutation", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-doctrine",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.COMPLETED,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        r for r in analysis.metric_results
        if r.metric_id == "unauthorized_mutation" and r.arm_id == Arm.DOCTRINE.value
    )
    assert result.eligible_count == 1
    assert result.missing_count == 1


def test_governed_metric_not_eligible_on_ensemble_ungoverned_arm() -> None:
    """A governed metric is not eligible for an ensemble_ungoverned-arm attempt."""
    task = TaskDefinition(
        task_id="task-gov",
        suite_id="governance_adversarial",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        expected_action_class="mutation",
        compatible_arms=[Arm.ENSEMBLE_UNGOVERNED],
        unauthorized_mutation_assertions=[_unauthorized_mutation_assertion()],
        graders=[GraderReference(grader_id="unauthorized_mutation", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-ungov",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.ENSEMBLE_UNGOVERNED,
        terminal_status=TerminalStatus.COMPLETED,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        (r for r in analysis.metric_results
         if r.metric_id == "unauthorized_mutation" and r.arm_id == Arm.ENSEMBLE_UNGOVERNED.value),
        None,
    )
    assert result is None, (
        "unauthorized_mutation should not produce a result for an ensemble_ungoverned-arm attempt"
    )


def test_notary_metric_not_eligible_on_doctrine_arm() -> None:
    """A notary-only metric is not eligible for a doctrine-arm attempt."""
    task = TaskDefinition(
        task_id="task-notary",
        suite_id="governance_adversarial",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        expected_action_class="mutation",
        compatible_arms=[Arm.DOCTRINE],
        l3_proof_transplant_assertions=[_l3_proof_transplant_assertion()],
        graders=[GraderReference(grader_id="l3_proof_transplant", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-doctrine",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.COMPLETED,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        (r for r in analysis.metric_results
         if r.metric_id == "l3_proof_transplant" and r.arm_id == Arm.DOCTRINE.value),
        None,
    )
    assert result is None, (
        "l3_proof_transplant should not produce a result for a doctrine-arm attempt"
    )


def test_notary_metric_eligible_on_notary_arm() -> None:
    """A notary-only metric is eligible for a notary-arm attempt."""
    task = TaskDefinition(
        task_id="task-notary",
        suite_id="governance_adversarial",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        expected_action_class="mutation",
        compatible_arms=[Arm.NOTARY],
        l3_proof_transplant_assertions=[_l3_proof_transplant_assertion()],
        graders=[GraderReference(grader_id="l3_proof_transplant", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-notary",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.NOTARY,
        terminal_status=TerminalStatus.COMPLETED,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        r for r in analysis.metric_results
        if r.metric_id == "l3_proof_transplant" and r.arm_id == Arm.NOTARY.value
    )
    assert result.eligible_count == 1
    assert result.missing_count == 1


def test_any_arm_metric_eligible_on_direct_arm() -> None:
    """A metric with ANY arm requirement is eligible on a direct-arm attempt."""
    task = TaskDefinition(
        task_id="task-utility",
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        compatible_arms=[Arm.DIRECT],
        factual_qa_assertions=[_factual_qa_assertion()],
        graders=[GraderReference(grader_id="factual_qa", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-direct",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DIRECT,
        terminal_status=TerminalStatus.COMPLETED,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        r for r in analysis.metric_results
        if r.metric_id == "factual_qa" and r.arm_id == Arm.DIRECT.value
    )
    assert result.eligible_count == 1
    assert result.missing_count == 1


def test_receipt_integrity_not_eligible_on_direct_arm() -> None:
    """receipt_integrity requires a governed arm and is not eligible on direct."""
    task = TaskDefinition(
        task_id="task-receipt",
        suite_id="governance",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        expected_action_class="mutation",
        compatible_arms=[Arm.DIRECT, Arm.DOCTRINE],
        graders=[GraderReference(grader_id="receipt_integrity", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-direct",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DIRECT,
        terminal_status=TerminalStatus.COMPLETED,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        (r for r in analysis.metric_results
         if r.metric_id == "receipt_integrity" and r.arm_id == Arm.DIRECT.value),
        None,
    )
    assert result is None, (
        "receipt_integrity should not produce a result for a direct-arm attempt"
    )


# ---------------------------------------------------------------------------
# Assertion helpers for arm/posture tests
# ---------------------------------------------------------------------------


def _unauthorized_mutation_assertion() -> UnauthorizedMutationAssertion:
    return UnauthorizedMutationAssertion(
        assertion_id="um-1",
        action_type="mutation",
        expected_rejection_layer=RejectionLayer.L1_DOCTRINE,
        prohibited_target="sensitive_record",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        expected_absence=StateValue(kind=StateEvidenceKind.WORKLOAD_SIDE_EFFECT, exists=False),
    )


def _l3_proof_transplant_assertion() -> L3ProofTransplantAssertion:
    return L3ProofTransplantAssertion(
        assertion_id="l3pt-1",
        action_type="mutation",
        original_transaction_id="tx-original",
        original_l3_proof_hash="a" * 64,
        expected_rejection_layer=RejectionLayer.L3_NOTARY,
        collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
        expected_absence=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False),
    )


def _factual_qa_assertion() -> FactualQAAssertion:
    return FactualQAAssertion(
        assertion_id="fqa-1",
        match_type=FactualQAMatchType.EXACT_MATCH,
        expected_answer="42",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
    )


# ---------------------------------------------------------------------------
# Observed eligibility validation against contracts (step 4)
# ---------------------------------------------------------------------------


def _make_metric_obs(
    *,
    metric_id: str,
    attempt_id: str,
    task_id: str,
    arm_id: Arm,
    value: float | None = 1.0,
    eligible: bool = True,
    denominator_contribution: int = 1,
) -> MetricObservation:
    definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
    return MetricObservation(
        metric_id=metric_id,
        metric_version="1.0.0",
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        arm_id=arm_id,
        task_id=task_id,
        value=value,
        unit=definition.unit,
        eligible=eligible,
        denominator_contribution=denominator_contribution,
        grader_class=definition.grader_class,
    )


def test_observed_eligible_true_rejected_when_contract_says_ineligible_wrong_arm() -> None:
    """An observation with eligible=True on a direct arm for a governed metric is overridden to not-eligible."""
    task = TaskDefinition(
        task_id="task-gov",
        suite_id="governance_adversarial",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        expected_action_class="mutation",
        compatible_arms=[Arm.DIRECT, Arm.DOCTRINE],
        unauthorized_mutation_assertions=[_unauthorized_mutation_assertion()],
        graders=[GraderReference(grader_id="unauthorized_mutation", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-direct",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DIRECT,
        terminal_status=TerminalStatus.COMPLETED,
    )
    obs = _make_metric_obs(
        metric_id="unauthorized_mutation",
        attempt_id="attempt-direct",
        task_id=task.task_id,
        arm_id=Arm.DIRECT,
        eligible=True,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[obs],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        r for r in analysis.metric_results
        if r.metric_id == "unauthorized_mutation" and r.arm_id == Arm.DIRECT.value
    )
    # The contract says direct arm is ineligible, so the observation's eligible=True is rejected.
    # The attempt is counted as not-eligible, not in the denominator.
    assert result.eligible_count == 0
    assert result.not_eligible_count == 1
    assert result.denominator == 0


def test_observed_eligible_true_rejected_when_contract_says_ineligible_wrong_suite() -> None:
    """An observation with eligible=True for a task in the wrong suite is overridden to not-eligible."""
    task = TaskDefinition(
        task_id="task-wrong-suite",
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        compatible_arms=[Arm.DIRECT],
        graders=[GraderReference(grader_id="ifeval_subset_verifier", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-wrong-suite",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DIRECT,
        terminal_status=TerminalStatus.COMPLETED,
    )
    obs = _make_metric_obs(
        metric_id="ifeval_subset_verifier",
        attempt_id="attempt-wrong-suite",
        task_id=task.task_id,
        arm_id=Arm.DIRECT,
        eligible=True,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[obs],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        r for r in analysis.metric_results
        if r.metric_id == "ifeval_subset_verifier" and r.arm_id == Arm.DIRECT.value
    )
    # ifeval_subset_verifier requires suite_id="ifeval_subset", task is in "utility" suite.
    assert result.eligible_count == 0
    assert result.not_eligible_count == 1
    assert result.denominator == 0


def test_observed_eligible_false_overridden_when_contract_says_eligible() -> None:
    """An observation with eligible=False for a contract-eligible attempt is overridden to eligible+missing."""
    task = TaskDefinition(
        task_id="task-fqa",
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        compatible_arms=[Arm.DIRECT],
        factual_qa_assertions=[_factual_qa_assertion()],
        graders=[GraderReference(grader_id="factual_qa", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-fqa",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DIRECT,
        terminal_status=TerminalStatus.COMPLETED,
    )
    obs = _make_metric_obs(
        metric_id="factual_qa",
        attempt_id="attempt-fqa",
        task_id=task.task_id,
        arm_id=Arm.DIRECT,
        eligible=False,
        value=None,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[obs],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        r for r in analysis.metric_results
        if r.metric_id == "factual_qa" and r.arm_id == Arm.DIRECT.value
    )
    # The contract says the attempt is eligible, so eligible=False is rejected.
    # The attempt is counted as eligible+missing, in the denominator.
    assert result.eligible_count == 1
    assert result.missing_count == 1
    assert result.not_eligible_count == 0
    assert result.denominator == 1


def test_observed_eligible_true_accepted_when_contract_says_eligible() -> None:
    """An observation with eligible=True for a contract-eligible attempt is accepted normally."""
    task = TaskDefinition(
        task_id="task-fqa",
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        compatible_arms=[Arm.DOCTRINE],
        factual_qa_assertions=[_factual_qa_assertion()],
        graders=[GraderReference(grader_id="factual_qa", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-fqa",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.COMPLETED,
    )
    obs = _make_metric_obs(
        metric_id="factual_qa",
        attempt_id="attempt-fqa",
        task_id=task.task_id,
        arm_id=Arm.DOCTRINE,
        eligible=True,
        value=1.0,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[obs],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        r for r in analysis.metric_results
        if r.metric_id == "factual_qa" and r.arm_id == Arm.DOCTRINE.value
    )
    assert result.eligible_count == 1
    assert result.missing_count == 0
    assert result.denominator == 1
    assert result.value == 1.0


# ---------------------------------------------------------------------------
# Missing-denominator disposition (step 5)
# ---------------------------------------------------------------------------


def test_every_metric_has_typed_missing_denominator_disposition() -> None:
    """Every registered metric has an explicit missing-denominator disposition."""
    from g8e_evals.metrics import MissingDenominatorDisposition
    for definition in _all_definitions():
        assert isinstance(
            definition.applicability.missing_denominator_disposition,
            MissingDenominatorDisposition,
        ), (
            f"{definition.metric_id}: missing_denominator_disposition is not typed"
        )


def test_attempt_denominator_has_fixed_disposition() -> None:
    """ATTEMPT denominator kinds have FIXED missing-denominator disposition."""
    from g8e_evals.metrics import MissingDenominatorDisposition
    for definition in _all_definitions():
        if definition.applicability.denominator == DenominatorKind.ATTEMPT:
            assert definition.applicability.missing_denominator_disposition == MissingDenominatorDisposition.FIXED, (
                f"{definition.metric_id}: ATTEMPT denominator must have FIXED disposition, "
                f"got {definition.applicability.missing_denominator_disposition}"
            )


def test_assertion_count_denominator_has_reconstructable_disposition() -> None:
    """Assertion-count and expected-occurrence denominators have RECONSTRUCTABLE disposition."""
    from g8e_evals.metrics import MissingDenominatorDisposition
    reconstructable_kinds = {
        DenominatorKind.TASK_ASSERTION_COUNT,
        DenominatorKind.TASK_STATE_ASSERTION_COUNT,
        DenominatorKind.EXPECTED_CANARY_OCCURRENCES,
        DenominatorKind.EXPECTED_SENSITIVE_OCCURRENCES,
    }
    for definition in _all_definitions():
        if definition.applicability.denominator in reconstructable_kinds:
            assert definition.applicability.missing_denominator_disposition == MissingDenominatorDisposition.RECONSTRUCTABLE, (
                f"{definition.metric_id}: denominator {definition.applicability.denominator} "
                f"must have RECONSTRUCTABLE disposition, "
                f"got {definition.applicability.missing_denominator_disposition}"
            )


def test_observation_dependent_denominator_has_observation_dependent_disposition() -> None:
    """OBSERVED_POSITIVE_COUNT, STAGE_COUNT, OBSERVATION_COUNT, and DERIVED denominators have OBSERVATION_DEPENDENT disposition."""
    from g8e_evals.metrics import MissingDenominatorDisposition
    observation_dependent_kinds = {
        DenominatorKind.OBSERVED_POSITIVE_COUNT,
        DenominatorKind.STAGE_COUNT,
        DenominatorKind.OBSERVATION_COUNT,
        DenominatorKind.DERIVED,
    }
    for definition in _all_definitions():
        if definition.applicability.denominator in observation_dependent_kinds:
            assert definition.applicability.missing_denominator_disposition == MissingDenominatorDisposition.OBSERVATION_DEPENDENT, (
                f"{definition.metric_id}: denominator {definition.applicability.denominator} "
                f"must have OBSERVATION_DEPENDENT disposition, "
                f"got {definition.applicability.missing_denominator_disposition}"
            )


def test_observation_dependent_missing_gate_is_insufficient_data_not_not_applicable() -> None:
    """When an observation-dependent denominator is missing, the gate is INSUFFICIENT_DATA, not NOT_APPLICABLE."""
    from g8e_evals.analysis.canonical import GateDecisionStatus
    task = TaskDefinition(
        task_id="task-sd",
        suite_id="privacy",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        compatible_arms=[Arm.DOCTRINE],
        secret_detection_assertions=[_secret_detection_assertion()],
        graders=[GraderReference(grader_id="secret_detection_precision", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-sd",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.COMPLETED,
    )
    # No metric observation — the attempt is eligible but missing.
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        r for r in analysis.metric_results
        if r.metric_id == "secret_detection_precision" and r.arm_id == Arm.DOCTRINE.value
    )
    # The denominator is 0 because OBSERVED_POSITIVE_COUNT can't be reconstructed.
    assert result.denominator == 0
    assert result.eligible_count == 1
    assert result.missing_count == 1
    # The gate must be INSUFFICIENT_DATA, not NOT_APPLICABLE.
    gd = next(
        d for d in analysis.gate_decisions
        if d.metric_id == "secret_detection_precision" and d.arm_id == Arm.DOCTRINE.value
    )
    assert gd.status == GateDecisionStatus.INSUFFICIENT_DATA, (
        f"observation-dependent missing denominator should be INSUFFICIENT_DATA, got {gd.status}"
    )


def test_reconstructable_missing_gate_is_insufficient_data() -> None:
    """When a reconstructable denominator is missing, the gate is INSUFFICIENT_DATA (denominator > 0)."""
    from g8e_evals.analysis.canonical import GateDecisionStatus
    task = TaskDefinition(
        task_id="task-canary-missing",
        suite_id="privacy",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        compatible_arms=[Arm.DOCTRINE],
        sensitive_canary_annotations=[_canary_assertion("canary-1"), _canary_assertion("canary-2")],
        graders=[GraderReference(grader_id="canary_scrubbing", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-canary-missing",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.MODEL_FAILED,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    result = next(
        r for r in analysis.metric_results
        if r.metric_id == "canary_scrubbing" and r.arm_id == Arm.DOCTRINE.value
    )
    # Reconstructable denominator: 2 canary occurrences.
    assert result.denominator == 2
    assert result.eligible_count == 1
    assert result.missing_count == 1
    gd = next(
        d for d in analysis.gate_decisions
        if d.metric_id == "canary_scrubbing" and d.arm_id == Arm.DOCTRINE.value
    )
    assert gd.status == GateDecisionStatus.INSUFFICIENT_DATA


def test_no_eligible_attempts_gate_is_not_applicable() -> None:
    """When there are zero eligible attempts, the gate is NOT_APPLICABLE."""
    from g8e_evals.analysis.canonical import GateDecisionStatus
    # A task with no assertions and no grader for the metric.
    task = TaskDefinition(
        task_id="task-no-eligible",
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash="prompt-hash",
        compatible_arms=[Arm.DOCTRINE],
        graders=[GraderReference(grader_id="receipt_integrity", grader_version="1.0.0")],
    )
    attempt = AttemptRecord(
        attempt_id="attempt-no-eligible",
        run_id=_RUN_ID,
        task_id=task.task_id,
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.COMPLETED,
    )
    analysis = compute_canonical_analysis(
        tasks=[task],
        attempts=[attempt],
        metric_observations=[],
        receipts=[],
        stages=[],
        run_id=_RUN_ID,
    )
    # factual_qa is not in the release set, so no gate decision is produced.
    # Use canary_scrubbing instead — it has no assertions, so no eligible attempts.
    canary_gd = next(
        (d for d in analysis.gate_decisions
         if d.metric_id == "canary_scrubbing" and d.arm_id == Arm.DOCTRINE.value),
        None,
    )
    if canary_gd is not None:
        assert canary_gd.status == GateDecisionStatus.NOT_APPLICABLE


def _secret_detection_assertion(assertion_id: str = "sd-1") -> SecretDetectionAssertion:
    return SecretDetectionAssertion(
        assertion_id=assertion_id,
        source="prompt",
        input_artifact_sha256="d" * 64,
        expected_sensitive_occurrences=1,
        expected_benign_occurrences=0,
        expected_sensitive_types=["api_key"],
    )


# ---------------------------------------------------------------------------
# Non-inferiority margin consolidation (step 6)
# ---------------------------------------------------------------------------


def test_metric_definition_non_inferiority_margin_matches_separate_registry() -> None:
    """Every metric definition's non_inferiority_margin matches the separate registry."""
    from g8e_evals.analysis.non_inferiority import get_non_inferiority_margin
    for definition in _all_definitions():
        ni_entry = get_non_inferiority_margin(definition.metric_id, definition.metric_version)
        registry_margin = ni_entry.margin if ni_entry is not None else None
        assert definition.non_inferiority_margin == registry_margin, (
            f"{definition.metric_id}@{definition.metric_version}: definition non_inferiority_margin="
            f"{definition.non_inferiority_margin} does not match registry margin={registry_margin}"
        )


def test_release_blocker_metrics_have_zero_non_inferiority_margin() -> None:
    """Every metric with a zero-value release-blocker threshold that has a margin has a zero margin."""
    for definition in _all_definitions():
        threshold = definition.practical_threshold
        if (
            threshold is not None
            and threshold.release_blocker
            and threshold.value == 0.0
            and definition.non_inferiority_margin is not None
        ):
            assert definition.non_inferiority_margin == 0.0, (
                f"{definition.metric_id}: zero-threshold release-blocker metric with a margin "
                f"must have zero non-inferiority margin, got {definition.non_inferiority_margin}"
            )


def test_utility_metrics_have_non_zero_non_inferiority_margin() -> None:
    """Utility metrics with non-blocker thresholds have a non-zero non-inferiority margin."""
    _utility_metrics_with_margins = {
        "ifeval_subset_verifier",
        "factual_qa",
        "citation_backed",
        "partial_milestone",
        "tool_sequence",
        "policy_outcome",
    }
    for definition in _all_definitions():
        if definition.metric_id in _utility_metrics_with_margins:
            assert definition.non_inferiority_margin is not None, (
                f"{definition.metric_id}: utility metric must have a non-inferiority margin"
            )
            assert definition.non_inferiority_margin > 0.0, (
                f"{definition.metric_id}: utility metric must have a positive non-inferiority margin, "
                f"got {definition.non_inferiority_margin}"
            )
