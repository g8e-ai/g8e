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


def _ensemble_service_block() -> list[str]:
    lines = _ROOT_COMPOSE_FILE.read_text().splitlines()
    start = lines.index(_ENSEMBLE_SERVICE)
    end = next(
        index
        for index, line in enumerate(lines[start + 1 :], start=start + 1)
        if line.startswith("  ") and not line.startswith("    ") and line.endswith(":")
    )
    return lines[start:end]


def test_ensemble_uses_only_its_own_runtime_volume() -> None:
    service = _ensemble_service_block()

    assert _OPERATOR_VOLUME not in service
    assert not any("operator-state" in line or "--secrets-dir" in line for line in service)
    assert "      - g8e-ensemble-data:/root/.g8e" in service


def test_ensemble_platform_config_is_passed_as_arguments_not_environment() -> None:
    service = _ensemble_service_block()

    assert not any(line.strip() == "environment:" for line in service)
    assert not any("G8E_" in line for line in service)
