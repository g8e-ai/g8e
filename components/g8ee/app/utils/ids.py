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


# Canonical shape of a command execution ID: ``cmd_{hex12}_{unix_timestamp}``.
# Exposed as a module constant so event consumers (frontend routers, cross-
# component wire validators) have a single source of truth instead of
# rebuilding the pattern ad-hoc. ``\d{9,11}`` bounds the timestamp to a
# plausible Unix epoch window without over-constraining the format across
# clock-drift scenarios.
COMMAND_EXECUTION_ID_PATTERN = re.compile(
    rf"^{re.escape(EXECUTION_ID_PREFIX)}_[0-9a-f]{{12}}_\d{{9,11}}$"
)


def is_valid_command_execution_id(value: str) -> bool:
    """Return True iff ``value`` matches the canonical command execution ID shape."""
    return isinstance(value, str) and COMMAND_EXECUTION_ID_PATTERN.match(value) is not None


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
    """Generate a unique execution ID for operator command executions.

    The output shape is pinned by ``COMMAND_EXECUTION_ID_PATTERN`` and
    asserted here so that a future drift in ``_hex`` / ``_ts`` (e.g. a
    truncated uuid slice, a ms-timestamp migration) fails loudly at the
    generator instead of leaking malformed IDs into the approval pipeline.
    """
    candidate = f"{EXECUTION_ID_PREFIX}_{_hex()}_{_ts()}"
    if not COMMAND_EXECUTION_ID_PATTERN.match(candidate):
        raise RuntimeError(
            f"generate_command_execution_id produced an ID that violates "
            f"COMMAND_EXECUTION_ID_PATTERN: {candidate!r}"
        )
    return candidate


def generate_batch_id() -> str:
    """Generate a unique batch ID correlating per-operator executions from a single approval."""
    return f"batch_{_hex()}_{_ts()}"


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
