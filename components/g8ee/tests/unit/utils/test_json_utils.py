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

"""Unit tests for app/utils/json_utils.py."""

import json
from datetime import UTC, date, datetime
from decimal import Decimal

import pytest

from app.utils.json_utils import (
    DateTimeEncoder,
    _json_dumps,
    _json_serial,
    dumps_with_datetime,
    extract_json_from_text,
    loads_with_datetime,
)

pytestmark = pytest.mark.unit


class TestJsonSerial:

    def test_datetime_serialized_as_iso(self):
        dt = datetime(2026, 3, 15, 12, 30, 0, tzinfo=UTC)
        result = _json_serial(dt)
        assert result == dt.isoformat()

    def test_date_serialized_as_iso(self):
        d = date(2026, 3, 15)
        result = _json_serial(d)
        assert str(result) == "2026-03-15"

    def test_decimal_serialized_as_float(self):
        result = _json_serial(Decimal("3.14"))
        assert result == 3.14
        assert isinstance(result, float)

    def test_unhandled_type_raises_type_error(self):
        with pytest.raises(TypeError, match="not JSON serializable"):
            _json_serial(object())

    def test_naive_datetime_serialized(self):
        dt = datetime(2026, 1, 1, 0, 0, 0)
        result = _json_serial(dt)
        assert "2026-01-01" in str(result)


class TestJsonDumps:

    def test_serializes_dict_with_datetime(self):
        dt = datetime(2026, 3, 15, 10, 0, 0, tzinfo=UTC)
        result = _json_dumps({"ts": dt})
        parsed = json.loads(result)
        assert "2026-03-15" in parsed["ts"]

    def test_serializes_dict_with_date(self):
        d = date(2026, 3, 15)
        result = _json_dumps({"d": d})
        assert '"2026-03-15"' in result

    def test_serializes_dict_with_decimal(self):
        result = _json_dumps({"val": Decimal("1.5")})
        parsed = json.loads(result)
        assert parsed["val"] == 1.5

    def test_plain_types_pass_through(self):
        result = _json_dumps({"a": 1, "b": "hello", "c": True, "d": None})
        parsed = json.loads(result)
        assert parsed == {"a": 1, "b": "hello", "c": True, "d": None}


class TestDateTimeEncoder:

    def test_encodes_datetime(self):
        dt = datetime(2026, 3, 15, 12, 0, 0, tzinfo=UTC)
        result = json.dumps({"ts": dt}, cls=DateTimeEncoder)
        parsed = json.loads(result)
        assert "2026-03-15" in parsed["ts"]

    def test_encodes_date(self):
        d = date(2026, 3, 15)
        result = json.dumps({"d": d}, cls=DateTimeEncoder)
        parsed = json.loads(result)
        assert parsed["d"] == "2026-03-15"

    def test_encodes_decimal(self):
        result = json.dumps({"val": Decimal("9.99")}, cls=DateTimeEncoder)
        parsed = json.loads(result)
        assert abs(parsed["val"] - 9.99) < 1e-6

    def test_encodes_object_with_dict(self):
        class _Obj:
            def __init__(self):
                self.x = 1
                self.y = "hello"

        result = json.dumps({"obj": _Obj()}, cls=DateTimeEncoder)
        parsed = json.loads(result)
        assert parsed["obj"]["x"] == 1
        assert parsed["obj"]["y"] == "hello"

    def test_falls_back_to_super_for_unknown(self):
        encoder = DateTimeEncoder()
        with pytest.raises(TypeError):
            encoder.default(set())


class TestDumpsWithDatetime:

    def test_returns_string(self):
        result = dumps_with_datetime({"key": "value"})
        assert isinstance(result, str)

    def test_datetime_in_output(self):
        dt = datetime(2026, 6, 1, 0, 0, 0, tzinfo=UTC)
        result = dumps_with_datetime({"ts": dt})
        assert "2026-06-01" in result

    def test_nested_datetime(self):
        dt = datetime(2026, 1, 15, tzinfo=UTC)
        result = dumps_with_datetime({"outer": {"ts": dt}})
        parsed = json.loads(result)
        assert "2026-01-15" in parsed["outer"]["ts"]

    def test_plain_dict_round_trips(self):
        data = {"a": 1, "b": "str", "c": None}
        result = dumps_with_datetime(data)
        assert json.loads(result) == data


class TestLoadsWithDatetime:

    def test_parses_iso_string_with_t_to_datetime(self):
        raw = json.dumps({"ts": "2026-03-15T12:30:00"})
        result = loads_with_datetime(raw)
        assert isinstance(result["ts"], datetime)
        assert result["ts"].year == 2026

    def test_parses_z_suffix_iso_string(self):
        raw = json.dumps({"ts": "2026-03-15T12:30:00Z"})
        result = loads_with_datetime(raw)
        assert isinstance(result["ts"], datetime)
        assert result["ts"].tzinfo is not None

    def test_non_iso_string_left_as_string(self):
        raw = json.dumps({"name": "alice"})
        result = loads_with_datetime(raw)
        assert result["name"] == "alice"
        assert isinstance(result["name"], str)

    def test_numeric_values_unchanged(self):
        raw = json.dumps({"count": 42})
        result = loads_with_datetime(raw)
        assert result["count"] == 42

    def test_string_without_t_left_as_string(self):
        raw = json.dumps({"date": "2026-03-15"})
        result = loads_with_datetime(raw)
        assert isinstance(result["date"], str)

    def test_invalid_iso_like_string_left_as_string(self):
        raw = json.dumps({"ts": "not-a-real-T-datetime"})
        result = loads_with_datetime(raw)
        assert isinstance(result["ts"], str)

    def test_nested_object_strings_parsed(self):
        raw = json.dumps({"outer": {"ts": "2026-01-01T00:00:00"}})
        result = loads_with_datetime(raw)
        assert isinstance(result["outer"]["ts"], datetime)


class TestExtractJsonFromText:

    def test_extract_simple_json(self):
        text = '{"status": "ok"}'
        result = extract_json_from_text(text)
        assert result == {"status": "ok"}

    def test_extract_with_whitespace(self):
        text = '   {"status": "ok"}   '
        result = extract_json_from_text(text)
        assert result == {"status": "ok"}

    def test_extract_from_markdown_fence(self):
        text = '```json\n{"status": "ok"}\n```'
        result = extract_json_from_text(text)
        assert result == {"status": "ok"}

    def test_extract_from_plain_markdown_fence(self):
        text = '```\n{"status": "ok"}\n```'
        result = extract_json_from_text(text)
        assert result == {"status": "ok"}

    def test_extract_with_preamble(self):
        text = 'Here is the response:\n```json\n{"status": "ok"}\n```'
        result = extract_json_from_text(text)
        assert result == {"status": "ok"}

    def test_extract_partial_json_match(self):
        text = 'Some text before {"status": "ok"} some text after'
        result = extract_json_from_text(text)
        assert result == {"status": "ok"}

    def test_returns_none_for_invalid_json(self):
        text = 'not json'
        result = extract_json_from_text(text)
        assert result is None

    def test_returns_none_for_empty_input(self):
        assert extract_json_from_text("") is None
        assert extract_json_from_text(None) is None
