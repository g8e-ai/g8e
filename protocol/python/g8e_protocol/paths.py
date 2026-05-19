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

import os
from pathlib import Path

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
    """
    env_root = os.environ.get('G8E_PROJECT_ROOT')
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

PROJECT_ROOT = resolve_project_root()
G8E_DIR = PROJECT_ROOT / ".g8e"

def resolve_path(env_var: str, default: Path) -> Path:
    """Resolve a path from an environment variable or a default.
    
    If the environment variable contains a relative path, it is resolved
    relative to the project root.
    """
    val = os.environ.get(env_var)
    if val:
        p = Path(val)
        if p.is_absolute():
            return p
        return (PROJECT_ROOT / p).resolve()
    return default.resolve()

PKI_DIR = resolve_path("G8E_PKI_DIR", G8E_DIR / "pki")
SECRETS_DIR = resolve_path("G8E_SECRETS_DIR", G8E_DIR / "secrets")
TRUST_BUNDLE_PATH = PKI_DIR / "trust" / "hub-bundle.pem"

def get_trust_bundle() -> str:
    """Return the path to the hub trust bundle.
    
    Resolution order:
    1. G8E_TRUST_BUNDLE (explicit override)
    2. PKI_DIR/trust/hub-bundle.pem
    
    Raises FileNotFoundError if no bundle is available.
    """
    explicit = os.environ.get("G8E_TRUST_BUNDLE", "").strip()
    if explicit:
        path = Path(explicit)
        if not path.exists():
            raise FileNotFoundError(f"G8E_TRUST_BUNDLE points to a missing file: {path}")
        return str(path)

    if not TRUST_BUNDLE_PATH.exists():
        raise FileNotFoundError(
            f"Hub trust bundle not found at {TRUST_BUNDLE_PATH}. "
            "Run `./g8e platform start` to provision PKI, or set G8E_TRUST_BUNDLE."
        )
    return str(TRUST_BUNDLE_PATH)
