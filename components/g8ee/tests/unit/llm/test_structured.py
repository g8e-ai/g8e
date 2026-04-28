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

"""Unit tests for the shared structured-response parser."""

from enum import Enum

import pytest
from pydantic import ValidationError

from app.llm.structured import parse_structured_response
from app.models.base import G8eBaseModel

pytestmark = [pytest.mark.unit]


class Level(str, Enum):
    LOW = "LOW"
    MEDIUM = "MEDIUM"
    HIGH = "HIGH"


class SingleEnum(G8eBaseModel):
    risk_level: Level


class SingleInt(G8eBaseModel):
    count: int


class MultiRequired(G8eBaseModel):
    a: str
    b: str


class Nested(G8eBaseModel):
    value: str


class SingleNested(G8eBaseModel):
    nested: Nested


class OptionalOnly(G8eBaseModel):
    a: str | None = None
    b: str | None = None


# ---------------------------------------------------------------------------
# Happy-path recovery strategies
# ---------------------------------------------------------------------------

def test_direct_json_parse():
    result = parse_structured_response('{"risk_level": "LOW"}', SingleEnum)
    assert result.risk_level == Level.LOW


def test_strips_leading_trailing_whitespace():
    result = parse_structured_response('\n  {"risk_level": "HIGH"}  \n', SingleEnum)
    assert result.risk_level == Level.HIGH


def test_fenced_json_block():
    result = parse_structured_response(
        '```json\n{"risk_level": "MEDIUM"}\n```', SingleEnum
    )
    assert result.risk_level == Level.MEDIUM


def test_fenced_block_without_language_tag():
    result = parse_structured_response(
        '```\n{"risk_level": "LOW"}\n```', SingleEnum
    )
    assert result.risk_level == Level.LOW


def test_json_embedded_after_preamble():
    result = parse_structured_response(
        'Here is my answer:\n{"risk_level": "HIGH"}\nThanks.', SingleEnum
    )
    assert result.risk_level == Level.HIGH


def test_truncated_json_single_closing_brace():
    result = parse_structured_response('{"risk_level": "LOW"', SingleEnum)
    assert result.risk_level == Level.LOW


# ---------------------------------------------------------------------------
# Bare-value coercion
# ---------------------------------------------------------------------------

def test_bare_enum_value():
    result = parse_structured_response("LOW", SingleEnum)
    assert result.risk_level == Level.LOW


def test_bare_enum_with_surrounding_quotes():
    result = parse_structured_response('"MEDIUM"', SingleEnum)
    assert result.risk_level == Level.MEDIUM


def test_bare_integer_coerced_via_json_loads():
    result = parse_structured_response("42", SingleInt)
    assert result.count == 42


def test_bare_value_disabled_still_accepts_real_json():
    """allow_bare_value=False must still succeed on valid JSON."""
    result = parse_structured_response(
        '{"risk_level": "LOW"}', SingleEnum, allow_bare_value=False
    )
    assert result.risk_level == Level.LOW


def test_bare_value_disabled_raises_on_bare_input():
    with pytest.raises(ValidationError):
        parse_structured_response("LOW", SingleEnum, allow_bare_value=False)


# ---------------------------------------------------------------------------
# Guards against over-coercion
# ---------------------------------------------------------------------------

def test_bare_value_refused_for_multi_required_schema():
    """A schema with >1 required field must never coerce a bare string."""
    with pytest.raises(ValidationError):
        parse_structured_response("something", MultiRequired)


def test_bare_value_refused_for_nested_object_field():
    """Single required field whose type is a nested model must not be
    coerced from a bare string — that would silently misrepresent data."""
    with pytest.raises(ValidationError):
        parse_structured_response("something", SingleNested)


def test_empty_input_raises():
    with pytest.raises(ValidationError):
        parse_structured_response("", SingleEnum)


def test_none_input_raises():
    with pytest.raises(ValidationError):
        parse_structured_response(None, SingleEnum)


# ---------------------------------------------------------------------------
# No-required-field models fall back to direct JSON behavior
# ---------------------------------------------------------------------------

def test_optional_only_accepts_empty_object():
    result = parse_structured_response("{}", OptionalOnly)
    assert result.a is None
    assert result.b is None


def test_optional_only_rejects_bare_string():
    """Zero required fields => bare coercion must never run."""
    with pytest.raises(ValidationError):
        parse_structured_response("hello", OptionalOnly)
