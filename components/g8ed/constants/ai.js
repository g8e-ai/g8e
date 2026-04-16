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

import { _STATUS } from './shared.js';

/**
 * AI Constants
 * AI source identifiers, task IDs, command error types, system health,
 * runtime environment, and source component identifiers.
 * Wire-protocol values are sourced from shared/constants/status.json.
 */

/**
 * AI Task ID identifiers
 * Labels the category of AI-dispatched operator task.
 * Canonical values from shared/constants/status.json ai.task.id.
 */
export const AITaskId = Object.freeze({
    COMMAND:            _STATUS['ai.task.id']['command'],
    DIRECT_COMMAND:     _STATUS['ai.task.id']['direct.command'],
    FILE_EDIT:          _STATUS['ai.task.id']['file.edit'],
    FS_LIST:            _STATUS['ai.task.id']['fs.list'],
    FS_READ:            _STATUS['ai.task.id']['fs.read'],
    PORT_CHECK:         _STATUS['ai.task.id']['port.check'],
    FETCH_LOGS:         _STATUS['ai.task.id']['fetch.logs'],
    FETCH_HISTORY:      _STATUS['ai.task.id']['fetch.history'],
    FETCH_FILE_HISTORY: _STATUS['ai.task.id']['fetch.file.history'],
    RESTORE_FILE:       _STATUS['ai.task.id']['restore.file'],
    FETCH_FILE_DIFF:    _STATUS['ai.task.id']['fetch.file.diff'],
    INTENT_GRANT:       _STATUS['ai.task.id']['intent.grant'],
    INTENT_REVOKE:      _STATUS['ai.task.id']['intent.revoke'],
});

/**
 * Command Error Types
 * error_type values for operator command and file operation failures.
 * Canonical values from shared/constants/status.json command.error.type.
 * Mirrors: components/g8ee/app/constants.py CommandErrorType
 */
export const CommandErrorType = Object.freeze({
    VALIDATION_ERROR:              _STATUS['command.error.type']['validation.error'],
    SECURITY_ERROR:                _STATUS['command.error.type']['security.error'],
    SECURITY_VIOLATION:            _STATUS['command.error.type']['security.violation'],
    BINDING_VIOLATION:             _STATUS['command.error.type']['binding.violation'],
    NO_OPERATORS_AVAILABLE:        _STATUS['command.error.type']['no.operators.available'],
    OPERATOR_RESOLUTION_ERROR:     _STATUS['command.error.type']['g8e.resolution.error'],
    CLOUD_OPERATOR_REQUIRED:       _STATUS['command.error.type']['cloud.g8e.required'],
    BLACKLIST_VIOLATION:           _STATUS['command.error.type']['blacklist.violation'],
    WHITELIST_VIOLATION:           _STATUS['command.error.type']['whitelist.violation'],
    EXECUTION_FAILED:              _STATUS['command.error.type']['execution.failed'],
    EXECUTION_ERROR:               _STATUS['command.error.type']['execution.error'],
    USER_DENIED:                   _STATUS['command.error.type']['user.denied'],
    USER_FEEDBACK:                 _STATUS['command.error.type']['user.feedback'],
    PERMISSION_DENIED:             _STATUS['command.error.type']['permission.denied'],
    COMMAND_TIMEOUT:               _STATUS['command.error.type']['command.timeout'],
    COMMAND_EXECUTION_FAILED:      _STATUS['command.error.type']['command.execution.failed'],
    PUBSUB_SUBSCRIPTION_NOT_READY: _STATUS['command.error.type']['pubsub.subscription.not.ready'],
    UNKNOWN_TOOL:              _STATUS['command.error.type']['unknown.tool'],
    FS_LIST_FAILED:                _STATUS['command.error.type']['fs.list.failed'],
    FS_READ_FAILED:                _STATUS['command.error.type']['fs.read.failed'],
    USER_CANCELLED:                _STATUS['command.error.type']['user.cancelled'],
    RISK_ANALYSIS_BLOCKED:         _STATUS['command.error.type']['risk.analysis.blocked'],
    APPROVAL_DENIED:               _STATUS['command.error.type']['approval.denied'],
    OPERATION_TIMEOUT:             _STATUS['command.error.type']['operation.timeout'],
    INVALID_INTENT:                _STATUS['command.error.type']['invalid.intent'],
    MISSING_OPERATOR_ID:           _STATUS['command.error.type']['missing.g8e.id'],
    PARTIAL_IAM_UPDATE_FAILED:     _STATUS['command.error.type']['partial.iam.update.failed'],
    PARTIAL_IAM_DETACH_FAILED:     _STATUS['command.error.type']['partial.iam.detach.failed'],
    RESTORE_FILE_FAILED:           _STATUS['command.error.type']['restore.file.failed'],
    FETCH_FILE_DIFF_FAILED:        _STATUS['command.error.type']['fetch.file.diff.failed'],
    FETCH_LOGS_FAILED:             _STATUS['command.error.type']['fetch.logs.failed'],
    FETCH_HISTORY_FAILED:          _STATUS['command.error.type']['fetch.history.failed'],
    FETCH_FILE_HISTORY_FAILED:     _STATUS['command.error.type']['fetch.file.history.failed'],
    PORT_CHECK_FAILED:             _STATUS['command.error.type']['port.check.failed'],
    APPROVAL_TIMEOUT:              _STATUS['command.error.type']['approval.timeout'],
    PERMISSION_ERROR:              _STATUS['command.error.type']['permission.error'],
});

/**
 * System Health Status
 * Used by console metrics to report health of platform components.
 * Canonical values from shared/constants/status.json system.health.
 */
export const SystemHealth = Object.freeze({
    HEALTHY:   _STATUS['system.health']['healthy'],
    DEGRADED:  _STATUS['system.health']['degraded'],
    UNHEALTHY: _STATUS['system.health']['unhealthy'],
    UNKNOWN:   null,
});

export const Environment = Object.freeze({
    PRODUCTION: _STATUS['environment']['production'],
    DEV:        _STATUS['environment']['dev'],
    TEST:       _STATUS['environment']['test'],
});

/**
 * LLM Provider identifiers
 * Values written to the platform_settings DB document and read by G8EE.
 * Must match g8ee's LLMProvider enum in app/constants/config.py exactly.
 */
export const LLMProvider = Object.freeze({
    OPENAI:    'openai',
    OLLAMA:    'ollama',
    GEMINI:    'gemini',
    ANTHROPIC: 'anthropic',
});

/**
 * Search Provider identifiers
 * Values written to the platform_settings DB document and read by G8EE.
 */
export const SearchProvider = Object.freeze({
    GOOGLE_VERTEX: 'google_vertex',
});

/**
 * Gemini model identifiers.
 * Sourced from shared/constants/status.json llm.models.gemini.
 * Must match g8ee's constants/settings.py GEMINI_* constants exactly.
 */
export const GeminiModel = Object.freeze({
    PRO:         _STATUS['llm.models']['gemini']['3.1.pro'],
    FLASH:       _STATUS['llm.models']['gemini']['3.1.flash'],
    FLASH_LITE:  _STATUS['llm.models']['gemini']['3.1.flash.lite'],
});

/**
 * OpenAI model identifiers.
 * Sourced from shared/constants/status.json llm.models.openai.
 * Must match g8ee's constants/settings.py OPENAI_* constants exactly.
 */
export const OpenAIModel = Object.freeze({
    GPT_5_4_THINKING: _STATUS['llm.models']['openai']['gpt.5.4.thinking'],
    GPT_5_4_INSTANT:  _STATUS['llm.models']['openai']['gpt.5.4.instant'],
    GPT_5_4_MINI:     _STATUS['llm.models']['openai']['gpt.5.4.mini'],
});

/**
 * Anthropic model identifiers.
 * Sourced from shared/constants/status.json llm.models.anthropic.
 * Must match g8ee's constants/settings.py ANTHROPIC_* and CLAUDE_* constants exactly.
 */
export const AnthropicModel = Object.freeze({
    ANTHROPIC_CLAUDE_OPUS_4_6:   _STATUS['llm.models']['anthropic']['claude.4.6.opus'],
    ANTHROPIC_CLAUDE_SONNET_4_6: _STATUS['llm.models']['anthropic']['claude.4.6.sonnet'],
    ANTHROPIC_CLAUDE_HAIKU_4_5: _STATUS['llm.models']['anthropic']['claude.4.5.haiku'],
});

/**
 * Ollama model identifiers.
 * Sourced from shared/constants/status.json llm.models.ollama.
 * Must match g8ee's constants/settings.py OLLAMA_* constants exactly.
 */
export const OllamaModel = Object.freeze({
    QWEN3_5_122B:    _STATUS['llm.models']['ollama']['qwen3.5.122b'],
    GLM_5_1:         _STATUS['llm.models']['ollama']['glm.5.1'],
    GEMMA4_26B:      _STATUS['llm.models']['ollama']['gemma4.26b'],
    NEMOTRON_3_30B:  _STATUS['llm.models']['ollama']['nemotron.3.30b'],
    LLAMA_3_2_3B:    _STATUS['llm.models']['ollama']['llama.3.2.3b'],
    QWEN3_5_2B:      _STATUS['llm.models']['ollama']['qwen3.5.2b'],
});

export const PROVIDER_MODELS = Object.freeze({
    [LLMProvider.GEMINI]: {
        primary: [
            { id: GeminiModel.PRO, label: 'Gemini 3.1 Pro' },
        ],
        assistant: [
            { id: GeminiModel.FLASH, label: 'Gemini 3.1 Flash' },
        ],
        lite: [
            { id: GeminiModel.FLASH_LITE, label: 'Gemini 3.1 Flash Lite' },
        ],
        defaultPrimary: GeminiModel.PRO,
        defaultAssistant: GeminiModel.FLASH,
        defaultLite: GeminiModel.FLASH_LITE,
    },
    [LLMProvider.ANTHROPIC]: {
        primary: [
            { id: AnthropicModel.ANTHROPIC_CLAUDE_OPUS_4_6, label: 'Claude Opus 4.6' },
        ],
        assistant: [
            { id: AnthropicModel.ANTHROPIC_CLAUDE_SONNET_4_6, label: 'Claude Sonnet 4.6' },
        ],
        lite: [
            { id: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5, label: 'Claude Haiku 4.5' },
        ],
        defaultPrimary: AnthropicModel.ANTHROPIC_CLAUDE_OPUS_4_6,
        defaultAssistant: AnthropicModel.ANTHROPIC_CLAUDE_SONNET_4_6,
        defaultLite: AnthropicModel.ANTHROPIC_CLAUDE_HAIKU_4_5,
    },
    [LLMProvider.OPENAI]: {
        primary: [
            { id: OpenAIModel.GPT_5_4_THINKING, label: 'GPT-5.4 Thinking' },
        ],
        assistant: [
            { id: OpenAIModel.GPT_5_4_INSTANT, label: 'GPT-5.4 Instant' },
        ],
        lite: [
            { id: OpenAIModel.GPT_5_4_MINI, label: 'GPT-5.4 Mini' },
        ],
        defaultPrimary: OpenAIModel.GPT_5_4_THINKING,
        defaultAssistant: OpenAIModel.GPT_5_4_INSTANT,
        defaultLite: OpenAIModel.GPT_5_4_MINI,
    },
    [LLMProvider.OLLAMA]: {
        primary: [
            { id: OllamaModel.QWEN3_5_122B, label: 'Qwen 3.5 122B' },
            { id: OllamaModel.GLM_5_1, label: 'GLM 5.1' },
        ],
        assistant: [
            { id: OllamaModel.GEMMA4_26B, label: 'Gemma 4 26B' },
            { id: OllamaModel.NEMOTRON_3_30B, label: 'Nemotron 3 30B' },
        ],
        lite: [
            { id: OllamaModel.LLAMA_3_2_3B, label: 'Llama 3.2 3B' },
            { id: OllamaModel.QWEN3_5_2B, label: 'Qwen 3.5 2B' },
        ],
        defaultPrimary: OllamaModel.QWEN3_5_122B,
        defaultAssistant: OllamaModel.GEMMA4_26B,
        defaultLite: OllamaModel.LLAMA_3_2_3B,
    },
});

/**
 * Source Component identifiers
 * Identifies which component originated a message or record.
 * Canonical values from shared/constants/status.json component.name.
 */
export const SourceComponent = Object.freeze({
    G8EE:  _STATUS['component.name']['g8ee'],
    G8EO:  _STATUS['component.name']['g8eo'],
    G8ED:  _STATUS['component.name']['g8ed'],
});
