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

"""Thinking-level translators.

Application code speaks in canonical ThinkingLevel values (off/minimal/low/
medium/high). Each provider speaks a different dialect for the same concept:

  * Gemini 3+:  config.thinking_config.thinking_level (enum string)
                + include_thoughts toggle
  * OpenAI:     reasoning.effort (enum string), or omit the key entirely
  * Anthropic:  thinking={"type": "enabled", "budget_tokens": N}
  * Ollama:     per-model dialect; NATIVE_TOGGLE uses the chat() think kwarg,
                NONE omits all thinking params.

Translators are pure functions from (level, model_config) to a small typed
result the provider adapter then applies to its outbound request. They must
not mutate either input.
"""

from __future__ import annotations

from dataclasses import dataclass

from app.constants import ThinkingDialect, ThinkingLevel
from app.models.model_configs import (
    ANTHROPIC_DEFAULT_THINKING_BUDGETS,
    LLMModelConfig,
    clamp_thinking_level,
)


# ---------------------------------------------------------------------------
# Gemini
# ---------------------------------------------------------------------------

@dataclass(frozen=True)
class GeminiThinkingTranslation:
    """Result of translating a ThinkingLevel for Gemini 3+ requests.

    thinking_level is the string the google-genai SDK expects (e.g. "high").
    When thinking is OFF the provider should omit the ``thinking_config``
    field entirely — this is signalled by ``enabled=False``.
    """
    enabled: bool
    thinking_level: str | None
    include_thoughts: bool


def translate_for_gemini(
    level: ThinkingLevel,
    config: LLMModelConfig,
    include_thoughts: bool,
) -> GeminiThinkingTranslation:
    clamped = clamp_thinking_level(level, config)
    if clamped is ThinkingLevel.OFF:
        return GeminiThinkingTranslation(
            enabled=False,
            thinking_level=None,
            include_thoughts=False,
        )
    return GeminiThinkingTranslation(
        enabled=True,
        thinking_level=clamped.value,
        include_thoughts=include_thoughts,
    )


# ---------------------------------------------------------------------------
# Anthropic
# ---------------------------------------------------------------------------

@dataclass(frozen=True)
class AnthropicThinkingTranslation:
    """Result of translating a ThinkingLevel for Anthropic Messages API.

    When ``enabled`` is True the provider MUST:
      - set thinking = {"type": "enabled", "budget_tokens": budget_tokens}
      - drop top_k and top_p

    When ``enabled`` is False the provider should omit the thinking key and
    leave sampling params untouched.
    """
    enabled: bool
    budget_tokens: int
    level: ThinkingLevel


def translate_for_anthropic(
    level: ThinkingLevel,
    config: LLMModelConfig,
) -> AnthropicThinkingTranslation:
    clamped = clamp_thinking_level(level, config)
    if clamped is ThinkingLevel.OFF:
        return AnthropicThinkingTranslation(
            enabled=False,
            budget_tokens=0,
            level=ThinkingLevel.OFF,
        )

    per_model = config.thinking_budgets or {}
    if clamped in per_model:
        budget = per_model[clamped]
    else:
        budget = ANTHROPIC_DEFAULT_THINKING_BUDGETS[clamped]

    return AnthropicThinkingTranslation(
        enabled=True,
        budget_tokens=budget,
        level=clamped,
    )


# ---------------------------------------------------------------------------
# OpenAI
# ---------------------------------------------------------------------------

@dataclass(frozen=True)
class OpenAIThinkingTranslation:
    """Result of translating a ThinkingLevel for the OpenAI Chat Completions API.

    reasoning_effort is the string value to place under ``reasoning.effort``.
    When ``enabled`` is False the provider should omit the reasoning key.
    """
    enabled: bool
    reasoning_effort: str | None


def translate_for_openai(
    level: ThinkingLevel,
    config: LLMModelConfig,
) -> OpenAIThinkingTranslation:
    clamped = clamp_thinking_level(level, config)
    if clamped is ThinkingLevel.OFF:
        return OpenAIThinkingTranslation(enabled=False, reasoning_effort=None)
    return OpenAIThinkingTranslation(enabled=True, reasoning_effort=clamped.value)


# ---------------------------------------------------------------------------
# Ollama
# ---------------------------------------------------------------------------

@dataclass(frozen=True)
class OllamaThinkingTranslation:
    """Result of translating a ThinkingLevel for self-hosted Ollama models.

    Ollama hosts a heterogeneous zoo of families. Different families toggle
    reasoning differently; thinking_dialect on the model config selects the
    wire encoding.

    ``think`` is the boolean to pass to ``AsyncClient.chat(think=...)``. It
    is None when the model does not support the kwarg at all (dialect=NONE);
    providers should omit the kwarg in that case.
    """
    enabled: bool
    think: bool | None


def translate_for_ollama(
    level: ThinkingLevel,
    config: LLMModelConfig,
) -> OllamaThinkingTranslation:
    clamped = clamp_thinking_level(level, config)
    # Every Ollama model MUST declare its dialect at registration time (see
    # _OLLAMA_CONFIGS validation in model_configs.py). A None here means the
    # caller supplied a config that was never registered as an Ollama model —
    # fail loudly rather than silently masquerading as "no thinking".
    if config.thinking_dialect is None:
        raise ValueError(
            f"Ollama model {config.name!r} has no thinking_dialect set. "
            "Register the model in _OLLAMA_CONFIGS with an explicit "
            "ThinkingDialect before invoking translate_for_ollama()."
        )
    dialect = config.thinking_dialect

    if dialect == ThinkingDialect.NONE:
        return OllamaThinkingTranslation(enabled=False, think=None)

    if dialect == ThinkingDialect.NATIVE_TOGGLE:
        enabled = clamped != ThinkingLevel.OFF
        return OllamaThinkingTranslation(enabled=enabled, think=enabled)

    raise ValueError(
        f"Unknown ThinkingDialect {dialect!r} for Ollama model "
        f"{config.name!r}. Extend translate_for_ollama() to handle it."
    )
