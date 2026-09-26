# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""W11: ensemble SSE producer sites must use registry-backed payload pairs."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from app.constants import EventType
from app.models.events import SSE_PAYLOADS
pytestmark = pytest.mark.unit

_REPO_ROOT = Path(__file__).resolve().parents[4]


def _load_repo_events() -> dict:
    return json.loads((_REPO_ROOT / "protocol/constants/events.json").read_text())["events"]


def _ensemble_sse_registry_events() -> set[str]:
    events: set[str] = set()
    for entry in _load_repo_events().values():
        transport = entry.get("transport") or []
        producers = entry.get("producers") or []
        if "sse" in transport and "ensemble" in producers:
            value = entry.get("value")
            if value:
                events.add(value)
    return events


def test_sse_payloads_are_registered_for_ensemble_sse() -> None:
    registry_events = _ensemble_sse_registry_events()
    extra = {event.value for event in SSE_PAYLOADS} - registry_events
    assert not extra, f"SSE_PAYLOADS has events outside ensemble+sse registry: {sorted(extra)}"


def test_sse_payload_values_match_registry_json() -> None:
    events_json = _load_repo_events()
    by_value = {entry["value"]: key for key, entry in events_json.items() if entry.get("value")}
    for event_type, payload_cls in SSE_PAYLOADS.items():
        key = by_value.get(event_type.value)
        assert key is not None, f"{event_type} missing from events.json"
        entry = events_json[key]
        assert "sse" in (entry.get("transport") or [])
        assert "ensemble" in (entry.get("producers") or [])


def test_event_type_enum_covers_sse_payload_keys() -> None:
    for event_type in SSE_PAYLOADS:
        assert isinstance(event_type, EventType)
