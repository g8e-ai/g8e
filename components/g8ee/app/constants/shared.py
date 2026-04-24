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

# Shared Constants Loader for Python
# Mirrors g8ed's shared.js to provide a single entry point for shared constants.

logger = logging.getLogger(__name__)

_SHARED_DIR = Path(__file__).parent.parent.parent.parent.parent / "shared" / "constants"

def _load_shared_json(filename: str) -> dict[str, Any]:
    path = _SHARED_DIR / filename
    if not path.exists():
        # Fallback for containerized environments where the path might differ
        path = Path("/app/shared/constants") / filename
        
    logger.info("Loading shared JSON %s from %s", filename, path)
    with open(path, "r") as f:
        return json.load(f)

_EVENTS = _load_shared_json("events.json")
_STATUS = _load_shared_json("status.json")
_MSG = _load_shared_json("senders.json")
_COLLECTIONS = _load_shared_json("collections.json")
_KV = _load_shared_json("kv_keys.json")
_CHANNELS = _load_shared_json("channels.json")
_PUBSUB = _load_shared_json("pubsub.json")
_INTENTS = _load_shared_json("intents.json")
_PROMPTS = _load_shared_json("prompts.json")
_TIMESTAMP = _load_shared_json("timestamp.json")
_HEADERS = _load_shared_json("headers.json")
_DOCUMENT_IDS = _load_shared_json("document_ids.json")
_PLATFORM = _load_shared_json("platform.json")
_AGENTS = _load_shared_json("agents.json")
