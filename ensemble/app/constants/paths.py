# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import json
import os
from pathlib import Path
from typing import TypedDict, cast

from app.constants.env_vars import EnvVar
from app.constants.generated_paths import PortConstants
from app.utils.path import resolve_project_root


class InfraPaths(TypedDict):
    db_path: str
    ca_cert_path: str
    app_cert_dir: str
    pki_dir: str
    secrets_dir: str
    docs_dir: str
    ssh_config_path: str


class G8eePaths(TypedDict):
    cert_name: str
    config_dir: str | None


class PathsDict(TypedDict):
    infra: InfraPaths
    g8ee: G8eePaths
    ports: dict[str, int]


def _resolve_host_path(raw_path: str | None, default: Path) -> Path:
    path = Path(raw_path) if raw_path else default
    if not path.is_absolute():
        path = resolve_project_root() / path
    return path.expanduser().resolve()


def _host_runtime_paths() -> tuple[Path, Path]:
    runtime_dir = _resolve_host_path(
        os.environ.get(EnvVar.RUNTIME_DIR),
        resolve_project_root() / ".g8e",
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
    project_root = resolve_project_root()

    # Default paths when no protocol volume
    pki_path, secrets_path = _host_runtime_paths()
    default_pki_dir = str(pki_path)
    default_secrets_dir = str(secrets_path)
    app_cert_dir = str(Path(default_pki_dir) / "issued" / "apps")

    default_ca_cert_path = str(Path(default_pki_dir) / "trust" / "hub-bundle.pem")
    ca_cert_path = os.environ.get(EnvVar.CA_CERT_PATH) or default_ca_cert_path

    paths = {
        "infra": {
            "db_path": str(project_root / ".g8e" / "db"),
            "ca_cert_path": ca_cert_path,
            "app_cert_dir": app_cert_dir,
            "pki_dir": os.environ.get(EnvVar.PKI_DIR, default_pki_dir),
            "secrets_dir": os.environ.get(EnvVar.SECRETS_DIR, default_secrets_dir),
            "docs_dir": str(project_root / "docs"),
            "ssh_config_path": str(project_root / ".g8e" / "ssh_config"),
        },
        "ports": {
            "operator_http": PortConstants.PORT_OPERATOR_HTTP,
            "operator_https": PortConstants.PORT_OPERATOR_HTTPS,
            "g8ee_https": PortConstants.G8E_PORT_G8EE_HTTPS,
        },
        "g8ee": {
            "cert_name": "g8ee",
        },
    }

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
