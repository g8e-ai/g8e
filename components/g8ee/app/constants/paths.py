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
import os

# The bridge to shared paths.
# In container, this is always /app/shared/constants/paths.json
# On host, respect G8E_SHARED_DIR environment variable
_SHARED_DIR = os.environ.get("G8E_SHARED_DIR", "/app/shared")
_CONTAINER_SHARED_CONSTANTS_DIR = _SHARED_DIR + "/constants"
_PATH_FILE = _CONTAINER_SHARED_CONSTANTS_DIR + "/paths.json"

def _load_paths() -> dict:
    try:
        with open(_PATH_FILE) as f:
            paths = json.load(f)
    except FileNotFoundError:
        # Emergency fallbacks for when shared volume isn't ready
        paths = {
            "infra": {
                "db_path": "/data/g8e.db",
                "ca_cert_path": "/g8es/ca.crt",
                "ssl_dir": "/g8es",
                "docs_dir": "/docs",
                "shared_dir": _SHARED_DIR,
                "shared_constants_dir": _SHARED_DIR + "/constants",
                "shared_models_dir": _SHARED_DIR + "/models",
                "ssh_config_path": "/etc/g8e/ssh_config",
            }
        }
    except Exception as e:
        raise RuntimeError(f"Failed to load paths from {_PATH_FILE}: {e}") from e

    # Override container paths with G8E_SHARED_DIR when running on host
    # This allows evals and other host-based commands to use host paths
    if "infra" in paths and _SHARED_DIR != "/app/shared":
        paths["infra"]["shared_dir"] = _SHARED_DIR
        paths["infra"]["shared_constants_dir"] = _SHARED_DIR + "/constants"
        paths["infra"]["shared_models_dir"] = _SHARED_DIR + "/models"

    return paths

PATHS = _load_paths()
