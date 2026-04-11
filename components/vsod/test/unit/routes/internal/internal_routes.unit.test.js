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
import { createInternalRouter } from '@vsod/routes/internal/internal_routes.js';
import { InternalPaths } from '@vsod/constants/api_paths.js';
import { SourceComponent, SystemHealth } from '@vsod/constants/ai.js';

describe('Internal Routes [UNIT]', () => {
    let router;
    let mockAuthorizationMiddleware;

    beforeEach(() => {
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };

        // Other services are just passed through to sub-routers, so we can use empty mocks
        router = createInternalRouter({
            services: {
                sseService: {},
                bindingService: {},
                operatorService: {},
                userService: {},
                webSessionService: {},
                passkeyAuthService: {},
                deviceLinkService: {},
                settingsService: {},
                g8eNodeOperatorService: {}
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

    describe(`GET ${InternalPaths.HEALTH}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === '/health');
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should return internal health status', () => {
            const req = createMockReq();
            const res = createMockRes();

            getRoute()(req, res);

            // The middleware is applied at the route level: router.get(path, middleware, handler)
            // In Express, the middleware and the final handler are both in the route's stack.
            // When we call the final handler directly in the test, we bypass the middleware.
            // To test middleware presence, we can inspect the route stack.
            const layer = router.stack.find(s => s.route?.path === '/health');
            const middleware = layer.route.stack.find(s => s.handle === mockAuthorizationMiddleware.requireInternalOrigin);
            expect(middleware).toBeDefined();

            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                message: 'Internal API healthy',
                vsodb_status: 'healthy',
                vse_status: 'healthy', 
                vsa_status: 'healthy',
                uptime_seconds: expect.any(Number),
                memory_usage: expect.any(Object)
            }));
        });
    });
});
