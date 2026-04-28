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
JSON Utilities for g8e Components

Shared utilities for JSON serialization and deserialization.
"""

import json
import logging
import re
from datetime import date, datetime
from decimal import Decimal
from functools import partial
from typing import Any

logger = logging.getLogger(__name__)


def extract_json_from_text(text: str) -> dict | None:
    """Extract and parse a JSON object from a potentially messy text string.
    
    Handles:
    - Markdown code blocks (```json ... ```)
    - Leading/trailing whitespace
    - Empty input
    """
    if not text or not text.strip():
        return None

    cleaned = text.strip()

    # Try simple parse first
    try:
        data = json.loads(cleaned)
        if isinstance(data, dict):
            return data
    except json.JSONDecodeError:
        pass

    # Try stripping Markdown fences
    # Matches ```json { ... } ``` or ``` { ... } ```
    pattern = r"```(?:json)?\s*(\{.*?\})\s*```"
    match = re.search(pattern, cleaned, re.DOTALL)
    if match:
        try:
            data = json.loads(match.group(1).strip())
            if isinstance(data, dict):
                return data
        except json.JSONDecodeError:
            pass

    # Try finding first '{' and last '}'
    start = cleaned.find("{")
    end = cleaned.rfind("}")
    if start != -1 and end != -1 and end > start:
        try:
            data = json.loads(cleaned[start : end + 1])
            if isinstance(data, dict):
                return data
        except json.JSONDecodeError:
            pass

    return None


def _json_serial(obj):
    """JSON serializer for objects not handled by default json.dumps."""
    if isinstance(obj, datetime):
        return obj.isoformat()
    if isinstance(obj, date):
        return obj.isoformat()
    if isinstance(obj, Decimal):
        return float(obj)
    raise TypeError(f"Object of type {type(obj).__name__} is not JSON serializable")


_json_dumps = partial(json.dumps, default=_json_serial)


class DateTimeEncoder(json.JSONEncoder):
    """Custom JSON encoder that handles datetime objects and other special types."""

    def default(self, o: Any) -> Any:
        if isinstance(o, datetime) or isinstance(o, date):
            return o.isoformat()
        if isinstance(o, Decimal):
            return float(o)
        if hasattr(o, "__dict__"):
            return o.__dict__
        return super().default(o)


def dumps_with_datetime(obj: Any, **kwargs) -> str:
    """Dump JSON with datetime support."""
    return json.dumps(obj, cls=DateTimeEncoder, **kwargs)


def loads_with_datetime(json_str: str, **kwargs) -> Any:
    """Load JSON and attempt to parse ISO datetime strings."""

    def datetime_parser(dct):
        for key, value in dct.items():
            if isinstance(value, str):
                try:
                    if "T" in value:
                        if value.endswith("Z"):
                            dct[key] = datetime.fromisoformat(value.replace("Z", "+00:00"))
                        else:
                            dct[key] = datetime.fromisoformat(value)
                except (ValueError, AttributeError):
                    pass
        return dct

    return json.loads(json_str, object_hook=datetime_parser, **kwargs)
