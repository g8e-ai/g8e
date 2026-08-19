# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging

from app.errors import ConfigurationError
from app.models.settings import G8eeAppSettings
from app.services.cache.cache_aside import CacheAsideService
from app.llm.factory import set_settings
from app.services.infra.settings_service import SettingsService
from app.services.infra.bootstrap_service import BootstrapService

logger = logging.getLogger(__name__)


async def initialize_g8e_service(
    service_name: str,
    settings: G8eeAppSettings,
    cache_aside_service: CacheAsideService,
    use_db_config: bool = True,
) -> G8eeAppSettings:
    if use_db_config:
        if cache_aside_service is None:
            raise ConfigurationError("cache_aside_service is required when use_db_config=True")
        logger.info("Loading configuration from operator app_settings for %s", service_name)

        bootstrap_service = BootstrapService()
        service = SettingsService(
            cache_aside_service=cache_aside_service, bootstrap_service=bootstrap_service
        )
        settings = await G8eeAppSettings.from_db(service)
    elif not settings:
        logger.info("Creating default configuration for %s", service_name)
        settings = G8eeAppSettings()
    else:
        logger.info("Using provided configuration for %s", service_name)

    set_settings(settings)

    return settings
