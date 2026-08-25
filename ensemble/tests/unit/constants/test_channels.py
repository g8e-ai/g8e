# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for Phase 9 — Channel values sourced from g8e.constants.CHANNELS."""

import pytest

from g8e.constants import channel as _g8e_channel

from app.constants.channels import (
    CHANNEL_SEGMENT_COUNT,
    OperatorChannel,
    PubSubAction,
    PubSubAuthPrefix,
    PubSubChannel,
    PubSubField,
    PubSubMessageType,
    PubSubWireEventType,
)

pytestmark = pytest.mark.unit


class TestSharedChannelsFromG8e:
    """Verify shared channel values match g8e protocol constants."""

    @pytest.mark.parametrize(
        "local,g8e_key",
        [
            (OperatorChannel.GOVERNANCE, "Governance"),
            (OperatorChannel.OPERATOR_INTENT, "OperatorIntent"),
            (OperatorChannel.OPERATOR_DEVICE, "OperatorDevice"),
            (OperatorChannel.SSE_EVENT, "SseEvent"),
            (OperatorChannel.STORAGE_DOCUMENT, "StorageDocument"),
            (OperatorChannel.STORAGE_KV, "StorageKv"),
            (OperatorChannel.STORAGE_BLOB, "StorageBlob"),
            (OperatorChannel.ERROR, "Error"),
            (OperatorChannel.MESSAGE, "Message"),
        ],
    )
    def test_channel_matches_g8e(self, local: str, g8e_key: str):
        assert local == _g8e_channel(g8e_key)


class TestG8eeSpecificChannels:
    """Verify g8ee-specific channels are local strings not in g8e protocol."""

    @pytest.mark.parametrize(
        "member,expected",
        [
            (OperatorChannel.CMD, "cmd"),
            (OperatorChannel.RESULTS, "results"),
            (OperatorChannel.HEARTBEAT, "heartbeat"),
            (OperatorChannel.G8EO_RESULTS, "g8eo_results"),
            (OperatorChannel.OPERATOR_HEARTBEATS, "operator_heartbeats"),
            (OperatorChannel.SSE_EVENTS, "sse_events"),
            (OperatorChannel.SYSTEM_EVENTS, "system_events"),
        ],
    )
    def test_g8ee_specific_channel_value(self, member: OperatorChannel, expected: str):
        assert member.value == expected


class TestChannelHelpers:
    """Verify helper methods produce correct structured channel strings."""

    def test_cmd(self):
        assert OperatorChannel.cmd("op-001", "sess-001") == "cmd:op-001:sess-001"

    def test_results(self):
        assert OperatorChannel.results("op-001", "sess-001") == "results:op-001:sess-001"

    def test_heartbeat(self):
        assert OperatorChannel.heartbeat("op-001", "sess-001") == "heartbeat:op-001:sess-001"

    def test_storage_document(self):
        assert (
            OperatorChannel.storage_document("op-001", "sess-001")
            == "storage_document:op-001:sess-001"
        )

    def test_storage_kv(self):
        assert OperatorChannel.storage_kv("op-001", "sess-001") == "storage_kv:op-001:sess-001"

    def test_storage_blob(self):
        assert OperatorChannel.storage_blob("op-001", "sess-001") == "storage_blob:op-001:sess-001"

    def test_governance(self):
        assert OperatorChannel.governance("op-001", "sess-001") == "governance:op-001:sess-001"

    def test_operator_intent(self):
        assert (
            OperatorChannel.operator_intent("op-001", "sess-001")
            == "operator_intent:op-001:sess-001"
        )

    def test_operator_device(self):
        assert (
            OperatorChannel.operator_device("op-001", "sess-001")
            == "operator_device:op-001:sess-001"
        )

    def test_sse_event(self):
        assert OperatorChannel.sse_event("op-001", "sess-001") == "sse_event:op-001:sess-001"


class TestChannelParse:
    """Verify parse() correctly splits structured channels."""

    def test_parse_valid(self):
        prefix, op_id, sess_id = OperatorChannel.parse("cmd:op-001:sess-001")
        assert prefix == "cmd"
        assert op_id == "op-001"
        assert sess_id == "sess-001"

    def test_parse_invalid_too_few_parts(self):
        with pytest.raises(ValueError, match="Invalid structured channel format"):
            OperatorChannel.parse("cmd:op-001")

    def test_parse_invalid_too_many_parts(self):
        with pytest.raises(ValueError, match="Invalid structured channel format"):
            OperatorChannel.parse("cmd:op-001:sess-001:extra")

    def test_segment_count(self):
        assert CHANNEL_SEGMENT_COUNT == 3


class TestPubSubChannelAlias:
    """Verify PubSubChannel is an alias for OperatorChannel."""

    def test_pubsub_channel_is_operator_channel(self):
        assert PubSubChannel is OperatorChannel


class TestPubSubEnums:
    """Verify g8ee-specific pubsub enums have correct values."""

    @pytest.mark.parametrize(
        "member,expected",
        [
            (PubSubAuthPrefix.AUTH_PUBLISH_PREFIX, "auth.publish:"),
            (PubSubAuthPrefix.AUTH_PUBLISH_SESSION_PREFIX, "auth.publish:session:"),
            (PubSubAuthPrefix.AUTH_RESPONSE_PREFIX, "auth.response:"),
            (PubSubAuthPrefix.AUTH_RESPONSE_SESSION_PREFIX, "auth.response:session:"),
            (PubSubAuthPrefix.AUTH_SESSION_PREFIX, "auth.session:"),
        ],
    )
    def test_auth_prefix_values(self, member: PubSubAuthPrefix, expected: str):
        assert member.value == expected

    @pytest.mark.parametrize(
        "member,expected",
        [
            (PubSubAction.SUBSCRIBE, "subscribe"),
            (PubSubAction.PSUBSCRIBE, "psubscribe"),
            (PubSubAction.UNSUBSCRIBE, "unsubscribe"),
            (PubSubAction.PUBLISH, "publish"),
        ],
    )
    def test_action_values(self, member: PubSubAction, expected: str):
        assert member.value == expected

    @pytest.mark.parametrize(
        "member,expected",
        [
            (PubSubWireEventType.MESSAGE, "message"),
            (PubSubWireEventType.PMESSAGE, "pmessage"),
            (PubSubWireEventType.SUBSCRIBED, "subscribed"),
        ],
    )
    def test_wire_event_type_values(self, member: PubSubWireEventType, expected: str):
        assert member.value == expected

    @pytest.mark.parametrize(
        "member,expected",
        [
            (PubSubField.ACTION, "action"),
            (PubSubField.CHANNEL, "channel"),
            (PubSubField.DATA, "data"),
            (PubSubField.MESSAGE, "message"),
            (PubSubField.PATTERN, "pattern"),
            (PubSubField.TYPE, "type"),
            (PubSubField.SENDER, "sender"),
        ],
    )
    def test_field_values(self, member: PubSubField, expected: str):
        assert member.value == expected

    @pytest.mark.parametrize(
        "member,expected",
        [
            (PubSubMessageType.MESSAGE, "message"),
            (PubSubMessageType.EVENT, "event"),
        ],
    )
    def test_message_type_values(self, member: PubSubMessageType, expected: str):
        assert member.value == expected
