"""Contract tests for the bench-side SSE wire envelope.

Pins `g8e_evals.sut.wire.SSEWireEnvelope` against the canonical shape
produced by the protocol (`g8e_protocol.models.events.SessionEventWire` /
`BackgroundEventWire`) and persisted by the Operator's
`/api/internal/sse/events` endpoint.

If the publisher renames `event`, `event.type`, `event.data`, or moves
`investigation_id` / `content` / `transaction_hash` to a new location,
these tests fail loudly — preventing silent bench drift.
"""

from g8e_evals.sut.g8ee_chat import (
    _extract_investigation_id,
    _extract_text_chunk,
)
from g8e_evals.sut.wire import SSEWireEnvelope


SESSION_WIRE_FIXTURE = {
    "web_session_id": "sess-123",
    "user_id": "user-abc",
    "event": {
        "type": "g8e.v1.ai.llm.chat.iteration.text.chunk.received",
        "data": {
            "content": "hello world",
            "timestamp": "2026-01-15T10:30:00Z",
            "user_id": "user-abc",
            "web_session_id": "sess-123",
            "case_id": "case-xyz",
            "investigation_id": "inv-def",
        },
    },
}


BACKGROUND_WIRE_FIXTURE = {
    "user_id": "user-abc",
    "event": {
        "type": "g8e.v1.ai.governance.warden.receipt.signed",
        "data": {
            "transaction_hash": "tx-abc",
            "investigation_id": "inv-def",
        },
    },
}


def test_session_wire_parses_text_chunk():
    envelope = SSEWireEnvelope.parse(SESSION_WIRE_FIXTURE)
    assert envelope is not None
    assert envelope.web_session_id == "sess-123"
    assert envelope.user_id == "user-abc"
    assert envelope.event is not None
    assert envelope.event.type == "g8e.v1.ai.llm.chat.iteration.text.chunk.received"
    assert envelope.text_chunk() == "hello world"
    assert envelope.investigation_id() == "inv-def"


def test_background_wire_parses_without_web_session_id():
    envelope = SSEWireEnvelope.parse(BACKGROUND_WIRE_FIXTURE)
    assert envelope is not None
    assert envelope.web_session_id is None
    assert envelope.cli_session_id is None
    assert envelope.user_id == "user-abc"
    assert envelope.field_in_data("transaction_hash") == "tx-abc"
    assert envelope.investigation_id() == "inv-def"


def test_parse_returns_none_for_non_dict():
    assert SSEWireEnvelope.parse(None) is None
    assert SSEWireEnvelope.parse("not a dict") is None
    assert SSEWireEnvelope.parse(42) is None


def test_parse_tolerates_missing_event_field():
    envelope = SSEWireEnvelope.parse({"user_id": "user-abc"})
    assert envelope is not None
    assert envelope.event is None
    assert envelope.text_chunk() == ""
    assert envelope.investigation_id() == ""


def test_parse_tolerates_extra_unknown_fields():
    # Forward-compat: new top-level fields added on the publisher must not
    # break parsing on the bench.
    envelope = SSEWireEnvelope.parse({**SESSION_WIRE_FIXTURE, "future_field": 1})
    assert envelope is not None
    assert envelope.text_chunk() == "hello world"


def test_extract_text_chunk_uses_typed_envelope():
    assert _extract_text_chunk(SESSION_WIRE_FIXTURE) == "hello world"
    # Top-level `data` (legacy ad-hoc shape) must NOT be recognized: the
    # canonical wire shape always nests data under `event`.
    assert _extract_text_chunk({"data": {"content": "stray"}}) == ""


def test_extract_investigation_id_only_reads_event_data():
    assert _extract_investigation_id(SESSION_WIRE_FIXTURE) == "inv-def"
    # A bare top-level `investigation_id` (not under event.data) is NOT
    # produced by SessionEventWire and must not be honoured.
    assert _extract_investigation_id({"investigation_id": "stray"}) == ""
    assert _extract_investigation_id("not a dict") == ""
