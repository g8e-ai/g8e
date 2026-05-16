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
Robust structured-response parsing for LLM output.

Small local models (e.g. gemma4:e2b) routinely ignore the JSON-schema
`format` hint and return fenced blocks, prose-wrapped JSON, truncated
JSON, or bare enum/primitive values. Every call site that validates an
LLM response against a Pydantic model needs the same recovery ladder,
so the rules live here rather than in each consumer.

Recovery order (stops at first success):

1. Direct `model_validate_json` on the stripped text.
2. Strip a fenced ```json ... ``` (or ``` ... ```) block and retry.
3. Extract the substring from the first `{` to the last `}` and retry.
4. If the text starts with `{` but is truncated, try common closing suffixes.
5. For a schema with exactly one required field whose resolved JSON-Schema
   type is a primitive (string/number/integer/boolean) or `enum`, coerce
   the bare text into `{field: text}` and validate.

Raises the original ValidationError/ValueError from step 1 when every
strategy fails, so callers see a faithful error and can fall back.
"""

import json
import re
from typing import TypeVar

from app.models.base import G8eBaseModel

__all__ = ["parse_structured_response"]


_JSON_FENCE_RE = re.compile(r"```(?:json)?\s*(.*?)\s*```", re.DOTALL | re.IGNORECASE)
_TRUNCATED_JSON_SUFFIXES: tuple[str, ...] = ("}", "]}", "}}", "}}]", "]}")
_PRIMITIVE_JSON_TYPES: frozenset[str] = frozenset({"string", "number", "integer", "boolean"})


T = TypeVar("T", bound=G8eBaseModel)


def parse_structured_response(
    response_text: str | None,
    response_model: type[T],
    *,
    allow_bare_value: bool = True,
) -> T:
    """Parse an LLM structured response into a Pydantic model with recovery.

    Args:
        response_text: Raw text from the LLM. None or empty is treated as
            "empty response" and raises the same ValidationError that
            ``model_validate_json("")`` would produce.
        response_model: The target Pydantic model class.
        allow_bare_value: When True (default), enable bare-value coercion
            for single primitive/enum-required-field schemas.

    Returns:
        A validated instance of ``response_model``.

    Raises:
        The original exception from the first direct parse attempt when no
        recovery strategy produces a valid model.
    """
    stripped = (response_text or "").strip()
    try:
        return response_model.model_validate_json(stripped)
    except Exception as first_error:
        fenced = _JSON_FENCE_RE.search(stripped)
        if fenced:
            inner = fenced.group(1).strip()
            try:
                return response_model.model_validate_json(inner)
            except Exception:
                pass

        start = stripped.find("{")
        end = stripped.rfind("}")
        if start != -1 and end != -1 and end > start:
            try:
                return response_model.model_validate_json(stripped[start : end + 1])
            except Exception:
                pass

        if start != -1 and end == -1:
            truncated = stripped[start:]
            for suffix in _TRUNCATED_JSON_SUFFIXES:
                try:
                    return response_model.model_validate_json(truncated + suffix)
                except Exception:
                    continue

        if allow_bare_value:
            coerced = _coerce_bare_value(stripped, response_model)
            if coerced is not None:
                return coerced

        raise first_error


def _coerce_bare_value(text: str, response_model: type[T]) -> T | None:
    """Wrap bare text into ``{field: text}`` when the schema permits.

    Only runs when the model has exactly one required field whose resolved
    JSON-Schema type is a primitive (string/number/integer/boolean) or
    carries an ``enum`` list. Guards against nested-object fields where a
    bare string would silently misrepresent structured data.
    """
    schema = response_model.model_json_schema()
    required = schema.get("required") or []
    if len(required) != 1:
        return None
    field_name = required[0]
    if not _field_is_primitive_or_enum(schema, field_name):
        return None
    bare = text.strip().strip('"').strip("'").strip()
    if not bare:
        return None
    try:
        return response_model.model_validate({field_name: bare})
    except Exception:
        try:
            return response_model.model_validate({field_name: json.loads(bare)})
        except Exception:
            return None


def _field_is_primitive_or_enum(schema: dict, field_name: str) -> bool:
    """Return True when ``field_name`` resolves to a primitive or enum type."""
    props = schema.get("properties") or {}
    field_schema = props.get(field_name) or {}
    resolved = _resolve_ref(field_schema, schema)
    if resolved.get("enum"):
        return True
    field_type = resolved.get("type")
    if isinstance(field_type, list):
        return any(t in _PRIMITIVE_JSON_TYPES for t in field_type)
    return field_type in _PRIMITIVE_JSON_TYPES


def _resolve_ref(node: dict, root: dict) -> dict:
    """Resolve a single ``$ref`` inside the root schema's ``$defs`` block."""
    ref = node.get("$ref")
    if not ref or not ref.startswith("#/$defs/"):
        return node
    def_name = ref[len("#/$defs/"):]
    return (root.get("$defs") or {}).get(def_name) or node
