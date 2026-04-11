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

import uuid

from app.constants import (
    APPROVAL_ID_PREFIX,
    EXECUTION_ID_PREFIX,
    FILE_EDIT_EXECUTION_ID_PREFIX,
    IAM_EXECUTION_ID_PREFIX,
    IAM_PENDING_EXECUTION_ID_PREFIX,
    IAM_REVOKE_EXECUTION_ID_PREFIX,
    IAM_REVOKE_INTENT_EXECUTION_ID_PREFIX,
    IAM_VERIFY_EXECUTION_ID_PREFIX,
    INTENT_APPROVAL_ID_PREFIX,
    INTENT_EXECUTION_ID_PREFIX,
)
from app.utils.timestamp import now, to_timestamp


def _ts() -> int:
    return int(to_timestamp(now()))


def _hex() -> str:
    return uuid.uuid4().hex[:12]


def generate_execution_id() -> str:
    """Generate a unique execution tracking ID for G8eHttpContext."""
    return f"exec_{_hex()}"


def generate_approval_id() -> str:
    """Generate a unique approval ID for command and file edit approval requests."""
    return f"{APPROVAL_ID_PREFIX}_{_hex()}_{_ts()}"


def generate_intent_approval_id() -> str:
    """Generate a unique approval ID for intent permission approval requests."""
    return f"{INTENT_APPROVAL_ID_PREFIX}_{_hex()}_{_ts()}"


def generate_command_execution_id() -> str:
    """Generate a unique execution ID for operator command executions."""
    return f"{EXECUTION_ID_PREFIX}_{_hex()}_{_ts()}"


def generate_file_edit_execution_id() -> str:
    """Generate a unique execution ID for file edit operations."""
    return f"{FILE_EDIT_EXECUTION_ID_PREFIX}_{_hex()}_{_ts()}"


def generate_intent_execution_id() -> str:
    """Generate a unique execution ID for intent permission operations."""
    return f"{INTENT_EXECUTION_ID_PREFIX}_{_hex()}_{_ts()}"


def generate_iam_execution_id(intent: str) -> str:
    """Generate a unique execution ID for an IAM policy attach operation."""
    return f"{IAM_EXECUTION_ID_PREFIX}_{intent}_{_hex()}"


def generate_iam_verify_execution_id(intent: str) -> str:
    """Generate a unique execution ID for an IAM policy verification operation."""
    return f"{IAM_VERIFY_EXECUTION_ID_PREFIX}_{intent}_{_hex()[:8]}"


def generate_iam_pending_execution_id() -> str:
    """Generate a unique execution ID for a pending IAM operation."""
    return f"{IAM_PENDING_EXECUTION_ID_PREFIX}_{_hex()}"


def generate_iam_revoke_execution_id() -> str:
    """Generate a unique execution ID for an IAM policy revoke operation."""
    return f"{IAM_REVOKE_EXECUTION_ID_PREFIX}_{_hex()}"


def generate_iam_revoke_intent_execution_id(intent: str) -> str:
    """Generate a unique execution ID for revoking a specific IAM intent policy."""
    return f"{IAM_REVOKE_INTENT_EXECUTION_ID_PREFIX}_{intent}_{_hex()}"
