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

"""Tests for UAP JSON envelope decoder and enum conversion."""

import pytest
import json

from app.constants import ExecutionStatus, EventType
from app.proto import operator_pb2
from app.utils.envelope_builder import (
    decode_uap_envelope,
    decode_g8eo_result_envelope,
    protobuf_execution_status_to_python,
)

pytestmark = [pytest.mark.unit]


class TestProtobufExecutionStatusToPython:
    """Test protobuf enum to Python ExecutionStatus string conversion."""

    def test_convert_unspecified_to_pending(self):
        """EXECUTION_STATUS_UNSPECIFIED maps to PENDING."""
        result = protobuf_execution_status_to_python(operator_pb2.EXECUTION_STATUS_UNSPECIFIED)
        assert result == ExecutionStatus.PENDING

    def test_convert_executing(self):
        """EXECUTION_STATUS_EXECUTING maps to EXECUTING."""
        result = protobuf_execution_status_to_python(operator_pb2.EXECUTION_STATUS_EXECUTING)
        assert result == ExecutionStatus.EXECUTING

    def test_convert_completed(self):
        """EXECUTION_STATUS_COMPLETED maps to COMPLETED."""
        result = protobuf_execution_status_to_python(operator_pb2.EXECUTION_STATUS_COMPLETED)
        assert result == ExecutionStatus.COMPLETED

    def test_convert_failed(self):
        """EXECUTION_STATUS_FAILED maps to FAILED."""
        result = protobuf_execution_status_to_python(operator_pb2.EXECUTION_STATUS_FAILED)
        assert result == ExecutionStatus.FAILED

    def test_convert_cancelled(self):
        """EXECUTION_STATUS_CANCELLED maps to CANCELLED."""
        result = protobuf_execution_status_to_python(operator_pb2.EXECUTION_STATUS_CANCELLED)
        assert result == ExecutionStatus.CANCELLED

    def test_convert_timeout(self):
        """EXECUTION_STATUS_TIMEOUT maps to TIMEOUT."""
        result = protobuf_execution_status_to_python(operator_pb2.EXECUTION_STATUS_TIMEOUT)
        assert result == ExecutionStatus.TIMEOUT

    def test_raise_on_unknown_status(self):
        """Unknown protobuf enum value raises ValueError."""
        with pytest.raises(ValueError, match="Unknown protobuf ExecutionStatus value"):
            protobuf_execution_status_to_python(999)


class TestDecodeUAPEnvelope:
    """Test UAP JSON envelope decoding."""

    def test_decode_valid_envelope(self):
        """Valid UAP JSON envelope decodes successfully."""
        envelope_data = {
            "protocol_version": "1.0",
            "message_id": "test-id",
            "intent": {"action_type": "test.event"},
            "intent_data": {"key": "value"}
        }
        envelope_json = json.dumps(envelope_data)
        decoded = decode_uap_envelope(envelope_json)

        assert decoded["message_id"] == "test-id"
        assert decoded["intent"]["action_type"] == "test.event"
        assert decoded["intent_data"]["key"] == "value"

    def test_decode_invalid_json_raises(self):
        """Invalid JSON raises JSONDecodeError."""
        with pytest.raises(json.JSONDecodeError):
            decode_uap_envelope("invalid-json")


class TestDecodeG8eoResultEnvelope:
    """Test g8eo result envelope decoding with UAP JSON."""

    def test_decode_command_result_completed(self):
        """EXECUTE_BASH_RESULT with completed status decodes correctly."""
        envelope_data = {
            "id": "test-exec-id",
            "event_type": EventType.OPERATOR_COMMAND_RESULT,
            "operator_id": "op-1",
            "action_type": "EXECUTE_BASH_RESULT",
            "intent_data": {
                "execution_id": "test-exec-id",
                "payload_type": "execution_result",
                "status": "completed",
                "stdout": "test output",
                "return_code": 0
            },
            "operator_session_id": "sess-1",
            "case_id": "case-1"
        }

        # Decode
        decoded = decode_g8eo_result_envelope(envelope_data)

        assert decoded["id"] == "test-exec-id"
        assert decoded["event_type"] == EventType.OPERATOR_COMMAND_RESULT
        assert decoded["operator_id"] == "op-1"
        assert decoded["operator_session_id"] == "sess-1"
        assert decoded["case_id"] == "case-1"

        # Check payload conversion
        payload = decoded["payload"]
        assert payload["payload_type"] == "execution_result"
        assert payload["execution_id"] == "test-exec-id"
        assert payload["status"] == "completed"
        assert payload["stdout"] == "test output"
        assert payload["return_code"] == 0

    def test_decode_command_result_failed(self):
        """EXECUTE_BASH_RESULT with failed status decodes correctly."""
        envelope_data = {
            "id": "test-exec-id",
            "event_type": EventType.OPERATOR_COMMAND_RESULT,
            "operator_id": "op-1",
            "action_type": "EXECUTE_BASH_RESULT",
            "intent_data": {
                "execution_id": "test-exec-id",
                "payload_type": "execution_result",
                "status": "failed",
                "error": "test error",
                "return_code": 1
            },
            "operator_session_id": "sess-1"
        }

        decoded = decode_g8eo_result_envelope(envelope_data)

        payload = decoded["payload"]
        assert payload["payload_type"] == "execution_result"
        assert payload["status"] == "failed"
        assert payload["error"] == "test error"
        assert payload["return_code"] == 1

    def test_decode_execution_status_update(self):
        """EXECUTE_STATUS_UPDATE decodes correctly."""
        envelope_data = {
            "id": "test-id",
            "event_type": EventType.OPERATOR_COMMAND_STATUS_UPDATED,
            "operator_id": "op-1",
            "action_type": "EXECUTE_STATUS_UPDATE",
            "intent_data": {
                "execution_id": "test-exec-id",
                "payload_type": "execution_status",
                "status": "executing",
                "process_alive": True,
                "elapsed_seconds": 5.0
            },
            "operator_session_id": "sess-1"
        }

        decoded = decode_g8eo_result_envelope(envelope_data)

        payload = decoded["payload"]
        assert payload["payload_type"] == "execution_status"
        assert payload["execution_id"] == "test-exec-id"
        assert payload["status"] == "executing"
        assert payload["process_alive"] is True
        assert payload["elapsed_seconds"] == 5.0

    def test_decode_file_edit_result(self):
        """FILE_EDIT_RESULT decodes correctly."""
        envelope_data = {
            "id": "test-id",
            "event_type": EventType.OPERATOR_FILE_EDIT_COMPLETED,
            "operator_id": "op-1",
            "action_type": "FILE_EDIT_RESULT",
            "intent_data": {
                "execution_id": "test-exec-id",
                "payload_type": "file_edit_result",
                "status": "completed",
                "file_path": "/test/file.txt",
                "operation": "write"
            },
            "operator_session_id": "sess-1"
        }

        decoded = decode_g8eo_result_envelope(envelope_data)

        payload = decoded["payload"]
        assert payload["payload_type"] == "file_edit_result"
        assert payload["execution_id"] == "test-exec-id"
        assert payload["status"] == "completed"
        assert payload["file_path"] == "/test/file.txt"
        assert payload["operation"] == "write"

    def test_decode_fs_list_result(self):
        """FS_LIST_RESULT decodes correctly."""
        envelope_data = {
            "id": "test-id",
            "event_type": EventType.OPERATOR_FS_LIST_COMPLETED,
            "operator_id": "op-1",
            "action_type": "FS_LIST_RESULT",
            "intent_data": {
                "execution_id": "test-exec-id",
                "payload_type": "fs_list_result",
                "status": "completed",
                "path": "/test",
                "total_count": 5,
                "truncated": False
            },
            "operator_session_id": "sess-1"
        }

        decoded = decode_g8eo_result_envelope(envelope_data)

        payload = decoded["payload"]
        assert payload["payload_type"] == "fs_list_result"
        assert payload["execution_id"] == "test-exec-id"
        assert payload["status"] == "completed"
        assert payload["path"] == "/test"
        assert payload["total_count"] == 5
        assert payload["truncated"] is False

    def test_decode_unknown_action_type(self):
        """Unknown action type returns unknown payload type."""
        envelope_data = {
            "id": "test-id",
            "event_type": "unknown.event",
            "operator_id": "op-1",
            "action_type": "unknown.action.type",
            "intent_data": {"payload_type": "unknown", "test": "data"},
            "operator_session_id": "sess-1"
        }

        decoded = decode_g8eo_result_envelope(envelope_data)

        assert decoded["payload"]["payload_type"] == "unknown"

    def test_decode_command_cancelled(self):
        """EXECUTE_BASH_CANCELLED event decodes correctly."""
        envelope_data = {
            "id": "test-id",
            "event_type": EventType.OPERATOR_COMMAND_CANCELLED,
            "operator_id": "op-1",
            "action_type": "EXECUTE_BASH_CANCELLED",
            "intent_data": {
                "execution_id": "test-exec-id",
                "payload_type": "cancellation_result",
                "status": "cancelled"
            },
            "operator_session_id": "sess-1"
        }

        decoded = decode_g8eo_result_envelope(envelope_data)

        payload = decoded["payload"]
        assert payload["payload_type"] == "cancellation_result"
        assert payload["status"] == "cancelled"
