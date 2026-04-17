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

Thinking capability is encoded exclusively via supported_thinking_levels:
  - []                          => model cannot think; providers omit all
                                    thinking-related request fields.
  - [OFF, LOW, MEDIUM, HIGH]    => thinking is opt-in; OFF disables it.
  - [LOW, MEDIUM, HIGH] (no OFF) => always-on reasoning model; OFF is not a
                                    legal request and callers must supply one
                                    of the listed levels.

thinking_budgets maps each supported ThinkingLevel to a token budget for
providers that require a token count (currently Anthropic). Unset levels
fall back to AnthropicThinkingTranslator's default table.

thinking_dialect selects the on-the-wire encoding for self-hosted (Ollama)
models whose families toggle reasoning differently. Cloud providers ignore it.

Provider-specific behavior (e.g. thought signatures, response_format)
lives in the respective provider adapter under app/llm/providers/.
"""

from pydantic import Field

from app.constants import (
    ANTHROPIC_CLAUDE_HAIKU_4_5,
    ANTHROPIC_CLAUDE_OPUS_4_6,
    ANTHROPIC_CLAUDE_SONNET_4_6,
    ANTHROPIC_DEFAULT_MODEL,
    GEMINI_3_1_PRO,
    GEMINI_3_1_PRO_CUSTOM_TOOLS,
    GEMINI_3_1_FLASH_LITE,
    GEMINI_3_FLASH,
    GEMINI_DEFAULT_MODEL,
    OLLAMA_DEFAULT_MODEL,
    OLLAMA_GEMMA4_26B,
    OLLAMA_GEMMA4_E2B,
    OLLAMA_GEMMA4_E4B,
    OLLAMA_GLM_5_1,
    OLLAMA_LLAMA_3_2_3B,
    OLLAMA_NEMOTRON_3_30B,
    OLLAMA_QWEN3_5_122B,
    OLLAMA_QWEN3_5_2B,
    OPENAI_DEFAULT_MODEL,
    OPENAI_GPT_5_4_MINI,
    THINKING_LEVEL_PRIORITY_ASC,
    ThinkingDialect,
    ThinkingLevel,
)
from app.models.base import G8eBaseModel


class LLMModelConfig(G8eBaseModel):
    """Configuration for an LLM model including capability constraints."""

    name: str
    supported_thinking_levels: list[ThinkingLevel] = Field(default_factory=list)
    supports_image_generation: bool = False
    supports_tools: bool | None = None
    context_window_input: int | None = None
    context_window_output: int | None = None
    default_temperature: float | None = None
    top_k: int | None = None
    top_p: float | None = None
    max_output_tokens: int | None = None
    stop_sequences: list[str] | None = None
    # Anthropic-style providers require a token budget per internal level.
    # When unset the AnthropicThinkingTranslator default table is used.
    thinking_budgets: dict[ThinkingLevel, int] | None = None
    # Ollama-only: which wire dialect the model expects for reasoning toggling.
    thinking_dialect: ThinkingDialect | None = None


# -----------------------------------------------------------------------------
# Default per-level token budgets for Anthropic extended thinking.
# Models may override via LLMModelConfig.thinking_budgets.
# -----------------------------------------------------------------------------
ANTHROPIC_DEFAULT_THINKING_BUDGETS: dict[ThinkingLevel, int] = {
    ThinkingLevel.MINIMAL: 1_024,
    ThinkingLevel.LOW:     2_048,
    ThinkingLevel.MEDIUM:  8_192,
    ThinkingLevel.HIGH:    16_384,
}


# =============================================================================
# Gemini models
# =============================================================================

GEMINI_3_1_PRO_CONFIG = LLMModelConfig(
    name=GEMINI_3_1_PRO,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
    supports_tools=True,
    context_window_input=1_000_000,
    context_window_output=64_000,
    default_temperature=1.0,
    max_output_tokens=64_000,
)

GEMINI_3_1_PRO_CUSTOM_TOOLS_CONFIG = LLMModelConfig(
    name=GEMINI_3_1_PRO_CUSTOM_TOOLS,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
    supports_tools=True,
    context_window_input=1_000_000,
    context_window_output=64_000,
    default_temperature=1.0,
    max_output_tokens=64_000,
)

GEMINI_3_1_FLASH_LITE_CONFIG = LLMModelConfig(
    name=GEMINI_3_1_FLASH_LITE,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.MINIMAL, ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
    supports_tools=True,
    context_window_input=1_000_000,
    context_window_output=64_000,
    default_temperature=1.0,
    max_output_tokens=64_000,
)

GEMINI_3_FLASH_CONFIG = LLMModelConfig(
    name=GEMINI_3_FLASH,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
    supports_tools=True,
    context_window_input=1_000_000,
    context_window_output=64_000,
    default_temperature=1.0,
    max_output_tokens=64_000,
)


# =============================================================================
# Ollama models
#
# Thinking capability per family:
#   - Qwen3, GLM, Nemotron: native `think` toggle; treated as OFF/HIGH binary.
#   - Gemma, Llama: no reasoning support; supported_thinking_levels=[].
# =============================================================================

OLLAMA_QWEN3_5_122B_CONFIG = LLMModelConfig(
    name=OLLAMA_QWEN3_5_122B,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.HIGH],
    thinking_dialect=ThinkingDialect.NATIVE_TOGGLE,
    supports_tools=True,
    context_window_input=256_000,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)

OLLAMA_GLM_5_1_CONFIG = LLMModelConfig(
    name=OLLAMA_GLM_5_1,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.HIGH],
    thinking_dialect=ThinkingDialect.NATIVE_TOGGLE,
    supports_tools=True,
    context_window_input=256_000,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)

OLLAMA_GEMMA4_26B_CONFIG = LLMModelConfig(
    name=OLLAMA_GEMMA4_26B,
    supported_thinking_levels=[],
    thinking_dialect=ThinkingDialect.NONE,
    supports_tools=True,
    context_window_input=128_000,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)

OLLAMA_GEMMA4_E4B_CONFIG = LLMModelConfig(
    name=OLLAMA_GEMMA4_E4B,
    supported_thinking_levels=[],
    thinking_dialect=ThinkingDialect.NONE,
    supports_tools=True,
    context_window_input=32_768,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)

OLLAMA_GEMMA4_E2B_CONFIG = LLMModelConfig(
    name=OLLAMA_GEMMA4_E2B,
    supported_thinking_levels=[],
    thinking_dialect=ThinkingDialect.NONE,
    supports_tools=True,
    context_window_input=32_768,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)

OLLAMA_NEMOTRON_3_30B_CONFIG = LLMModelConfig(
    name=OLLAMA_NEMOTRON_3_30B,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.HIGH],
    thinking_dialect=ThinkingDialect.NATIVE_TOGGLE,
    supports_tools=True,
    context_window_input=128_000,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)

OLLAMA_LLAMA_3_2_3B_CONFIG = LLMModelConfig(
    name=OLLAMA_LLAMA_3_2_3B,
    supported_thinking_levels=[],
    thinking_dialect=ThinkingDialect.NONE,
    supports_tools=True,
    context_window_input=32_768,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)

OLLAMA_QWEN3_5_2B_CONFIG = LLMModelConfig(
    name=OLLAMA_QWEN3_5_2B,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.HIGH],
    thinking_dialect=ThinkingDialect.NATIVE_TOGGLE,
    supports_tools=True,
    context_window_input=32_768,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)


# =============================================================================
# Anthropic models
#
# Claude's extended thinking expects a token budget rather than a level name.
# thinking_budgets overrides the AnthropicThinkingTranslator default table
# when the model benefits from non-default values.
# =============================================================================

ANTHROPIC_CLAUDE_OPUS_4_6_CONFIG = LLMModelConfig(
    name=ANTHROPIC_CLAUDE_OPUS_4_6,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
    thinking_budgets={
        ThinkingLevel.LOW:    4_096,
        ThinkingLevel.MEDIUM: 16_384,
        ThinkingLevel.HIGH:   32_000,
    },
    supports_tools=True,
    context_window_input=200_000,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)

ANTHROPIC_CLAUDE_SONNET_4_6_CONFIG = LLMModelConfig(
    name=ANTHROPIC_CLAUDE_SONNET_4_6,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
    supports_tools=True,
    context_window_input=200_000,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)

ANTHROPIC_CLAUDE_HAIKU_4_5_CONFIG = LLMModelConfig(
    name=ANTHROPIC_CLAUDE_HAIKU_4_5,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.MINIMAL, ThinkingLevel.LOW],
    supports_tools=True,
    context_window_input=200_000,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)

ANTHROPIC_DEFAULT_CONFIG = LLMModelConfig(
    name=ANTHROPIC_DEFAULT_MODEL,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.LOW, ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
    supports_tools=True,
    context_window_input=200_000,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)


# =============================================================================
# OpenAI models
#
# GPT-5 mini exposes reasoning.effort at minimal/low. The generic default
# entry has no declared thinking capability — set supported_thinking_levels
# explicitly on any new model that does.
# =============================================================================

OPENAI_GPT_5_4_MINI_CONFIG = LLMModelConfig(
    name=OPENAI_GPT_5_4_MINI,
    supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.MINIMAL, ThinkingLevel.LOW],
    supports_tools=True,
    context_window_input=200_000,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)

OPENAI_DEFAULT_CONFIG = LLMModelConfig(
    name=OPENAI_DEFAULT_MODEL,
    supported_thinking_levels=[],
    supports_tools=True,
    context_window_input=128_000,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)


# =============================================================================
# Ollama default fallback (unknown model name)
# =============================================================================

OLLAMA_DEFAULT_CONFIG = LLMModelConfig(
    name=OLLAMA_DEFAULT_MODEL,
    supported_thinking_levels=[],
    thinking_dialect=ThinkingDialect.NONE,
    supports_tools=True,
    context_window_input=128_000,
    context_window_output=8_192,
    top_k=40,
    top_p=1.0,
    max_output_tokens=8_192,
)


# =============================================================================
# Thinking-level helpers
# =============================================================================

def _intensity_levels(config: "LLMModelConfig") -> list[ThinkingLevel]:
    """Return supported levels excluding OFF, in registry-declared order."""
    return [lvl for lvl in config.supported_thinking_levels if lvl is not ThinkingLevel.OFF]


def _lowest_thinking_level(config: "LLMModelConfig") -> ThinkingLevel | None:
    levels = _intensity_levels(config)
    if not levels:
        return None
    for level in THINKING_LEVEL_PRIORITY_ASC:
        if level in levels:
            return level
    return None


def _highest_thinking_level(config: "LLMModelConfig") -> ThinkingLevel | None:
    levels = _intensity_levels(config)
    if not levels:
        return None
    for level in reversed(THINKING_LEVEL_PRIORITY_ASC):
        if level in levels:
            return level
    return None


def clamp_thinking_level(desired: ThinkingLevel, config: "LLMModelConfig") -> ThinkingLevel:
    """Clamp a desired ThinkingLevel to what the given model supports.

    Rules:
      - If the model has no thinking capability (empty list), return OFF.
      - If OFF is desired and OFF is supported, return OFF.
      - If OFF is desired but the model is always-on reasoning, return the
        lowest supported intensity (caller asked for off, but model cannot).
      - Otherwise, return the highest supported level <= desired. If none
        exists below desired, return the lowest supported level.
    """
    if not config.supported_thinking_levels:
        return ThinkingLevel.OFF

    if desired is ThinkingLevel.OFF:
        if ThinkingLevel.OFF in config.supported_thinking_levels:
            return ThinkingLevel.OFF
        lowest = _lowest_thinking_level(config)
        return lowest if lowest is not None else ThinkingLevel.OFF

    intensity_order = list(THINKING_LEVEL_PRIORITY_ASC)
    desired_rank = intensity_order.index(desired)
    supported_intensity = _intensity_levels(config)

    # Walk desired -> lower looking for the highest supported level not
    # exceeding desired.
    for rank in range(desired_rank, -1, -1):
        candidate = intensity_order[rank]
        if candidate in supported_intensity:
            return candidate

    # desired was below everything supported; fall back to the lowest
    # supported level.
    lowest = _lowest_thinking_level(config)
    return lowest if lowest is not None else ThinkingLevel.OFF


# Module-level singleton returned for unknown model names. Using a shared
# constant (instead of fabricating a fresh LLMModelConfig per call) guarantees
# stable identity across callers and prevents accidental per-call mutation
# leaking between tests. Its name is a sentinel ("unknown") rather than the
# requested model name because the config is meant to be opaque — callers that
# need the real model string must resolve it from their own context.
UNKNOWN_MODEL_CONFIG = LLMModelConfig(
    name="unknown",
    supported_thinking_levels=[],
    supports_tools=True,
    context_window_input=128_000,
    context_window_output=8_192,
)


class LLMModelRegistry(G8eBaseModel):
    """Registry of all known LLM model configurations."""

    configs: list[LLMModelConfig] = Field(default_factory=list)

    def get(self, model_name: str | None) -> LLMModelConfig:
        """Return the config for a model, or the shared UNKNOWN_MODEL_CONFIG.

        Unknown-model fallback assumes no thinking capability; operators that
        need thinking on custom models must register a proper config. Returning
        the shared constant (rather than a fresh object) preserves identity so
        callers can safely compare, cache, or monkeypatch registry entries.
        """
        if not model_name:
            return UNKNOWN_MODEL_CONFIG
        for config in self.configs:
            if config.name == model_name:
                return config
        return UNKNOWN_MODEL_CONFIG

    def available_models(self) -> list[str]:
        """Return list of all registered model names."""
        return [cfg.name for cfg in self.configs]

    def get_lowest_thinking_level(self, model_name: str) -> ThinkingLevel | None:
        """Return the least expensive thinking level the model supports, or None."""
        return _lowest_thinking_level(self.get(model_name))

    def get_highest_thinking_level(self, model_name: str) -> ThinkingLevel | None:
        """Return the most capable thinking level the model supports, or None."""
        return _highest_thinking_level(self.get(model_name))


MODEL_REGISTRY = LLMModelRegistry(configs=[
    ANTHROPIC_CLAUDE_OPUS_4_6_CONFIG,
    ANTHROPIC_CLAUDE_SONNET_4_6_CONFIG,
    ANTHROPIC_CLAUDE_HAIKU_4_5_CONFIG,
    ANTHROPIC_DEFAULT_CONFIG,
    OPENAI_GPT_5_4_MINI_CONFIG,
    OPENAI_DEFAULT_CONFIG,
    OLLAMA_QWEN3_5_122B_CONFIG,
    OLLAMA_GLM_5_1_CONFIG,
    OLLAMA_GEMMA4_26B_CONFIG,
    OLLAMA_GEMMA4_E4B_CONFIG,
    OLLAMA_GEMMA4_E2B_CONFIG,
    OLLAMA_NEMOTRON_3_30B_CONFIG,
    OLLAMA_LLAMA_3_2_3B_CONFIG,
    OLLAMA_QWEN3_5_2B_CONFIG,
    OLLAMA_DEFAULT_CONFIG,
    GEMINI_3_1_PRO_CONFIG,
    GEMINI_3_1_PRO_CUSTOM_TOOLS_CONFIG,
    GEMINI_3_1_FLASH_LITE_CONFIG,
    GEMINI_3_FLASH_CONFIG,
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
