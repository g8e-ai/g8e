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
import { createInternalSSERouter } from '@vsod/routes/internal/internal_sse_routes.js';
import { EventType } from '@vsod/constants/events.js';

describe('Internal SSE Routes [UNIT]', () => {
    let router;
    let mockSSEService;
    let mockAuthorizationMiddleware;
    let mockOperatorService;

    beforeEach(() => {
        mockSSEService = {
            publishEvent: vi.fn()
        };
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };
        mockOperatorService = {
            getUserOperators: vi.fn()
        };

        router = createInternalSSERouter({
            services: {
                sseService: mockSSEService,
                operatorService: mockOperatorService
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
                message: 'Event delivered'
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

        it('should normalize citation numbers for citations ready event', async () => {
            const event = {
                type: EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
                grounding_metadata: {
                    sources: [
                        { citation_num: 10, title: 'Doc A' },
                        { citation_num: 20, title: 'Doc B' }
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
            const sources = publishedEvent._payload.grounding_metadata.sources;
            expect(sources[0].citation_num).toBe(1);
            expect(sources[1].citation_num).toBe(2);
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

        it('should replace g8ee operator payload with full operator list for OPERATOR_PANEL_LIST_UPDATED', async () => {
            const event = {
                type: EventType.OPERATOR_PANEL_LIST_UPDATED,
                operator_id: 'g8ee-operator-123'
            };
            const mockOperatorList = {
                type: EventType.OPERATOR_PANEL_LIST_UPDATED,
                operators: [
                    { operator_id: 'op-1', status: 'ACTIVE' },
                    { operator_id: 'op-2', status: 'AVAILABLE' }
                ],
                total_count: 2,
                active_count: 1,
                used_slots: 0,
                max_slots: 2
            };
            const req = createMockReq({
                body: {
                    web_session_id: 'ws_123',
                    user_id: 'user-456',
                    event
                }
            });
            const res = createMockRes();

            mockOperatorService.getUserOperators.mockResolvedValue(mockOperatorList);
            mockSSEService.publishEvent.mockResolvedValue(true);

            await getRoute()(req, res);

            expect(mockOperatorService.getUserOperators).toHaveBeenCalledWith('user-456');
            const publishedEvent = mockSSEService.publishEvent.mock.calls[0][1];
            const wireFormat = publishedEvent.forWire();
            expect(wireFormat.type).toBe(EventType.OPERATOR_PANEL_LIST_UPDATED);
            expect(wireFormat.data.operators).toEqual(mockOperatorList.operators);
            expect(wireFormat.data.total_count).toBe(mockOperatorList.total_count);
            expect(wireFormat.data.active_count).toBe(mockOperatorList.active_count);
            expect(wireFormat.data.used_slots).toBe(mockOperatorList.used_slots);
            expect(wireFormat.data.max_slots).toBe(mockOperatorList.max_slots);
            expect(mockSSEService.publishEvent).toHaveBeenCalledWith('ws_123', expect.any(Object), expect.any(Function));
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                message: 'Event delivered'
            }));
        });

        it('should fallback to original event if operator list fetch fails', async () => {
            const event = {
                type: EventType.OPERATOR_PANEL_LIST_UPDATED,
                operator_id: 'g8ee-operator-123'
            };
            const req = createMockReq({
                body: {
                    web_session_id: 'ws_123',
                    user_id: 'user-456',
                    event
                }
            });
            const res = createMockRes();

            mockOperatorService.getUserOperators.mockRejectedValue(new Error('DB error'));
            mockSSEService.publishEvent.mockResolvedValue(true);

            await getRoute()(req, res);

            const publishedEvent = mockSSEService.publishEvent.mock.calls[0][1];
            expect(publishedEvent._payload).toEqual(event);
            expect(mockSSEService.publishEvent).toHaveBeenCalledWith('ws_123', expect.any(Object), expect.any(Function));
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                message: 'Event delivered'
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

    describe('normalizeCitationNums', () => {

        it('should handle missing or empty sources', async () => {
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
