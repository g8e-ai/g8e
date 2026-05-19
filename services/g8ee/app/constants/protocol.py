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

import json
import logging
from pathlib import Path
from typing import Any, TypeVar

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
)

# Protocol Constants Loader for Python
# Mirrors client's protocol.js to provide a single entry point for protocol constants.

logger = logging.getLogger(__name__)

T = TypeVar("T")

_PROTOCOL_DIR = Path(__file__).parent.parent.parent.parent.parent / "protocol" / "constants"

def _load_protocol_json[T](filename: str, model_cls: type[T] | None = None) -> Any:
    path = _PROTOCOL_DIR / filename
    if not path.exists():
        # Fallback for containerized environments where the path might differ
        path = Path("/app/protocol/constants") / filename

    logger.info("Loading protocol JSON %s from %s", filename, path)
    try:
        with open(path) as f:
            data = json.load(f)
            if model_cls:
                # Use Pydantic to validate and parse the JSON data
                if hasattr(model_cls, "model_validate"):
                    model_inst = model_cls.model_validate(data)
                else:
                    model_inst = model_cls(**data)
                return model_inst.model_dump(by_alias=True) if hasattr(model_inst, "model_dump") else model_inst
            return data
    except Exception as e:
        logger.error("Failed to load/validate protocol JSON %s: %s", filename, e)
        # For protocol constants, we prefer to fail hard if they are invalid
        raise RuntimeError(f"Failed to load protocol constants file {path}: {e}") from e

_EVENTS = _load_protocol_json("events.json", EventsConstants)
_STATUS = _load_protocol_json("status.json", StatusConstants)
_MSG = _load_protocol_json("senders.json", SendersConstants)
_COLLECTIONS = _load_protocol_json("collections.json", CollectionsConstants)
_KV = _load_protocol_json("kv_keys.json", KVKeysConstants)
_CHANNELS = _load_protocol_json("channels.json", ChannelsConstants)
_PUBSUB = _load_protocol_json("pubsub.json", PubSubConstants)
_INTENTS = _load_protocol_json("intents.json", IntentsConstants)
_PROMPTS = _load_protocol_json("prompts.json", PromptsConstants)
_TIMESTAMP = _load_protocol_json("timestamp.json")
_HEADERS = _load_protocol_json("headers.json", HeadersConstants)
_DOCUMENT_IDS = _load_protocol_json("document_ids.json", DocumentIdsConstants)
_PLATFORM = _load_protocol_json("platform.json", PlatformConstants)
_AGENTS = _load_protocol_json("agents.json", AgentsConstants)
