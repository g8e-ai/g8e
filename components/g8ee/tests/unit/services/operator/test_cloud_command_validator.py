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

import re

import pytest

from app.services.operator.cloud_command_validator import (
    CLOUD_ONLY_COMMAND_PATTERNS,
    is_cloud_only_command,
)

pytestmark = pytest.mark.unit


class TestIsCloudOnlyCommand:

    @pytest.mark.parametrize("command", [
        "aws s3 ls",
        "aws sts get-caller-identity",
        "gcloud compute instances list",
        "gsutil cp file gs://bucket",
        "bq query --use_legacy_sql=false 'SELECT 1'",
        "az vm list",
        "kubectl get pods",
        "helm install myrelease ./chart",
        "k9s",
        "kubectx production",
        "kubens kube-system",
        "terraform plan",
        "tofu apply",
        "pulumi up",
        "ansible all -m ping",
        "ansible-playbook site.yml",
        "eksctl create cluster",
        "sam deploy",
        "cdk deploy",
        "serverless deploy",
    ])
    def test_cloud_commands_are_detected(self, command):
        assert is_cloud_only_command(command) is True

    @pytest.mark.parametrize("command", [
        "ls -la",
        "cat /etc/hosts",
        "systemctl status nginx",
        "docker ps",
        "ps aux",
        "grep -r error /var/log",
        "tail -f /var/log/syslog",
        "echo hello",
        "python3 script.py",
        "curl https://example.com",
    ])
    def test_non_cloud_commands_are_not_detected(self, command):
        assert is_cloud_only_command(command) is False

    def test_strips_leading_whitespace(self):
        assert is_cloud_only_command("  aws s3 ls") is True

    def test_partial_match_does_not_trigger(self):
        assert is_cloud_only_command("echo aws") is False
        assert is_cloud_only_command("cat terraform.tfvars") is False

    def test_empty_command_is_false(self):
        assert is_cloud_only_command("") is False

    def test_k9s_with_trailing_content(self):
        assert is_cloud_only_command("k9s --context prod") is True



class TestPatternListsNotEmpty:

    def test_cloud_only_patterns_not_empty(self):
        assert len(CLOUD_ONLY_COMMAND_PATTERNS) > 0

    def test_all_patterns_are_compiled(self):
        for pattern in CLOUD_ONLY_COMMAND_PATTERNS:
            assert isinstance(pattern, re.Pattern)
