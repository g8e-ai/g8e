"""Typed SSE wire envelopes consumed by the bench.

These models mirror the canonical wire shape produced by the g8e Engine
(`g8e_protocol.models.events.SessionEventWire` / `BackgroundEventWire`).
The Operator's `/api/internal/sse/events` endpoint returns one of these
envelopes per row in the `payload` field (see
`services/g8eo/internal/services/listen/listen_http.go:internalSSEPushPayload`).

The bench must NOT do ad-hoc dict-spelunking against this shape: any schema
drift in the protocol silently breaks the bench. Parsing through these typed models
makes drift visible — fields that disappear or change type fail validation
loudly, and the contract test in `evals/tests/test_sse_wire.py` pins the
shape against the protocol definition.
"""

from __future__ import annotations

from typing import Any

from pydantic import BaseModel, ConfigDict, Field, ValidationError


class SSEEventBody(BaseModel):
    """Inner `event` object: `{type, data}`."""

    model_config = ConfigDict(extra="ignore")

    type: str
    data: dict[str, Any] = Field(default_factory=dict)


class SSEWireEnvelope(BaseModel):
    """Outer SSE envelope as persisted by the Operator and returned by
    `GET /api/internal/sse/events`.

    Covers both `SessionEventWire` (has `web_session_id` / `cli_session_id`)
    and `BackgroundEventWire` (only `user_id`). All routing fields are
    optional here because the bench consumes the union without needing to
    discriminate.
    """

    model_config = ConfigDict(extra="ignore")

    web_session_id: str | None = None
    cli_session_id: str | None = None
    user_id: str | None = None
    event: SSEEventBody | None = None

    @classmethod
    def parse(cls, payload: Any) -> "SSEWireEnvelope | None":
        """Tolerantly parse a payload dict. Returns None if the shape is
        unrecognizable so the caller can skip silently rather than crashing
        the drain loop on a single malformed row."""
        if not isinstance(payload, dict):
            return None
        try:
            return cls.model_validate(payload)
        except ValidationError:
            return None

    def text_chunk(self) -> str:
        """Return the streaming text chunk content, or "" if absent.

        Source of truth: `ChatResponseChunkPayload.content` in
        `g8e_protocol.models.events`.
        """
        if self.event is None:
            return ""
        content = self.event.data.get("content")
        return content if isinstance(content, str) else ""

    def investigation_id(self) -> str:
        """Return the investigation correlation id, or "" if absent.

        `SessionEventWire.from_session_event` always copies
        `investigation_id` into `event.data` when set on the source
        `SessionEvent`. That is the only canonical location.
        """
        if self.event is None:
            return ""
        inv = self.event.data.get("investigation_id")
        return inv if isinstance(inv, str) else ""

    def field_in_data(self, name: str) -> Any:
        """Generic typed accessor for a key inside `event.data`."""
        if self.event is None:
            return None
        return self.event.data.get(name)
