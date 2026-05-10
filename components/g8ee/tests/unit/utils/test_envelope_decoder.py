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

"""Tests for protobuf envelope decoder and enum conversion."""

import pytest

from app.constants import ExecutionStatus
from app.proto import common_pb2, operator_pb2
from app.utils.envelope_builder import (
    decode_universal_envelope,
    decode_g8eo_result_envelope,
    protobuf_execution_status_to_python,
    build_universal_envelope_bytes,
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


class TestDecodeUniversalEnvelope:
    """Test UniversalEnvelope decoding."""

    def test_decode_valid_envelope(self):
        """Valid envelope bytes decode successfully."""
        # Build a minimal envelope
        envelope = common_pb2.UniversalEnvelope()
        envelope.id = "test-id"
        envelope.event_type = "test.event"
        envelope.payload = b"test-payload"

        envelope_bytes = envelope.SerializeToString()
        decoded = decode_universal_envelope(envelope_bytes)

        assert decoded.id == "test-id"
        assert decoded.event_type == "test.event"
        assert decoded.payload == b"test-payload"

    def test_decode_missing_id_raises(self):
        """Envelope without id field raises ValueError."""
        envelope = common_pb2.UniversalEnvelope()
        envelope.event_type = "test.event"
        envelope.payload = b"test-payload"

        envelope_bytes = envelope.SerializeToString()
        with pytest.raises(ValueError, match="missing id field"):
            decode_universal_envelope(envelope_bytes)

    def test_decode_invalid_bytes_raises(self):
        """Invalid bytes raise ValueError."""
        with pytest.raises(ValueError, match="Failed to decode"):
            decode_universal_envelope(b"invalid-bytes")


class TestDecodeG8eoResultEnvelope:
    """Test g8eo result envelope decoding with enum conversion."""

    def test_decode_command_result_completed(self):
        """CommandResult with COMPLETED status decodes correctly."""
        # Build envelope with CommandResult payload
        envelope = common_pb2.UniversalEnvelope()
        envelope.id = "test-exec-id"
        envelope.event_type = "g8e.v1.operator.command.completed"
        envelope.operator_id = "op-1"
        envelope.operator_session_id = "sess-1"
        envelope.case_id = "case-1"

        # Build CommandResult payload
        command_result = operator_pb2.CommandResult()
        command_result.execution_id = "test-exec-id"
        command_result.status = operator_pb2.EXECUTION_STATUS_COMPLETED
        command_result.output = "test output"
        command_result.exit_code = 0

        envelope.payload = command_result.SerializeToString()
        envelope_bytes = envelope.SerializeToString()

        # Decode
        decoded = decode_g8eo_result_envelope(envelope_bytes)

        assert decoded["id"] == "test-exec-id"
        assert decoded["event_type"] == "g8e.v1.operator.command.completed"
        assert decoded["operator_id"] == "op-1"
        assert decoded["operator_session_id"] == "sess-1"
        assert decoded["case_id"] == "case-1"

        # Check payload conversion
        payload = decoded["payload"]
        assert payload["payload_type"] == "execution_result"
        assert payload["execution_id"] == "test-exec-id"
        assert payload["status"] == "completed"  # Enum converted to string
        assert payload["stdout"] == "test output"
        assert payload["return_code"] == 0

    def test_decode_command_result_failed(self):
        """CommandResult with FAILED status decodes correctly."""
        envelope = common_pb2.UniversalEnvelope()
        envelope.id = "test-exec-id"
        envelope.event_type = "g8e.v1.operator.command.failed"
        envelope.operator_id = "op-1"
        envelope.operator_session_id = "sess-1"

        command_result = operator_pb2.CommandResult()
        command_result.execution_id = "test-exec-id"
        command_result.status = operator_pb2.EXECUTION_STATUS_FAILED
        command_result.error = "test error"
        command_result.exit_code = 1

        envelope.payload = command_result.SerializeToString()
        envelope_bytes = envelope.SerializeToString()

        decoded = decode_g8eo_result_envelope(envelope_bytes)

        payload = decoded["payload"]
        assert payload["payload_type"] == "execution_result"
        assert payload["status"] == "failed"
        assert payload["error"] == "test error"
        assert payload["return_code"] == 1

    def test_decode_execution_status_update(self):
        """ExecutionStatusUpdate decodes correctly."""
        envelope = common_pb2.UniversalEnvelope()
        envelope.id = "test-id"
        envelope.event_type = "g8e.v1.operator.command.status.updated"
        envelope.operator_id = "op-1"
        envelope.operator_session_id = "sess-1"

        status_update = operator_pb2.ExecutionStatusUpdate()
        status_update.execution_id = "test-exec-id"
        status_update.status = operator_pb2.EXECUTION_STATUS_EXECUTING
        status_update.process_alive = True
        status_update.elapsed_seconds = 5.0

        envelope.payload = status_update.SerializeToString()
        envelope_bytes = envelope.SerializeToString()

        decoded = decode_g8eo_result_envelope(envelope_bytes)

        payload = decoded["payload"]
        assert payload["payload_type"] == "execution_status"
        assert payload["execution_id"] == "test-exec-id"
        assert payload["status"] == "executing"
        assert payload["process_alive"] is True
        assert payload["elapsed_seconds"] == 5.0

    def test_decode_file_edit_result(self):
        """FileEditResult decodes correctly."""
        envelope = common_pb2.UniversalEnvelope()
        envelope.id = "test-id"
        envelope.event_type = "g8e.v1.operator.file.edit.completed"
        envelope.operator_id = "op-1"
        envelope.operator_session_id = "sess-1"

        file_result = operator_pb2.FileEditResult()
        file_result.execution_id = "test-exec-id"
        file_result.status = operator_pb2.EXECUTION_STATUS_COMPLETED
        file_result.file_path = "/test/file.txt"
        file_result.operation = "write"

        envelope.payload = file_result.SerializeToString()
        envelope_bytes = envelope.SerializeToString()

        decoded = decode_g8eo_result_envelope(envelope_bytes)

        payload = decoded["payload"]
        assert payload["payload_type"] == "file_edit_result"
        assert payload["execution_id"] == "test-exec-id"
        assert payload["status"] == "completed"
        assert payload["file_path"] == "/test/file.txt"
        assert payload["operation"] == "write"

    def test_decode_fs_list_result(self):
        """FsListResult decodes correctly."""
        envelope = common_pb2.UniversalEnvelope()
        envelope.id = "test-id"
        envelope.event_type = "g8e.v1.operator.fs.list.completed"
        envelope.operator_id = "op-1"
        envelope.operator_session_id = "sess-1"

        fs_list = operator_pb2.FsListResult()
        fs_list.execution_id = "test-exec-id"
        fs_list.status = operator_pb2.EXECUTION_STATUS_COMPLETED
        fs_list.path = "/test"
        fs_list.total_count = 5
        fs_list.truncated = False

        envelope.payload = fs_list.SerializeToString()
        envelope_bytes = envelope.SerializeToString()

        decoded = decode_g8eo_result_envelope(envelope_bytes)

        payload = decoded["payload"]
        assert payload["payload_type"] == "fs_list_result"
        assert payload["execution_id"] == "test-exec-id"
        assert payload["status"] == "completed"
        assert payload["path"] == "/test"
        assert payload["total_count"] == 5
        assert payload["truncated"] is False

    def test_decode_fs_grep_result(self):
        """FsGrepResult decodes correctly."""
        envelope = common_pb2.UniversalEnvelope()
        envelope.id = "test-id"
        envelope.event_type = "g8e.v1.operator.fs.grep.completed"
        envelope.operator_id = "op-1"
        envelope.operator_session_id = "sess-1"

        fs_grep = operator_pb2.FsGrepResult()
        fs_grep.execution_id = "test-exec-id"
        fs_grep.status = operator_pb2.EXECUTION_STATUS_COMPLETED
        fs_grep.path = "/test"
        fs_grep.total_matches = 3
        fs_grep.truncated = False

        envelope.payload = fs_grep.SerializeToString()
        envelope_bytes = envelope.SerializeToString()

        decoded = decode_g8eo_result_envelope(envelope_bytes)

        payload = decoded["payload"]
        assert payload["payload_type"] == "fs_grep_result"
        assert payload["execution_id"] == "test-exec-id"
        assert payload["status"] == "completed"
        assert payload["path"] == "/test"
        assert payload["total_matches"] == 3

    def test_decode_fs_read_result(self):
        """FsReadResult decodes correctly."""
        envelope = common_pb2.UniversalEnvelope()
        envelope.id = "test-id"
        envelope.event_type = "g8e.v1.operator.fs.read.completed"
        envelope.operator_id = "op-1"
        envelope.operator_session_id = "sess-1"

        fs_read = operator_pb2.FsReadResult()
        fs_read.execution_id = "test-exec-id"
        fs_read.status = operator_pb2.EXECUTION_STATUS_COMPLETED
        fs_read.path = "/test/file.txt"
        fs_read.content = "test content"
        fs_read.size_bytes = 12

        envelope.payload = fs_read.SerializeToString()
        envelope_bytes = envelope.SerializeToString()

        decoded = decode_g8eo_result_envelope(envelope_bytes)

        payload = decoded["payload"]
        assert payload["payload_type"] == "fs_read_result"
        assert payload["execution_id"] == "test-exec-id"
        assert payload["status"] == "completed"
        assert payload["path"] == "/test/file.txt"
        assert payload["content"] == "test content"
        assert payload["size_bytes"] == 12

    def test_decode_port_check_result(self):
        """PortCheckResult decodes correctly."""
        envelope = common_pb2.UniversalEnvelope()
        envelope.id = "test-id"
        envelope.event_type = "g8e.v1.operator.port.check.completed"
        envelope.operator_id = "op-1"
        envelope.operator_session_id = "sess-1"

        port_check = operator_pb2.PortCheckResult()
        port_check.execution_id = "test-exec-id"
        port_check.status = operator_pb2.EXECUTION_STATUS_COMPLETED

        envelope.payload = port_check.SerializeToString()
        envelope_bytes = envelope.SerializeToString()

        decoded = decode_g8eo_result_envelope(envelope_bytes)

        payload = decoded["payload"]
        assert payload["payload_type"] == "port_check_result"
        assert payload["execution_id"] == "test-exec-id"
        assert payload["status"] == "completed"

    def test_decode_unknown_event_type(self):
        """Unknown event type returns unknown payload type."""
        envelope = common_pb2.UniversalEnvelope()
        envelope.id = "test-id"
        envelope.event_type = "unknown.event.type"
        envelope.operator_id = "op-1"
        envelope.operator_session_id = "sess-1"
        envelope.payload = b"test-payload"

        envelope_bytes = envelope.SerializeToString()
        decoded = decode_g8eo_result_envelope(envelope_bytes)

        assert decoded["payload"]["payload_type"] == "unknown"

    def test_decode_command_cancelled(self):
        """Command cancelled event decodes correctly."""
        envelope = common_pb2.UniversalEnvelope()
        envelope.id = "test-id"
        envelope.event_type = "g8e.v1.operator.command.cancelled"
        envelope.operator_id = "op-1"
        envelope.operator_session_id = "sess-1"

        command_result = operator_pb2.CommandResult()
        command_result.execution_id = "test-exec-id"
        command_result.status = operator_pb2.EXECUTION_STATUS_CANCELLED

        envelope.payload = command_result.SerializeToString()
        envelope_bytes = envelope.SerializeToString()

        decoded = decode_g8eo_result_envelope(envelope_bytes)

        payload = decoded["payload"]
        assert payload["payload_type"] == "cancellation_result"
        assert payload["status"] == "cancelled"
