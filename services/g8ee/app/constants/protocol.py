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
from typing import Any

# Protocol Constants Loader for Python
# Mirrors client's protocol.js to provide a single entry point for protocol constants.

logger = logging.getLogger(__name__)

_PROTOCOL_DIR = Path(__file__).parent.parent.parent.parent.parent / "protocol" / "constants"

def _load_protocol_json(filename: str) -> dict[str, Any]:
    path = _PROTOCOL_DIR / filename
    if not path.exists():
        # Fallback for containerized environments where the path might differ
        path = Path("/app/protocol/constants") / filename

    logger.info("Loading protocol JSON %s from %s", filename, path)
    with open(path) as f:
        return json.load(f)

_EVENTS = _load_protocol_json("events.json")
_STATUS = _load_protocol_json("status.json")
_MSG = _load_protocol_json("senders.json")
_COLLECTIONS = _load_protocol_json("collections.json")
_KV = _load_protocol_json("kv_keys.json")
_CHANNELS = _load_protocol_json("channels.json")
_PUBSUB = _load_protocol_json("pubsub.json")
_INTENTS = _load_protocol_json("intents.json")
_PROMPTS = _load_protocol_json("prompts.json")
_TIMESTAMP = _load_protocol_json("timestamp.json")
_HEADERS = _load_protocol_json("headers.json")
_DOCUMENT_IDS = _load_protocol_json("document_ids.json")
_PLATFORM = _load_protocol_json("platform.json")
_AGENTS = _load_protocol_json("agents.json")
