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

"""Message sender identifiers for conversation history persistence.

These are NOT SSE event types. They identify the source of a message
in the database (user, AI, system, operator terminal) for conversation
history tracking and display.

Canonical source: shared/constants/senders.json
"""

from enum import Enum


class MessageSender(str, Enum):
    """Message sender identifiers for DB persistence.

    These values identify who sent a message in the conversation history.
    They are NOT SSE event types - use EventType for pub/sub events.
    """
    USER_CHAT = "g8e.v1.source.user.chat"
    USER_TERMINAL = "g8e.v1.source.user.terminal"
    AI_PRIMARY = "g8e.v1.source.ai.primary"
    AI_ASSISTANT = "g8e.v1.source.ai.assistant"
    AI_TRIAGE = "g8e.v1.source.ai.triage"
    SYSTEM = "g8e.v1.source.system"
