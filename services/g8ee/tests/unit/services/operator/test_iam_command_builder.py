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

import pytest

from app.services.operator.iam_command_builder import IamCommandBuilder

pytestmark = [pytest.mark.unit]


@pytest.fixture
def builder() -> IamCommandBuilder:
    return IamCommandBuilder()


class TestBuildAttachCommand:
    def test_returns_string(self, builder):
        result = builder.build_attach_command("ec2_discovery")
        assert isinstance(result, str)

    def test_contains_intent_in_session_name(self, builder):
        result = builder.build_attach_command("ec2_discovery")
        assert "g8e-intent-grant-ec2_discovery" in result

    def test_contains_intent_in_policy_arn(self, builder):
        result = builder.build_attach_command("s3_read")
        assert "Intent-s3_read" in result

    def test_contains_attach_role_policy(self, builder):
        result = builder.build_attach_command("ec2_discovery")
        assert "attach-role-policy" in result

    def test_contains_escalation_role_assume(self, builder):
        result = builder.build_attach_command("ec2_discovery")
        assert "assume-role" in result

    def test_contains_success_message_with_intent(self, builder):
        result = builder.build_attach_command("cloudwatch_logs")
        assert "Intent policy cloudwatch_logs attached" in result

    def test_contains_set_e(self, builder):
        result = builder.build_attach_command("ec2_discovery")
        assert result.startswith("set -e")

    def test_different_intents_produce_different_commands(self, builder):
        assert builder.build_attach_command("ec2_discovery") != builder.build_attach_command("s3_read")

    def test_contains_root_guard(self, builder):
        result = builder.build_attach_command("ec2_discovery")
        assert "Running as root, skipping policy update" in result

    def test_contains_iam_user_error(self, builder):
        result = builder.build_attach_command("ec2_discovery")
        assert "not a role (likely IAM User)" in result

    def test_unsets_credentials_after_use(self, builder):
        result = builder.build_attach_command("ec2_discovery")
        assert "unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN" in result


class TestBuildDetachCommand:
    def test_returns_string(self, builder):
        result = builder.build_detach_command("s3_read")
        assert isinstance(result, str)

    def test_contains_intent_in_session_name(self, builder):
        result = builder.build_detach_command("s3_read")
        assert "g8e-intent-revoke-s3_read" in result

    def test_contains_intent_in_policy_arn(self, builder):
        result = builder.build_detach_command("ec2_discovery")
        assert "Intent-ec2_discovery" in result

    def test_contains_detach_role_policy(self, builder):
        result = builder.build_detach_command("s3_read")
        assert "detach-role-policy" in result

    def test_contains_escalation_role_assume(self, builder):
        result = builder.build_detach_command("s3_read")
        assert "assume-role" in result

    def test_contains_success_message_with_intent(self, builder):
        result = builder.build_detach_command("s3_read")
        assert "Intent policy s3_read detached" in result

    def test_contains_set_e(self, builder):
        result = builder.build_detach_command("s3_read")
        assert result.startswith("set -e")

    def test_different_intents_produce_different_commands(self, builder):
        assert builder.build_detach_command("s3_read") != builder.build_detach_command("rds_management")

    def test_contains_root_guard(self, builder):
        result = builder.build_detach_command("s3_read")
        assert "Running as root, skipping policy detach" in result

    def test_gracefully_handles_already_detached(self, builder):
        result = builder.build_detach_command("s3_read")
        assert "Policy may already be detached" in result

    def test_unsets_credentials_after_use(self, builder):
        result = builder.build_detach_command("s3_read")
        assert "unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN" in result

    def test_detach_differs_from_attach_for_same_intent(self, builder):
        assert builder.build_detach_command("ec2_discovery") != builder.build_attach_command("ec2_discovery")


class TestBuildVerifyCommand:
    def test_returns_string(self, builder):
        result = builder.build_verify_command("ec2_discovery", "ec2:DescribeInstances")
        assert isinstance(result, str)

    def test_contains_verification_action(self, builder):
        result = builder.build_verify_command("ec2_discovery", "ec2:DescribeInstances")
        assert "ec2:DescribeInstances" in result

    def test_contains_simulate_principal_policy(self, builder):
        result = builder.build_verify_command("ec2_discovery", "ec2:DescribeInstances")
        assert "simulate-principal-policy" in result

    def test_contains_success_message_with_action(self, builder):
        result = builder.build_verify_command("s3_read", "s3:GetObject")
        assert "Permission s3:GetObject is now active" in result

    def test_contains_retry_loop(self, builder):
        result = builder.build_verify_command("ec2_discovery", "ec2:DescribeInstances")
        assert "MAX_ATTEMPTS=10" in result
        assert "while [ $ATTEMPT -lt $MAX_ATTEMPTS ]" in result

    def test_contains_set_e(self, builder):
        result = builder.build_verify_command("ec2_discovery", "ec2:DescribeInstances")
        assert result.startswith("set -e")

    def test_exits_zero_on_timeout(self, builder):
        result = builder.build_verify_command("ec2_discovery", "ec2:DescribeInstances")
        assert "Permission verification timed out, proceeding anyway" in result
        assert "exit 0" in result

    def test_different_actions_produce_different_commands(self, builder):
        a = builder.build_verify_command("ec2_discovery", "ec2:DescribeInstances")
        b = builder.build_verify_command("s3_read", "s3:GetObject")
        assert a != b

    def test_verify_differs_from_attach(self, builder):
        attach = builder.build_attach_command("ec2_discovery")
        verify = builder.build_verify_command("ec2_discovery", "ec2:DescribeInstances")
        assert attach != verify
