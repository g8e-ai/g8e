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

"""Tests for pure synchronous helper methods on OperatorIntentService.

Separate from the async revocation/grant tests because module-level asyncio
pytestmark causes pytest-asyncio to fail on non-async test functions.
"""

import pytest

from app.services.operator.intent_service import OperatorIntentService
from tests.fakes.builder import build_intent_service

pytestmark = pytest.mark.unit


def _make_service() -> OperatorIntentService:
    return build_intent_service()


# =============================================================================
# IAM COMMAND BUILDER TESTS
# =============================================================================

class TestIamHelperMethods:
    """Test pure IAM command builder methods on OperatorIntentService."""

    def test_build_iam_detach_command_contains_intent_name(self):
        cmd = _make_service()._build_iam_detach_command("ec2_discovery")
        assert "ec2_discovery" in cmd

    def test_build_iam_detach_command_contains_detach_role_policy(self):
        cmd = _make_service()._build_iam_detach_command("s3_read")
        assert "detach-role-policy" in cmd

    def test_build_iam_detach_command_contains_assume_role(self):
        cmd = _make_service()._build_iam_detach_command("ec2_discovery")
        assert "assume-role" in cmd

    def test_build_iam_detach_command_is_unique_per_intent(self):
        svc = _make_service()
        exec_ec2 = svc._build_iam_detach_command("ec2_discovery")
        exec_s3 = svc._build_iam_detach_command("s3_read")
        assert exec_ec2 != exec_s3
        assert "ec2_discovery" in exec_ec2
        assert "s3_read" in exec_s3

    def test_build_iam_attach_command_contains_intent_name(self):
        cmd = _make_service()._build_iam_attach_command("ec2_discovery")
        assert "ec2_discovery" in cmd

    def test_build_iam_attach_command_contains_attach_role_policy(self):
        cmd = _make_service()._build_iam_attach_command("ec2_discovery")
        assert "attach-role-policy" in cmd

    def test_build_iam_attach_command_contains_assume_role(self):
        cmd = _make_service()._build_iam_attach_command("ec2_discovery")
        assert "assume-role" in cmd

    def test_build_iam_attach_command_is_unique_per_intent(self):
        svc = _make_service()
        exec_ec2 = svc._build_iam_attach_command("ec2_discovery")
        exec_s3 = svc._build_iam_attach_command("s3_read")
        assert exec_ec2 != exec_s3
        assert "ec2_discovery" in exec_ec2
        assert "s3_read" in exec_s3

    def test_build_iam_verify_command_contains_intent_action(self):
        cmd = _make_service()._build_iam_verify_command("ec2_discovery", "ec2:DescribeInstances")
        assert "ec2:DescribeInstances" in cmd
        assert "simulate-principal-policy" in cmd

    def test_build_iam_verify_command_exits_zero_on_timeout(self):
        """Verify command must always exit 0 (warning-only) to not block intent grant."""
        cmd = _make_service()._build_iam_verify_command("ec2_discovery", "ec2:DescribeInstances")
        assert "exit 0" in cmd

    def test_get_verification_action_returns_action_for_known_intent(self):
        action = _make_service()._get_verification_action_for_intent("ec2_discovery")
        assert action is not None
        assert "ec2:" in action

    def test_get_verification_action_returns_none_for_unknown_intent(self):
        action = _make_service()._get_verification_action_for_intent("nonexistent_intent_xyz")
        assert action is None

    def test_detach_and_attach_commands_differ_for_same_intent(self):
        """Attach and detach commands must be distinct — they perform opposite IAM operations."""
        svc = _make_service()
        attach = svc._build_iam_attach_command("ec2_discovery")
        detach = svc._build_iam_detach_command("ec2_discovery")
        assert attach != detach
        assert "attach-role-policy" in attach
        assert "detach-role-policy" in detach


# =============================================================================
# INTENT DEPENDENCY RESOLUTION TESTS
# =============================================================================

class TestIntentDependencyResolution:
    """Test _resolve_intent_dependencies pure logic."""

    def test_no_dependencies_returns_same_intent(self):
        result = _make_service()._resolve_intent_dependencies(["ec2_discovery"])
        assert "ec2_discovery" in result

    def test_dependent_intent_pulls_in_prerequisite(self):
        """ec2_management requires ec2_discovery — both must appear in the resolved list."""
        result = _make_service()._resolve_intent_dependencies(["ec2_management"])
        assert "ec2_management" in result
        assert "ec2_discovery" in result

    def test_already_included_dependency_not_duplicated(self):
        result = _make_service()._resolve_intent_dependencies(["ec2_management", "ec2_discovery"])
        assert result.count("ec2_discovery") == 1

    def test_multiple_independent_intents_all_included(self):
        result = _make_service()._resolve_intent_dependencies(["ec2_discovery", "s3_read"])
        assert "ec2_discovery" in result
        assert "s3_read" in result

    def test_result_is_sorted(self):
        result = _make_service()._resolve_intent_dependencies(["s3_read", "ec2_discovery"])
        assert result == sorted(result)

    def test_empty_input_returns_empty(self):
        result = _make_service()._resolve_intent_dependencies([])
        assert result == []

    def test_transitive_dependencies_resolved(self):
        """If B depends on A and C depends on B, requesting C must include A and B."""
        svc = _make_service()
        result = svc._resolve_intent_dependencies(["ec2_management"])
        assert len(result) >= 2

    def test_single_intent_no_deps_count(self):
        """ec2_discovery has no dependencies — result length must be exactly 1."""
        result = _make_service()._resolve_intent_dependencies(["ec2_discovery"])
        assert len(result) == 1
