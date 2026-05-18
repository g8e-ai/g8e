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

from app.constants.env_vars import EnvVar
from app.utils.path import resolve_project_root

# The bridge to protocol paths.
# In container, this is always /app/protocol/constants/paths.json
# On host, respect G8E_PROTOCOL_DIR environment variable
_PROTOCOL_DIR = os.environ.get(EnvVar.PROTOCOL_DIR) or None
if _PROTOCOL_DIR is None:
    # If not provided, try to resolve from project root
    try:
        _PROTOCOL_DIR = str(resolve_project_root() / "protocol")
    except Exception:
        _PROTOCOL_DIR = "/app/protocol"

_CONTAINER_PROTOCOL_CONSTANTS_DIR = _PROTOCOL_DIR + "/constants"
_PATH_FILE = _CONTAINER_PROTOCOL_CONSTANTS_DIR + "/paths.json"

def _resolve_host_path(raw_path: str | None, default: Path) -> Path:
    path = Path(raw_path) if raw_path else default
    if not path.is_absolute():
        path = Path(_PROTOCOL_DIR).parent / path
    return path.expanduser().resolve()

def _host_runtime_paths() -> tuple[Path, Path]:
    runtime_dir = _resolve_host_path(
        os.environ.get(EnvVar.RUNTIME_DIR),
        Path(_PROTOCOL_DIR).parent / ".g8e",
    )
    pki_dir = _resolve_host_path(
        os.environ.get(EnvVar.PKIDir),
        runtime_dir / "pki",
    )
    secrets_dir = _resolve_host_path(
        os.environ.get(EnvVar.SECRETS_DIR),
        runtime_dir / "secrets",
    )
    return pki_dir, secrets_dir

def _load_paths() -> dict:
    try:
        with Path(_PATH_FILE).open() as f:
            paths = json.load(f)
    except FileNotFoundError:
        # Emergency fallbacks for when protocol volume isn't ready
        # On host, default to .g8e/pki (Operator listen mode PKI directory)
        # In container, default to /pki for backwards compatibility
        if _PROTOCOL_DIR != "/app/protocol":
            pki_path, secrets_path = _host_runtime_paths()
            default_pki_dir = str(pki_path)
            default_secrets_dir = str(secrets_path)
        else:
            default_pki_dir = os.environ.get(EnvVar.PKIDir, "/pki")
            default_secrets_dir = os.environ.get(EnvVar.SECRETS_DIR, "/secrets")
        app_cert_dir = str(Path(default_pki_dir) / "issued" / "apps")
        paths = {
            "infra": {
                "db_path": "/data/g8e.db",
                "ca_cert_path": str(Path(default_pki_dir) / "trust" / "hub-bundle.pem"),
                "app_cert_dir": app_cert_dir,
                "pki_dir": os.environ.get(EnvVar.PKIDir, default_pki_dir),
                "secrets_dir": os.environ.get(EnvVar.SECRETS_DIR, default_secrets_dir),
                "docs_dir": "/docs",
                "protocol_dir": _PROTOCOL_DIR,
                "protocol_constants_dir": _PROTOCOL_DIR + "/constants",
                "protocol_models_dir": _PROTOCOL_DIR + "/models",
                "ssh_config_path": "/etc/g8e/ssh_config",
            },
            "g8ee": {
                "cert_name": "g8ee",
            }
        }
    except Exception as e:
        raise RuntimeError(f"Failed to load paths from {_PATH_FILE}: {e}") from e

    # Override container paths with G8E_PROTOCOL_DIR when running on host
    # This allows evals and other host-based commands to use host paths
    if "infra" in paths and _PROTOCOL_DIR != "/app/protocol":
        paths["infra"]["protocol_dir"] = _PROTOCOL_DIR
        paths["infra"]["protocol_constants_dir"] = _PROTOCOL_DIR + "/constants"
        paths["infra"]["protocol_models_dir"] = _PROTOCOL_DIR + "/models"
        # Override PKI/secrets paths to use host runtime directory when running on host
        pki_path, secrets_path = _host_runtime_paths()
        paths["infra"]["pki_dir"] = str(pki_path)
        paths["infra"]["secrets_dir"] = str(secrets_path)
        paths["infra"]["ca_cert_path"] = str(pki_path / "trust" / "hub-bundle.pem")
        paths["infra"]["app_cert_dir"] = str(pki_path / "issued" / "apps")

    return paths

PATHS = _load_paths()

def get_app_cert_paths(app_name: str = None) -> tuple[str, str]:
    if app_name is None:
        app_name = PATHS.get("g8ee", {}).get("cert_name", "g8ee")
    app_cert_dir = PATHS["infra"]["app_cert_dir"]
    cert_path = str(Path(app_cert_dir) / f"{app_name}.crt")
    key_path = str(Path(app_cert_dir) / f"{app_name}.key")
    return cert_path, key_path
