# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for Jev validation in ChatPipelineService.validate_llm_config."""

from __future__ import annotations

import pytest

from app.constants import LLMProvider
from app.errors import ConfigurationError
from app.models.settings import G8eeUserSettings, LLMSettings
from app.services.ai.chat_pipeline import ChatPipelineService

pytestmark = pytest.mark.unit


def _pipeline() -> ChatPipelineService:
    return ChatPipelineService.__new__(ChatPipelineService)


class TestChatPipelineJevRoleValidation:
    def test_rejects_jev_on_primary_role(self):
        settings = G8eeUserSettings(
            llm=LLMSettings(
                llm_primary_provider=LLMProvider.JEV,
                llm_model="nimble",
            )
        )

        with pytest.raises(ConfigurationError, match="only supported for the lite role"):
            _pipeline().validate_llm_config(settings)

    def test_rejects_jev_on_assistant_role(self):
        settings = G8eeUserSettings(
            llm=LLMSettings(
                llm_primary_provider=LLMProvider.OLLAMA,
                llm_model="qwen3:0.6b",
                llm_assistant_provider=LLMProvider.JEV,
                llm_assistant_model="nimble",
            )
        )

        with pytest.raises(ConfigurationError, match="only supported for the lite role"):
            _pipeline().validate_llm_config(settings)


class TestChatPipelineJevLiteCoexistence:
    def test_rejects_jev_lite_when_tribunal_enabled(self):
        settings = G8eeUserSettings(
            llm=LLMSettings(
                llm_primary_provider=LLMProvider.OLLAMA,
                llm_model="qwen3:0.6b",
                llm_lite_provider=LLMProvider.JEV,
                llm_lite_model="nimble",
                llm_command_gen_enabled=True,
            )
        )

        with pytest.raises(ConfigurationError, match="Tribunal command generation"):
            _pipeline().validate_llm_config(settings)

    def test_accepts_jev_lite_when_tribunal_disabled(self):
        settings = G8eeUserSettings(
            llm=LLMSettings(
                llm_primary_provider=LLMProvider.OLLAMA,
                llm_model="qwen3:0.6b",
                llm_lite_provider=LLMProvider.JEV,
                llm_lite_model="nimble",
                llm_command_gen_enabled=False,
            )
        )

        _pipeline().validate_llm_config(settings)
