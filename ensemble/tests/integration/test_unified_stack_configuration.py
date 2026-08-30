# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

_ROOT_COMPOSE_FILE = Path(__file__).resolve().parents[3] / "docker-compose.yml"
_ENSEMBLE_SERVICE = "  ensemble:"
_OPERATOR_VOLUME = "      - g8e-operator-data:/operator-state:ro"
_OPERATOR_SECRETS_ENV = "      - G8E_SECRETS_DIR=/operator-state/secrets"


def _ensemble_service_block() -> list[str]:
    lines = _ROOT_COMPOSE_FILE.read_text().splitlines()
    start = lines.index(_ENSEMBLE_SERVICE)
    end = next(
        index
        for index, line in enumerate(lines[start + 1 :], start=start + 1)
        if line.startswith("  ") and not line.startswith("    ") and line.endswith(":")
    )
    return lines[start:end]


def test_ensemble_uses_mounted_operator_secrets_directory() -> None:
    service = _ensemble_service_block()

    assert _OPERATOR_VOLUME in service
    assert _OPERATOR_SECRETS_ENV in service
