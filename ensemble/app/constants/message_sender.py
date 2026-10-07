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

Values are sourced from the protocol SSOT (``protocol/constants/senders.json``
via the ``g8e`` package) — do not hand-roll duplicates here; see
``generated_status.py``.
"""

from enum import StrEnum

from g8e.constants import MSG as _PROTOCOL_SENDERS


def _sender(name: str) -> str:
    """Return the wire value for a sender from the protocol SSOT."""
    return _PROTOCOL_SENDERS["senders"][name]["value"]


class MessageSender(StrEnum):
    """Message sender identifiers for DB persistence.

    These values identify who sent a message in the conversation history.
    They are NOT SSE event types - use EventType for pub/sub events.
    """

    USER_CHAT = _sender("UserChat")
    USER_TERMINAL = _sender("UserTerminal")
    AI_PRIMARY = _sender("AiPrimary")
    AI_ASSISTANT = _sender("AiAssistant")
    AI_TRIAGE = _sender("AiTriage")
    SYSTEM = _sender("System")
