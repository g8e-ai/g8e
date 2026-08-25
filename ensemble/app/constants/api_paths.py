# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import json
from pathlib import Path
from typing import Any

import g8e.constants as _g8e_constants

from app.constants.models import APIPathsConstants


def _load[T](filename: str, model_cls: type[T]) -> T:
    # Load from local app/constants directory
    path = Path(__file__).parent / filename
    try:
        with open(path) as f:
            data = json.load(f)
            # Use Pydantic to validate and parse the JSON data
            if hasattr(model_cls, "model_validate"):
                return model_cls.model_validate(data)
            return model_cls(**data)
    except FileNotFoundError as e:
        raise RuntimeError(f"API paths file not found: {path}") from e
    except (json.JSONDecodeError, Exception) as e:
        raise RuntimeError(f"Failed to load/validate API paths file {path}: {e}") from e


_API_PATHS_DATA = _load("api_paths.json", APIPathsConstants)
API_PATHS = _API_PATHS_DATA.model_dump()


class _InternalAPIPathsMeta(type):
    def __getattr__(cls, name: str) -> str:
        # Priority 1: Explicit FULL_ prefix
        if name.startswith("FULL_"):
            sub_name = name.removeprefix("FULL_")
            if sub_name.startswith("G8EE_"):
                key = sub_name.removeprefix("G8EE_").lower()
                if key in cls._G8EE_FULL_PATHS:
                    return cls._G8EE_FULL_PATHS[key]
            elif sub_name.startswith("CLIENT_"):
                key = sub_name.removeprefix("CLIENT_").lower()
                if key in cls._CLIENT_FULL_PATHS:
                    return cls._CLIENT_FULL_PATHS[key]

        # Priority 2: G8EE_ prefix (try full then sub)
        if name.startswith("G8EE_"):
            key = name.removeprefix("G8EE_").lower()
            if key in cls._G8EE_FULL_PATHS:
                return cls._G8EE_FULL_PATHS[key]
            if key in cls._G8EE_PATHS:
                return cls.PREFIX + cls._G8EE_PATHS[key]

        # Priority 3: CLIENT_ prefix (try full then sub)
        elif name.startswith("CLIENT_"):
            key = name.removeprefix("CLIENT_").lower()
            if key in cls._CLIENT_FULL_PATHS:
                return cls._CLIENT_FULL_PATHS[key]
            if key in cls._CLIENT_PATHS:
                return cls.PREFIX + cls._CLIENT_PATHS[key]

        raise AttributeError(f"'{cls.__name__}' object has no attribute '{name}'")


class InternalAPIPaths(metaclass=_InternalAPIPathsMeta):
    """Internal API paths shared across g8ee and client."""

    PREFIX: str = _API_PATHS_DATA.internal_prefix

    _G8EE_PATHS: dict[str, str] = _API_PATHS_DATA.g8ee
    _G8EE_FULL_PATHS: dict[str, str] = _API_PATHS_DATA.g8ee_full
    _CLIENT_PATHS: dict[str, str] = _API_PATHS_DATA.client
    _CLIENT_FULL_PATHS: dict[str, str] = _API_PATHS_DATA.client_full


# ---------------------------------------------------------------------------
# GatewayAPIPaths — Gateway-side API paths sourced from g8e.constants.API_PATHS
# ---------------------------------------------------------------------------

_G8E_API_PATHS: dict[str, Any] = _g8e_constants.API_PATHS


class _GatewayAPIPathsMeta(type):
    def __getattr__(cls, name: str) -> str:
        key = name.lower()
        if key in _G8E_API_PATHS:
            value = _G8E_API_PATHS[key]
            if isinstance(value, str):
                return value
        raise AttributeError(f"'{cls.__name__}' object has no attribute '{name}'")


class GatewayAPIPaths(metaclass=_GatewayAPIPathsMeta):
    """Gateway API paths sourced from g8e.constants.API_PATHS.

    Provides typed attribute access to the Gateway's canonical API path
    constants. All values are sourced from the g8e protocol package to
    ensure g8ee stays in sync with the Gateway's routing surface.

    Usage:
        GatewayAPIPaths.GOVERNANCE_ENVELOPES  # -> "/api/v1/governance/envelopes"
        GatewayAPIPaths.OPERATORS_BIND        # -> "/api/v1/operators/bind"
    """


def validate_api_paths_sync() -> None:
    """Validate that all keys in api_paths.json are accessible via InternalAPIPaths."""
    errors = []

    for key in _API_PATHS_DATA.g8ee:
        attr_name = f"G8EE_{key.upper()}"
        try:
            getattr(InternalAPIPaths, attr_name)
        except AttributeError:
            errors.append(f"g8ee key '{key}' not accessible as '{attr_name}'")

    for key in _API_PATHS_DATA.client:
        attr_name = f"CLIENT_{key.upper()}"
        try:
            getattr(InternalAPIPaths, attr_name)
        except AttributeError:
            errors.append(f"client key '{key}' not accessible as '{attr_name}'")

    if errors:
        raise RuntimeError(
            "api_paths.json and InternalAPIPaths are out of sync:\n" + "\n".join(errors)
        )
