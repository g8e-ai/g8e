# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for CampaignBinding schema and RunManifest campaign bindings.

Verifies that the CampaignBinding typed model and its sub-models
(BackendArtifactIdentity, TokenizerTemplateIdentity) enforce strict
field validation, reject unknown fields, round-trip through JSON, and
that RunManifest carries an optional CampaignBinding without breaking
backward compatibility for non-campaign runs.
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with missing required fields
# and unknown extra fields to verify pydantic validation rejects them.

from __future__ import annotations

import json

import pytest
from pydantic import ValidationError

from g8e_evals.arms import Arm, GovernancePosture
from g8e_evals.schema import (
    ArmManifestEntry,
    CampaignBinding,
    CampaignTrack,
    BackendArtifactIdentity,
    TokenizerTemplateIdentity,
    ReportRole,
    RunManifest,
    TrackArmAssignment,
)


pytestmark = pytest.mark.unit


_VALID_HASH = "a" * 64


def _backend_artifact_identity() -> BackendArtifactIdentity:
    return BackendArtifactIdentity(
        backend_name="ollama",
        backend_version="0.1.48",
        served_model_tag="qwen3:8b",
        artifact_digest=_VALID_HASH,
        artifact_bytes=8_000_000_000,
        quantization="q4_0",
        tensor_format="gguf",
    )


def _tokenizer_template_identity() -> TokenizerTemplateIdentity:
    return TokenizerTemplateIdentity(
        tokenizer_digest=_VALID_HASH,
        chat_template_hash=_VALID_HASH,
        prompt_serialization_version="1.0",
    )


def _track_arm_assignments() -> list[TrackArmAssignment]:
    return [TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct")]


def _campaign_binding(
    *,
    report_role: ReportRole = ReportRole.SINGLE,
    child_campaign_id: str | None = None,
    child_campaign_revision: str | None = None,
) -> CampaignBinding:
    return CampaignBinding(
        campaign_id="v2.1.8-ifeval-pipeline-integrity",
        campaign_revision="1",
        report_role=report_role,
        child_campaign_id=child_campaign_id,
        child_campaign_revision=child_campaign_revision,
        campaign_profile_hash=_VALID_HASH,
        model_registry_hash=_VALID_HASH,
        required_record_policy_hash=_VALID_HASH,
        orchestrator_hardware_identity="linux/amd64/rtx-4090",
        orchestrator_environment_stratum="single-machine",
        provider_hardware_identity="unavailable",
        provider_environment_stratum="unavailable",
        track_arm_assignments=_track_arm_assignments(),
    )


class TestCampaignTrack:
    def test_direct_track_value(self):
        assert CampaignTrack.DIRECT.value == "direct"

    def test_tier_fitness_track_value(self):
        assert CampaignTrack.TIER_FITNESS.value == "tier_fitness"

    def test_governed_track_value(self):
        assert CampaignTrack.GOVERNED.value == "governed"


class TestBackendArtifactIdentity:
    def test_round_trip_preserves_all_fields(self):
        identity = _backend_artifact_identity()
        restored = BackendArtifactIdentity.model_validate_json(identity.model_dump_json())
        assert restored == identity

    def test_rejects_unknown_fields(self):
        with pytest.raises(ValidationError):
            BackendArtifactIdentity(
                backend_name="ollama",
                served_model_tag="qwen3:8b",
                artifact_digest=_VALID_HASH,
                extra_field="bad",
            )

    def test_requires_backend_name(self):
        with pytest.raises(ValidationError):
            BackendArtifactIdentity(
                served_model_tag="qwen3:8b",
                artifact_digest=_VALID_HASH,
            )

    def test_requires_served_model_tag(self):
        with pytest.raises(ValidationError):
            BackendArtifactIdentity(
                backend_name="ollama",
                artifact_digest=_VALID_HASH,
            )

    def test_requires_artifact_digest(self):
        with pytest.raises(ValidationError):
            BackendArtifactIdentity(
                backend_name="ollama",
                served_model_tag="qwen3:8b",
            )

    def test_rejects_short_artifact_digest(self):
        with pytest.raises(ValidationError):
            BackendArtifactIdentity(
                backend_name="ollama",
                served_model_tag="qwen3:8b",
                artifact_digest="short",
            )

    def test_artifact_bytes_defaults_to_zero(self):
        identity = BackendArtifactIdentity(
            backend_name="ollama",
            served_model_tag="qwen3:8b",
            artifact_digest=_VALID_HASH,
        )
        assert identity.artifact_bytes == 0

    def test_rejects_negative_artifact_bytes(self):
        with pytest.raises(ValidationError):
            BackendArtifactIdentity(
                backend_name="ollama",
                served_model_tag="qwen3:8b",
                artifact_digest=_VALID_HASH,
                artifact_bytes=-1,
            )


class TestTokenizerTemplateIdentity:
    def test_round_trip_preserves_all_fields(self):
        identity = _tokenizer_template_identity()
        restored = TokenizerTemplateIdentity.model_validate_json(identity.model_dump_json())
        assert restored == identity

    def test_rejects_unknown_fields(self):
        with pytest.raises(ValidationError):
            TokenizerTemplateIdentity(
                tokenizer_digest=_VALID_HASH,
                chat_template_hash=_VALID_HASH,
                extra_field="bad",
            )

    def test_requires_tokenizer_digest(self):
        with pytest.raises(ValidationError):
            TokenizerTemplateIdentity(chat_template_hash=_VALID_HASH)

    def test_requires_chat_template_hash(self):
        with pytest.raises(ValidationError):
            TokenizerTemplateIdentity(tokenizer_digest=_VALID_HASH)

    def test_rejects_short_hashes(self):
        with pytest.raises(ValidationError):
            TokenizerTemplateIdentity(
                tokenizer_digest="short",
                chat_template_hash=_VALID_HASH,
            )


class TestCampaignBinding:
    def test_round_trip_preserves_all_fields(self):
        binding = _campaign_binding()
        restored = CampaignBinding.model_validate_json(binding.model_dump_json())
        assert restored == binding

    def test_rejects_unknown_fields(self):
        with pytest.raises(ValidationError):
            CampaignBinding(
                campaign_id="c1",
                campaign_revision="1",
                report_role=ReportRole.SINGLE,
                campaign_profile_hash=_VALID_HASH,
                model_registry_hash=_VALID_HASH,
                required_record_policy_hash=_VALID_HASH,
                orchestrator_hardware_identity="linux/amd64/rtx-4090",
                orchestrator_environment_stratum="single-machine",
                extra_field="bad",
            )

    def test_requires_campaign_id(self):
        with pytest.raises(ValidationError):
            CampaignBinding(
                campaign_revision="1",
                report_role=ReportRole.SINGLE,
                campaign_profile_hash=_VALID_HASH,
                model_registry_hash=_VALID_HASH,
                required_record_policy_hash=_VALID_HASH,
                orchestrator_hardware_identity="linux/amd64/rtx-4090",
                orchestrator_environment_stratum="single-machine",
                track_arm_assignments=_track_arm_assignments(),
            )

    def test_requires_campaign_revision(self):
        with pytest.raises(ValidationError):
            CampaignBinding(
                campaign_id="c1",
                report_role=ReportRole.SINGLE,
                campaign_profile_hash=_VALID_HASH,
                model_registry_hash=_VALID_HASH,
                required_record_policy_hash=_VALID_HASH,
                orchestrator_hardware_identity="linux/amd64/rtx-4090",
                orchestrator_environment_stratum="single-machine",
                track_arm_assignments=_track_arm_assignments(),
            )

    def test_requires_report_role(self):
        with pytest.raises(ValidationError):
            CampaignBinding(
                campaign_id="c1",
                campaign_revision="1",
                campaign_profile_hash=_VALID_HASH,
                model_registry_hash=_VALID_HASH,
                required_record_policy_hash=_VALID_HASH,
                orchestrator_hardware_identity="linux/amd64/rtx-4090",
                orchestrator_environment_stratum="single-machine",
                track_arm_assignments=_track_arm_assignments(),
            )

    def test_requires_campaign_profile_hash(self):
        with pytest.raises(ValidationError):
            CampaignBinding(
                campaign_id="c1",
                campaign_revision="1",
                report_role=ReportRole.SINGLE,
                model_registry_hash=_VALID_HASH,
                required_record_policy_hash=_VALID_HASH,
                orchestrator_hardware_identity="linux/amd64/rtx-4090",
                orchestrator_environment_stratum="single-machine",
                track_arm_assignments=_track_arm_assignments(),
            )

    def test_requires_model_registry_hash(self):
        with pytest.raises(ValidationError):
            CampaignBinding(
                campaign_id="c1",
                campaign_revision="1",
                report_role=ReportRole.SINGLE,
                campaign_profile_hash=_VALID_HASH,
                required_record_policy_hash=_VALID_HASH,
                orchestrator_hardware_identity="linux/amd64/rtx-4090",
                orchestrator_environment_stratum="single-machine",
                track_arm_assignments=_track_arm_assignments(),
            )

    def test_requires_required_record_policy_hash(self):
        with pytest.raises(ValidationError):
            CampaignBinding(
                campaign_id="c1",
                campaign_revision="1",
                report_role=ReportRole.SINGLE,
                campaign_profile_hash=_VALID_HASH,
                model_registry_hash=_VALID_HASH,
                orchestrator_hardware_identity="linux/amd64/rtx-4090",
                orchestrator_environment_stratum="single-machine",
                track_arm_assignments=_track_arm_assignments(),
            )

    def test_requires_orchestrator_hardware_identity(self):
        with pytest.raises(ValidationError):
            CampaignBinding(
                campaign_id="c1",
                campaign_revision="1",
                report_role=ReportRole.SINGLE,
                campaign_profile_hash=_VALID_HASH,
                model_registry_hash=_VALID_HASH,
                required_record_policy_hash=_VALID_HASH,
                orchestrator_environment_stratum="single-machine",
                track_arm_assignments=_track_arm_assignments(),
            )

    def test_requires_orchestrator_environment_stratum(self):
        with pytest.raises(ValidationError):
            CampaignBinding(
                campaign_id="c1",
                campaign_revision="1",
                report_role=ReportRole.SINGLE,
                campaign_profile_hash=_VALID_HASH,
                model_registry_hash=_VALID_HASH,
                required_record_policy_hash=_VALID_HASH,
                orchestrator_hardware_identity="linux/amd64/rtx-4090",
                track_arm_assignments=_track_arm_assignments(),
            )

    def test_requires_track_arm_assignments(self):
        with pytest.raises(ValidationError):
            CampaignBinding(
                campaign_id="c1",
                campaign_revision="1",
                report_role=ReportRole.SINGLE,
                campaign_profile_hash=_VALID_HASH,
                model_registry_hash=_VALID_HASH,
                required_record_policy_hash=_VALID_HASH,
                orchestrator_hardware_identity="linux/amd64/rtx-4090",
                orchestrator_environment_stratum="single-machine",
            )

    def test_track_arm_assignments_round_trip(self):
        binding = _campaign_binding()
        assert binding.track_arm_assignments == _track_arm_assignments()
        restored = CampaignBinding.model_validate_json(binding.model_dump_json())
        assert restored.track_arm_assignments == _track_arm_assignments()

    def test_child_role_carries_child_campaign_identity(self):
        binding = _campaign_binding(
            report_role=ReportRole.CHILD,
            child_campaign_id="child-campaign-1",
            child_campaign_revision="1",
        )
        assert binding.report_role == ReportRole.CHILD
        assert binding.child_campaign_id == "child-campaign-1"
        assert binding.child_campaign_revision == "1"

    def test_single_role_has_null_child_campaign_identity(self):
        binding = _campaign_binding(report_role=ReportRole.SINGLE)
        assert binding.report_role == ReportRole.SINGLE
        assert binding.child_campaign_id is None
        assert binding.child_campaign_revision is None

    def test_provider_hardware_identity_defaults_to_unavailable(self):
        binding = _campaign_binding()
        assert binding.provider_hardware_identity == "unavailable"
        assert binding.provider_environment_stratum == "unavailable"

    def test_rejects_short_profile_hash(self):
        with pytest.raises(ValidationError):
            CampaignBinding(
                campaign_id="c1",
                campaign_revision="1",
                report_role=ReportRole.SINGLE,
                campaign_profile_hash="short",
                model_registry_hash=_VALID_HASH,
                required_record_policy_hash=_VALID_HASH,
                orchestrator_hardware_identity="linux/amd64/rtx-4090",
                orchestrator_environment_stratum="single-machine",
                track_arm_assignments=_track_arm_assignments(),
            )

    def test_rejects_short_registry_hash(self):
        with pytest.raises(ValidationError):
            CampaignBinding(
                campaign_id="c1",
                campaign_revision="1",
                report_role=ReportRole.SINGLE,
                campaign_profile_hash=_VALID_HASH,
                model_registry_hash="short",
                required_record_policy_hash=_VALID_HASH,
                orchestrator_hardware_identity="linux/amd64/rtx-4090",
                orchestrator_environment_stratum="single-machine",
                track_arm_assignments=_track_arm_assignments(),
            )


class TestRunManifestCampaignBinding:
    def test_manifest_without_campaign_binding_is_backward_compatible(self):
        manifest = RunManifest(
            run_id="r1",
            suite_id="s",
            suite_version="1.0",
        )
        assert manifest.campaign_binding is None

    def test_manifest_with_campaign_binding_round_trips(self):
        manifest = RunManifest(
            run_id="r1",
            suite_id="s",
            suite_version="1.0",
            arms=[ArmManifestEntry(
                arm_id=Arm.DIRECT,
                requested_posture=GovernancePosture.NONE,
                uses_g8ee=False,
                uses_gateway=False,
                receipt_binding=False,
                is_production_posture=False,
            )],
            campaign_binding=_campaign_binding(),
        )
        json_str = manifest.model_dump_json()
        restored = RunManifest.model_validate_json(json_str)
        assert restored.campaign_binding is not None
        assert restored.campaign_binding.campaign_id == "v2.1.8-ifeval-pipeline-integrity"
        assert restored.campaign_binding.report_role == ReportRole.SINGLE
        assert restored.campaign_binding.campaign_profile_hash == _VALID_HASH
        assert restored.campaign_binding.model_registry_hash == _VALID_HASH
        assert restored.campaign_binding.required_record_policy_hash == _VALID_HASH
        assert restored.campaign_binding.orchestrator_hardware_identity == "linux/amd64/rtx-4090"
        assert restored.campaign_binding.orchestrator_environment_stratum == "single-machine"

    def test_manifest_campaign_binding_appears_in_json(self):
        manifest = RunManifest(
            run_id="r1",
            suite_id="s",
            suite_version="1.0",
            campaign_binding=_campaign_binding(),
        )
        json_str = manifest.model_dump_json()
        data = json.loads(json_str)
        assert "campaign_binding" in data
        assert data["campaign_binding"]["campaign_id"] == "v2.1.8-ifeval-pipeline-integrity"
        assert data["campaign_binding"]["report_role"] == "single"

    def test_manifest_without_campaign_binding_has_null_in_json(self):
        manifest = RunManifest(
            run_id="r1",
            suite_id="s",
            suite_version="1.0",
        )
        json_str = manifest.model_dump_json()
        data = json.loads(json_str)
        assert data["campaign_binding"] is None

    def test_manifest_rejects_campaign_binding_with_unknown_fields(self):
        with pytest.raises(ValidationError):
            RunManifest(
                run_id="r1",
                suite_id="s",
                suite_version="1.0",
                campaign_binding=CampaignBinding(
                    campaign_id="c1",
                    campaign_revision="1",
                    report_role=ReportRole.SINGLE,
                    campaign_profile_hash=_VALID_HASH,
                    model_registry_hash=_VALID_HASH,
                    required_record_policy_hash=_VALID_HASH,
                    orchestrator_hardware_identity="linux/amd64/rtx-4090",
                    orchestrator_environment_stratum="single-machine",
                    track_arm_assignments=_track_arm_assignments(),
                ),
                extra_field="bad",
            )
