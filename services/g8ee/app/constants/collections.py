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
from typing import TypeVar
from app.constants.paths import PATHS
from app.constants.models import CollectionsConstants, DocumentIdsConstants

_PROTOCOL_DIR = PATHS["infra"]["protocol_constants_dir"]

T = TypeVar("T")

def _load(filename: str, model_cls: type[T]) -> T:
    path = _PROTOCOL_DIR + "/" + filename
    try:
        with open(path) as f:
            data = json.load(f)
            # Use Pydantic to validate and parse the JSON data
            if hasattr(model_cls, "model_validate"):
                return model_cls.model_validate(data)
            return model_cls(**data)
    except FileNotFoundError as e:
        raise RuntimeError(f"Protocol constants file not found: {path}") from e
    except (json.JSONDecodeError, Exception) as e:
        raise RuntimeError(f"Failed to load/validate protocol constants file {path}: {e}") from e

_COLLECTIONS_DATA = _load("collections.json", CollectionsConstants)
_DOCUMENT_IDS_DATA = _load("document_ids.json", DocumentIdsConstants)

_c = _COLLECTIONS_DATA.collections
_d = _DOCUMENT_IDS_DATA.document_ids
_s = _DOCUMENT_IDS_DATA.sentinel_id

DB_COLLECTION_SETTINGS: str          = _c["settings"].value # one 'settings' collection for both 'platform_settings' and user-specific settings documents
DB_COLLECTION_USERS: str             = _c["users"].value
DB_COLLECTION_WEB_SESSIONS: str      = _c["web_sessions"].value
DB_COLLECTION_OPERATOR_SESSIONS: str = _c["operator_sessions"].value
DB_COLLECTION_API_KEYS: str          = _c["api_keys"].value
DB_COLLECTION_CLI_SESSIONS: str      = _c["cli_sessions"].value
DB_COLLECTION_ORGANIZATIONS: str     = _c["organizations"].value
DB_COLLECTION_OPERATORS: str         = _c["operators"].value
DB_COLLECTION_OPERATOR_USAGE: str    = _c["operator_usage"].value
DB_COLLECTION_CASES: str             = _c["cases"].value
DB_COLLECTION_INVESTIGATIONS: str    = _c["investigations"].value
DB_COLLECTION_TASKS: str             = _c["tasks"].value
DB_COLLECTION_MEMORIES: str          = _c["memories"].value
DB_COLLECTION_REVOKED_CERTS: str      = _c["revoked_certificates"].value
DB_COLLECTION_TRIBUNAL_COMMANDS: str = _c["tribunal_commands"].value
DB_COLLECTION_AGENT_ACTIVITY_METADATA: str = _c["agent_activity_metadata"].value
DB_COLLECTION_REPUTATION_STATE: str        = _c["reputation_state"].value
DB_COLLECTION_REPUTATION_COMMITMENTS: str  = _c["reputation_commitments"].value
DB_COLLECTION_STAKE_RESOLUTIONS: str       = _c["stake_resolutions"].value

# Document IDs for settings collection
PLATFORM_SETTINGS_DOC: str = _d["platform_settings"].value
USER_SETTINGS_DOC_PREFIX: str = _d["user_settings_prefix"].value

# Sentinel ID values
SENTINEL_ID_UNKNOWN: str = _s["unknown"].value
