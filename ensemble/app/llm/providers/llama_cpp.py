# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""LLM provider implementation."""

import logging

from app.llm.providers.open_ai import OpenAIProvider

logger = logging.getLogger(__name__)


class LlamaCppProvider(OpenAIProvider):
    """llama.cpp provider using OpenAI-compatible API.

    llama.cpp server provides an OpenAI-compatible HTTP API, so we inherit from
    OpenAIProvider to reuse its implementation.
    """

    @property
    def service_name(self) -> str:
        return "llamacpp"

    @staticmethod
    def validate_config(api_key: str | None, endpoint: str | None) -> list[str]:
        """Validate llama.cpp provider configuration.

        llama.cpp requires an endpoint URL but does not require an API key.

        Args:
            api_key: The API key for the provider (unused by llama.cpp)
            endpoint: The endpoint URL for the provider

        Returns:
            List of validation error messages. Empty if configuration is valid.
        """
        errors = []
        if not endpoint:
            errors.append("Provider 'llamacpp' requires an endpoint URL.")
        return errors

    def __init__(self, endpoint: str, api_key: str):
        super().__init__(endpoint=endpoint, api_key=api_key)
        # CodeQL: Don't log full endpoint strings to avoid accidental leakage
        logger.info("llama.cpp provider initialized")
