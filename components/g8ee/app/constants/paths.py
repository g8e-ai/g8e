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

# The bridge to shared paths. 
# In container, this is always /app/shared/constants/paths.json
_CONTAINER_SHARED_CONSTANTS_DIR = "/app/shared/constants"
_PATH_FILE = _CONTAINER_SHARED_CONSTANTS_DIR + "/paths.json"

def _load_paths() -> dict:
    try:
        with open(_PATH_FILE, "r") as f:
            return json.load(f)
    except Exception:
        # Emergency fallbacks for when shared volume isn't ready
        return {
            "infra": {
                "db_path": "/data/g8e.db",
                "ca_cert_path": "/vsodb/ca.crt",
                "ssl_dir": "/vsodb",
                "docs_dir": "/docs",
                "shared_constants_dir": "/app/shared/constants"
            }
        }

PATHS = _load_paths()
