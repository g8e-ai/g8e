import logging
import os
from pathlib import Path

logger = logging.getLogger(__name__)

def resolve_project_root() -> Path:
    """
    Resolves the project root directory.
    This is the canonical root detection heuristic - all languages must match this logic.
    Priority:
    1. G8E_PROJECT_ROOT environment variable
    2. Walk up from current directory looking for marker: services/ directory AND g8e file
    3. If in services/g8eo, walk up 2 levels to root
    4. If in services/g8ee, walk up 2 levels to root
    5. Fallback to current directory

    Note: This function must NOT import from app.constants to avoid circular dependencies.
    """
    env_root = os.environ.get("G8E_PROJECT_ROOT")
    if env_root:
        return Path(env_root).resolve()

    cwd = Path.cwd()

    # Try to find root by looking for the marker: services/ directory AND g8e file
    for parent in [cwd] + list(cwd.parents):
        if (parent / "services").exists() and (parent / "g8e").exists():
            return parent.resolve()

    # If in services/g8eo, walk up 2 levels to root
    if "services/g8eo" in str(cwd):
        for parent in [cwd] + list(cwd.parents):
            if parent.name == "g8eo" and parent.parent.name == "services":
                return parent.parent.parent.resolve()

    # If in services/g8ee, walk up 2 levels to root
    if "services/g8ee" in str(cwd):
        for parent in [cwd] + list(cwd.parents):
            if parent.name == "g8ee" and parent.parent.name == "services":
                return parent.parent.parent.resolve()

    # Fallback to current directory
    return cwd.resolve()

def resolve_config_path(filename: str) -> Path:
    """
    Resolves a config file path using centralized PATHS if available, 
    otherwise falls back to repo-relative resolution.
    """
    from app.constants.paths import PATHS
    
    # Check if PATHS has it
    if "g8ee" in PATHS and "config_dir" in PATHS["g8ee"]:
        target_dir = Path(PATHS["g8ee"]["config_dir"])
        # Handle container absolute paths when running on host
        if not target_dir.exists() and len(target_dir.parts) >= 2 and target_dir.parts[0:2] == ("/", "app"):
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
