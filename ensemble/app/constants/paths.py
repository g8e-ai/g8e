# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging
from collections.abc import ItemsView, KeysView, ValuesView
from pathlib import Path
from typing import Literal, TypedDict, cast, overload

from app.constants.bootstrap import BootstrapSettings, get_bootstrap
from app.constants.generated_paths import PortConstants
from app.constants.models import PathsConstants
from app.utils.path import resolve_project_root

logger = logging.getLogger(__name__)


class InfraPaths(TypedDict):
    db_path: str
    ca_cert_path: str
    app_cert_dir: str
    pki_dir: str
    docs_dir: str
    ssh_config_path: str
    pending_enrollment_dir: str
    pending_enrollment_file: str


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


def resolve_runtime_dir() -> Path:
    """Resolve the ``.g8e`` runtime directory (``--runtime-dir`` or ``<project>/.g8e``)."""
    return _resolve_host_path(
        get_bootstrap().runtime_dir,
        resolve_project_root() / ".g8e",
    )


def _host_runtime_paths() -> Path:
    bootstrap = get_bootstrap()
    runtime_dir = resolve_runtime_dir()
    return _resolve_host_path(bootstrap.pki_dir, runtime_dir / "pki")


def _load_paths() -> PathsDict:
    project_root = resolve_project_root()

    # Default paths when no protocol volume
    pki_path = _host_runtime_paths()
    default_pki_dir = str(pki_path)
    app_cert_dir = str(Path(default_pki_dir) / "issued" / "apps")

    default_ca_cert_path = str(Path(default_pki_dir) / "trust" / "g8eg-ca-bundle.pem")
    ca_cert_path = get_bootstrap().ca_cert_path or default_ca_cert_path
    pending_enrollment_dir = str(Path(default_pki_dir) / "pending-enrollment")

    paths = {
        "infra": {
            "db_path": str(project_root / ".g8e" / "db"),
            "ca_cert_path": ca_cert_path,
            "app_cert_dir": app_cert_dir,
            "pki_dir": default_pki_dir,
            "docs_dir": str(project_root / "docs"),
            "ssh_config_path": str(project_root / ".g8e" / "ssh_config"),
            "pending_enrollment_dir": pending_enrollment_dir,
            "pending_enrollment_file": "g8ee.json",
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
    try:
        validated = PathsConstants.model_validate(paths)
        # Return as dict for compatibility with existing TypedDict usage
        return cast(PathsDict, validated.model_dump())
    except Exception as e:
        raise RuntimeError(f"Failed to validate paths: {e}") from e


class _PathsCache:
    """Cache for loaded paths, keyed by the bootstrap settings they were resolved from."""

    paths: PathsDict | None = None
    bootstrap: BootstrapSettings | None = None


def get_paths() -> PathsDict:
    """Get paths, loading from file system on first call and caching thereafter.

    Resolution reads the typed bootstrap settings (``app.constants.bootstrap``).
    Installing new settings with ``configure_bootstrap`` invalidates the cache.
    """
    bootstrap = get_bootstrap()
    if _PathsCache.paths is None or _PathsCache.bootstrap is not bootstrap:
        _PathsCache.paths = _load_paths()
        _PathsCache.bootstrap = bootstrap
    return _PathsCache.paths


def reload_paths() -> None:
    """Clear the paths cache to force re-resolution on next get_paths() call.

    This is primarily for tests that change bootstrap settings and verify path
    resolution changes.
    """
    _PathsCache.paths = None
    _PathsCache.bootstrap = None


# Backwards compatibility: expose PATHS as a property that calls get_paths()
# This maintains existing code while enabling dynamic resolution
class _PathsProxy:
    """Proxy object that provides dict-like access to dynamically resolved paths."""

    @overload
    def __getitem__(self, key: Literal["infra"]) -> InfraPaths: ...
    @overload
    def __getitem__(self, key: Literal["g8ee"]) -> G8eePaths: ...
    @overload
    def __getitem__(self, key: Literal["ports"]) -> dict[str, int]: ...
    def __getitem__(self, key: str) -> InfraPaths | G8eePaths | dict[str, int]:
        return cast(dict[str, InfraPaths | G8eePaths | dict[str, int]], get_paths())[key]

    def get(self, key: Literal["host"], default: str) -> str:
        """Return the optional ``host`` entry, which the resolved paths do not define."""
        value: object = cast(dict[str, object], get_paths()).get(key, default)
        return value if isinstance(value, str) else default

    def __contains__(self, key: str) -> bool:
        return key in get_paths()

    def keys(self) -> KeysView[str]:
        return get_paths().keys()

    def values(self) -> ValuesView[object]:
        return get_paths().values()

    def items(self) -> ItemsView[str, object]:
        return get_paths().items()


PATHS = _PathsProxy()


def get_app_cert_paths(app_name: str | None = None) -> tuple[str, str]:
    if app_name is None:
        app_name = PATHS["g8ee"].get("cert_name", "g8ee")
    app_cert_dir = PATHS["infra"]["app_cert_dir"]
    cert_path = str(Path(app_cert_dir) / f"{app_name}.crt")
    key_path = str(Path(app_cert_dir) / f"{app_name}.key")
    return cert_path, key_path


def resolve_config_path(filename: str) -> Path:
    """
    Resolves a config file path using centralized PATHS if available,
    otherwise falls back to repo-relative resolution.
    """
    config_dir = PATHS["g8ee"].get("config_dir")
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
