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
    PRO:         'gemini-3.1-pro-preview',
    FLASH:       'gemini-3-flash-preview',
    FLASH_LITE:  'gemini-3.1-flash-lite-preview',
});

export const OpenAIModel = Object.freeze({
    GPT_5_4:         'gpt-5.4',
    GPT_5_4_PRO:     'gpt-5.4-pro',
    GPT_5_4_MINI:    'gpt-5.4-mini',
    GPT_5_4_NANO:    'gpt-5.4-nano',
});

export const AnthropicModel = Object.freeze({
    ANTHROPIC_CLAUDE_OPUS_4_6:   'claude-opus-4-6',
    ANTHROPIC_CLAUDE_SONNET_4_6: 'claude-sonnet-4-6',
    ANTHROPIC_CLAUDE_HAIKU_4_5:  'claude-haiku-4-5',
});

export const OllamaModel = Object.freeze({
    QWEN3_5_122B:    'qwen3.5:122b',
    GLM_5_1:         'glm-5.1:cloud',
    GEMMA4_26B:      'gemma4:26b',
    GEMMA4_E4B:      'gemma4:e4b',
    GEMMA4_E2B:      'gemma4:e2b',
    NEMOTRON_3_30B:  'nemotron-3-nano:30b',
    LLAMA_3_2_3B:    'llama3.2:3b',
    QWEN3_5_2B:      'qwen3.5:2b',
});

// Every model is available at every tier for every provider. The user picks
// which model serves each role; we do not gate availability by tier.
const _tierAll = (models) => ({ all: models, primary: models, assistant: models, lite: models });

export const PROVIDER_MODELS = Object.freeze({
    [LLMProvider.GEMINI]: _tierAll([
        { id: GeminiModel.PRO, label: 'Gemini 3.1 Pro' },
        { id: GeminiModel.FLASH, label: 'Gemini 3 Flash' },
        { id: GeminiModel.FLASH_LITE, label: 'Gemini 3.1 Flash Lite' },
    ]),
    [LLMProvider.ANTHROPIC]: _tierAll([
        { id: AnthropicModel.ANTHROPIC_CLAUDE_OPUS_4_6, label: 'Claude Opus 4.6' },
        { id: AnthropicModel.ANTHROPIC_CLAUDE_SONNET_4_6, label: 'Claude Sonnet 4.6' },
        { id: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5, label: 'Claude Haiku 4.5' },
    ]),
    [LLMProvider.OPENAI]: _tierAll([
        { id: OpenAIModel.GPT_5_4, label: 'GPT-5.4' },
        { id: OpenAIModel.GPT_5_4_PRO, label: 'GPT-5.4 Pro' },
        { id: OpenAIModel.GPT_5_4_MINI, label: 'GPT-5.4 Mini' },
        { id: OpenAIModel.GPT_5_4_NANO, label: 'GPT-5.4 Nano' },
    ]),
    [LLMProvider.OLLAMA]: _tierAll([
        { id: OllamaModel.QWEN3_5_122B, label: 'Qwen 3.5 122B' },
        { id: OllamaModel.GLM_5_1, label: 'GLM 5.1' },
        { id: OllamaModel.GEMMA4_26B, label: 'Gemma 4 26B' },
        { id: OllamaModel.GEMMA4_E4B, label: 'Gemma 4 E4B' },
        { id: OllamaModel.GEMMA4_E2B, label: 'Gemma 4 E2B' },
        { id: OllamaModel.NEMOTRON_3_30B, label: 'Nemotron 3 Nano 30B' },
        { id: OllamaModel.LLAMA_3_2_3B, label: 'Llama 3.2 3B' },
        { id: OllamaModel.QWEN3_5_2B, label: 'Qwen 3.5 2B' },
    ]),
});
