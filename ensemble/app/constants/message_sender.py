# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Message sender identifiers for conversation history persistence.

These are NOT SSE event types. They identify the source of a message
in the database (user, AI, system, operator terminal) for conversation
history tracking and display.
"""

from enum import StrEnum


class MessageSender(StrEnum):
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
