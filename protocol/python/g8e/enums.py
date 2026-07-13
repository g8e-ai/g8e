"""Dynamic enum generation from protocol constants.

Generates Python ``StrEnum`` / ``IntEnum`` classes from the ``STATUS`` and
``EVENTS`` dicts in :mod:`g8e.constants`, so downstream consumers (g8ee,
evals, CLI) can use typed enums instead of raw string lookups.

Categories whose values are integer-like produce ``IntEnum``; all others
produce ``StrEnum``.

Enum **member names** are SCREAMING_SNAKE_CASE (derived from the JSON
``_python_const`` or PascalCase event keys).  Enum **values** preserve
the raw protocol wire format (e.g. ``"g8e.v1.ai.llm.chat.iteration.thinking.started"``,
``"user.cancelled"``, ``"G8E-1000"``) so they round-trip exactly to
Go/protobuf/JVM consumers.
"""

from __future__ import annotations

import re
from enum import IntEnum
from functools import lru_cache

from g8e.constants import STATUS, EVENTS, StrEnum


# Categories that use integer values
_INT_CATEGORIES: frozenset[str] = frozenset({
    "citation_layout",
    "priority",
    "scrubber_priority",
    "severity",
    "slash_tier",
})


def _to_pascal(snake: str) -> str:
    """Convert snake_case to PascalCase, handling dotted names."""
    parts = snake.replace(".", "_").split("_")
    return "".join(p.capitalize() for p in parts)


def _pascal_to_screaming_snake(name: str) -> str:
    """Convert PascalCase to SCREAMING_SNAKE_CASE.

    Handles consecutive capitals: ``AiLLMChat`` -> ``AI_LLM_CHAT``,
    ``G8eActionType`` -> ``G8E_ACTION_TYPE``.
    """
    s = re.sub(r"(?<=[a-z0-9])(?=[A-Z])", "_", name)
    s = re.sub(r"(?<=[A-Z])(?=[A-Z][a-z])", "_", s)
    s = re.sub(r"(?<=[a-z])(?=[0-9])", "_", s)
    return s.upper()


@lru_cache(maxsize=None)
def _build_enum(cat_name: str) -> type:
    """Build an enum class from a STATUS category."""
    cat_vals = STATUS["status"][cat_name]

    # Determine base class
    sample_val = next(iter(cat_vals.values()))["value"]
    is_int = isinstance(sample_val, (int,)) or (
        isinstance(sample_val, str) and sample_val.lstrip("-").isdigit()
    )
    base = IntEnum if (is_int or cat_name in _INT_CATEGORIES) else StrEnum

    # Build members
    members: dict[str, str | int] = {}
    for _key, meta in cat_vals.items():
        py_name = meta["_python_const"]
        raw_val = meta["value"]
        if base is IntEnum:
            members[py_name] = int(raw_val)
        else:
            members[py_name] = str(raw_val)

    cls_name = _to_pascal(cat_name)
    cls = base(cls_name, members)  # type: ignore[arg-type]
    return cls


@lru_cache(maxsize=None)
def _build_event_type_enum() -> type:
    """Build the EventType enum from the EVENTS dict.

    Event keys are PascalCase (e.g. ``AiLLMChatIterationStarted``).
    Member names are SCREAMING_SNAKE_CASE; values preserve the raw
    protocol wire format (e.g. ``g8e.v1.ai.llm.chat.iteration.started``).
    """
    evts = EVENTS.get("events", {})
    members: dict[str, str] = {}
    for key, meta in evts.items():
        member_name = _pascal_to_screaming_snake(key)
        members[member_name] = meta["value"]

    return StrEnum("EventType", members)  # type: ignore[arg-type]


def _to_snake(pascal: str) -> str:
    """Convert PascalCase to snake_case, handling consecutive capitals.

    e.g. AISource -> ai_source, G8eActionType -> g8e_action_type,
         ApprovalErrorType -> approval_error_type
    """
    # Insert _ before each uppercase that follows a lowercase or digit,
    # and before each uppercase that precedes a lowercase
    s = re.sub(r"(?<=[a-z0-9])(?=[A-Z])", "_", pascal)
    s = re.sub(r"(?<=[A-Z])(?=[A-Z][a-z])", "_", s)
    return s.lower()


def __getattr__(name: str):
    """Dynamic attribute access: g8e.enums.OperatorToolName, EventType, etc."""
    if name == "EventType":
        return _build_event_type_enum()

    snake = _to_snake(name)
    if snake in STATUS["status"]:
        return _build_enum(snake)

    # Try dotted-name conversion (e.g. ApprovalErrorType -> approval.error.type)
    for cat in STATUS["status"]:
        pascal = _to_pascal(cat)
        if pascal == name:
            return _build_enum(cat)

    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")


def __dir__() -> list[str]:
    """List all available enum class names."""
    names = [_to_pascal(cat) for cat in STATUS["status"]]
    names.append("EventType")
    return names


__all__ = [_to_pascal(cat) for cat in STATUS["status"]] + ["EventType"]
