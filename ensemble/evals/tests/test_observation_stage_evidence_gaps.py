# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for stage identity, typed role identity, and indexed
source-evidence bindings on ResourceObservation and event records.

Closes the Phase 1 re-audit gaps identified by the Senior Integrator:

- ``ResourceObservation`` does not yet carry a ``stage_id`` binding the
  observation to the specific stage that produced the inference.
- ``ResourceObservation`` carries only a bare ``source_evidence_hash``
  string with no indexed source-evidence references or verification
  status, so a VERIFIED observation is not independently verifiable.
- ``EscalationRecord.expected_role``, ``EscalationRecord.routed_to_role``,
  and ``CorrelatedErrorRecord.stage_role`` are free-form strings using
  ``"light"`` while ``ModelRole`` uses ``"lite"``, so the schemas do not
  provide a completely typed role/stage join.
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with missing required fields,
# unknown extra fields, and invalid enum values to verify pydantic
# validation rejects them.

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.index import (
    MeasurementAvailability,
    MeasurementScope,
    ModelRole,
    ResourceObservation,
    UnavailableMeasurement,
)
from g8e_evals.schema import (
    CorrelatedErrorRecord,
    ErrorClassLabel,
    EscalationOutcome,
    EscalationRecord,
    SecurityEventRecord,
    StackCompositionType,
    ToolCallScorecard,
    VerificationStatus,
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
    defaults: dict = {
        "campaign_id": "campaign-1",
        "child_id": "campaign-1",
        "run_id": "run-1",
        "assignment_id": "assignment-1",
        "attempt_id": "att-1",
        "inference_id": "inf-1",
        "stage_id": "att-1:direct:1",
        "role": ModelRole.PRIMARY,
        "model_variant_id": "qwen3-8b-q4_0",
        "task_id": "task-1",
        "orchestrator_scope": "linux/amd64/cpu",
        "provider_scope": "linux/amd64/rtx-4090",
        "observation_boundary": "provider_call",
        "clock_domain": "monotonic",
        "collection_tool": "psutil-5.9",
        "source_evidence_refs": ["evidence/abc.json"],
        "source_evidence_sha256": _VALID_HASH,
        "verification_status": VerificationStatus.VERIFIED,
        "unavailable_measurements": _all_unavailable(),
    }
    for f in _MEASUREMENT_FIELDS:
        defaults[f] = None
    defaults.update(kwargs)
    return ResourceObservation(**defaults)


def _security_event_defaults(**kwargs) -> dict:
    defaults = {
        "record_id": "se-1",
        "campaign_id": "campaign-1",
        "child_id": "campaign-1",
        "assignment_id": "assignment-1",
        "attempt_id": "att-1",
        "run_id": "run-1",
        "task_id": "task-1",
        "role": ModelRole.PRIMARY,
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


# ---------------------------------------------------------------------------
# ResourceObservation: stage identity
# ---------------------------------------------------------------------------


class TestResourceObservationStageIdentity:
    """ResourceObservation must bind to the stage that produced the inference."""

    def test_has_stage_id_field(self):
        obs = _make_observation()
        assert obs.stage_id == "att-1:direct:1"

    def test_stage_id_round_trips(self):
        obs = _make_observation(stage_id="att-1:sse:evt-3:call:2")
        restored = ResourceObservation.model_validate_json(obs.model_dump_json())
        assert restored == obs
        assert restored.stage_id == "att-1:sse:evt-3:call:2"

    def test_requires_stage_id(self):
        data = _make_observation().model_dump()
        del data["stage_id"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_empty_stage_id(self):
        data = _make_observation().model_dump()
        data["stage_id"] = ""
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_old_source_evidence_hash_field(self):
        """The old bare source_evidence_hash field is replaced by indexed evidence."""
        data = _make_observation().model_dump()
        data["source_evidence_hash"] = _VALID_HASH
        with pytest.raises(ValidationError, match="extra_forbidden"):
            ResourceObservation(**data)


# ---------------------------------------------------------------------------
# ResourceObservation: indexed source evidence
# ---------------------------------------------------------------------------


class TestResourceObservationSourceEvidence:
    """ResourceObservation must carry indexed source-evidence refs, sha256, and status."""

    def test_has_source_evidence_refs(self):
        obs = _make_observation()
        assert obs.source_evidence_refs == ["evidence/abc.json"]

    def test_has_source_evidence_sha256(self):
        obs = _make_observation()
        assert obs.source_evidence_sha256 == _VALID_HASH

    def test_has_verification_status(self):
        obs = _make_observation()
        assert obs.verification_status == VerificationStatus.VERIFIED

    def test_verified_requires_source_evidence_refs(self):
        data = _make_observation().model_dump()
        data["source_evidence_refs"] = []
        with pytest.raises(ValidationError, match="source evidence"):
            ResourceObservation(**data)

    def test_verified_requires_source_evidence_sha256(self):
        data = _make_observation().model_dump()
        data["source_evidence_sha256"] = None
        with pytest.raises(ValidationError, match="source evidence"):
            ResourceObservation(**data)

    def test_pending_allows_empty_refs(self):
        data = _make_observation(verification_status=VerificationStatus.PENDING).model_dump()
        data["source_evidence_refs"] = []
        data["source_evidence_sha256"] = None
        obs = ResourceObservation(**data)
        assert obs.verification_status == VerificationStatus.PENDING
        assert obs.source_evidence_refs == []
        assert obs.source_evidence_sha256 is None

    def test_rejects_duplicate_source_evidence_refs(self):
        data = _make_observation().model_dump()
        data["source_evidence_refs"] = ["evidence/a.json", "evidence/a.json"]
        with pytest.raises(ValidationError, match="unique"):
            ResourceObservation(**data)

    def test_rejects_invalid_sha256(self):
        data = _make_observation().model_dump()
        data["source_evidence_sha256"] = "not-a-hash"
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_round_trip_preserves_source_evidence(self):
        obs = _make_observation(
            source_evidence_refs=["evidence/a.json", "evidence/b.json"],
            source_evidence_sha256="b" * 64,
        )
        restored = ResourceObservation.model_validate_json(obs.model_dump_json())
        assert restored == obs
        assert restored.source_evidence_refs == ["evidence/a.json", "evidence/b.json"]
        assert restored.source_evidence_sha256 == "b" * 64


# ---------------------------------------------------------------------------
# EscalationRecord: typed role identity
# ---------------------------------------------------------------------------


class TestEscalationRecordTypedRoles:
    """EscalationRecord expected_role and routed_to_role must be typed ModelRole."""

    def _make_escalation(self, **kwargs) -> EscalationRecord:
        defaults = {
            "record_id": "er-1",
            "campaign_id": "campaign-1",
            "child_id": "campaign-1",
            "assignment_id": "assignment-1",
            "attempt_id": "att-1",
            "run_id": "run-1",
            "task_id": "task-1",
            "agent_persona": "triage",
            "model_variant_id": "v1",
            "expected_role": ModelRole.LITE,
            "ground_truth_complexity": "light",
            "routed_to_role": ModelRole.LITE,
            "outcome": "correct_autonomous",
            "task_succeeded": True,
        }
        defaults.update(kwargs)
        return EscalationRecord(**defaults)

    def test_expected_role_is_typed_model_role(self):
        er = self._make_escalation()
        assert er.expected_role == ModelRole.LITE

    def test_routed_to_role_is_typed_model_role(self):
        er = self._make_escalation()
        assert er.routed_to_role == ModelRole.LITE

    def test_expected_role_accepts_string_lite(self):
        er = self._make_escalation(expected_role="lite")
        assert er.expected_role == ModelRole.LITE

    def test_routed_to_role_accepts_string_primary(self):
        er = self._make_escalation(routed_to_role="primary")
        assert er.routed_to_role == ModelRole.PRIMARY

    def test_rejects_light_string_for_expected_role(self):
        """The old 'light' string is rejected; the canonical value is 'lite'."""
        with pytest.raises(ValidationError):
            self._make_escalation(expected_role="light")

    def test_rejects_light_string_for_routed_to_role(self):
        """The old 'light' string is rejected; the canonical value is 'lite'."""
        with pytest.raises(ValidationError):
            self._make_escalation(routed_to_role="light")

    def test_rejects_unknown_role_string(self):
        with pytest.raises(ValidationError):
            self._make_escalation(expected_role="unknown_role")

    def test_round_trip_preserves_typed_roles(self):
        er = self._make_escalation(
            expected_role=ModelRole.PRIMARY,
            routed_to_role=ModelRole.ASSISTANT,
        )
        restored = EscalationRecord.model_validate_json(er.model_dump_json())
        assert restored == er
        assert restored.expected_role == ModelRole.PRIMARY
        assert restored.routed_to_role == ModelRole.ASSISTANT


# ---------------------------------------------------------------------------
# CorrelatedErrorRecord: typed stage role
# ---------------------------------------------------------------------------


class TestCorrelatedErrorRecordTypedStageRole:
    """CorrelatedErrorRecord stage_role must be typed ModelRole."""

    def _make_error(self, **kwargs) -> CorrelatedErrorRecord:
        defaults = {
            "record_id": "ce-1",
            "campaign_id": "campaign-1",
            "child_id": "campaign-1",
            "assignment_id": "assignment-1",
            "attempt_id": "att-1",
            "inference_id": "inf-1",
            "run_id": "run-1",
            "task_id": "task-1",
            "agent_persona": "sage",
            "model_variant_id": "v1",
            "stage_role": ModelRole.PRIMARY,
            "error_class": "unsupported_causal_claim",
            "stack_id": "stack-1",
            "stack_composition_type": "heterogeneous",
        }
        defaults.update(kwargs)
        return CorrelatedErrorRecord(**defaults)

    def test_stage_role_is_typed_model_role(self):
        ce = self._make_error()
        assert ce.stage_role == ModelRole.PRIMARY

    def test_stage_role_accepts_string_lite(self):
        ce = self._make_error(stage_role="lite")
        assert ce.stage_role == ModelRole.LITE

    def test_rejects_light_string_for_stage_role(self):
        """The old 'light' string is rejected; the canonical value is 'lite'."""
        with pytest.raises(ValidationError):
            self._make_error(stage_role="light")

    def test_rejects_unknown_role_string(self):
        with pytest.raises(ValidationError):
            self._make_error(stage_role="unknown_role")

    def test_round_trip_preserves_typed_stage_role(self):
        ce = self._make_error(stage_role=ModelRole.ASSISTANT)
        restored = CorrelatedErrorRecord.model_validate_json(ce.model_dump_json())
        assert restored == ce
        assert restored.stage_role == ModelRole.ASSISTANT


# ---------------------------------------------------------------------------
# Cross-record role join: all role fields use the same typed enum
# ---------------------------------------------------------------------------


class TestCrossRecordRoleJoin:
    """All role fields across event records use the same ModelRole enum so
    records can be joined by role without string normalization."""

    def test_resource_observation_role_uses_model_role(self):
        obs = _make_observation(role=ModelRole.LITE)
        assert obs.role == ModelRole.LITE

    def test_tool_call_scorecard_role_uses_model_role(self):
        sc = ToolCallScorecard(
            scorecard_id="sc-1",
            campaign_id="campaign-1",
            child_id="campaign-1",
            assignment_id="assignment-1",
            attempt_id="att-1",
            inference_id="inf-1",
            run_id="run-1",
            task_id="task-1",
            role=ModelRole.LITE,
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
        assert sc.role == ModelRole.LITE

    def test_security_event_role_uses_model_role(self):
        se = SecurityEventRecord(
            **_security_event_defaults(role=ModelRole.LITE),
        )
        assert se.role == ModelRole.LITE

    def test_all_role_fields_are_same_enum_type(self):
        """All role fields are ModelRole instances, not free-form strings."""
        obs = _make_observation(role=ModelRole.LITE)
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
            expected_role=ModelRole.LITE,
            ground_truth_complexity="light",
            routed_to_role=ModelRole.LITE,
            outcome=EscalationOutcome.CORRECT_AUTONOMOUS,
            task_succeeded=True,
        )
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
            stage_role=ModelRole.LITE,
            error_class=ErrorClassLabel.UNSUPPORTED_CAUSAL_CLAIM,
            stack_id="stack-1",
            stack_composition_type=StackCompositionType.HETEROGENEOUS,
        )
        assert isinstance(obs.role, ModelRole)
        assert isinstance(er.expected_role, ModelRole)
        assert isinstance(er.routed_to_role, ModelRole)
        assert isinstance(ce.stage_role, ModelRole)
        # All use the same value "lite" — no "light" vs "lite" mismatch
        assert obs.role.value == "lite"
        assert er.expected_role.value == "lite"
        assert er.routed_to_role.value == "lite"
        assert ce.stage_role.value == "lite"
