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
Cloud Command Validator

Pattern-based checks for cloud operator command routing and auto-approval rules.
"""

import re
from app.constants import CloudSubtype

CLOUD_ONLY_COMMAND_PATTERNS: list[re.Pattern[str]] = [
    re.compile(r"^aws\s"),
    re.compile(r"^gcloud\s"),
    re.compile(r"^gsutil\s"),
    re.compile(r"^bq\s"),
    re.compile(r"^az\s"),
    re.compile(r"^kubectl\s"),
    re.compile(r"^helm\s"),
    re.compile(r"^k9s\b"),
    re.compile(r"^kubectx\b"),
    re.compile(r"^kubens\b"),
    re.compile(r"^terraform\s"),
    re.compile(r"^tofu\s"),
    re.compile(r"^pulumi\s"),
    re.compile(r"^ansible\b"),
    re.compile(r"^ansible-playbook\s"),
    re.compile(r"^eksctl\s"),
    re.compile(r"^sam\s"),
    re.compile(r"^cdk\s"),
    re.compile(r"^serverless\s"),
]


def is_cloud_only_command(command: str) -> bool:
    """Return True if the command requires a Cloud Operator (operator_type: cloud)."""
    command = command.strip()
    return any(p.match(command) for p in CLOUD_ONLY_COMMAND_PATTERNS)
