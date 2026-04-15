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
    PRO_PREVIEW:              'gemini-3.1-pro-preview',
    PRO_PREVIEW_CUSTOMTOOLS:  'gemini-3.1-pro-preview-customtools',
    FLASH_PREVIEW:            'gemini-3-flash-preview',
});

export const OpenAIModel = Object.freeze({
    GPT_5_4:        'gpt-5.4',
    GPT_5_3_INSTANT: 'gpt-5.3-instant',
    GPT_5_4_MINI:   'gpt-5.4-mini',
    GPT_5_4_NANO:   'gpt-5.4-nano',
    GPT_4O:         'gpt-4o',
    GPT_4O_MINI:    'gpt-4o-mini',
    GPT_4_TURBO:    'gpt-4-turbo',
    GPT_3_5_TURBO:  'gpt-3.5-turbo',
});

export const AnthropicModel = Object.freeze({
    ANTHROPIC_CLAUDE_OPUS_4_6:   'claude-opus-4-6',
    ANTHROPIC_CLAUDE_SONNET_4_6: 'claude-sonnet-4-6',
    ANTHROPIC_CLAUDE_HAIKU_4_5:  'claude-haiku-4-5',
});

export const OllamaModel = Object.freeze({
    GEMMA3_27B:         'gemma3:27b',
    GEMMA3_12B:         'gemma3:12b',
    GEMMA3_4B:          'gemma3:4b',
    GEMMA3_1B:          'gemma3:1b',
    GEMMA4_E4B:         'gemma4:e4b',
    GEMMA4_E2B:         'gemma4:e2b',
    GEMMA4:             'gemma4',
    LLAMA3_8B:          'llama3:8b',
    CODELLAMA_7B:       'codellama:7b',
    MISTRAL_7B:         'mistral:7b',
});

export const PROVIDER_MODELS = Object.freeze({
    [LLMProvider.GEMINI]: {
        primary: [
            { id: GeminiModel.PRO_PREVIEW_CUSTOMTOOLS, label: 'Gemini 3.1 Pro (Custom Tools)' },
            { id: GeminiModel.PRO_PREVIEW, label: 'Gemini 3.1 Pro' },
            { id: GeminiModel.FLASH_PREVIEW, label: 'Gemini 3 Flash' },
        ],
        assistant: [
            { id: GeminiModel.FLASH_PREVIEW, label: 'Gemini 3 Flash' },
            { id: GeminiModel.PRO_PREVIEW, label: 'Gemini 3.1 Pro' },
        ],
        lite: [
            { id: GeminiModel.FLASH_PREVIEW, label: 'Gemini 3 Flash' },
        ],
        defaultPrimary: GeminiModel.PRO_PREVIEW_CUSTOMTOOLS,
        defaultAssistant: GeminiModel.FLASH_PREVIEW,
        defaultLite: GeminiModel.FLASH_PREVIEW,
    },
    [LLMProvider.ANTHROPIC]: {
        primary: [
            { id: AnthropicModel.ANTHROPIC_CLAUDE_OPUS_4_6, label: 'Claude Opus 4.6' },
            { id: AnthropicModel.ANTHROPIC_CLAUDE_SONNET_4_6, label: 'Claude Sonnet 4.6' },
            { id: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5, label: 'Claude Haiku 4.5' },
        ],
        assistant: [
            { id: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5, label: 'Claude Haiku 4.5' },
            { id: AnthropicModel.ANTHROPIC_CLAUDE_SONNET_4_6, label: 'Claude Sonnet 4.6' },
        ],
        lite: [
            { id: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5, label: 'Claude Haiku 4.5' },
        ],
        defaultPrimary: AnthropicModel.ANTHROPIC_CLAUDE_OPUS_4_6,
        defaultAssistant: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5,
        defaultLite: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5,
    },
    [LLMProvider.OPENAI]: {
        primary: [
            { id: OpenAIModel.GPT_5_4, label: 'GPT-5.4' },
            { id: OpenAIModel.GPT_5_3_INSTANT, label: 'GPT-5.3 Instant' },
            { id: OpenAIModel.GPT_5_4_MINI, label: 'GPT-5.4 Mini' },
            { id: OpenAIModel.GPT_4O, label: 'GPT-4o' },
        ],
        assistant: [
            { id: OpenAIModel.GPT_5_4_MINI, label: 'GPT-5.4 Mini' },
            { id: OpenAIModel.GPT_5_4_NANO, label: 'GPT-5.4 Nano' },
            { id: OpenAIModel.GPT_4O_MINI, label: 'GPT-4o Mini' },
        ],
        lite: [
            { id: OpenAIModel.GPT_5_4_NANO, label: 'GPT-5.4 Nano' },
            { id: OpenAIModel.GPT_4O_MINI, label: 'GPT-4o Mini' },
        ],
        defaultPrimary: OpenAIModel.GPT_5_4,
        defaultAssistant: OpenAIModel.GPT_5_4_MINI,
        defaultLite: OpenAIModel.GPT_5_4_NANO,
    },
    [LLMProvider.OLLAMA]: {
        primary: [
            { id: OllamaModel.GEMMA4_E4B, label: 'Gemma 4 e4b' },
            { id: OllamaModel.GEMMA4_E2B, label: 'Gemma 4 e2b' },
            { id: OllamaModel.GEMMA4, label: 'Gemma 4' },
            { id: OllamaModel.GEMMA3_27B, label: 'Gemma 3 27B' },
            { id: OllamaModel.GEMMA3_12B, label: 'Gemma 3 12B' },
            { id: OllamaModel.LLAMA3_8B, label: 'Llama 3 8B' },
            { id: OllamaModel.MISTRAL_7B, label: 'Mistral 7B' },
            { id: OllamaModel.CODELLAMA_7B, label: 'CodeLlama 7B' },
        ],
        assistant: [
            { id: OllamaModel.GEMMA4_E4B, label: 'Gemma 4 e4b' },
            { id: OllamaModel.GEMMA4_E2B, label: 'Gemma 4 e2b' },
            { id: OllamaModel.GEMMA4, label: 'Gemma 4' },
            { id: OllamaModel.GEMMA3_4B, label: 'Gemma 3 4B' },
            { id: OllamaModel.GEMMA3_1B, label: 'Gemma 3 1B' },
            { id: OllamaModel.LLAMA3_8B, label: 'Llama 3 8B' },
            { id: OllamaModel.MISTRAL_7B, label: 'Mistral 7B' },
        ],
        lite: [
            { id: OllamaModel.GEMMA3_1B, label: 'Gemma 3 1B' },
            { id: OllamaModel.GEMMA3_4B, label: 'Gemma 3 4B' },
        ],
        defaultPrimary: OllamaModel.GEMMA4_E4B,
        defaultAssistant: OllamaModel.GEMMA4_E4B,
        defaultLite: OllamaModel.GEMMA3_1B,
    },
});
