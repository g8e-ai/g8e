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

import logging

from app.errors import ConfigurationError
from app.models.settings import G8eePlatformSettings
from app.services.cache.cache_aside import CacheAsideService
from app.llm.factory import set_settings

logger = logging.getLogger(__name__)


async def initialize_vso_service(
    service_name: str,
    settings: G8eePlatformSettings,
    cache_aside_service: CacheAsideService,
    use_db_config: bool = True,
) -> G8eePlatformSettings:
    if use_db_config:
        if cache_aside_service is None:
            raise ConfigurationError("cache_aside_service is required when use_db_config=True")
        logger.info("Loading configuration from VSODB platform_settings for %s", service_name)
        
        from app.services.infra.settings_service import SettingsService
        from app.services.infra.bootstrap_service import BootstrapService
        bootstrap_service = BootstrapService()
        service = SettingsService(cache_aside_service=cache_aside_service, bootstrap_service=bootstrap_service)
        settings = await G8eePlatformSettings.from_db(service)
    else:
        if not settings:
            logger.info("Creating default configuration for %s", service_name)
            settings = G8eePlatformSettings()
        else:
            logger.info("Using provided configuration for %s", service_name)
    
    set_settings(settings)

    return settings
