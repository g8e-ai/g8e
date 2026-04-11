# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
Unit tests for shared/constants/intents.json and the constants derived from it.

Verifies that:
- CloudIntent enum is loaded from the shared JSON (all intent names present, correct values)
- CLOUD_INTENT_DEPENDENCIES reflects the JSON dependency graph exactly
- CLOUD_INTENT_VERIFICATION_ACTIONS covers every intent and uses correct IAM action format
- All dependency targets are themselves valid CloudIntent members
- OperatorCommandService._get_verification_action_for_intent delegates to the shared map
- OperatorIntentService.execute_intent_permission_request rejects unknown intent names using
  CloudIntent._value2member_map_ (not a hand-rolled set)
"""

import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import (
    CLOUD_INTENT_DEPENDENCIES,
    CLOUD_INTENT_VERIFICATION_ACTIONS,
    CloudIntent,
)
from app.models.command_payloads import GrantIntentArgs
from app.models.tool_results import IntentPermissionResult
from app.services.operator.command_service import OperatorCommandService
from tests.fakes.builder import build_command_service
from tests.fakes.factories import build_vso_http_context

pytestmark = [pytest.mark.unit]


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _load_intents_json() -> dict:
    shared_dir = "/app/shared/constants"
    with open(shared_dir + "/intents.json") as f:
        return json.load(f)


def _make_service() -> OperatorCommandService:
    return build_command_service()


def _make_vso_context():
    return build_vso_http_context(
        web_session_id="web-test-123",
        case_id="case-456",
        investigation_id="inv-123",
    )


# ---------------------------------------------------------------------------
# CloudIntent enum — loaded from shared JSON
# ---------------------------------------------------------------------------

class TestCloudIntentEnum:
    """CloudIntent enum members match shared/constants/intents.json exactly."""

    def test_every_json_intent_is_a_cloud_intent_member(self):
        """Every key in intents.json must appear as a CloudIntent value."""
        raw = _load_intents_json()
        for name in raw["intents"]:
            assert name in CloudIntent._value2member_map_, (
                f"Intent '{name}' in intents.json has no CloudIntent member"
            )

    def test_no_cloud_intent_member_is_missing_from_json(self):
        """Every CloudIntent member must have an entry in intents.json."""
        raw = _load_intents_json()
        for member in CloudIntent._value2member_map_.values():
            assert member.value in raw["intents"], (
                f"CloudIntent member ({member.value}) is not in intents.json"
            )

    def test_cloud_intent_count_matches_json(self):
        """Number of CloudIntent members must equal number of entries in intents.json."""
        raw = _load_intents_json()
        assert len(CloudIntent._value2member_map_) == len(raw["intents"])

    def test_cloud_intent_members_are_strings(self):
        """CloudIntent is a str,Enum — members must equal their string value."""
        for member in CloudIntent._value2member_map_.values():
            assert member == member.value
            assert isinstance(member, str)

    def test_spot_check_known_members(self):
        """Spot-check a representative set of expected intents exist."""
        expected = [
            "ec2_discovery", "ec2_management", "s3_read", "s3_write",
            "lambda_invoke", "rds_discovery", "aurora_cluster_management",
            "ecs_management", "kms_crypto", "stepfunctions_execution",
            "athena_query_execution", "cost_explorer",
        ]
        for name in expected:
            assert name in CloudIntent._value2member_map_, (
                f"Expected intent '{name}' not found in CloudIntent"
            )

    def test_member_name_is_uppercase_of_value(self):
        """Each enum member name must be the uppercase version of its value."""
        for member in CloudIntent._value2member_map_.values():
            assert member.name == member.value.upper(), (
                f"CloudIntent member name mismatch: {member.name} vs {member.value.upper()}"
            )


# ---------------------------------------------------------------------------
# CLOUD_INTENT_DEPENDENCIES
# ---------------------------------------------------------------------------

class TestCloudIntentDependencies:
    """CLOUD_INTENT_DEPENDENCIES matches the dependency graph in intents.json."""

    def test_every_dependency_key_is_a_valid_intent(self):
        """Every key in CLOUD_INTENT_DEPENDENCIES must be a CloudIntent value."""
        for intent_name in CLOUD_INTENT_DEPENDENCIES:
            assert intent_name in CloudIntent._value2member_map_, (
                f"Dependency key '{intent_name}' is not a valid CloudIntent"
            )

    def test_every_dependency_target_is_a_valid_intent(self):
        """Every dependency listed in the graph must be a CloudIntent value."""
        for intent_name, deps in CLOUD_INTENT_DEPENDENCIES.items():
            for dep in deps:
                assert dep in CloudIntent._value2member_map_, (
                    f"Intent '{intent_name}' has dependency '{dep}' which is not a valid CloudIntent"
                )

    def test_intents_with_no_dependencies_are_absent(self):
        """Intents with empty dependency lists must not appear in CLOUD_INTENT_DEPENDENCIES."""
        raw = _load_intents_json()
        for name, meta in raw["intents"].items():
            if not meta["dependencies"]:
                assert name not in CLOUD_INTENT_DEPENDENCIES, (
                    f"Intent '{name}' has no dependencies but appears in CLOUD_INTENT_DEPENDENCIES"
                )

    def test_dependency_entries_match_json_exactly(self):
        """CLOUD_INTENT_DEPENDENCIES must exactly reflect the JSON dependency lists."""
        raw = _load_intents_json()
        for name, meta in raw["intents"].items():
            expected_deps = meta["dependencies"]
            actual_deps = CLOUD_INTENT_DEPENDENCIES.get(name, [])
            assert sorted(actual_deps) == sorted(expected_deps), (
                f"Dependency mismatch for '{name}': expected {expected_deps}, got {actual_deps}"
            )

    def test_spot_check_known_dependencies(self):
        """Spot-check representative dependency relationships."""
        cases = [
            ("ec2_management", ["ec2_discovery"]),
            ("ec2_snapshot_management", ["ec2_discovery"]),
            ("s3_write", ["s3_read"]),
            ("s3_delete", ["s3_read"]),
            ("lambda_invoke", ["lambda_discovery"]),
            ("rds_management", ["rds_discovery"]),
            ("ecs_management", ["ecs_discovery"]),
            ("kms_crypto", ["kms_discovery"]),
            ("dynamodb_write", ["dynamodb_read"]),
            ("stepfunctions_execution", ["stepfunctions_discovery"]),
            ("athena_query_execution", ["athena_discovery"]),
            ("route53_management", ["route53_discovery"]),
            ("autoscaling_management", ["autoscaling_discovery"]),
            ("sns_publish", ["sns_discovery"]),
            ("sqs_management", ["sqs_discovery"]),
        ]
        for intent_name, expected_deps in cases:
            actual = CLOUD_INTENT_DEPENDENCIES.get(intent_name, [])
            for dep in expected_deps:
                assert dep in actual, (
                    f"Expected '{dep}' in dependencies of '{intent_name}', got {actual}"
                )

    def test_intents_without_dependencies_resolve_to_empty(self):
        """Intents with no dependencies return [] from .get()."""
        no_dep_intents = ["ec2_discovery", "s3_read", "cloudwatch_logs", "iam_discovery", "cost_explorer"]
        for name in no_dep_intents:
            assert CLOUD_INTENT_DEPENDENCIES.get(name, []) == [], (
                f"Expected no dependencies for '{name}'"
            )


# ---------------------------------------------------------------------------
# CLOUD_INTENT_VERIFICATION_ACTIONS
# ---------------------------------------------------------------------------

class TestCloudIntentVerificationActions:
    """CLOUD_INTENT_VERIFICATION_ACTIONS covers every intent with a valid IAM action string."""

    def test_every_cloud_intent_has_a_verification_action(self):
        """Every CloudIntent value must have an entry in CLOUD_INTENT_VERIFICATION_ACTIONS."""
        for value in CloudIntent._value2member_map_:
            assert value in CLOUD_INTENT_VERIFICATION_ACTIONS, (
                f"CloudIntent '{value}' has no verification action"
            )

    def test_no_extra_entries_beyond_cloud_intents(self):
        """CLOUD_INTENT_VERIFICATION_ACTIONS must not contain entries absent from CloudIntent."""
        for name in CLOUD_INTENT_VERIFICATION_ACTIONS:
            assert name in CloudIntent._value2member_map_, (
                f"Verification action entry '{name}' is not a valid CloudIntent"
            )

    def test_all_verification_actions_are_non_empty_strings(self):
        """Each verification action must be a non-empty string."""
        for name, action in CLOUD_INTENT_VERIFICATION_ACTIONS.items():
            assert isinstance(action, str) and action.strip(), (
                f"Verification action for '{name}' is empty or not a string"
            )

    def test_verification_actions_use_service_colon_action_format(self):
        """All IAM actions must follow the 'service:Action' format."""
        for name, action in CLOUD_INTENT_VERIFICATION_ACTIONS.items():
            assert ":" in action, (
                f"Verification action for '{name}' does not use 'service:Action' format: '{action}'"
            )
            service, _, api_action = action.partition(":")
            assert service and api_action, (
                f"Verification action for '{name}' has an empty service or action part: '{action}'"
            )

    def test_spot_check_known_verification_actions(self):
        """Spot-check representative verification action mappings."""
        cases = [
            ("ec2_discovery", "ec2:DescribeInstances"),
            ("ec2_management", "ec2:StartInstances"),
            ("s3_read", "s3:GetObject"),
            ("s3_write", "s3:PutObject"),
            ("s3_bucket_discovery", "s3:ListAllMyBuckets"),
            ("lambda_discovery", "lambda:ListFunctions"),
            ("lambda_invoke", "lambda:InvokeFunction"),
            ("rds_discovery", "rds:DescribeDBInstances"),
            ("cloudwatch_logs", "logs:DescribeLogGroups"),
            ("secrets_read", "secretsmanager:GetSecretValue"),
            ("iam_discovery", "iam:ListRoles"),
            ("kms_discovery", "kms:ListKeys"),
            ("kms_crypto", "kms:Encrypt"),
            ("cost_explorer", "ce:GetCostAndUsage"),
            ("stepfunctions_execution", "states:StartExecution"),
            ("athena_query_execution", "athena:StartQueryExecution"),
        ]
        for intent_name, expected_action in cases:
            actual = CLOUD_INTENT_VERIFICATION_ACTIONS.get(intent_name)
            assert actual == expected_action, (
                f"Verification action mismatch for '{intent_name}': "
                f"expected '{expected_action}', got '{actual}'"
            )

    def test_count_matches_cloud_intent_enum(self):
        """CLOUD_INTENT_VERIFICATION_ACTIONS must have exactly one entry per CloudIntent member."""
        assert len(CLOUD_INTENT_VERIFICATION_ACTIONS) == len(CloudIntent._value2member_map_)


# ---------------------------------------------------------------------------
# OperatorCommandService._get_verification_action_for_intent
# ---------------------------------------------------------------------------

class TestGetVerificationActionForIntent:
    """_get_verification_action_for_intent delegates to CLOUD_INTENT_VERIFICATION_ACTIONS."""

    def test_returns_correct_action_for_known_intent(self):
        service = _make_service()
        assert service._intent_service._get_verification_action_for_intent("ec2_discovery") == "ec2:DescribeInstances"
        assert service._intent_service._get_verification_action_for_intent("s3_read") == "s3:GetObject"
        assert service._intent_service._get_verification_action_for_intent("lambda_invoke") == "lambda:InvokeFunction"
        assert service._intent_service._get_verification_action_for_intent("cost_explorer") == "ce:GetCostAndUsage"

    def test_returns_none_for_unknown_intent(self):
        service = _make_service()
        assert service._intent_service._get_verification_action_for_intent("totally_fake_intent") is None
        assert service._intent_service._get_verification_action_for_intent("") is None

    def test_returns_action_for_every_cloud_intent(self):
        """No CloudIntent member should produce None from this method."""
        service = _make_service()
        for value in CloudIntent._value2member_map_:
            result = service._intent_service._get_verification_action_for_intent(value)
            assert result is not None, (
                f"_get_verification_action_for_intent returned None for '{value}'"
            )
            assert ":" in result

    def test_result_matches_cloud_intent_verification_actions_map(self):
        """Method must return exactly the value from CLOUD_INTENT_VERIFICATION_ACTIONS."""
        service = _make_service()
        for name, expected_action in CLOUD_INTENT_VERIFICATION_ACTIONS.items():
            assert service._intent_service._get_verification_action_for_intent(name) == expected_action


# ---------------------------------------------------------------------------
# OperatorIntentService.execute_intent_permission_request — intent validation
# ---------------------------------------------------------------------------

class TestExecuteIntentPermissionRequestValidation:
    """Invalid-intent validation in execute_intent_permission_request uses CloudIntent enum."""

    def _make_cloud_investigation(self):
        from app.constants import OperatorType
        op_doc = MagicMock()
        op_doc.operator_type = OperatorType.CLOUD
        op_doc.operator_id = "op-cloud-1"
        op_doc.operator_session_id = "sess-cloud-1"
        investigation = MagicMock()
        investigation.id = "inv-001"
        investigation.operator_documents = [op_doc]
        return investigation

    async def test_rejects_unknown_intent_with_invalid_intent_error_type(self):
        service = _make_service()
        result = await service._intent_service.execute_intent_permission_request(
            args=GrantIntentArgs(intent_name="totally_fake_service_intent", justification="Test"),
            vso_context=_make_vso_context(),
            investigation=self._make_cloud_investigation(),
        )
        assert isinstance(result, IntentPermissionResult)
        assert result.success is False
        from app.constants import CommandErrorType
        assert result.error_type == CommandErrorType.INVALID_INTENT

    async def test_invalid_intent_error_names_the_bad_intent(self):
        service = _make_service()
        result = await service._intent_service.execute_intent_permission_request(
            args=GrantIntentArgs(intent_name="bad_intent_name", justification="Test"),
            vso_context=_make_vso_context(),
            investigation=self._make_cloud_investigation(),
        )
        assert result.error is not None
        assert "bad_intent_name" in result.error

    async def test_invalid_intent_error_lists_valid_intents(self):
        """Error message must enumerate valid intents from CloudIntent, not a hard-coded set."""
        service = _make_service()
        result = await service._intent_service.execute_intent_permission_request(
            args=GrantIntentArgs(intent_name="fake_intent", justification="Test"),
            vso_context=_make_vso_context(),
            investigation=self._make_cloud_investigation(),
        )
        assert result.error is not None
        assert "ec2_discovery" in result.error
        assert "s3_read" in result.error

    async def test_every_valid_cloud_intent_passes_validation(self):
        """No CloudIntent member should be rejected as invalid."""
        from app.constants import CommandErrorType, ApprovalType, FileOperation

        def _auto_deny(aid, pending):
            pending.resolve(approved=False, reason="test-deny")

        for value in CloudIntent._value2member_map_:
            service = _make_service()
            service._approval_service._pending_approvals.clear()
            service._approval_service.set_on_approval_requested(_auto_deny)

            service._approval_service.request_intent_approval = AsyncMock(
                return_value=MagicMock(approved=False, feedback=False, reason="test-deny", approval_id="app-123", error_type=None)
            )

            result = await service._intent_service.execute_intent_permission_request(
                args=GrantIntentArgs(intent_name=value, justification="Automated validation test"),
                vso_context=_make_vso_context(),
                investigation=self._make_cloud_investigation(),
            )
            assert result.error_type != CommandErrorType.INVALID_INTENT, (
                f"Valid CloudIntent '{value}' was incorrectly rejected as invalid"
            )

            service._approval_service._pending_approvals.clear()
            service._approval_service.set_on_approval_requested(None)

    async def test_rejects_empty_intent_name(self):
        service = _make_service()
        result = await service._intent_service.execute_intent_permission_request(
            args=GrantIntentArgs(intent_name="", justification="Test"),
            vso_context=_make_vso_context(),
            investigation=self._make_cloud_investigation(),
        )
        assert result.success is False

    async def test_rejects_missing_justification(self):
        service = _make_service()
        result = await service._intent_service.execute_intent_permission_request(
            args=GrantIntentArgs(intent_name="ec2_discovery", justification=""),
            vso_context=_make_vso_context(),
            investigation=self._make_cloud_investigation(),
        )
        assert result.success is False
        from app.constants import CommandErrorType
        assert result.error_type == CommandErrorType.VALIDATION_ERROR

    async def test_comma_separated_intents_all_validated(self):
        """Comma-separated intents: if any are invalid the whole request is rejected."""
        service = _make_service()
        result = await service._intent_service.execute_intent_permission_request(
            args=GrantIntentArgs(intent_name="ec2_discovery,completely_fake_intent", justification="Multi-intent test"),
            vso_context=_make_vso_context(),
            investigation=self._make_cloud_investigation(),
        )
        assert result.success is False
        from app.constants import CommandErrorType
        assert result.error_type == CommandErrorType.INVALID_INTENT
        assert result.error is not None
        assert "completely_fake_intent" in result.error

    async def test_dependency_intents_also_validated(self):
        """After dependency resolution, if a dep is somehow invalid it is caught."""
        from app.constants import CLOUD_INTENT_DEPENDENCIES
        service = _make_service()
        original_deps = CLOUD_INTENT_DEPENDENCIES.copy()
        try:
            CLOUD_INTENT_DEPENDENCIES["ec2_management"] = ["ec2_discovery", "fake_dep_intent"]
            result = await service._intent_service.execute_intent_permission_request(
                args=GrantIntentArgs(intent_name="ec2_management", justification="Test with injected bad dep"),
                vso_context=_make_vso_context(),
                investigation=self._make_cloud_investigation(),
            )
            assert result.success is False
            from app.constants import CommandErrorType
            assert result.error_type == CommandErrorType.INVALID_INTENT
            assert result.error is not None
            assert "fake_dep_intent" in result.error
        finally:
            CLOUD_INTENT_DEPENDENCIES.clear()
            CLOUD_INTENT_DEPENDENCIES.update(original_deps)

    async def test_non_cloud_operator_rejected_before_intent_validation(self):
        """Standard (non-cloud) operators are rejected before intent name checks."""
        from app.constants import OperatorType
        op_doc = MagicMock()
        op_doc.operator_type = OperatorType.SYSTEM
        op_doc.operator_id = "op-sys-1"
        op_doc.operator_session_id = "sess-sys-1"
        investigation = MagicMock()
        investigation.id = "inv-sys"
        investigation.operator_documents = [op_doc]

        service = _make_service()
        result = await service._intent_service.execute_intent_permission_request(
            args=GrantIntentArgs(intent_name="ec2_discovery", justification="Test"),
            vso_context=_make_vso_context(),
            investigation=investigation,
        )
        assert result.success is False
        from app.constants import CommandErrorType
        assert result.error_type == CommandErrorType.CLOUD_OPERATOR_REQUIRED
