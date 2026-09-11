# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for observation and event record schema gaps.

Verifies that the extended schemas close the identity, measurement
availability, and binding gaps identified in the Phase 0 Beacon
observation audit:

- ``ResourceObservation`` has campaign, child/report, assignment,
  attempt, inference, role, task, orchestrator scope, provider scope,
  observation boundary, and clock domain fields.
- ``ResourceObservation`` distinguishes unavailable from measured zero
  using typed ``unavailable_measurements`` entries.
- ``ResourceObservation`` supports multiple observations per attempt
  (one per inference).
- Event records (``ToolCallScorecard``, ``SecurityEventRecord``,
  ``CorrelatedErrorRecord``) have campaign, child/report, assignment,
  and inference bindings.
- ``SecurityEventRecord.governance_layer`` is a typed enum, not a
  free-form string.
- ``AnalysisInputRecord`` has a ``resource_observations`` field that
  accepts ``ResourceObservation`` instances from ``index.py``.
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with extra fields to verify
# validation rejects them.

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.index import (
    MeasurementAvailability,
    MeasurementScope,
    ModelRole,
    ResourceObservation,
    UnavailableMeasurement,
)
from g8e_evals.schema import (
    CorrelatedErrorRecord,
    EscalationRecord,
    GovernanceLayer,
    SecurityEventRecord,
    ToolCallScorecard,
)


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64


def _unavailable(field_name: str) -> UnavailableMeasurement:
    return UnavailableMeasurement(
        field_name=field_name,
        availability=MeasurementAvailability.UNAVAILABLE,
        scope=MeasurementScope.PROVIDER_REMOTE,
        reason=f"{field_name} not available",
    )


_MEASUREMENT_FIELDS = (
    "model_load_time_seconds",
    "peak_resident_memory_bytes",
    "peak_accelerator_memory_bytes",
    "artifact_bytes",
    "measured_energy_joules",
    "end_to_end_latency_seconds",
    "provider_call_latency_seconds",
    "output_throughput_tokens_per_second",
    "hidden_reasoning_throughput_tokens_per_second",
    "time_to_first_token_seconds",
    "generation_duration_seconds",
    "accelerator_memory_before_bytes",
    "gpu_utilization_percent",
    "gpu_temperature_celsius",
    "gpu_power_draw_watts",
    "gpu_clock_mhz",
)


def _all_unavailable() -> list[UnavailableMeasurement]:
    return [_unavailable(f) for f in _MEASUREMENT_FIELDS]


def _make_observation(**kwargs) -> ResourceObservation:
    defaults = {
        "campaign_id": "campaign-1",
        "child_id": "campaign-1",
        "run_id": "run-1",
        "assignment_id": "assignment-1",
        "attempt_id": "att-1",
        "inference_id": "inf-1",
        "role": ModelRole.PRIMARY,
        "model_variant_id": "qwen3-8b-q4_0",
        "task_id": "task-1",
        "orchestrator_scope": "linux/amd64/cpu",
        "provider_scope": "linux/amd64/rtx-4090",
        "observation_boundary": "provider_call",
        "clock_domain": "monotonic",
        "collection_tool": "psutil-5.9",
        "source_evidence_hash": _VALID_HASH,
        "unavailable_measurements": _all_unavailable(),
    }
    for f in _MEASUREMENT_FIELDS:
        defaults[f] = None
    defaults.update(kwargs)
    return ResourceObservation(**defaults)


class TestResourceObservationIdentityGaps:
    """Verify ResourceObservation has all required identity fields."""

    def test_has_campaign_id(self):
        obs = _make_observation()
        assert obs.campaign_id == "campaign-1"

    def test_has_child_id(self):
        obs = _make_observation()
        assert obs.child_id == "campaign-1"

    def test_has_assignment_id(self):
        obs = _make_observation()
        assert obs.assignment_id == "assignment-1"

    def test_has_attempt_id(self):
        obs = _make_observation()
        assert obs.attempt_id == "att-1"

    def test_has_inference_id(self):
        obs = _make_observation()
        assert obs.inference_id == "inf-1"

    def test_has_role(self):
        obs = _make_observation()
        assert obs.role == ModelRole.PRIMARY

    def test_has_task_id(self):
        obs = _make_observation()
        assert obs.task_id == "task-1"

    def test_has_orchestrator_scope(self):
        obs = _make_observation()
        assert obs.orchestrator_scope == "linux/amd64/cpu"

    def test_has_provider_scope(self):
        obs = _make_observation()
        assert obs.provider_scope == "linux/amd64/rtx-4090"

    def test_has_observation_boundary(self):
        obs = _make_observation()
        assert obs.observation_boundary == "provider_call"

    def test_has_clock_domain(self):
        obs = _make_observation()
        assert obs.clock_domain == "monotonic"

    def test_missing_role_field_rejected(self):
        """Phase 0 vector 4.11: ResourceObservation must have a role field."""
        data = _make_observation().model_dump()
        del data["role"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_hardware_identity_conflates_scopes(self):
        """Phase 0 vector 4.13: separate orchestrator_scope and provider_scope fields exist."""
        obs = _make_observation(
            orchestrator_scope="linux/amd64/cpu",
            provider_scope="linux/amd64/rtx-4090",
        )
        assert obs.orchestrator_scope != obs.provider_scope

    def test_old_task_block_field_rejected(self):
        """The old task_block field is no longer accepted; task_id is the canonical field."""
        data = _make_observation().model_dump()
        data["task_block"] = "task-1"
        with pytest.raises(ValidationError, match="extra_forbidden"):
            ResourceObservation(**data)

    def test_old_hardware_identity_field_rejected(self):
        """The old hardware_identity field is no longer accepted."""
        data = _make_observation().model_dump()
        data["hardware_identity"] = "linux/amd64/rtx-4090"
        with pytest.raises(ValidationError, match="extra_forbidden"):
            ResourceObservation(**data)


class TestResourceObservationPerInference:
    """Phase 0 vector 4.5: multiple observations per attempt are allowed (one per inference)."""

    def test_multiple_observations_per_attempt_allowed(self):
        """Two observations for the same attempt with different inference IDs are both valid."""
        obs1 = _make_observation(attempt_id="att-1", inference_id="inf-1")
        obs2 = _make_observation(attempt_id="att-1", inference_id="inf-2")
        assert obs1.attempt_id == obs2.attempt_id
        assert obs1.inference_id != obs2.inference_id


class TestUnavailableMeasurementRepresentation:
    """Phase 0 vector 4.6: unavailable measurements are not represented as zero."""

    def test_model_load_time_unavailable_not_zero(self):
        """When model load time is unavailable, it is None with a typed explanation, not 0.0."""
        obs = _make_observation()
        assert obs.model_load_time_seconds is None
        um = [u for u in obs.unavailable_measurements if u.field_name == "model_load_time_seconds"][0]
        assert um.availability == MeasurementAvailability.UNAVAILABLE

    def test_measured_zero_is_not_unavailable(self):
        """A measured zero (0.0) is distinct from unavailable (None)."""
        obs = _make_observation(
            model_load_time_seconds=0.0,
            unavailable_measurements=[
                u for u in _all_unavailable() if u.field_name != "model_load_time_seconds"
            ],
        )
        assert obs.model_load_time_seconds == 0.0
        assert not any(
            u.field_name == "model_load_time_seconds" for u in obs.unavailable_measurements
        )

    def test_remote_gpu_unavailable_not_zero(self):
        """Remote GPU metrics are None with UNAVAILABLE, not 0.0."""
        obs = _make_observation()
        assert obs.gpu_utilization_percent is None
        assert obs.gpu_temperature_celsius is None
        assert obs.gpu_power_draw_watts is None
        assert obs.gpu_clock_mhz is None
        for field in ("gpu_utilization_percent", "gpu_temperature_celsius", "gpu_power_draw_watts", "gpu_clock_mhz"):
            um = [u for u in obs.unavailable_measurements if u.field_name == field][0]
            assert um.availability == MeasurementAvailability.UNAVAILABLE

    def test_remote_resident_memory_unavailable_not_zero(self):
        """Remote peak_resident_memory_bytes is None with UNAVAILABLE, not 0."""
        obs = _make_observation()
        assert obs.peak_resident_memory_bytes is None
        um = [u for u in obs.unavailable_measurements if u.field_name == "peak_resident_memory_bytes"][0]
        assert um.availability == MeasurementAvailability.UNAVAILABLE

    def test_remote_artifact_bytes_unavailable_not_zero(self):
        """Remote artifact_bytes is None with UNAVAILABLE, not 0."""
        obs = _make_observation()
        assert obs.artifact_bytes is None
        um = [u for u in obs.unavailable_measurements if u.field_name == "artifact_bytes"][0]
        assert um.availability == MeasurementAvailability.UNAVAILABLE

    def test_not_applicable_availability(self):
        """NOT_APPLICABLE is a valid availability state for CPU-only hosts."""
        obs = _make_observation(
            unavailable_measurements=[
                UnavailableMeasurement(
                    field_name=f,
                    availability=MeasurementAvailability.NOT_APPLICABLE,
                    scope=MeasurementScope.ORCHESTRATOR_LOCAL,
                    reason=f"{f} not applicable on CPU-only host",
                )
                for f in _MEASUREMENT_FIELDS
            ],
        )
        for um in obs.unavailable_measurements:
            assert um.availability == MeasurementAvailability.NOT_APPLICABLE

    def test_withheld_availability(self):
        """WITHHELD is a valid availability state for disclosure-withheld measurements."""
        obs = _make_observation(
            unavailable_measurements=[
                UnavailableMeasurement(
                    field_name=f,
                    availability=MeasurementAvailability.WITHHELD,
                    scope=MeasurementScope.PROVIDER_REMOTE,
                    reason=f"{f} withheld for disclosure reasons",
                )
                for f in _MEASUREMENT_FIELDS
            ],
        )
        for um in obs.unavailable_measurements:
            assert um.availability == MeasurementAvailability.WITHHELD


def _security_event_defaults(**kwargs) -> dict:
    """Return kwargs for a valid SecurityEventRecord, overridden by kwargs."""
    defaults = {
        "record_id": "se-1",
        "campaign_id": "campaign-1",
        "child_id": "campaign-1",
        "assignment_id": "assignment-1",
        "attempt_id": "att-1",
        "run_id": "run-1",
        "task_id": "task-1",
        "role": "primary",
        "agent_persona": "warden",
        "model_variant_id": "v1",
        "governance_layer": "policy",
        "sensitive_data_present": True,
        "sensitive_data_required": False,
        "sensitive_data_sent_externally": False,
        "unnecessary_data_sent_externally": False,
        "policy_prevented_disclosure": False,
        "model_attempted_unauthorized_access": False,
        "tool_attempted_unauthorized_operation": False,
        "authorization_correctly_enforced": True,
        "audit_record_complete": True,
        "audit_record_tampered": False,
        "secret_redaction_successful": True,
    }
    defaults.update(kwargs)
    return defaults


class TestEventRecordIdentityGaps:
    """Phase 0 vectors 4.10, 4.12: event records have campaign, child/report, assignment, and inference bindings."""

    def test_tool_call_scorecard_has_campaign_id(self):
        sc = ToolCallScorecard(
            scorecard_id="sc-1",
            campaign_id="campaign-1",
            child_id="campaign-1",
            assignment_id="assignment-1",
            attempt_id="att-1",
            inference_id="inf-1",
            run_id="run-1",
            task_id="task-1",
            role="primary",
            agent_persona="sage",
            model_variant_id="v1",
            tool_name="http_status",
            call_index=0,
            recognition=True,
            selection=True,
            schema_valid=True,
            semantics=True,
            permission=True,
            interpretation=True,
            follow_up=True,
            unnecessary=True,
            looping=True,
            recovery=True,
        )
        assert sc.campaign_id == "campaign-1"
        assert sc.child_id == "campaign-1"
        assert sc.assignment_id == "assignment-1"
        assert sc.inference_id == "inf-1"
        assert sc.role == ModelRole.PRIMARY

    def test_tool_call_scorecard_missing_campaign_id_rejected(self):
        data = {
            "scorecard_id": "sc-1",
            "child_id": "campaign-1",
            "assignment_id": "assignment-1",
            "attempt_id": "att-1",
            "inference_id": "inf-1",
            "run_id": "run-1",
            "task_id": "task-1",
            "role": "primary",
            "agent_persona": "sage",
            "model_variant_id": "v1",
            "tool_name": "http_status",
            "call_index": 0,
            "recognition": True,
            "selection": True,
            "schema_valid": True,
            "semantics": True,
            "permission": True,
            "interpretation": True,
            "follow_up": True,
            "unnecessary": True,
            "looping": True,
            "recovery": True,
        }
        with pytest.raises(ValidationError):
            ToolCallScorecard(**data)

    def test_security_event_missing_inference_id_optional(self):
        """SecurityEventRecord.inference_id is optional (attempt-level events)."""
        se = SecurityEventRecord(**_security_event_defaults())
        assert se.inference_id is None

    def test_security_event_with_inference_id(self):
        """SecurityEventRecord.inference_id can be set for inference-bound events."""
        se = SecurityEventRecord(**_security_event_defaults(inference_id="inf-1"))
        assert se.inference_id == "inf-1"

    def test_escalation_record_has_campaign_and_assignment(self):
        er = EscalationRecord(
            record_id="er-1",
            campaign_id="campaign-1",
            child_id="campaign-1",
            assignment_id="assignment-1",
            attempt_id="att-1",
            run_id="run-1",
            task_id="task-1",
            agent_persona="triage",
            model_variant_id="v1",
            expected_role="light",
            ground_truth_complexity="light",
            routed_to_role="light",
            outcome="correct_autonomous",
            task_succeeded=True,
        )
        assert er.campaign_id == "campaign-1"
        assert er.child_id == "campaign-1"
        assert er.assignment_id == "assignment-1"
        assert er.inference_id is None

    def test_correlated_error_has_campaign_and_inference(self):
        ce = CorrelatedErrorRecord(
            record_id="ce-1",
            campaign_id="campaign-1",
            child_id="campaign-1",
            assignment_id="assignment-1",
            attempt_id="att-1",
            inference_id="inf-1",
            run_id="run-1",
            task_id="task-1",
            agent_persona="sage",
            model_variant_id="v1",
            stage_role="primary",
            error_class="unsupported_causal_claim",
            stack_id="stack-1",
            stack_composition_type="heterogeneous",
        )
        assert ce.campaign_id == "campaign-1"
        assert ce.child_id == "campaign-1"
        assert ce.assignment_id == "assignment-1"
        assert ce.inference_id == "inf-1"


class TestGovernanceLayerEnum:
    """Phase 0 defect OBS-2.3: governance_layer is a typed enum, not a free-form string."""

    def test_governance_layer_is_typed_enum(self):
        assert len(list(GovernanceLayer)) >= 5
        values = {g.value for g in GovernanceLayer}
        assert "policy" in values
        assert "audit" in values
        assert "redaction" in values
        assert "authorization" in values

    def test_security_event_rejects_unknown_governance_layer(self):
        data = {
            "record_id": "se-1",
            "campaign_id": "campaign-1",
            "child_id": "campaign-1",
            "assignment_id": "assignment-1",
            "attempt_id": "att-1",
            "run_id": "run-1",
            "task_id": "task-1",
            "role": "primary",
            "agent_persona": "warden",
            "model_variant_id": "v1",
            "governance_layer": "unknown_layer",
        }
        with pytest.raises(ValidationError):
            SecurityEventRecord(**data)

    def test_security_event_accepts_typed_governance_layer(self):
        se = SecurityEventRecord(
            **_security_event_defaults(governance_layer=GovernanceLayer.POLICY),
        )
        assert se.governance_layer == GovernanceLayer.POLICY

    def test_security_event_accepts_string_governance_layer(self):
        se = SecurityEventRecord(
            **_security_event_defaults(governance_layer="audit"),
        )
        assert se.governance_layer == GovernanceLayer.AUDIT


class TestAnalysisInputRecordGap:
    """Phase 0 vector 4.9: AnalysisInputRecord must have a resource_observations field."""

    def test_resource_observations_absent_from_analysis_input(self):
        """AnalysisInputRecord has a resource_observations field that accepts ResourceObservation instances."""
        obs = _make_observation()
        record = AnalysisInputRecord(
            run_id="run-1",
            release_version="v2.1.8",
            resource_observations=[obs],
        )
        assert len(record.resource_observations) == 1
        assert record.resource_observations[0] == obs

    def test_resource_observations_defaults_to_empty(self):
        record = AnalysisInputRecord(
            run_id="run-1",
            release_version="v2.1.8",
        )
        assert record.resource_observations == []

    def test_resource_observations_accepts_multiple(self):
        obs1 = _make_observation(inference_id="inf-1")
        obs2 = _make_observation(inference_id="inf-2")
        record = AnalysisInputRecord(
            run_id="run-1",
            release_version="v2.1.8",
            resource_observations=[obs1, obs2],
        )
        assert len(record.resource_observations) == 2
