# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import importlib.util

import pytest

from app.models.settings import G8eeAppSettings
from app.services.ai.tool_registry import get_tool_spec
from app.services.infra.settings_service import SettingsService

pytestmark = pytest.mark.unit


def test_model_cannot_invoke_docker_stream_executor():
    assert get_tool_spec("stream_operator_to_ssh_fleet") is None
    assert importlib.util.find_spec("app.services.operator.stream_executor") is None


def test_ensemble_settings_hold_no_operator_secrets_or_docker_identity():
    assert "auditor_hmac_key" not in G8eeAppSettings().model_dump_json()
    assert "docker_gid" not in SettingsService().get_local_settings().model_dump_json()
