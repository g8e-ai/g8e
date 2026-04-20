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

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { createInternalSSERouter } from '@g8ed/routes/internal/internal_sse_routes.js';
import { EventType } from '@g8ed/constants/events.js';

describe('Internal SSE Routes [UNIT]', () => {
    let router;
    let mockSSEService;
    let mockAuthorizationMiddleware;

    beforeEach(() => {
        mockSSEService = {
            publishEvent: vi.fn(),
            publishToUser: vi.fn()
        };
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };

        router = createInternalSSERouter({
            services: {
                sseService: mockSSEService
            },
            authorizationMiddleware: mockAuthorizationMiddleware
        });
    });

    const createMockReq = (overrides = {}) => ({
        body: {},
        ip: '127.0.0.1',
        headers: {},
        connection: { remoteAddress: '127.0.0.1' },
        ...overrides
    });

    const createMockRes = () => {
        const res = {};
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        return res;
    };

    describe('POST /push', () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === '/push');
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should successfully push an event to a web session', async () => {
            const event = { type: EventType.LLM_CHAT_ITERATION_TEXT_RECEIVED, text: 'hello' };
            const req = createMockReq({
                body: {
                    web_session_id: 'ws_123',
                    user_id: 'user-456',
                    event
                }
            });
            const res = createMockRes();

            mockSSEService.publishEvent.mockResolvedValue(true);

            await getRoute()(req, res);

            expect(mockSSEService.publishEvent).toHaveBeenCalledWith('ws_123', expect.objectContaining({
                _payload: event
            }), expect.any(Function));
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                delivered: 1
            }));
        });

        it('should use web_session_id directly without any binding resolution', async () => {
            const event = { type: EventType.LLM_CHAT_ITERATION_TEXT_RECEIVED, text: 'hello' };
            const req = createMockReq({
                body: {
                    web_session_id: 'web_session_abc123',
                    user_id: 'user-456',
                    event
                }
            });
            const res = createMockRes();

            mockSSEService.publishEvent.mockResolvedValue(true);

            await getRoute()(req, res);

            expect(mockSSEService.publishEvent).toHaveBeenCalledWith('web_session_abc123', expect.any(Object), expect.any(Function));
        });

        it('should forward citations ready event payload as-is (no transport-layer normalization)', async () => {
            // g8ee is authoritative for citation numbering; g8ed transport MUST NOT mutate payloads.
            // See g8ee tests: test_citation_numbers_are_sequential_from_one, test_grounding_sources_sequential_citation_numbers.
            const event = {
                type: EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
                grounding_metadata: {
                    sources: [
                        { citation_num: 1, title: 'Doc A' },
                        { citation_num: 2, title: 'Doc B' }
                    ]
                }
            };
            const req = createMockReq({
                body: {
                    web_session_id: 'ws_123',
                    user_id: 'user-456',
                    event
                }
            });
            const res = createMockRes();

            mockSSEService.publishEvent.mockResolvedValue(true);

            await getRoute()(req, res);

            const publishedEvent = mockSSEService.publishEvent.mock.calls[0][1];
            expect(publishedEvent._payload).toEqual(event);
            expect(mockSSEService.publishEvent).toHaveBeenCalledWith('ws_123', expect.any(Object), expect.any(Function));
        });

        it('should return 500 if an error occurs', async () => {
            const req = createMockReq({
                body: {
                    web_session_id: 'ws_123',
                    user_id: 'user-456',
                    event: { type: 'test' }
                }
            });
            const res = createMockRes();

            mockSSEService.publishEvent.mockRejectedValue(new Error('Internal error'));

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Internal error'
            }));
            expect(mockSSEService.publishEvent).toHaveBeenCalledWith('ws_123', expect.any(Object), expect.any(Function));
        });

        it('should return 500 if publishEvent returns false', async () => {
            const req = createMockReq({
                body: {
                    web_session_id: 'ws_123',
                    user_id: 'user-456',
                    event: { type: 'test' }
                }
            });
            const res = createMockRes();

            mockSSEService.publishEvent.mockResolvedValue(false);

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Failed to publish event'
            }));
            expect(mockSSEService.publishEvent).toHaveBeenCalledWith('ws_123', expect.any(Object), expect.any(Function));
        });

        it('should pass through OPERATOR_PANEL_LIST_UPDATED event as-is', async () => {
            const event = {
                type: EventType.OPERATOR_PANEL_LIST_UPDATED,
                operator_id: 'g8ee-operator-123',
                data: { some: 'context' }
            };
            const req = createMockReq({
                body: {
                    web_session_id: 'ws_123',
                    user_id: 'user-456',
                    event
                }
            });
            const res = createMockRes();

            mockSSEService.publishEvent.mockResolvedValue(true);

            await getRoute()(req, res);

            const publishedEvent = mockSSEService.publishEvent.mock.calls[0][1];
            expect(publishedEvent._payload).toEqual(event);
            expect(mockSSEService.publishEvent).toHaveBeenCalledWith('ws_123', expect.any(Object), expect.any(Function));
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                delivered: 1
            }));
        });

        it('should fan out to user sessions when web_session_id is absent (BackgroundEvent)', async () => {
            const event = { type: EventType.OPERATOR_HEARTBEAT_RECEIVED, operator_id: 'op-1' };
            const req = createMockReq({
                body: {
                    user_id: 'user-456',
                    event
                }
            });
            const res = createMockRes();

            mockSSEService.publishToUser.mockResolvedValue(2);

            await getRoute()(req, res);

            expect(mockSSEService.publishToUser).toHaveBeenCalledWith('user-456', expect.objectContaining({
                _payload: event
            }));
            expect(mockSSEService.publishEvent).not.toHaveBeenCalled();
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                delivered: 2
            }));
        });

        it('should return 200 with delivered:0 when BackgroundEvent fan-out finds no connected sessions', async () => {
            // Zero-delivery fan-out is a documented legitimate outcome for BackgroundEvents
            // (user has no locally connected sessions) and MUST NOT be surfaced as an error.
            const event = { type: EventType.OPERATOR_HEARTBEAT_RECEIVED, operator_id: 'op-1' };
            const req = createMockReq({
                body: {
                    user_id: 'user-456',
                    event
                }
            });
            const res = createMockRes();

            mockSSEService.publishToUser.mockResolvedValue(0);

            await getRoute()(req, res);

            expect(res.status).not.toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                delivered: 0
            }));
        });

        it('should handle malformed event payload gracefully', async () => {
            const req = createMockReq({
                body: {
                    web_session_id: 'ws_123',
                    user_id: 'user-456',
                    event: null
                }
            });
            const res = createMockRes();

            mockSSEService.publishEvent.mockResolvedValue(true);

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: expect.stringContaining('event')
            }));
        });
    });

    const getRoute = () => {
        const layer = router.stack.find(s => s.route?.path === '/push');
        return layer.route.stack[layer.route.stack.length - 1].handle;
    };

    describe('citations payload pass-through', () => {

        it('should forward citations event with empty grounding_metadata unchanged', async () => {
            const event = {
                type: EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
                grounding_metadata: {}
            };
            const req = createMockReq({
                body: { web_session_id: 'ws_123', user_id: 'user-456', event }
            });
            const res = createMockRes();
            mockSSEService.publishEvent.mockResolvedValue(true);

            await getRoute()(req, res);

            const publishedEvent = mockSSEService.publishEvent.mock.calls[0][1];
            expect(publishedEvent._payload).toEqual(event);
            expect(mockSSEService.publishEvent).toHaveBeenCalledWith('ws_123', expect.any(Object), expect.any(Function));
        });
    });
});
