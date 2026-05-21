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

"""
Parity test for protocol constants.

Ensures that JSON files in protocol/constants/ match the pydantic models
in app.constants.models. This test will fail loudly if the exporter
(Worker B in ssot_go_constants.md) produces malformed JSON.

Run after 'make constants' to verify the exporter output is valid.
"""

import json
from pathlib import Path

import pytest

from app.constants.models import (
    AgentsConstants,
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
    PathsConstants,
)


def _load_json_file(filename: str) -> dict:
    """Load a JSON file from protocol/constants/."""
    protocol_dir = Path(__file__).parent.parent.parent.parent / "protocol" / "constants"
    path = protocol_dir / filename
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
    model = EventsConstants.model_validate(data)
    # Spot-check a well-known key (keys are map keys from registry.go)
    assert "OperatorCommandCompleted" in data["events"]


def test_status_json_matches_model():
    """status.json must validate against StatusConstants model."""
    data = _load_json_file("status.json")
    model = StatusConstants.model_validate(data)
    # Spot-check a well-known key
    assert "available" in data["status"]["operator_status"]
    assert data["status"]["operator_status"]["available"]["value"] == "available"


def test_senders_json_matches_model():
    """senders.json must validate against SendersConstants model."""
    data = _load_json_file("senders.json")
    model = SendersConstants.model_validate(data)
    # senders.json has a nested structure with senders
    assert "senders" in data
    assert "AiAssistant" in data["senders"]


def test_headers_json_matches_model():
    """headers.json must validate against HeadersConstants model."""
    data = _load_json_file("headers.json")
    model = HeadersConstants.model_validate(data)
    # Spot-check a well-known key
    assert "APIKey" in data["headers"]
    assert data["headers"]["APIKey"]["value"] == "X-API-Key"


def test_channels_json_matches_model():
    """channels.json must validate against ChannelsConstants model."""
    data = _load_json_file("channels.json")
    model = ChannelsConstants.model_validate(data)


def test_pubsub_json_matches_model():
    """pubsub.json must validate against PubSubConstants model."""
    data = _load_json_file("pubsub.json")
    model = PubSubConstants.model_validate(data)


def test_intents_json_matches_model():
    """intents.json must validate against IntentsConstants model."""
    data = _load_json_file("intents.json")
    model = IntentsConstants.model_validate(data)


def test_prompts_json_matches_model():
    """prompts.json must validate against PromptsConstants model."""
    data = _load_json_file("prompts.json")
    model = PromptsConstants.model_validate(data)


def test_platform_json_matches_model():
    """platform.json must validate against PlatformConstants model."""
    data = _load_json_file("platform.json")
    model = PlatformConstants.model_validate(data)


def test_agents_json_matches_model():
    """agents.json must validate against AgentsConstants model."""
    data = _load_json_file("agents.json")
    model = AgentsConstants.model_validate(data)


def test_document_ids_json_matches_model():
    """document_ids.json must validate against DocumentIdsConstants model."""
    data = _load_json_file("document_ids.json")
    model = DocumentIdsConstants.model_validate(data)


def test_kv_keys_json_matches_model():
    """kv_keys.json must validate against KVKeysConstants model."""
    data = _load_json_file("kv_keys.json")
    model = KVKeysConstants.model_validate(data)


def test_paths_json_matches_model():
    """paths.json must validate against PathsConstants model."""
    data = _load_json_file("paths.json")
    model = PathsConstants.model_validate(data)
    assert model.infra is not None
    assert model.g8ee is not None
