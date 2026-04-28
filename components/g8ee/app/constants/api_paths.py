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

_SHARED_DIR = PATHS["infra"]["shared_constants_dir"]

def _load(filename: str) -> dict:
    path = _SHARED_DIR + "/" + filename
    try:
        with open(path) as f:
            return json.load(f)
    except FileNotFoundError as e:
        raise RuntimeError(f"Shared constants file not found: {path}") from e
    except json.JSONDecodeError as e:
        raise RuntimeError(f"Invalid JSON in shared constants file {path}: {e}") from e

API_PATHS = _load("api_paths.json")

class _InternalApiPathsMeta(type):
    def __getattr__(cls, name: str) -> str:
        if name.startswith("G8EE_"):
            key = name[5:].lower()
            if key in cls._G8EE_PATHS:
                return cls.PREFIX + cls._G8EE_PATHS[key]
        elif name.startswith("G8ED_"):
            key = name[5:].lower()
            if key in cls._G8ED_PATHS:
                return cls.PREFIX + cls._G8ED_PATHS[key]
        raise AttributeError(f"'{cls.__name__}' object has no attribute '{name}'")

class InternalApiPaths(metaclass=_InternalApiPathsMeta):
    """Internal API paths shared across g8ee and g8ed."""
    PREFIX: str = API_PATHS["internal_prefix"]

    _G8EE_PATHS: dict = API_PATHS["g8ee"]
    _G8ED_PATHS: dict = API_PATHS["g8ed"]


def validate_api_paths_sync() -> None:
    """Validate that all keys in api_paths.json are accessible via InternalApiPaths."""
    errors = []
    
    for key in API_PATHS["g8ee"]:
        attr_name = f"G8EE_{key.upper()}"
        try:
            getattr(InternalApiPaths, attr_name)
        except AttributeError:
            errors.append(f"g8ee key '{key}' not accessible as '{attr_name}'")
    
    for key in API_PATHS["g8ed"]:
        attr_name = f"G8ED_{key.upper()}"
        try:
            getattr(InternalApiPaths, attr_name)
        except AttributeError:
            errors.append(f"g8ed key '{key}' not accessible as '{attr_name}'")
    
    if errors:
        raise RuntimeError(
            "api_paths.json and InternalApiPaths are out of sync:\n" + "\n".join(errors)
        )
