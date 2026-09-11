# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 and Tier 2 tests for the EF12 correlated-error tracking.

Verifies that:
- ``ErrorClassLabel`` enumerates the semantic error class labels.
- ``StackCompositionType`` classifies stacks as homogeneous or heterogeneous.
- ``CorrelatedErrorRecord`` is frozen with ``extra="forbid"`` and carries all
  required bindings (agent persona, model variant, stage role, error class,
  stack ID, stack composition type).
- ``validate_correlated_error_records`` rejects duplicate
  (run, attempt, task, stage_role) tuples.
- Verified records require source evidence.
- The campaign verifier has a ``correlated_errors`` layer that validates
  records when present, passes when absent, rejects malformed records,
  rejects duplicates, rejects symlinks, and rejects records bound to
  unknown attempts.
- The 4 correlated-error metrics are registered in the metric registry with
  correct grader class, direction, aggregation, evidence requirements,
  non-inferiority margins, and release domain.
- The derived producers compute correct correlated failure rates from
  ``CorrelatedErrorRecord`` records, including the same-family vs
  cross-family split and the failure-independence inversion.
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with extra fields and
# invalid evidence to verify pydantic validation rejects them.

from __future__ import annotations

import asyncio
import hashlib
import json
from dataclasses import dataclass
from pathlib import Path

import pytest
from pydantic import ValidationError

from g8e_evals.analysis.canonical import (
    ClaimPolicy,
    ContinuousTestPolicy,
    PreregistrationConfig,
)
from g8e_evals.analysis.derived import (
    produce_correlated_failure_rate_observations,
    produce_cross_family_correlated_rate_observations,
    produce_failure_independence_observations,
    produce_same_family_correlated_rate_observations,
)
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.arms import Arm
from g8e_evals.campaign import (
    InitialStateAssignmentManifest,
    ModelCohort,
    RetryPolicy,
    RoleModelBinding,
    SamplingSettings,
    TaskAssignmentManifest,
    compute_initial_state_hash,
    compute_model_cohort_hash,
    compute_task_assignment_hash,
)
from g8e_evals.campaign_verify import verify_campaign
from g8e_evals.constants import CORRELATED_ERRORS_JSONL
from g8e_evals.harness import Response, Score, Task
from g8e_evals.metrics import (
    DEFAULT_METRIC_REGISTRY,
    GraderClass,
    MetricDirection,
)
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.release_metric_set import MetricDomain, RELEASE_METRIC_SET
from g8e_evals.runner import CampaignRunner, CampaignSpec
from g8e_evals.schema import (
    AttemptRecord,
    CorrelatedErrorRecord,
    ErrorClassLabel,
    StackCompositionType,
    VerificationStatus,
    validate_correlated_error_records,
)


_VALID_HASH = "a" * 64

_CAMPAIGN_ID = "v2.1.8-ifeval-pipeline-integrity"
_RELEASE_VERSION = "v2.1.8"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_ID = "cohort-qwen3-8b"
_REPLICATE_IDS = ["replicate-1"]
_INITIAL_STATE_ID = "no-initial-state-v1"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64


# --- Tier 1: Model validation ---


class TestErrorClassLabel:
    pytestmark = pytest.mark.unit

    def test_enum_has_expected_values(self):
        values = {label.value for label in ErrorClassLabel}
        assert values == {
            "unsupported_causal_claim",
            "wrong_tool_selected",
            "schema_violation",
            "argument_semantics_error",
            "misinterpreted_result",
            "incomplete_analysis",
            "hallucinated_evidence",
            "wrong_classification",
            "protocol_violation",
            "unsupported_conclusion",
            "missed_escalation",
            "other",
        }

    def test_enum_values_are_unique(self):
        values = [label.value for label in ErrorClassLabel]
        assert len(values) == len(set(values))

    def test_enum_is_str_enum(self):
        assert isinstance(ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM, str)
        assert ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM == "unsupported_causal_claim"


class TestStackCompositionType:
    pytestmark = pytest.mark.unit

    def test_enum_has_two_values(self):
        assert len(list(StackCompositionType)) == 2

    def test_homogeneous_value(self):
        assert StackCompositionType.HOMOGENEOUS == "homogeneous"

    def test_heterogeneous_value(self):
        assert StackCompositionType.HETEROGENEOUS == "heterogeneous"

    def test_is_str_enum(self):
        assert isinstance(StackCompositionType.HOMOGENEOUS, str)


class TestCorrelatedErrorRecordModel:
    pytestmark = pytest.mark.unit

    def test_record_is_frozen(self):
        rec = _make_record()
        with pytest.raises((TypeError, ValueError)):
            rec.error_class = ErrorClassLabel.OTHER  # type: ignore[misc]

    def test_record_rejects_extra_fields(self):
        with pytest.raises(ValidationError):
            CorrelatedErrorRecord(
                record_id="ce-1",
                attempt_id="att-1",
                run_id="run-1",
                task_id="task-1",
                agent_persona="sage",
                model_variant_id="qwen3-8b-q4_0",
                stage_role="primary",
                error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM,
                stack_id="stack-best-accuracy",
                stack_composition_type=StackCompositionType.HETEROGENEOUS,
                extra_field="bad",
            )

    def test_record_has_all_required_fields(self):
        rec = _make_record()
        assert rec.record_id == "ce-1"
        assert rec.attempt_id == "att-1"
        assert rec.run_id == "run-1"
        assert rec.task_id == "task-1"
        assert rec.agent_persona == "sage"
        assert rec.model_variant_id == "qwen3-8b-q4_0"
        assert rec.stage_role == "primary"
        assert rec.error_class == ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM
        assert rec.stack_id == "stack-best-accuracy"
        assert rec.stack_composition_type == StackCompositionType.HETEROGENEOUS

    def test_record_binds_persona_model_stage_role_task(self):
        rec = _make_record(
            agent_persona="dash",
            model_variant_id="phi-4-mini-q4_0",
            stage_role="assistant",
            task_id="NET-014",
        )
        assert rec.agent_persona == "dash"
        assert rec.model_variant_id == "phi-4-mini-q4_0"
        assert rec.stage_role == "assistant"
        assert rec.task_id == "NET-014"

    def test_record_supports_homogeneous_stack(self):
        rec = _make_record(stack_composition_type=StackCompositionType.HOMOGENEOUS)
        assert rec.stack_composition_type == StackCompositionType.HOMOGENEOUS

    def test_record_supports_heterogeneous_stack(self):
        rec = _make_record(stack_composition_type=StackCompositionType.HETEROGENEOUS)
        assert rec.stack_composition_type == StackCompositionType.HETEROGENEOUS

    def test_record_supports_all_error_class_labels(self):
        for label in ErrorClassLabel:
            rec = _make_record(error_class=label)
            assert rec.error_class == label

    def test_empty_record_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(record_id="")

    def test_empty_attempt_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(attempt_id="")

    def test_empty_run_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(run_id="")

    def test_empty_task_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(task_id="")

    def test_empty_agent_persona_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(agent_persona="")

    def test_empty_model_variant_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(model_variant_id="")

    def test_empty_stage_role_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(stage_role="")

    def test_empty_stack_id_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(stack_id="")

    def test_verified_record_requires_evidence_refs(self):
        with pytest.raises(ValidationError, match="source evidence"):
            _make_record(verification_status=VerificationStatus.VERIFIED)

    def test_verified_record_requires_evidence_sha256(self):
        with pytest.raises(ValidationError, match="source evidence"):
            _make_record(
                verification_status=VerificationStatus.VERIFIED,
                source_evidence_refs=["ev-1"],
            )

    def test_verified_record_with_evidence_passes(self):
        rec = _make_record(
            verification_status=VerificationStatus.VERIFIED,
            source_evidence_refs=["ev-1"],
            source_evidence_sha256=_VALID_HASH,
        )
        assert rec.verification_status == VerificationStatus.VERIFIED

    def test_duplicate_evidence_refs_rejected(self):
        with pytest.raises(ValidationError, match="unique"):
            _make_record(source_evidence_refs=["ev-1", "ev-1"])

    def test_invalid_evidence_sha256_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(source_evidence_sha256="not-a-hash")

    def test_pending_record_without_evidence_passes(self):
        rec = _make_record()
        assert rec.verification_status == VerificationStatus.PENDING
        assert rec.source_evidence_refs == []
        assert rec.source_evidence_sha256 is None

    def test_record_serialization_roundtrip(self):
        rec = _make_record()
        data = json.loads(rec.model_dump_json())
        assert data["record_id"] == "ce-1"
        assert data["error_class"] == "unsupported_causal_claim"
        assert data["stack_composition_type"] == "heterogeneous"
        restored = CorrelatedErrorRecord.model_validate(data)
        assert restored == rec


class TestValidateCorrelatedErrorRecords:
    pytestmark = pytest.mark.unit

    def test_valid_records_pass(self):
        ce1 = _make_record(record_id="ce-1", task_id="task-1", stage_role="primary")
        ce2 = _make_record(record_id="ce-2", task_id="task-1", stage_role="assistant")
        validate_correlated_error_records([ce1, ce2])

    def test_duplicate_run_attempt_task_stage_role_rejected(self):
        ce1 = _make_record(record_id="ce-1", task_id="task-1", stage_role="primary")
        ce2 = _make_record(record_id="ce-2", task_id="task-1", stage_role="primary")
        with pytest.raises(ValueError, match="duplicate"):
            validate_correlated_error_records([ce1, ce2])

    def test_same_task_same_attempt_different_stage_role_passes(self):
        ce1 = _make_record(record_id="ce-1", task_id="task-1", stage_role="primary")
        ce2 = _make_record(record_id="ce-2", task_id="task-1", stage_role="assistant")
        ce3 = _make_record(record_id="ce-3", task_id="task-1", stage_role="light")
        validate_correlated_error_records([ce1, ce2, ce3])

    def test_same_task_different_attempt_passes(self):
        ce1 = _make_record(record_id="ce-1", attempt_id="att-1", task_id="task-1", stage_role="primary")
        ce2 = _make_record(record_id="ce-2", attempt_id="att-2", task_id="task-1", stage_role="primary")
        validate_correlated_error_records([ce1, ce2])

    def test_same_task_different_run_passes(self):
        ce1 = _make_record(record_id="ce-1", run_id="run-1", task_id="task-1", stage_role="primary")
        ce2 = _make_record(record_id="ce-2", run_id="run-2", task_id="task-1", stage_role="primary")
        validate_correlated_error_records([ce1, ce2])

    def test_empty_list_passes(self):
        validate_correlated_error_records([])


# --- Tier 1: Metric registry ---


class TestCorrelatedErrorMetrics:
    pytestmark = pytest.mark.unit

    _CORRELATED_METRIC_IDS = frozenset({
        "correlated_failure_rate",
        "failure_independence",
        "same_family_correlated_rate",
        "cross_family_correlated_rate",
    })

    _LOWER_IS_BETTER_METRICS = frozenset({
        "correlated_failure_rate",
        "same_family_correlated_rate",
        "cross_family_correlated_rate",
    })

    _HIGHER_IS_BETTER_METRICS = frozenset({
        "failure_independence",
    })

    def test_all_four_correlated_metrics_registered(self):
        for metric_id in self._CORRELATED_METRIC_IDS:
            assert DEFAULT_METRIC_REGISTRY.is_registered(metric_id, "1.0.0"), (
                f"metric {metric_id} is not registered"
            )

    def test_correlated_metrics_are_analysis_class(self):
        for metric_id in self._CORRELATED_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.grader_class == GraderClass.ANALYSIS, (
                f"metric {metric_id} has grader_class {definition.grader_class}, expected ANALYSIS"
            )

    def test_correlated_metrics_have_proportion_aggregation(self):
        for metric_id in self._CORRELATED_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.aggregation.value == "proportion", (
                f"metric {metric_id} has aggregation {definition.aggregation}, expected proportion"
            )

    def test_correlated_metrics_have_correlated_error_record_evidence(self):
        for metric_id in self._CORRELATED_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert "correlated_error_record" in definition.evidence_requirements, (
                f"metric {metric_id} does not require correlated_error_record evidence"
            )

    def test_correlated_metrics_have_non_inferiority_margin(self):
        for metric_id in self._CORRELATED_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.non_inferiority_margin is not None, (
                f"metric {metric_id} has no non-inferiority margin"
            )

    def test_correlated_metrics_in_release_set(self):
        release_ids = {m.metric_id for m in RELEASE_METRIC_SET.metrics}
        for metric_id in self._CORRELATED_METRIC_IDS:
            assert metric_id in release_ids, (
                f"metric {metric_id} is not in the release set"
            )

    def test_correlated_metrics_have_reliability_domain(self):
        domain_map = {m.metric_id: m.domain for m in RELEASE_METRIC_SET.metrics}
        for metric_id in self._CORRELATED_METRIC_IDS:
            assert metric_id in domain_map, (
                f"metric {metric_id} has no domain mapping"
            )
            assert domain_map[metric_id] == MetricDomain.RELIABILITY, (
                f"metric {metric_id} has domain {domain_map[metric_id]}, expected RELIABILITY"
            )

    def test_lower_is_better_metrics_have_lower_is_better_direction(self):
        for metric_id in self._LOWER_IS_BETTER_METRICS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.direction == MetricDirection.LOWER_IS_BETTER, (
                f"metric {metric_id} has direction {definition.direction}, expected LOWER_IS_BETTER"
            )

    def test_higher_is_better_metrics_have_higher_is_better_direction(self):
        for metric_id in self._HIGHER_IS_BETTER_METRICS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.direction == MetricDirection.HIGHER_IS_BETTER, (
                f"metric {metric_id} has direction {definition.direction}, expected HIGHER_IS_BETTER"
            )


# --- Tier 1: Derived producers ---


class TestCorrelatedErrorDerivedProducers:
    pytestmark = pytest.mark.unit

    def test_correlated_failure_rate_single_stage_failure_is_zero(self):
        """A single-stage failure is not a correlated failure."""
        record = _make_analysis_record([
            _make_record(record_id="ce-1", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM),
        ])
        results = produce_correlated_failure_rate_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0
        assert results[0].denominator_contribution == 1

    def test_correlated_failure_rate_two_stages_same_error_is_one(self):
        """Two stages with the same error class is a correlated failure."""
        record = _make_analysis_record([
            _make_record(record_id="ce-1", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM),
            _make_record(record_id="ce-2", stage_role="assistant", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM),
        ])
        results = produce_correlated_failure_rate_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0
        assert results[0].denominator_contribution == 1

    def test_correlated_failure_rate_two_stages_different_errors_is_zero(self):
        """Two stages with different error classes is not a correlated failure."""
        record = _make_analysis_record([
            _make_record(record_id="ce-1", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM),
            _make_record(record_id="ce-2", stage_role="assistant", error_class=ErrorClassLabel.WRONG_TOOL_SELECTED),
        ])
        results = produce_correlated_failure_rate_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0

    def test_correlated_failure_rate_three_stages_two_same_error_is_one(self):
        """Three stages where two share the same error class is a correlated failure."""
        record = _make_analysis_record([
            _make_record(record_id="ce-1", stage_role="primary", error_class=ErrorClassLabel.SCHEMA_VIOLATION),
            _make_record(record_id="ce-2", stage_role="assistant", error_class=ErrorClassLabel.SCHEMA_VIOLATION),
            _make_record(record_id="ce-3", stage_role="light", error_class=ErrorClassLabel.OTHER),
        ])
        results = produce_correlated_failure_rate_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_correlated_failure_rate_same_stage_different_errors_not_correlated(self):
        """Two records for the same stage with different errors are not cross-stage correlation."""
        record = _make_analysis_record([
            _make_record(record_id="ce-1", attempt_id="att-1", task_id="task-1", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM),
            _make_record(record_id="ce-2", attempt_id="att-1", task_id="task-1", stage_role="assistant", error_class=ErrorClassLabel.WRONG_TOOL_SELECTED),
        ])
        results = produce_correlated_failure_rate_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0

    def test_failure_independence_inverts_correlated_failure_rate(self):
        """failure_independence = 1 - correlated_failure_rate."""
        record = _make_analysis_record([
            _make_record(record_id="ce-1", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM),
            _make_record(record_id="ce-2", stage_role="assistant", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM),
        ])
        correlated = produce_correlated_failure_rate_observations(record)
        independence = produce_failure_independence_observations(record)
        assert len(correlated) == 1
        assert len(independence) == 1
        assert correlated[0].value == 1.0
        assert independence[0].value == 0.0

    def test_failure_independence_single_stage_failure_is_one(self):
        """A single-stage failure has full independence (no correlation)."""
        record = _make_analysis_record([
            _make_record(record_id="ce-1", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM),
        ])
        results = produce_failure_independence_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_same_family_correlated_rate_filters_homogeneous_only(self):
        """same_family_correlated_rate only considers homogeneous stacks."""
        record = _make_analysis_record([
            _make_record(record_id="ce-1", attempt_id="att-1", task_id="task-1", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM, stack_composition_type=StackCompositionType.HOMOGENEOUS),
            _make_record(record_id="ce-2", attempt_id="att-1", task_id="task-1", stage_role="assistant", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM, stack_composition_type=StackCompositionType.HOMOGENEOUS),
            _make_record(record_id="ce-3", attempt_id="att-2", task_id="task-2", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM, stack_composition_type=StackCompositionType.HETEROGENEOUS),
            _make_record(record_id="ce-4", attempt_id="att-2", task_id="task-2", stage_role="assistant", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM, stack_composition_type=StackCompositionType.HETEROGENEOUS),
        ], attempts=[
            _make_attempt(attempt_id="att-1", task_id="task-1"),
            _make_attempt(attempt_id="att-2", task_id="task-2"),
        ])
        results = produce_same_family_correlated_rate_observations(record)
        assert len(results) == 1
        assert results[0].attempt_id == "att-1"
        assert results[0].value == 1.0

    def test_cross_family_correlated_rate_filters_heterogeneous_only(self):
        """cross_family_correlated_rate only considers heterogeneous stacks."""
        record = _make_analysis_record([
            _make_record(record_id="ce-1", attempt_id="att-1", task_id="task-1", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM, stack_composition_type=StackCompositionType.HOMOGENEOUS),
            _make_record(record_id="ce-2", attempt_id="att-1", task_id="task-1", stage_role="assistant", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM, stack_composition_type=StackCompositionType.HOMOGENEOUS),
            _make_record(record_id="ce-3", attempt_id="att-2", task_id="task-2", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM, stack_composition_type=StackCompositionType.HETEROGENEOUS),
            _make_record(record_id="ce-4", attempt_id="att-2", task_id="task-2", stage_role="assistant", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM, stack_composition_type=StackCompositionType.HETEROGENEOUS),
        ], attempts=[
            _make_attempt(attempt_id="att-1", task_id="task-1"),
            _make_attempt(attempt_id="att-2", task_id="task-2"),
        ])
        results = produce_cross_family_correlated_rate_observations(record)
        assert len(results) == 1
        assert results[0].attempt_id == "att-2"
        assert results[0].value == 1.0

    def test_no_correlated_error_records_produces_no_observations(self):
        record = _make_analysis_record([])
        assert produce_correlated_failure_rate_observations(record) == []
        assert produce_failure_independence_observations(record) == []
        assert produce_same_family_correlated_rate_observations(record) == []
        assert produce_cross_family_correlated_rate_observations(record) == []

    def test_multiple_attempts_produce_separate_observations(self):
        record = _make_analysis_record([
            _make_record(record_id="ce-1", attempt_id="att-1", task_id="task-1", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM),
            _make_record(record_id="ce-2", attempt_id="att-1", task_id="task-1", stage_role="assistant", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM),
            _make_record(record_id="ce-3", attempt_id="att-2", task_id="task-2", stage_role="primary", error_class=ErrorClassLabel.WRONG_TOOL_SELECTED),
        ], attempts=[
            _make_attempt(attempt_id="att-1", task_id="task-1"),
            _make_attempt(attempt_id="att-2", task_id="task-2"),
        ])
        results = produce_correlated_failure_rate_observations(record)
        assert len(results) == 2
        att1_result = next(r for r in results if r.attempt_id == "att-1")
        att2_result = next(r for r in results if r.attempt_id == "att-2")
        assert att1_result.value == 1.0
        assert att2_result.value == 0.0

    def test_verified_records_produce_verified_observations(self):
        record = _make_analysis_record([
            _make_record(
                record_id="ce-1",
                stage_role="primary",
                error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM,
                verification_status=VerificationStatus.VERIFIED,
                source_evidence_refs=["ev-1"],
                source_evidence_sha256=_VALID_HASH,
            ),
            _make_record(
                record_id="ce-2",
                stage_role="assistant",
                error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM,
                verification_status=VerificationStatus.VERIFIED,
                source_evidence_refs=["ev-2"],
                source_evidence_sha256=_VALID_HASH,
            ),
        ])
        results = produce_correlated_failure_rate_observations(record)
        assert len(results) == 1
        assert results[0].verification_status == VerificationStatus.VERIFIED

    def test_pending_records_produce_pending_observations(self):
        record = _make_analysis_record([
            _make_record(
                record_id="ce-1",
                stage_role="primary",
                error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM,
                verification_status=VerificationStatus.PENDING,
            ),
            _make_record(
                record_id="ce-2",
                stage_role="assistant",
                error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM,
                verification_status=VerificationStatus.VERIFIED,
                source_evidence_refs=["ev-2"],
                source_evidence_sha256=_VALID_HASH,
            ),
        ])
        results = produce_correlated_failure_rate_observations(record)
        assert len(results) == 1
        assert results[0].verification_status == VerificationStatus.PENDING

    def test_evidence_refs_include_all_record_ids(self):
        record = _make_analysis_record([
            _make_record(record_id="ce-1", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM),
            _make_record(record_id="ce-2", stage_role="assistant", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM),
        ])
        results = produce_correlated_failure_rate_observations(record)
        assert len(results) == 1
        assert set(results[0].evidence_refs) == {"ce-1", "ce-2"}

    def test_same_family_rate_no_homogeneous_records_produces_no_observations(self):
        record = _make_analysis_record([
            _make_record(record_id="ce-1", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM, stack_composition_type=StackCompositionType.HETEROGENEOUS),
        ])
        results = produce_same_family_correlated_rate_observations(record)
        assert results == []

    def test_cross_family_rate_no_heterogeneous_records_produces_no_observations(self):
        record = _make_analysis_record([
            _make_record(record_id="ce-1", stage_role="primary", error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM, stack_composition_type=StackCompositionType.HOMOGENEOUS),
        ])
        results = produce_cross_family_correlated_rate_observations(record)
        assert results == []


# --- Tier 2: Campaign verifier integration ---


@dataclass
class _FakeSUT:
    model_id: str
    answer: str = "This is a test answer with no commas."

    async def get_answer(self, task: Task) -> Response:
        return Response(answer=self.answer, model=self.model_id, arm=Arm.DIRECT)


@dataclass
class _FakeGrader:
    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        return Score(task_id=task.id, passed=True, details=ScoreDetails())


def _make_spec() -> CampaignSpec:
    cohort = ModelCohort(
        cohort_id=_COHORT_ID,
        role_bindings=[RoleModelBinding(
            role="primary",
            model_id="qwen3:8b",
            provider="ollama",
            endpoint="http://192.168.1.2:11434",
            sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
            timeout_seconds=120.0,
            seed_capable=True,
        )],
        content_hash=compute_model_cohort_hash(_COHORT_ID, [RoleModelBinding(
            role="primary",
            model_id="qwen3:8b",
            provider="ollama",
            endpoint="http://192.168.1.2:11434",
            sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
            timeout_seconds=120.0,
            seed_capable=True,
        )]),
    )
    task_assignment = TaskAssignmentManifest(
        task_assignment_id="task-assignment-v1",
        suite_id="ifeval_subset",
        dataset_hash=_DATASET_HASH,
        task_ids=_TASK_IDS,
        content_hash=compute_task_assignment_hash("task-assignment-v1", "ifeval_subset", _DATASET_HASH, _TASK_IDS),
    )
    initial_state = InitialStateAssignmentManifest(
        initial_state_assignment_id=_INITIAL_STATE_ID,
        state_type="no_initial_state",
        snapshot_hash=_NO_STATE_HASH,
        content_hash=compute_initial_state_hash(_INITIAL_STATE_ID, "no_initial_state", _NO_STATE_HASH),
    )
    prereg = PreregistrationConfig(
        config_id="test-config-1",
        config_version="1.0.0",
        baseline_arm_id="direct",
        comparison_arm_ids=["ensemble_ungoverned"],
        model_cohort_ids=[_COHORT_ID],
        task_assignment_id="task-assignment-v1",
        initial_state_assignment_id=_INITIAL_STATE_ID,
        required_replicate_ids=_REPLICATE_IDS,
        required_replicate_count=1,
        primary_metric_ids=["ifeval_subset_verifier"],
        continuous_test_policy=ContinuousTestPolicy.PAIRED_T,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=0,
        significance_level=0.05,
        claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY,
    )
    prompt_bundle = "\n".join(f"Prompt for {tid}" for tid in _TASK_IDS).encode()
    return CampaignSpec(
        campaign_id=_CAMPAIGN_ID,
        release_version=_RELEASE_VERSION,
        suite="ifeval_subset",
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        dataset_hash=_DATASET_HASH,
        prompt_bundle_hash=hashlib.sha256(prompt_bundle).hexdigest(),
        grader_bundle_hash="g" * 64,
        preregistration=prereg,
        cohorts=[cohort],
        task_assignment=task_assignment,
        initial_state=initial_state,
        retry_policy=RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"]),
        randomization_seed=42,
    )


def _make_tasks() -> list[Task]:
    tasks = []
    for tid in _TASK_IDS:
        tasks.append(Task(
            id=tid,
            prompt=f"Prompt for {tid}",
            metadata=TaskMetadata(
                benchmark="ifeval_subset",
                instruction_id_list=["punctuation:no_comma"],
                kwargs=[{"no_comma": True}],
            ),
        ))
    return tasks


def _fake_sut_factory(cohort: ModelCohort, arm: Arm):
    return _FakeSUT(model_id=cohort.role_bindings[0].model_id)


def _run_campaign(tmp_path: Path) -> Path:
    spec = _make_spec()
    runner = CampaignRunner(
        spec=spec,
        sut_factory=_fake_sut_factory,
        tasks=_make_tasks(),
        grader=_FakeGrader(),
        output_dir=tmp_path,
    )
    result = asyncio.run(runner.run())
    return result.report_dir


def _first_attempt_id(report_dir: Path) -> str:
    """Read the first attempt_id from the attempts JSONL file."""
    from g8e_evals.constants import ATTEMPTS_JSONL
    attempts_path = report_dir / ATTEMPTS_JSONL
    lines = attempts_path.read_text().strip().splitlines()
    if not lines:
        pytest.skip("no attempts in campaign")
    first = json.loads(lines[0])
    return first["attempt_id"]


class TestCampaignVerifierCorrelatedErrorsLayer:
    pytestmark = pytest.mark.integration

    def test_verifier_has_correlated_errors_layer(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert "correlated_errors" in result.checked_layers

    def test_verifier_passes_without_correlated_errors(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert result.ok, f"campaign without correlated errors should pass: {result.failures}"
        assert "correlated_errors" in result.checked_layers

    def test_verifier_validates_correlated_errors_when_present(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        attempt_id = _first_attempt_id(report_dir)
        ce_path = report_dir / CORRELATED_ERRORS_JSONL
        ce_data = _make_record_dict(attempt_id=attempt_id)
        ce_path.write_text(json.dumps(ce_data) + "\n")
        result = verify_campaign(report_dir)
        assert result.ok, f"valid correlated error record should pass: {result.failures}"

    def test_verifier_rejects_malformed_correlated_errors(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ce_path = report_dir / CORRELATED_ERRORS_JSONL
        bad_ce = _make_record_dict()
        del bad_ce["run_id"]
        ce_path.write_text(json.dumps(bad_ce) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("correlated error record" in f.lower() for f in result.failures)

    def test_verifier_rejects_duplicate_correlated_errors(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        attempt_id = _first_attempt_id(report_dir)
        ce_path = report_dir / CORRELATED_ERRORS_JSONL
        ce_data = _make_record_dict(attempt_id=attempt_id)
        ce_path.write_text(json.dumps(ce_data) + "\n" + json.dumps(ce_data) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("correlated error record" in f.lower() and "duplicate" in f.lower() for f in result.failures)

    def test_verifier_rejects_correlated_error_bound_to_unknown_attempt(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ce_path = report_dir / CORRELATED_ERRORS_JSONL
        ce_data = _make_record_dict(attempt_id="nonexistent-attempt")
        ce_path.write_text(json.dumps(ce_data) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("unknown attempt" in f.lower() for f in result.failures)

    def test_verifier_rejects_symlinked_correlated_errors(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        attempt_id = _first_attempt_id(report_dir)
        ce_path = report_dir / CORRELATED_ERRORS_JSONL
        target = report_dir / "real-correlated-errors.jsonl"
        target.write_text(json.dumps(_make_record_dict(attempt_id=attempt_id)) + "\n")
        ce_path.symlink_to(target)
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("symlink" in f.lower() and "correlated" in f.lower() for f in result.failures)


# --- Helpers ---


def _make_record(
    *,
    record_id: str = "ce-1",
    attempt_id: str = "att-1",
    run_id: str = "run-1",
    task_id: str = "task-1",
    agent_persona: str = "sage",
    model_variant_id: str = "qwen3-8b-q4_0",
    stage_role: str = "primary",
    error_class: ErrorClassLabel = ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM,
    stack_id: str = "stack-best-accuracy",
    stack_composition_type: StackCompositionType = StackCompositionType.HETEROGENEOUS,
    source_evidence_refs: list[str] | None = None,
    source_evidence_sha256: str | None = None,
    verification_status: VerificationStatus = VerificationStatus.PENDING,
) -> CorrelatedErrorRecord:
    return CorrelatedErrorRecord(
        record_id=record_id,
        attempt_id=attempt_id,
        run_id=run_id,
        task_id=task_id,
        agent_persona=agent_persona,
        model_variant_id=model_variant_id,
        stage_role=stage_role,
        error_class=error_class,
        stack_id=stack_id,
        stack_composition_type=stack_composition_type,
        source_evidence_refs=source_evidence_refs or [],
        source_evidence_sha256=source_evidence_sha256,
        verification_status=verification_status,
    )


def _make_record_dict(
    *,
    attempt_id: str = "att-1",
    run_id: str = "run-1",
    task_id: str = "task-1",
    record_id: str = "ce-1",
) -> dict:
    ce = _make_record(
        record_id=record_id,
        attempt_id=attempt_id,
        run_id=run_id,
        task_id=task_id,
    )
    return json.loads(ce.model_dump_json())


def _make_attempt(
    attempt_id: str = "att-1",
    task_id: str = "task-1",
    arm_id: Arm = Arm.DIRECT,
) -> AttemptRecord:
    return AttemptRecord(
        attempt_id=attempt_id,
        run_id="run-1",
        task_id=task_id,
        arm_id=arm_id,
    )


def _make_analysis_record(
    correlated_error_records: list[CorrelatedErrorRecord],
    attempts: list[AttemptRecord] | None = None,
) -> AnalysisInputRecord:
    return AnalysisInputRecord(
        run_id="run-1",
        release_version="v2.1.8",
        tasks=[],
        attempts=attempts or [_make_attempt()],
        correlated_error_records=correlated_error_records,
    )
