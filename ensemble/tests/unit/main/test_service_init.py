# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Unit tests for initialize_g8e_service.

Covers:
- use_db_config=True: loads config from operator via cache_aside_service
- use_db_config=True without cache_aside_service: raises ValueError
- use_db_config=False with explicit settings: uses supplied settings object
- use_db_config=False without settings: creates G8eeAppSettings()
"""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.errors import ConfigurationError
from app.models.settings import G8eeAppSettings
from app.utils.service_init import initialize_g8e_service
from app.constants.generated_paths import PathConstants, PortConstants

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


def _make_cache_aside_service():
    return MagicMock()


def _make_settings():
    return G8eeAppSettings(port=PortConstants.G8E_PORT_G8EE_HTTPS)


class TestUseDbConfigTrue:
    async def test_requires_cache_aside_service(self):
        with pytest.raises(ConfigurationError, match="cache_aside_service"):
            await initialize_g8e_service(
                "test-service",
                settings=MagicMock(),
                cache_aside_service=None,
                use_db_config=True,
            )

    async def test_loads_settings_from_db(self):
        cache_svc = _make_cache_aside_service()
        expected_settings = _make_settings()

        with patch(
            "app.models.settings.G8eeAppSettings.from_db",
            new_callable=AsyncMock,
            return_value=expected_settings,
        ) as mock_from_db:
            result = await initialize_g8e_service(
                "test-service",
                settings=MagicMock(),
                cache_aside_service=cache_svc,
                use_db_config=True,
            )

        mock_from_db.assert_called_once()
        assert result is expected_settings

    async def test_returns_settings_from_db(self):
        cache_svc = _make_cache_aside_service()
        loaded_settings = _make_settings()

        with patch(
            "app.models.settings.G8eeAppSettings.from_db",
            new_callable=AsyncMock,
            return_value=loaded_settings,
        ):
            result = await initialize_g8e_service(
                "my-service",
                settings=MagicMock(),
                cache_aside_service=cache_svc,
                use_db_config=True,
            )

        assert result is loaded_settings


class TestUseDbConfigFalse:
    async def test_uses_provided_settings_when_given(self):
        explicit_settings = _make_settings()
        result = await initialize_g8e_service(
            "test-service",
            settings=explicit_settings,
            cache_aside_service=MagicMock(),
            use_db_config=False,
        )
        assert result is explicit_settings

    async def test_creates_default_settings_when_none_provided(self):
        with patch("app.utils.service_init.G8eeAppSettings") as mock_settings_class:
            mock_settings_instance = MagicMock(spec=G8eeAppSettings)
            mock_settings_class.return_value = mock_settings_instance
            result = await initialize_g8e_service(
                "test-service",
                settings=None,
                cache_aside_service=MagicMock(),
                use_db_config=False,
            )
            assert result is mock_settings_instance
            # Ensure G8eeAppSettings was called to create default
            mock_settings_class.assert_called_once()
