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

"""Unit tests for the thinking-level translator layer.

Covers:
  * clamp_thinking_level rules — empty support, OFF handling, downward clamp,
    lowest-fallback when desired is below everything supported.
  * Each of the four per-provider translators:
      - translate_for_gemini
      - translate_for_anthropic (default + per-model budget tables)
      - translate_for_openai
      - translate_for_ollama (dialect dispatch: NONE omits, NATIVE_TOGGLE emits)

Translators are pure functions: given (level, model_config), they produce a
typed result. Tests avoid any provider SDK mocking.
"""

import pytest

from app.constants import ThinkingDialect, ThinkingLevel
from app.llm.thinking import (
    translate_for_anthropic,
    translate_for_gemini,
    translate_for_ollama,
    translate_for_openai,
)
from app.models.model_configs import (
    ANTHROPIC_CLAUDE_HAIKU_4_5_CONFIG,
    ANTHROPIC_CLAUDE_OPUS_4_6_CONFIG,
    ANTHROPIC_CLAUDE_SONNET_4_6_CONFIG,
    ANTHROPIC_DEFAULT_THINKING_BUDGETS,
    GEMINI_3_1_FLASH_LITE_CONFIG,
    GEMINI_3_FLASH_CONFIG,
    LLMModelConfig,
    OLLAMA_LLAMA_3_2_3B_CONFIG,
    OLLAMA_QWEN3_5_122B_CONFIG,
    OPENAI_DEFAULT_CONFIG,
    OPENAI_GPT_5_4_MINI_CONFIG,
    clamp_thinking_level,
)

pytestmark = [pytest.mark.unit]


# ---------------------------------------------------------------------------
# clamp_thinking_level
# ---------------------------------------------------------------------------


class TestClampThinkingLevel:
    """Matrix coverage of the clamp rules documented on clamp_thinking_level."""

    def test_empty_support_returns_off(self):
        """A model with no declared thinking levels always maps to OFF,
        regardless of desired level — the caller asked for something the
        model cannot provide."""
        cfg = LLMModelConfig(name="no-thinking", supported_thinking_levels=[])
        for desired in ThinkingLevel:
            assert clamp_thinking_level(desired, cfg) is ThinkingLevel.OFF

    def test_off_desired_returns_off_when_supported(self):
        """Sonnet declares OFF; asking for OFF must return OFF."""
        assert clamp_thinking_level(ThinkingLevel.OFF, ANTHROPIC_CLAUDE_SONNET_4_6_CONFIG) is ThinkingLevel.OFF

    def test_off_desired_on_always_on_model_returns_lowest(self):
        """An always-on model (no OFF in supported list) must still return a
        valid non-OFF level when the caller asks for OFF."""
        always_on = LLMModelConfig(
            name="always-on",
            supported_thinking_levels=[ThinkingLevel.MEDIUM, ThinkingLevel.HIGH],
        )
        assert clamp_thinking_level(ThinkingLevel.OFF, always_on) is ThinkingLevel.MEDIUM

    def test_high_clamps_down_to_supported_high(self):
        """Gemini 3 Flash supports HIGH; no clamp needed."""
        assert clamp_thinking_level(ThinkingLevel.HIGH, GEMINI_3_FLASH_CONFIG) is ThinkingLevel.HIGH

    def test_high_clamps_down_to_low_when_high_unsupported(self):
        """Haiku supports only MINIMAL and LOW; HIGH must clamp to LOW."""
        assert clamp_thinking_level(ThinkingLevel.HIGH, ANTHROPIC_CLAUDE_HAIKU_4_5_CONFIG) is ThinkingLevel.LOW

    def test_minimal_clamps_up_to_low_when_minimal_unsupported(self):
        """Sonnet's supported intensity range starts at LOW; a MINIMAL request
        falls back to the model's lowest supported level (LOW). The translator
        docstring calls this the 'below-everything-supported' branch."""
        assert clamp_thinking_level(ThinkingLevel.MINIMAL, ANTHROPIC_CLAUDE_SONNET_4_6_CONFIG) is ThinkingLevel.LOW

    def test_medium_stays_medium_on_sonnet(self):
        assert clamp_thinking_level(ThinkingLevel.MEDIUM, ANTHROPIC_CLAUDE_SONNET_4_6_CONFIG) is ThinkingLevel.MEDIUM


# ---------------------------------------------------------------------------
# translate_for_gemini
# ---------------------------------------------------------------------------


class TestTranslateForGemini:

    def test_off_disables_and_omits_thoughts(self):
        translation = translate_for_gemini(
            ThinkingLevel.OFF,
            GEMINI_3_FLASH_CONFIG,
            include_thoughts=True,
        )
        # include_thoughts must be forced False when thinking is off: the
        # Gemini provider uses (enabled=False AND include_thoughts=False) as
        # the signal to drop the thinking_config key entirely.
        assert translation.enabled is False
        assert translation.thinking_level is None
        assert translation.include_thoughts is False

    def test_high_enables_and_preserves_thoughts(self):
        translation = translate_for_gemini(
            ThinkingLevel.HIGH,
            GEMINI_3_FLASH_CONFIG,
            include_thoughts=True,
        )
        assert translation.enabled is True
        assert translation.thinking_level == "high"
        assert translation.include_thoughts is True

    def test_unsupported_level_clamps(self):
        """Flash does not support MINIMAL; translator clamps to LOW."""
        translation = translate_for_gemini(
            ThinkingLevel.MINIMAL,
            GEMINI_3_FLASH_CONFIG,
            include_thoughts=False,
        )
        assert translation.enabled is True
        assert translation.thinking_level == "low"

    def test_minimal_passes_through_on_flash_lite(self):
        """Flash-Lite declares MINIMAL; it should not clamp."""
        translation = translate_for_gemini(
            ThinkingLevel.MINIMAL,
            GEMINI_3_1_FLASH_LITE_CONFIG,
            include_thoughts=False,
        )
        assert translation.thinking_level == "minimal"


# ---------------------------------------------------------------------------
# translate_for_anthropic
# ---------------------------------------------------------------------------


class TestTranslateForAnthropic:

    def test_off_disables(self):
        translation = translate_for_anthropic(
            ThinkingLevel.OFF,
            ANTHROPIC_CLAUDE_SONNET_4_6_CONFIG,
        )
        assert translation.enabled is False
        assert translation.budget_tokens == 0
        assert translation.level is ThinkingLevel.OFF

    def test_default_table_used_when_no_override(self):
        """Sonnet declares no thinking_budgets map; defaults apply."""
        translation = translate_for_anthropic(
            ThinkingLevel.HIGH,
            ANTHROPIC_CLAUDE_SONNET_4_6_CONFIG,
        )
        assert translation.enabled is True
        assert translation.budget_tokens == ANTHROPIC_DEFAULT_THINKING_BUDGETS[ThinkingLevel.HIGH]
        assert translation.level is ThinkingLevel.HIGH

    def test_per_model_override_wins_over_default(self):
        """Opus declares thinking_budgets and must use its override."""
        translation = translate_for_anthropic(
            ThinkingLevel.HIGH,
            ANTHROPIC_CLAUDE_OPUS_4_6_CONFIG,
        )
        assert translation.budget_tokens == 32_000

    def test_default_table_applies_for_uncovered_override_level(self):
        """Opus declares LOW/MEDIUM/HIGH overrides but not MINIMAL. Asking for
        MINIMAL clamps to LOW (since Opus does not support MINIMAL either),
        and LOW is in the override table."""
        translation = translate_for_anthropic(
            ThinkingLevel.MINIMAL,
            ANTHROPIC_CLAUDE_OPUS_4_6_CONFIG,
        )
        assert translation.level is ThinkingLevel.LOW
        assert translation.budget_tokens == 4_096

    def test_haiku_minimal_uses_default_table(self):
        translation = translate_for_anthropic(
            ThinkingLevel.MINIMAL,
            ANTHROPIC_CLAUDE_HAIKU_4_5_CONFIG,
        )
        assert translation.level is ThinkingLevel.MINIMAL
        assert translation.budget_tokens == ANTHROPIC_DEFAULT_THINKING_BUDGETS[ThinkingLevel.MINIMAL]


# ---------------------------------------------------------------------------
# translate_for_openai
# ---------------------------------------------------------------------------


class TestTranslateForOpenAI:

    def test_off_disables(self):
        translation = translate_for_openai(
            ThinkingLevel.OFF,
            OPENAI_GPT_5_4_MINI_CONFIG,
        )
        assert translation.enabled is False
        assert translation.reasoning_effort is None

    def test_minimal_emits_effort_string(self):
        translation = translate_for_openai(
            ThinkingLevel.MINIMAL,
            OPENAI_GPT_5_4_MINI_CONFIG,
        )
        assert translation.enabled is True
        assert translation.reasoning_effort == "minimal"

    def test_unsupported_level_on_mini_clamps_to_low(self):
        """gpt-5.4-mini supports only OFF/MINIMAL/LOW. HIGH must clamp to LOW."""
        translation = translate_for_openai(
            ThinkingLevel.HIGH,
            OPENAI_GPT_5_4_MINI_CONFIG,
        )
        assert translation.reasoning_effort == "low"

    def test_default_openai_model_has_no_reasoning(self):
        """The generic OpenAI default config declares no thinking capability;
        any desired level must yield enabled=False."""
        translation = translate_for_openai(
            ThinkingLevel.HIGH,
            OPENAI_DEFAULT_CONFIG,
        )
        assert translation.enabled is False
        assert translation.reasoning_effort is None


# ---------------------------------------------------------------------------
# translate_for_ollama
# ---------------------------------------------------------------------------


class TestTranslateForOllama:

    def test_none_dialect_always_omits_think(self):
        """Llama has dialect=NONE; no think kwarg under any circumstance."""
        for level in ThinkingLevel:
            translation = translate_for_ollama(level, OLLAMA_LLAMA_3_2_3B_CONFIG)
            assert translation.enabled is False
            assert translation.think is None, f"dialect=NONE must omit think for level={level}"

    def test_native_toggle_emits_true_when_thinking_requested(self):
        """Qwen3 has dialect=NATIVE_TOGGLE; HIGH must send think=True."""
        translation = translate_for_ollama(
            ThinkingLevel.HIGH,
            OLLAMA_QWEN3_5_122B_CONFIG,
        )
        assert translation.enabled is True
        assert translation.think is True

    def test_native_toggle_emits_false_when_off(self):
        """Qwen3 + OFF must send think=False to explicitly disable."""
        translation = translate_for_ollama(
            ThinkingLevel.OFF,
            OLLAMA_QWEN3_5_122B_CONFIG,
        )
        assert translation.enabled is False
        assert translation.think is False

    def test_missing_dialect_raises_loudly(self):
        """A config without a thinking_dialect must fail loudly.

        A silent fallback to NONE would bake "no reasoning" into a new
        Ollama model forever. The contract now is: every Ollama model
        registers its dialect explicitly; unregistered configs blow up.
        """
        cfg = LLMModelConfig(
            name="weird",
            supported_thinking_levels=[ThinkingLevel.OFF, ThinkingLevel.HIGH],
            thinking_dialect=None,
        )
        with pytest.raises(ValueError, match="thinking_dialect"):
            translate_for_ollama(ThinkingLevel.HIGH, cfg)
