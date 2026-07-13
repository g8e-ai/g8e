"""Tests for g8e dynamic enum generation from protocol constants."""

from enum import IntEnum

import pytest

from g8e.constants import StrEnum
import g8e.enums
from g8e.enums import _build_enum, _build_event_type_enum, _to_pascal, _pascal_to_screaming_snake, _to_snake


class TestNameConversions:
    """Verify PascalCase <-> snake_case conversion helpers."""

    @pytest.mark.parametrize(
        "snake,expected",
        [
            ("action_status", "ActionStatus"),
            ("action_type", "ActionType"),
            ("priority", "Priority"),
            ("slash_tier", "SlashTier"),
        ],
    )
    def test_to_pascal(self, snake, expected):
        assert _to_pascal(snake) == expected

    @pytest.mark.parametrize(
        "pascal,expected",
        [
            ("AiLLMChat", "AI_LLM_CHAT"),
            ("G8eActionType", "G8E_ACTION_TYPE"),
            ("ActionStatus", "ACTION_STATUS"),
            ("EventType", "EVENT_TYPE"),
        ],
    )
    def test_pascal_to_screaming_snake(self, pascal, expected):
        assert _pascal_to_screaming_snake(pascal) == expected

    @pytest.mark.parametrize(
        "pascal,expected",
        [
            ("ActionStatus", "action_status"),
            ("AISource", "ai_source"),
            ("G8eActionType", "g8e_action_type"),
            ("ApprovalErrorType", "approval_error_type"),
        ],
    )
    def test_to_snake(self, pascal, expected):
        assert _to_snake(pascal) == expected


class TestStatusEnumGeneration:
    """Verify that status categories produce valid enums."""

    def test_action_status_enum_is_str_enum(self):
        cls = _build_enum("action_status")
        assert issubclass(cls, StrEnum)

    def test_action_status_enum_has_members(self):
        cls = _build_enum("action_status")
        members = list(cls)
        assert len(members) > 0

    def test_action_status_cancelled_value(self):
        cls = _build_enum("action_status")
        assert cls.CANCELLED == "cancelled"

    def test_action_status_completed_value(self):
        cls = _build_enum("action_status")
        assert cls.COMPLETED == "completed"

    def test_priority_enum_is_int_enum(self):
        cls = _build_enum("priority")
        assert issubclass(cls, IntEnum)

    def test_severity_enum_is_int_enum(self):
        cls = _build_enum("severity")
        assert issubclass(cls, IntEnum)

    def test_enum_values_preserve_protocol_wire_format(self):
        cls = _build_enum("action_status")
        assert cls.USER_CANCELLED == "user.cancelled"


class TestEventTypeEnum:
    """Verify EventType enum generation from EVENTS."""

    def test_event_type_is_str_enum(self):
        cls = _build_event_type_enum()
        assert issubclass(cls, StrEnum)

    def test_event_type_has_members(self):
        cls = _build_event_type_enum()
        members = list(cls)
        assert len(members) > 0

    def test_event_type_values_are_dotted_namespaces(self):
        cls = _build_event_type_enum()
        for member in cls:
            assert "." in member.value, (
                f"EventType member {member} value '{member.value}' should be dotted"
            )


class TestDynamicAttributeAccess:
    """Verify __getattr__ dynamic enum access works."""

    def test_access_action_status_via_getattr(self):
        cls = g8e.enums.ActionStatus
        assert issubclass(cls, StrEnum)
        assert cls.CANCELLED == "cancelled"

    def test_access_event_type_via_getattr(self):
        cls = g8e.enums.EventType
        assert issubclass(cls, StrEnum)

    def test_invalid_attribute_raises_attribute_error(self):
        with pytest.raises(AttributeError):
            _ = g8e.enums.NonExistentEnum

    def test_dir_lists_all_enums(self):
        names = dir(g8e.enums)
        assert "EventType" in names
        assert "ActionStatus" in names
