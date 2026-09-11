# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 and Tier 2 tests for the EF5 security and privacy event recording.

Verifies that:
- ``SecurityEventRecord`` is frozen with ``extra="forbid"`` and carries all
  required bindings (agent persona, model variant, governance layer,
  optional tool call, 11 boolean event fields).
- ``SecurityEventType`` enumerates exactly the 11 event types.
- ``validate_security_event_records`` rejects duplicate (run, attempt, task)
  tuples.
- Verified records require source evidence.
- The campaign verifier has a ``security_events`` layer that validates
  records when present, passes when absent, rejects malformed records,
  rejects duplicates, rejects symlinks, and rejects records bound to
  unknown attempts.
- The 11 security metrics are registered in the metric registry with
  correct grader class, direction, aggregation, evidence requirements,
  non-inferiority margins, and release domain.
- The derived producers compute correct proportions from
  ``SecurityEventRecord`` records.
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
    produce_security_authorization_correctly_enforced_observations,
    produce_security_audit_record_complete_observations,
    produce_security_audit_record_tampered_observations,
    produce_security_model_attempted_unauthorized_access_observations,
    produce_security_policy_prevented_disclosure_observations,
    produce_security_secret_redaction_successful_observations,
    produce_security_sensitive_data_present_observations,
    produce_security_sensitive_data_required_observations,
    produce_security_sensitive_data_sent_externally_observations,
    produce_security_tool_attempted_unauthorized_operation_observations,
    produce_security_unnecessary_data_sent_externally_observations,
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
from g8e_evals.constants import SECURITY_EVENTS_JSONL
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
    SecurityEventRecord,
    SecurityEventType,
    VerificationStatus,
    validate_security_event_records,
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


class TestSecurityEventRecordModel:
    pytestmark = pytest.mark.unit

    def test_record_is_frozen(self):
        se = _make_record()
        with pytest.raises((TypeError, ValueError)):
            se.sensitive_data_present = False  # type: ignore[misc]

    def test_record_rejects_extra_fields(self):
        with pytest.raises(ValidationError):
            SecurityEventRecord(
                record_id="se-1",
                campaign_id="campaign-1",
                child_id="campaign-1",
                assignment_id="assignment-1",
                attempt_id="att-1",
                run_id="run-1",
                task_id="task-1",
                role="primary",
                agent_persona="warden",
                model_variant_id="qwen3-8b-q4_0",
                governance_layer="policy",
                sensitive_data_present=True,
                sensitive_data_required=False,
                sensitive_data_sent_externally=False,
                unnecessary_data_sent_externally=False,
                policy_prevented_disclosure=False,
                model_attempted_unauthorized_access=False,
                tool_attempted_unauthorized_operation=False,
                authorization_correctly_enforced=True,
                audit_record_complete=True,
                audit_record_tampered=False,
                secret_redaction_successful=True,
                extra_field="bad",
            )

    def test_record_has_all_required_fields(self):
        se = _make_record()
        assert se.record_id == "se-1"
        assert se.attempt_id == "att-1"
        assert se.run_id == "run-1"
        assert se.task_id == "task-1"
        assert se.agent_persona == "warden"
        assert se.model_variant_id == "qwen3-8b-q4_0"
        assert se.governance_layer == "policy"
        assert se.tool_call_id is None

    def test_event_type_enum_has_exactly_eleven_values(self):
        assert len(list(SecurityEventType)) == 11
        values = {t.value for t in SecurityEventType}
        assert values == {
            "sensitive_data_present",
            "sensitive_data_required",
            "sensitive_data_sent_externally",
            "unnecessary_data_sent_externally",
            "policy_prevented_disclosure",
            "model_attempted_unauthorized_access",
            "tool_attempted_unauthorized_operation",
            "authorization_correctly_enforced",
            "audit_record_complete",
            "audit_record_tampered",
            "secret_redaction_successful",
        }

    def test_record_has_eleven_boolean_event_fields(self):
        se = _make_record()
        assert isinstance(se.sensitive_data_present, bool)
        assert isinstance(se.sensitive_data_required, bool)
        assert isinstance(se.sensitive_data_sent_externally, bool)
        assert isinstance(se.unnecessary_data_sent_externally, bool)
        assert isinstance(se.policy_prevented_disclosure, bool)
        assert isinstance(se.model_attempted_unauthorized_access, bool)
        assert isinstance(se.tool_attempted_unauthorized_operation, bool)
        assert isinstance(se.authorization_correctly_enforced, bool)
        assert isinstance(se.audit_record_complete, bool)
        assert isinstance(se.audit_record_tampered, bool)
        assert isinstance(se.secret_redaction_successful, bool)

    def test_record_binds_persona_model_layer_task(self):
        se = _make_record(
            agent_persona="auditor",
            model_variant_id="granite-8b-q4_0",
            governance_layer="audit",
            task_id="SEC-002",
        )
        assert se.agent_persona == "auditor"
        assert se.model_variant_id == "granite-8b-q4_0"
        assert se.governance_layer == "audit"
        assert se.task_id == "SEC-002"

    def test_tool_call_id_is_optional(self):
        se = _make_record(tool_call_id="tc-001")
        assert se.tool_call_id == "tc-001"
        se2 = _make_record()
        assert se2.tool_call_id is None

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

    def test_empty_governance_layer_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(governance_layer="")

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
        se = _make_record(
            verification_status=VerificationStatus.VERIFIED,
            source_evidence_refs=["ev-1"],
            source_evidence_sha256=_VALID_HASH,
        )
        assert se.verification_status == VerificationStatus.VERIFIED

    def test_duplicate_evidence_refs_rejected(self):
        with pytest.raises(ValidationError, match="unique"):
            _make_record(source_evidence_refs=["ev-1", "ev-1"])

    def test_invalid_evidence_sha256_rejected(self):
        with pytest.raises(ValidationError):
            _make_record(source_evidence_sha256="not-a-hash")

    def test_pending_record_without_evidence_passes(self):
        se = _make_record()
        assert se.verification_status == VerificationStatus.PENDING
        assert se.source_evidence_refs == []
        assert se.source_evidence_sha256 is None

    def test_record_serialization_roundtrip(self):
        se = _make_record()
        data = json.loads(se.model_dump_json())
        assert data["record_id"] == "se-1"
        assert data["sensitive_data_present"] is True
        restored = SecurityEventRecord.model_validate(data)
        assert restored == se

    def test_events_property_returns_all_eleven_fields(self):
        se = _make_record()
        events = se.events
        assert len(events) == 11
        assert set(events.keys()) == {
            "sensitive_data_present",
            "sensitive_data_required",
            "sensitive_data_sent_externally",
            "unnecessary_data_sent_externally",
            "policy_prevented_disclosure",
            "model_attempted_unauthorized_access",
            "tool_attempted_unauthorized_operation",
            "authorization_correctly_enforced",
            "audit_record_complete",
            "audit_record_tampered",
            "secret_redaction_successful",
        }
        assert all(isinstance(v, bool) for v in events.values())


class TestValidateSecurityEventRecords:
    pytestmark = pytest.mark.unit

    def test_valid_records_pass(self):
        se1 = _make_record(record_id="se-1", task_id="task-1")
        se2 = _make_record(record_id="se-2", task_id="task-2")
        validate_security_event_records([se1, se2])

    def test_duplicate_run_attempt_task_rejected(self):
        se1 = _make_record(record_id="se-1", task_id="task-1")
        se2 = _make_record(record_id="se-2", task_id="task-1")
        with pytest.raises(ValueError, match="duplicate"):
            validate_security_event_records([se1, se2])

    def test_same_task_different_attempt_passes(self):
        se1 = _make_record(record_id="se-1", attempt_id="att-1", task_id="task-1")
        se2 = _make_record(record_id="se-2", attempt_id="att-2", task_id="task-1")
        validate_security_event_records([se1, se2])

    def test_same_task_different_run_passes(self):
        se1 = _make_record(record_id="se-1", run_id="run-1", task_id="task-1")
        se2 = _make_record(record_id="se-2", run_id="run-2", task_id="task-1")
        validate_security_event_records([se1, se2])

    def test_empty_list_passes(self):
        validate_security_event_records([])


# --- Tier 1: Metric registry ---


class TestSecurityMetrics:
    pytestmark = pytest.mark.unit

    _SECURITY_METRIC_IDS = frozenset({
        "security_sensitive_data_present",
        "security_sensitive_data_required",
        "security_sensitive_data_sent_externally",
        "security_unnecessary_data_sent_externally",
        "security_policy_prevented_disclosure",
        "security_model_attempted_unauthorized_access",
        "security_tool_attempted_unauthorized_operation",
        "security_authorization_correctly_enforced",
        "security_audit_record_complete",
        "security_audit_record_tampered",
        "security_secret_redaction_successful",
    })

    _GOVERNANCE_METRICS = frozenset({
        "security_policy_prevented_disclosure",
        "security_model_attempted_unauthorized_access",
        "security_tool_attempted_unauthorized_operation",
        "security_authorization_correctly_enforced",
        "security_audit_record_complete",
        "security_audit_record_tampered",
    })

    _PRIVACY_METRICS = frozenset({
        "security_sensitive_data_present",
        "security_sensitive_data_required",
        "security_sensitive_data_sent_externally",
        "security_unnecessary_data_sent_externally",
        "security_secret_redaction_successful",
    })

    _NEUTRAL_METRICS = frozenset({
        "security_sensitive_data_present",
        "security_sensitive_data_required",
    })

    _LOWER_IS_BETTER_METRICS = frozenset({
        "security_sensitive_data_sent_externally",
        "security_unnecessary_data_sent_externally",
        "security_model_attempted_unauthorized_access",
        "security_tool_attempted_unauthorized_operation",
        "security_audit_record_tampered",
    })

    _HIGHER_IS_BETTER_METRICS = frozenset({
        "security_policy_prevented_disclosure",
        "security_authorization_correctly_enforced",
        "security_audit_record_complete",
        "security_secret_redaction_successful",
    })

    def test_all_eleven_security_metrics_registered(self):
        for metric_id in self._SECURITY_METRIC_IDS:
            assert DEFAULT_METRIC_REGISTRY.is_registered(metric_id, "1.0.0"), (
                f"metric {metric_id} is not registered"
            )

    def test_security_metrics_are_analysis_class(self):
        for metric_id in self._SECURITY_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.grader_class == GraderClass.ANALYSIS, (
                f"metric {metric_id} has grader_class {definition.grader_class}, expected ANALYSIS"
            )

    def test_security_metrics_have_proportion_aggregation(self):
        for metric_id in self._SECURITY_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.aggregation.value == "proportion", (
                f"metric {metric_id} has aggregation {definition.aggregation}, expected proportion"
            )

    def test_security_metrics_have_security_event_record_evidence(self):
        for metric_id in self._SECURITY_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert "security_event_record" in definition.evidence_requirements, (
                f"metric {metric_id} does not require security_event_record evidence"
            )

    def test_security_metrics_have_non_inferiority_margin(self):
        for metric_id in self._SECURITY_METRIC_IDS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.non_inferiority_margin is not None, (
                f"metric {metric_id} has no non-inferiority margin"
            )

    def test_security_metrics_in_release_set(self):
        release_ids = {m.metric_id for m in RELEASE_METRIC_SET.metrics}
        for metric_id in self._SECURITY_METRIC_IDS:
            assert metric_id in release_ids, (
                f"metric {metric_id} is not in the release set"
            )

    def test_security_metrics_have_domain_mapping(self):
        domain_map = {m.metric_id: m.domain for m in RELEASE_METRIC_SET.metrics}
        for metric_id in self._SECURITY_METRIC_IDS:
            assert metric_id in domain_map, (
                f"metric {metric_id} has no domain mapping"
            )
        for metric_id in self._GOVERNANCE_METRICS:
            assert domain_map[metric_id] == MetricDomain.GOVERNANCE, (
                f"metric {metric_id} has domain {domain_map[metric_id]}, expected GOVERNANCE"
            )
        for metric_id in self._PRIVACY_METRICS:
            assert domain_map[metric_id] == MetricDomain.PRIVACY, (
                f"metric {metric_id} has domain {domain_map[metric_id]}, expected PRIVACY"
            )

    def test_neutral_metrics_have_neutral_direction(self):
        for metric_id in self._NEUTRAL_METRICS:
            definition = DEFAULT_METRIC_REGISTRY.get(metric_id, "1.0.0")
            assert definition.direction == MetricDirection.NEUTRAL, (
                f"metric {metric_id} has direction {definition.direction}, expected NEUTRAL"
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


class TestSecurityDerivedProducers:
    pytestmark = pytest.mark.unit

    def test_sensitive_data_present_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="se-1", sensitive_data_present=True),
            _make_record(record_id="se-2", sensitive_data_present=False),
        ])
        results = produce_security_sensitive_data_present_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.5
        assert results[0].denominator_contribution == 2

    def test_sensitive_data_required_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="se-1", sensitive_data_required=True),
            _make_record(record_id="se-2", sensitive_data_required=True),
            _make_record(record_id="se-3", sensitive_data_required=False),
        ])
        results = produce_security_sensitive_data_required_observations(record)
        assert len(results) == 1
        assert results[0].value == round(2 / 3, 10)
        assert results[0].denominator_contribution == 3

    def test_sensitive_data_sent_externally_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="se-1", sensitive_data_sent_externally=False),
            _make_record(record_id="se-2", sensitive_data_sent_externally=True),
        ])
        results = produce_security_sensitive_data_sent_externally_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.5
        assert results[0].denominator_contribution == 2

    def test_unnecessary_data_sent_externally_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="se-1", unnecessary_data_sent_externally=False),
            _make_record(record_id="se-2", unnecessary_data_sent_externally=False),
        ])
        results = produce_security_unnecessary_data_sent_externally_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0
        assert results[0].denominator_contribution == 2

    def test_policy_prevented_disclosure_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="se-1", policy_prevented_disclosure=True),
            _make_record(record_id="se-2", policy_prevented_disclosure=True),
        ])
        results = produce_security_policy_prevented_disclosure_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0
        assert results[0].denominator_contribution == 2

    def test_model_attempted_unauthorized_access_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="se-1", model_attempted_unauthorized_access=True),
            _make_record(record_id="se-2", model_attempted_unauthorized_access=False),
        ])
        results = produce_security_model_attempted_unauthorized_access_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.5
        assert results[0].denominator_contribution == 2

    def test_tool_attempted_unauthorized_operation_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="se-1", tool_attempted_unauthorized_operation=False),
            _make_record(record_id="se-2", tool_attempted_unauthorized_operation=False),
        ])
        results = produce_security_tool_attempted_unauthorized_operation_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0
        assert results[0].denominator_contribution == 2

    def test_authorization_correctly_enforced_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="se-1", authorization_correctly_enforced=True),
            _make_record(record_id="se-2", authorization_correctly_enforced=True),
            _make_record(record_id="se-3", authorization_correctly_enforced=False),
        ])
        results = produce_security_authorization_correctly_enforced_observations(record)
        assert len(results) == 1
        assert results[0].value == round(2 / 3, 10)
        assert results[0].denominator_contribution == 3

    def test_audit_record_complete_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="se-1", audit_record_complete=True),
            _make_record(record_id="se-2", audit_record_complete=False),
        ])
        results = produce_security_audit_record_complete_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.5
        assert results[0].denominator_contribution == 2

    def test_audit_record_tampered_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="se-1", audit_record_tampered=False),
            _make_record(record_id="se-2", audit_record_tampered=True),
        ])
        results = produce_security_audit_record_tampered_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.5
        assert results[0].denominator_contribution == 2

    def test_secret_redaction_successful_producer_computes_proportion(self):
        record = _make_analysis_record([
            _make_record(record_id="se-1", secret_redaction_successful=True),
            _make_record(record_id="se-2", secret_redaction_successful=True),
        ])
        results = produce_security_secret_redaction_successful_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0
        assert results[0].denominator_contribution == 2

    def test_no_security_records_produces_no_observations(self):
        record = _make_analysis_record([])
        results = produce_security_sensitive_data_present_observations(record)
        assert results == []
        results = produce_security_secret_redaction_successful_observations(record)
        assert results == []

    def test_multiple_attempts_produce_separate_observations(self):
        record = _make_analysis_record([
            _make_record(record_id="se-1", attempt_id="att-1", task_id="task-1", sensitive_data_present=True),
            _make_record(record_id="se-2", attempt_id="att-2", task_id="task-2", sensitive_data_present=False),
        ], attempts=[
            _make_attempt(attempt_id="att-1", task_id="task-1"),
            _make_attempt(attempt_id="att-2", task_id="task-2"),
        ])
        results = produce_security_sensitive_data_present_observations(record)
        assert len(results) == 2
        att1_result = next(r for r in results if r.attempt_id == "att-1")
        att2_result = next(r for r in results if r.attempt_id == "att-2")
        assert att1_result.value == 1.0
        assert att2_result.value == 0.0

    def test_verified_records_produce_verified_observations(self):
        record = _make_analysis_record([
            _make_record(
                record_id="se-1",
                sensitive_data_present=True,
                verification_status=VerificationStatus.VERIFIED,
                source_evidence_refs=["ev-1"],
                source_evidence_sha256=_VALID_HASH,
            ),
        ])
        results = produce_security_sensitive_data_present_observations(record)
        assert len(results) == 1
        assert results[0].verification_status == VerificationStatus.VERIFIED

    def test_pending_records_produce_pending_observations(self):
        record = _make_analysis_record([
            _make_record(
                record_id="se-1",
                sensitive_data_present=True,
                verification_status=VerificationStatus.PENDING,
            ),
        ])
        results = produce_security_sensitive_data_present_observations(record)
        assert len(results) == 1
        assert results[0].verification_status == VerificationStatus.PENDING


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


def _campaign_identity(report_dir: Path) -> dict:
    """Read the actual campaign identity from the report directory."""
    from g8e_evals.constants import ATTEMPTS_JSONL, CAMPAIGN_MANIFEST_JSON
    cm = json.loads((report_dir / CAMPAIGN_MANIFEST_JSON).read_text())
    attempts_path = report_dir / ATTEMPTS_JSONL
    lines = attempts_path.read_text().strip().splitlines()
    if not lines:
        pytest.skip("no attempts in campaign")
    first = json.loads(lines[0])
    return {
        "campaign_id": cm["campaign_id"],
        "child_id": cm["campaign_id"],
        "run_id": first["run_id"],
        "task_id": first["task_id"],
        "assignment_id": first.get("assignment_id", ""),
        "attempt_id": first["attempt_id"],
    }


class TestCampaignVerifierSecurityEventsLayer:
    pytestmark = pytest.mark.integration

    def test_verifier_has_security_events_layer(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert "security_events" in result.checked_layers

    def test_verifier_passes_without_security_events(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        result = verify_campaign(report_dir)
        assert result.ok, f"campaign without security events should pass: {result.failures}"
        assert "security_events" in result.checked_layers

    def test_verifier_validates_security_events_when_present(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        se_path = report_dir / SECURITY_EVENTS_JSONL
        se_data = _make_record_dict(**ident)
        se_path.write_text(json.dumps(se_data) + "\n")
        result = verify_campaign(report_dir)
        assert result.ok, f"valid security event record should pass: {result.failures}"

    def test_verifier_rejects_malformed_security_events(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        se_path = report_dir / SECURITY_EVENTS_JSONL
        bad_se = _make_record_dict()
        del bad_se["run_id"]
        se_path.write_text(json.dumps(bad_se) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("security event record" in f.lower() for f in result.failures)

    def test_verifier_rejects_duplicate_security_events(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        se_path = report_dir / SECURITY_EVENTS_JSONL
        se_data = _make_record_dict(**ident)
        se_path.write_text(json.dumps(se_data) + "\n" + json.dumps(se_data) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("security event record" in f.lower() and "duplicate" in f.lower() for f in result.failures)

    def test_verifier_rejects_security_event_bound_to_unknown_attempt(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        se_path = report_dir / SECURITY_EVENTS_JSONL
        se_data = _make_record_dict(attempt_id="nonexistent-attempt")
        se_path.write_text(json.dumps(se_data) + "\n")
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("unknown attempt" in f.lower() for f in result.failures)

    def test_verifier_rejects_symlinked_security_events(self, tmp_path: Path):
        report_dir = _run_campaign(tmp_path)
        ident = _campaign_identity(report_dir)
        se_path = report_dir / SECURITY_EVENTS_JSONL
        target = report_dir / "real-security-events.jsonl"
        target.write_text(json.dumps(_make_record_dict(**ident)) + "\n")
        se_path.symlink_to(target)
        result = verify_campaign(report_dir)
        assert not result.ok
        assert any("symlink" in f.lower() and "security" in f.lower() for f in result.failures)


# --- Helpers ---


def _make_record(
    *,
    record_id: str = "se-1",
    campaign_id: str = "campaign-1",
    child_id: str = "campaign-1",
    assignment_id: str = "assignment-1",
    attempt_id: str = "att-1",
    inference_id: str | None = None,
    run_id: str = "run-1",
    task_id: str = "task-1",
    role: str = "primary",
    agent_persona: str = "warden",
    model_variant_id: str = "qwen3-8b-q4_0",
    governance_layer: str = "policy",
    tool_call_id: str | None = None,
    sensitive_data_present: bool = True,
    sensitive_data_required: bool = False,
    sensitive_data_sent_externally: bool = False,
    unnecessary_data_sent_externally: bool = False,
    policy_prevented_disclosure: bool = False,
    model_attempted_unauthorized_access: bool = False,
    tool_attempted_unauthorized_operation: bool = False,
    authorization_correctly_enforced: bool = True,
    audit_record_complete: bool = True,
    audit_record_tampered: bool = False,
    secret_redaction_successful: bool = True,
    source_evidence_refs: list[str] | None = None,
    source_evidence_sha256: str | None = None,
    verification_status: VerificationStatus = VerificationStatus.PENDING,
) -> SecurityEventRecord:
    return SecurityEventRecord(
        record_id=record_id,
        campaign_id=campaign_id,
        child_id=child_id,
        assignment_id=assignment_id,
        attempt_id=attempt_id,
        inference_id=inference_id,
        run_id=run_id,
        task_id=task_id,
        role=role,
        agent_persona=agent_persona,
        model_variant_id=model_variant_id,
        governance_layer=governance_layer,
        tool_call_id=tool_call_id,
        sensitive_data_present=sensitive_data_present,
        sensitive_data_required=sensitive_data_required,
        sensitive_data_sent_externally=sensitive_data_sent_externally,
        unnecessary_data_sent_externally=unnecessary_data_sent_externally,
        policy_prevented_disclosure=policy_prevented_disclosure,
        model_attempted_unauthorized_access=model_attempted_unauthorized_access,
        tool_attempted_unauthorized_operation=tool_attempted_unauthorized_operation,
        authorization_correctly_enforced=authorization_correctly_enforced,
        audit_record_complete=audit_record_complete,
        audit_record_tampered=audit_record_tampered,
        secret_redaction_successful=secret_redaction_successful,
        source_evidence_refs=source_evidence_refs or [],
        source_evidence_sha256=source_evidence_sha256,
        verification_status=verification_status,
    )


def _make_record_dict(
    *,
    attempt_id: str = "att-1",
    run_id: str = "run-1",
    task_id: str = "task-1",
    campaign_id: str = "campaign-1",
    child_id: str = "campaign-1",
    assignment_id: str = "assignment-1",
    record_id: str = "se-1",
) -> dict:
    se = _make_record(
        record_id=record_id,
        campaign_id=campaign_id,
        child_id=child_id,
        assignment_id=assignment_id,
        attempt_id=attempt_id,
        run_id=run_id,
        task_id=task_id,
    )
    return json.loads(se.model_dump_json())


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
    security_events: list[SecurityEventRecord],
    attempts: list[AttemptRecord] | None = None,
) -> AnalysisInputRecord:
    return AnalysisInputRecord(
        run_id="run-1",
        release_version="v2.1.8",
        tasks=[],
        attempts=attempts or [_make_attempt()],
        security_events=security_events,
    )
