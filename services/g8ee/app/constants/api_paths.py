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
from typing import TypeVar, Type
from app.constants.paths import PATHS
from app.constants.models import ApiPathsConstants

_PROTOCOL_DIR = PATHS["infra"]["protocol_constants_dir"]

T = TypeVar("T")

def _load(filename: str, model_cls: Type[T]) -> T:
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

_API_PATHS_DATA = _load("api_paths.json", ApiPathsConstants)
API_PATHS = _API_PATHS_DATA.model_dump()

class _InternalApiPathsMeta(type):
    def __getattr__(cls, name: str) -> str:
        if name.startswith("G8EE_"):
            key = name.removeprefix("G8EE_").lower()
            if key in cls._G8EE_PATHS:
                return cls.PREFIX + cls._G8EE_PATHS[key]
        elif name.startswith("CLIENT_"):
            key = name.removeprefix("CLIENT_").lower()
            if key in cls._CLIENT_PATHS:
                return cls.PREFIX + cls._CLIENT_PATHS[key]
        raise AttributeError(f"'{cls.__name__}' object has no attribute '{name}'")

class InternalApiPaths(metaclass=_InternalApiPathsMeta):
    """Internal API paths shared across g8ee and client."""
    PREFIX: str = _API_PATHS_DATA.internal_prefix

    _G8EE_PATHS: dict[str, str] = _API_PATHS_DATA.g8ee
    _CLIENT_PATHS: dict[str, str] = _API_PATHS_DATA.client


def validate_api_paths_sync() -> None:
    """Validate that all keys in api_paths.json are accessible via InternalApiPaths."""
    errors = []

    for key in _API_PATHS_DATA.g8ee:
        attr_name = f"G8EE_{key.upper()}"
        try:
            getattr(InternalApiPaths, attr_name)
        except AttributeError:
            errors.append(f"g8ee key '{key}' not accessible as '{attr_name}'")

    for key in _API_PATHS_DATA.client:
        attr_name = f"CLIENT_{key.upper()}"
        try:
            getattr(InternalApiPaths, attr_name)
        except AttributeError:
            errors.append(f"client key '{key}' not accessible as '{attr_name}'")

    if errors:
        raise RuntimeError(
            "api_paths.json and InternalApiPaths are out of sync:\n" + "\n".join(errors)
        )
