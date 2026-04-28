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
Contract test: g8ee PubSubWireEventType, PubSubAction, and PubSubField enums
must exactly match the canonical values in shared/constants/pubsub.json.

Prevents the class of bug where g8ee defines wire protocol constants independently
and a typo or omission silently breaks the pub/sub pipeline (see Bug 2 in the
operator heartbeat TimeoutError fix).
"""

import json

import pytest

from app.constants import (
    PubSubAction,
    PubSubField,
    PubSubWireEventType,
)

pytestmark = pytest.mark.unit


def _load_pubsub_json() -> dict:
    with open("/app/shared/constants/pubsub.json") as f:
        return json.load(f)


class TestPubSubWireEventTypesMatchSharedJSON:
    """PubSubWireEventType enum values must match shared/constants/pubsub.json wire.event_types."""

    def test_message_matches(self):
        wire = _load_pubsub_json()["wire"]["event_types"]
        assert PubSubWireEventType.MESSAGE.value == wire["message"]

    def test_pmessage_matches(self):
        wire = _load_pubsub_json()["wire"]["event_types"]
        assert PubSubWireEventType.PMESSAGE.value == wire["pmessage"]

    def test_subscribed_matches(self):
        wire = _load_pubsub_json()["wire"]["event_types"]
        assert PubSubWireEventType.SUBSCRIBED.value == wire["subscribed"]

    def test_all_json_event_types_have_enum_members(self):
        wire = _load_pubsub_json()["wire"]["event_types"]
        for key, value in wire.items():
            assert value in PubSubWireEventType._value2member_map_, (
                f"shared/constants/pubsub.json wire.event_types.{key}={value!r} "
                f"has no PubSubWireEventType member"
            )

    def test_no_extra_enum_members_beyond_json(self):
        wire = _load_pubsub_json()["wire"]["event_types"]
        json_values = set(wire.values())
        for member in PubSubWireEventType:
            assert member.value in json_values, (
                f"PubSubWireEventType.{member.name}={member.value!r} "
                f"not in shared/constants/pubsub.json"
            )


class TestPubSubActionsMatchSharedJSON:
    """PubSubAction enum values must match shared/constants/pubsub.json wire.actions."""

    def test_subscribe_matches(self):
        wire = _load_pubsub_json()["wire"]["actions"]
        assert PubSubAction.SUBSCRIBE.value == wire["subscribe"]

    def test_psubscribe_matches(self):
        wire = _load_pubsub_json()["wire"]["actions"]
        assert PubSubAction.PSUBSCRIBE.value == wire["psubscribe"]

    def test_unsubscribe_matches(self):
        wire = _load_pubsub_json()["wire"]["actions"]
        assert PubSubAction.UNSUBSCRIBE.value == wire["unsubscribe"]

    def test_publish_matches(self):
        wire = _load_pubsub_json()["wire"]["actions"]
        assert PubSubAction.PUBLISH.value == wire["publish"]

    def test_all_json_actions_have_enum_members(self):
        wire = _load_pubsub_json()["wire"]["actions"]
        for key, value in wire.items():
            assert value in PubSubAction._value2member_map_, (
                f"shared/constants/pubsub.json wire.actions.{key}={value!r} "
                f"has no PubSubAction member"
            )

    def test_no_extra_enum_members_beyond_json(self):
        wire = _load_pubsub_json()["wire"]["actions"]
        json_values = set(wire.values())
        for member in PubSubAction:
            assert member.value in json_values, (
                f"PubSubAction.{member.name}={member.value!r} "
                f"not in shared/constants/pubsub.json"
            )


class TestPubSubFieldsMatchSharedJSON:
    """PubSubField enum values must match shared/constants/pubsub.json wire.fields."""

    def test_all_json_fields_have_enum_members(self):
        wire = _load_pubsub_json()["wire"]["fields"]
        for key, value in wire.items():
            assert value in PubSubField._value2member_map_, (
                f"shared/constants/pubsub.json wire.fields.{key}={value!r} "
                f"has no PubSubField member"
            )

    def test_no_extra_enum_members_beyond_json(self):
        wire = _load_pubsub_json()["wire"]["fields"]
        json_values = set(wire.values())
        for member in PubSubField:
            assert member.value in json_values, (
                f"PubSubField.{member.name}={member.value!r} "
                f"not in shared/constants/pubsub.json"
            )
