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
    G8EO_RESULTS = "g8eo_results"
    OPERATOR_HEARTBEATS = "operator_heartbeats"
    SSE_EVENTS = "sse_events"
    SYSTEM_EVENTS = "system_events"

    CMD_PREFIX = "cmd"
    RESULTS_PREFIX = "results"
    HEARTBEAT_PREFIX = "heartbeat"
    SEPARATOR = ":"
    SEGMENT_COUNT = 3

    @classmethod
    def parse(cls, channel: str) -> tuple[str, str, str]:
        """Parse a structured channel string into (prefix, operator_id, session_id)."""
        parts = channel.split(cls.SEPARATOR)
        if len(parts) == cls.SEGMENT_COUNT:
            return parts[0], parts[1], parts[2]
        return "", "", ""

    @classmethod
    def cmd(cls, operator_id: str, session_id: str) -> str:
        return f"cmd:{operator_id}:{session_id}"

    @classmethod
    def results(cls, operator_id: str, session_id: str) -> str:
        return f"results:{operator_id}:{session_id}"

    @classmethod
    def heartbeat(cls, operator_id: str, session_id: str) -> str:
        return f"heartbeat:{operator_id}:{session_id}"
