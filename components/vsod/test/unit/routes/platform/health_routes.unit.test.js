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
import { createHealthRouter } from '@vsod/routes/platform/health_routes.js';
import { HealthCheckService } from '@vsod/services/platform/health_check_service.js';
import { SystemHealth, SourceComponent } from '@vsod/constants/ai.js';
import { HealthPaths } from '@vsod/constants/api_paths.js';
import { Collections } from '@vsod/constants/collections.js';

describe('Health Routes [UNIT]', () => {
    let router;
    let mockCacheAsideService;
    let mockWebSessionService;
    let mockAuthorizationMiddleware;

    beforeEach(() => {
        mockCacheAsideService = {
            getDocument: vi.fn()
        };
        mockWebSessionService = {
            isHealthy: vi.fn(),
            getSessionCount: vi.fn()
        };
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };

        router = createHealthRouter({
            services: {
                healthCheckService: new HealthCheckService({
                    cacheAsideService: mockCacheAsideService,
                    webSessionService: mockWebSessionService
                })
            },
            authorizationMiddleware: mockAuthorizationMiddleware
        });
    });

    const createMockRes = () => {
        const res = {};
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        return res;
    };

    describe(`GET ${HealthPaths.ROOT}`, () => {
        it('should return basic healthy status', async () => {
            const req = {};
            const res = createMockRes();
            const route = router.stack.find(s => s.route?.path === HealthPaths.ROOT).route.stack[0].handle;

            await route(req, res);

            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                status: SystemHealth.HEALTHY,
                service: SourceComponent.VSOD
            }));
        });
    });

    describe(`GET ${HealthPaths.LIVE}`, () => {
        it('should return alive status', async () => {
            const req = {};
            const res = createMockRes();
            const route = router.stack.find(s => s.route?.path === HealthPaths.LIVE).route.stack[0].handle;

            await route(req, res);

            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                status: 'alive'
            }));
        });
    });

    describe(`GET ${HealthPaths.STORE}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === HealthPaths.STORE);
            // Index 1 because Index 0 is requireInternalOrigin middleware
            return layer.route.stack[1].handle;
        };

        it('should return ready when all components are up', async () => {
            mockWebSessionService.isHealthy.mockReturnValue(true);
            const req = {};
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                status: 'ready',
                details: expect.objectContaining({
                    checks: {
                        vsodb: 'up',
                        database: 'up'
                    }
                })
            }));
        });

        it('should return 503 when VSODB is down', async () => {
            mockWebSessionService.isHealthy.mockReturnValue(false);
            const req = {};
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(503);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: false,
                status: SystemHealth.UNHEALTHY,
                details: expect.objectContaining({
                    checks: expect.objectContaining({
                        vsodb: 'down'
                    })
                })
            }));
        });

        it('should return 503 when cacheAsideService is missing', async () => {
            router = createHealthRouter({
                services: {
                    healthCheckService: new HealthCheckService({
                        cacheAsideService: null,
                        webSessionService: mockWebSessionService
                    })
                },
                authorizationMiddleware: mockAuthorizationMiddleware
            });
            mockWebSessionService.isHealthy.mockReturnValue(true);
            const req = {};
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(503);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: false,
                status: 'unhealthy',
                details: expect.objectContaining({
                    checks: expect.objectContaining({
                        database: 'not_initialized'
                    })
                })
            }));
        });
    });

    describe(`GET ${HealthPaths.DETAILS}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === HealthPaths.DETAILS);
            return layer.route.stack[1].handle;
        };

        it('should return full healthy details', async () => {
            mockWebSessionService.isHealthy.mockReturnValue(true);
            mockWebSessionService.getSessionCount.mockResolvedValue(42);
            mockCacheAsideService.getDocument.mockResolvedValue({ id: 'platform_settings' });

            const req = {};
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(200);
            const response = res.json.mock.calls[0][0];
            expect(response.status).toBe(SystemHealth.HEALTHY);
            expect(response.checks.vsodb.activeSessions).toBe(42);
            expect(response.checks.database.status).toBe(SystemHealth.HEALTHY);
        });

        it('should return 503 if database check fails', async () => {
            mockWebSessionService.isHealthy.mockReturnValue(true);
            mockCacheAsideService.getDocument.mockResolvedValue(null);

            const req = {};
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(503);
            const response = res.json.mock.calls[0][0];
            expect(response.status).toBe(SystemHealth.UNHEALTHY);
            expect(response.checks.database.status).toBe(SystemHealth.UNHEALTHY);
        });

        it('should handle errors gracefully', async () => {
            mockWebSessionService.isHealthy.mockImplementation(() => {
                throw new Error('Explosion');
            });

            const req = {};
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(503);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                status: SystemHealth.UNHEALTHY
            }));
        });
    });
});
