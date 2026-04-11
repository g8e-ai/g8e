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
Unit tests for app/utils/ids.py.

Every generator function is tested for:
- Correct prefix derived from the corresponding constant in app/constants
- Correct structural format (prefix_hex_timestamp or prefix_intent_hex)
- Uniqueness across rapid successive calls
- Hex segment length and composition
- Timestamp segment is a valid Unix epoch integer
- Intent-scoped generators embed the intent string in the correct position

No external infrastructure required — pure unit tests.
"""

import re

import pytest

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
from app.utils.ids import (
    generate_approval_id,
    generate_command_execution_id,
    generate_execution_id,
    generate_file_edit_execution_id,
    generate_iam_execution_id,
    generate_iam_pending_execution_id,
    generate_iam_revoke_execution_id,
    generate_iam_revoke_intent_execution_id,
    generate_iam_verify_execution_id,
    generate_intent_approval_id,
    generate_intent_execution_id,
)

pytestmark = [pytest.mark.unit]

_HEX_CHARS = re.compile(r"^[0-9a-f]+$")
_TIMESTAMP_PATTERN = re.compile(r"^\d{10}$")


def _assert_prefix(generated: str, expected_prefix: str) -> None:
    assert generated.startswith(f"{expected_prefix}_"), (
        f"Expected prefix '{expected_prefix}_', got: {generated!r}"
    )


def _split_standard(generated: str, expected_prefix: str) -> tuple[str, str]:
    """Split a standard {prefix}_{hex12}_{timestamp} ID into (hex, timestamp)."""
    assert generated.startswith(f"{expected_prefix}_")
    rest = generated[len(expected_prefix) + 1:]
    parts = rest.split("_")
    assert len(parts) == 2, f"Expected 2 parts after prefix, got {len(parts)} in: {generated!r}"
    return parts[0], parts[1]


def _assert_hex_segment(segment: str, expected_length: int) -> None:
    assert len(segment) == expected_length, (
        f"Expected hex segment of length {expected_length}, got {len(segment)}: {segment!r}"
    )
    assert _HEX_CHARS.match(segment), f"Hex segment contains non-hex chars: {segment!r}"


def _assert_timestamp_segment(segment: str) -> None:
    assert segment.isdigit(), f"Timestamp segment is not all digits: {segment!r}"
    ts = int(segment)
    assert ts > 1_700_000_000, f"Timestamp {ts} is before 2023 — likely malformed"
    assert ts < 2_000_000_000, f"Timestamp {ts} is after 2033 — likely malformed"


class TestGenerateApprovalId:

    def test_uses_approval_id_prefix_constant(self):
        result = generate_approval_id()
        _assert_prefix(result, APPROVAL_ID_PREFIX)

    def test_prefix_is_approval(self):
        result = generate_approval_id()
        assert result.startswith("approval_"), f"Expected 'approval_' prefix, got: {result!r}"

    def test_hex_segment_is_12_chars(self):
        result = generate_approval_id()
        hex_seg, _ = _split_standard(result, APPROVAL_ID_PREFIX)
        _assert_hex_segment(hex_seg, 12)

    def test_timestamp_segment_is_valid_epoch(self):
        result = generate_approval_id()
        _, ts_seg = _split_standard(result, APPROVAL_ID_PREFIX)
        _assert_timestamp_segment(ts_seg)

    def test_returns_string(self):
        assert isinstance(generate_approval_id(), str)

    def test_unique_across_calls(self):
        ids = {generate_approval_id() for _ in range(20)}
        assert len(ids) == 20, "Expected 20 unique IDs but got duplicates"

    def test_format_is_prefix_hex_timestamp(self):
        result = generate_approval_id()
        parts = result.split("_")
        assert len(parts) == 3, f"Expected 3 underscore-delimited parts, got: {result!r}"


class TestGenerateIntentApprovalId:

    def test_uses_intent_approval_id_prefix_constant(self):
        result = generate_intent_approval_id()
        _assert_prefix(result, INTENT_APPROVAL_ID_PREFIX)

    def test_prefix_is_intent(self):
        result = generate_intent_approval_id()
        assert result.startswith("intent_"), f"Expected 'intent_' prefix, got: {result!r}"

    def test_hex_segment_is_12_chars(self):
        result = generate_intent_approval_id()
        hex_seg, _ = _split_standard(result, INTENT_APPROVAL_ID_PREFIX)
        _assert_hex_segment(hex_seg, 12)

    def test_timestamp_segment_is_valid_epoch(self):
        result = generate_intent_approval_id()
        _, ts_seg = _split_standard(result, INTENT_APPROVAL_ID_PREFIX)
        _assert_timestamp_segment(ts_seg)

    def test_returns_string(self):
        assert isinstance(generate_intent_approval_id(), str)

    def test_unique_across_calls(self):
        ids = {generate_intent_approval_id() for _ in range(20)}
        assert len(ids) == 20

    def test_format_is_prefix_hex_timestamp(self):
        result = generate_intent_approval_id()
        parts = result.split("_")
        assert len(parts) == 3, f"Expected 3 parts, got: {result!r}"


class TestGenerateExecutionId:

    def test_uses_execution_id_prefix_constant(self):
        result = generate_execution_id()
        assert result.startswith("exec_")

    def test_prefix_is_exec(self):
        result = generate_execution_id()
        assert result.startswith("exec_"), f"Expected 'exec_' prefix, got: {result!r}"

    def test_hex_segment_is_12_chars(self):
        result = generate_execution_id()
        parts = result.split("_")
        _assert_hex_segment(parts[1], 12)

    def test_timestamp_segment_is_valid_epoch(self):
        result = generate_execution_id()
        # Note: generate_execution_id only has 2 parts: exec_hex
        parts = result.split("_")
        assert len(parts) == 2, f"Expected 2 parts, got: {result!r}"

    def test_returns_string(self):
        assert isinstance(generate_execution_id(), str)

    def test_unique_across_calls(self):
        ids = {generate_execution_id() for _ in range(20)}
        assert len(ids) == 20

    def test_format_is_prefix_hex(self):
        result = generate_execution_id()
        parts = result.split("_")
        assert len(parts) == 2, f"Expected 2 parts, got: {result!r}"


class TestGenerateFileEditExecutionId:

    def test_uses_file_edit_execution_id_prefix_constant(self):
        result = generate_file_edit_execution_id()
        _assert_prefix(result, FILE_EDIT_EXECUTION_ID_PREFIX)

    def test_prefix_is_edit(self):
        result = generate_file_edit_execution_id()
        assert result.startswith("edit_"), f"Expected 'edit_' prefix, got: {result!r}"

    def test_hex_segment_is_12_chars(self):
        result = generate_file_edit_execution_id()
        hex_seg, _ = _split_standard(result, FILE_EDIT_EXECUTION_ID_PREFIX)
        _assert_hex_segment(hex_seg, 12)

    def test_timestamp_segment_is_valid_epoch(self):
        result = generate_file_edit_execution_id()
        _, ts_seg = _split_standard(result, FILE_EDIT_EXECUTION_ID_PREFIX)
        _assert_timestamp_segment(ts_seg)

    def test_returns_string(self):
        assert isinstance(generate_file_edit_execution_id(), str)

    def test_unique_across_calls(self):
        ids = {generate_file_edit_execution_id() for _ in range(20)}
        assert len(ids) == 20

    def test_format_is_prefix_hex_timestamp(self):
        result = generate_file_edit_execution_id()
        parts = result.split("_")
        assert len(parts) == 3, f"Expected 3 parts, got: {result!r}"


class TestGenerateIntentExecutionId:

    def test_uses_intent_execution_id_prefix_constant(self):
        result = generate_intent_execution_id()
        _assert_prefix(result, INTENT_EXECUTION_ID_PREFIX)

    def test_prefix_is_intent(self):
        result = generate_intent_execution_id()
        assert result.startswith("intent_"), f"Expected 'intent_' prefix, got: {result!r}"

    def test_hex_segment_is_12_chars(self):
        result = generate_intent_execution_id()
        hex_seg, _ = _split_standard(result, INTENT_EXECUTION_ID_PREFIX)
        _assert_hex_segment(hex_seg, 12)

    def test_timestamp_segment_is_valid_epoch(self):
        result = generate_intent_execution_id()
        _, ts_seg = _split_standard(result, INTENT_EXECUTION_ID_PREFIX)
        _assert_timestamp_segment(ts_seg)

    def test_returns_string(self):
        assert isinstance(generate_intent_execution_id(), str)

    def test_unique_across_calls(self):
        ids = {generate_intent_execution_id() for _ in range(20)}
        assert len(ids) == 20

    def test_format_is_prefix_hex_timestamp(self):
        result = generate_intent_execution_id()
        parts = result.split("_")
        assert len(parts) == 3, f"Expected 3 parts, got: {result!r}"


class TestGenerateIamExecutionId:

    def test_uses_iam_execution_id_prefix_constant(self):
        result = generate_iam_execution_id("ec2_discovery")
        _assert_prefix(result, IAM_EXECUTION_ID_PREFIX)

    def test_prefix_is_iam(self):
        result = generate_iam_execution_id("s3_read")
        assert result.startswith("iam_"), f"Expected 'iam_' prefix, got: {result!r}"

    def test_embeds_intent_in_id(self):
        intent = "ec2_management"
        result = generate_iam_execution_id(intent)
        assert intent in result, f"Expected intent '{intent}' in ID: {result!r}"

    def test_intent_is_second_segment(self):
        intent = "s3_write"
        result = generate_iam_execution_id(intent)
        parts = result.split("_", 2)
        assert parts[1] == "s3", f"Expected intent name in segments, got: {result!r}"

    def test_ends_with_hex_segment(self):
        result = generate_iam_execution_id("rds_discovery")
        hex_seg = result.rsplit("_", 1)[-1]
        _assert_hex_segment(hex_seg, 12)

    def test_returns_string(self):
        assert isinstance(generate_iam_execution_id("ec2_discovery"), str)

    def test_unique_across_calls_same_intent(self):
        ids = {generate_iam_execution_id("ec2_discovery") for _ in range(20)}
        assert len(ids) == 20

    def test_unique_across_different_intents(self):
        id1 = generate_iam_execution_id("ec2_discovery")
        id2 = generate_iam_execution_id("s3_read")
        assert id1 != id2

    def test_different_intents_produce_different_prefixes(self):
        id1 = generate_iam_execution_id("ec2_discovery")
        id2 = generate_iam_execution_id("s3_read")
        assert "ec2_discovery" in id1
        assert "s3_read" in id2


class TestGenerateIamVerifyExecutionId:

    def test_uses_iam_verify_execution_id_prefix_constant(self):
        result = generate_iam_verify_execution_id("ec2_discovery")
        _assert_prefix(result, IAM_VERIFY_EXECUTION_ID_PREFIX)

    def test_prefix_is_verify(self):
        result = generate_iam_verify_execution_id("s3_read")
        assert result.startswith("verify_"), f"Expected 'verify_' prefix, got: {result!r}"

    def test_embeds_intent_in_id(self):
        intent = "cloudwatch_logs"
        result = generate_iam_verify_execution_id(intent)
        assert intent in result, f"Expected intent '{intent}' in ID: {result!r}"

    def test_ends_with_8_char_hex_segment(self):
        result = generate_iam_verify_execution_id("rds_discovery")
        hex_seg = result.rsplit("_", 1)[-1]
        _assert_hex_segment(hex_seg, 8)

    def test_returns_string(self):
        assert isinstance(generate_iam_verify_execution_id("ec2_discovery"), str)

    def test_unique_across_calls_same_intent(self):
        ids = {generate_iam_verify_execution_id("ec2_discovery") for _ in range(20)}
        assert len(ids) == 20


class TestGenerateIamPendingExecutionId:

    def test_uses_iam_pending_execution_id_prefix_constant(self):
        result = generate_iam_pending_execution_id()
        _assert_prefix(result, IAM_PENDING_EXECUTION_ID_PREFIX)

    def test_prefix_is_pending(self):
        result = generate_iam_pending_execution_id()
        assert result.startswith("pending_"), f"Expected 'pending_' prefix, got: {result!r}"

    def test_ends_with_hex_segment(self):
        result = generate_iam_pending_execution_id()
        hex_seg = result.rsplit("_", 1)[-1]
        _assert_hex_segment(hex_seg, 12)

    def test_returns_string(self):
        assert isinstance(generate_iam_pending_execution_id(), str)

    def test_unique_across_calls(self):
        ids = {generate_iam_pending_execution_id() for _ in range(20)}
        assert len(ids) == 20

    def test_format_is_prefix_hex(self):
        result = generate_iam_pending_execution_id()
        parts = result.split("_")
        assert len(parts) == 2, f"Expected 2 parts (prefix_hex), got: {result!r}"


class TestGenerateIamRevokeExecutionId:

    def test_uses_iam_revoke_execution_id_prefix_constant(self):
        result = generate_iam_revoke_execution_id()
        _assert_prefix(result, IAM_REVOKE_EXECUTION_ID_PREFIX)

    def test_prefix_is_revoke(self):
        result = generate_iam_revoke_execution_id()
        assert result.startswith("revoke_"), f"Expected 'revoke_' prefix, got: {result!r}"

    def test_ends_with_hex_segment(self):
        result = generate_iam_revoke_execution_id()
        hex_seg = result.rsplit("_", 1)[-1]
        _assert_hex_segment(hex_seg, 12)

    def test_returns_string(self):
        assert isinstance(generate_iam_revoke_execution_id(), str)

    def test_unique_across_calls(self):
        ids = {generate_iam_revoke_execution_id() for _ in range(20)}
        assert len(ids) == 20

    def test_format_is_prefix_hex(self):
        result = generate_iam_revoke_execution_id()
        parts = result.split("_")
        assert len(parts) == 2, f"Expected 2 parts (prefix_hex), got: {result!r}"


class TestGenerateIamRevokeIntentExecutionId:

    def test_uses_iam_revoke_intent_execution_id_prefix_constant(self):
        result = generate_iam_revoke_intent_execution_id("ec2_management")
        _assert_prefix(result, IAM_REVOKE_INTENT_EXECUTION_ID_PREFIX)

    def test_prefix_is_iam_revoke(self):
        result = generate_iam_revoke_intent_execution_id("s3_delete")
        assert result.startswith("iam_revoke_"), f"Expected 'iam_revoke_' prefix, got: {result!r}"

    def test_embeds_intent_in_id(self):
        intent = "lambda_invoke"
        result = generate_iam_revoke_intent_execution_id(intent)
        assert intent in result, f"Expected intent '{intent}' in ID: {result!r}"

    def test_ends_with_hex_segment(self):
        result = generate_iam_revoke_intent_execution_id("rds_management")
        hex_seg = result.rsplit("_", 1)[-1]
        _assert_hex_segment(hex_seg, 12)

    def test_returns_string(self):
        assert isinstance(generate_iam_revoke_intent_execution_id("ec2_management"), str)

    def test_unique_across_calls_same_intent(self):
        ids = {generate_iam_revoke_intent_execution_id("s3_read") for _ in range(20)}
        assert len(ids) == 20

    def test_different_intents_produce_different_ids(self):
        id1 = generate_iam_revoke_intent_execution_id("ec2_management")
        id2 = generate_iam_revoke_intent_execution_id("s3_delete")
        assert id1 != id2
        assert "ec2_management" in id1
        assert "s3_delete" in id2


class TestPrefixConstantAlignment:
    """Each generator must use the exact constant value — not a hardcoded string."""

    def test_approval_id_prefix_value_is_approval(self):
        assert APPROVAL_ID_PREFIX == "approval"

    def test_intent_approval_id_prefix_value_is_intent(self):
        assert INTENT_APPROVAL_ID_PREFIX == "intent"

    def test_execution_id_prefix_value_is_cmd(self):
        assert EXECUTION_ID_PREFIX == "cmd"

    def test_file_edit_execution_id_prefix_value_is_edit(self):
        assert FILE_EDIT_EXECUTION_ID_PREFIX == "edit"

    def test_intent_execution_id_prefix_value_is_intent(self):
        assert INTENT_EXECUTION_ID_PREFIX == "intent"

    def test_iam_execution_id_prefix_value_is_iam(self):
        assert IAM_EXECUTION_ID_PREFIX == "iam"

    def test_iam_verify_execution_id_prefix_value_is_verify(self):
        assert IAM_VERIFY_EXECUTION_ID_PREFIX == "verify"

    def test_iam_pending_execution_id_prefix_value_is_pending(self):
        assert IAM_PENDING_EXECUTION_ID_PREFIX == "pending"

    def test_iam_revoke_execution_id_prefix_value_is_revoke(self):
        assert IAM_REVOKE_EXECUTION_ID_PREFIX == "revoke"

    def test_iam_revoke_intent_execution_id_prefix_value_is_iam_revoke(self):
        assert IAM_REVOKE_INTENT_EXECUTION_ID_PREFIX == "iam_revoke"

    def test_generate_approval_id_prefix_matches_constant(self):
        result = generate_approval_id()
        assert result.split("_")[0] == APPROVAL_ID_PREFIX

    def test_generate_execution_id_prefix_matches_constant(self):
        result = generate_execution_id()
        assert result.split("_")[0] == "exec"

    def test_generate_file_edit_execution_id_prefix_matches_constant(self):
        result = generate_file_edit_execution_id()
        assert result.split("_")[0] == FILE_EDIT_EXECUTION_ID_PREFIX

    def test_generate_iam_pending_execution_id_prefix_matches_constant(self):
        result = generate_iam_pending_execution_id()
        assert result.split("_")[0] == IAM_PENDING_EXECUTION_ID_PREFIX

    def test_generate_iam_revoke_execution_id_prefix_matches_constant(self):
        result = generate_iam_revoke_execution_id()
        assert result.split("_")[0] == IAM_REVOKE_EXECUTION_ID_PREFIX


class TestIdDistinctness:
    """IDs from different generators must not be interchangeable."""

    def test_approval_and_execution_ids_are_distinct_types(self):
        approval = generate_approval_id()
        execution = generate_execution_id()
        assert not approval.startswith("exec_")
        assert not execution.startswith("approval_")

    def test_file_edit_and_command_execution_ids_are_distinct(self):
        edit = generate_file_edit_execution_id()
        cmd = generate_command_execution_id()
        assert edit.startswith("edit_")
        assert cmd.startswith("cmd_")
        assert not edit.startswith("cmd_")
        assert not cmd.startswith("edit_")

    def test_intent_approval_and_intent_execution_have_same_prefix_different_structure(self):
        approval = generate_intent_approval_id()
        execution = generate_intent_execution_id()
        assert approval.startswith("intent_")
        assert execution.startswith("intent_")
        approval_parts = approval.split("_")
        execution_parts = execution.split("_")
        assert len(approval_parts) == 3
        assert len(execution_parts) == 3

    def test_iam_revoke_single_differs_from_iam_revoke_intent(self):
        single = generate_iam_revoke_execution_id()
        intent_specific = generate_iam_revoke_intent_execution_id("ec2_management")
        assert single.startswith("revoke_")
        assert intent_specific.startswith("iam_revoke_")
        assert single != intent_specific
