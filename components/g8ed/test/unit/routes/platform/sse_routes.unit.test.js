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

import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { createSSERouter } from '@g8ed/routes/platform/sse_routes.js';
import { SSEPaths } from '@g8ed/constants/api_paths.js';
import { EventType } from '@g8ed/constants/events.js';
import { SystemHealth } from '@g8ed/constants/ai.js';
import { ConnectionEstablishedEvent } from '@g8ed/models/sse_models.js';
import * as initialization from '@g8ed/services/initialization.js';

describe('SSE Routes [UNIT]', () => {
    let router;
    let mockSSEService;
    let mockOperatorService;
    let mockAuthMiddleware;
    let mockAuthorizationMiddleware;
    let mockRateLimiters;

    beforeEach(() => {
        vi.useFakeTimers();
        vi.spyOn(initialization, 'getOperatorService').mockImplementation(() => mockOperatorService);
        mockSSEService = {
            registerConnection: vi.fn().mockResolvedValue({ connectionId: 'sse_1', localConnections: 1, sessionConnections: 1 }),
            unregisterConnection: vi.fn(),
            publishEvent: vi.fn().mockResolvedValue(true),
            hasLocalConnection: vi.fn().mockReturnValue(true),
            getStats: vi.fn().mockReturnValue({ localConnections: 1, uniqueSessions: 1 }),
            isHealthy: vi.fn().mockReturnValue(true),
            pushInitialState: vi.fn().mockResolvedValue(true)
        };
        mockOperatorService = {
            syncSessionOnConnect: vi.fn().mockResolvedValue(true)
        };
        mockAuthMiddleware = {
            requireAuth: vi.fn((req, res, next) => next())
        };
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };
        mockRateLimiters = {
            sseRateLimiter: vi.fn((req, res, next) => next())
        };

        router = createSSERouter({
            services: {
                sseService: mockSSEService,
                operatorService: mockOperatorService
            },

            authMiddleware: mockAuthMiddleware,
            authorizationMiddleware: mockAuthorizationMiddleware,
            rateLimiters: mockRateLimiters
        });
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    const createMockReq = (overrides = {}) => {
        const req = {
            webSessionId: 'ws_123',
            userId: 'user_123',
            ip: '127.0.0.1',
            headers: { 'user-agent': 'vitest' },
            setTimeout: vi.fn(),
            on: vi.fn(),
            session: { organization_id: 'org_123' },
            ...overrides
        };
        return req;
    };

    const createMockRes = () => {
        const res = {
            socket: { setNoDelay: vi.fn() }
        };
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        res.setHeader = vi.fn().mockReturnValue(res);
        res.flushHeaders = vi.fn().mockReturnValue(res);
        res.end = vi.fn().mockReturnValue(res);
        res.setTimeout = vi.fn().mockReturnValue(res);
        res.write = vi.fn().mockReturnValue(true);
        return res;
    };

    describe(`GET ${SSEPaths.EVENTS}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === SSEPaths.EVENTS && s.route?.methods?.get);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should establish SSE connection and publish establishment event', async () => {
            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(req.setTimeout).toHaveBeenCalledWith(0);
            expect(res.setTimeout).toHaveBeenCalledWith(0);
            expect(res.setHeader).toHaveBeenCalledWith('Content-Type', 'text/event-stream');
            expect(mockSSEService.registerConnection).toHaveBeenCalledWith(
                'ws_123',
                'user_123',
                res,
                expect.objectContaining({ ip: '127.0.0.1' })
            );
            expect(mockSSEService.publishEvent).toHaveBeenCalledWith(
                'ws_123',
                expect.any(ConnectionEstablishedEvent)
            );
            expect(mockSSEService.pushInitialState).toHaveBeenCalledWith(
                'user_123',
                'ws_123',
                'org_123'
            );
            expect(mockOperatorService.syncSessionOnConnect).toHaveBeenCalledWith(
                'user_123',
                'ws_123'
            );
        });

        it('should handle registration failure', async () => {
            mockSSEService.registerConnection.mockRejectedValue(new Error('Registration failed'));
            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.end).toHaveBeenCalled();
            expect(mockSSEService.pushInitialState).not.toHaveBeenCalled();
            expect(mockOperatorService.syncSessionOnConnect).not.toHaveBeenCalled();
        });

        it('should perform cleanup on request close', async () => {
            let closeHandler;
            const req = createMockReq({
                on: vi.fn().mockImplementation((event, handler) => {
                    if (event === 'close') closeHandler = handler;
                })
            });
            const res = createMockRes();

            await getRoute()(req, res);
            expect(closeHandler).toBeDefined();

            await closeHandler();

            expect(mockSSEService.unregisterConnection).toHaveBeenCalledWith('ws_123', 'sse_1');
        });

        it('should handle keepalive interval correctly', async () => {
            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            // Advance time to trigger keepalive
            await vi.advanceTimersByTimeAsync(20000);

            expect(mockSSEService.hasLocalConnection).toHaveBeenCalledWith('ws_123');
            expect(mockSSEService.publishEvent).toHaveBeenCalledWith(
                'ws_123',
                expect.objectContaining({ type: EventType.PLATFORM_SSE_KEEPALIVE_SENT })
            );
        });

        it('should cleanup on keepalive failure', async () => {
            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            mockSSEService.publishEvent.mockRejectedValue(new Error('Push failed'));

            await vi.advanceTimersByTimeAsync(20000);

            expect(mockSSEService.unregisterConnection).toHaveBeenCalled();
        });

        it('should handle connection errors and log specific details', async () => {
            let errorHandler;
            const req = createMockReq({
                on: vi.fn().mockImplementation((event, handler) => {
                    if (event === 'error') errorHandler = handler;
                })
            });
            const res = createMockRes();

            await getRoute()(req, res);
            expect(errorHandler).toBeDefined();

            // Simulate quick failure
            await errorHandler(new Error('Quick fail'));
            expect(mockSSEService.unregisterConnection).toHaveBeenCalled();
            
            // Re-setup router and mock for second case to avoid 'cleanedUp' flag interference
            mockSSEService.unregisterConnection.mockClear();
            const req2 = createMockReq({
                on: vi.fn().mockImplementation((event, handler) => {
                    if (event === 'error') errorHandler = handler;
                })
            });
            await getRoute()(req2, res);

            // Simulate idle timeout
            const timeoutError = new Error('Idle timeout');
            timeoutError.code = 'ECONNRESET';
            
            // Advance time to trigger idle timeout condition (>5min)
            await vi.advanceTimersByTimeAsync(360000); 
            await errorHandler(timeoutError);
            expect(mockSSEService.unregisterConnection).toHaveBeenCalledTimes(1);
        });

        it('should handle options request', async () => {
            const optionsLayer = router.stack.find(s => s.route?.path === SSEPaths.EVENTS && s.route?.methods?.options);
            const handler = optionsLayer.route.stack[0].handle;
            const req = createMockReq();
            const res = createMockRes();
            res.sendStatus = vi.fn();

            await handler(req, res);
            expect(res.sendStatus).toHaveBeenCalledWith(204);
        });
    });

    describe(`GET ${SSEPaths.HEALTH}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === SSEPaths.HEALTH);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should return health stats', async () => {
            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                status: SystemHealth.HEALTHY,
                service: 'g8ed_sse',
                localConnections: 1,
                uniqueSessions: 1
            }));
        });
    });
});
