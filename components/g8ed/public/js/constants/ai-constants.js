// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * AI Constants for Frontend
 * LLM model identifiers - must match shared/constants/status.json llm.models
 */

export const LLMProvider = Object.freeze({
    OPENAI:    'openai',
    OLLAMA:    'ollama',
    GEMINI:    'gemini',
    ANTHROPIC: 'anthropic',
});

export const GeminiModel = Object.freeze({
    PRO:         'gemini-3.1-pro',
    FLASH:       'gemini-3.1-flash',
    FLASH_LITE:  'gemini-3.1-flash-lite',
});

export const OpenAIModel = Object.freeze({
    GPT_5_4_THINKING: 'gpt-5.4-thinking',
    GPT_5_4_INSTANT:  'gpt-5.4-instant',
    GPT_5_4_MINI:     'gpt-5.4-mini',
});

export const AnthropicModel = Object.freeze({
    ANTHROPIC_CLAUDE_OPUS_4_6:   'claude-opus-4.6',
    ANTHROPIC_CLAUDE_SONNET_4_6: 'claude-sonnet-4.6',
    ANTHROPIC_CLAUDE_HAIKU_4_5:  'claude-haiku-4.5',
});

export const OllamaModel = Object.freeze({
    QWEN3_5_122B:    'qwen3.5-122b',
    GLM_5_1:         'glm-5.1',
    GEMMA4_26B:      'gemma4-26b',
    NEMOTRON_3_30B:  'nemotron-3-30b',
    LLAMA_3_2_3B:    'llama-3.2-3b',
    QWEN3_5_2B:      'qwen3.5-2b',
});

export const PROVIDER_MODELS = Object.freeze({
    [LLMProvider.GEMINI]: {
        primary: [
            { id: GeminiModel.PRO, label: 'Gemini 3.1 Pro (Frontier Reasoning)' },
        ],
        assistant: [
            { id: GeminiModel.FLASH, label: 'Gemini 3.1 Flash (Balanced)' },
        ],
        lite: [
            { id: GeminiModel.FLASH_LITE, label: 'Gemini 3.1 Flash Lite (Utility)' },
        ],
        defaultPrimary: GeminiModel.PRO,
        defaultAssistant: GeminiModel.FLASH,
        defaultLite: GeminiModel.FLASH_LITE,
    },
    [LLMProvider.ANTHROPIC]: {
        primary: [
            { id: AnthropicModel.ANTHROPIC_CLAUDE_OPUS_4_6, label: 'Claude Opus 4.6 (Frontier Reasoning)' },
        ],
        assistant: [
            { id: AnthropicModel.ANTHROPIC_CLAUDE_SONNET_4_6, label: 'Claude Sonnet 4.6 (Balanced)' },
        ],
        lite: [
            { id: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5, label: 'Claude Haiku 4.5 (Utility)' },
        ],
        defaultPrimary: AnthropicModel.ANTHROPIC_CLAUDE_OPUS_4_6,
        defaultAssistant: AnthropicModel.ANTHROPIC_CLAUDE_SONNET_4_6,
        defaultLite: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5,
    },
    [LLMProvider.OPENAI]: {
        primary: [
            { id: OpenAIModel.GPT_5_4_THINKING, label: 'GPT-5.4 Thinking (Frontier Reasoning)' },
        ],
        assistant: [
            { id: OpenAIModel.GPT_5_4_INSTANT, label: 'GPT-5.4 Instant (Balanced)' },
        ],
        lite: [
            { id: OpenAIModel.GPT_5_4_MINI, label: 'GPT-5.4 Mini (Utility)' },
        ],
        defaultPrimary: OpenAIModel.GPT_5_4_THINKING,
        defaultAssistant: OpenAIModel.GPT_5_4_INSTANT,
        defaultLite: OpenAIModel.GPT_5_4_MINI,
    },
    [LLMProvider.OLLAMA]: {
        primary: [
            { id: OllamaModel.QWEN3_5_122B, label: 'Qwen 3.5 122B (Frontier Reasoning)' },
            { id: OllamaModel.GLM_5_1, label: 'GLM 5.1 (Frontier Reasoning)' },
        ],
        assistant: [
            { id: OllamaModel.GEMMA4_26B, label: 'Gemma 4 26B (Balanced)' },
            { id: OllamaModel.NEMOTRON_3_30B, label: 'Nemotron 3 30B (Balanced)' },
        ],
        lite: [
            { id: OllamaModel.LLAMA_3_2_3B, label: 'Llama 3.2 3B (Utility)' },
            { id: OllamaModel.QWEN3_5_2B, label: 'Qwen 3.5 2B (Utility)' },
        ],
        defaultPrimary: OllamaModel.QWEN3_5_122B,
        defaultAssistant: OllamaModel.GEMMA4_26B,
        defaultLite: OllamaModel.LLAMA_3_2_3B,
    },
});
