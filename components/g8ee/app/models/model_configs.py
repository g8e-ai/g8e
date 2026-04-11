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

"""LLM Model Configurations

Provider-agnostic model configuration registry. Each model entry describes
capabilities and constraints (context window, thinking support, output limits)
that the request builder and agent use to configure generation requests.

Provider-specific behavior (e.g. thought signatures, response_format)
lives in the respective provider adapter under app/llm/providers/.
"""

from pydantic import Field

from app.constants import (
    GEMINI_3_1_PRO_PREVIEW,
    GEMINI_3_1_FLASH_LITE_PREVIEW,
    GEMINI_3_FLASH_PREVIEW,
    GEMMA3_1B,
    GEMMA3_4B,
    GEMMA3_12B,
    GEMMA3_27B,
    GEMMA4_E4B,
    GEMMA4_E2B,
    OPENAI_DEFAULT_MODEL,
    OLLAMA_DEFAULT_MODEL,
    ANTHROPIC_DEFAULT_MODEL,
    GEMINI_DEFAULT_MODEL,
    OPENAI_GPT_4O,
    OPENAI_GPT_4O_MINI,
    OPENAI_GPT_4_TURBO,
    OPENAI_GPT_3_5_TURBO,
    OLLAMA_LLAMA3_8B,
    OLLAMA_LLAMA3_70B,
    OLLAMA_CODELLAMA_7B,
    OLLAMA_MISTRAL_7B,
    QWEN25_7B,
    QWEN25_14B,
    QWEN3_1B7,
    QWEN3_CODER_30B,
    ThinkingLevel,
)
from app.models.base import G8eBaseModel


class LLMModelConfig(G8eBaseModel):
    """Configuration for an LLM model including capability constraints."""

    name: str
    supported_thinking_levels: list[ThinkingLevel] = Field(default_factory=list)
    supports_image_generation: bool = False
    supports_thinking: bool = True
    supports_tools: bool = True
    context_window_input: int = 1_000_000
    context_window_output: int = 8_192


# Google Gemma 3 models (Ollama)
GEMINI_3_PRO_PREVIEW = LLMModelConfig(
    name=GEMINI_3_1_PRO_PREVIEW,
    supported_thinking_levels=[ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
    supports_thinking=True,
    context_window_input=1_000_000,
    context_window_output=64_000,
)

GEMINI_3_FLASH_PREVIEW = LLMModelConfig(
    name=GEMINI_3_FLASH_PREVIEW,
    supported_thinking_levels=[ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
    supports_thinking=True,
    context_window_input=1_000_000,
    context_window_output=64_000,
)

GEMINI_3_1_FLASH_LITE_PREVIEW_CONFIG = LLMModelConfig(
    name=GEMINI_3_1_FLASH_LITE_PREVIEW,
    supported_thinking_levels=[ThinkingLevel.MINIMAL, ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
    supports_thinking=True,
    context_window_input=1_000_000,
    context_window_output=64_000,
)

GEMMA3_27B = LLMModelConfig(
    name=GEMMA3_27B,
    supported_thinking_levels=[],
    supports_thinking=True,
    supports_tools=True,
    context_window_input=128_000,
    context_window_output=8_192,
)

GEMMA3_12B = LLMModelConfig(
    name=GEMMA3_12B,
    supported_thinking_levels=[],
    supports_thinking=True,
    supports_tools=True,
    context_window_input=128_000,
    context_window_output=8_192,
)

GEMMA3_4B = LLMModelConfig(
    name=GEMMA3_4B,
    supported_thinking_levels=[],
    supports_thinking=True,
    supports_tools=True,
    context_window_input=128_000,
    context_window_output=8_192,
)

GEMMA3_1B = LLMModelConfig(
    name=GEMMA3_1B,
    supported_thinking_levels=[],
    supports_thinking=True,
    supports_tools=True,
    context_window_input=32_768,
    context_window_output=8_192,
)

# Gemma 4 models (Ollama) - support thinking
GEMMA4_E4B_CONFIG = LLMModelConfig(
    name=GEMMA4_E4B,
    supported_thinking_levels=[ThinkingLevel.MINIMAL, ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
    supports_thinking=True,
    supports_tools=True,
    context_window_input=128_000,
    context_window_output=8_192,
)

GEMMA4_E2B_CONFIG = LLMModelConfig(
    name=GEMMA4_E2B,
    supported_thinking_levels=[ThinkingLevel.MINIMAL, ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
    supports_thinking=True,
    supports_tools=True,
    context_window_input=128_000,
    context_window_output=8_192,
)

# Qwen 3 Coder models (Ollama) — agentic coding, MoE architecture, native tool calling
QWEN3_CODER_30B = LLMModelConfig(
    name=QWEN3_CODER_30B,
    supported_thinking_levels=[],
    supports_thinking=True,
    supports_tools=True,
    context_window_input=256_000,
    context_window_output=8_192,
)

# Qwen 3 models (Ollama) — native tool calling support across all sizes
QWEN3_1B7 = LLMModelConfig(
    name=QWEN3_1B7,
    supported_thinking_levels=[],
    supports_thinking=True,
    supports_tools=True,
    context_window_input=32_768,
    context_window_output=8_192,
)

# Qwen 2.5 models (Ollama) — strong tool calling support
QWEN25_14B = LLMModelConfig(
    name=QWEN25_14B,
    supported_thinking_levels=[],
    supports_thinking=True,
    supports_tools=True,
    context_window_input=32_768,
    context_window_output=8_192,
)

QWEN25_7B = LLMModelConfig(
    name=QWEN25_7B,
    supported_thinking_levels=[],
    supports_thinking=True,
    supports_tools=True,
    context_window_input=32_768,
    context_window_output=8_192,
)

# OpenAI models
OPENAI_DEFAULT_CONFIG = LLMModelConfig(
    name=OPENAI_DEFAULT_MODEL,
    supported_thinking_levels=[],
    supports_thinking=True,
    supports_tools=True,
    context_window_input=128_000,
    context_window_output=8_192,
)

# Ollama models
OLLAMA_DEFAULT_CONFIG = LLMModelConfig(
    name=OLLAMA_DEFAULT_MODEL,
    supported_thinking_levels=[],
    supports_thinking=True,
    supports_tools=True,
    context_window_input=128_000,
    context_window_output=8_192,
)


_THINKING_LEVEL_PRIORITY_ASC = [ThinkingLevel.MINIMAL, ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH]
_THINKING_LEVEL_PRIORITY_DESC = [ThinkingLevel.HIGH, ThinkingLevel.MEDIUM, ThinkingLevel.LOW, ThinkingLevel.MINIMAL]


def _lowest_thinking_level(config: "LLMModelConfig") -> "ThinkingLevel | None":
    if not config.supports_thinking or not config.supported_thinking_levels:
        return None
    levels = config.supported_thinking_levels
    for level in _THINKING_LEVEL_PRIORITY_ASC:
        if level.value in levels or level in levels:
            return level
    return None


def _highest_thinking_level(config: "LLMModelConfig") -> "ThinkingLevel | None":
    if not config.supports_thinking or not config.supported_thinking_levels:
        return None
    levels = config.supported_thinking_levels
    for level in _THINKING_LEVEL_PRIORITY_DESC:
        if level.value in levels or level in levels:
            return level
    return None


class LLMModelRegistry(G8eBaseModel):
    """Registry of all known LLM model configurations."""

    configs: list[LLMModelConfig] = Field(default_factory=list)

    def get(self, model_name: str | None) -> LLMModelConfig:
        """Return the config for a model, or a safe default for unknown models."""
        if not model_name:
            # Fallback to a safe default if no model name is provided
            return LLMModelConfig(
                name="unknown",
                supported_thinking_levels=[],
                supports_thinking=True,
                supports_tools=True,
                context_window_input=128_000,
                context_window_output=8_192,
            )
        for config in self.configs:
            if config.name == model_name:
                return config
        return LLMModelConfig(
            name=model_name,
            supported_thinking_levels=[],
            supports_thinking=True,
            supports_tools=True,
            context_window_input=128_000,
            context_window_output=8_192,
        )

    def available_models(self) -> list[str]:
        """Return list of all registered model names."""
        return [cfg.name for cfg in self.configs]

    def get_lowest_thinking_level(self, model_name: str) -> ThinkingLevel | None:
        """Return the least expensive thinking level the model supports, or None."""
        config = self.get(model_name)
        return _lowest_thinking_level(config)

    def get_highest_thinking_level(self, model_name: str) -> ThinkingLevel | None:
        """Return the most capable thinking level the model supports, or None."""
        config = self.get(model_name)
        return _highest_thinking_level(config)


MODEL_REGISTRY = LLMModelRegistry(configs=[
    OPENAI_DEFAULT_CONFIG,
    OLLAMA_DEFAULT_CONFIG,
    GEMINI_3_PRO_PREVIEW,
    GEMINI_3_FLASH_PREVIEW,
    GEMINI_3_1_FLASH_LITE_PREVIEW_CONFIG,
    GEMMA3_27B,
    GEMMA3_12B,
    GEMMA3_4B,
    GEMMA3_1B,
    GEMMA4_E4B_CONFIG,
    GEMMA4_E2B_CONFIG,
    QWEN3_CODER_30B,
    QWEN3_1B7,
    QWEN25_14B,
    QWEN25_7B,
])


def get_model_config(model_name: str | None) -> LLMModelConfig:
    """Get the configuration for a model by name."""
    return MODEL_REGISTRY.get(model_name)


def get_lowest_thinking_level(model_name: str | None) -> ThinkingLevel | None:
    """Get the lowest supported thinking level for a model by name."""
    return _lowest_thinking_level(MODEL_REGISTRY.get(model_name))


def get_highest_thinking_level(model_name: str | None) -> ThinkingLevel | None:
    """Get the highest supported thinking level for a model by name."""
    return _highest_thinking_level(MODEL_REGISTRY.get(model_name))


def thinking_level_for_config(config: LLMModelConfig) -> ThinkingLevel | None:
    """Get the highest supported thinking level directly from a model config."""
    return _highest_thinking_level(config)


def get_available_models() -> list[str]:
    """Get list of all registered model names."""
    return MODEL_REGISTRY.available_models()

