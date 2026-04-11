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
 * SSE Connection Lifecycle Tests [INTEGRATION]
 *
 * Tests real SSEService connection state management:
 * - delivery to registered vs unregistered connections
 * - session isolation (event for session A does not reach session B)
 * - destroyed/non-writable connection detection
 * - stale connection ID guard on unregisterConnection
 * - reconnect: new registration after unregister delivers correctly
 *
 * Uses real SSEService + MockSSEResponse. Asserts on raw written bytes via
 * getWrittenData() — no MockSSEBrowser, no waitForEvent, no string literals.
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { EventType } from '@vsod/constants/events.js';
import { MockSSEResponse } from '@test/mocks/mock-sse-browser.js';
import { G8eePassthroughEvent } from '@vsod/models/sse_models.js';
import { SSEService } from '@vsod/services/platform/sse_service.js';

const SESSION_A = 'lifecycle-test-session-a';
const SESSION_B = 'lifecycle-test-session-b';
const INVESTIGATION_ID = 'lifecycle-test-investigation';
const CASE_ID = 'lifecycle-test-case';

function makeSSEService() {
    return new SSEService({
        OperatorDataService: null,
        vsodConfig: null,
        internalHttpClient: null,
        boundSessionsService: null,
    });
}

function makeResponse() {
    const res = new MockSSEResponse();
    res.flushHeaders();
    return res;
}

function readFrames(response) {
    return response.getWrittenData()
        .join('')
        .split('\n\n')
        .filter(f => f.startsWith('data: '))
        .map(f => JSON.parse(f.slice(6).trim()));
}

describe('SSE Connection Lifecycle Tests [INTEGRATION]', () => {
    let sseService;
    let response;
    let connectionId;

    beforeEach(async () => {
        sseService = makeSSEService();
        response = makeResponse();
        const result = await sseService.registerConnection(SESSION_A, response);
        connectionId = result.connectionId;
    });

    afterEach(() => {
        sseService.unregisterConnection(SESSION_A, connectionId);
        response.destroy();
    });

    describe('delivery to registered connection', () => {
        it('should write a valid SSE frame for a published G8eePassthroughEvent', async () => {
            await sseService.publishEvent(SESSION_A, new G8eePassthroughEvent({
                _payload: {
                    type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                    content: 'hello',
                    investigation_id: INVESTIGATION_ID,
                    case_id: CASE_ID,
                    web_session_id: SESSION_A,
                },
            }));

            const frames = readFrames(response);
            expect(frames).toHaveLength(1);
            expect(frames[0].type).toBe(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED);
            expect(frames[0].content).toBe('hello');
            expect(frames[0].investigation_id).toBe(INVESTIGATION_ID);
            expect(frames[0].case_id).toBe(CASE_ID);
            expect(frames[0].web_session_id).toBe(SESSION_A);
        });

        it('should write all frames for a burst of published events', async () => {
            const COUNT = 10;
            for (let i = 0; i < COUNT; i++) {
                await sseService.publishEvent(SESSION_A, new G8eePassthroughEvent({
                    _payload: {
                        type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                        content: `chunk-${i}`,
                        investigation_id: INVESTIGATION_ID,
                        case_id: CASE_ID,
                        web_session_id: SESSION_A,
                    },
                }));
            }

            const frames = readFrames(response);
            expect(frames).toHaveLength(COUNT);
            frames.forEach((frame, i) => {
                expect(frame.content).toBe(`chunk-${i}`);
            });
        });
    });

    describe('session isolation', () => {
        it('should not write to session B when publishing to session A', async () => {
            const responseB = makeResponse();
            const regB = await sseService.registerConnection(SESSION_B, responseB);

            try {
                await sseService.publishEvent(SESSION_A, new G8eePassthroughEvent({
                    _payload: {
                        type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                        content: 'for-a-only',
                        investigation_id: INVESTIGATION_ID,
                        case_id: CASE_ID,
                        web_session_id: SESSION_A,
                    },
                }));

                expect(readFrames(response)).toHaveLength(1);
                expect(readFrames(responseB)).toHaveLength(0);
            } finally {
                sseService.unregisterConnection(SESSION_B, regB.connectionId);
                responseB.destroy();
            }
        });

        it('should write to each session independently when publishing to both', async () => {
            const responseB = makeResponse();
            const regB = await sseService.registerConnection(SESSION_B, responseB);

            try {
                await sseService.publishEvent(SESSION_A, new G8eePassthroughEvent({
                    _payload: {
                        type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                        content: 'for-a',
                        investigation_id: INVESTIGATION_ID,
                        case_id: CASE_ID,
                        web_session_id: SESSION_A,
                    },
                }));
                await sseService.publishEvent(SESSION_B, new G8eePassthroughEvent({
                    _payload: {
                        type: EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
                        content: 'for-b',
                        investigation_id: INVESTIGATION_ID,
                        case_id: CASE_ID,
                        web_session_id: SESSION_B,
                    },
                }));

                const framesA = readFrames(response);
                const framesB = readFrames(responseB);

                expect(framesA).toHaveLength(1);
                expect(framesA[0].type).toBe(EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED);
                expect(framesA[0].content).toBe('for-a');

                expect(framesB).toHaveLength(1);
                expect(framesB[0].type).toBe(EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED);
                expect(framesB[0].content).toBe('for-b');
            } finally {
                sseService.unregisterConnection(SESSION_B, regB.connectionId);
                responseB.destroy();
            }
        });
    });

    describe('unregistered connection', () => {
        it('should not write to response after unregisterConnection', async () => {
            sseService.unregisterConnection(SESSION_A, connectionId);

            await sseService.publishEvent(SESSION_A, new G8eePassthroughEvent({
                _payload: {
                    type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                    content: 'should not arrive',
                    investigation_id: INVESTIGATION_ID,
                    case_id: CASE_ID,
                    web_session_id: SESSION_A,
                },
            }));

            expect(readFrames(response)).toHaveLength(0);

            const reReg = await sseService.registerConnection(SESSION_A, response);
            connectionId = reReg.connectionId;
        });

        it('should not remove a newer connection when unregistering a stale connectionId', async () => {
            const response2 = makeResponse();
            const reg2 = await sseService.registerConnection(SESSION_A, response2);

            sseService.unregisterConnection(SESSION_A, connectionId);

            await sseService.publishEvent(SESSION_A, new G8eePassthroughEvent({
                _payload: {
                    type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                    content: 'reaches-new-connection',
                    investigation_id: INVESTIGATION_ID,
                    case_id: CASE_ID,
                    web_session_id: SESSION_A,
                },
            }));

            expect(readFrames(response2)).toHaveLength(1);
            expect(readFrames(response2)[0].content).toBe('reaches-new-connection');

            sseService.unregisterConnection(SESSION_A, reg2.connectionId);
            response2.destroy();
            connectionId = reg2.connectionId;
        });
    });

    describe('destroyed connection', () => {
        it('should not write to a destroyed response', async () => {
            response.destroy();

            await sseService.publishEvent(SESSION_A, new G8eePassthroughEvent({
                _payload: {
                    type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                    content: 'unreachable',
                    investigation_id: INVESTIGATION_ID,
                    case_id: CASE_ID,
                    web_session_id: SESSION_A,
                },
            }));

            expect(readFrames(response)).toHaveLength(0);
        });
    });

    describe('reconnect', () => {
        it('should deliver to a new response registered after the old one is unregistered', async () => {
            sseService.unregisterConnection(SESSION_A, connectionId);

            const response2 = makeResponse();
            const reg2 = await sseService.registerConnection(SESSION_A, response2);

            await sseService.publishEvent(SESSION_A, new G8eePassthroughEvent({
                _payload: {
                    type: EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                    content: 'post-reconnect',
                    investigation_id: INVESTIGATION_ID,
                    case_id: CASE_ID,
                    web_session_id: SESSION_A,
                },
            }));

            expect(readFrames(response2)).toHaveLength(1);
            expect(readFrames(response2)[0].content).toBe('post-reconnect');
            expect(readFrames(response)).toHaveLength(0);

            sseService.unregisterConnection(SESSION_A, reg2.connectionId);
            response2.destroy();
            connectionId = reg2.connectionId;
        });
    });
});
