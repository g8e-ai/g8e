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

import { _EVENTS } from './shared.js';

/**
 * Wire Event Type Constants
 * Canonical values loaded from shared/constants/events.json.
 * That file is the single source of truth shared across g8ee, g8eo, and VSOD.
 *
 * Mirrors: components/g8ee/app/constants/events.py EventType (flat naming)
 */

export const EventType = Object.freeze({

    CASE_CREATED:            _EVENTS['app']['case']['created'],
    CASE_UPDATED:            _EVENTS['app']['case']['updated'],
    CASE_ASSIGNED:           _EVENTS['app']['case']['assigned'],
    CASE_ESCALATED:          _EVENTS['app']['case']['escalated'],
    CASE_RESOLVED:           _EVENTS['app']['case']['resolved'],
    CASE_CLOSED:             _EVENTS['app']['case']['closed'],
    CASE_SELECTED:           _EVENTS['app']['case']['selected'],
    CASE_CLEARED:            _EVENTS['app']['case']['cleared'],
    CASE_SWITCHED:           _EVENTS['app']['case']['switched'],
    CASE_CREATION_REQUESTED: _EVENTS['app']['case']['creation']['requested'],
    CASE_UPDATE_REQUESTED:   _EVENTS['app']['case']['update']['requested'],

    TASK_CREATED:   _EVENTS['app']['task']['created'],
    TASK_UPDATED:   _EVENTS['app']['task']['updated'],
    TASK_ASSIGNED:  _EVENTS['app']['task']['assigned'],
    TASK_STARTED:   _EVENTS['app']['task']['started'],
    TASK_COMPLETED: _EVENTS['app']['task']['completed'],
    TASK_FAILED:    _EVENTS['app']['task']['failed'],

    INVESTIGATION_CREATED:        _EVENTS['app']['investigation']['created'],
    INVESTIGATION_UPDATED:        _EVENTS['app']['investigation']['updated'],
    INVESTIGATION_STARTED:        _EVENTS['app']['investigation']['started'],
    INVESTIGATION_CLOSED:         _EVENTS['app']['investigation']['closed'],
    INVESTIGATION_ESCALATED:      _EVENTS['app']['investigation']['escalated'],
    INVESTIGATION_LIST_REQUESTED: _EVENTS['app']['investigation']['list']['requested'],
    INVESTIGATION_LIST_RECEIVED:  _EVENTS['app']['investigation']['list']['received'],
    INVESTIGATION_LIST_COMPLETED: _EVENTS['app']['investigation']['list']['completed'],
    INVESTIGATION_LIST_FAILED:    _EVENTS['app']['investigation']['list']['failed'],
    INVESTIGATION_LOADED:         _EVENTS['app']['investigation']['loaded'],
    INVESTIGATION_REQUESTED:      _EVENTS['app']['investigation']['requested'],
    INVESTIGATION_STATUS_UPDATED_OPEN:      _EVENTS['app']['investigation']['status']['updated']['open'],
    INVESTIGATION_STATUS_UPDATED_CLOSED:    _EVENTS['app']['investigation']['status']['updated']['closed'],
    INVESTIGATION_STATUS_UPDATED_ESCALATED: _EVENTS['app']['investigation']['status']['updated']['escalated'],
    INVESTIGATION_STATUS_UPDATED_RESOLVED:  _EVENTS['app']['investigation']['status']['updated']['resolved'],

    INVESTIGATION_CHAT_MESSAGE_USER:   _EVENTS['app']['investigation']['chat']['message']['user'],
    INVESTIGATION_CHAT_MESSAGE_AI:     _EVENTS['app']['investigation']['chat']['message']['ai'],
    INVESTIGATION_CHAT_MESSAGE_SYSTEM: _EVENTS['app']['investigation']['chat']['message']['system'],

    OPERATOR_HEARTBEAT_SENT:      _EVENTS['operator']['heartbeat']['sent'],
    OPERATOR_HEARTBEAT_REQUESTED: _EVENTS['operator']['heartbeat']['requested'],
    OPERATOR_HEARTBEAT_RECEIVED:  _EVENTS['operator']['heartbeat']['received'],
    OPERATOR_HEARTBEAT_MISSED:    _EVENTS['operator']['heartbeat']['missed'],
    OPERATOR_SHUTDOWN_REQUESTED:    _EVENTS['operator']['shutdown']['requested'],
    OPERATOR_SHUTDOWN_ACKNOWLEDGED: _EVENTS['operator']['shutdown']['acknowledged'],
    OPERATOR_PANEL_LIST_UPDATED:  _EVENTS['operator']['panel']['list']['updated'],
    OPERATOR_API_KEY_REFRESHED:   _EVENTS['operator']['api']['key']['refreshed'],
    OPERATOR_DEVICE_REGISTERED:   _EVENTS['operator']['device']['registered'],

    OPERATOR_STATUS_UPDATED_ACTIVE:      _EVENTS['operator']['status']['updated']['active'],
    OPERATOR_STATUS_UPDATED_AVAILABLE:   _EVENTS['operator']['status']['updated']['available'],
    OPERATOR_STATUS_UPDATED_UNAVAILABLE: _EVENTS['operator']['status']['updated']['unavailable'],
    OPERATOR_STATUS_UPDATED_BOUND:       _EVENTS['operator']['status']['updated']['bound'],
    OPERATOR_STATUS_UPDATED_OFFLINE:     _EVENTS['operator']['status']['updated']['offline'],
    OPERATOR_STATUS_UPDATED_STALE:       _EVENTS['operator']['status']['updated']['stale'],
    OPERATOR_STATUS_UPDATED_STOPPED:     _EVENTS['operator']['status']['updated']['stopped'],
    OPERATOR_STATUS_UPDATED_TERMINATED:  _EVENTS['operator']['status']['updated']['terminated'],

    OPERATOR_COMMAND_REQUESTED:         _EVENTS['operator']['command']['requested'],
    OPERATOR_COMMAND_STARTED:           _EVENTS['operator']['command']['started'],
    OPERATOR_COMMAND_COMPLETED:         _EVENTS['operator']['command']['completed'],
    OPERATOR_COMMAND_FAILED:            _EVENTS['operator']['command']['failed'],
    OPERATOR_COMMAND_CANCELLED:         _EVENTS['operator']['command']['cancelled'],
    OPERATOR_COMMAND_EXECUTION:         _EVENTS['operator']['command']['execution'],
    OPERATOR_COMMAND_RESULT:            _EVENTS['operator']['command']['result'],
    OPERATOR_COMMAND_OUTPUT_RECEIVED:   _EVENTS['operator']['command']['output']['received'],
    OPERATOR_COMMAND_STATUS_UPDATED_QUEUED:    _EVENTS['operator']['command']['status']['updated']['queued'],
    OPERATOR_COMMAND_STATUS_UPDATED_RUNNING:   _EVENTS['operator']['command']['status']['updated']['running'],
    OPERATOR_COMMAND_STATUS_UPDATED_COMPLETED: _EVENTS['operator']['command']['status']['updated']['completed'],
    OPERATOR_COMMAND_STATUS_UPDATED_FAILED:    _EVENTS['operator']['command']['status']['updated']['failed'],
    OPERATOR_COMMAND_STATUS_UPDATED_CANCELLED: _EVENTS['operator']['command']['status']['updated']['cancelled'],
    OPERATOR_COMMAND_CANCEL_REQUESTED:    _EVENTS['operator']['command']['cancel']['requested'],
    OPERATOR_COMMAND_CANCEL_ACKNOWLEDGED: _EVENTS['operator']['command']['cancel']['acknowledged'],
    OPERATOR_COMMAND_CANCEL_FAILED:       _EVENTS['operator']['command']['cancel']['failed'],
    OPERATOR_COMMAND_APPROVAL_PREPARING:  _EVENTS['operator']['command']['approval']['preparing'],
    OPERATOR_COMMAND_APPROVAL_REQUESTED:  _EVENTS['operator']['command']['approval']['requested'],
    OPERATOR_COMMAND_APPROVAL_GRANTED:    _EVENTS['operator']['command']['approval']['granted'],
    OPERATOR_COMMAND_APPROVAL_REJECTED:   _EVENTS['operator']['command']['approval']['rejected'],

    OPERATOR_FILE_EDIT_REQUESTED:          _EVENTS['operator']['file']['edit']['requested'],
    OPERATOR_FILE_EDIT_STARTED:            _EVENTS['operator']['file']['edit']['started'],
    OPERATOR_FILE_EDIT_COMPLETED:          _EVENTS['operator']['file']['edit']['completed'],
    OPERATOR_FILE_EDIT_FAILED:             _EVENTS['operator']['file']['edit']['failed'],
    OPERATOR_FILE_EDIT_TIMEOUT:            _EVENTS['operator']['file']['edit']['timeout'],
    OPERATOR_FILE_EDIT_APPROVAL_REQUESTED: _EVENTS['operator']['file']['edit']['approval']['requested'],
    OPERATOR_FILE_EDIT_APPROVAL_GRANTED:   _EVENTS['operator']['file']['edit']['approval']['granted'],
    OPERATOR_FILE_EDIT_APPROVAL_REJECTED:  _EVENTS['operator']['file']['edit']['approval']['rejected'],

    OPERATOR_FILE_HISTORY_FETCH_STARTED:   _EVENTS['operator']['file']['history']['fetch']['started'],
    OPERATOR_FILE_HISTORY_FETCH_REQUESTED: _EVENTS['operator']['file']['history']['fetch']['requested'],
    OPERATOR_FILE_HISTORY_FETCH_RECEIVED:  _EVENTS['operator']['file']['history']['fetch']['received'],
    OPERATOR_FILE_HISTORY_FETCH_COMPLETED: _EVENTS['operator']['file']['history']['fetch']['completed'],
    OPERATOR_FILE_HISTORY_FETCH_FAILED:    _EVENTS['operator']['file']['history']['fetch']['failed'],

    OPERATOR_FILE_DIFF_FETCH_STARTED:   _EVENTS['operator']['file']['diff']['fetch']['started'],
    OPERATOR_FILE_DIFF_FETCH_REQUESTED: _EVENTS['operator']['file']['diff']['fetch']['requested'],
    OPERATOR_FILE_DIFF_FETCH_RECEIVED:  _EVENTS['operator']['file']['diff']['fetch']['received'],
    OPERATOR_FILE_DIFF_FETCH_COMPLETED: _EVENTS['operator']['file']['diff']['fetch']['completed'],
    OPERATOR_FILE_DIFF_FETCH_FAILED:    _EVENTS['operator']['file']['diff']['fetch']['failed'],

    OPERATOR_FILE_RESTORE_REQUESTED: _EVENTS['operator']['file']['restore']['requested'],
    OPERATOR_FILE_RESTORE_RECEIVED:  _EVENTS['operator']['file']['restore']['received'],
    OPERATOR_FILE_RESTORE_COMPLETED: _EVENTS['operator']['file']['restore']['completed'],
    OPERATOR_FILE_RESTORE_FAILED:    _EVENTS['operator']['file']['restore']['failed'],

    OPERATOR_FILESYSTEM_LIST_STARTED:   _EVENTS['operator']['filesystem']['list']['started'],
    OPERATOR_FILESYSTEM_LIST_REQUESTED: _EVENTS['operator']['filesystem']['list']['requested'],
    OPERATOR_FILESYSTEM_LIST_RECEIVED:  _EVENTS['operator']['filesystem']['list']['received'],
    OPERATOR_FILESYSTEM_LIST_COMPLETED: _EVENTS['operator']['filesystem']['list']['completed'],
    OPERATOR_FILESYSTEM_LIST_FAILED:    _EVENTS['operator']['filesystem']['list']['failed'],

    OPERATOR_FILESYSTEM_READ_STARTED:   _EVENTS['operator']['filesystem']['read']['started'],
    OPERATOR_FILESYSTEM_READ_REQUESTED: _EVENTS['operator']['filesystem']['read']['requested'],
    OPERATOR_FILESYSTEM_READ_RECEIVED:  _EVENTS['operator']['filesystem']['read']['received'],
    OPERATOR_FILESYSTEM_READ_COMPLETED: _EVENTS['operator']['filesystem']['read']['completed'],
    OPERATOR_FILESYSTEM_READ_FAILED:    _EVENTS['operator']['filesystem']['read']['failed'],

    OPERATOR_LOGS_FETCH_REQUESTED: _EVENTS['operator']['logs']['fetch']['requested'],
    OPERATOR_LOGS_FETCH_RECEIVED:  _EVENTS['operator']['logs']['fetch']['received'],
    OPERATOR_LOGS_FETCH_COMPLETED: _EVENTS['operator']['logs']['fetch']['completed'],
    OPERATOR_LOGS_FETCH_FAILED:    _EVENTS['operator']['logs']['fetch']['failed'],

    OPERATOR_HISTORY_FETCH_REQUESTED: _EVENTS['operator']['history']['fetch']['requested'],
    OPERATOR_HISTORY_FETCH_RECEIVED:  _EVENTS['operator']['history']['fetch']['received'],
    OPERATOR_HISTORY_FETCH_COMPLETED: _EVENTS['operator']['history']['fetch']['completed'],
    OPERATOR_HISTORY_FETCH_FAILED:    _EVENTS['operator']['history']['fetch']['failed'],

    OPERATOR_INTENT_GRANTED:            _EVENTS['operator']['intent']['granted'],
    OPERATOR_INTENT_DENIED:             _EVENTS['operator']['intent']['denied'],
    OPERATOR_INTENT_REVOKED:            _EVENTS['operator']['intent']['revoked'],
    OPERATOR_INTENT_APPROVAL_REQUESTED: _EVENTS['operator']['intent']['approval']['requested'],
    OPERATOR_INTENT_APPROVAL_GRANTED:   _EVENTS['operator']['intent']['approval']['granted'],
    OPERATOR_INTENT_APPROVAL_REJECTED:  _EVENTS['operator']['intent']['approval']['rejected'],

    OPERATOR_NETWORK_PING_REQUESTED: _EVENTS['operator']['network']['ping']['requested'],
    OPERATOR_NETWORK_PING_RECEIVED:  _EVENTS['operator']['network']['ping']['received'],
    OPERATOR_NETWORK_PING_COMPLETED: _EVENTS['operator']['network']['ping']['completed'],
    OPERATOR_NETWORK_PING_FAILED:    _EVENTS['operator']['network']['ping']['failed'],

    OPERATOR_NETWORK_PORT_CHECK_REQUESTED: _EVENTS['operator']['network']['port']['check']['requested'],
    OPERATOR_NETWORK_PORT_CHECK_STARTED:   _EVENTS['operator']['network']['port']['check']['started'],
    OPERATOR_NETWORK_PORT_CHECK_RECEIVED:  _EVENTS['operator']['network']['port']['check']['received'],
    OPERATOR_NETWORK_PORT_CHECK_COMPLETED: _EVENTS['operator']['network']['port']['check']['completed'],
    OPERATOR_NETWORK_PORT_CHECK_FAILED:    _EVENTS['operator']['network']['port']['check']['failed'],

    OPERATOR_AUDIT_USER_RECORDED:                   _EVENTS['operator']['audit']['user']['recorded'],
    OPERATOR_AUDIT_AI_RECORDED:                     _EVENTS['operator']['audit']['ai']['recorded'],
    OPERATOR_AUDIT_COMMAND_RECORDED:                _EVENTS['operator']['audit']['command']['recorded'],
    OPERATOR_AUDIT_DIRECT_COMMAND_RECORDED:         _EVENTS['operator']['audit']['direct']['command']['recorded'],
    OPERATOR_AUDIT_DIRECT_COMMAND_RESULT_RECORDED:  _EVENTS['operator']['audit']['direct']['command']['result']['recorded'],

    OPERATOR_BOOTSTRAP_REQUESTED:      _EVENTS['operator']['bootstrap']['requested'],
    OPERATOR_BOOTSTRAP_RECEIVED:       _EVENTS['operator']['bootstrap']['received'],
    OPERATOR_BOOTSTRAP_COMPLETED:      _EVENTS['operator']['bootstrap']['completed'],
    OPERATOR_BOOTSTRAP_FAILED:         _EVENTS['operator']['bootstrap']['failed'],
    OPERATOR_BOOTSTRAP_CONFIG_RECEIVED: _EVENTS['operator']['bootstrap']['config']['received'],

    OPERATOR_BOUND:   _EVENTS['operator']['bound'],
    OPERATOR_UNBOUND: _EVENTS['operator']['unbound'],

    OPERATOR_TERMINAL_THINKING_APPEND:   _EVENTS['operator']['terminal']['thinking']['append'],
    OPERATOR_TERMINAL_THINKING_COMPLETE: _EVENTS['operator']['terminal']['thinking']['complete'],
    OPERATOR_TERMINAL_APPROVAL_DENIED:   _EVENTS['operator']['terminal']['approval']['denied'],
    OPERATOR_TERMINAL_AUTH_STATE_CHANGED: _EVENTS['operator']['terminal']['auth']['state']['changed'],

    OPERATOR_MCP_TOOLS_CALL:    _EVENTS['operator']['mcp']['tools']['call'],
    OPERATOR_MCP_TOOLS_RESULT:  _EVENTS['operator']['mcp']['tools']['result'],
    OPERATOR_MCP_RESOURCES_LIST: _EVENTS['operator']['mcp']['resources']['list'],
    OPERATOR_MCP_RESOURCES_READ:  _EVENTS['operator']['mcp']['resources']['read'],
    OPERATOR_MCP_RESOURCES_RESULT: _EVENTS['operator']['mcp']['resources']['result'],

    LLM_CONFIG_REQUESTED: _EVENTS['ai']['llm']['config']['requested'],
    LLM_CONFIG_RECEIVED:  _EVENTS['ai']['llm']['config']['received'],
    LLM_CONFIG_FAILED:    _EVENTS['ai']['llm']['config']['failed'],

    LLM_CHAT_SUBMITTED:    _EVENTS['ai']['llm']['chat']['submitted'],
    LLM_CHAT_STOP_SHOW:    _EVENTS['ai']['llm']['chat']['stop']['show'],
    LLM_CHAT_STOP_HIDE:    _EVENTS['ai']['llm']['chat']['stop']['hide'],
    LLM_CHAT_FILTER_EVENT: _EVENTS['ai']['llm']['chat']['filter']['event'],

    LLM_LIFECYCLE_REQUESTED:     _EVENTS['ai']['llm']['lifecycle']['requested'],
    LLM_LIFECYCLE_STARTED:       _EVENTS['ai']['llm']['lifecycle']['started'],
    LLM_LIFECYCLE_COMPLETED:     _EVENTS['ai']['llm']['lifecycle']['completed'],
    LLM_LIFECYCLE_FAILED:        _EVENTS['ai']['llm']['lifecycle']['failed'],
    LLM_LIFECYCLE_STOPPED:       _EVENTS['ai']['llm']['lifecycle']['stopped'],
    LLM_LIFECYCLE_ERROR_OCCURRED: _EVENTS['ai']['llm']['lifecycle']['error']['occurred'],

    LLM_TOOL_G8E_WEB_SEARCH_REQUESTED: _EVENTS['ai']['llm']['tool']['g8e']['web']['search']['requested'],
    LLM_TOOL_G8E_WEB_SEARCH_RECEIVED:  _EVENTS['ai']['llm']['tool']['g8e']['web']['search']['received'],
    LLM_TOOL_G8E_WEB_SEARCH_COMPLETED: _EVENTS['ai']['llm']['tool']['g8e']['web']['search']['completed'],
    LLM_TOOL_G8E_WEB_SEARCH_FAILED:    _EVENTS['ai']['llm']['tool']['g8e']['web']['search']['failed'],

    LLM_CHAT_MESSAGE_SENT:              _EVENTS['ai']['llm']['chat']['message']['sent'],
    LLM_CHAT_MESSAGE_REPLAYED:          _EVENTS['ai']['llm']['chat']['message']['replayed'],
    LLM_CHAT_MESSAGE_PROCESSING_FAILED: _EVENTS['ai']['llm']['chat']['message']['processing']['failed'],
    LLM_CHAT_MESSAGE_DEAD_LETTERED:     _EVENTS['ai']['llm']['chat']['message']['dead']['lettered'],

    LLM_CHAT_ITERATION_STARTED:               _EVENTS['ai']['llm']['chat']['iteration']['started'],
    LLM_CHAT_ITERATION_COMPLETED:             _EVENTS['ai']['llm']['chat']['iteration']['completed'],
    LLM_CHAT_ITERATION_FAILED:                _EVENTS['ai']['llm']['chat']['iteration']['failed'],
    LLM_CHAT_ITERATION_STOPPED:               _EVENTS['ai']['llm']['chat']['iteration']['stopped'],
    LLM_CHAT_ITERATION_THINKING_STARTED:      _EVENTS['ai']['llm']['chat']['iteration']['thinking']['started'],
    LLM_CHAT_ITERATION_CITATIONS_RECEIVED:    _EVENTS['ai']['llm']['chat']['iteration']['citations']['received'],
    LLM_CHAT_ITERATION_TEXT_RECEIVED:         _EVENTS['ai']['llm']['chat']['iteration']['text']['received'],
    LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED:   _EVENTS['ai']['llm']['chat']['iteration']['text']['chunk']['received'],
    LLM_CHAT_ITERATION_TEXT_COMPLETED:        _EVENTS['ai']['llm']['chat']['iteration']['text']['completed'],
    LLM_CHAT_ITERATION_TEXT_TRUNCATED:        _EVENTS['ai']['llm']['chat']['iteration']['text']['truncated'],
    LLM_CHAT_ITERATION_STREAM_STARTED:        _EVENTS['ai']['llm']['chat']['iteration']['stream']['started'],
    LLM_CHAT_ITERATION_STREAM_DELTA_RECEIVED: _EVENTS['ai']['llm']['chat']['iteration']['stream']['delta']['received'],
    LLM_CHAT_ITERATION_STREAM_COMPLETED:      _EVENTS['ai']['llm']['chat']['iteration']['stream']['completed'],
    LLM_CHAT_ITERATION_STREAM_FAILED:         _EVENTS['ai']['llm']['chat']['iteration']['stream']['failed'],

    TRIBUNAL_SESSION_STARTED:           _EVENTS['ai']['tribunal']['session']['started'],
    TRIBUNAL_SESSION_COMPLETED:         _EVENTS['ai']['tribunal']['session']['completed'],
    TRIBUNAL_SESSION_FAILED:            _EVENTS['ai']['tribunal']['session']['failed'],
    TRIBUNAL_SESSION_FALLBACK_TRIGGERED: _EVENTS['ai']['tribunal']['session']['fallback']['triggered'],

    TRIBUNAL_VOTING_STARTED:            _EVENTS['ai']['tribunal']['voting']['started'],
    TRIBUNAL_VOTING_FAILED:             _EVENTS['ai']['tribunal']['voting']['failed'],
    TRIBUNAL_VOTING_PASS_COMPLETED:     _EVENTS['ai']['tribunal']['voting']['pass']['completed'],
    TRIBUNAL_VOTING_PASS_FAILED:        _EVENTS['ai']['tribunal']['voting']['pass']['failed'],
    TRIBUNAL_VOTING_CONSENSUS_REACHED:     _EVENTS['ai']['tribunal']['voting']['consensus']['reached'],
    TRIBUNAL_VOTING_CONSENSUS_NOT_REACHED: _EVENTS['ai']['tribunal']['voting']['consensus']['not_reached'],
    TRIBUNAL_VOTING_REVIEW_STARTED:     _EVENTS['ai']['tribunal']['voting']['review']['started'],
    TRIBUNAL_VOTING_REVIEW_COMPLETED:   _EVENTS['ai']['tribunal']['voting']['review']['completed'],
    TRIBUNAL_VOTING_REVIEW_FAILED:      _EVENTS['ai']['tribunal']['voting']['review']['failed'],

    AUTH_LOGIN_REQUESTED:  _EVENTS['platform']['auth']['login']['requested'],
    AUTH_LOGIN_SUCCEEDED:  _EVENTS['platform']['auth']['login']['succeeded'],
    AUTH_LOGIN_FAILED:     _EVENTS['platform']['auth']['login']['failed'],
    AUTH_LOGOUT_REQUESTED: _EVENTS['platform']['auth']['logout']['requested'],
    AUTH_LOGOUT_SUCCEEDED: _EVENTS['platform']['auth']['logout']['succeeded'],
    AUTH_LOGOUT_FAILED:    _EVENTS['platform']['auth']['logout']['failed'],
    AUTH_SESSION_VALIDATION_REQUESTED: _EVENTS['platform']['auth']['session']['validation']['requested'],
    AUTH_SESSION_VALIDATION_SUCCEEDED: _EVENTS['platform']['auth']['session']['validation']['succeeded'],
    AUTH_SESSION_VALIDATION_FAILED:    _EVENTS['platform']['auth']['session']['validation']['failed'],

    PLATFORM_USAGE_UPDATED: _EVENTS['platform']['usage']['updated'],

    PLATFORM_SSE_KEEPALIVE_SENT:         _EVENTS['platform']['sse']['keepalive']['sent'],
    PLATFORM_SSE_CONNECTION_ESTABLISHED: _EVENTS['platform']['sse']['connection']['established'],
    PLATFORM_SSE_CONNECTION_OPENED:      _EVENTS['platform']['sse']['connection']['opened'],
    PLATFORM_SSE_CONNECTION_CLOSED:      _EVENTS['platform']['sse']['connection']['closed'],
    PLATFORM_SSE_CONNECTION_FAILED:      _EVENTS['platform']['sse']['connection']['failed'],
    PLATFORM_SSE_CONNECTION_ERROR:       _EVENTS['platform']['sse']['connection']['error'],

    PLATFORM_TERMINAL_OPENED:   _EVENTS['platform']['terminal']['opened'],
    PLATFORM_TERMINAL_MINIMIZED: _EVENTS['platform']['terminal']['minimized'],
    PLATFORM_TERMINAL_MAXIMIZED: _EVENTS['platform']['terminal']['maximized'],
    PLATFORM_TERMINAL_CLOSED:   _EVENTS['platform']['terminal']['closed'],

    PLATFORM_SENTINEL_MODE_CHANGED: _EVENTS['platform']['sentinel']['mode']['changed'],

    AUTH_USER_AUTHENTICATED:   _EVENTS['platform']['auth']['user']['authenticated'],
    AUTH_USER_UNAUTHENTICATED: _EVENTS['platform']['auth']['user']['unauthenticated'],
    AUTH_SESSION_EXPIRED:      _EVENTS['platform']['auth']['session']['expired'],
    AUTH_COMPONENT_INITIALIZED_AUTHSTATE: _EVENTS['platform']['auth']['component']['initialized']['authstate'],
    AUTH_COMPONENT_INITIALIZED_CHAT:      _EVENTS['platform']['auth']['component']['initialized']['chat'],
    AUTH_COMPONENT_INITIALIZED_OPERATOR:  _EVENTS['platform']['auth']['component']['initialized']['operator'],
    AUTH_INFO: _EVENTS['platform']['auth']['info'],

    PLATFORM_EXTERNAL_SERVICE_CONFIGURED: _EVENTS['platform']['external']['service']['configured'],

    PLATFORM_TELEMETRY_HEALTH_REPORTED:      _EVENTS['platform']['telemetry']['health']['reported'],
    PLATFORM_TELEMETRY_PERFORMANCE_RECORDED: _EVENTS['platform']['telemetry']['performance']['recorded'],
    PLATFORM_TELEMETRY_ERROR_LOGGED:         _EVENTS['platform']['telemetry']['error']['logged'],
    PLATFORM_TELEMETRY_AUDIT_LOGGED:         _EVENTS['platform']['telemetry']['audit']['logged'],

    PLATFORM_CONSOLE_LOG_ENTRY_RECEIVED:      _EVENTS['platform']['console']['log']['entry']['received'],
    PLATFORM_CONSOLE_LOG_CONNECTED_CONFIRMED: _EVENTS['platform']['console']['log']['connected']['confirmed'],

    EVENT_SOURCE_USER_CHAT:     _EVENTS['source']['user']['chat'],
    EVENT_SOURCE_USER_TERMINAL: _EVENTS['source']['user']['terminal'],
    EVENT_SOURCE_AI_PRIMARY:    _EVENTS['source']['ai']['primary'],
    EVENT_SOURCE_AI_ASSISTANT:  _EVENTS['source']['ai']['assistant'],
    EVENT_SOURCE_SYSTEM:        _EVENTS['source']['system'],
});

export const SSE_KEEPALIVE_INTERVAL_MS = 20_000;

export const ConnectionState = Object.freeze({
    DISCONNECTED: 'disconnected',
    CONNECTING:   'connecting',
    CONNECTED:    'connected',
    RECONNECTING: 'reconnecting',
    CLOSED:       'closed',
    ERROR:        'error',
});
