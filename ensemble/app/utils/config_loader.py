# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Centralized JSON configuration loading for validator modules."""

import json
import logging
from pathlib import Path

from app.errors import ConfigurationError

logger = logging.getLogger(__name__)


def load_json_config(path: Path, *, config_name: str) -> dict:
    """Load a JSON configuration file with uniform error handling.

    Args:
        path: Path to the JSON configuration file.
        config_name: Human-readable name for the configuration (used in error messages).

    Returns:
        Parsed JSON data as a dictionary.

    Raises:
        ConfigurationError: If the file does not exist or contains invalid JSON.
    """
    if not path.is_file():
        raise ConfigurationError(f"{config_name} not found at {path}")

    try:
        with path.open("r", encoding="utf-8") as handle:
            data = json.load(handle)
    except json.JSONDecodeError as exc:
        raise ConfigurationError(
            f"Invalid JSON in {config_name} at {path}: {exc}", cause=exc
        ) from exc

    return data
