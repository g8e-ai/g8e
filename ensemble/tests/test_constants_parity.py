# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Parity test for protocol constants.

Ensures that JSON files in protocol/constants/ match the pydantic models
in app.constants.models. This test will fail loudly if the exporter
(Worker B in ssot_go_constants.md) produces malformed JSON.

Run after 'make constants' to verify the exporter output is valid.
"""

import json
import os
from pathlib import Path

import pytest

from app.constants.models import (
    AgentsConstants,
    APIPathsConstants,
    ChannelsConstants,
    CollectionsConstants,
    DocumentIdsConstants,
    EventsConstants,
    HeadersConstants,
    IntentsConstants,
    KVKeysConstants,
    PlatformConstants,
    PromptsConstants,
    PubSubConstants,
    SendersConstants,
    StatusConstants,
)


def _resolve_protocol_constants_dir() -> Path:
    """Resolve the protocol/constants/ directory robustly.

    Checks G8E_PROTOCOL_DIR env var first, then falls back to the relative
    path from this test file (ensemble/tests/ → repo root → protocol/constants/).
    """
    env_dir = os.environ.get("G8E_PROTOCOL_DIR")
    if env_dir:
        candidate = Path(env_dir) / "constants"
        if candidate.is_dir():
            return candidate

    candidate = Path(__file__).parent.parent.parent / "protocol" / "constants"
    if not candidate.is_dir():
        raise FileNotFoundError(
            f"Protocol constants directory not found at {candidate}. "
            f"Set G8E_PROTOCOL_DIR to the protocol directory or run tests from the ensemble directory."
        )
    return candidate


_PROTOCOL_CONSTANTS_DIR = _resolve_protocol_constants_dir()


def _load_json_file(filename: str) -> dict:
    """Load a JSON file from the monorepo protocol/constants/ directory."""
    path = _PROTOCOL_CONSTANTS_DIR / filename
    with path.open() as f:
        return json.load(f)


def test_collections_json_matches_model():
    """collections.json must validate against CollectionsConstants model."""
    data = _load_json_file("collections.json")
    model = CollectionsConstants.model_validate(data)
    assert model.collections is not None
    # Spot-check a well-known key
    assert "users" in model.collections
    assert model.collections["users"].value == "users"


def test_events_json_matches_model():
    """events.json must validate against EventsConstants model."""
    data = _load_json_file("events.json")
    EventsConstants.model_validate(data)
    # Spot-check a well-known key (keys are map keys from registry.go)
    assert "OperatorCommandCompleted" in data["events"]


def test_status_json_matches_model():
    """status.json must validate against StatusConstants model."""
    data = _load_json_file("status.json")
    StatusConstants.model_validate(data)
    # Spot-check a well-known key
    assert "available" in data["status"]["operator_status"]
    assert data["status"]["operator_status"]["available"]["value"] == "available"


def test_senders_json_matches_model():
    """senders.json must validate against SendersConstants model."""
    data = _load_json_file("senders.json")
    SendersConstants.model_validate(data)
    # senders.json has a nested structure with senders
    assert "senders" in data
    assert "AiAssistant" in data["senders"]


def test_headers_json_matches_model():
    """headers.json must validate against HeadersConstants model."""
    data = _load_json_file("headers.json")
    HeadersConstants.model_validate(data)
    # Spot-check a well-known key
    assert "Authorization" in data["headers"]
    assert data["headers"]["Authorization"]["value"] == "Authorization"


def test_channels_json_matches_model():
    """channels.json must validate against ChannelsConstants model."""
    data = _load_json_file("channels.json")
    ChannelsConstants.model_validate(data)


def test_pubsub_json_matches_model():
    """pubsub.json must validate against PubSubConstants model."""
    data = _load_json_file("pubsub.json")
    PubSubConstants.model_validate(data)


def test_intents_json_matches_model():
    """intents.json must validate against IntentsConstants model."""
    data = _load_json_file("intents.json")
    IntentsConstants.model_validate(data)


def test_prompts_json_matches_model():
    """prompts.json must validate against PromptsConstants model."""
    data = _load_json_file("prompts.json")
    PromptsConstants.model_validate(data)


def test_platform_json_matches_model():
    """platform.json must validate against PlatformConstants model."""
    data = _load_json_file("platform.json")
    PlatformConstants.model_validate(data)


def test_agents_json_matches_model():
    """agents.json must validate against AgentsConstants model."""
    data = _load_json_file("agents.json")
    AgentsConstants.model_validate(data)


def test_document_ids_json_matches_model():
    """document_ids.json must validate against DocumentIdsConstants model."""
    data = _load_json_file("document_ids.json")
    DocumentIdsConstants.model_validate(data)


def test_kv_keys_json_matches_model():
    """kv_keys.json must validate against KVKeysConstants model."""
    data = _load_json_file("kv_keys.json")
    KVKeysConstants.model_validate(data)


def test_api_paths_json_matches_model():
    """api_paths.json must validate against APIPathsConstants model."""
    data = _load_json_file("api_paths.json")
    model = APIPathsConstants.model_validate(data)
    assert model.g8ee is not None
    assert model.client is not None
