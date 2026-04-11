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
 * SSE Event Contract Tests [INTEGRATION]
 *
 * Two responsibilities:
 * 1. Shared fixture compliance — assert the JSON fixture file has all required
 *    keys and that event type strings match VSOD EventType constants.
 * 2. Wire format compliance — publish typed model instances through the real
 *    SSEService and assert on the raw bytes written to MockSSEResponse.
 *
 * Pattern mirrors sse-event-pipeline.integration.test.js:
 *   typed model → SSEService.publishEvent() → MockSSEResponse.getWrittenData()
 *   → JSON.parse → assert wire shape
 *
 * No MockSSEBrowser. No waitForEvent. No string literals for event types.
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { readFileSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';
import { EventType } from '@vsod/constants/events.js';
import { MockSSEResponse } from '@test/mocks/mock-sse-browser.js';
import {
    VSEPassthroughEvent,
    ConnectionEstablishedEvent,
    KeepaliveEvent,
} from '@vsod/models/sse_models.js';
import { SSEService } from '@vsod/services/platform/sse_service.js';

const __dirname = dirname(fileURLToPath(import.meta.url));

const fixturesPath = resolve(__dirname, '../../../../../shared/test-fixtures/sse-events.json');
const sharedSSEEvents = JSON.parse(readFileSync(fixturesPath, 'utf8'));

const WEB_SESSION_ID = 'contract-test-session-123';
const INVESTIGATION_ID = 'contract-test-investigation-123';
const CASE_ID = 'contract-test-case-123';

function makeSSEService() {
    return new SSEService({
        OperatorDataService: null,
        vsodConfig: null,
        internalHttpClient: null,
        boundSessionsService: null,
    });
}

async function publishAndRead(sseService, response, event) {
    await sseService.publishEvent(WEB_SESSION_ID, event);
    const raw = response.getWrittenData().join('');
    const frame = raw.split('\n\n').find(f => f.startsWith('data: '));
    return JSON.parse(frame.slice(6).trim());
}

describe('SSE Event Contract Tests [INTEGRATION]', () => {
    let sseService;
    let response;

    beforeEach(async () => {
        sseService = makeSSEService();
        response = new MockSSEResponse();
        response.flushHeaders();
        await sseService.registerConnection(WEB_SESSION_ID, response);
    });

    afterEach(() => {
        response.destroy();
    });

    describe('shared fixture compliance', () => {
        it('should have all required fixture keys with correct shape', () => {
            const requiredKeys = [
                'text_chunk_received',
                'text_completed',
                'chat_iteration_failed',
                'g8e_web_search_requested',
                'g8e_web_search_completed',
                'g8e_web_search_failed',
                'port_check_requested',
                'port_check_completed',
                'port_check_failed',
                'citations_received',
                'operator_command_requested',
                'operator_command_started',
                'operator_command_completed',
                'operator_command_failed',
                'llm_lifecycle_started',
                'llm_lifecycle_completed',
                'platform_sse_connection_established',
                'platform_sse_keepalive_sent',
            ];

            for (const key of requiredKeys) {
                expect(sharedSSEEvents, `fixture missing: ${key}`).toHaveProperty(key);
                const fixture = sharedSSEEvents[key];
                expect(fixture, `${key}.type missing`).toHaveProperty('type');
                expect(fixture, `${key}.data missing`).toHaveProperty('data');
                expect(fixture.data, `${key}.data.web_session_id missing`).toHaveProperty('web_session_id');
                if (!key.startsWith('platform_sse_')) {
                    expect(fixture.data, `${key}.data.investigation_id missing`).toHaveProperty('investigation_id');
                    expect(fixture.data, `${key}.data.case_id missing`).toHaveProperty('case_id');
                }
            }
        });

        it('should have fixture type strings that match EventType constants', () => {
            const mapping = {
                text_chunk_received:                EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                text_completed:                     EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
                chat_iteration_failed:              EventType.LLM_CHAT_ITERATION_FAILED,
                g8e_web_search_requested:               EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED,
                g8e_web_search_completed:               EventType.LLM_TOOL_G8E_WEB_SEARCH_COMPLETED,
                g8e_web_search_failed:                  EventType.LLM_TOOL_G8E_WEB_SEARCH_FAILED,
                port_check_requested:               EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED,
                port_check_completed:               EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED,
                port_check_failed:                  EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED,
                citations_received:                 EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
                operator_command_requested:         EventType.OPERATOR_COMMAND_REQUESTED,
                operator_command_started:           EventType.OPERATOR_COMMAND_STARTED,
                operator_command_completed:         EventType.OPERATOR_COMMAND_COMPLETED,
                operator_command_failed:            EventType.OPERATOR_COMMAND_FAILED,
                llm_lifecycle_started:              EventType.LLM_LIFECYCLE_STARTED,
                llm_lifecycle_completed:            EventType.LLM_LIFECYCLE_COMPLETED,
                platform_sse_connection_established: EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED,
                platform_sse_keepalive_sent:        EventType.PLATFORM_SSE_KEEPALIVE_SENT,
            };

            for (const [key, expected] of Object.entries(mapping)) {
                expect(sharedSSEEvents[key].type, `${key} type mismatch`).toBe(expected);
            }
        });
    });

    describe('routing field compliance', () => {
        it('should preserve investigation_id, case_id, web_session_id through the wire for all fixture event types', async () => {
            const fixtureKeys = [
                'text_chunk_received',
                'text_completed',
                'chat_iteration_failed',
                'g8e_web_search_requested',
                'llm_lifecycle_started',
            ];

            for (const key of fixtureKeys) {
                const fixture = sharedSSEEvents[key];
                const testResponse = new MockSSEResponse();
                testResponse.flushHeaders();
                const testService = makeSSEService();
                await testService.registerConnection(WEB_SESSION_ID, testResponse);

                await testService.publishEvent(WEB_SESSION_ID, new VSEPassthroughEvent({
                    _payload: {
                        ...fixture.data,
                        type: fixture.type,
                        investigation_id: INVESTIGATION_ID,
                        case_id: CASE_ID,
                        web_session_id: WEB_SESSION_ID,
                    },
                }));

                const raw = testResponse.getWrittenData().join('');
                const wire = JSON.parse(raw.split('\n\n').find(f => f.startsWith('data: ')).slice(6).trim());

                expect(wire.type, `${key}: type`).toBe(fixture.type);
                expect(wire.investigation_id, `${key}: investigation_id`).toBe(INVESTIGATION_ID);
                expect(wire.case_id, `${key}: case_id`).toBe(CASE_ID);
                expect(wire.web_session_id, `${key}: web_session_id`).toBe(WEB_SESSION_ID);

                testResponse.destroy();
            }
        });
    });

    describe('platform event wire format', () => {
        it('should write ConnectionEstablishedEvent as a valid SSE frame with correct fields', async () => {
            const wire = await publishAndRead(sseService, response, new ConnectionEstablishedEvent({
                type: EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED,
                connectionId: WEB_SESSION_ID,
            }));

            expect(wire.type).toBe(EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED);
            expect(wire.connectionId).toBe(WEB_SESSION_ID);
            expect(wire.timestamp).toBeDefined();
            expect(wire).not.toHaveProperty('_payload');
        });

        it('should write KeepaliveEvent as a valid SSE frame with correct fields', async () => {
            const serverTime = Date.now();
            const wire = await publishAndRead(sseService, response, new KeepaliveEvent({
                type: EventType.PLATFORM_SSE_KEEPALIVE_SENT,
                serverTime,
            }));

            expect(wire.type).toBe(EventType.PLATFORM_SSE_KEEPALIVE_SENT);
            expect(wire.timestamp).toBeDefined();
            expect(wire.serverTime).toBe(serverTime);
            expect(wire).not.toHaveProperty('_payload');
        });
    });
});
