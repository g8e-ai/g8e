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
import { createMetricsRouter } from '@g8ed/routes/platform/metrics_routes.js';
import { MetricsPaths } from '@g8ed/constants/api_paths.js';
import { SourceComponent, SystemHealth } from '@g8ed/constants/ai.js';
import { MetricsHealthResponse } from '@g8ed/models/response_models.js';

describe('Metrics Routes [UNIT]', () => {
    let router;
    let mockCacheAsideService;
    let mockAuthorizationMiddleware;

    beforeEach(() => {
        mockCacheAsideService = {
            kvGet: vi.fn()
        };
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };

        router = createMetricsRouter({
            services: {
                cacheAsideService: mockCacheAsideService
            },

            authorizationMiddleware: mockAuthorizationMiddleware
        });
    });

    const createMockReq = () => ({
        query: {},
        params: {}
    });

    const createMockRes = () => {
        const res = {};
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        return res;
    };

    describe(`GET ${MetricsPaths.HEALTH}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === MetricsPaths.HEALTH);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should return healthy status when g8es KV is accessible', async () => {
            mockCacheAsideService.kvGet.mockResolvedValue('ok');

            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockCacheAsideService.kvGet).toHaveBeenCalledWith('__health_check__');
            expect(res.status).toHaveBeenCalledWith(200);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                status: SystemHealth.HEALTHY,
                service: SourceComponent.G8ED,
                g8es: { healthy: true }
            }));
        });

        it('should return degraded status when g8es KV check fails', async () => {
            mockCacheAsideService.kvGet.mockRejectedValue(new Error('KV connection lost'));

            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(503);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: false,
                status: SystemHealth.DEGRADED,
                g8es: { healthy: false }
            }));
        });

        it('should handle unexpected errors with 503 status', async () => {
            // Force an error outside the KV check try-catch
            mockCacheAsideService.kvGet.mockImplementation(() => {
                throw new Error('Unexpected crash');
            });

            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(503);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: false,
                status: SystemHealth.DEGRADED,
                g8es: { healthy: false }
            }));
        });
    });
});
