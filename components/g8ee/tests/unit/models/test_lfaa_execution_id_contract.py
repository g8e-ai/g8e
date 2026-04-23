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
    FetchFileHistoryResultPayload,
    FetchFileDiffResultPayload,
    RestoreFileResultPayload,
    FetchHistoryResultPayload,
    CancellationResultPayload,
    FileEditResultPayload,
    FsListResultPayload,
    FsReadResultPayload,
    ExecutionResultsPayload,
    ExecutionStatusPayload,
    FetchLogsResultPayload,
    PortCheckResultPayload,
)


class TestLFAAExecutionIdFieldPresence:
    """Verify all LFAA result payloads have execution_id field defined."""

    def test_fetch_file_history_result_payload_has_execution_id(self):
        """FetchFileHistoryResultPayload must have execution_id field."""
        assert "execution_id" in FetchFileHistoryResultPayload.model_fields, (
            "FetchFileHistoryResultPayload must have execution_id field for g8eo alignment"
        )

    def test_fetch_file_diff_result_payload_has_execution_id(self):
        """FetchFileDiffResultPayload must have execution_id field."""
        assert "execution_id" in FetchFileDiffResultPayload.model_fields, (
            "FetchFileDiffResultPayload must have execution_id field for g8eo alignment"
        )

    def test_restore_file_result_payload_has_execution_id(self):
        """RestoreFileResultPayload must have execution_id field."""
        assert "execution_id" in RestoreFileResultPayload.model_fields, (
            "RestoreFileResultPayload must have execution_id field for g8eo alignment"
        )

    def test_fetch_history_result_payload_has_execution_id(self):
        """FetchHistoryResultPayload must have execution_id field."""
        assert "execution_id" in FetchHistoryResultPayload.model_fields, (
            "FetchHistoryResultPayload must have execution_id field for g8eo alignment"
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
            FetchFileHistoryResultPayload,
            FetchFileDiffResultPayload,
            RestoreFileResultPayload,
            FetchHistoryResultPayload,
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

    def test_fetch_file_history_parses_execution_id(self):
        """FetchFileHistoryResultPayload must accept execution_id in JSON."""
        payload_dict = {
            "execution_id": "test-exec-123",
            "success": True,
            "file_path": "/etc/hosts",
            "history": [],
        }
        payload = FetchFileHistoryResultPayload.model_validate(payload_dict)
        assert payload.execution_id == "test-exec-123"

    def test_fetch_file_diff_parses_execution_id(self):
        """FetchFileDiffResultPayload must accept execution_id in JSON."""
        payload_dict = {
            "execution_id": "test-exec-456",
            "success": True,
            "diffs": [],
            "total": 0,
        }
        payload = FetchFileDiffResultPayload.model_validate(payload_dict)
        assert payload.execution_id == "test-exec-456"

    def test_restore_file_parses_execution_id(self):
        """RestoreFileResultPayload must accept execution_id in JSON."""
        payload_dict = {
            "execution_id": "test-exec-789",
            "success": True,
            "file_path": "/etc/hosts",
            "commit_hash": "deadbeef",
        }
        payload = RestoreFileResultPayload.model_validate(payload_dict)
        assert payload.execution_id == "test-exec-789"

    def test_fetch_history_parses_execution_id(self):
        """FetchHistoryResultPayload must accept execution_id in JSON."""
        payload_dict = {
            "execution_id": "test-exec-abc",
            "success": True,
            "operator_session_id": "sess-1",
            "events": [],
            "total": 0,
            "limit": 50,
            "offset": 0,
        }
        payload = FetchHistoryResultPayload.model_validate(payload_dict)
        assert payload.execution_id == "test-exec-abc"

    def test_lfaa_payloads_allow_null_execution_id(self):
        """LFAA payloads should allow null execution_id for error cases."""
        payload_dict = {
            "execution_id": None,
            "success": False,
            "error": "test error",
        }
        payload = FetchFileHistoryResultPayload.model_validate(payload_dict)
        assert payload.execution_id is None
        assert payload.success is False
