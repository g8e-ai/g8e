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

Covers FsListPayload and FsReadPayload:
- Construction with required and optional fields
- flatten_for_wire() produces only canonical wire fields
- flatten_for_wire() excludes None-valued optional fields
- Pydantic rejects invalid types
- Model is a G8eBaseModel subclass
"""

import pytest
from app.models.base import ValidationError, G8eBaseModel
from app.models.command_payloads import FetchLogsPayload, FsListPayload, FsReadPayload

pytestmark = [pytest.mark.unit]


class TestFsListPayload:

    def test_all_fields_set(self):
        p = FsListPayload(
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
        p = FsListPayload()
        assert p.path is None
        assert p.execution_id is None
        assert p.max_depth is None
        assert p.max_entries is None

    def test_is_g8e_base_model(self):
        assert issubclass(FsListPayload, G8eBaseModel)

    def test_wire_dump_includes_set_fields(self):
        p = FsListPayload(path="/app", execution_id="exec-1", max_depth=1, max_entries=50)
        wire = p.model_dump(mode="json")
        assert wire["path"] == "/app"
        assert wire["execution_id"] == "exec-1"
        assert wire["max_depth"] == 1
        assert wire["max_entries"] == 50

    def test_wire_dump_excludes_none_fields(self):
        p = FsListPayload(path="/app")
        wire = p.model_dump(mode="json")
        assert "execution_id" not in wire
        assert "max_depth" not in wire
        assert "max_entries" not in wire

    def test_wire_dump_only_canonical_fields(self):
        p = FsListPayload(path=".", execution_id="x", max_depth=0, max_entries=100)
        wire = p.model_dump(mode="json")
        assert set(wire.keys()) <= {"path", "execution_id", "max_depth", "max_entries"}

    def test_wire_dump_no_non_canonical_fields(self):
        p = FsListPayload(path="/tmp")
        wire = p.model_dump(mode="json")
        assert "requested_at" not in wire
        assert "source" not in wire
        assert "user_id" not in wire

    def test_extra_fields_ignored(self):
        p = FsListPayload(path="/tmp", user_id="u-1", source="tool_call", requested_at="2026-01-01T00:00:00Z")
        wire = p.model_dump(mode="json")
        assert "user_id" not in wire
        assert "source" not in wire
        assert "requested_at" not in wire



class TestFsReadPayload:

    def test_required_path(self):
        p = FsReadPayload(path="/etc/hosts")
        assert p.path == "/etc/hosts"

    def test_missing_path_raises(self):
        with pytest.raises(ValidationError):
            FsReadPayload()

    def test_all_fields_set(self):
        p = FsReadPayload(path="/app/config.json", execution_id="exec-xyz", max_size=8192)
        assert p.path == "/app/config.json"
        assert p.execution_id == "exec-xyz"
        assert p.max_size == 8192

    def test_optional_fields_default_to_none(self):
        p = FsReadPayload(path="/app/config.json")
        assert p.execution_id is None
        assert p.max_size is None

    def test_is_g8e_base_model(self):
        assert issubclass(FsReadPayload, G8eBaseModel)

    def test_wire_dump_includes_set_fields(self):
        p = FsReadPayload(path="/etc/passwd", execution_id="exec-2", max_size=4096)
        wire = p.model_dump(mode="json")
        assert wire["path"] == "/etc/passwd"
        assert wire["execution_id"] == "exec-2"
        assert wire["max_size"] == 4096

    def test_wire_dump_excludes_none_optional_fields(self):
        p = FsReadPayload(path="/app/log.txt")
        wire = p.model_dump(mode="json")
        assert "execution_id" not in wire
        assert "max_size" not in wire

    def test_wire_dump_only_canonical_fields(self):
        p = FsReadPayload(path="/app/main.py", execution_id="e", max_size=102400)
        wire = p.model_dump(mode="json")
        assert set(wire.keys()) <= {"path", "execution_id", "max_size"}

    def test_wire_dump_no_non_canonical_fields(self):
        p = FsReadPayload(path="/var/log/app.log")
        wire = p.model_dump(mode="json")
        assert "requested_at" not in wire
        assert "source" not in wire
        assert "user_id" not in wire

    def test_extra_fields_ignored(self):
        p = FsReadPayload(path="/tmp/test.txt", user_id="u-1", source="tool_call")
        wire = p.model_dump(mode="json")
        assert "user_id" not in wire
        assert "source" not in wire



class TestFetchLogsPayload:

    def test_requiredexecution_id(self):
        p = FetchLogsPayload(execution_id="exec-abc")
        assert p.execution_id == "exec-abc"

    def test_missingexecution_id_raises(self):
        with pytest.raises(ValidationError):
            FetchLogsPayload()

    def test_all_fields_set(self):
        p = FetchLogsPayload(execution_id="exec-abc", sentinel_mode="scrubbed")
        assert p.execution_id == "exec-abc"
        assert p.sentinel_mode == "scrubbed"

    def test_sentinel_mode_optional(self):
        p = FetchLogsPayload(execution_id="exec-abc")
        assert p.sentinel_mode is None

    def test_is_g8e_base_model(self):
        assert issubclass(FetchLogsPayload, G8eBaseModel)

    def test_wire_dump_includes_set_fields(self):
        p = FetchLogsPayload(execution_id="exec-xyz", sentinel_mode="raw")
        wire = p.model_dump(mode="json")
        assert wire["execution_id"] == "exec-xyz"
        assert wire["sentinel_mode"] == "raw"

    def test_wire_dump_excludes_none_sentinel_mode(self):
        p = FetchLogsPayload(execution_id="exec-xyz")
        wire = p.model_dump(mode="json")
        assert wire["execution_id"] == "exec-xyz"
        assert "sentinel_mode" not in wire

    def test_wire_dump_only_canonical_fields(self):
        p = FetchLogsPayload(execution_id="exec-xyz", sentinel_mode="scrubbed")
        wire = p.model_dump(mode="json")
        assert set(wire.keys()) <= {"execution_id", "sentinel_mode"}

    def test_wire_dump_no_non_canonical_fields(self):
        p = FetchLogsPayload(execution_id="exec-xyz")
        wire = p.model_dump(mode="json")
        assert "requested_at" not in wire
        assert "source" not in wire
        assert "user_id" not in wire

    def test_extra_fields_ignored(self):
        p = FetchLogsPayload(execution_id="exec-xyz", source="tool_call", requested_at="2026-01-01T00:00:00Z")
        wire = p.model_dump(mode="json")
        assert "source" not in wire
        assert "requested_at" not in wire

