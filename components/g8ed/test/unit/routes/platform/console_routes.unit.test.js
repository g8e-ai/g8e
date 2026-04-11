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

vi.mock('@g8ed/utils/logger.js', async () => {
    const actual = await vi.importActual('@g8ed/utils/logger.js');
    return {
        ...actual,
        logger: {
            error: vi.fn(),
            info: vi.fn(),
            warn: vi.fn(),
            debug: vi.fn()
        },
        getLogRingBuffer: vi.fn(),
        addLogListener: vi.fn(),
        removeLogListener: vi.fn()
    };
});

import { createConsoleRouter } from '@g8ed/routes/platform/console_routes.js';
import { ConsolePaths } from '@g8ed/constants/api_paths.js';
import { Collections } from '@g8ed/constants/collections.js';
import { EventType } from '@g8ed/constants/events.js';
import * as loggerUtils from '@g8ed/utils/logger.js';
import { 
    PlatformOverviewResponse,
    UserStatsResponse,
    OperatorStatsResponse,
    SessionStatsResponse,
    AIUsageStatsResponse,
    LoginAuditStatsResponse,
    RealTimeMetricsResponse,
    ComponentHealthResponse,
    DBCollectionsResponse, 
    DBQueryResponse, 
    ErrorResponse, 
    KVKeyResponse, 
    KVScanResponse, 
    SimpleSuccessResponse 
} from '@g8ed/models/response_models.js';

describe('Console Routes [UNIT]', () => {
    let router;
    let mockConsoleMetricsService;
    let mockAuthMiddleware;
    let mockRateLimiters;

    beforeEach(() => {
        mockConsoleMetricsService = {
            getPlatformOverview: vi.fn(),
            getUserStats: vi.fn(),
            getOperatorStats: vi.fn(),
            getSessionStats: vi.fn(),
            getAIUsageStats: vi.fn(),
            getLoginAuditStats: vi.fn(),
            getRealTimeMetrics: vi.fn(),
            clearCache: vi.fn(),
            getComponentHealth: vi.fn(),
            scanKV: vi.fn(),
            getKVKey: vi.fn(),
            queryCollection: vi.fn()
        };
        mockAuthMiddleware = {
            requireSuperAdmin: vi.fn((req, res, next) => next())
        };
        mockRateLimiters = {
            consoleRateLimiter: vi.fn((req, res, next) => next())
        };

        router = createConsoleRouter({
            services: {
                consoleMetricsService: mockConsoleMetricsService
            },

            authMiddleware: mockAuthMiddleware,
            rateLimiters: mockRateLimiters
        });
    });

    afterEach(() => {
        vi.restoreAllMocks();
    });


    const createMockReq = (overrides = {}) => ({
        userId: 'admin_123',
        query: {},
        params: {},
        on: vi.fn(),
        ...overrides
    });

    const createMockRes = () => {
        const res = {};
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        res.setHeader = vi.fn().mockReturnValue(res);
        res.status = vi.fn().mockReturnValue(res);
        res.flushHeaders = vi.fn().mockReturnValue(res);
        res.send = vi.fn().mockReturnValue(res);
        return res;
    };

    describe(`GET ${ConsolePaths.OVERVIEW}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === ConsolePaths.OVERVIEW);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should fetch and return platform overview using typed models', async () => {
            const mockData = { 
                timestamp: new Date(),
                users: { total: 10 },
                operators: { total: 5 },
                sessions: { total: 3 },
                cache: { hitRate: 0.8 },
                system: { status: 'healthy' }
            };
            mockConsoleMetricsService.getPlatformOverview.mockResolvedValue(mockData);

            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockConsoleMetricsService.getPlatformOverview).toHaveBeenCalled();
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                message: 'Overview fetched successfully',
                timestamp: mockData.timestamp.toISOString(),
                users: mockData.users,
                operators: mockData.operators,
                sessions: mockData.sessions,
                cache: mockData.cache,
                system: mockData.system
            }));
        });

        it('should handle service errors with typed error response', async () => {
            mockConsoleMetricsService.getPlatformOverview.mockRejectedValue(new Error('DB error'));

            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Failed to fetch platform overview'
            }));
        });
    });

    describe(`GET ${ConsolePaths.KV_SCAN}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === ConsolePaths.KV_SCAN);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should perform KV scan with default parameters', async () => {
            const mockData = { keys: ['k1'], count: 1, cursor: '0', has_more: false };
            mockConsoleMetricsService.scanKV.mockResolvedValue(mockData);

            const req = createMockReq({ query: { pattern: 'test:*' } });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockConsoleMetricsService.scanKV).toHaveBeenCalledWith('test:*', '0', 50);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                pattern: 'test:*',
                cursor: mockData.cursor,
                keys: mockData.keys,
                count: mockData.count,
                has_more: mockData.has_more
            }));
        });
    });

    describe('Metrics Routes', () => {
        const testMetricsRoute = (path, serviceMethod, responseMessage) => {
            describe(`GET ${path}`, () => {
                const getRoute = () => {
                    const layer = router.stack.find(s => s.route?.path === path);
                    return layer.route.stack[layer.route.stack.length - 1].handle;
                };

                it(`should fetch and return ${serviceMethod} successfully`, async () => {
                    let mockData;
                    if (serviceMethod === 'getUserStats') {
                        mockData = {
                            total: 100,
                            activity: { daily: 50, weekly: 80 },
                            newUsersLastWeek: 10
                        };
                    } else if (serviceMethod === 'getLoginAuditStats') {
                        mockData = {
                            total: 100,
                            successful: 80,
                            failed: 20,
                            locked: 5,
                            anomalies: 2,
                            byHour: { '00': 10, '01': 5 }
                        };
                    } else if (serviceMethod === 'getRealTimeMetrics') {
                        mockData = {
                            timestamp: new Date(),
                            g8es: { connections: 10 },
                            cache: { hitRate: 0.8 }
                        };
                    } else if (serviceMethod === 'getOperatorStats') {
                        mockData = {
                            total: 50,
                            statusDistribution: { active: 30, inactive: 20 },
                            typeDistribution: { chat: 25, analysis: 25 },
                            health: { healthy: 45, unhealthy: 5 }
                        };
                    } else if (serviceMethod === 'getSessionStats') {
                        mockData = {
                            web: 100,
                            operator: 50,
                            total: 150,
                            boundOperators: 25
                        };
                    } else if (serviceMethod === 'getAIUsageStats') {
                        mockData = {
                            totalInvestigations: 200,
                            activeInvestigations: 20,
                            completedInvestigations: 180
                        };
                    } else if (serviceMethod === 'getComponentHealth') {
                        mockData = {
                            overall: 'healthy',
                            timestamp: new Date(),
                            components: { g8es: 'up', cache: 'up' }
                        };
                    } else {
                        mockData = { some: 'data' };
                    }
                    mockConsoleMetricsService[serviceMethod].mockResolvedValue(mockData);

                    const req = createMockReq();
                    const res = createMockRes();

                    await getRoute()(req, res);

                    expect(mockConsoleMetricsService[serviceMethod]).toHaveBeenCalled();
                    // For specific metrics routes, check for structured fields instead of generic data
                    if (serviceMethod === 'getUserStats') {
                        expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                            success: true,
                            message: responseMessage,
                            total: expect.any(Number),
                            activity: expect.any(Object),
                            newUsersLastWeek: expect.any(Number)
                        }));
                    } else if (serviceMethod === 'getOperatorStats') {
                        expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                            success: true,
                            message: responseMessage,
                            total: expect.any(Number),
                            statusDistribution: expect.any(Object),
                            typeDistribution: expect.any(Object),
                            health: expect.any(Object)
                        }));
                    } else {
                        // For other routes, just check basic structure
                        expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                            success: true,
                            message: responseMessage
                        }));
                    }
                });

                it('should handle service errors', async () => {
                    mockConsoleMetricsService[serviceMethod].mockRejectedValue(new Error('Fail'));

                    const req = createMockReq();
                    const res = createMockRes();

                    await getRoute()(req, res);

                    expect(res.status).toHaveBeenCalledWith(500);
                });
            });
        };

        testMetricsRoute(ConsolePaths.METRICS_USERS, 'getUserStats', 'User stats fetched successfully');
        testMetricsRoute(ConsolePaths.METRICS_OPERATORS, 'getOperatorStats', 'Operator stats fetched successfully');
        testMetricsRoute(ConsolePaths.METRICS_SESSIONS, 'getSessionStats', 'Session stats fetched successfully');
        testMetricsRoute(ConsolePaths.METRICS_AI, 'getAIUsageStats', 'AI stats fetched successfully');
        testMetricsRoute(ConsolePaths.METRICS_LOGIN_AUDIT, 'getLoginAuditStats', 'Login audit stats fetched successfully');
        testMetricsRoute(ConsolePaths.METRICS_REALTIME, 'getRealTimeMetrics', 'Real-time metrics fetched successfully');
        testMetricsRoute(ConsolePaths.COMPONENTS_HEALTH, 'getComponentHealth', 'Component health fetched successfully');
    });

    describe(`POST ${ConsolePaths.CACHE_CLEAR}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === ConsolePaths.CACHE_CLEAR);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should clear cache successfully', async () => {
            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockConsoleMetricsService.clearCache).toHaveBeenCalled();
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                message: 'Metrics cache cleared'
            }));
        });
    });

    describe(`GET ${ConsolePaths.KV_KEY}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === ConsolePaths.KV_KEY);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should fetch KV key successfully', async () => {
            const createdDate = new Date();
            const updatedDate = new Date();
            const mockData = { value: 'val', exists: true, content_type: 'string', created_at: createdDate, updated_at: updatedDate };
            mockConsoleMetricsService.getKVKey.mockResolvedValue(mockData);

            const req = createMockReq({ query: { key: 'test:key' } });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockConsoleMetricsService.getKVKey).toHaveBeenCalledWith('test:key');
            const response = res.json.mock.calls[0][0];
            expect(response.success).toBe(true);
            expect(response.message).toBe('KV key fetched successfully');
            expect(response.key).toBe('test:key');
            expect(response.exists).toBe(mockData.exists);
            expect(response.value).toBe(mockData.value);
            expect(response.content_type).toBe(mockData.content_type);
            // forClient() serializes dates to strings for the wire
            expect(response.created_at).toBe(createdDate.toISOString());
            expect(response.updated_at).toBe(updatedDate.toISOString());
        });

        it('should return 400 if key is missing', async () => {
            const req = createMockReq({ query: {} });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(400);
        });
    });

    describe(`GET ${ConsolePaths.DB_COLLECTIONS}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === ConsolePaths.DB_COLLECTIONS);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should return all collections', async () => {
            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                collections: expect.arrayContaining(Object.values(Collections))
            }));
        });
    });

    describe(`GET ${ConsolePaths.DB_QUERY}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === ConsolePaths.DB_QUERY);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should query a valid collection', async () => {
            const mockData = { documents: [], count: 0 };
            mockConsoleMetricsService.queryCollection.mockResolvedValue(mockData);

            const req = createMockReq({ query: { collection: Collections.USERS, limit: '10' } });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockConsoleMetricsService.queryCollection).toHaveBeenCalledWith(Collections.USERS, 10);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                collection: Collections.USERS,
                documents: mockData.documents,
                count: mockData.count,
                limit: 10
            }));
        });

        it('should reject invalid collection', async () => {
            const req = createMockReq({ query: { collection: 'forbidden_table' } });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(400);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Invalid or missing collection'
            }));
        });
    });
});
