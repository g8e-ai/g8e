# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Operator channel and pubsub enums.

Shared channel values are sourced from g8e.constants.CHANNELS to stay in
sync with the protocol. g8ee-specific channels (CMD, RESULTS, HEARTBEAT,
G8EO_RESULTS, OPERATOR_HEARTBEATS, SSE_EVENTS, SYSTEM_EVENTS) are defined
locally as application-level constructs not in the protocol.
"""

from enum import StrEnum

from g8e.constants import channel as _g8e_channel

CHANNEL_SEGMENT_COUNT = 3


class OperatorChannel(StrEnum):
    # Channels sourced from g8e protocol constants
    GOVERNANCE = _g8e_channel("Governance")
    OPERATOR_INTENT = _g8e_channel("OperatorIntent")
    OPERATOR_DEVICE = _g8e_channel("OperatorDevice")
    SSE_EVENT = _g8e_channel("SseEvent")
    STORAGE_DOCUMENT = _g8e_channel("StorageDocument")
    STORAGE_KV = _g8e_channel("StorageKv")
    STORAGE_BLOB = _g8e_channel("StorageBlob")
    ERROR = _g8e_channel("Error")
    MESSAGE = _g8e_channel("Message")

    # g8ee-specific channels not in the g8e protocol
    CMD = "cmd"
    RESULTS = "results"
    HEARTBEAT = "heartbeat"
    G8EO_RESULTS = "g8eo_results"
    OPERATOR_HEARTBEATS = "operator_heartbeats"
    SSE_EVENTS = "sse_events"
    SYSTEM_EVENTS = "system_events"

    @classmethod
    def cmd(cls, operator_id: str, operator_session_id: str) -> str:
        return f"{cls.CMD}:{operator_id}:{operator_session_id}"

    @classmethod
    def results(cls, operator_id: str, operator_session_id: str) -> str:
        return f"{cls.RESULTS}:{operator_id}:{operator_session_id}"

    @classmethod
    def heartbeat(cls, operator_id: str, operator_session_id: str) -> str:
        return f"{cls.HEARTBEAT}:{operator_id}:{operator_session_id}"

    @classmethod
    def storage_document(cls, operator_id: str, operator_session_id: str) -> str:
        return f"{cls.STORAGE_DOCUMENT}:{operator_id}:{operator_session_id}"

    @classmethod
    def storage_kv(cls, operator_id: str, operator_session_id: str) -> str:
        return f"{cls.STORAGE_KV}:{operator_id}:{operator_session_id}"

    @classmethod
    def storage_blob(cls, operator_id: str, operator_session_id: str) -> str:
        return f"{cls.STORAGE_BLOB}:{operator_id}:{operator_session_id}"

    @classmethod
    def governance(cls, operator_id: str, operator_session_id: str) -> str:
        return f"{cls.GOVERNANCE}:{operator_id}:{operator_session_id}"

    @classmethod
    def operator_intent(cls, operator_id: str, operator_session_id: str) -> str:
        return f"{cls.OPERATOR_INTENT}:{operator_id}:{operator_session_id}"

    @classmethod
    def operator_device(cls, operator_id: str, operator_session_id: str) -> str:
        return f"{cls.OPERATOR_DEVICE}:{operator_id}:{operator_session_id}"

    @classmethod
    def sse_event(cls, operator_id: str, operator_session_id: str) -> str:
        return f"{cls.SSE_EVENT}:{operator_id}:{operator_session_id}"

    @classmethod
    def parse(cls, channel: str) -> tuple[str, str, str]:
        """Parse a structured channel into (prefix, operator_id, operator_session_id)."""
        parts = channel.split(":")
        if len(parts) != 3:
            raise ValueError(f"Invalid structured channel format: {channel}")
        return parts[0], parts[1], parts[2]


PubSubChannel = OperatorChannel


class PubSubAuthPrefix(StrEnum):
    AUTH_PUBLISH_PREFIX = "auth.publish:"
    AUTH_PUBLISH_SESSION_PREFIX = "auth.publish:session:"
    AUTH_RESPONSE_PREFIX = "auth.response:"
    AUTH_RESPONSE_SESSION_PREFIX = "auth.response:session:"
    AUTH_SESSION_PREFIX = "auth.session:"


class PubSubAction(StrEnum):
    SUBSCRIBE = "subscribe"
    PSUBSCRIBE = "psubscribe"
    UNSUBSCRIBE = "unsubscribe"
    PUBLISH = "publish"


class PubSubWireEventType(StrEnum):
    MESSAGE = "message"
    PMESSAGE = "pmessage"
    SUBSCRIBED = "subscribed"


class PubSubField(StrEnum):
    ACTION = "action"
    CHANNEL = "channel"
    DATA = "data"
    MESSAGE = "message"
    PATTERN = "pattern"
    TYPE = "type"
    SENDER = "sender"


class PubSubMessageType(StrEnum):
    MESSAGE = "message"
    EVENT = "event"
