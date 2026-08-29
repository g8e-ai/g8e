# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the canonical analytical data model.

Verifies schema validation, extra-field rejection, hash property
resolution, and the logical key structure of attempt records.
"""

from __future__ import annotations

from datetime import datetime, timezone

import pytest
from pydantic import ValidationError

from g8e_evals.arms import Arm, GovernancePosture
from g8e_evals.schema import (
    SCHEMA_VERSION,
    AttemptRecord,
    ArmManifestEntry,
    ContentHash,
    EvidenceIndex,
    EvidenceMediaType,
    GraderClass,
    MetricObservation,
    ModelIdentity,
    PostureObservation,
    PrivacyClassification,
    RoleToModelMapping,
    RunManifest,
    SamplingSettings,
    StackEnvironment,
    StageKind,
    StageObservation,
    TaskDefinition,
    TerminalStatus,
    UsageReconciliation,
    VerificationStatus,
)

pytestmark = pytest.mark.unit


def test_schema_version_is_pinned():
    assert SCHEMA_VERSION == "1.3.0"


def test_usage_reconciliation_flags_mismatched_stage_totals():
    reconciliation = UsageReconciliation(
        reported_input_tokens=20,
        reported_output_tokens=10,
        observed_input_tokens=18,
        observed_output_tokens=10,
        observed_call_count=2,
        expected_call_count=2,
    )

    assert reconciliation.reconciled is False
    assert reconciliation.input_token_delta == -2
    assert reconciliation.output_token_delta == 0


def test_run_manifest_resolves_dataset_hash():
    manifest = RunManifest(
        run_id="r1",
        suite_id="ifeval_subset",
        suite_version="1.0",
        content_hashes=[
            ContentHash(name="dataset", sha256="abc123"),
            ContentHash(name="grader_bundle", sha256="def456"),
            ContentHash(name="prompt_bundle", sha256="ghi789"),
        ],
    )
    assert manifest.dataset_hash == "abc123"
    assert manifest.grader_bundle_hash == "def456"
    assert manifest.prompt_bundle_hash == "ghi789"


def test_run_manifest_hash_properties_return_none_when_absent():
    manifest = RunManifest(run_id="r1", suite_id="s", suite_version="1.0")
    assert manifest.dataset_hash is None
    assert manifest.grader_bundle_hash is None
    assert manifest.prompt_bundle_hash is None


def test_run_manifest_rejects_extra_fields():
    with pytest.raises(ValidationError):
        RunManifest(run_id="r1", suite_id="s", suite_version="1.0", extra_field="bad")  # type: ignore[call-arg]


def test_task_definition_rejects_extra_fields():
    with pytest.raises(ValidationError):
        TaskDefinition(
            task_id="t1",
            suite_id="s",
            suite_version="1.0",
            prompt_hash="h",
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_attempt_record_rejects_extra_fields():
    with pytest.raises(ValidationError):
        AttemptRecord(
            attempt_id="a1",
            run_id="r1",
            task_id="t1",
            arm_id=Arm.DIRECT,
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_attempt_record_logical_key_fields():
    ar = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        model_cohort_id="cohort-1",
        state_snapshot_hash="snap-1",
        replicate_id="1",
    )
    assert ar.arm_id == Arm.DOCTRINE
    assert ar.model_cohort_id == "cohort-1"
    assert ar.state_snapshot_hash == "snap-1"
    assert ar.replicate_id == "1"
    assert ar.parent_attempt_id is None


def test_attempt_record_default_posture_is_none():
    ar = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DIRECT,
    )
    assert ar.posture.requested_posture == GovernancePosture.NONE
    assert ar.posture.observed_posture is None


def test_stage_observation_rejects_extra_fields():
    with pytest.raises(ValidationError):
        StageObservation(
            stage_id="s1",
            attempt_id="a1",
            run_id="r1",
            kind=StageKind.MODEL_INFERENCE,
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_metric_obsation_rejects_extra_fields():
    with pytest.raises(ValidationError):
        MetricObservation(
            metric_id="m1",
            attempt_id="a1",
            run_id="r1",
            arm_id=Arm.DIRECT,
            task_id="t1",
            value=1.0,
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_evidence_index_rejects_extra_fields():
    with pytest.raises(ValidationError):
        EvidenceIndex(
            artifact_id="e1",
            run_id="r1",
            media_type=EvidenceMediaType.APPLICATION_JSON,
            sha256="abc",
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_model_identity_rejects_extra_fields():
    with pytest.raises(ValidationError):
        ModelIdentity(
            role="primary",
            provider="ollama",
            model="test",
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_posture_observation_records_requested_and_observed():
    po = PostureObservation(
        requested_posture=GovernancePosture.L2_CONSENSUS,
        observed_posture=GovernancePosture.L2_CONSENSUS,
        observation_source="gateway",
        posture_match=True,
    )
    assert po.requested_posture == GovernancePosture.L2_CONSENSUS
    assert po.observed_posture == GovernancePosture.L2_CONSENSUS
    assert po.posture_match is True


def test_arm_manifest_entry_rejects_extra_fields():
    with pytest.raises(ValidationError):
        ArmManifestEntry(
            arm_id=Arm.DIRECT,
            requested_posture=GovernancePosture.NONE,
            uses_g8ee=False,
            uses_gateway=False,
            receipt_binding=False,
            is_production_posture=False,
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_terminal_status_has_seven_values():
    assert set(TerminalStatus) == {
        TerminalStatus.COMPLETED,
        TerminalStatus.MODEL_FAILED,
        TerminalStatus.GOVERNANCE_REJECTED,
        TerminalStatus.HUMAN_DENIED,
        TerminalStatus.TIMED_OUT,
        TerminalStatus.INFRASTRUCTURE_FAILED,
        TerminalStatus.INVALID_EVIDENCE,
    }


def test_stage_kind_covers_all_canonical_kinds():
    expected = {
        StageKind.MODEL_INFERENCE,
        StageKind.DETERMINISTIC_DOCTRINE,
        StageKind.TRIBUNAL_GENERATION,
        StageKind.TRIBUNAL_AUDITOR,
        StageKind.PROTOCOL_L2,
        StageKind.L3_CEREMONY,
        StageKind.L4_VERIFICATION,
        StageKind.L5_EXECUTION,
        StageKind.SCRUBBING,
        StageKind.REHYDRATION,
        StageKind.GRADING,
        StageKind.RECEIPT_PERSISTENCE,
        StageKind.COMMITMENT_APPEND,
    }
    assert set(StageKind) == expected


def test_run_manifest_serializes_to_json():
    manifest = RunManifest(
        run_id="r1",
        suite_id="s",
        suite_version="1.0",
        arms=[ArmManifestEntry(
            arm_id=Arm.DOCTRINE,
            requested_posture=GovernancePosture.L1_DOCTRINE,
            uses_g8ee=True,
            uses_gateway=True,
            receipt_binding=True,
            is_production_posture=True,
        )],
    )
    json_str = manifest.model_dump_json()
    assert '"run_id":"r1"' in json_str
    assert '"arm_id":"doctrine"' in json_str


def test_attempt_record_with_infrastructure_retry_linkage():
    parent = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.INFRASTRUCTURE_FAILED,
    )
    child = AttemptRecord(
        attempt_id="a2",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        parent_attempt_id="a1",
        assignment_order=1,
    )
    assert child.parent_attempt_id == "a1"
    assert parent.terminal_status == TerminalStatus.INFRASTRUCTURE_FAILED
    assert parent.parent_attempt_id is None
