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
AI Generation Config Builder - Pure functions for building GenerateContentConfig.

Stateless config constructors used by services that never hold an AIRequestBuilder
instance (triage, title generator, response analyzer, memory generation).

Separation of Concerns:
- AIGenerationConfigBuilder: stateless config factory (this file)
- AIRequestBuilder: stateful request assembly (tool executor, contents, attachments)
"""

import logging

from app.constants import LLM_DEFAULT_TEMPERATURE, LLM_DEFAULT_MAX_OUTPUT_TOKENS
import app.llm.llm_types as types
from app.llm.llm_types import (
    AssistantLLMSettings,
    LiteLLMSettings,
    PrimaryLLMSettings,
)
from app.models.model_configs import get_model_config, thinking_level_for_config

logger = logging.getLogger(__name__)


class AIGenerationConfigBuilder:
    """Stateless factory for GenerateContentConfig objects.

    All methods are static — no instance required. Used directly by services
    that build configs without needing AIRequestBuilder's tool/attachment state.
    """

    @staticmethod
    def _build_thinking_config(model_name: str) -> types.ThinkingConfig:
        """Build ThinkingConfig based on model capabilities.

        The High Reasoning Agent (The Proposer) uses the highest supported thinking
        level to perform complex architectural planning and deep logical exploration.
        """
        config = get_model_config(model_name) if model_name else None

        if config and config.supports_thinking and config.supported_thinking_levels:
            thinking_level = thinking_level_for_config(config)
            logger.info(f"[CONFIG] ThinkingConfig: thinking_level={thinking_level}, include_thoughts=True (model={config.name})")
            return types.ThinkingConfig(
                thinking_level=thinking_level,
                include_thoughts=True,
            )

        logger.info(f"[CONFIG] ThinkingConfig: disabled (model={config.name if config else 'None'} does not support thinking)")
        return types.ThinkingConfig(
            thinking_level=None,
            include_thoughts=False,
        )

    @staticmethod
    def build_primary_settings(
        model: str,
        temperature: float | None,
        max_tokens: int | None,
        system_instructions: str,
        tools: list[types.ToolGroup],
    ) -> PrimaryLLMSettings:
        """Build PrimaryLLMSettings for main-model calls."""
        thinking_config = AIGenerationConfigBuilder._build_thinking_config(model_name=model)
        model_config = get_model_config(model)

        if temperature is not None:
            effective_temperature = temperature
        else:
            effective_temperature = model_config.default_temperature if model_config and model_config.default_temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = max_tokens if max_tokens is not None else (model_config.max_output_tokens if model_config and model_config.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS)
        effective_top_k = model_config.top_k if model_config and model_config.top_k is not None else None
        effective_top_p = model_config.top_p if model_config and model_config.top_p is not None else 1.0

        settings = PrimaryLLMSettings(
            temperature=effective_temperature,
            max_output_tokens=effective_max_tokens,
            top_k_filtering=effective_top_k,
            top_p_nucleus_sampling=effective_top_p,
            thinking_config=thinking_config,
            tools=tools,
            system_instructions=system_instructions,
        )

        thinking_level = getattr(thinking_config, "thinking_level", None)
        tool_names = []
        for t in (tools or []):
            for fd in (t.tools or []):
                tool_names.append(getattr(fd, 'name', '?'))  # type: ignore[attr-defined]
        logger.info(
            f" [BUILD_CONFIG] primary model={model}, max_tokens={effective_max_tokens}, "
            f"thinking_level={thinking_level}, tools_count={len(tools) if tools else 0}, "
            f"tool_names={tool_names}"
        )
        return settings

    @staticmethod
    def build_assistant_settings(
        model: str,
        temperature: float | None,
        max_tokens: int | None,
        system_instructions: str,
        response_format: types.ResponseFormat | None = None,
    ) -> AssistantLLMSettings:
        """Build AssistantLLMSettings for analysis calls."""
        model_config = get_model_config(model)

        if temperature is not None:
            effective_temperature = temperature
        else:
            effective_temperature = model_config.default_temperature if model_config and model_config.default_temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = max_tokens if max_tokens is not None else (model_config.max_output_tokens if model_config and model_config.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS)
        effective_top_k = model_config.top_k if model_config and model_config.top_k is not None else None
        effective_top_p = model_config.top_p if model_config and model_config.top_p is not None else 1.0

        settings = AssistantLLMSettings(
            temperature=effective_temperature,
            max_output_tokens=effective_max_tokens,
            top_k_filtering=effective_top_k,
            top_p_nucleus_sampling=effective_top_p,
            system_instructions=system_instructions,
            response_format=response_format if response_format else None,
        )

        logger.info(
            f" [BUILD_CONFIG] assistant model={model}, max_tokens={effective_max_tokens}"
        )
        return settings

    @staticmethod
    def build_lite_settings(
        model: str,
        temperature: float | None,
        max_tokens: int | None,
        system_instructions: str,
        response_format: types.ResponseFormat | None = None,
    ) -> LiteLLMSettings:
        """Build LiteLLMSettings for stateless analysis calls.

        Used by triage, memory updates, risk analysis, and error analysis.
        """
        model_config = get_model_config(model)

        if temperature is not None:
            effective_temperature = temperature
        else:
            effective_temperature = model_config.default_temperature if model_config and model_config.default_temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = max_tokens if max_tokens is not None else (model_config.max_output_tokens if model_config and model_config.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS)
        effective_top_k = model_config.top_k if model_config and model_config.top_k is not None else None
        effective_top_p = model_config.top_p if model_config and model_config.top_p is not None else 1.0

        settings = LiteLLMSettings(
            temperature=effective_temperature,
            max_output_tokens=effective_max_tokens,
            top_k_filtering=effective_top_k,
            top_p_nucleus_sampling=effective_top_p,
            system_instructions=system_instructions,
            response_format=response_format if response_format else None,
        )

        logger.info(
            f" [BUILD_CONFIG] lite model={model}, max_tokens={effective_max_tokens}"
        )
        return settings

    @staticmethod
    def build_config(
        model: str,
        temperature: float | None,
        max_tokens: int | None,
        system_instructions: str,
        tools: list[types.ToolGroup],
    ) -> types.GenerateContentConfig:
        """Shared config construction. All public methods delegate here."""
        thinking_config = AIGenerationConfigBuilder._build_thinking_config(model_name=model)
        model_config = get_model_config(model)

        if temperature is not None:
            effective_temperature = temperature
        else:
            effective_temperature = model_config.default_temperature if model_config and model_config.default_temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = max_tokens if max_tokens is not None else (model_config.max_output_tokens if model_config and model_config.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS)
        effective_top_k = model_config.top_k if model_config and model_config.top_k is not None else None
        effective_top_p = model_config.top_p if model_config and model_config.top_p is not None else 1.0

        config = types.GenerateContentConfig(
            temperature=effective_temperature,
            max_output_tokens=effective_max_tokens,
            top_k_filtering=effective_top_k,
            top_p_nucleus_sampling=effective_top_p,
            thinking_config=thinking_config,
            tools=tools,
            system_instructions=system_instructions,
        )

        thinking_level = getattr(thinking_config, "thinking_level", None)
        tool_names = []
        for t in (tools or []):
            for fd in (t.tools or []):
                tool_names.append(getattr(fd, 'name', '?'))  # type: ignore[attr-defined]
        logger.info(
            f" [BUILD_CONFIG] model={model}, max_tokens={effective_max_tokens}, "
            f"thinking_level={thinking_level}, tools_count={len(tools) if tools else 0}, "
            f"tool_names={tool_names}"
        )
        return config

    @staticmethod
    def get_lite_generation_config(
        model: str,
        temperature: float | None,
        max_tokens: int | None,
        system_instructions: str,
    ) -> types.GenerateContentConfig:
        """Build a lightweight GenerateContentConfig for stateless analysis calls.

        Used by triage, memory updates, risk analysis, and error analysis.
        Thinking is explicitly disabled — these calls expect concise, parseable
        text output and must never emit thought tokens.
        """
        no_thinking = types.ThinkingConfig(thinking_level=None, include_thoughts=False)
        model_config = get_model_config(model)

        if temperature is not None:
            effective_temperature = temperature
        else:
            effective_temperature = model_config.default_temperature if model_config and model_config.default_temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = max_tokens if max_tokens is not None else (model_config.max_output_tokens if model_config and model_config.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS)
        effective_top_k = model_config.top_k if model_config and model_config.top_k is not None else None
        effective_top_p = model_config.top_p if model_config and model_config.top_p is not None else 1.0

        config = types.GenerateContentConfig(
            temperature=effective_temperature,
            max_output_tokens=effective_max_tokens,
            top_k_filtering=effective_top_k,
            top_p_nucleus_sampling=effective_top_p,
            thinking_config=no_thinking,
            system_instructions=system_instructions,
        )

        logger.info(
            f" [BUILD_CONFIG] lite model={model}, max_tokens={effective_max_tokens}, thinking=disabled"
        )
        return config

    @staticmethod
    def get_title_generation_config(
        model: str,
    ) -> types.GenerateContentConfig:
        """Build a GenerateContentConfig for case title generation.

        Stops at the first newline.
        """
        model_config = get_model_config(model)
        effective_temperature = model_config.default_temperature if model_config and model_config.default_temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = model_config.max_output_tokens if model_config and model_config.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS
        effective_top_p = model_config.top_p if model_config and model_config.top_p is not None else 1.0

        config = types.GenerateContentConfig(
            temperature=effective_temperature,
            max_output_tokens=effective_max_tokens,
            top_p_nucleus_sampling=effective_top_p,
            stop_sequences=["\n"],
        )

        logger.info(
            f" [BUILD_CONFIG] title model={model}"
        )
        return config

    @staticmethod
    def get_lite_generation_config_with_schema(
        model: str,
        json_schema: dict[str, object],
        temperature: float | None,
        max_tokens: int | None,
        system_instructions: str,
    ) -> types.GenerateContentConfig:
        """Build a lightweight GenerateContentConfig for structured JSON output calls."""
        no_thinking = types.ThinkingConfig(thinking_level=None, include_thoughts=False)
        model_config = get_model_config(model)

        if temperature is not None:
            effective_temperature = temperature
        else:
            effective_temperature = model_config.default_temperature if model_config and model_config.default_temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = max_tokens if max_tokens is not None else (model_config.max_output_tokens if model_config and model_config.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS)
        effective_top_k = model_config.top_k if model_config and model_config.top_k is not None else None
        effective_top_p = model_config.top_p if model_config and model_config.top_p is not None else 1.0

        config = types.GenerateContentConfig(
            temperature=effective_temperature,
            max_output_tokens=effective_max_tokens,
            top_k_filtering=effective_top_k,
            top_p_nucleus_sampling=effective_top_p,
            thinking_config=no_thinking,
            response_format=types.ResponseFormat.from_pydantic_schema(json_schema),  # type: ignore[arg-type]
            system_instructions=system_instructions,
        )

        logger.info(
            f"[BUILD_CONFIG] lite+schema model={model}, max_tokens={effective_max_tokens}, thinking=disabled"
        )
        return config

    @staticmethod
    def get_lite_generation_config_for_json(
        model: str,
        temperature: float | None,
        max_tokens: int | None,
        system_instructions: str,
    ) -> types.GenerateContentConfig:
        """Build a lightweight GenerateContentConfig that requests JSON but doesn't enforce schema.

        More forgiving than get_lite_generation_config_with_schema - suitable for local
        models that may return partial JSON, markdown-wrapped JSON, or plain text.
        The caller is responsible for parsing the response with fallback strategies.
        """
        no_thinking = types.ThinkingConfig(thinking_level=None, include_thoughts=False)
        model_config = get_model_config(model)

        if temperature is not None:
            effective_temperature = temperature
        else:
            effective_temperature = model_config.default_temperature if model_config and model_config.default_temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = max_tokens if max_tokens is not None else (model_config.max_output_tokens if model_config and model_config.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS)
        effective_top_k = model_config.top_k if model_config and model_config.top_k is not None else None
        effective_top_p = model_config.top_p if model_config and model_config.top_p is not None else 1.0

        config = types.GenerateContentConfig(
            temperature=effective_temperature,
            max_output_tokens=effective_max_tokens,
            top_k_filtering=effective_top_k,
            top_p_nucleus_sampling=effective_top_p,
            thinking_config=no_thinking,
            system_instructions=system_instructions,
        )

        logger.info(
            f"[BUILD_CONFIG] lite+json model={model}, max_tokens={effective_max_tokens}, thinking=disabled, schema=flexible"
        )
        return config
