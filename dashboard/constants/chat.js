// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { _STATUS, _EVENTS, _MSG } from './shared.js';

/**
 * Chat Constants
 * Lifecycle states, event types, senders, and stream chunk types
 * for the AI chat and investigation conversation domain.
 * Canonical values are sourced from protocol/constants/*.json.
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
 * Mirrors: protocol/constants/events.json
 */
export const EventType = Object.freeze({
    // App/Case/Investigation
    CASE_CREATED:                  _EVENTS['app']['case']['created'],
    CASE_UPDATED:                  _EVENTS['app']['case']['updated'],
    INVESTIGATION_CREATED:         _EVENTS['app']['investigation']['created'],

    // Chat Messages (investigation conversation history)
    USER_MESSAGE:                  _EVENTS['app']['investigation']['chat']['message']['user'],
    AI_RESPONSE:                   _EVENTS['app']['investigation']['chat']['message']['ai'],
    SYSTEM_MESSAGE:                _EVENTS['app']['investigation']['chat']['message']['system'],

    // Operator Commands
    OPERATOR_COMMAND_REQUESTED:    _EVENTS['operator']['command']['requested'],
    OPERATOR_COMMAND_STARTED:      _EVENTS['operator']['command']['started'],
    OPERATOR_COMMAND_COMPLETED:    _EVENTS['operator']['command']['completed'],
    OPERATOR_COMMAND_FAILED:       _EVENTS['operator']['command']['failed'],
    OPERATOR_COMMAND_CANCELLED:    _EVENTS['operator']['command']['cancelled'],
    OPERATOR_COMMAND_EXECUTION:    _EVENTS['operator']['command']['execution'],
    OPERATOR_COMMAND_RESULT:       _EVENTS['operator']['command']['result'],

    // Approvals
    OPERATOR_APPROVAL_REQUEST:     _EVENTS['operator']['command']['approval']['requested'],
    OPERATOR_APPROVAL_GRANTED:     _EVENTS['operator']['command']['approval']['granted'],
    OPERATOR_APPROVAL_REJECTED:    _EVENTS['operator']['command']['approval']['rejected'],
    OPERATOR_APPROVAL_PREPARING:    _EVENTS['operator']['command']['approval']['preparing'],

    // File/Edit
    OPERATOR_FILE_EDIT_REQUESTED:  _EVENTS['operator']['file']['edit']['requested'],
    OPERATOR_FILE_EDIT_COMPLETED:  _EVENTS['operator']['file']['edit']['completed'],
    OPERATOR_FILE_EDIT_FAILED:     _EVENTS['operator']['file']['edit']['failed'],
    OPERATOR_FILE_EDIT_TIMEOUT:    _EVENTS['operator']['file']['edit']['timeout'],
    FILE_EDIT_APPROVAL_REQUEST:    _EVENTS['operator']['file']['edit']['approval']['requested'],
    FILE_EDIT_APPROVAL_GRANTED:    _EVENTS['operator']['file']['edit']['approval']['granted'],
    FILE_EDIT_APPROVAL_REJECTED:   _EVENTS['operator']['file']['edit']['approval']['rejected'],
    FILE_EDIT_APPROVAL_FEEDBACK:   _EVENTS['operator']['file']['edit']['approval']['feedback'],

    // Intent
    INTENT_APPROVAL_REQUEST:       _EVENTS['operator']['intent']['approval']['requested'],
    INTENT_APPROVAL_GRANTED:       _EVENTS['operator']['intent']['approval']['granted'],
    INTENT_APPROVAL_REJECTED:      _EVENTS['operator']['intent']['approval']['rejected'],

    // AI/LLM Iterations
    LLM_CHAT_ITERATION_STARTED:    _EVENTS['ai']['llm']['chat']['iteration']['started'],
    LLM_CHAT_ITERATION_COMPLETED:  _EVENTS['ai']['llm']['chat']['iteration']['completed'],
    LLM_CHAT_ITERATION_FAILED:     _EVENTS['ai']['llm']['chat']['iteration']['failed'],
    LLM_CHAT_ITERATION_TEXT_CHUNK: _EVENTS['ai']['llm']['chat']['iteration']['text']['chunk']['received'],

    // Platform
    SYSTEM_NOTIFICATION:           _EVENTS['platform']['notification'],

    EVENT_SOURCE_USER_CHAT:     _MSG['message']['sender']['user']['chat'],
    EVENT_SOURCE_USER_TERMINAL: _MSG['message']['sender']['user']['terminal'],
    EVENT_SOURCE_AI_PRIMARY:    _MSG['message']['sender']['ai']['primary'],
    EVENT_SOURCE_AI_ASSISTANT:  _MSG['message']['sender']['ai']['assistant'],
    EVENT_SOURCE_SYSTEM:        _MSG['message']['sender']['system'],
});


/**
 * Stream Chunk Types
 * Types of chunks emitted by the G8eAgent streaming pipeline.
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
