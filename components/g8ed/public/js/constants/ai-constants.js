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
 * LLM model identifiers sourced from shared/constants/status.json llm.models
 */

import { _STATUS } from './shared.js';

export const LLMProvider = Object.freeze({
    OPENAI:    'openai',
    OLLAMA:    'ollama',
    GEMINI:    'gemini',
    ANTHROPIC: 'anthropic',
});

export const GeminiModel = Object.freeze({
    PRO_PREVIEW:              _STATUS['llm.models']['gemini']['3.1.pro.preview'],
    PRO_PREVIEW_CUSTOMTOOLS:  _STATUS['llm.models']['gemini']['3.1.pro.preview.customtools'],
    FLASH_PREVIEW:            _STATUS['llm.models']['gemini']['3.flash.preview'],
    FLASH_LITE_PREVIEW:       _STATUS['llm.models']['gemini']['3.1.flash.lite.preview'],
});

export const OpenAIModel = Object.freeze({
    GPT_5_4:        _STATUS['llm.models']['openai']['gpt.5.4'],
    GPT_5_3_INSTANT: _STATUS['llm.models']['openai']['gpt.5.3.instant'],
    GPT_5_4_MINI:   _STATUS['llm.models']['openai']['gpt.5.4.mini'],
    GPT_5_4_NANO:   _STATUS['llm.models']['openai']['gpt.5.4.nano'],
    GPT_4O:         _STATUS['llm.models']['openai']['gpt.4o'],
    GPT_4O_MINI:    _STATUS['llm.models']['openai']['gpt.4o.mini'],
    GPT_4_TURBO:    _STATUS['llm.models']['openai']['gpt.4.turbo'],
    GPT_3_5_TURBO:  _STATUS['llm.models']['openai']['gpt.3.5.turbo'],
});

export const AnthropicModel = Object.freeze({
    CLAUDE_4_6_OPUS:   _STATUS['llm.models']['anthropic']['claude.4.6.opus'],
    CLAUDE_4_6_SONNET: _STATUS['llm.models']['anthropic']['claude.4.6.sonnet'],
    CLAUDE_3_5_SONNET: _STATUS['llm.models']['anthropic']['claude.3.5.sonnet'],
});

export const OllamaModel = Object.freeze({
    GEMMA3_27B:         _STATUS['llm.models']['ollama']['gemma3.27b'],
    GEMMA3_12B:         _STATUS['llm.models']['ollama']['gemma3.12b'],
    GEMMA3_4B:          _STATUS['llm.models']['ollama']['gemma3.4b'],
    GEMMA3_1B:          _STATUS['llm.models']['ollama']['gemma3.1b'],
    GEMMA4:             _STATUS['llm.models']['ollama']['gemma4'],
    LLAMA3_8B:          _STATUS['llm.models']['ollama']['llama3.8b'],
    LLAMA3_70B:         _STATUS['llm.models']['ollama']['llama3.70b'],
    CODELLAMA_7B:       _STATUS['llm.models']['ollama']['codellama.7b'],
    MISTRAL_7B:         _STATUS['llm.models']['ollama']['mistral.7b'],
});
