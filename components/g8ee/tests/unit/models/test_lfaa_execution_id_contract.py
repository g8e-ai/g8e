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

"""Contract tests ensuring LFAA result payloads have execution_id field.

This test file verifies that g8eo and g8ee models are aligned for execution_id.
The bug was that g8eo Go models had ExecutionID field but g8ee Python models
were missing execution_id for LFAA payloads, requiring fallback logic in
command_service.py.

These unit tests verify model field alignment without requiring a running
operator, ensuring this regression is caught early in CI.
"""

from __future__ import annotations

import pytest

from app.models.pubsub_messages import (
    FetchFileHistorySuccessPayload,
    FetchFileHistoryErrorPayload,
    FetchFileDiffByIdSuccessPayload,
    FetchFileDiffBySessionSuccessPayload,
    FetchFileDiffErrorPayload,
    FetchHistorySuccessPayload,
    FetchHistoryErrorPayload,
    RestoreFileSuccessPayload,
    RestoreFileErrorPayload,
    CancellationResultPayload,
    FileEditResultPayload,
    FsListResultPayload,
    FsReadResultPayload,
    ExecutionResultsPayload,
    ExecutionStatusPayload,
    FetchLogsResultPayload,
    FetchLogsErrorPayload,
    PortCheckResultPayload,
)


class TestLFAAExecutionIdFieldPresence:
    """Verify all LFAA result payloads have execution_id field defined."""

    def test_fetch_file_history_success_payload_has_execution_id(self):
        """FetchFileHistorySuccessPayload must have execution_id field."""
        assert "execution_id" in FetchFileHistorySuccessPayload.model_fields, (
            "FetchFileHistorySuccessPayload must have execution_id field for g8eo alignment"
        )

    def test_fetch_file_diff_by_id_success_payload_has_execution_id(self):
        """FetchFileDiffByIdSuccessPayload must have execution_id field."""
        assert "execution_id" in FetchFileDiffByIdSuccessPayload.model_fields, (
            "FetchFileDiffByIdSuccessPayload must have execution_id field for g8eo alignment"
        )

    def test_fetch_file_diff_by_session_success_payload_has_execution_id(self):
        """FetchFileDiffBySessionSuccessPayload must have execution_id field."""
        assert "execution_id" in FetchFileDiffBySessionSuccessPayload.model_fields, (
            "FetchFileDiffBySessionSuccessPayload must have execution_id field for g8eo alignment"
        )

    def test_restore_file_success_payload_has_execution_id(self):
        """RestoreFileSuccessPayload must have execution_id field."""
        assert "execution_id" in RestoreFileSuccessPayload.model_fields, (
            "RestoreFileSuccessPayload must have execution_id field for g8eo alignment"
        )

    def test_fetch_history_success_payload_has_execution_id(self):
        """FetchHistorySuccessPayload must have execution_id field."""
        assert "execution_id" in FetchHistorySuccessPayload.model_fields, (
            "FetchHistorySuccessPayload must have execution_id field for g8eo alignment"
        )

    def test_error_payloads_require_execution_id(self):
        """Error payloads should have required execution_id field for correlation."""
        error_payloads = [
            FetchFileHistoryErrorPayload,
            FetchFileDiffErrorPayload,
            RestoreFileErrorPayload,
            FetchHistoryErrorPayload,
            FetchLogsErrorPayload,
        ]
        for payload_type in error_payloads:
            assert "execution_id" in payload_type.model_fields, (
                f"{payload_type.__name__} must have execution_id field"
            )
            field = payload_type.model_fields["execution_id"]
            assert field.is_required(), (
                f"{payload_type.__name__} execution_id should be required for error correlation"
            )


class TestAllResultPayloadsHaveExecutionId:
    """Verify all result payloads have execution_id field for consistency."""

    def test_all_result_payloads_have_execution_id_field(self):
        """Every result payload type must have execution_id field.

        This ensures g8eo and g8ee models are aligned and no fallback logic
        is needed in command_service.py.
        """
        result_payload_types = [
            CancellationResultPayload,
            FileEditResultPayload,
            FsListResultPayload,
            FsReadResultPayload,
            ExecutionResultsPayload,
            ExecutionStatusPayload,
            FetchLogsResultPayload,
            PortCheckResultPayload,
            FetchFileHistorySuccessPayload,
            FetchFileDiffByIdSuccessPayload,
            FetchFileDiffBySessionSuccessPayload,
            RestoreFileSuccessPayload,
            FetchHistorySuccessPayload,
        ]

        missing_fields = []
        for payload_type in result_payload_types:
            if "execution_id" not in payload_type.model_fields:
                missing_fields.append(payload_type.__name__)

        assert not missing_fields, (
            f"The following result payload types are missing execution_id field: {missing_fields}. "
            "All result payloads must have execution_id to match g8eo Go models."
        )


class TestLFAAExecutionIdFieldParsing:
    """Verify LFAA payloads can parse execution_id from g8eo JSON responses."""

    def test_fetch_file_history_success_parses_execution_id(self):
        """FetchFileHistorySuccessPayload must accept execution_id in JSON."""
        payload_dict = {
            "execution_id": "test-exec-123",
            "file_path": "/etc/hosts",
            "history": [],
        }
        payload = FetchFileHistorySuccessPayload(**payload_dict)
        assert payload.execution_id == "test-exec-123"

    def test_fetch_file_diff_by_id_parses_execution_id(self):
        """FetchFileDiffByIdSuccessPayload must accept execution_id in JSON."""
        from app.models.tool_results import FileDiffEntry
        payload_dict = {
            "execution_id": "test-exec-456",
            "diff": {
                "id": "diff-1",
                "timestamp": "2024-01-01T00:00:00Z",
                "file_path": "/etc/hosts",
                "operation": "WRITE",
                "ledger_hash_before": "abc123",
                "ledger_hash_after": "def456",
                "diff_stat": "+1 -1",
                "diff_size": 100,
                "operator_session_id": "sess-1",
            },
        }
        payload = FetchFileDiffByIdSuccessPayload(**payload_dict)
        assert payload.execution_id == "test-exec-456"

    def test_fetch_file_diff_by_session_parses_execution_id(self):
        """FetchFileDiffBySessionSuccessPayload must accept execution_id in JSON."""
        payload_dict = {
            "execution_id": "test-exec-789",
            "diffs": [],
            "total": 0,
            "operator_session_id": "sess-1",
        }
        payload = FetchFileDiffBySessionSuccessPayload(**payload_dict)
        assert payload.execution_id == "test-exec-789"

    def test_restore_file_success_parses_execution_id(self):
        """RestoreFileSuccessPayload must accept execution_id in JSON."""
        payload_dict = {
            "execution_id": "test-exec-abc",
            "file_path": "/etc/hosts",
            "commit_hash": "deadbeef",
        }
        payload = RestoreFileSuccessPayload(**payload_dict)
        assert payload.execution_id == "test-exec-abc"

    def test_fetch_history_success_parses_execution_id(self):
        """FetchHistorySuccessPayload must accept execution_id in JSON."""
        from app.models.tool_results import AuditSessionMetadata
        payload_dict = {
            "execution_id": "test-exec-def",
            "operator_session_id": "sess-1",
            "session": {
                "id": "session-1",
                "title": "Test Session",
                "user_identity": "test-user",
            },
            "events": [],
            "total": 0,
            "limit": 50,
            "offset": 0,
        }
        payload = FetchHistorySuccessPayload(**payload_dict)
        assert payload.execution_id == "test-exec-def"

    def test_error_payloads_require_non_null_execution_id(self):
        """Error payloads should require non-null execution_id."""
        payload_dict = {
            "execution_id": "test-exec-error",
            "error": "test error",
        }
        payload = FetchFileHistoryErrorPayload(**payload_dict)
        assert payload.execution_id == "test-exec-error"
