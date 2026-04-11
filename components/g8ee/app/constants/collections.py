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
from typing import Any
from app.constants.paths import PATHS

_SHARED_DIR = PATHS["infra"]["shared_constants_dir"]

def _load(filename: str) -> dict[str, object]:
    path = _SHARED_DIR + "/" + filename
    try:
        with open(path) as f:
            return json.load(f)
    except FileNotFoundError as e:
        raise RuntimeError(f"Shared constants file not found: {path}") from e
    except json.JSONDecodeError as e:
        raise RuntimeError(f"Invalid JSON in shared constants file {path}: {e}") from e

_COLLECTIONS: dict[str, object] = _load("collections.json")
_DOCUMENT_IDS: dict[str, object] = _load("document_ids.json")

_c: dict[str, str] = _COLLECTIONS["collections"]
_d: dict[str, str] = _DOCUMENT_IDS["document_ids"]

DB_COLLECTION_SETTINGS          = _c["settings"] # one 'settings' collection for both 'platform_settings' and user-specific settings documents
DB_COLLECTION_USERS             = _c["users"]
DB_COLLECTION_WEB_SESSIONS      = _c["web_sessions"]
DB_COLLECTION_OPERATOR_SESSIONS = _c["operator_sessions"]
DB_COLLECTION_API_KEYS          = _c["api_keys"]
DB_COLLECTION_ORGANIZATIONS     = _c["organizations"]
DB_COLLECTION_OPERATORS         = _c["operators"]
DB_COLLECTION_OPERATOR_USAGE    = _c["operator_usage"]
DB_COLLECTION_CASES             = _c["cases"]
DB_COLLECTION_INVESTIGATIONS    = _c["investigations"]
DB_COLLECTION_TASKS             = _c["tasks"]
DB_COLLECTION_MEMORIES          = _c["memories"]

# Document IDs for settings collection
PLATFORM_SETTINGS_DOC = _d["platform_settings"]
USER_SETTINGS_DOC_PREFIX = _d["user_settings_prefix"]
