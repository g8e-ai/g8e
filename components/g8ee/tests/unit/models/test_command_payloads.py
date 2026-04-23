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
Unit tests for command_payloads.py typed wire models.

Covers FsListRequestPayload, FsReadRequestPayload, FetchFileHistoryRequestPayload, FetchFileDiffRequestPayload, and CheckPortRequestPayload:
- Construction with required and optional fields
- flatten_for_wire() produces only canonical wire fields
- flatten_for_wire() excludes None-valued optional fields
- Pydantic rejects invalid types
- Model is a G8eBaseModel subclass
- TargetedOperatorBase payloads inherit target_operator field
"""

import pytest
from app.models.base import ValidationError, G8eBaseModel
from app.models.command_request_payloads import (
    FetchLogsRequestPayload,
    FetchFileHistoryRequestPayload,
    FetchFileDiffRequestPayload,
    CheckPortRequestPayload,
    FsListRequestPayload,
    FsReadRequestPayload,
    TargetedOperatorBase,
)

pytestmark = [pytest.mark.unit]


class TestFsListRequestPayload:

    def test_all_fields_set(self):
        p = FsListRequestPayload(
            path="/var/log",
            execution_id="exec-abc",
            max_depth=2,
            max_entries=200,
        )
        assert p.path == "/var/log"
        assert p.execution_id == "exec-abc"
        assert p.max_depth == 2
        assert p.max_entries == 200

    def test_all_fields_optional(self):
        p = FsListRequestPayload(execution_id="exec-test")
        assert p.path is None
        assert p.execution_id == "exec-test"
        assert p.max_depth is None
        assert p.max_entries is None

    def test_is_g8e_base_model(self):
        assert issubclass(FsListRequestPayload, G8eBaseModel)

    def test_wire_dump_includes_set_fields(self):
        p = FsListRequestPayload(path="/app", execution_id="exec-1", max_depth=1, max_entries=50)
        wire = p.model_dump(mode="json")
        assert wire["path"] == "/app"
        assert wire["execution_id"] == "exec-1"
        assert wire["max_depth"] == 1
        assert wire["max_entries"] == 50

    def test_wire_dump_excludes_none_fields(self):
        p = FsListRequestPayload(path="/app", execution_id="exec-1")
        wire = p.model_dump(mode="json")
        assert wire["execution_id"] == "exec-1"
        assert "max_depth" not in wire
        assert "max_entries" not in wire

    def test_wire_dump_only_canonical_fields(self):
        p = FsListRequestPayload(path=".", execution_id="x", max_depth=0, max_entries=100)
        wire = p.model_dump(mode="json")
        assert set(wire.keys()) <= {"path", "execution_id", "max_depth", "max_entries", "payload_type"}

    def test_wire_dump_no_non_canonical_fields(self):
        p = FsListRequestPayload(path="/tmp", execution_id="exec-test")
        wire = p.model_dump(mode="json")
        assert "requested_at" not in wire
        assert "source" not in wire
        assert "user_id" not in wire

    def test_extra_fields_ignored(self):
        p = FsListRequestPayload(path="/tmp", execution_id="exec-test", user_id="u-1", source="tool_call", requested_at="2026-01-01T00:00:00Z")
        wire = p.model_dump(mode="json")
        assert "user_id" not in wire
        assert "source" not in wire
        assert "requested_at" not in wire



class TestFsReadRequestPayload:

    def test_required_path(self):
        p = FsReadRequestPayload(path="/etc/hosts", execution_id="exec-test")
        assert p.path == "/etc/hosts"

    def test_missing_path_raises(self):
        with pytest.raises(ValidationError):
            FsReadRequestPayload()

    def test_all_fields_set(self):
        p = FsReadRequestPayload(path="/app/config.json", execution_id="exec-xyz", max_size=8192)
        assert p.path == "/app/config.json"
        assert p.execution_id == "exec-xyz"
        assert p.max_size == 8192

    def test_optional_fields_default_to_none(self):
        p = FsReadRequestPayload(path="/app/config.json", execution_id="exec-test")
        assert p.execution_id == "exec-test"
        assert p.max_size is None

    def test_is_g8e_base_model(self):
        assert issubclass(FsReadRequestPayload, G8eBaseModel)

    def test_wire_dump_includes_set_fields(self):
        p = FsReadRequestPayload(path="/etc/passwd", execution_id="exec-2", max_size=4096)
        wire = p.model_dump(mode="json")
        assert wire["path"] == "/etc/passwd"
        assert wire["execution_id"] == "exec-2"
        assert wire["max_size"] == 4096

    def test_wire_dump_excludes_none_optional_fields(self):
        p = FsReadRequestPayload(path="/app/log.txt", execution_id="exec-test")
        wire = p.model_dump(mode="json")
        assert wire["execution_id"] == "exec-test"
        assert "max_size" not in wire

    def test_wire_dump_only_canonical_fields(self):
        p = FsReadRequestPayload(path="/app/main.py", execution_id="e", max_size=102400)
        wire = p.model_dump(mode="json")
        assert set(wire.keys()) <= {"path", "execution_id", "max_size", "payload_type"}

    def test_wire_dump_no_non_canonical_fields(self):
        p = FsReadRequestPayload(path="/var/log/app.log", execution_id="exec-test")
        wire = p.model_dump(mode="json")
        assert "requested_at" not in wire
        assert "source" not in wire
        assert "user_id" not in wire

    def test_extra_fields_ignored(self):
        p = FsReadRequestPayload(path="/tmp/test.txt", execution_id="exec-test", user_id="u-1", source="tool_call")
        wire = p.model_dump(mode="json")
        assert "user_id" not in wire
        assert "source" not in wire



class TestFetchLogsRequestPayload:

    def test_requiredexecution_id(self):
        p = FetchLogsRequestPayload(execution_id="exec-abc")
        assert p.execution_id == "exec-abc"

    def test_missingexecution_id_raises(self):
        with pytest.raises(ValidationError):
            FetchLogsRequestPayload()

    def test_all_fields_set(self):
        p = FetchLogsRequestPayload(execution_id="exec-abc", sentinel_mode="scrubbed")
        assert p.execution_id == "exec-abc"
        assert p.sentinel_mode == "scrubbed"

    def test_sentinel_mode_optional(self):
        p = FetchLogsRequestPayload(execution_id="exec-abc")
        assert p.sentinel_mode is None

    def test_is_g8e_base_model(self):
        assert issubclass(FetchLogsRequestPayload, G8eBaseModel)

    def test_wire_dump_includes_set_fields(self):
        p = FetchLogsRequestPayload(execution_id="exec-xyz", sentinel_mode="raw")
        wire = p.model_dump(mode="json")
        assert wire["execution_id"] == "exec-xyz"
        assert wire["sentinel_mode"] == "raw"

    def test_wire_dump_excludes_none_sentinel_mode(self):
        p = FetchLogsRequestPayload(execution_id="exec-xyz")
        wire = p.model_dump(mode="json")
        assert wire["execution_id"] == "exec-xyz"
        assert "sentinel_mode" not in wire

    def test_wire_dump_only_canonical_fields(self):
        p = FetchLogsRequestPayload(execution_id="exec-xyz", sentinel_mode="scrubbed")
        wire = p.model_dump(mode="json")
        assert set(wire.keys()) <= {"execution_id", "sentinel_mode", "payload_type"}

    def test_wire_dump_no_non_canonical_fields(self):
        p = FetchLogsRequestPayload(execution_id="exec-xyz")
        wire = p.model_dump(mode="json")
        assert "requested_at" not in wire
        assert "source" not in wire
        assert "user_id" not in wire

    def test_extra_fields_ignored(self):
        p = FetchLogsRequestPayload(execution_id="exec-xyz", source="tool_call", requested_at="2026-01-01T00:00:00Z")
        wire = p.model_dump(mode="json")
        assert "source" not in wire
        assert "requested_at" not in wire


class TestFetchFileHistoryRequestPayload:

    def test_all_fields_set(self):
        p = FetchFileHistoryRequestPayload(
            execution_id="exec-abc",
            file_path="/etc/hosts",
            limit=50,
            target_operator="op-123",
        )
        assert p.execution_id == "exec-abc"
        assert p.file_path == "/etc/hosts"
        assert p.limit == 50
        assert p.target_operator == "op-123"

    def test_target_operator_optional(self):
        p = FetchFileHistoryRequestPayload(execution_id="exec-test", file_path="/etc/hosts")
        assert p.target_operator is None

    def test_inherits_from_targeted_operator_base(self):
        assert issubclass(FetchFileHistoryRequestPayload, TargetedOperatorBase)

    def test_is_g8e_base_model(self):
        assert issubclass(FetchFileHistoryRequestPayload, G8eBaseModel)

    def test_wire_dump_includes_target_operator(self):
        p = FetchFileHistoryRequestPayload(execution_id="exec-1", file_path="/app/config.json", target_operator="op-456")
        wire = p.model_dump(mode="json")
        assert wire["execution_id"] == "exec-1"
        assert wire["file_path"] == "/app/config.json"
        assert wire["target_operator"] == "op-456"

    def test_wire_dump_excludes_none_target_operator(self):
        p = FetchFileHistoryRequestPayload(execution_id="exec-1", file_path="/app/config.json")
        wire = p.model_dump(mode="json")
        assert wire["execution_id"] == "exec-1"
        assert "target_operator" not in wire


class TestFetchFileDiffRequestPayload:

    def test_all_fields_set(self):
        p = FetchFileDiffRequestPayload(
            execution_id="exec-abc",
            diff_id="diff-123",
            operator_session_id="session-456",
            file_path="/etc/hosts",
            limit=50,
            target_operator="op-789",
        )
        assert p.execution_id == "exec-abc"
        assert p.diff_id == "diff-123"
        assert p.operator_session_id == "session-456"
        assert p.file_path == "/etc/hosts"
        assert p.limit == 50
        assert p.target_operator == "op-789"

    def test_target_operator_optional(self):
        p = FetchFileDiffRequestPayload(execution_id="exec-test", diff_id="diff-123")
        assert p.target_operator is None

    def test_inherits_from_targeted_operator_base(self):
        assert issubclass(FetchFileDiffRequestPayload, TargetedOperatorBase)

    def test_is_g8e_base_model(self):
        assert issubclass(FetchFileDiffRequestPayload, G8eBaseModel)

    def test_wire_dump_includes_target_operator(self):
        p = FetchFileDiffRequestPayload(execution_id="exec-1", diff_id="diff-1", target_operator="op-456")
        wire = p.model_dump(mode="json")
        assert wire["execution_id"] == "exec-1"
        assert wire["diff_id"] == "diff-1"
        assert wire["target_operator"] == "op-456"

    def test_wire_dump_excludes_none_target_operator(self):
        p = FetchFileDiffRequestPayload(execution_id="exec-1", diff_id="diff-1")
        wire = p.model_dump(mode="json")
        assert wire["execution_id"] == "exec-1"
        assert "target_operator" not in wire


class TestCheckPortRequestPayload:

    def test_all_fields_set(self):
        p = CheckPortRequestPayload(
            execution_id="exec-abc",
            port=8080,
            host="192.168.1.1",
            protocol="tcp",
            target_operator="op-123",
        )
        assert p.execution_id == "exec-abc"
        assert p.port == 8080
        assert p.host == "192.168.1.1"
        assert p.protocol == "tcp"
        assert p.target_operator == "op-123"

    def test_target_operator_optional(self):
        p = CheckPortRequestPayload(execution_id="exec-test", port=443)
        assert p.target_operator is None

    def test_inherits_from_targeted_operator_base(self):
        assert issubclass(CheckPortRequestPayload, TargetedOperatorBase)

    def test_is_g8e_base_model(self):
        assert issubclass(CheckPortRequestPayload, G8eBaseModel)

    def test_wire_dump_includes_target_operator(self):
        p = CheckPortRequestPayload(execution_id="exec-1", port=22, target_operator="op-456")
        wire = p.model_dump(mode="json")
        assert wire["execution_id"] == "exec-1"
        assert wire["port"] == 22
        assert wire["target_operator"] == "op-456"

    def test_wire_dump_excludes_none_target_operator(self):
        p = CheckPortRequestPayload(execution_id="exec-1", port=22)
        wire = p.model_dump(mode="json")
        assert wire["execution_id"] == "exec-1"
        assert "target_operator" not in wire

