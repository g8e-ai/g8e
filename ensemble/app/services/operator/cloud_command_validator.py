# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Cloud Command Validator

Pattern-based checks for cloud operator command routing and auto-approval rules.
"""

import re

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
