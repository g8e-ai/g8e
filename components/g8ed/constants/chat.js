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

import { _STATUS, _EVENTS, _MSG } from './shared.js';

/**
 * Chat Constants
 * Lifecycle states, event types, senders, and stream chunk types
 * for the AI chat and investigation conversation domain.
 * Canonical values are sourced from shared/constants/*.json.
 */

/**
 * Conversation Status
 * Lifecycle states of an AI chat conversation session.
 */
export const ConversationStatus = Object.freeze({
    ACTIVE:    _STATUS['conversation.status']['active'],
    INACTIVE:  _STATUS['conversation.status']['inactive'],
    COMPLETED: _STATUS['conversation.status']['completed'],
});

/**
 * Event Types
 * Canonical identifiers for all events recorded in investigation history.
 * Mirrors: shared/constants/events.json
 */
export const EventType = Object.freeze({
    // App/Case/Investigation
    CASE_CREATED:                  'g8e.v1.app.case.created',
    CASE_UPDATED:                  'g8e.v1.app.case.updated',
    INVESTIGATION_CREATED:         'g8e.v1.app.investigation.created',

    // Chat Messages (investigation conversation history)
    USER_MESSAGE:                  _EVENTS['app']['investigation']['chat']['message']['user'],
    AI_RESPONSE:                   _EVENTS['app']['investigation']['chat']['message']['ai'],
    SYSTEM_MESSAGE:                _EVENTS['app']['investigation']['chat']['message']['system'],

    // Operator Commands
    OPERATOR_COMMAND_REQUESTED:    'g8e.v1.operator.command.requested',
    OPERATOR_COMMAND_STARTED:      'g8e.v1.operator.command.started',
    OPERATOR_COMMAND_COMPLETED:    'g8e.v1.operator.command.completed',
    OPERATOR_COMMAND_FAILED:       'g8e.v1.operator.command.failed',
    OPERATOR_COMMAND_CANCELLED:    'g8e.v1.operator.command.cancelled',
    OPERATOR_COMMAND_EXECUTION:    'g8e.v1.operator.command.output.received',
    OPERATOR_COMMAND_RESULT:       'g8e.v1.operator.command.completed',

    // Approvals
    OPERATOR_APPROVAL_REQUEST:     'g8e.v1.operator.command.approval.requested',
    OPERATOR_APPROVAL_GRANTED:     'g8e.v1.operator.command.approval.granted',
    OPERATOR_APPROVAL_REJECTED:    'g8e.v1.operator.command.approval.rejected',
    OPERATOR_APPROVAL_PREPARING:    'g8e.v1.operator.command.approval.requested',

    // File/Edit
    OPERATOR_FILE_EDIT_REQUESTED:  'g8e.v1.operator.file.edit.requested',
    OPERATOR_FILE_EDIT_COMPLETED:  'g8e.v1.operator.file.edit.completed',
    OPERATOR_FILE_EDIT_FAILED:     'g8e.v1.operator.file.edit.failed',
    OPERATOR_FILE_EDIT_TIMEOUT:    'g8e.v1.operator.file.edit.failed',
    FILE_EDIT_APPROVAL_REQUEST:    'g8e.v1.operator.file.edit.approval.requested',
    FILE_EDIT_APPROVAL_GRANTED:    'g8e.v1.operator.file.edit.approval.granted',
    FILE_EDIT_APPROVAL_REJECTED:   'g8e.v1.operator.file.edit.approval.rejected',
    FILE_EDIT_APPROVAL_FEEDBACK:   'g8e.v1.operator.file.edit.approval.feedback',

    // Intent
    INTENT_APPROVAL_REQUEST:       'g8e.v1.operator.intent.approval.requested',
    INTENT_APPROVAL_GRANTED:       'g8e.v1.operator.intent.granted',
    INTENT_APPROVAL_REJECTED:      'g8e.v1.operator.intent.denied',

    // AI/LLM Iterations
    LLM_CHAT_ITERATION_STARTED:    'g8e.v1.ai.llm.chat.iteration.started',
    LLM_CHAT_ITERATION_COMPLETED:  'g8e.v1.ai.llm.chat.iteration.completed',
    LLM_CHAT_ITERATION_FAILED:     'g8e.v1.ai.llm.chat.iteration.failed',
    LLM_CHAT_ITERATION_TEXT_CHUNK: 'g8e.v1.ai.llm.chat.iteration.text.chunk.received',

    // Platform
    SYSTEM_NOTIFICATION:           'g8e.v1.platform.notification',

    EVENT_SOURCE_USER_CHAT:     _MSG['message']['sender']['user']['chat'],
    EVENT_SOURCE_USER_TERMINAL: _MSG['message']['sender']['user']['terminal'],
    EVENT_SOURCE_AI_PRIMARY:    _MSG['message']['sender']['ai']['primary'],
    EVENT_SOURCE_AI_ASSISTANT:  _MSG['message']['sender']['ai']['assistant'],
    EVENT_SOURCE_SYSTEM:        _MSG['message']['sender']['system'],
});


/**
 * Stream Chunk Types
 * Types of chunks emitted by the g8eEngine streaming pipeline.
 */
export const StreamChunkType = Object.freeze({
    TEXT:            'text',
    THINKING:        'thinking',
    THINKING_UPDATE: 'thinking.update',
    THINKING_END:    'thinking.end',
    TOOL_CALL:       'tool.call',
    TOOL_RESULT:     'tool.result',
    CITATIONS:       'citations',
    COMPLETE:        'complete',
    ERROR:           'error',
    RETRY:           'retry',
});
