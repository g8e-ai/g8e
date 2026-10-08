# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging

from app.db.document_service import DocumentService
from app.errors import ConfigurationError
from app.llm.factory import set_settings
from app.models.settings import G8eeAppSettings
from app.services.infra.settings_service import SettingsService

logger = logging.getLogger(__name__)


async def initialize_g8e_service(
    service_name: str,
    settings: G8eeAppSettings,
    document_service: DocumentService | None,
    use_db_config: bool = True,
) -> G8eeAppSettings:
    if use_db_config:
        if document_service is None:
            raise ConfigurationError("document_service is required when use_db_config=True")
        logger.info("Loading configuration from operator app_settings for %s", service_name)

        service = SettingsService(document_service=document_service)
        settings = await G8eeAppSettings.from_db(service)
    elif not settings:
        logger.info("Creating default configuration for %s", service_name)
        settings = G8eeAppSettings()
    else:
        logger.info("Using provided configuration for %s", service_name)

    set_settings(settings)

    return settings
