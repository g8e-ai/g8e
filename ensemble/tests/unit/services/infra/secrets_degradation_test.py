# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Ensemble settings never load Operator secrets."""

import pytest

from app.models.settings import AuthSettings
from app.services.infra.settings_service import SettingsService

pytestmark = pytest.mark.unit


def test_auth_settings_have_no_operator_session_or_auditor_key():
    assert "session_encryption_key" not in AuthSettings.model_fields
    assert "auditor_hmac_key" not in AuthSettings.model_fields
    assert SettingsService().get_local_settings().auth.internal_api_key is None
