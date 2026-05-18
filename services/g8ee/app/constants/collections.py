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
from app.constants.paths import PATHS

_PROTOCOL_DIR = PATHS["infra"]["protocol_constants_dir"]

def _load(filename: str) -> dict[str, object]:
    path = _PROTOCOL_DIR + "/" + filename
    try:
        with open(path) as f:
            return json.load(f)
    except FileNotFoundError as e:
        raise RuntimeError(f"Protocol constants file not found: {path}") from e
    except json.JSONDecodeError as e:
        raise RuntimeError(f"Invalid JSON in protocol constants file {path}: {e}") from e

_COLLECTIONS: dict[str, object] = _load("collections.json")
_DOCUMENT_IDS: dict[str, object] = _load("document_ids.json")

_c: dict[str, dict] = _COLLECTIONS["collections"]
_d: dict[str, dict] = _DOCUMENT_IDS["document_ids"]
_s: dict[str, dict] = _DOCUMENT_IDS["sentinel_id"]

DB_COLLECTION_SETTINGS          = _c["settings"]["value"] # one 'settings' collection for both 'platform_settings' and user-specific settings documents
DB_COLLECTION_USERS             = _c["users"]["value"]
DB_COLLECTION_WEB_SESSIONS      = _c["web_sessions"]["value"]
DB_COLLECTION_OPERATOR_SESSIONS = _c["operator_sessions"]["value"]
DB_COLLECTION_API_KEYS          = _c["api_keys"]["value"]
DB_COLLECTION_CLI_SESSIONS      = _c["cli_sessions"]["value"]
DB_COLLECTION_ORGANIZATIONS     = _c["organizations"]["value"]
DB_COLLECTION_OPERATORS         = _c["operators"]["value"]
DB_COLLECTION_OPERATOR_USAGE    = _c["operator_usage"]["value"]
DB_COLLECTION_CASES             = _c["cases"]["value"]
DB_COLLECTION_INVESTIGATIONS    = _c["investigations"]["value"]
DB_COLLECTION_TASKS             = _c["tasks"]["value"]
DB_COLLECTION_MEMORIES          = _c["memories"]["value"]
DB_COLLECTION_REVOKED_CERTS      = _c["revoked_certificates"]["value"] if "revoked_certificates" in _c else "revoked_certificates"
DB_COLLECTION_TRIBUNAL_COMMANDS = _c["tribunal_commands"]["value"]
DB_COLLECTION_AGENT_ACTIVITY_METADATA = _c["agent_activity_metadata"]["value"]
DB_COLLECTION_REPUTATION_STATE        = _c["reputation_state"]["value"]
DB_COLLECTION_REPUTATION_COMMITMENTS  = _c["reputation_commitments"]["value"]
DB_COLLECTION_STAKE_RESOLUTIONS       = _c["stake_resolutions"]["value"]

# Document IDs for settings collection
PLATFORM_SETTINGS_DOC = _d["platform_settings"]["value"]
USER_SETTINGS_DOC_PREFIX = _d["user_settings_prefix"]["value"]

# Sentinel ID values
SENTINEL_ID_UNKNOWN = _s["unknown"]["value"]
