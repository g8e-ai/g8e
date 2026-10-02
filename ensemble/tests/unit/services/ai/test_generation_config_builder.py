# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest

from app.constants import ANTHROPIC_CLAUDE_HAIKU_4_5, OLLAMA_GEMMA4_E4B
from app.llm.llm_types import AssistantLLMSettings, LiteLLMSettings, PrimaryLLMSettings
from app.models.model_configs import get_model_config
from app.services.ai.generation_config_builder import AIGenerationConfigBuilder

pytestmark = [pytest.mark.unit]


class TestResolveMaxOutputTokens:
    """The builder is the only place an output limit is resolved."""

    def test_caller_value_wins_over_model_ceiling(self):
        config = get_model_config(ANTHROPIC_CLAUDE_HAIKU_4_5)
        assert AIGenerationConfigBuilder.resolve_max_output_tokens(config, 512) == 512

    def test_model_ceiling_used_when_caller_value_unset(self):
        config = get_model_config(ANTHROPIC_CLAUDE_HAIKU_4_5)
        assert AIGenerationConfigBuilder.resolve_max_output_tokens(config, None) == 64_000

    def test_local_model_without_ceiling_stays_unset(self):
        config = get_model_config(OLLAMA_GEMMA4_E4B)
        assert config.max_output_tokens is None
        assert AIGenerationConfigBuilder.resolve_max_output_tokens(config, None) is None

    def test_no_model_config_stays_unset(self):
        assert AIGenerationConfigBuilder.resolve_max_output_tokens(None, None) is None


class TestSettingsTypesDefaultToUnset:
    @pytest.mark.parametrize(
        "settings_type", [PrimaryLLMSettings, AssistantLLMSettings, LiteLLMSettings]
    )
    def test_max_output_tokens_defaults_to_none(self, settings_type):
        assert settings_type().max_output_tokens is None
