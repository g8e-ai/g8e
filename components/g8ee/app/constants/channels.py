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

from enum import Enum
from typing import Final


class PubSubAction(str, Enum):
    PUBLISH = "publish"
    SUBSCRIBE = "subscribe"
    PSUBSCRIBE = "psubscribe"
    UNSUBSCRIBE = "unsubscribe"


class PubSubField(str, Enum):
    ACTION = "action"
    CHANNEL = "channel"
    DATA = "data"
    MESSAGE = "message"
    PATTERN = "pattern"
    TYPE = "type"
    SENDER = "sender"


class PubSubWireEventType(str, Enum):
    MESSAGE = "message"
    PMESSAGE = "pmessage"
    SUBSCRIBED = "subscribed"


PubSubMessageType = PubSubWireEventType


class PubSubChannel(str, Enum):
    """Well-known, non-parameterized pub/sub channel names.

    Canonical values mirror `shared/constants/channels.json` -> `pubsub.prefixes`
    and the four platform-wide broadcast channels. Parameterized per-operator
    channels (cmd/results/heartbeat) are constructed via `OperatorChannel` below.
    """

    G8EO_RESULTS = "g8eo_results"
    OPERATOR_HEARTBEATS = "operator_heartbeats"
    SSE_EVENTS = "sse_events"
    SYSTEM_EVENTS = "system_events"

    CMD_PREFIX = "cmd"
    RESULTS_PREFIX = "results"
    HEARTBEAT_PREFIX = "heartbeat"

    AUTH_PUBLISH_SESSION_PREFIX = "auth.publish:session:"
    AUTH_RESPONSE_SESSION_PREFIX = "auth.response:session:"

    SEPARATOR = ":"

    # Backwards-compatible constructors. Delegate to OperatorChannel — a single
    # source of truth for channel format so it cannot drift from `parse()`.
    @classmethod
    def cmd(cls, operator_id: str, operator_session_id: str) -> str:
        return OperatorChannel.cmd(operator_id, operator_session_id)

    @classmethod
    def results(cls, operator_id: str, operator_session_id: str) -> str:
        return OperatorChannel.results(operator_id, operator_session_id)

    @classmethod
    def heartbeat(cls, operator_id: str, operator_session_id: str) -> str:
        return OperatorChannel.heartbeat(operator_id, operator_session_id)

    @classmethod
    def parse(cls, channel: str) -> tuple[str, str, str]:
        return OperatorChannel.parse(channel)


# Canonical per-operator-session channel format:
#     {prefix}{SEPARATOR}{operator_id}{SEPARATOR}{operator_session_id}
# Mirrors `shared/constants/channels.json -> pubsub.segment_count` exactly.
CHANNEL_SEGMENT_COUNT: Final[int] = 3


class OperatorChannel:
    """Typed constructor/parser for per-operator-session pub/sub channels.

    All callers MUST go through this class; never format channel names by hand.
    The operator_session_id is used verbatim — g8ed generates it as
    `operator_session_<ts>_<uuid>` and that full string is the session id.
    """

    _SEP: Final[str] = PubSubChannel.SEPARATOR.value

    @staticmethod
    def cmd(operator_id: str, operator_session_id: str) -> str:
        return OperatorChannel._build(PubSubChannel.CMD_PREFIX.value, operator_id, operator_session_id)

    @staticmethod
    def results(operator_id: str, operator_session_id: str) -> str:
        return OperatorChannel._build(PubSubChannel.RESULTS_PREFIX.value, operator_id, operator_session_id)

    @staticmethod
    def heartbeat(operator_id: str, operator_session_id: str) -> str:
        return OperatorChannel._build(PubSubChannel.HEARTBEAT_PREFIX.value, operator_id, operator_session_id)

    @staticmethod
    def parse(channel: str) -> tuple[str, str, str]:
        """Split a per-operator channel into (prefix, operator_id, operator_session_id).

        Uses a bounded split so operator_session_id values that themselves
        contain the separator survive intact. Raises ValueError on malformed input.
        """
        parts = channel.split(OperatorChannel._SEP, CHANNEL_SEGMENT_COUNT - 1)
        if len(parts) != CHANNEL_SEGMENT_COUNT or not all(parts):
            raise ValueError(f"invalid per-operator channel: {channel!r}")
        return parts[0], parts[1], parts[2]

    @staticmethod
    def _build(prefix: str, operator_id: str, operator_session_id: str) -> str:
        if not prefix or not operator_id or not operator_session_id:
            raise ValueError(
                "channel segments must all be non-empty "
                f"(prefix={prefix!r}, operator_id={operator_id!r}, operator_session_id={operator_session_id!r})"
            )
        sep = OperatorChannel._SEP
        return f"{prefix}{sep}{operator_id}{sep}{operator_session_id}"
