# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Phase 1.1 tests for the extended metric registry.

Verifies that the new derived analysis metrics and primary telemetry
metrics are registered, have correct grader class assignments, are in
the release metric set, and have valid domain mappings. Also tests the
new LocalResourceObservation and HumanWaitObservation schema types.
"""

from __future__ import annotations

from datetime import UTC, datetime

import pytest

from g8e_evals.metrics import (
    DEFAULT_METRIC_REGISTRY,
    GraderClass,
    MetricDirection,
)
from g8e_evals.release_metric_set import (
    MetricDomain,
    RELEASE_METRIC_SET,
)
from g8e_evals.schema import (
    AttemptRecord,
    GraderClass as SchemaGraderClass,
    HumanWaitObservation,
    LocalResourceObservation,
    VerificationStatus,
)
from g8e_evals.arms import Arm


pytestmark = pytest.mark.unit


# --- New metric IDs ---

_DERIVED_ANALYSIS_METRIC_IDS = frozenset({
    "allow_block_confusion_matrix",
    "attack_success_rate",
    "expected_layer_detection",
    "balanced_accuracy",
    "matthews_correlation_coefficient",
    "harm_weighted_loss",
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
})

_NEW_TELEMETRY_METRIC_IDS = frozenset({
    "stage_latency_seconds",
    "provider_usage_tokens",
    "provider_cost_usd",
    "local_resource_peak_memory_bytes",
    "local_resource_cpu_seconds",
    "human_wait_seconds",
})


# --- Tests for derived analysis metrics ---

@pytest.mark.unit
def test_all_derived_analysis_metrics_are_registered():
    for metric_id in _DERIVED_ANALYSIS_METRIC_IDS:
        assert DEFAULT_METRIC_REGISTRY.is_registered(metric_id, "1.0.0"), (
            f"derived analysis metric {metric_id} is not registered"
        )


@pytest.mark.unit
def test_derived_analysis_metrics_have_analysis_grader_class():
    for metric_id in _DERIVED_ANALYSIS_METRIC_IDS:
        definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
        assert definition.grader_class == GraderClass.ANALYSIS, (
            f"derived analysis metric {metric_id} has grader_class {definition.grader_class}, expected ANALYSIS"
        )


@pytest.mark.unit
def test_derived_analysis_metrics_have_no_grader_ref():
    for metric_id in _DERIVED_ANALYSIS_METRIC_IDS:
        definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
        assert definition.grader_ref is None, (
            f"derived analysis metric {metric_id} has a grader_ref, expected None"
        )


@pytest.mark.unit
def test_derived_analysis_metrics_are_in_release_set():
    release_ids = RELEASE_METRIC_SET.metric_ids
    for metric_id in _DERIVED_ANALYSIS_METRIC_IDS:
        assert (metric_id, "1.0.0") in release_ids, (
            f"derived analysis metric {metric_id} is not in the release set"
        )


@pytest.mark.unit
def test_derived_analysis_metrics_have_valid_domains():
    for entry in RELEASE_METRIC_SET.metrics:
        if entry.metric_id in _DERIVED_ANALYSIS_METRIC_IDS:
            assert isinstance(entry.domain, MetricDomain), (
                f"derived analysis metric {entry.metric_id} has invalid domain {entry.domain}"
            )


@pytest.mark.unit
def test_l2_through_l5_proof_properties_are_governance_domain():
    for metric_id in ("l2_proof_property", "l3_proof_property", "l4_proof_property", "l5_proof_property"):
        entry = next(e for e in RELEASE_METRIC_SET.metrics if e.metric_id == metric_id)
        assert entry.domain == MetricDomain.GOVERNANCE, (
            f"{metric_id} has domain {entry.domain}, expected GOVERNANCE"
        )


@pytest.mark.unit
def test_attack_success_rate_is_governance_adversarial_domain():
    entry = next(e for e in RELEASE_METRIC_SET.metrics if e.metric_id == "attack_success_rate")
    assert entry.domain == MetricDomain.GOVERNANCE_ADVERSARIAL


@pytest.mark.unit
def test_harm_weighted_loss_is_governance_adversarial_domain():
    entry = next(e for e in RELEASE_METRIC_SET.metrics if e.metric_id == "harm_weighted_loss")
    assert entry.domain == MetricDomain.GOVERNANCE_ADVERSARIAL


@pytest.mark.unit
def test_state_linkage_is_state_domain():
    entry = next(e for e in RELEASE_METRIC_SET.metrics if e.metric_id == "state_linkage")
    assert entry.domain == MetricDomain.STATE


@pytest.mark.unit
def test_evidence_validity_is_reliability_domain():
    entry = next(e for e in RELEASE_METRIC_SET.metrics if e.metric_id == "evidence_validity")
    assert entry.domain == MetricDomain.RELIABILITY


@pytest.mark.unit
def test_attack_success_rate_is_lower_is_better():
    definition = DEFAULT_METRIC_REGISTRY.get("attack_success_rate", "1.0.0")
    assert definition.direction == MetricDirection.LOWER_IS_BETTER


@pytest.mark.unit
def test_harm_weighted_loss_is_lower_is_better():
    definition = DEFAULT_METRIC_REGISTRY.get("harm_weighted_loss", "1.0.0")
    assert definition.direction == MetricDirection.LOWER_IS_BETTER


@pytest.mark.unit
def test_confusion_matrix_is_neutral_direction():
    definition = DEFAULT_METRIC_REGISTRY.get("allow_block_confusion_matrix", "1.0.0")
    assert definition.direction == MetricDirection.NEUTRAL


@pytest.mark.unit
def test_balanced_accuracy_is_higher_is_better():
    definition = DEFAULT_METRIC_REGISTRY.get("balanced_accuracy", "1.0.0")
    assert definition.direction == MetricDirection.HIGHER_IS_BETTER


@pytest.mark.unit
def test_matthews_correlation_coefficient_is_higher_is_better():
    definition = DEFAULT_METRIC_REGISTRY.get("matthews_correlation_coefficient", "1.0.0")
    assert definition.direction == MetricDirection.HIGHER_IS_BETTER


@pytest.mark.unit
def test_l2_through_l5_proof_properties_have_release_blocker_thresholds():
    for metric_id in ("l2_proof_property", "l3_proof_property", "l4_proof_property", "l5_proof_property"):
        definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
        assert definition.threshold_rendering is not None
        assert "1.0" in definition.threshold_rendering


@pytest.mark.unit
def test_linkage_metrics_have_release_blocker_thresholds():
    for metric_id in ("receipt_linkage", "envelope_linkage", "state_linkage", "persistence_linkage", "commitment_linkage", "audit_linkage"):
        definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
        assert definition.threshold_rendering is not None
        assert "1.0" in definition.threshold_rendering


@pytest.mark.unit
def test_evidence_validity_has_release_blocker_threshold():
    definition = DEFAULT_METRIC_REGISTRY.get("evidence_validity", "1.0.0")
    assert definition.threshold_rendering is not None
    assert "1.0" in definition.threshold_rendering


@pytest.mark.unit
def test_attack_success_rate_has_zero_threshold():
    definition = DEFAULT_METRIC_REGISTRY.get("attack_success_rate", "1.0.0")
    assert definition.threshold_rendering is not None
    assert "0.0" in definition.threshold_rendering


@pytest.mark.unit
def test_expected_layer_detection_has_release_blocker_threshold():
    definition = DEFAULT_METRIC_REGISTRY.get("expected_layer_detection", "1.0.0")
    assert definition.threshold_rendering is not None
    assert "1.0" in definition.threshold_rendering


# --- Tests for new primary telemetry metrics ---

@pytest.mark.unit
def test_all_new_telemetry_metrics_are_registered():
    for metric_id in _NEW_TELEMETRY_METRIC_IDS:
        assert DEFAULT_METRIC_REGISTRY.is_registered(metric_id, "1.0.0"), (
            f"telemetry metric {metric_id} is not registered"
        )


@pytest.mark.unit
def test_new_telemetry_metrics_have_no_grader_ref():
    for metric_id in _NEW_TELEMETRY_METRIC_IDS:
        definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
        assert definition.grader_ref is None, (
            f"telemetry metric {metric_id} has a grader_ref, expected None"
        )


@pytest.mark.unit
def test_new_telemetry_metrics_are_in_release_set():
    release_ids = RELEASE_METRIC_SET.metric_ids
    for metric_id in _NEW_TELEMETRY_METRIC_IDS:
        assert (metric_id, "1.0.0") in release_ids, (
            f"telemetry metric {metric_id} is not in the release set"
        )


@pytest.mark.unit
def test_stage_latency_is_neutral_direction():
    definition = DEFAULT_METRIC_REGISTRY.get("stage_latency_seconds", "1.0.0")
    assert definition.direction == MetricDirection.NEUTRAL


@pytest.mark.unit
def test_provider_usage_tokens_is_neutral_direction():
    definition = DEFAULT_METRIC_REGISTRY.get("provider_usage_tokens", "1.0.0")
    assert definition.direction == MetricDirection.NEUTRAL


@pytest.mark.unit
def test_provider_cost_usd_is_neutral_direction():
    definition = DEFAULT_METRIC_REGISTRY.get("provider_cost_usd", "1.0.0")
    assert definition.direction == MetricDirection.NEUTRAL


@pytest.mark.unit
def test_local_resource_metrics_are_neutral_direction():
    for metric_id in ("local_resource_peak_memory_bytes", "local_resource_cpu_seconds"):
        definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
        assert definition.direction == MetricDirection.NEUTRAL


@pytest.mark.unit
def test_human_wait_seconds_is_neutral_direction():
    definition = DEFAULT_METRIC_REGISTRY.get("human_wait_seconds", "1.0.0")
    assert definition.direction == MetricDirection.NEUTRAL


@pytest.mark.unit
def test_provider_cost_usd_is_economics_domain():
    entry = next(e for e in RELEASE_METRIC_SET.metrics if e.metric_id == "provider_cost_usd")
    assert entry.domain == MetricDomain.ECONOMICS


@pytest.mark.unit
def test_telemetry_metrics_are_telemetry_domain():
    telemetry_metric_ids = {"stage_latency_seconds", "provider_usage_tokens", "local_resource_peak_memory_bytes", "local_resource_cpu_seconds", "human_wait_seconds"}
    for metric_id in telemetry_metric_ids:
        entry = next(e for e in RELEASE_METRIC_SET.metrics if e.metric_id == metric_id)
        assert entry.domain == MetricDomain.TELEMETRY, (
            f"{metric_id} has domain {entry.domain}, expected TELEMETRY"
        )


@pytest.mark.unit
def test_telemetry_metrics_have_no_threshold():
    for metric_id in _NEW_TELEMETRY_METRIC_IDS:
        definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
        assert definition.threshold_rendering is None, (
            f"telemetry metric {metric_id} has a threshold_rendering, expected None"
        )


# --- Tests for new observation types ---

@pytest.mark.unit
def test_local_resource_observation_is_valid_with_all_fields():
    obs = LocalResourceObservation(
        observation_id="obs-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        peak_memory_bytes=1024 * 1024,
        average_memory_bytes=512 * 1024,
        cpu_seconds=0.5,
        disk_bytes_written=2048,
        disk_bytes_read=4096,
        peak_fd_count=10,
        collected_at=datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC),
        source_evidence_refs=["evidence-1"],
        source_evidence_sha256="a" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )
    assert obs.peak_memory_bytes == 1024 * 1024
    assert obs.cpu_seconds == 0.5


@pytest.mark.unit
def test_local_resource_observation_allows_none_values():
    obs = LocalResourceObservation(
        observation_id="obs-2",
        attempt_id="attempt-2",
        run_id="run-1",
        task_id="task-1",
        peak_memory_bytes=None,
        collected_at=datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC),
        verification_status=VerificationStatus.PENDING,
    )
    assert obs.peak_memory_bytes is None


@pytest.mark.unit
def test_local_resource_observation_rejects_extra_fields():
    with pytest.raises(Exception, match="extra"):
        LocalResourceObservation(
            observation_id="obs-3",
            attempt_id="attempt-3",
            run_id="run-1",
            task_id="task-1",
            collected_at=datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC),
            extra_field="not allowed",  # type: ignore[call-arg]
        )


@pytest.mark.unit
def test_local_resource_observation_verified_requires_evidence():
    with pytest.raises(Exception, match="source evidence"):
        LocalResourceObservation(
            observation_id="obs-4",
            attempt_id="attempt-4",
            run_id="run-1",
            task_id="task-1",
            collected_at=datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


@pytest.mark.unit
def test_human_wait_observation_is_valid_with_all_fields():
    obs = HumanWaitObservation(
        observation_id="obs-hw-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        human_wait_seconds=30.5,
        human_action_type="approval",
        collected_at=datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC),
        source_evidence_refs=["evidence-1"],
        source_evidence_sha256="b" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )
    assert obs.human_wait_seconds == 30.5
    assert obs.human_action_type == "approval"


@pytest.mark.unit
def test_human_wait_observation_allows_none_wait():
    obs = HumanWaitObservation(
        observation_id="obs-hw-2",
        attempt_id="attempt-2",
        run_id="run-1",
        task_id="task-1",
        human_wait_seconds=None,
        collected_at=datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC),
        verification_status=VerificationStatus.PENDING,
    )
    assert obs.human_wait_seconds is None


@pytest.mark.unit
def test_human_wait_observation_rejects_extra_fields():
    with pytest.raises(Exception, match="extra"):
        HumanWaitObservation(
            observation_id="obs-hw-3",
            attempt_id="attempt-3",
            run_id="run-1",
            task_id="task-1",
            collected_at=datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC),
            extra_field="not allowed",  # type: ignore[call-arg]
        )


@pytest.mark.unit
def test_human_wait_observation_verified_requires_evidence():
    with pytest.raises(Exception, match="source evidence"):
        HumanWaitObservation(
            observation_id="obs-hw-4",
            attempt_id="attempt-4",
            run_id="run-1",
            task_id="task-1",
            collected_at=datetime(2026, 1, 1, 12, 0, 0, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


@pytest.mark.unit
def test_attempt_record_has_local_resource_observation_refs():
    attempt = AttemptRecord(
        attempt_id="attempt-lr",
        run_id="run-1",
        task_id="task-1",
        arm_id=Arm.DOCTRINE,
    )
    assert hasattr(attempt, "local_resource_observation_refs")
    assert attempt.local_resource_observation_refs == []


@pytest.mark.unit
def test_attempt_record_has_human_wait_observation_refs():
    attempt = AttemptRecord(
        attempt_id="attempt-hw",
        run_id="run-1",
        task_id="task-1",
        arm_id=Arm.DOCTRINE,
    )
    assert hasattr(attempt, "human_wait_observation_refs")
    assert attempt.human_wait_observation_refs == []


# --- Tests for GraderClass.ANALYSIS ---

@pytest.mark.unit
def test_grader_class_analysis_exists():
    assert SchemaGraderClass.ANALYSIS == "analysis"
    assert GraderClass.ANALYSIS == "analysis"


@pytest.mark.unit
def test_grader_class_analysis_is_distinct_from_others():
    assert GraderClass.ANALYSIS != GraderClass.DETERMINISTIC
    assert GraderClass.ANALYSIS != GraderClass.LLM_JUDGE
    assert GraderClass.ANALYSIS != GraderClass.HUMAN


# --- Test for total metric count ---

@pytest.mark.unit
def test_total_registered_metric_count_includes_new_metrics():
    """The registry should now have 36 original + 17 derived + 6 telemetry + 8 scenario + 10 tool scorecard + 5 escalation + 11 security events + 4 correlated errors = 97 metrics."""
    all_defs = DEFAULT_METRIC_REGISTRY.all_definitions()
    assert len(all_defs) == 97, f"expected 97 metrics, got {len(all_defs)}"
