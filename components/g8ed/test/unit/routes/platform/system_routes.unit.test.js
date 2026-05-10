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
import { createSystemRouter } from '@g8ed/routes/platform/system_routes.js';
import { SystemPaths } from '@g8ed/constants/api_paths.js';

describe('System Routes [UNIT]', () => {
    let router;
    let mockConfig;
    let mockAuthMiddleware;
    let mockRateLimiters;

    beforeEach(() => {
        mockConfig = {
            host_ips: '192.168.1.100, 10.0.0.5',
            app_url: 'https://g8e.local'
        };
        mockAuthMiddleware = {
            requireAuth: vi.fn((req, res, next) => next())
        };
        mockRateLimiters = {
            apiRateLimiter: vi.fn((req, res, next) => next())
        };

        router = createSystemRouter({
            services: {
                settingsService: mockConfig
            },

            authMiddleware: mockAuthMiddleware,
            rateLimiters: mockRateLimiters
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

    describe(`GET ${SystemPaths.NETWORK_INTERFACES}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === SystemPaths.NETWORK_INTERFACES);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should return network interfaces from host_ips and app_url', async () => {
            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                interfaces: [
                    { name: 'host', address: '192.168.1.100' },
                    { name: 'host', address: '10.0.0.5' },
                    { name: 'APP_URL', address: 'g8e.local' }
                ]
            }));
        });

        it('should handle missing host_ips', async () => {
            mockConfig.host_ips = undefined;
            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                interfaces: [
                    { name: 'APP_URL', address: 'g8e.local' }
                ]
            }));
        });

        it('should deduplicate addresses', async () => {
            mockConfig.host_ips = '192.168.1.100';
            mockConfig.app_url = 'https://192.168.1.100';
            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            const responseData = res.json.mock.calls[0][0];
            expect(responseData.interfaces).toHaveLength(1);
            expect(responseData.interfaces[0].address).toBe('192.168.1.100');
        });
    });
});
