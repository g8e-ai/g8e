# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for SessionType and EventType re-exports from g8e.

Verifies:
- SessionType is re-exported from g8e.enums with WEB, OPERATOR, CLI, APP members
- EventType is re-exported from g8e.enums (includes intent members)
- Both are StrEnum subclasses
"""

from enum import StrEnum

import pytest

from g8e.enums import EventType as G8eEventType
from g8e.enums import SessionType as G8eSessionType

from app.constants.generated_status import EventType, SessionType

pytestmark = pytest.mark.unit


class TestSessionType:
    """Verify SessionType is re-exported from g8e with all 4 members."""

    def test_is_strenum(self):
        assert issubclass(SessionType, StrEnum)

    def test_has_web(self):
        assert SessionType.WEB == "web"

    def test_has_operator(self):
        assert SessionType.OPERATOR == "operator"

    def test_has_cli(self):
        assert SessionType.CLI == "cli"

    def test_has_app(self):
        assert SessionType.APP == "app"

    def test_member_count(self):
        assert len(list(SessionType)) == 4

    def test_is_g8e_reexport(self):
        assert SessionType is G8eSessionType

    def test_cli_in_g8e(self):
        g8e_members = {m.name for m in G8eSessionType}
        assert "CLI" in g8e_members


class TestEventType:
    """Verify EventType is re-exported from g8e (includes intent members)."""

    def test_is_strenum(self):
        assert issubclass(EventType, StrEnum)

    def test_is_g8e_reexport(self):
        assert EventType is G8eEventType

    @pytest.mark.parametrize(
        "member_name,expected_value",
        [
            ("OPERATOR_INTENT_REQUESTED", "g8e.v1.operator.intent.requested"),
            ("OPERATOR_INTENT_REVOKE_REQUESTED", "g8e.v1.operator.intent.revoke.requested"),
            ("OPERATOR_INTENT_APPROVAL_REQUESTED", "g8e.v1.operator.intent.approval.requested"),
            ("OPERATOR_INTENT_DENIED", "g8e.v1.operator.intent.denied"),
            ("OPERATOR_INTENT_GRANTED", "g8e.v1.operator.intent.granted"),
            ("OPERATOR_INTENT_REVOKED", "g8e.v1.operator.intent.revoked"),
            ("OPERATOR_INTENT_APPROVAL_REJECTED", "g8e.v1.operator.intent.approval.rejected"),
            ("OPERATOR_INTENT_APPROVAL_GRANTED", "g8e.v1.operator.intent.approval.granted"),
        ],
    )
    def test_intent_member_exists(self, member_name: str, expected_value: str):
        assert hasattr(EventType, member_name), f"EventType.{member_name} not found"
        assert EventType[member_name].value == expected_value

    def test_intent_members_in_g8e(self):
        g8e_names = {m.name for m in G8eEventType}
        intent_names = {
            "OPERATOR_INTENT_REQUESTED",
            "OPERATOR_INTENT_REVOKE_REQUESTED",
            "OPERATOR_INTENT_APPROVAL_REQUESTED",
            "OPERATOR_INTENT_DENIED",
            "OPERATOR_INTENT_GRANTED",
            "OPERATOR_INTENT_REVOKED",
            "OPERATOR_INTENT_APPROVAL_REJECTED",
            "OPERATOR_INTENT_APPROVAL_GRANTED",
        }
        for name in intent_names:
            assert name in g8e_names, f"{name} should be in g8e EventType"

    def test_total_member_count(self):
        assert len(list(EventType)) == len(list(G8eEventType))

    def test_intent_values_use_dotted_naming(self):
        intent_members = [
            EventType.OPERATOR_INTENT_REQUESTED,
            EventType.OPERATOR_INTENT_REVOKE_REQUESTED,
            EventType.OPERATOR_INTENT_APPROVAL_REQUESTED,
            EventType.OPERATOR_INTENT_DENIED,
            EventType.OPERATOR_INTENT_GRANTED,
            EventType.OPERATOR_INTENT_REVOKED,
            EventType.OPERATOR_INTENT_APPROVAL_REJECTED,
            EventType.OPERATOR_INTENT_APPROVAL_GRANTED,
        ]
        for member in intent_members:
            assert member.value.startswith("g8e.v1.operator.intent."), (
                f"{member.name} value {member.value!r} does not use dotted naming convention"
            )
