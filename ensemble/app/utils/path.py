# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging
from pathlib import Path

logger = logging.getLogger(__name__)


def resolve_project_root() -> Path:
    """
    Resolves the project root directory.
    """
    return Path(__file__).parent.parent.parent


def resolve_config_path(filename: str) -> Path:
    """
    Resolves a config file path using centralized PATHS if available,
    otherwise falls back to repo-relative resolution.
    """
    from app.constants.paths import PATHS

    config_dir = PATHS.get("g8ee", {}).get("config_dir")
    if config_dir:
        target_dir = Path(config_dir)
        # Handle container absolute paths when running on host
        if (
            not target_dir.exists()
            and len(target_dir.parts) >= 2
            and target_dir.parts[0:2] == ("/", "app")
        ):
            try:
                root = resolve_project_root()
                # Remove /app/ and join with root
                target_dir = root / Path(*target_dir.parts[2:])
            except (OSError, IndexError) as e:
                logger.warning("Failed to remap container path to host: %s", e)

        target = target_dir / filename
        if target.exists():
            return target

    # Fallback to local config dir
    return Path(__file__).parent.parent.parent / "config" / filename
