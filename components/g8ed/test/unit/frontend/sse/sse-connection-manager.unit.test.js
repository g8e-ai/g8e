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

// @vitest-environment jsdom

/**
 * SSEConnectionManager — handleSSEEvent [FRONTEND - jsdom]
 *
 * Tests the wire-parsing and eventBus dispatch layer of SSEConnectionManager.
 * Covers every routing branch:
 *   - Infrastructure events (keepalive, connection_established) are silently consumed
 *   - Non-infrastructure events without a `data` field are dropped with a warning
 *   - Non-infrastructure events with a `data` field are emitted on the eventBus with
 *     the payload exactly as received from the SSE frame
 *   - Events with a missing or non-string `type` are dropped with a warning
 *
 * Wire format exercised:
 *   g8ee serializes: { type: "...", data: { ... } }
 *   SSEService.sendToLocal writes: data: <json>\n\n
 *   SSEConnectionManager.onmessage parses: JSON.parse(event.data) → calls handleSSEEvent(data)
 *   handleSSEEvent destructures: { type: eventType, data: payload } → eventBus.emit(eventType, payload)
 *
 * Every test in this file drives handleSSEEvent directly with the exact object that
 * JSON.parse(event.data) would produce — no network, no HTTP, no server startup.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { MockEventBus } from '@test/mocks/mock-browser-env.js';
import { EventType } from '@g8ed/public/js/constants/events.js';

class MockEventSource {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSED = 2;

    constructor(url, options = {}) {
        this.url = url;
        this.withCredentials = options.withCredentials || false;
        this.readyState = MockEventSource.CONNECTING;
        this.onopen = null;
        this.onmessage = null;
        this.onerror = null;
        MockEventSource._lastInstance = this;
    }

    close() {
        this.readyState = MockEventSource.CLOSED;
    }

    _simulateOpen() {
        this.readyState = MockEventSource.OPEN;
        if (this.onopen) this.onopen();
    }

    _simulateMessage(data) {
        if (this.onmessage) this.onmessage({ data: JSON.stringify(data) });
    }

    _simulateError(error) {
        if (this.onerror) this.onerror(error || new Error('connection error'));
    }
}

globalThis.EventSource = MockEventSource;

const { SSEConnectionManager } = await import('@g8ed/public/js/utils/sse-connection-manager.js');

function makeManager() {
    const eventBus = new MockEventBus();
    const manager = new SSEConnectionManager(eventBus);
    return { manager, eventBus };
}

// ---------------------------------------------------------------------------
// Infrastructure events
// ---------------------------------------------------------------------------

describe('SSEConnectionManager.handleSSEEvent — infrastructure events [FRONTEND - jsdom]', () => {
    it('does not emit on eventBus for PLATFORM_SSE_CONNECTION_ESTABLISHED', () => {
        const { manager, eventBus } = makeManager();

        const result = manager.handleSSEEvent({
            type: EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED,
            connectionId: 'session_abc',
        });

        expect(result).toEqual({ handled: true, infrastructure: true });
        expect(eventBus.getEmittedEvents()).toHaveLength(0);
    });

    it('does not emit on eventBus for PLATFORM_SSE_KEEPALIVE_SENT', () => {
        const { manager, eventBus } = makeManager();

        const result = manager.handleSSEEvent({
            type: EventType.PLATFORM_SSE_KEEPALIVE_SENT,
            serverTime: Date.now(),
        });

        expect(result).toEqual({ handled: true, infrastructure: true });
        expect(eventBus.getEmittedEvents()).toHaveLength(0);
    });
});

// ---------------------------------------------------------------------------
// Missing / invalid type field
// ---------------------------------------------------------------------------

describe('SSEConnectionManager.handleSSEEvent — invalid type [FRONTEND - jsdom]', () => {
    it('returns handled:false when type is missing', () => {
        const { manager, eventBus } = makeManager();

        const result = manager.handleSSEEvent({ data: { content: 'hello' } });

        expect(result.handled).toBe(false);
        expect(eventBus.getEmittedEvents()).toHaveLength(0);
    });

    it('returns handled:false when type is null', () => {
        const { manager, eventBus } = makeManager();

        const result = manager.handleSSEEvent({ type: null, data: {} });

        expect(result.handled).toBe(false);
        expect(eventBus.getEmittedEvents()).toHaveLength(0);
    });

    it('returns handled:false when type is a number', () => {
        const { manager, eventBus } = makeManager();

        const result = manager.handleSSEEvent({ type: 42, data: {} });

        expect(result.handled).toBe(false);
        expect(eventBus.getEmittedEvents()).toHaveLength(0);
    });

    it('returns handled:false when type is an empty string', () => {
        const { manager, eventBus } = makeManager();

        const result = manager.handleSSEEvent({ type: '', data: {} });

        expect(result.handled).toBe(false);
        expect(eventBus.getEmittedEvents()).toHaveLength(0);
    });
});

// ---------------------------------------------------------------------------
// Non-infrastructure event — missing data field
// ---------------------------------------------------------------------------

describe('SSEConnectionManager.handleSSEEvent — non-infrastructure without data [FRONTEND - jsdom]', () => {
    it('drops event and returns handled:false when data field is absent', () => {
        const { manager, eventBus } = makeManager();

        const result = manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
        });

        expect(result.handled).toBe(false);
        expect(eventBus.getEmittedEvents()).toHaveLength(0);
    });

    it('drops event when top-level frame has no data field (G8eePassthrough g8ee shape)', () => {
        const { manager, eventBus } = makeManager();

        const result = manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
        });

        expect(result.handled).toBe(false);
        expect(eventBus.getEmittedEvents()).toHaveLength(0);
    });
});

// ---------------------------------------------------------------------------
// Non-infrastructure events — correct dispatch
// ---------------------------------------------------------------------------

describe('SSEConnectionManager.handleSSEEvent — eventBus dispatch [FRONTEND - jsdom]', () => {
    it('emits LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            content: 'Hello world',
            web_session_id: 'session_abc',
            investigation_id: 'inv_001',
        };

        const result = manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            data: payload,
        });

        expect(result).toEqual({ handled: true, eventType: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED });
        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload).toEqual(payload);
    });

    it('emits LLM_CHAT_ITERATION_TEXT_COMPLETED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            finish_reason: 'STOP',
        };

        manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload).toEqual(payload);
    });

    it('emits LLM_CHAT_ITERATION_COMPLETED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            turn: 1,
        };

        manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_COMPLETED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_COMPLETED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload).toEqual(payload);
    });

    it('emits LLM_CHAT_ITERATION_FAILED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            error: 'model overloaded',
        };

        manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_FAILED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_FAILED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload).toEqual(payload);
    });

    it('emits LLM_CHAT_ITERATION_STOPPED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
        };

        manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_STOPPED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_STOPPED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload).toEqual(payload);
    });

    it('emits LLM_CHAT_ITERATION_CITATIONS_RECEIVED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            grounding_metadata: {
                grounding_used: true,
                sources: [{ citation_num: 1, title: 'Example', url: 'https://example.com' }],
            },
        };

        manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.grounding_metadata.sources).toHaveLength(1);
        expect(emitted[0].payload.grounding_metadata.sources[0].citation_num).toBe(1);
    });

    it('emits LLM_TOOL_G8E_WEB_SEARCH_REQUESTED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            query: 'disk usage',
            execution_id: 'exec_001',
            status: 'started',
        };

        manager.handleSSEEvent({
            type: EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.query).toBe('disk usage');
        expect(emitted[0].payload.execution_id).toBe('exec_001');
    });

    it('emits LLM_TOOL_G8E_WEB_SEARCH_COMPLETED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            query: 'disk usage',
            execution_id: 'exec_001',
            status: 'completed',
        };

        manager.handleSSEEvent({
            type: EventType.LLM_TOOL_G8E_WEB_SEARCH_COMPLETED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.LLM_TOOL_G8E_WEB_SEARCH_COMPLETED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.execution_id).toBe('exec_001');
    });

    it('emits LLM_TOOL_G8E_WEB_SEARCH_FAILED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            execution_id: 'exec_fail_001',
            status: 'failed',
        };

        manager.handleSSEEvent({
            type: EventType.LLM_TOOL_G8E_WEB_SEARCH_FAILED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.LLM_TOOL_G8E_WEB_SEARCH_FAILED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.execution_id).toBe('exec_fail_001');
    });

    it('emits OPERATOR_NETWORK_PORT_CHECK_REQUESTED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            port: '443',
            execution_id: 'exec_port_001',
            status: 'started',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.port).toBe('443');
        expect(emitted[0].payload.execution_id).toBe('exec_port_001');
    });

    it('emits OPERATOR_NETWORK_PORT_CHECK_COMPLETED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            execution_id: 'exec_port_001',
            status: 'completed',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED);
        expect(emitted).toHaveLength(1);
    });

    it('emits OPERATOR_NETWORK_PORT_CHECK_FAILED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            execution_id: 'exec_port_001',
            status: 'failed',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED);
        expect(emitted).toHaveLength(1);
    });

    it('emits OPERATOR_COMMAND_APPROVAL_REQUESTED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            approval_id: 'approval_001',
            command: 'rm -rf /tmp/test',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.approval_id).toBe('approval_001');
    });

    it('emits OPERATOR_COMMAND_STARTED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            execution_id: 'exec_exec_001',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_COMMAND_STARTED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_COMMAND_STARTED);
        expect(emitted).toHaveLength(1);
    });

    it('emits OPERATOR_COMMAND_COMPLETED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            execution_id: 'exec_exec_001',
            status: 'completed',
            output: 'success',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_COMMAND_COMPLETED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_COMMAND_COMPLETED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.execution_id).toBe('exec_exec_001');
    });

    it('emits OPERATOR_COMMAND_FAILED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            execution_id: 'exec_exec_001',
            status: 'failed',
            error: 'command not found',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_COMMAND_FAILED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_COMMAND_FAILED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.error).toBe('command not found');
    });

    it('emits OPERATOR_FILE_EDIT_APPROVAL_REQUESTED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            approval_id: 'approval_file_001',
            file_path: '/etc/hosts',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.approval_id).toBe('approval_file_001');
    });

    it('emits OPERATOR_FILE_EDIT_STARTED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            execution_id: 'exec_file_001',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_FILE_EDIT_STARTED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_FILE_EDIT_STARTED);
        expect(emitted).toHaveLength(1);
    });

    it('emits OPERATOR_FILE_EDIT_COMPLETED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            execution_id: 'exec_file_001',
            status: 'completed',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_FILE_EDIT_COMPLETED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_FILE_EDIT_COMPLETED);
        expect(emitted).toHaveLength(1);
    });

    it('emits OPERATOR_FILE_EDIT_FAILED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            execution_id: 'exec_file_001',
            error: 'permission denied',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_FILE_EDIT_FAILED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_FILE_EDIT_FAILED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.error).toBe('permission denied');
    });

    it('emits OPERATOR_INTENT_APPROVAL_REQUESTED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            approval_id: 'approval_intent_001',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_INTENT_APPROVAL_REQUESTED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_INTENT_APPROVAL_REQUESTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.approval_id).toBe('approval_intent_001');
    });

    it('emits CASE_CREATED with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            case_id: 'case_new_001',
            investigation_id: 'inv_001',
        };

        manager.handleSSEEvent({
            type: EventType.CASE_CREATED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.CASE_CREATED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.case_id).toBe('case_new_001');
    });

    it('emits OPERATOR_STATUS_UPDATED_ACTIVE with the data payload', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            operator_id: 'op_001',
            status: 'active',
        };

        manager.handleSSEEvent({
            type: EventType.OPERATOR_STATUS_UPDATED_ACTIVE,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_STATUS_UPDATED_ACTIVE);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.operator_id).toBe('op_001');
    });
});

// ---------------------------------------------------------------------------
// Payload fidelity — data field passed through verbatim
// ---------------------------------------------------------------------------

describe('SSEConnectionManager.handleSSEEvent — payload fidelity [FRONTEND - jsdom]', () => {
    it('emits the data field exactly as received, not the top-level frame', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            content: 'chunk text',
            extra_field: 'extra_value',
        };

        manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED);
        expect(emitted[0].payload).toBe(payload);
    });

    it('does not emit the `type` field on the payload — type is only on the frame', () => {
        const { manager, eventBus } = makeManager();
        const payload = {
            investigation_id: 'inv_001',
            web_session_id: 'session_abc',
            content: 'text',
        };

        manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            data: payload,
        });

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED);
        expect(emitted[0].payload).not.toHaveProperty('type');
    });

    it('handles a null data field by emitting null as payload', () => {
        const { manager, eventBus } = makeManager();

        manager.handleSSEEvent({
            type: EventType.OPERATOR_COMMAND_STARTED,
            data: null,
        });

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_COMMAND_STARTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload).toBeNull();
    });

    it('multiple distinct events accumulate independently on the eventBus', () => {
        const { manager, eventBus } = makeManager();

        manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            data: { content: 'chunk 1', web_session_id: 'session_abc', investigation_id: 'inv_001' },
        });
        manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            data: { content: 'chunk 2', web_session_id: 'session_abc', investigation_id: 'inv_001' },
        });
        manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
            data: { investigation_id: 'inv_001', web_session_id: 'session_abc' },
        });

        expect(eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED)).toHaveLength(2);
        expect(eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED)).toHaveLength(1);
    });
});

// ---------------------------------------------------------------------------
// EventSource constructor and readyState regression tests
//
// Regression coverage for the accidental EventType-as-constructor rename.
// These paths were broken when a sweeping refactor replaced the browser's
// native EventSource with EventType (a frozen constants object).
// ---------------------------------------------------------------------------

describe('SSEConnectionManager.connect — EventSource construction [FRONTEND - jsdom]', () => {
    afterEach(() => {
        vi.restoreAllMocks();
        MockEventSource._lastInstance = null;
    });

    it('creates an EventSource instance when connect() is called', () => {
        const { manager } = makeManager();

        manager.connect('session_abc');

        const es = MockEventSource._lastInstance;
        expect(es).toBeInstanceOf(MockEventSource);
        expect(es.url).toBe('/sse/events');
        expect(es.withCredentials).toBe(true);
    });

    it('sets isConnected=true and resets reconnect counters on open', () => {
        const { manager } = makeManager();

        manager.connect('session_abc');
        const es = MockEventSource._lastInstance;
        es._simulateOpen();

        expect(manager.isConnected).toBe(true);
        expect(manager.reconnectAttempts).toBe(0);
        expect(manager.consecutiveFailures).toBe(0);
    });

    it('emits PLATFORM_SSE_CONNECTION_OPENED on open', () => {
        const { manager, eventBus } = makeManager();

        manager.connect('session_abc');
        MockEventSource._lastInstance._simulateOpen();

        const events = eventBus.getEmittedEvents(EventType.PLATFORM_SSE_CONNECTION_OPENED);
        expect(events).toHaveLength(1);
        expect(events[0].payload.webSessionId).toBe('session_abc');
    });

    it('parses SSE messages and routes through handleSSEEvent', () => {
        const { manager, eventBus } = makeManager();

        manager.connect('session_abc');
        const es = MockEventSource._lastInstance;
        es._simulateOpen();
        es._simulateMessage({
            type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            data: { content: 'hello', web_session_id: 'session_abc', investigation_id: 'inv_1' },
        });

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.content).toBe('hello');
    });

    it('closes previous EventSource when reconnecting to new session', () => {
        const { manager } = makeManager();

        manager.connect('session_1');
        const first = MockEventSource._lastInstance;
        first._simulateOpen();

        manager.connect('session_2');
        expect(first.readyState).toBe(MockEventSource.CLOSED);

        const second = MockEventSource._lastInstance;
        expect(second).not.toBe(first);
    });

    it('sets isConnected=false on error', () => {
        const { manager } = makeManager();

        manager.connect('session_abc');
        const es = MockEventSource._lastInstance;
        es._simulateOpen();
        expect(manager.isConnected).toBe(true);

        es._simulateError();
        expect(manager.isConnected).toBe(false);
    });
});

describe('SSEConnectionManager.isConnectionActive — readyState check [FRONTEND - jsdom]', () => {
    it('returns true when connected and readyState is OPEN', () => {
        const { manager } = makeManager();

        manager.connect('session_abc');
        MockEventSource._lastInstance._simulateOpen();

        expect(manager.isConnectionActive()).toBe(true);
    });

    it('returns false when readyState is CONNECTING', () => {
        const { manager } = makeManager();

        manager.connect('session_abc');

        expect(manager.isConnectionActive()).toBe(false);
    });

    it('returns false when eventSource is null', () => {
        const { manager } = makeManager();

        expect(manager.isConnectionActive()).toBe(false);
    });
});

describe('SSEConnectionManager.disconnect [FRONTEND - jsdom]', () => {
    it('closes EventSource and nulls the reference', () => {
        const { manager } = makeManager();

        manager.connect('session_abc');
        const es = MockEventSource._lastInstance;
        es._simulateOpen();

        manager.disconnect();

        expect(es.readyState).toBe(MockEventSource.CLOSED);
        expect(manager.eventSource).toBeNull();
        expect(manager.isConnected).toBe(false);
    });

    it('resets reconnect state', () => {
        const { manager } = makeManager();

        manager.connect('session_abc');
        MockEventSource._lastInstance._simulateOpen();
        manager.reconnectAttempts = 5;
        manager.consecutiveFailures = 3;

        manager.disconnect();

        expect(manager.reconnectAttempts).toBe(0);
        expect(manager.consecutiveFailures).toBe(0);
    });
});

describe('SSEConnectionManager.getConnectionStatus [FRONTEND - jsdom]', () => {
    it('returns OPEN readyState when connected', () => {
        const { manager } = makeManager();

        manager.connect('session_abc');
        MockEventSource._lastInstance._simulateOpen();

        const status = manager.getConnectionStatus();
        expect(status.isConnected).toBe(true);
        expect(status.readyState).toBe(EventSource.OPEN);
        expect(status.url).toBe('/sse/events');
    });

    it('returns CLOSED readyState when no connection exists', () => {
        const { manager } = makeManager();

        const status = manager.getConnectionStatus();
        expect(status.isConnected).toBe(false);
        expect(status.readyState).toBe(EventSource.CLOSED);
        expect(status.url).toBeNull();
    });
});

describe('SSEConnectionManager.resetKeepaliveTimeout [FRONTEND - jsdom]', () => {
    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('closes EventSource on keepalive timeout', () => {
        vi.useFakeTimers();
        const { manager } = makeManager();

        manager.connect('session_abc');
        const es = MockEventSource._lastInstance;
        es._simulateOpen();

        manager.resetKeepaliveTimeout();
        vi.advanceTimersByTime(120_001);

        expect(es.readyState).toBe(MockEventSource.CLOSED);
        expect(manager.isConnected).toBe(false);

        vi.useRealTimers();
    });
});

// ---------------------------------------------------------------------------
// Event payload validation
// ---------------------------------------------------------------------------

describe('SSEConnectionManager._validateEventPayload [FRONTEND - jsdom]', () => {
    it('returns error for null payload', () => {
        const { manager } = makeManager();
        const error = manager._validateEventPayload(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, null);
        expect(error).toBe('Payload must be an object');
    });

    it('returns error for non-object payload', () => {
        const { manager } = makeManager();
        const error = manager._validateEventPayload(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, 'string');
        expect(error).toBe('Payload must be an object');
    });

    it('returns null for unknown event types (no validation)', () => {
        const { manager } = makeManager();
        const error = manager._validateEventPayload('unknown.event.type', { foo: 'bar' });
        expect(error).toBeNull();
    });

    it('validates LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED requires web_session_id and content', () => {
        const { manager } = makeManager();
        
        let error = manager._validateEventPayload(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, {
            web_session_id: 'sess_1'
        });
        expect(error).toBe('Missing required field: content');

        error = manager._validateEventPayload(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, {
            content: 'hello'
        });
        expect(error).toBe('Missing required field: web_session_id');

        error = manager._validateEventPayload(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED, {
            web_session_id: 'sess_1',
            content: 'hello'
        });
        expect(error).toBeNull();
    });

    it('validates LLM_CHAT_ITERATION_TEXT_COMPLETED requires web_session_id', () => {
        const { manager } = makeManager();
        
        let error = manager._validateEventPayload(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED, {});
        expect(error).toBe('Missing required field: web_session_id');

        error = manager._validateEventPayload(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED, {
            web_session_id: 'sess_1'
        });
        expect(error).toBeNull();
    });

    it('validates LLM_CHAT_ITERATION_CITATIONS_RECEIVED requires web_session_id and grounding_metadata', () => {
        const { manager } = makeManager();
        
        let error = manager._validateEventPayload(EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED, {
            web_session_id: 'sess_1'
        });
        expect(error).toBe('Missing required field: grounding_metadata');

        error = manager._validateEventPayload(EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED, {
            grounding_metadata: {}
        });
        expect(error).toBe('Missing required field: web_session_id');

        error = manager._validateEventPayload(EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED, {
            web_session_id: 'sess_1',
            grounding_metadata: {}
        });
        expect(error).toBeNull();
    });

    it('validates LLM_TOOL_G8E_WEB_SEARCH_REQUESTED requires web_session_id, execution_id, and query', () => {
        const { manager } = makeManager();
        
        let error = manager._validateEventPayload(EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED, {
            web_session_id: 'sess_1',
            execution_id: 'exec_1'
        });
        expect(error).toBe('Missing required field: query');

        error = manager._validateEventPayload(EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED, {
            web_session_id: 'sess_1',
            query: 'test'
        });
        expect(error).toBe('Missing required field: execution_id');

        error = manager._validateEventPayload(EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED, {
            execution_id: 'exec_1',
            query: 'test'
        });
        expect(error).toBe('Missing required field: web_session_id');

        error = manager._validateEventPayload(EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED, {
            web_session_id: 'sess_1',
            execution_id: 'exec_1',
            query: 'test'
        });
        expect(error).toBeNull();
    });

    it('validates OPERATOR_NETWORK_PORT_CHECK_REQUESTED requires web_session_id, execution_id, and port', () => {
        const { manager } = makeManager();
        
        let error = manager._validateEventPayload(EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED, {
            web_session_id: 'sess_1',
            execution_id: 'exec_1'
        });
        expect(error).toBe('Missing required field: port');

        error = manager._validateEventPayload(EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED, {
            web_session_id: 'sess_1',
            port: '443'
        });
        expect(error).toBe('Missing required field: execution_id');

        error = manager._validateEventPayload(EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED, {
            execution_id: 'exec_1',
            port: '443'
        });
        expect(error).toBe('Missing required field: web_session_id');

        error = manager._validateEventPayload(EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED, {
            web_session_id: 'sess_1',
            execution_id: 'exec_1',
            port: '443'
        });
        expect(error).toBeNull();
    });

    it('drops event with validation failure and logs error', () => {
        const { manager, eventBus } = makeManager();
        
        const result = manager.handleSSEEvent({
            type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            data: { content: 'hello' } // missing web_session_id
        });

        expect(result.handled).toBe(false);
        expect(eventBus.getEmittedEvents()).toHaveLength(0);
    });
});
