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

"""
Unit tests for initialize_vso_service.

Covers:
- use_db_config=True: loads config from VSODB via cache_aside_service
- use_db_config=True without cache_aside_service: raises ValueError
- use_db_config=False with explicit settings: uses supplied settings object
- use_db_config=False without settings: creates VSEPlatformSettings()
"""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.models.settings import VSEPlatformSettings
from app.utils.service_init import initialize_vso_service
from app.errors import ConfigurationError

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


def _make_cache_aside_service():
    return MagicMock()


def _make_settings():
    return VSEPlatformSettings(port=443)


class TestUseDbConfigTrue:

    async def test_requires_cache_aside_service(self):
        with pytest.raises(ConfigurationError, match="cache_aside_service"):
            await initialize_vso_service(
                "test-service",
                settings=MagicMock(),
                cache_aside_service=None,
                use_db_config=True,
            )

    async def test_loads_settings_from_db(self):
        cache_svc = _make_cache_aside_service()
        expected_settings = _make_settings()

        with patch(
            "app.models.settings.VSEPlatformSettings.from_db",
            new_callable=AsyncMock,
            return_value=expected_settings,
        ) as mock_from_db:
            result = await initialize_vso_service(
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
            "app.models.settings.VSEPlatformSettings.from_db",
            new_callable=AsyncMock,
            return_value=loaded_settings,
        ):
            result = await initialize_vso_service(
                "my-service",
                settings=MagicMock(),
                cache_aside_service=cache_svc,
                use_db_config=True,
            )

        assert result is loaded_settings


class TestUseDbConfigFalse:

    async def test_uses_provided_settings_when_given(self):
        explicit_settings = _make_settings()
        result = await initialize_vso_service(
            "test-service",
            settings=explicit_settings,
            cache_aside_service=MagicMock(),
            use_db_config=False,
        )
        assert result is explicit_settings

    async def test_creates_default_settings_when_none_provided(self):
        with patch("app.utils.service_init.VSEPlatformSettings") as mock_settings_class:
            mock_settings_instance = MagicMock(spec=VSEPlatformSettings)
            mock_settings_class.return_value = mock_settings_instance
            result = await initialize_vso_service(
                "test-service",
                settings=None,
                cache_aside_service=MagicMock(),
                use_db_config=False,
            )
            assert result is mock_settings_instance
            # Ensure VSEPlatformSettings was called to create default
            mock_settings_class.assert_called_once()
