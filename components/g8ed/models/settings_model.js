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

import { LLMProvider, SearchProvider, GeminiModel, OpenAIModel, AnthropicModel, OllamaModel, LlamaCppModel, PROVIDER_MODELS } from '../constants/ai.js';

// All models for each provider are available at every tier; the user decides
// which model serves primary / assistant / lite.
const OPENAI_MODEL_OPTIONS = Object.freeze([
    Object.freeze({ value: OpenAIModel.GPT_5_4,       label: 'GPT-5.4' }),
    Object.freeze({ value: OpenAIModel.GPT_5_4_PRO,   label: 'GPT-5.4 Pro' }),
    Object.freeze({ value: OpenAIModel.GPT_5_4_MINI,  label: 'GPT-5.4 Mini' }),
    Object.freeze({ value: OpenAIModel.GPT_5_4_NANO,  label: 'GPT-5.4 Nano' }),
]);
const ANTHROPIC_MODEL_OPTIONS = Object.freeze([
    Object.freeze({ value: AnthropicModel.ANTHROPIC_CLAUDE_OPUS_4_6,   label: 'Claude Opus 4.6' }),
    Object.freeze({ value: AnthropicModel.ANTHROPIC_CLAUDE_SONNET_4_6, label: 'Claude Sonnet 4.6' }),
    Object.freeze({ value: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5,  label: 'Claude Haiku 4.5' }),
]);
const LLAMACPP_MODEL_OPTIONS = Object.freeze([
    Object.freeze({ value: LlamaCppModel.GEMMA4_E2B, label: 'Gemma 4 E2B (llama.cpp)' }),
]);

// ---------------------------------------------------------------------------
// USER_SETTINGS — user-configurable settings shown and saved via the UI
// ---------------------------------------------------------------------------

export const USER_SETTINGS = Object.freeze([
    // -------------------------------------------------------------------------
    // LLM Provider
    // -------------------------------------------------------------------------
    Object.freeze({
        key: 'llm_primary_provider',
        section: 'llm',
        label: 'Primary LLM Provider',
        description: 'Main AI provider type.',
        type: 'select',
        options: Object.freeze([
            Object.freeze({ value: LLMProvider.OPENAI,    label: 'OpenAI' }),
            Object.freeze({ value: LLMProvider.OLLAMA,    label: 'Ollama' }),
            Object.freeze({ value: LLMProvider.GEMINI,    label: 'Gemini (Google)' }),
            Object.freeze({ value: LLMProvider.ANTHROPIC, label: 'Anthropic (Claude)' }),
            Object.freeze({ value: LLMProvider.LLAMACPP,  label: 'llama.cpp' }),
        ]),
        secret: false,
        placeholder: '',
        default: '',
    }),
    Object.freeze({
        key: 'llm_assistant_provider',
        section: 'llm',
        label: 'Assistant LLM Provider',
        description: 'Assistant AI provider type.',
        type: 'select',
        options: Object.freeze([
            Object.freeze({ value: LLMProvider.OPENAI,    label: 'OpenAI' }),
            Object.freeze({ value: LLMProvider.OLLAMA,    label: 'Ollama' }),
            Object.freeze({ value: LLMProvider.GEMINI,    label: 'Gemini (Google)' }),
            Object.freeze({ value: LLMProvider.ANTHROPIC, label: 'Anthropic (Claude)' }),
            Object.freeze({ value: LLMProvider.LLAMACPP,  label: 'llama.cpp' }),
        ]),
        secret: false,
        placeholder: '',
        default: '',
    }),
    Object.freeze({
        key: 'llm_lite_provider',
        section: 'llm',
        label: 'Lite LLM Provider',
        description: 'Lite AI provider type.',
        type: 'select',
        options: Object.freeze([
            Object.freeze({ value: LLMProvider.OPENAI,    label: 'OpenAI' }),
            Object.freeze({ value: LLMProvider.OLLAMA,    label: 'Ollama' }),
            Object.freeze({ value: LLMProvider.GEMINI,    label: 'Gemini (Google)' }),
            Object.freeze({ value: LLMProvider.ANTHROPIC, label: 'Anthropic (Claude)' }),
            Object.freeze({ value: LLMProvider.LLAMACPP,  label: 'llama.cpp' }),
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
        options: OPENAI_MODEL_OPTIONS,
        secret: false,
        placeholder: '',
        default: ''
    }),
    Object.freeze({
        key: 'llm_assistant_model',
        section: 'llm',
        label: 'Assistant LLM Model',
        description: 'Lightweight model for assistant tasks and command generation.',
        type: 'select',
        provider: LLMProvider.OPENAI,
        options: OPENAI_MODEL_OPTIONS,
        secret: false,
        placeholder: '',
        default: ''
    }),
    Object.freeze({
        key: 'llm_lite_model',
        section: 'llm',
        label: 'Lite LLM Model',
        description: 'Ultra-lightweight model for quick tasks.',
        type: 'select',
        provider: LLMProvider.OPENAI,
        options: OPENAI_MODEL_OPTIONS,
        secret: false,
        placeholder: '',
        default: ''
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
        key: 'ollama_endpoint',
        section: 'llm',
        label: 'Ollama Host',
        description: 'Host and port of your Ollama server (e.g. 192.168.1.100:11434). Do not include a scheme or path.',
        type: 'text',
        provider: LLMProvider.OLLAMA,
        secret: false,
        placeholder: '192.168.1.100:11434',
        default: '',
    }),
    Object.freeze({
        key: 'ollama_api_key',
        section: 'llm',
        label: 'Ollama API Key',
        description: 'API key for Ollama (optional - only required for authenticated instances).',
        type: 'password',
        provider: LLMProvider.OLLAMA,
        secret: true,
        placeholder: '',
        default: '',
    }),

    // -------------------------------------------------------------------------
    // Gemini Specific
    // -------------------------------------------------------------------------
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
        options: ANTHROPIC_MODEL_OPTIONS,
        secret: false,
        placeholder: '',
        default: ''
    }),
    Object.freeze({
        key: 'llm_assistant_model',
        section: 'llm',
        label: 'Assistant LLM Model',
        description: 'Lightweight model for assistant tasks and command generation.',
        type: 'select',
        provider: LLMProvider.ANTHROPIC,
        options: ANTHROPIC_MODEL_OPTIONS,
        secret: false,
        placeholder: '',
        default: ''
    }),
    Object.freeze({
        key: 'llm_lite_model',
        section: 'llm',
        label: 'Lite LLM Model',
        description: 'Ultra-lightweight model for quick tasks.',
        type: 'select',
        provider: LLMProvider.ANTHROPIC,
        options: ANTHROPIC_MODEL_OPTIONS,
        secret: false,
        placeholder: '',
        default: ''
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

    // -------------------------------------------------------------------------
    // llama.cpp Specific
    // -------------------------------------------------------------------------
    Object.freeze({
        key: 'llm_model',
        section: 'llm',
        label: 'Primary LLM Model',
        description: 'Main model used for investigations and AI reasoning.',
        type: 'select',
        provider: LLMProvider.LLAMACPP,
        options: LLAMACPP_MODEL_OPTIONS,
        secret: false,
        placeholder: '',
        default: ''
    }),
    Object.freeze({
        key: 'llm_assistant_model',
        section: 'llm',
        label: 'Assistant LLM Model',
        description: 'Lightweight model for assistant tasks and command generation.',
        type: 'select',
        provider: LLMProvider.LLAMACPP,
        options: LLAMACPP_MODEL_OPTIONS,
        secret: false,
        placeholder: '',
        default: ''
    }),
    Object.freeze({
        key: 'llm_lite_model',
        section: 'llm',
        label: 'Lite LLM Model',
        description: 'Ultra-lightweight model for quick tasks.',
        type: 'select',
        provider: LLMProvider.LLAMACPP,
        options: LLAMACPP_MODEL_OPTIONS,
        secret: false,
        placeholder: '',
        default: ''
    }),
    Object.freeze({
        key: 'llamacpp_endpoint',
        section: 'llm',
        label: 'llama.cpp Host',
        description: 'Host and port of your llama.cpp server (e.g. g8el:11444). Do not include a scheme or path.',
        type: 'text',
        provider: LLMProvider.LLAMACPP,
        secret: false,
        placeholder: 'g8el:11444',
        default: '',
    }),
    Object.freeze({
        key: 'llamacpp_api_key',
        section: 'llm',
        label: 'llama.cpp API Key',
        description: 'API key for llama.cpp (optional - only required for authenticated instances).',
        type: 'password',
        provider: LLMProvider.LLAMACPP,
        secret: true,
        placeholder: '',
        default: '',
    }),
    Object.freeze({
        key: 'llm_max_tokens',
        section: 'llm_internal',
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
        section: 'llm_internal',
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
        section: 'llm_internal',
        label: 'Command Generation Auditor',
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
        section: 'llm_internal',
        label: 'Command Generation Passes',
        description: 'Number of generation passes in the tribunal (1–10).',
        type: 'text',
        group: 'universal',
        secret: false,
        placeholder: '',
        default: '',
        validate: v => Number.isInteger(Number(v)) && Number(v) >= 1 && Number(v) <= 10,
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
        key: 'whitelisted_commands_csv',
        section: 'validation',
        label: 'Whitelisted Commands',
        description: 'Comma-separated list of whitelisted commands (e.g., uptime,df,free). When non-empty, this REPLACES the JSON whitelist entirely and uses only basic character-level validation. The JSON whitelist\'s per-command safe_options and validation regexes are NOT applied in CSV mode. Leave empty to use JSON whitelist with rich validation.',
        type: 'text',
        secret: false,
        placeholder: 'uptime,df,free,ps',
        default: '',
        validate: v => {
            if (typeof v !== 'string') return false;
            if (v === '') return true;
            const parts = v.split(',').map(p => p.trim()).filter(Boolean);
            const unsafeChars = /[;|`$<>&\n\r\t ]/;
            return parts.every(part => !unsafeChars.test(part));
        },
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
        section: 'security',
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

const PROVIDER_CREDENTIAL_REQUIREMENTS = Object.freeze({
    [LLMProvider.OPENAI]: ['openai_api_key'],
    [LLMProvider.OLLAMA]: ['ollama_endpoint'],
    [LLMProvider.GEMINI]: ['gemini_api_key'],
    [LLMProvider.ANTHROPIC]: ['anthropic_api_key'],
    [LLMProvider.LLAMACPP]: ['llamacpp_endpoint'],
});

/**
 * Validates cross-field dependencies for provider-specific credentials
 * @param {Object} updates - Settings updates to validate
 * @returns {Array} - Array of error messages for cross-field validation failures
 */
function validateCrossFieldDependencies(updates) {
    const errors = [];

    // Helper to derive provider from model ID
    function deriveProviderFromModel(modelId) {
        if (!modelId || modelId === 'custom' || modelId.trim() === '') return null;
        for (const [provider, config] of Object.entries(PROVIDER_MODELS)) {
            const allModels = [...config.primary, ...config.assistant];
            if (allModels.some(m => m.id === modelId)) return provider;
        }
        return null;
    }

    // Derive providers from models (UI now derives these, validation should too)
    const primaryProvider = updates.llm_primary_provider || deriveProviderFromModel(updates.llm_model);
    const assistantProvider = updates.llm_assistant_provider || deriveProviderFromModel(updates.llm_assistant_model);
    const liteProvider = updates.llm_lite_provider || deriveProviderFromModel(updates.llm_lite_model);

    if (primaryProvider && primaryProvider !== '') {
        const required = PROVIDER_CREDENTIAL_REQUIREMENTS[primaryProvider];
        if (required) {
            for (const credField of required) {
                if (!updates[credField] || updates[credField].trim() === '') {
                    errors.push(`${credField} is required when ${primaryProvider} is set as primary provider`);
                }
            }
        }
    }

    if (assistantProvider && assistantProvider !== '') {
        const required = PROVIDER_CREDENTIAL_REQUIREMENTS[assistantProvider];
        if (required) {
            for (const credField of required) {
                if (!updates[credField] || updates[credField].trim() === '') {
                    errors.push(`${credField} is required when ${assistantProvider} is set as assistant provider`);
                }
            }
        }
    }

    if (liteProvider && liteProvider !== '') {
        const required = PROVIDER_CREDENTIAL_REQUIREMENTS[liteProvider];
        if (required) {
            for (const credField of required) {
                if (!updates[credField] || updates[credField].trim() === '') {
                    errors.push(`${credField} is required when ${liteProvider} is set as lite provider`);
                }
            }
        }
    }

    if (updates.vertex_search_enabled === true) {
        if (!updates.vertex_search_project_id || updates.vertex_search_project_id.trim() === '') {
            errors.push('vertex_search_project_id is required when vertex_search_enabled is true');
        }
        if (!updates.vertex_search_engine_id || updates.vertex_search_engine_id.trim() === '') {
            errors.push('vertex_search_engine_id is required when vertex_search_enabled is true');
        }
        if (!updates.vertex_search_api_key || updates.vertex_search_api_key.trim() === '') {
            errors.push('vertex_search_api_key is required when vertex_search_enabled is true');
        }
    }

    return errors;
}

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

    const crossFieldErrors = validateCrossFieldDependencies(updates);
    for (const error of crossFieldErrors) {
        errors.push(error);
    }

    return { valid, invalid, errors };
}

/**
 * Validates platform settings updates against PLATFORM_SETTINGS schema.
 *
 * `writeOnce` keys (e.g. `internal_auth_token`, `session_encryption_key`) are
 * bootstrap secrets owned by g8eo's SecretManager and the SSL volume. Once set
 * to a non-empty value they cannot be overwritten via this path — any such
 * attempt is recorded in `skipped` and excluded from `valid`. This prevents
 * UI writes from silently diverging from the volume-authoritative value, which
 * would then be clobbered on the next g8eo restart.
 *
 * @param {Object} updates - Settings updates to validate
 * @param {Object} [existingSettings] - Current persisted settings, used to
 *   enforce the writeOnce guard. Pass `{}` when no document exists yet.
 * @returns {Object} - { valid: Object, invalid: Array, skipped: Array, errors: Array }
 */
export function validatePlatformSettings(updates, existingSettings = {}) {
    const valid = {};
    const invalid = [];
    const skipped = [];
    const errors = [];

    for (const [key, value] of Object.entries(updates)) {
        const field = CONFIG_BY_KEY.get(key);
        if (!field) {
            invalid.push(key);
            errors.push(`Unknown platform setting: ${key}`);
            continue;
        }

        if (field.writeOnce) {
            const existing = existingSettings?.[key];
            if (existing !== undefined && existing !== null && existing !== '') {
                skipped.push(key);
                errors.push(`${key} is writeOnce and already set; refusing overwrite`);
                continue;
            }
        }

        valid[key] = value;
    }

    return { valid, invalid, skipped, errors };
}

// ---------------------------------------------------------------------------
// SETTINGS_PAGE_SECTIONS — ordered nav sections rendered by the Settings UI
// ---------------------------------------------------------------------------

export const SETTINGS_PAGE_SECTIONS = Object.freeze([
    Object.freeze({ id: 'llm',        label: 'LLM',                icon: 'psychology' }),
    Object.freeze({ id: 'search',     label: 'Web Search',         icon: 'travel_explore' }),
    Object.freeze({ id: 'validation', label: 'Command Validation', icon: 'verified_user' }),
    Object.freeze({ id: 'security',   label: 'Security',           icon: 'shield' }),
    Object.freeze({ id: 'advanced',   label: 'Advanced',           icon: 'code' }),
]);

// ---------------------------------------------------------------------------
// Flat-key to nested-group mapping
// ---------------------------------------------------------------------------

const LLM_KEY_MAP = Object.freeze({
    llm_primary_provider:   'primary_provider',
    llm_assistant_provider: 'assistant_provider',
    llm_lite_provider:      'lite_provider',
    llm_model:              'primary_model',
    llm_assistant_model:    'assistant_model',
    llm_lite_model:         'lite_model',
    openai_api_key:         'openai_api_key',
    ollama_endpoint:        'ollama_endpoint',
    ollama_api_key:         'ollama_api_key',
    gemini_api_key:         'gemini_api_key',
    anthropic_api_key:      'anthropic_api_key',
    llamacpp_endpoint:      'llamacpp_endpoint',
    llamacpp_api_key:       'llamacpp_api_key',
    llm_max_tokens:         'llm_max_tokens',
    llm_command_gen_enabled:  'llm_command_gen_enabled',
    llm_command_gen_verifier: 'llm_command_gen_verifier',
    llm_command_gen_passes:   'llm_command_gen_passes',
});

const SEARCH_KEY_MAP = Object.freeze({
    vertex_search_enabled:    'enabled',
    vertex_search_project_id: 'project_id',
    vertex_search_engine_id:  'engine_id',
    vertex_search_location:   'location',
    vertex_search_api_key:    'api_key',
});

const EVAL_JUDGE_KEY_MAP = Object.freeze({
    eval_judge_model:       'eval_judge_model',
    eval_judge_max_tokens:  'eval_judge_max_tokens',
});

const COMMAND_VALIDATION_KEY_MAP = Object.freeze({
    enable_command_whitelisting: 'enable_whitelisting',
    whitelisted_commands_csv: 'whitelisted_commands',
    enable_command_blacklisting: 'enable_blacklisting',
});

const SECURITY_KEY_MAP = Object.freeze({
    g8e_api_key: 'g8e_api_key',
});

/**
 * Convert flat UI settings into the nested document shape
 * expected by g8ee's UserSettingsDocument.
 *
 * Input:  { llm_primary_provider: 'gemini', llm_model: '...', vertex_search_enabled: true, ... }
 * Output: { llm: { primary_provider: 'gemini', primary_model: '...' }, search: { enabled: true }, eval_judge: {} }
 *
 * @param {Object} flat - Flat key/value pairs from UI
 * @returns {{ llm: Object, search: Object, eval_judge: Object, command_validation: Object, security: Object }}
 */
export function structureUserSettings(flat) {
    const llm = {};
    const search = {};
    const evalJudge = {};
    const commandValidation = {};
    const security = {};

    for (const [key, value] of Object.entries(flat)) {
        if (key in LLM_KEY_MAP) {
            llm[LLM_KEY_MAP[key]] = value;
        } else if (key in SEARCH_KEY_MAP) {
            search[SEARCH_KEY_MAP[key]] = value;
        } else if (key in EVAL_JUDGE_KEY_MAP) {
            evalJudge[EVAL_JUDGE_KEY_MAP[key]] = value;
        } else if (key in COMMAND_VALIDATION_KEY_MAP) {
            commandValidation[COMMAND_VALIDATION_KEY_MAP[key]] = value;
        } else if (key in SECURITY_KEY_MAP) {
            security[SECURITY_KEY_MAP[key]] = value;
        }
    }

    return { llm, search, eval_judge: evalJudge, command_validation: commandValidation, security };
}

const REVERSE_LLM_MAP    = Object.freeze({
    primary_provider: 'llm_primary_provider',
    assistant_provider: 'llm_assistant_provider',
    lite_provider: 'llm_lite_provider',
    primary_model: 'llm_model',
    assistant_model: 'llm_assistant_model',
    lite_model: 'llm_lite_model',
    openai_api_key: 'openai_api_key',
    ollama_endpoint: 'ollama_endpoint',
    ollama_api_key: 'ollama_api_key',
    gemini_api_key:         'gemini_api_key',
    anthropic_api_key:      'anthropic_api_key',
    llamacpp_endpoint:      'llamacpp_endpoint',
    llamacpp_api_key:       'llamacpp_api_key',
    llm_max_tokens:         'llm_max_tokens',
    llm_command_gen_enabled: 'llm_command_gen_enabled',
    llm_command_gen_verifier: 'llm_command_gen_verifier',
    llm_command_gen_passes: 'llm_command_gen_passes',
});
const REVERSE_SEARCH_MAP = Object.freeze(Object.fromEntries(Object.entries(SEARCH_KEY_MAP).map(([k, v]) => [v, k])));
const REVERSE_EVAL_MAP   = Object.freeze(Object.fromEntries(Object.entries(EVAL_JUDGE_KEY_MAP).map(([k, v]) => [v, k])));
const REVERSE_COMMAND_VALIDATION_MAP = Object.freeze(Object.fromEntries(Object.entries(COMMAND_VALIDATION_KEY_MAP).map(([k, v]) => [v, k])));
const REVERSE_SECURITY_MAP = Object.freeze(Object.fromEntries(Object.entries(SECURITY_KEY_MAP).map(([k, v]) => [v, k])));

/**
 * Flatten nested user settings back to flat UI keys.
 *
 * Input:  { llm: { primary_provider: 'gemini', ... }, search: { enabled: true }, eval_judge: {} }
 * Output: { llm_primary_provider: 'gemini', ..., vertex_search_enabled: true, ... }
 *
 * @param {{ llm?: Object, search?: Object, eval_judge?: Object, command_validation?: Object, security?: Object }} nested
 * @returns {Object}
 */
export function flattenUserSettings(nested) {
    const flat = {};

    if (nested.llm) {
        for (const [nestedKey, value] of Object.entries(nested.llm)) {
            const flatKey = REVERSE_LLM_MAP[nestedKey];
            if (flatKey) flat[flatKey] = value;
        }
    }
    if (nested.search) {
        for (const [nestedKey, value] of Object.entries(nested.search)) {
            const flatKey = REVERSE_SEARCH_MAP[nestedKey];
            if (flatKey) flat[flatKey] = value;
        }
    }
    if (nested.eval_judge) {
        for (const [nestedKey, value] of Object.entries(nested.eval_judge)) {
            const flatKey = REVERSE_EVAL_MAP[nestedKey];
            if (flatKey) flat[flatKey] = value;
        }
    }
    if (nested.command_validation) {
        for (const [nestedKey, value] of Object.entries(nested.command_validation)) {
            const flatKey = REVERSE_COMMAND_VALIDATION_MAP[nestedKey];
            if (flatKey) flat[flatKey] = value;
        }
    }
    if (nested.security) {
        for (const [nestedKey, value] of Object.entries(nested.security)) {
            const flatKey = REVERSE_SECURITY_MAP[nestedKey];
            if (flatKey) flat[flatKey] = value;
        }
    }

    return flat;
}

// ---------------------------------------------------------------------------
// Pre-built lookup maps for O(1) access
// ---------------------------------------------------------------------------

export const SETTINGS_BY_KEY = new Map(USER_SETTINGS.map(s => [s.key, s]));
export const CONFIG_BY_KEY   = new Map(PLATFORM_SETTINGS.map(s => [s.key, s]));
