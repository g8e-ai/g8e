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
from pathlib import Path
from typing import TypedDict, cast

from app.constants.env_vars import EnvVar
from app.utils.path import resolve_project_root
from app.constants.generated_paths import PathConstants, PortConstants


class InfraPaths(TypedDict):
    db_path: str
    ca_cert_path: str
    app_cert_dir: str
    pki_dir: str
    secrets_dir: str
    docs_dir: str
    protocol_dir: str
    protocol_constants_dir: str
    protocol_models_dir: str
    ssh_config_path: str


class G8eePaths(TypedDict):
    cert_name: str
    config_dir: str | None


class PathsDict(TypedDict):
    infra: InfraPaths
    g8ee: G8eePaths
    ports: dict[str, int]


def _get_protocol_dir() -> str:
    """Resolve protocol directory dynamically from environment or project root."""
    protocol_dir = os.environ.get(EnvVar.PROTOCOL_DIR) or ""
    if not protocol_dir:
        # If not provided, try to resolve from project root
        try:
            protocol_dir = str(resolve_project_root() / "protocol")
        except Exception:
            protocol_dir = "/app/protocol"
    return protocol_dir

def _get_path_file() -> str:
    """Resolve paths.json file location dynamically."""
    protocol_dir = _get_protocol_dir()
    return protocol_dir + "/constants/paths.json"

def _resolve_host_path(raw_path: str | None, default: Path) -> Path:
    path = Path(raw_path) if raw_path else default
    if not path.is_absolute():
        path = Path(_get_protocol_dir()).parent / path
    return path.expanduser().resolve()

def _host_runtime_paths() -> tuple[Path, Path]:
    protocol_dir = _get_protocol_dir()
    runtime_dir = _resolve_host_path(
        os.environ.get(EnvVar.RUNTIME_DIR),
        Path(protocol_dir).parent / ".g8e",
    )
    pki_dir = _resolve_host_path(
        os.environ.get(EnvVar.PKI_DIR),
        runtime_dir / "pki",
    )
    secrets_dir = _resolve_host_path(
        os.environ.get(EnvVar.SECRETS_DIR),
        runtime_dir / "secrets",
    )
    return pki_dir, secrets_dir

def _load_paths() -> PathsDict:
    protocol_dir = _get_protocol_dir()
    path_file = _get_path_file()
    
    try:
        with Path(path_file).open() as f:
            paths = json.load(f)
    except FileNotFoundError:
        # Emergency fallbacks for when protocol volume isn't ready
        # On host, default to .g8e/pki (Operator listen mode PKI directory)
        # In container, default to /pki for backwards compatibility
        if protocol_dir != "/app/protocol":
            pki_path, secrets_path = _host_runtime_paths()
            default_pki_dir = str(pki_path)
            default_secrets_dir = str(secrets_path)
        else:
            default_pki_dir = os.environ.get(EnvVar.PKI_DIR, PathConstants.PATH_PKI_DIR)
            default_secrets_dir = os.environ.get(EnvVar.SECRETS_DIR, PathConstants.PATH_SECRETS_DIR)
        app_cert_dir = str(Path(default_pki_dir) / "issued" / "apps")
        paths = {
            "infra": {
                "db_path": PathConstants.PATH_DB_PATH,
                "ca_cert_path": str(Path(default_pki_dir) / "trust" / "hub-bundle.pem"),
                "app_cert_dir": app_cert_dir,
                "pki_dir": os.environ.get(EnvVar.PKI_DIR, default_pki_dir),
                "secrets_dir": os.environ.get(EnvVar.SECRETS_DIR, default_secrets_dir),
                "docs_dir": PathConstants.PATH_DOCS_DIR,
                "protocol_dir": protocol_dir,
                "protocol_constants_dir": protocol_dir + "/constants",
                "protocol_models_dir": protocol_dir + "/models",
                "ssh_config_path": PathConstants.PATH_SSH_CONFIG_PATH,
            },
            "ports": {
                "operator_https": PortConstants.PORT_OPERATOR_HTTPS,
                "operator_bootstrap_https": PortConstants.PORT_OPERATOR_BOOTSTRAP_HTTPS,
                "operator_public_https": PortConstants.PORT_OPERATOR_PUBLIC_HTTPS,
                "g8ee_https": PortConstants.G8E_PORT_G8EE_HTTPS,
            },
            "g8ee": {
                "cert_name": "g8ee",
            }
        }
    except Exception as e:
        raise RuntimeError(f"Failed to load paths from {path_file}: {e}") from e

    # Override container paths with G8E_PROTOCOL_DIR when running on host
    # This allows evals and other host-based commands to use host paths
    if "infra" in paths and protocol_dir != "/app/protocol":
        paths["infra"]["protocol_dir"] = protocol_dir
        paths["infra"]["protocol_constants_dir"] = protocol_dir + "/constants"
        paths["infra"]["protocol_models_dir"] = protocol_dir + "/models"
        # Override PKI/secrets paths to use host runtime directory when running on host
        pki_path, secrets_path = _host_runtime_paths()
        paths["infra"]["pki_dir"] = str(pki_path)
        paths["infra"]["secrets_dir"] = str(secrets_path)
        paths["infra"]["ca_cert_path"] = str(pki_path / "trust" / "hub-bundle.pem")
        paths["infra"]["app_cert_dir"] = str(pki_path / "issued" / "apps")

    # Validate and normalize using Pydantic
    from app.constants.models import PathsConstants
    try:
        validated = PathsConstants.model_validate(paths)
        # Return as dict for compatibility with existing TypedDict usage
        return cast(PathsDict, validated.model_dump())
    except Exception as e:
        raise RuntimeError(f"Failed to validate paths: {e}") from e

# Cache for loaded paths to avoid repeated file I/O
_paths_cache: PathsDict | None = None

def get_paths() -> PathsDict:
    """Get paths, loading from file system on first call and caching thereafter.
    
    This function resolves environment variables and file system state dynamically
    on each call, making it test-friendly. Tests can monkeypatch environment variables
    and call reload_paths() to force re-resolution.
    """
    global _paths_cache
    if _paths_cache is None:
        _paths_cache = _load_paths()
    return _paths_cache

def reload_paths() -> None:
    """Clear the paths cache to force re-resolution on next get_paths() call.
    
    This is primarily for tests that need to monkeypatch environment variables
    and verify path resolution changes.
    """
    global _paths_cache
    _paths_cache = None

# Backwards compatibility: expose PATHS as a property that calls get_paths()
# This maintains existing code while enabling dynamic resolution
class _PathsProxy:
    """Proxy object that provides dict-like access to dynamically resolved paths."""
    def __getitem__(self, key):
        return get_paths()[key]
    
    def get(self, key, default=None):
        return get_paths().get(key, default)
    
    def __contains__(self, key):
        return key in get_paths()
    
    def keys(self):
        return get_paths().keys()
    
    def values(self):
        return get_paths().values()
    
    def items(self):
        return get_paths().items()

PATHS = _PathsProxy()

def get_app_cert_paths(app_name: str | None = None) -> tuple[str, str]:
    if app_name is None:
        app_name = PATHS.get("g8ee", {}).get("cert_name", "g8ee")
    app_cert_dir = PATHS["infra"]["app_cert_dir"]
    cert_path = str(Path(app_cert_dir) / f"{app_name}.crt")
    key_path = str(Path(app_cert_dir) / f"{app_name}.key")
    return cert_path, key_path
