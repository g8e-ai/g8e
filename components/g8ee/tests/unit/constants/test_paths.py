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

import pytest

from app.constants import paths as paths_module

pytestmark = [pytest.mark.unit]

load_paths = paths_module.__dict__["_load_paths"]


def _configure_shared_paths(monkeypatch: pytest.MonkeyPatch, tmp_path):
    shared_dir = tmp_path / "runner" / "work" / "g8e" / "g8e" / "shared"
    constants_dir = shared_dir / "constants"
    constants_dir.mkdir(parents=True)
    path_file = constants_dir / "paths.json"
    path_file.write_text(
        json.dumps(
            {
                "infra": {
                    "db_path": ".g8e/data/g8e.db",
                    "ca_cert_path": ".g8e/pki/trust/hub-bundle.pem",
                    "pki_dir": ".g8e/pki",
                    "docs_dir": "/docs",
                    "shared_dir": "/app/shared",
                    "shared_constants_dir": "/app/shared/constants",
                    "shared_models_dir": "/app/shared/models",
                    "ssh_config_path": "/etc/g8e/ssh_config",
                }
            }
        )
    )
    monkeypatch.setattr(paths_module, "_SHARED_DIR", str(shared_dir))
    monkeypatch.setattr(paths_module, "_CONTAINER_SHARED_CONSTANTS_DIR", str(constants_dir))
    monkeypatch.setattr(paths_module, "_PATH_FILE", str(path_file))
    return shared_dir


def test_load_paths_prefers_explicit_host_pki_dir(monkeypatch: pytest.MonkeyPatch, tmp_path):
    _configure_shared_paths(monkeypatch, tmp_path)
    pki_dir = tmp_path / "runner" / "work" / "g8e" / "g8e" / ".g8e" / "pki"
    monkeypatch.setenv("G8E_PKI_DIR", str(pki_dir))
    monkeypatch.delenv("G8E_RUNTIME_DIR", raising=False)

    paths = load_paths()

    assert paths["infra"]["pki_dir"] == str(pki_dir)
    assert paths["infra"]["ca_cert_path"] == str(pki_dir / "trust" / "hub-bundle.pem")


def test_load_paths_uses_host_runtime_dir_when_pki_dir_unset(monkeypatch: pytest.MonkeyPatch, tmp_path):
    _configure_shared_paths(monkeypatch, tmp_path)
    runtime_dir = tmp_path / "runner" / "work" / "g8e" / "g8e" / ".g8e"
    monkeypatch.delenv("G8E_PKI_DIR", raising=False)
    monkeypatch.setenv("G8E_RUNTIME_DIR", str(runtime_dir))

    paths = load_paths()

    assert paths["infra"]["pki_dir"] == str(runtime_dir / "pki")
    assert paths["infra"]["ca_cert_path"] == str(runtime_dir / "pki" / "trust" / "hub-bundle.pem")
