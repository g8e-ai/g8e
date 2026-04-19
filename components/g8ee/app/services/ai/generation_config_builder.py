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

from app.constants import LLM_DEFAULT_MAX_OUTPUT_TOKENS, ThinkingLevel
import app.llm.llm_types as types
from app.llm.llm_types import (
    AssistantLLMSettings,
    LiteLLMSettings,
    PrimaryLLMSettings,
)
from app.models.model_configs import clamp_thinking_level, get_model_config, LLMModelConfig

logger = logging.getLogger(__name__)


class AIGenerationConfigBuilder:
    """Stateless factory for GenerateContentConfig objects.

    All methods are static — no instance required. Used directly by services
    that build configs without needing AIRequestBuilder's tool/attachment state.
    """

    @staticmethod
    def _get_effective_max_tokens(
        model_config: LLMModelConfig | None,
        max_tokens: int | None,
    ) -> int:
        """Get effective max_tokens value with fallback to model config or default."""
        if max_tokens is not None:
            return max_tokens
        return (
            model_config.max_output_tokens
            if model_config and model_config.max_output_tokens is not None
            else LLM_DEFAULT_MAX_OUTPUT_TOKENS
        )

    @staticmethod
    def _get_effective_values(
        model_config: LLMModelConfig | None,
        max_tokens: int | None,
    ) -> tuple[int, int | None, float | None, list[str] | None]:
        """Get all effective values from model config with fallbacks.

        Returns:
            (max_tokens, top_k, top_p, stop_sequences)
        """
        return (
            AIGenerationConfigBuilder._get_effective_max_tokens(model_config, max_tokens),
            model_config.top_k if model_config else None,
            model_config.top_p if model_config else None,
            model_config.stop_sequences if model_config else None,
        )

    @staticmethod
    def _extract_tool_names(tools: list[types.ToolGroup]) -> list[str]:
        """Extract tool names from tool groups for logging."""
        tool_names = []
        for t in (tools or []):
            for fd in (t.tools or []):
                tool_names.append(getattr(fd, 'name', '?'))
        return tool_names

    @staticmethod
    def _build_thinking_config(
        model_name: str,
        desired_level: ThinkingLevel = ThinkingLevel.HIGH,
        include_thoughts: bool = True,
    ) -> types.ThinkingConfig:
        """Build ThinkingConfig based on model capabilities.

        The caller declares a desired ThinkingLevel; we clamp it to the
        closest level the bound model actually supports. When the model has
        no thinking capability the clamp returns ThinkingLevel.OFF and
        include_thoughts is forced False.

        Default desired_level is HIGH so primary/tool-capable model calls
        continue to opt into the highest reasoning tier the model exposes,
        matching previous behavior for the High Reasoning Agent.
        """
        config = get_model_config(model_name) if model_name else None
        if config is None:
            logger.info("[CONFIG] ThinkingConfig: disabled (no model bound)")
            return types.ThinkingConfig(
                thinking_level=ThinkingLevel.OFF,
                include_thoughts=False,
            )

        clamped = clamp_thinking_level(desired_level, config)
        if clamped is ThinkingLevel.OFF:
            logger.info(
                f"[CONFIG] ThinkingConfig: disabled "
                f"(model={config.name} has no supported thinking levels)"
            )
            return types.ThinkingConfig(
                thinking_level=ThinkingLevel.OFF,
                include_thoughts=False,
            )

        if clamped is not desired_level:
            logger.info(
                f"[CONFIG] ThinkingConfig: clamped desired={desired_level} -> {clamped} "
                f"(model={config.name})"
            )
        logger.info(
            f"[CONFIG] ThinkingConfig: thinking_level={clamped}, "
            f"include_thoughts={include_thoughts} (model={config.name})"
        )
        return types.ThinkingConfig(
            thinking_level=clamped,
            include_thoughts=include_thoughts,
        )

    @staticmethod
    def build_primary_settings(
        model: str,
        
        max_tokens: int | None,
        system_instructions: str,
        tools: list[types.ToolGroup],
    ) -> PrimaryLLMSettings:
        """Build PrimaryLLMSettings for main-model calls."""
        thinking_config = AIGenerationConfigBuilder._build_thinking_config(model_name=model)
        model_config = get_model_config(model)

        effective_max_tokens, effective_top_k, effective_top_p, stop_sequences = (
            AIGenerationConfigBuilder._get_effective_values(model_config, max_tokens)
        )

        settings = PrimaryLLMSettings(
            max_output_tokens=effective_max_tokens,
            top_k_filtering=effective_top_k,
            top_p_nucleus_sampling=effective_top_p,
            thinking_config=thinking_config,
            tools=tools,
            system_instructions=system_instructions,
            stop_sequences=stop_sequences,
            response_modalities=["TEXT"],
            tool_config=types.ToolConfig(tool_calling_config=types.ToolCallingConfig(mode="AUTO")),
        )

        thinking_level = getattr(thinking_config, "thinking_level", None)
        tool_names = AIGenerationConfigBuilder._extract_tool_names(tools)
        logger.info(
            f" [BUILD_CONFIG] primary model={model}, max_tokens={effective_max_tokens}, "
            f"thinking_level={thinking_level}, tools_count={len(tools) if tools else 0}, "
            f"tool_names={tool_names}"
        )
        return settings

    @staticmethod
    def build_assistant_settings(
        model: str,
        
        max_tokens: int | None,
        system_instructions: str,
        response_format: types.ResponseFormat | None = None,
    ) -> AssistantLLMSettings:
        """Build AssistantLLMSettings for analysis calls."""
        model_config = get_model_config(model)

        effective_max_tokens, effective_top_k, effective_top_p, stop_sequences = (
            AIGenerationConfigBuilder._get_effective_values(model_config, max_tokens)
        )

        settings = AssistantLLMSettings(
            max_output_tokens=effective_max_tokens,
            top_k_filtering=effective_top_k,
            top_p_nucleus_sampling=effective_top_p,
            system_instructions=system_instructions,
            response_format=response_format,
            stop_sequences=stop_sequences,
        )

        logger.info(
            f" [BUILD_CONFIG] assistant model={model}, max_tokens={effective_max_tokens}"
        )
        return settings

    @staticmethod
    def build_lite_settings(
        model: str,
        
        max_tokens: int | None,
        system_instructions: str,
        response_format: types.ResponseFormat | None = None,
    ) -> LiteLLMSettings:
        """Build LiteLLMSettings for stateless analysis calls.

        Used by triage, memory updates, risk analysis, and error analysis.
        """
        model_config = get_model_config(model)

        effective_max_tokens, effective_top_k, effective_top_p, stop_sequences = (
            AIGenerationConfigBuilder._get_effective_values(model_config, max_tokens)
        )

        return LiteLLMSettings(
            max_output_tokens=effective_max_tokens,
            top_k_filtering=effective_top_k,
            top_p_nucleus_sampling=effective_top_p,
            stop_sequences=stop_sequences,
            system_instructions=system_instructions,
            response_format=response_format,
        )

        logger.info(
            f" [BUILD_CONFIG] lite model={model}, max_tokens={effective_max_tokens}"
        )

    @staticmethod
    def build_config(
        model: str,
        
        max_tokens: int | None,
        system_instructions: str,
        tools: list[types.ToolGroup],
    ) -> types.GenerateContentConfig:
        """Build GenerateContentConfig for main-model calls with tools."""
        thinking_config = AIGenerationConfigBuilder._build_thinking_config(model_name=model)
        model_config = get_model_config(model)

        effective_max_tokens, effective_top_k, effective_top_p, stop_sequences = (
            AIGenerationConfigBuilder._get_effective_values(model_config, max_tokens)
        )

        config = types.GenerateContentConfig(
            max_output_tokens=effective_max_tokens,
            top_k_filtering=effective_top_k,
            top_p_nucleus_sampling=effective_top_p,
            thinking_config=thinking_config,
            tools=tools,
            system_instructions=system_instructions,
            stop_sequences=stop_sequences,
        )

        thinking_level = getattr(thinking_config, "thinking_level", None)
        tool_names = AIGenerationConfigBuilder._extract_tool_names(tools)
        logger.info(
            f" [BUILD_CONFIG] model={model}, max_tokens={effective_max_tokens}, "
            f"thinking_level={thinking_level}, tools_count={len(tools) if tools else 0}, "
            f"tool_names={tool_names}"
        )
        return config

    @staticmethod
    def get_lite_generation_config(
        model: str,
        
        max_tokens: int | None,
        system_instructions: str,
    ) -> types.GenerateContentConfig:
        """Build a lightweight GenerateContentConfig for stateless analysis calls.

        Used by triage, memory updates, risk analysis, and error analysis.
        Thinking is explicitly disabled — these calls expect concise, parseable
        text output and must never emit thought tokens.
        """
        no_thinking = types.ThinkingConfig(thinking_level=ThinkingLevel.OFF, include_thoughts=False)
        model_config = get_model_config(model)

        effective_max_tokens, effective_top_k, effective_top_p, _ = (
            AIGenerationConfigBuilder._get_effective_values(model_config, max_tokens)
        )

        config = types.GenerateContentConfig(
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

        effective_max_tokens, _, effective_top_p, stop_sequences = (
            AIGenerationConfigBuilder._get_effective_values(model_config, None)
        )

        config = types.GenerateContentConfig(
            max_output_tokens=effective_max_tokens,
            top_p_nucleus_sampling=effective_top_p,
            stop_sequences=stop_sequences,
            system_instructions="",
        )

        logger.info(f" [BUILD_CONFIG] title model={model}")
        return config
