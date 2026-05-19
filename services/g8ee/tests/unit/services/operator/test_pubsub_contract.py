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

"""Contract tests for PubSub Service payload parsing."""

from typing import get_args

import pytest

from app.errors import ValidationError
from app.models.pubsub_messages import (
    ExecutionResultsPayload,
    FetchFileDiffByIdSuccessPayload,
    FetchFileDiffBySessionSuccessPayload,
    FetchFileHistorySuccessPayload,
    FetchHistoryErrorPayload,
    FetchHistorySuccessPayload,
    FsListResultPayload,
    FsReadResultPayload,
    G8eoResultPayload,
    PortCheckResultPayload,
    RestoreFileSuccessPayload,
)
from app.utils.envelope_builder import parse_inbound_g8eo_payload

pytestmark = [pytest.mark.unit]

def test_discriminator_parsing_execution_result():
    """Verify that discriminator-based parsing works for execution results."""
    payload_raw = {
        "payload_type": "execution_result",
        "execution_id": "exec-123",
        "status": "completed",
        "duration_seconds": 1.5,
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, ExecutionResultsPayload)
    assert result.execution_id == "exec-123"
    assert result.payload_type == "execution_result"

def test_discriminator_parsing_port_check():
    """Verify that discriminator-based parsing works for port check results."""
    payload_raw = {
        "payload_type": "port_check_result",
        "execution_id": "exec-456",
        "host": "localhost",
        "port": 8080,
        "is_open": True,
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, PortCheckResultPayload)
    assert result.execution_id == "exec-456"
    assert result.payload_type == "port_check_result"

def test_discriminator_parsing_fs_list():
    """Verify that discriminator-based parsing works for filesystem list results."""
    payload_raw = {
        "payload_type": "fs_list_result",
        "execution_id": "exec-789",
        "path": "/tmp",
        "status": "completed",
        "entries": [],
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, FsListResultPayload)
    assert result.execution_id == "exec-789"
    assert result.payload_type == "fs_list_result"

def test_invalid_payload_type_raises_validation_error():
    """Verify that invalid payload_type raises ValidationError."""
    payload_raw = {
        "payload_type": "invalid_type",
        "execution_id": "exec-123",
    }

    with pytest.raises(ValidationError):
        parse_inbound_g8eo_payload(payload_raw)

def test_discriminator_parsing_fs_read_with_content():
    """Verify that FsReadResultPayload parses correctly when content is present."""
    payload_raw = {
        "payload_type": "fs_read_result",
        "execution_id": "exec-read-1",
        "path": "/tmp/test.txt",
        "status": "completed",
        "content": "hello world",
        "size_bytes": 11,
        "truncated": False,
        "duration_seconds": 0.1,
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, FsReadResultPayload)
    assert result.content == "hello world"
    assert result.payload_type == "fs_read_result"


def test_discriminator_parsing_fs_read_empty_content():
    """Regression: FsReadResultPayload with empty content must NOT be misidentified
    as FsListResultPayload. This was the root cause of empty file read results.
    Without payload_type on the wire, Pydantic picked FsListResultPayload because
    content was absent (Go omitempty on empty string)."""
    payload_raw = {
        "payload_type": "fs_read_result",
        "execution_id": "exec-read-empty",
        "path": "/tmp/empty.txt",
        "status": "completed",
        "content": "",
        "size_bytes": 0,
        "truncated": False,
        "duration_seconds": 0.05,
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, FsReadResultPayload), (
        f"Expected FsReadResultPayload, got {type(result).__name__} - "
        "empty content must not cause misidentification as FsListResultPayload"
    )
    assert result.content == ""
    assert result.payload_type == "fs_read_result"


def test_discriminator_parsing_fs_read_failed():
    """Verify that a failed FsReadResultPayload parses correctly."""
    payload_raw = {
        "payload_type": "fs_read_result",
        "execution_id": "exec-read-fail",
        "path": "/tmp/no-such-file.txt",
        "status": "failed",
        "content": "",
        "size_bytes": 0,
        "truncated": False,
        "duration_seconds": 0.01,
        "error_message": "open /tmp/no-such-file.txt: no such file or directory",
        "error_type": "read_error",
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, FsReadResultPayload)
    assert result.status.value == "failed"
    assert result.error_message == "open /tmp/no-such-file.txt: no such file or directory"


def test_discriminator_parsing_fetch_history_success():
    """Verify that FetchHistorySuccessPayload parses correctly."""
    payload_raw = {
        "payload_type": "fetch_history_success",
        "execution_id": "exec-hist-1",
        "operator_session_id": "sess-abc",
        "web_session": None,
        "events": [],
        "total": 0,
        "limit": 50,
        "offset": 0,
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, FetchHistorySuccessPayload)
    assert result.payload_type == "fetch_history_success"


def test_discriminator_parsing_fetch_history_error():
    """Verify that FetchHistoryErrorPayload parses correctly."""
    payload_raw = {
        "payload_type": "fetch_history_error",
        "execution_id": "exec-hist-err",
        "error": "history handler not available",
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, FetchHistoryErrorPayload)
    assert result.error == "history handler not available"


def test_discriminator_parsing_fetch_file_history_success():
    """Verify that FetchFileHistorySuccessPayload parses correctly."""
    payload_raw = {
        "payload_type": "fetch_file_history_success",
        "execution_id": "exec-fhist-1",
        "file_path": "/tmp/foo.py",
        "history": [],
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, FetchFileHistorySuccessPayload)
    assert result.file_path == "/tmp/foo.py"


def test_discriminator_parsing_restore_file_success():
    """Verify that RestoreFileSuccessPayload parses correctly."""
    payload_raw = {
        "payload_type": "restore_file_success",
        "execution_id": "exec-restore-1",
        "file_path": "/tmp/foo.py",
        "commit_hash": "abc123",
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, RestoreFileSuccessPayload)
    assert result.commit_hash == "abc123"


def test_discriminator_parsing_fetch_file_diff_by_id():
    """Verify that FetchFileDiffByIdSuccessPayload parses correctly."""
    payload_raw = {
        "payload_type": "fetch_file_diff_by_id_success",
        "execution_id": "exec-diff-1",
        "diff": {
            "id": "diff-abc",
            "timestamp": "2026-01-01T00:00:00Z",
            "file_path": "/tmp/foo.py",
            "operation": "edit",
            "ledger_hash_before": "aaa",
            "ledger_hash_after": "bbb",
            "diff_stat": "+1 -0",
            "diff_size": 42,
            "operator_session_id": "sess-1",
        },
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, FetchFileDiffByIdSuccessPayload)
    assert result.diff.id == "diff-abc"


def test_discriminator_parsing_fetch_file_diff_by_session():
    """Verify that FetchFileDiffBySessionSuccessPayload parses correctly."""
    payload_raw = {
        "payload_type": "fetch_file_diff_by_session_success",
        "execution_id": "exec-diff-sess-1",
        "operator_session_id": "sess-abc",
        "diffs": [],
        "total": 0,
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, FetchFileDiffBySessionSuccessPayload)
    assert result.operator_session_id == "sess-abc"


def test_fs_read_result_size_bytes_field():
    """Regression: FsReadResultPayload must use size_bytes (not size) to match Go wire format."""
    payload_raw = {
        "payload_type": "fs_read_result",
        "execution_id": "exec-size-1",
        "path": "/tmp/test.txt",
        "status": "completed",
        "content": "hello",
        "size_bytes": 5,
        "truncated": False,
        "duration_seconds": 0.1,
    }
    result = parse_inbound_g8eo_payload(payload_raw)
    assert isinstance(result, FsReadResultPayload)
    assert result.size_bytes == 5


def test_all_payload_models_have_discriminator():
    """Verify that all models in G8eoResultPayload union have a payload_type field."""
    union_types = get_args(G8eoResultPayload)

    for model_class in union_types:
        assert "payload_type" in model_class.model_fields, (
            f"Model {model_class.__name__} is missing payload_type discriminator field"
        )
