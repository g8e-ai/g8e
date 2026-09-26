# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed lookup over the canonical event registry."""

from __future__ import annotations

from functools import lru_cache

from g8e.constants import EVENTS


class UnknownEventError(ValueError):
    """Raised when an event is not present in the registry."""


class EventNotGovernedError(ValueError):
    """Raised when an event is not a governed request."""


@lru_cache(maxsize=1)
def _events_by_value() -> dict[str, dict]:
    return {
        entry["value"]: entry
        for entry in EVENTS["events"].values()
        if entry.get("value")
    }


def _entry(event_type: str) -> dict:
    entry = _events_by_value().get(event_type)
    if entry is None:
        raise UnknownEventError(f"unknown event type: {event_type}")
    return entry


def action_for(event_type: str) -> str:
    """Return the governed action class for a request event."""
    entry = _entry(event_type)
    if entry.get("kind") != "request":
        raise EventNotGovernedError(f"event is not a governed request: {event_type}")
    transport = entry.get("transport") or []
    if "governed" not in transport:
        raise EventNotGovernedError(f"event is not on the governed transport: {event_type}")
    governance = entry.get("governance") or {}
    action_type = governance.get("action_type")
    if not action_type:
        raise EventNotGovernedError(f"event has no governance action: {event_type}")
    return action_type


def meta(event_type: str) -> dict:
    """Return registry metadata for an event."""
    return _entry(event_type)


def validate_result_envelope(request_event: str, outcome_event: str, action_type: str) -> None:
    """Ensure a correlated result envelope matches the originating request."""
    expected_action = action_for(request_event)
    if action_type != expected_action:
        raise EventNotGovernedError(
            f"request {request_event!r} expects action {expected_action!r}, got {action_type!r}"
        )
    outcome = _entry(outcome_event)
    if outcome.get("kind") not in {"outcome", "fact", "stream"}:
        raise EventNotGovernedError(f"outcome event has invalid kind: {outcome_event}")
    request = _entry(request_event)
    outcomes = request.get("outcomes") or []
    if not outcomes:
        return
    outcome_keys = {entry["value"]: key for key, entry in EVENTS["events"].items() if entry.get("value")}
    allowed = set(outcomes)
    outcome_key = outcome_keys.get(outcome_event)
    if outcome_key not in allowed:
        raise EventNotGovernedError(
            f"outcome {outcome_event!r} not listed for request {request_event!r}"
        )
