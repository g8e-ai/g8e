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
 * SSE Event Pipeline [INTEGRATION - jsdom]
 *
 * Proves the full wire path for every event type that g8ee publishes:
 *
 *   G8eePassthroughEvent.forWire()
 *     → SSEService.publishEvent() → sendToLocal() → MockSSEResponse.write("data: ...\n\n")
 *     → parse raw SSE frame (exactly as SSEConnectionManager.onmessage does)
 *     → SSEConnectionManager.handleSSEEvent(parsed)
 *     → eventBus.emit(eventType, payload)
 *     → assert payload shape matches what ChatSSEHandlersMixin handlers expect
 *
 * This test catches:
 *   - Event type string mismatches between g8ee constants and frontend constants
 *   - Wire shape mismatches (missing `data` wrapper, wrong nesting)
 *   - normalizeCitationNums side-effects on citation_num sequencing
 *   - Handler payload field access bugs (wrong key names)
 *   - Missing handler registrations (not caught here — covered in chat-sse-handlers tests)
 *
 * Real components under test:
 *   G8eePassthroughEvent (server model) — forWire() produces the exact wire object
 *   SSEService.publishEvent / sendToLocal — writes the SSE frame to a stream
 *   MockSSEResponse — captures the raw write and parses the SSE frame
 *   SSEConnectionManager.handleSSEEvent — front-end wire parser and eventBus dispatcher
 *   MockEventBus — records all emitted events for assertion
 *
 * Only mocked:
 *   @g8ed/utils/logger.js — silence output
 *   SSEService dependencies (OperatorDataService, g8edSettings, etc.) — not exercised here
 */

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { MockEventBus } from '@test/mocks/mock-browser-env.js';
import { MockSSEResponse } from '@test/mocks/mock-sse-browser.js';
import { EventType } from '@g8ed/public/js/constants/events.js';
import { SSEConnectionManager } from '@g8ed/public/js/utils/sse-connection-manager.js';
import { SSEService } from '@g8ed/services/platform/sse_service.js';
import { G8eePassthroughEvent } from '@g8ed/models/sse_models.js';

vi.mock('@g8ed/utils/logger.js', () => ({
    logger: { info: vi.fn(), warn: vi.fn(), error: vi.fn(), debug: vi.fn() },
    addLogListener: vi.fn(),
    removeLogListener: vi.fn(),
    getLogRingBuffer: vi.fn(() => []),
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const WEB_SESSION_ID = 'session_pipeline_test';
const INVESTIGATION_ID = 'inv_pipeline_test';

function makeSSEService() {
    return new SSEService({
        OperatorDataService: null,
        g8edSettings: null,
        internalHttpClient: null,
        boundSessionsService: null,
    });
}

/**
 * Simulate the full pipeline for a single G8eePassthroughEvent:
 * 1. Register a MockSSEResponse as the SSE connection for WEB_SESSION_ID
 * 2. Call SSEService.publishEvent — writes "data: <json>\n\n" to the response
 * 3. Parse every SSE frame written (exactly as SSEConnectionManager.onmessage does)
 * 4. Pass each parsed object through SSEConnectionManager.handleSSEEvent
 * 5. Return the MockEventBus so callers can assert on emitted events
 */
async function pipelineRun(event) {
    const sseService = makeSSEService();
    const mockResponse = new MockSSEResponse();
    mockResponse.flushHeaders();
    await sseService.registerConnection(WEB_SESSION_ID, 'u-test', mockResponse);

    const eventBus = new MockEventBus();
    const manager = new SSEConnectionManager(eventBus);

    await sseService.publishEvent(WEB_SESSION_ID, event);

    const writtenData = mockResponse.getWrittenData().join('');
    const frames = writtenData.split('\n\n').filter(f => f.trim());
    for (const frame of frames) {
        if (frame.startsWith('data: ')) {
            const jsonStr = frame.slice(6).trim();
            const parsed = JSON.parse(jsonStr);
            manager.handleSSEEvent(parsed);
        }
    }

    return eventBus;
}

// ---------------------------------------------------------------------------
// g8ee chat iteration events
// ---------------------------------------------------------------------------

describe('SSE pipeline — LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED [INTEGRATION - jsdom]', () => {
    it('emits on eventBus with content and session fields intact', async () => {
        const payload = {
            type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            data: {
                content: 'Hello from the AI',
                web_session_id: WEB_SESSION_ID,
                investigation_id: INVESTIGATION_ID,
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.content).toBe('Hello from the AI');
        expect(emitted[0].payload.web_session_id).toBe(WEB_SESSION_ID);
        expect(emitted[0].payload.investigation_id).toBe(INVESTIGATION_ID);
    });
});

describe('SSE pipeline — LLM_CHAT_ITERATION_TEXT_COMPLETED [INTEGRATION - jsdom]', () => {
    it('emits with finish_reason and session fields', async () => {
        const payload = {
            type: EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                finish_reason: 'STOP',
                message_id: 'msg_001',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.finish_reason).toBe('STOP');
        expect(emitted[0].payload.investigation_id).toBe(INVESTIGATION_ID);
    });
});

describe('SSE pipeline — LLM_CHAT_ITERATION_COMPLETED [INTEGRATION - jsdom]', () => {
    it('emits with turn number', async () => {
        const payload = {
            type: EventType.LLM_CHAT_ITERATION_COMPLETED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                turn: 2,
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_COMPLETED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.turn).toBe(2);
    });
});

describe('SSE pipeline — LLM_CHAT_ITERATION_FAILED [INTEGRATION - jsdom]', () => {
    it('emits with error string and session fields', async () => {
        const payload = {
            type: EventType.LLM_CHAT_ITERATION_FAILED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                error: 'model overloaded',
                raw_error: 'ResourceExhausted: quota exceeded',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_FAILED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.error).toBe('model overloaded');
        expect(emitted[0].payload.raw_error).toBe('ResourceExhausted: quota exceeded');
    });
});

describe('SSE pipeline — LLM_CHAT_ITERATION_STOPPED [INTEGRATION - jsdom]', () => {
    it('emits with session fields', async () => {
        const payload = {
            type: EventType.LLM_CHAT_ITERATION_STOPPED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_STOPPED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.web_session_id).toBe(WEB_SESSION_ID);
    });
});

// ---------------------------------------------------------------------------
// Citations — normalizeCitationNums is applied in internal_sse_routes BEFORE
// publishEvent, so the pipeline test receives already-normalized citation_num.
// We assert the shape the frontend handler expects (sequential 1-based ints).
// ---------------------------------------------------------------------------

describe('SSE pipeline — LLM_CHAT_ITERATION_CITATIONS_RECEIVED [INTEGRATION - jsdom]', () => {
    it('emits grounding_metadata with sources array intact', async () => {
        const payload = {
            type: EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                grounding_metadata: {
                    grounding_used: true,
                    sources: [
                        { citation_num: 1, title: 'Example', url: 'https://example.com' },
                        { citation_num: 2, title: 'Another', url: 'https://another.com' },
                    ],
                },
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED);
        expect(emitted).toHaveLength(1);
        const meta = emitted[0].payload.grounding_metadata;
        expect(meta.grounding_used).toBe(true);
        expect(meta.sources).toHaveLength(2);
        expect(meta.sources[0].citation_num).toBe(1);
        expect(meta.sources[1].citation_num).toBe(2);
    });

    it('emits with grounding_used false and empty sources without error', async () => {
        const payload = {
            type: EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                grounding_metadata: {
                    grounding_used: false,
                    sources: [],
                },
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.grounding_metadata.grounding_used).toBe(false);
        expect(emitted[0].payload.grounding_metadata.sources).toHaveLength(0);
    });
});

// ---------------------------------------------------------------------------
// Search web tool events
// ---------------------------------------------------------------------------

describe('SSE pipeline — LLM_TOOL_G8E_WEB_SEARCH_REQUESTED [INTEGRATION - jsdom]', () => {
    it('emits with query, execution_id, and status', async () => {
        const payload = {
            type: EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                query: 'check disk usage on operator',
                execution_id: 'exec_search_001',
                status: 'started',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.query).toBe('check disk usage on operator');
        expect(emitted[0].payload.execution_id).toBe('exec_search_001');
        expect(emitted[0].payload.status).toBe('started');
    });
});

describe('SSE pipeline — LLM_TOOL_G8E_WEB_SEARCH_COMPLETED [INTEGRATION - jsdom]', () => {
    it('emits with execution_id and status', async () => {
        const payload = {
            type: EventType.LLM_TOOL_G8E_WEB_SEARCH_COMPLETED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                query: 'check disk usage on operator',
                execution_id: 'exec_search_001',
                status: 'completed',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.LLM_TOOL_G8E_WEB_SEARCH_COMPLETED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.execution_id).toBe('exec_search_001');
    });
});

describe('SSE pipeline — LLM_TOOL_G8E_WEB_SEARCH_FAILED [INTEGRATION - jsdom]', () => {
    it('emits with execution_id', async () => {
        const payload = {
            type: EventType.LLM_TOOL_G8E_WEB_SEARCH_FAILED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                execution_id: 'exec_search_fail_001',
                status: 'failed',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.LLM_TOOL_G8E_WEB_SEARCH_FAILED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.execution_id).toBe('exec_search_fail_001');
    });
});

// ---------------------------------------------------------------------------
// Network port check events
// ---------------------------------------------------------------------------

describe('SSE pipeline — OPERATOR_NETWORK_PORT_CHECK_REQUESTED [INTEGRATION - jsdom]', () => {
    it('emits with port, execution_id, and status', async () => {
        const payload = {
            type: EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                port: '8443',
                execution_id: 'exec_port_001',
                status: 'started',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.port).toBe('8443');
        expect(emitted[0].payload.execution_id).toBe('exec_port_001');
    });
});

describe('SSE pipeline — OPERATOR_NETWORK_PORT_CHECK_COMPLETED [INTEGRATION - jsdom]', () => {
    it('emits with execution_id', async () => {
        const payload = {
            type: EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                execution_id: 'exec_port_001',
                status: 'completed',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.execution_id).toBe('exec_port_001');
    });
});

describe('SSE pipeline — OPERATOR_NETWORK_PORT_CHECK_FAILED [INTEGRATION - jsdom]', () => {
    it('emits with execution_id', async () => {
        const payload = {
            type: EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                execution_id: 'exec_port_001',
                status: 'failed',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED);
        expect(emitted).toHaveLength(1);
    });
});

// ---------------------------------------------------------------------------
// Operator command approval events
// ---------------------------------------------------------------------------

describe('SSE pipeline — OPERATOR_COMMAND_APPROVAL_REQUESTED [INTEGRATION - jsdom]', () => {
    it('emits with approval_id and command fields', async () => {
        const payload = {
            type: EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                approval_id: 'approval_exec_001',
                command: 'rm -rf /tmp/stale',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.approval_id).toBe('approval_exec_001');
        expect(emitted[0].payload.command).toBe('rm -rf /tmp/stale');
    });
});

describe('SSE pipeline — OPERATOR_COMMAND_STARTED [INTEGRATION - jsdom]', () => {
    it('emits with execution_id', async () => {
        const payload = {
            type: EventType.OPERATOR_COMMAND_STARTED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                execution_id: 'exec_exec_001',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_COMMAND_STARTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.execution_id).toBe('exec_exec_001');
    });
});

describe('SSE pipeline — OPERATOR_COMMAND_COMPLETED [INTEGRATION - jsdom]', () => {
    it('emits with execution_id and output fields', async () => {
        const payload = {
            type: EventType.OPERATOR_COMMAND_COMPLETED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                execution_id: 'exec_exec_001',
                status: 'completed',
                output: 'done',
                exit_code: 0,
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_COMMAND_COMPLETED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.execution_id).toBe('exec_exec_001');
        expect(emitted[0].payload.exit_code).toBe(0);
    });
});

describe('SSE pipeline — OPERATOR_COMMAND_FAILED [INTEGRATION - jsdom]', () => {
    it('emits with execution_id and error fields', async () => {
        const payload = {
            type: EventType.OPERATOR_COMMAND_FAILED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                execution_id: 'exec_exec_002',
                status: 'failed',
                error: 'command not found: foo',
                exit_code: 127,
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_COMMAND_FAILED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.error).toBe('command not found: foo');
        expect(emitted[0].payload.exit_code).toBe(127);
    });
});

// ---------------------------------------------------------------------------
// File edit approval events
// ---------------------------------------------------------------------------

describe('SSE pipeline — OPERATOR_FILE_EDIT_APPROVAL_REQUESTED [INTEGRATION - jsdom]', () => {
    it('emits with approval_id and file_path fields', async () => {
        const payload = {
            type: EventType.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                approval_id: 'approval_file_001',
                file_path: '/etc/nginx/nginx.conf',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.approval_id).toBe('approval_file_001');
        expect(emitted[0].payload.file_path).toBe('/etc/nginx/nginx.conf');
    });
});

describe('SSE pipeline — OPERATOR_FILE_EDIT_STARTED [INTEGRATION - jsdom]', () => {
    it('emits with execution_id', async () => {
        const payload = {
            type: EventType.OPERATOR_FILE_EDIT_STARTED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                execution_id: 'exec_file_001',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_FILE_EDIT_STARTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.execution_id).toBe('exec_file_001');
    });
});

describe('SSE pipeline — OPERATOR_FILE_EDIT_COMPLETED [INTEGRATION - jsdom]', () => {
    it('emits with execution_id', async () => {
        const payload = {
            type: EventType.OPERATOR_FILE_EDIT_COMPLETED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                execution_id: 'exec_file_001',
                status: 'completed',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_FILE_EDIT_COMPLETED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.execution_id).toBe('exec_file_001');
    });
});

describe('SSE pipeline — OPERATOR_FILE_EDIT_FAILED [INTEGRATION - jsdom]', () => {
    it('emits with execution_id and error', async () => {
        const payload = {
            type: EventType.OPERATOR_FILE_EDIT_FAILED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                execution_id: 'exec_file_001',
                error: 'permission denied',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_FILE_EDIT_FAILED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.error).toBe('permission denied');
    });
});

// ---------------------------------------------------------------------------
// Intent approval events
// ---------------------------------------------------------------------------

describe('SSE pipeline — OPERATOR_INTENT_APPROVAL_REQUESTED [INTEGRATION - jsdom]', () => {
    it('emits with approval_id', async () => {
        const payload = {
            type: EventType.OPERATOR_INTENT_APPROVAL_REQUESTED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                approval_id: 'approval_intent_001',
            },
        };

        const eventBus = await pipelineRun(new G8eePassthroughEvent({ _payload: payload }));

        const emitted = eventBus.getEmittedEvents(EventType.OPERATOR_INTENT_APPROVAL_REQUESTED);
        expect(emitted).toHaveLength(1);
        expect(emitted[0].payload.approval_id).toBe('approval_intent_001');
    });
});

// ---------------------------------------------------------------------------
// Multi-event sequence — proves multiple frames are each dispatched correctly
// ---------------------------------------------------------------------------

describe('SSE pipeline — multi-event sequence [INTEGRATION - jsdom]', () => {
    it('delivers all events in a typical AI chat turn in order', async () => {
        const sseService = makeSSEService();
        const mockResponse = new MockSSEResponse();
        mockResponse.flushHeaders();
        await sseService.registerConnection(WEB_SESSION_ID, 'u-test', mockResponse);

        const eventBus = new MockEventBus();
        const manager = new SSEConnectionManager(eventBus);

        const events = [
            new G8eePassthroughEvent({
                _payload: {
                    type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                    data: { content: 'Part one ', web_session_id: WEB_SESSION_ID, investigation_id: INVESTIGATION_ID },
                },
            }),
            new G8eePassthroughEvent({
                _payload: {
                    type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                    data: { content: 'part two.', web_session_id: WEB_SESSION_ID, investigation_id: INVESTIGATION_ID },
                },
            }),
            new G8eePassthroughEvent({
                _payload: {
                    type: EventType.LLM_CHAT_ITERATION_COMPLETED,
                    data: { investigation_id: INVESTIGATION_ID, web_session_id: WEB_SESSION_ID, turn: 1 },
                },
            }),
            new G8eePassthroughEvent({
                _payload: {
                    type: EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
                    data: { investigation_id: INVESTIGATION_ID, web_session_id: WEB_SESSION_ID, finish_reason: 'STOP' },
                },
            }),
        ];

        for (const event of events) {
            await sseService.publishEvent(WEB_SESSION_ID, event);
        }

        const writtenData = mockResponse.getWrittenData().join('');
        const frames = writtenData.split('\n\n').filter(f => f.trim());
        for (const frame of frames) {
            if (frame.startsWith('data: ')) {
                manager.handleSSEEvent(JSON.parse(frame.slice(6).trim()));
            }
        }

        const chunks = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED);
        expect(chunks).toHaveLength(2);
        expect(chunks[0].payload.content).toBe('Part one ');
        expect(chunks[1].payload.content).toBe('part two.');

        const completed = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_COMPLETED);
        expect(completed).toHaveLength(1);
        expect(completed[0].payload.turn).toBe(1);

        const textCompleted = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED);
        expect(textCompleted).toHaveLength(1);
        expect(textCompleted[0].payload.finish_reason).toBe('STOP');
    });

    it('delivers search web + citations sequence correctly', async () => {
        const sseService = makeSSEService();
        const mockResponse = new MockSSEResponse();
        mockResponse.flushHeaders();
        await sseService.registerConnection(WEB_SESSION_ID, 'u-test', mockResponse);

        const eventBus = new MockEventBus();
        const manager = new SSEConnectionManager(eventBus);

        const events = [
            new G8eePassthroughEvent({
                _payload: {
                    type: EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED,
                    data: {
                        investigation_id: INVESTIGATION_ID,
                        web_session_id: WEB_SESSION_ID,
                        query: 'nginx config best practices',
                        execution_id: 'exec_s_001',
                        status: 'started',
                    },
                },
            }),
            new G8eePassthroughEvent({
                _payload: {
                    type: EventType.LLM_TOOL_G8E_WEB_SEARCH_COMPLETED,
                    data: {
                        investigation_id: INVESTIGATION_ID,
                        web_session_id: WEB_SESSION_ID,
                        query: 'nginx config best practices',
                        execution_id: 'exec_s_001',
                        status: 'completed',
                    },
                },
            }),
            new G8eePassthroughEvent({
                _payload: {
                    type: EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
                    data: {
                        investigation_id: INVESTIGATION_ID,
                        web_session_id: WEB_SESSION_ID,
                        grounding_metadata: {
                            grounding_used: true,
                            sources: [
                                { citation_num: 1, title: 'Nginx Docs', url: 'https://nginx.org/docs' },
                            ],
                        },
                    },
                },
            }),
        ];

        for (const event of events) {
            await sseService.publishEvent(WEB_SESSION_ID, event);
        }

        const writtenData = mockResponse.getWrittenData().join('');
        const frames = writtenData.split('\n\n').filter(f => f.trim());
        for (const frame of frames) {
            if (frame.startsWith('data: ')) {
                manager.handleSSEEvent(JSON.parse(frame.slice(6).trim()));
            }
        }

        const searchReq = eventBus.getEmittedEvents(EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED);
        expect(searchReq).toHaveLength(1);
        expect(searchReq[0].payload.execution_id).toBe('exec_s_001');
        expect(searchReq[0].payload.query).toBe('nginx config best practices');

        const searchDone = eventBus.getEmittedEvents(EventType.LLM_TOOL_G8E_WEB_SEARCH_COMPLETED);
        expect(searchDone).toHaveLength(1);
        expect(searchDone[0].payload.execution_id).toBe('exec_s_001');

        const citations = eventBus.getEmittedEvents(EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED);
        expect(citations).toHaveLength(1);
        expect(citations[0].payload.grounding_metadata.sources[0].citation_num).toBe(1);
    });
});

// ---------------------------------------------------------------------------
// Wire shape contract — G8eePassthroughEvent.forWire() shape assertion
// Proves the server-side model produces the exact shape the frontend expects
// ---------------------------------------------------------------------------

describe('SSE pipeline — G8eePassthroughEvent wire shape contract [INTEGRATION - jsdom]', () => {
    it('forWire() returns the inner _payload directly (no outer wrapper)', () => {
        const innerPayload = {
            type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            data: { content: 'hello', web_session_id: 'session_abc', investigation_id: 'inv_001' },
        };
        const event = new G8eePassthroughEvent({ _payload: innerPayload });
        const wire = event.forWire();

        expect(wire).toBe(innerPayload);
        expect(wire.type).toBe(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED);
        expect(wire.data).toBeDefined();
        expect(wire.data.content).toBe('hello');
    });

    it('the SSE frame written to the stream is exactly "data: <json>\\n\\n"', async () => {
        const sseService = makeSSEService();
        const mockResponse = new MockSSEResponse();
        mockResponse.flushHeaders();
        await sseService.registerConnection(WEB_SESSION_ID, 'u-test', mockResponse);

        const innerPayload = {
            type: EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
            data: { investigation_id: INVESTIGATION_ID, web_session_id: WEB_SESSION_ID, finish_reason: 'STOP' },
        };
        await sseService.publishEvent(WEB_SESSION_ID, new G8eePassthroughEvent({ _payload: innerPayload }));

        const writtenData = mockResponse.getWrittenData().join('');
        expect(writtenData).toMatch(/^data: /);
        expect(writtenData).toContain('\n\n');

        const jsonStr = writtenData.replace(/^data: /, '').replace(/\n\n$/, '');
        const parsed = JSON.parse(jsonStr);
        expect(parsed.type).toBe(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED);
        expect(parsed.data.finish_reason).toBe('STOP');
        expect(parsed.data.investigation_id).toBe(INVESTIGATION_ID);
    });

    it('handleSSEEvent receives type at top-level and payload under data key', async () => {
        const sseService = makeSSEService();
        const mockResponse = new MockSSEResponse();
        mockResponse.flushHeaders();
        await sseService.registerConnection(WEB_SESSION_ID, 'u-test', mockResponse);

        const innerPayload = {
            type: EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED,
            data: {
                investigation_id: INVESTIGATION_ID,
                web_session_id: WEB_SESSION_ID,
                query: 'test query',
                execution_id: 'exec_contract_001',
                status: 'started',
            },
        };
        await sseService.publishEvent(WEB_SESSION_ID, new G8eePassthroughEvent({ _payload: innerPayload }));

        const writtenData = mockResponse.getWrittenData().join('');
        const frame = writtenData.split('\n\n').find(f => f.startsWith('data: '));
        const parsed = JSON.parse(frame.slice(6).trim());

        expect(parsed).toHaveProperty('type', EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED);
        expect(parsed).toHaveProperty('data');
        expect(parsed.data).toHaveProperty('query', 'test query');
        expect(parsed.data).toHaveProperty('execution_id', 'exec_contract_001');
    });
});
