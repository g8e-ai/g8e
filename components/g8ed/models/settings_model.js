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
 * Settings Model
 *
 * Defines the schema and validation for user and platform settings.
 * Moved from settings_service.js to follow proper model separation.
 */

import { LLMProvider, SearchProvider, GeminiModel, OpenAIModel, AnthropicModel, OllamaModel } from '../constants/ai.js';

// ---------------------------------------------------------------------------
// USER_SETTINGS — user-configurable settings shown and saved via the UI
// ---------------------------------------------------------------------------

export const USER_SETTINGS = Object.freeze([
    // -------------------------------------------------------------------------
    // LLM Provider
    // -------------------------------------------------------------------------
    Object.freeze({
        key: 'llm_provider',
        section: 'llm',
        label: 'LLM Provider',
        description: 'AI provider type.',
        type: 'select',
        options: Object.freeze([
            Object.freeze({ value: LLMProvider.OPENAI,    label: 'OpenAI' }),
            Object.freeze({ value: LLMProvider.OLLAMA,    label: 'Ollama' }),
            Object.freeze({ value: LLMProvider.GEMINI,    label: 'Gemini (Google)' }),
            Object.freeze({ value: LLMProvider.ANTHROPIC, label: 'Anthropic (Claude)' }),
        ]),
        secret: false,
        placeholder: '',
        default: '',
    }),
    // -------------------------------------------------------------------------
    // OpenAI Specific
    // -------------------------------------------------------------------------
    Object.freeze({
        key: 'llm_model',
        section: 'llm',
        label: 'Primary LLM Model',
        description: 'Main model used for investigations and AI reasoning.',
        type: 'select',
        provider: LLMProvider.OPENAI,
        options: Object.freeze([
            Object.freeze({ value: OpenAIModel.GPT_5_4, label: 'GPT-5.4 (Flagship)' }),
            Object.freeze({ value: OpenAIModel.GPT_5_3_INSTANT, label: 'GPT-5.3 Instant' }),
            Object.freeze({ value: OpenAIModel.GPT_4O, label: 'GPT-4o' }),
        ]),
        secret: false,
        placeholder: '',
        default: OpenAIModel.GPT_5_4,
    }),
    Object.freeze({
        key: 'llm_assistant_model',
        section: 'llm',
        label: 'Assistant LLM Model',
        description: 'Lightweight model for assistant tasks and command generation.',
        type: 'select',
        provider: LLMProvider.OPENAI,
        options: Object.freeze([
            Object.freeze({ value: OpenAIModel.GPT_5_4_MINI, label: 'GPT-5.4 mini' }),
            Object.freeze({ value: OpenAIModel.GPT_5_4_NANO, label: 'GPT-5.4 nano' }),
            Object.freeze({ value: OpenAIModel.GPT_4O_MINI, label: 'GPT-4o mini' }),
        ]),
        secret: false,
        placeholder: '',
        default: OpenAIModel.GPT_5_4_MINI,
    }),
    Object.freeze({
        key: 'openai_endpoint',
        section: 'llm',
        label: 'OpenAI Endpoint URL',
        description: 'API endpoint for OpenAI provider (e.g. https://api.openai.com/v1).',
        type: 'text',
        provider: LLMProvider.OPENAI,
        secret: false,
        placeholder: 'https://api.openai.com/v1',
        default: '',
    }),
    Object.freeze({
        key: 'openai_api_key',
        section: 'llm',
        label: 'OpenAI API Key',
        description: 'API key for OpenAI provider.',
        type: 'password',
        provider: LLMProvider.OPENAI,
        secret: true,
        placeholder: 'sk-...',
        default: '',
    }),

    // -------------------------------------------------------------------------
    // Ollama Specific
    // -------------------------------------------------------------------------
    Object.freeze({
        key: 'llm_model',
        section: 'llm',
        label: 'Primary LLM Model',
        description: 'Main model used for investigations and AI reasoning.',
        type: 'text',
        provider: LLMProvider.OLLAMA,
        secret: false,
        placeholder: 'gemma4:e4b',
        default: OllamaModel.GEMMA4_E4B,
    }),
    Object.freeze({
        key: 'llm_assistant_model',
        section: 'llm',
        label: 'Assistant LLM Model',
        description: 'Lightweight model for assistant tasks and command generation.',
        type: 'text',
        provider: LLMProvider.OLLAMA,
        secret: false,
        placeholder: 'gemma4:e4b',
        default: OllamaModel.GEMMA4_E4B,
    }),
    Object.freeze({
        key: 'ollama_endpoint',
        section: 'llm',
        label: 'Ollama Endpoint URL',
        description: 'API endpoint for Ollama (e.g. https://your-ollama-host:11434/v1).',
        type: 'text',
        provider: LLMProvider.OLLAMA,
        secret: false,
        placeholder: 'https://your-ollama-host:11434/v1',
        default: '',
    }),
    Object.freeze({
        key: 'ollama_api_key',
        section: 'llm',
        label: 'Ollama API Key',
        description: 'API key for Ollama (use "ollama" for local unauthenticated instances).',
        type: 'password',
        provider: LLMProvider.OLLAMA,
        secret: true,
        placeholder: LLMProvider.OLLAMA,
        default: '',
    }),

    // -------------------------------------------------------------------------
    // Gemini Specific
    // -------------------------------------------------------------------------
    Object.freeze({
        key: 'llm_model',
        section: 'llm',
        label: 'Primary LLM Model',
        description: 'Main model used for investigations and AI reasoning.',
        type: 'select',
        provider: LLMProvider.GEMINI,
        options: Object.freeze([
            Object.freeze({ value: GeminiModel.PRO_PREVIEW,        label: 'Gemini 3.1 Pro' }),
            Object.freeze({ value: GeminiModel.FLASH_PREVIEW,      label: 'Gemini 3 Flash Preview' }),
            Object.freeze({ value: GeminiModel.FLASH_LITE_PREVIEW, label: 'Gemini 3.1 Flash Lite' }),
        ]),
        secret: false,
        placeholder: '',
        default: GeminiModel.FLASH_PREVIEW,
    }),
    Object.freeze({
        key: 'llm_assistant_model',
        section: 'llm',
        label: 'Assistant LLM Model',
        description: 'Lightweight model for assistant tasks and command generation.',
        type: 'select',
        provider: LLMProvider.GEMINI,
        options: Object.freeze([
            Object.freeze({ value: GeminiModel.FLASH_LITE_PREVIEW, label: 'Gemini 3.1 Flash Lite' }),
            Object.freeze({ value: GeminiModel.FLASH_PREVIEW,      label: 'Gemini 3 Flash' }),
        ]),
        secret: false,
        placeholder: '',
        default: GeminiModel.FLASH_PREVIEW,
    }),
    Object.freeze({
        key: 'gemini_api_key',
        section: 'llm',
        label: 'Gemini API Key',
        description: 'Google Cloud API key for Gemini provider.',
        type: 'password',
        provider: LLMProvider.GEMINI,
        secret: true,
        placeholder: 'your-gemini-api-key-here',
        default: '',
    }),

    // -------------------------------------------------------------------------
    // Anthropic Specific
    // -------------------------------------------------------------------------
    Object.freeze({
        key: 'llm_model',
        section: 'llm',
        label: 'Primary LLM Model',
        description: 'Main model used for investigations and AI reasoning.',
        type: 'select',
        provider: LLMProvider.ANTHROPIC,
        options: Object.freeze([
            Object.freeze({ value: AnthropicModel.ANTHROPIC_CLAUDE_OPUS_4_6, label: 'Claude Opus 4.6' }),
            Object.freeze({ value: AnthropicModel.ANTHROPIC_CLAUDE_SONNET_4_6, label: 'Claude Sonnet 4.6' }),
            Object.freeze({ value: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5, label: 'Claude Haiku 4.5' }),
        ]),
        secret: false,
        placeholder: '',
        default: AnthropicModel.ANTHROPIC_CLAUDE_OPUS_4_6,
    }),
    Object.freeze({
        key: 'llm_assistant_model',
        section: 'llm',
        label: 'Assistant LLM Model',
        description: 'Lightweight model for assistant tasks and command generation.',
        type: 'select',
        provider: LLMProvider.ANTHROPIC,
        options: Object.freeze([
            Object.freeze({ value: AnthropicModel.ANTHROPIC_CLAUDE_SONNET_4_6, label: 'Claude Sonnet 4.6' }),
            Object.freeze({ value: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5, label: 'Claude Haiku 4.5' }),
        ]),
        secret: false,
        placeholder: '',
        default: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5,
    }),
    Object.freeze({
        key: 'anthropic_endpoint',
        section: 'llm',
        label: 'Anthropic Endpoint URL',
        description: 'API endpoint for Anthropic (e.g. https://api.anthropic.com/v1).',
        type: 'text',
        provider: LLMProvider.ANTHROPIC,
        secret: false,
        placeholder: 'https://api.anthropic.com/v1',
        default: '',
    }),
    Object.freeze({
        key: 'anthropic_api_key',
        section: 'llm',
        label: 'Anthropic API Key',
        description: 'Anthropic API key for Claude provider.',
        type: 'password',
        provider: LLMProvider.ANTHROPIC,
        secret: true,
        placeholder: 'your-anthropic-api-key-here',
        default: '',
    }),
    Object.freeze({
        key: 'llm_temperature',
        section: 'llm',
        label: 'Temperature',
        description: 'LLM sampling temperature (0.0–2.0).',
        type: 'text',
        group: 'universal',
        secret: false,
        placeholder: '',
        default: '',
        validate: v => {
            const f = parseFloat(v);
            return !isNaN(f) && isFinite(f) && f >= 0.0 && f <= 2.0;
        },
    }),
    Object.freeze({
        key: 'llm_max_tokens',
        section: 'llm',
        label: 'Max Tokens',
        description: 'Maximum tokens per LLM response.',
        type: 'text',
        group: 'universal',
        secret: false,
        placeholder: '',
        default: '',
        validate: v => {
            const n = Number(v);
            return Number.isInteger(n) && n > 0 && n <= 1000000;
        },
    }),
    Object.freeze({
        key: 'llm_command_gen_enabled',
        section: 'llm',
        label: 'Command Generation Enabled',
        description: 'Enable AI command generation and rewriting.',
        type: 'select',
        group: 'universal',
        options: Object.freeze([
            Object.freeze({ value: true,  label: 'Enabled' }),
            Object.freeze({ value: false, label: 'Disabled' }),
        ]),
        secret: false,
        placeholder: '',
        default: '',
    }),
    Object.freeze({
        key: 'llm_command_gen_verifier',
        section: 'llm',
        label: 'Command Generation Verifier',
        description: 'Enable verifier pass in the command generation tribunal.',
        type: 'select',
        group: 'universal',
        options: Object.freeze([
            Object.freeze({ value: true, label: 'Enabled' }),
            Object.freeze({ value: false, label: 'Disabled' }),
        ]),
        secret: false,
        placeholder: '',
        default: '',
    }),
    Object.freeze({
        key: 'llm_command_gen_passes',
        section: 'llm',
        label: 'Command Generation Passes',
        description: 'Number of generation passes in the tribunal (1–10).',
        type: 'text',
        group: 'universal',
        secret: false,
        placeholder: '',
        default: '',
        validate: v => Number.isInteger(Number(v)) && Number(v) >= 1 && Number(v) <= 10,
    }),
    Object.freeze({
        key: 'llm_command_gen_temp',
        section: 'llm',
        label: 'Command Generation Temperature',
        description: 'Sampling temperature for command generation passes (0.0–2.0).',
        type: 'text',
        group: 'universal',
        secret: false,
        placeholder: '',
        default: '',
        validate: v => !isNaN(parseFloat(v)) && isFinite(v),
    }),

    // -------------------------------------------------------------------------
    // Google Vertex AI Search
    // -------------------------------------------------------------------------
    Object.freeze({
        key: 'vertex_search_enabled',
        section: 'search',
        label: 'Google Vertex AI (Discovery Engine)',
        description: 'Enable the search_web AI tool via Google Vertex AI Search (Discovery Engine).',
        type: 'select',
        options: Object.freeze([
            Object.freeze({ value: false, label: 'Disabled' }),
            Object.freeze({ value: true, label: 'Enabled' }),
        ]),
        secret: false,
        placeholder: '',
        default: false,
        validate: v => typeof v === 'boolean',
    }),

    Object.freeze({
        key: 'vertex_search_project_id',
        section: 'search',
        label: 'GCP Project ID',
        description: 'Google Cloud project ID containing the Vertex AI Search app.',
        type: 'text',
        secret: false,
        placeholder: 'your-gcp-project-id',
        default: '',
        validate: v => typeof v === 'string' && (v === '' || /^[a-z0-9-]+$/.test(v)),
    }),
    Object.freeze({
        key: 'vertex_search_engine_id',
        section: 'search',
        label: 'Search Engine ID',
        description: 'App ID shown in the Vertex AI Search app details page.',
        type: 'text',
        secret: false,
        placeholder: 'your-engine-id',
        default: '',
        validate: v => typeof v === 'string' && (v === '' || /^[a-z0-9-]+$/.test(v)),
    }),
    Object.freeze({
        key: 'vertex_search_location',
        section: 'search',
        label: 'Search Location',
        description: 'Data store location — must match the location chosen when creating the data store. Most use global.',
        type: 'text',
        secret: false,
        placeholder: 'global',
        default: 'global',
        validate: v => typeof v === 'string' && v.length > 0,
    }),
    Object.freeze({
        key: 'vertex_search_api_key',
        section: 'search',
        label: 'Search API Key',
        description: 'GCP API key restricted to the Discovery Engine API. Can be the same key as GEMINI_API_KEY if scoped to both APIs.',
        type: 'password',
        secret: true,
        placeholder: 'AIza...',
        default: '',
        validate: v => typeof v === 'string' && (v === '' || v.startsWith('AIza')),
    }),

    // -------------------------------------------------------------------------
    // g8ee — Command Validation
    // -------------------------------------------------------------------------
    Object.freeze({
        key: 'enable_command_whitelisting',
        section: 'validation',
        label: 'Command Whitelisting',
        description: 'Only allow commands that match the configured whitelist.',
        type: 'select',
        options: Object.freeze([
            Object.freeze({ value: false, label: 'Disabled (default)' }),
            Object.freeze({ value: true, label: 'Enabled' }),
        ]),
        secret: false,
        placeholder: '',
        default: false,
    }),
    Object.freeze({
        key: 'enable_command_blacklisting',
        section: 'validation',
        label: 'Command Blacklisting',
        description: 'Block commands that match the configured blacklist.',
        type: 'select',
        options: Object.freeze([
            Object.freeze({ value: false, label: 'Disabled (default)' }),
            Object.freeze({ value: true, label: 'Enabled' }),
        ]),
        secret: false,
        placeholder: '',
        default: false,
    }),
    Object.freeze({
        key: 'g8e_api_key',
        section: 'validation',
        label: 'g8e API Key',
        description: 'API key for g8ee external API authentication (optional).',
        type: 'password',
        secret: true,
        placeholder: 'your-g8e-api-key-here',
        default: '',
    }),
]);

// ---------------------------------------------------------------------------
// PLATFORM_SETTINGS — deployment-time config, resolved at boot only.
// Never shown in the Settings UI, never accepted by saveSettings().
// ---------------------------------------------------------------------------

export const PLATFORM_SETTINGS = Object.freeze([
    Object.freeze({
        key: 'internal_auth_token',
        writeOnce: true,
        secret: true,
        default: '',
    }),
    Object.freeze({
        key: 'session_encryption_key',
        writeOnce: true,
        secret: true,
        default: '',
    }),
    Object.freeze({ key: 'passkey_rp_id',               default: 'localhost'        }),
    Object.freeze({ key: 'passkey_origin',              default: 'https://localhost' }),
    Object.freeze({ key: 'app_url',                     default: 'https://localhost' }),
    Object.freeze({ key: 'allowed_origins',             default: 'https://localhost' }),
    Object.freeze({ key: 'setup_complete',                  default: false              }),
    Object.freeze({ key: 'g8e_internal_http_url',   default: 'https://g8es:9000' }),
    Object.freeze({ key: 'g8e_internal_pubsub_url', default: 'wss://g8es:9001' }),
    Object.freeze({ key: 'g8ee_url',                     default: 'https://g8ee'   }),
    Object.freeze({ key: 'docker_gid',                  default: '988'              }),
    Object.freeze({ key: 'https_port',                  default: '443'              }),
    Object.freeze({ key: 'http_port',                   default: '80'               }),
    Object.freeze({ key: 'port',                        default: '443'             }),
    Object.freeze({ key: 'ssl_dir',                     default: '/g8es'       }),
    Object.freeze({ key: 'tls_cert_path',               default: ''                 }),
    Object.freeze({ key: 'tls_key_path',                default: ''                 }),
    Object.freeze({ key: 'g8e_pubsub_ca_cert',      default: '/g8es/ssl/ca.crt' }),
    Object.freeze({ key: 'upload_path',                 default: ''                 }),
    Object.freeze({ key: 'session_ttl',                 default: '28800'            }),
    Object.freeze({ key: 'absolute_session_timeout',    default: '86400'            }),
    Object.freeze({ key: 'g8ep_operator_endpoint',  default: 'g8e.local'    }),
    Object.freeze({ key: 'host_ips',                     default: ''                 }),
    Object.freeze({ key: 'docs_dir',                     default: '/docs'            }),
    Object.freeze({ key: 'readme_path',                  default: '/readme/README.md' }),
    Object.freeze({ key: 'environment',                  default: 'production'       }),
    Object.freeze({ key: 'g8es_http_port',              default: '9000'            }),
    Object.freeze({ key: 'g8es_wss_port',               default: '9001'            }),
    Object.freeze({ key: 'supervisor_port',              default: '443'             }),
    Object.freeze({ key: 'g8ep_operator_api_key',   default: '', secret: true  }),
]);

// ---------------------------------------------------------------------------
// Validation Functions - Model Level Validation
// ---------------------------------------------------------------------------

/**
 * Validates user settings updates against USER_SETTINGS schema
 * @param {Object} updates - Settings updates to validate
 * @returns {Object} - { valid: Object, invalid: Array, errors: Array }
 */
export function validateUserSettings(updates) {
    const valid = {};
    const invalid = [];
    const errors = [];

    for (const [key, value] of Object.entries(updates)) {
        const field = SETTINGS_BY_KEY.get(key);
        if (!field) {
            invalid.push(key);
            errors.push(`Unknown setting: ${key}`);
            continue;
        }

        if (field.validate && !field.validate(value)) {
            invalid.push(key);
            errors.push(`Invalid value for ${key}: ${value}`);
            continue;
        }

        valid[key] = value;
    }

    return { valid, invalid, errors };
}

/**
 * Validates platform settings updates against PLATFORM_SETTINGS schema
 * @param {Object} updates - Settings updates to validate
 * @returns {Object} - { valid: Object, invalid: Array, errors: Array }
 */
export function validatePlatformSettings(updates) {
    const valid = {};
    const invalid = [];
    const errors = [];

    for (const [key, value] of Object.entries(updates)) {
        const field = CONFIG_BY_KEY.get(key);
        if (!field) {
            invalid.push(key);
            errors.push(`Unknown platform setting: ${key}`);
            continue;
        }

        valid[key] = value;
    }

    return { valid, invalid, errors };
}

// ---------------------------------------------------------------------------
// SETTINGS_PAGE_SECTIONS — ordered nav sections rendered by the Settings UI
// ---------------------------------------------------------------------------

export const SETTINGS_PAGE_SECTIONS = Object.freeze([
    Object.freeze({ id: 'llm',        label: 'LLM',                icon: 'psychology' }),
    Object.freeze({ id: 'search',     label: 'Web Search',         icon: 'travel_explore' }),
    Object.freeze({ id: 'validation', label: 'Command Validation', icon: 'verified_user' }),
    Object.freeze({ id: 'advanced',   label: 'Advanced',           icon: 'code' }),
]);

// ---------------------------------------------------------------------------
// Pre-built lookup maps for O(1) access
// ---------------------------------------------------------------------------

export const SETTINGS_BY_KEY = new Map(USER_SETTINGS.map(s => [s.key, s]));
export const CONFIG_BY_KEY   = new Map(PLATFORM_SETTINGS.map(s => [s.key, s]));
