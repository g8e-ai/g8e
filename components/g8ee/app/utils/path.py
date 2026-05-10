import os
from pathlib import Path

def resolve_project_root() -> Path:
    """
    Resolves the project root directory.
    Priority:
    1. G8E_PROJECT_ROOT environment variable
    2. Fallback: walks up from current working directory
    """
    env_root = os.environ.get("G8E_PROJECT_ROOT")
    if env_root:
        return Path(env_root).resolve()

    # Current structure for g8ee is: <root>/components/g8ee
    cwd = Path.cwd()
    
    # Check if we are in components/g8ee or a subdirectory
    for parent in [cwd] + list(cwd.parents):
        if parent.name == "g8ee" and parent.parent.name == "components":
            return parent.parent.parent.resolve()
            
    # Generic fallback
    return (cwd / ".." / "..").resolve()
