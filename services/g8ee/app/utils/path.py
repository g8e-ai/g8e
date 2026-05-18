import logging
import os
from pathlib import Path

from app.constants.env_vars import EnvVar

logger = logging.getLogger(__name__)

def resolve_project_root() -> Path:
    """
    Resolves the project root directory.
    Priority:
    1. G8E_PROJECT_ROOT environment variable
    2. Fallback: walks up from current working directory
    """
    env_root = os.environ.get(EnvVar.PROJECT_ROOT)
    if env_root:
        return Path(env_root).resolve()

    # Current structure for g8ee is: <root>/services/g8ee
    cwd = Path.cwd()

    # Check if we are in services/g8ee or a subdirectory
    for parent in [cwd] + list(cwd.parents):
        if parent.name == "g8ee" and parent.parent.name == "components":
            return parent.parent.parent.resolve()

    # Generic fallback
    return (cwd / ".." / "..").resolve()

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
