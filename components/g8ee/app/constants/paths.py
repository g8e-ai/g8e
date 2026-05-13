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
from app.utils.path import resolve_project_root

# The bridge to shared paths.
# In container, this is always /app/shared/constants/paths.json
# On host, respect G8E_SHARED_DIR environment variable
_SHARED_DIR = os.environ.get("G8E_SHARED_DIR")
if _SHARED_DIR is None:
    # If not provided, try to resolve from project root
    try:
        _SHARED_DIR = str(resolve_project_root() / "shared")
    except Exception:
        _SHARED_DIR = "/app/shared"

_CONTAINER_SHARED_CONSTANTS_DIR = _SHARED_DIR + "/constants"
_PATH_FILE = _CONTAINER_SHARED_CONSTANTS_DIR + "/paths.json"

def _load_paths() -> dict:
    try:
        with Path(_PATH_FILE).open() as f:
            paths = json.load(f)
    except FileNotFoundError:
        # Emergency fallbacks for when shared volume isn't ready
        # On host, default to .g8e/pki (Operator listen mode PKI directory)
        # In container, default to /pki for backwards compatibility
        if _SHARED_DIR != "/app/shared":
            default_runtime_dir = os.environ.get("G8E_RUNTIME_DIR", str(Path(_SHARED_DIR).parent / ".g8e"))
            default_pki_dir = os.environ.get("G8E_PKI_DIR", str(Path(default_runtime_dir) / "pki"))
            default_secrets_dir = os.environ.get("G8E_SECRETS_DIR", str(Path(default_runtime_dir) / "secrets"))
        else:
            default_pki_dir = os.environ.get("G8E_PKI_DIR", "/pki")
            default_secrets_dir = os.environ.get("G8E_SECRETS_DIR", "/secrets")
        paths = {
            "infra": {
                "db_path": "/data/g8e.db",
                "ca_cert_path": str(Path(default_pki_dir) / "trust" / "hub-bundle.pem"),
                "client_cert_path": str(Path(default_pki_dir) / "issued" / "apps" / "g8ee.crt"),
                "client_key_path": str(Path(default_pki_dir) / "issued" / "apps" / "g8ee.key"),
                "pki_dir": os.environ.get("G8E_PKI_DIR", default_pki_dir),
                "secrets_dir": os.environ.get("G8E_SECRETS_DIR", default_secrets_dir),
                "docs_dir": "/docs",
                "shared_dir": _SHARED_DIR,
                "shared_constants_dir": _SHARED_DIR + "/constants",
                "shared_models_dir": _SHARED_DIR + "/models",
                "ssh_config_path": "/etc/g8e/ssh_config",
            }
        }
    except Exception as e:
        raise RuntimeError(f"Failed to load paths from {_PATH_FILE}: {e}") from e

    # Override container paths with G8E_SHARED_DIR when running on host
    # This allows evals and other host-based commands to use host paths
    if "infra" in paths and _SHARED_DIR != "/app/shared":
        paths["infra"]["shared_dir"] = _SHARED_DIR
        paths["infra"]["shared_constants_dir"] = _SHARED_DIR + "/constants"
        paths["infra"]["shared_models_dir"] = _SHARED_DIR + "/models"
        # Override PKI/secrets paths to use host runtime directory when running on host
        host_runtime_dir = os.environ.get("G8E_RUNTIME_DIR", str(Path(_SHARED_DIR).parent / ".g8e"))
        host_pki_dir = os.environ.get("G8E_PKI_DIR", str(Path(host_runtime_dir) / "pki"))
        host_secrets_dir = os.environ.get("G8E_SECRETS_DIR", str(Path(host_runtime_dir) / "secrets"))
        paths["infra"]["pki_dir"] = host_pki_dir
        paths["infra"]["secrets_dir"] = host_secrets_dir
        paths["infra"]["ca_cert_path"] = str(Path(host_pki_dir) / "trust" / "hub-bundle.pem")
        paths["infra"]["client_cert_path"] = str(Path(host_pki_dir) / "issued" / "apps" / "g8ee.crt")
        paths["infra"]["client_key_path"] = str(Path(host_pki_dir) / "issued" / "apps" / "g8ee.key")

    return paths

PATHS = _load_paths()
