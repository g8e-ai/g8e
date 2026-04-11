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
import { createInternalSettingsRouter } from '@vsod/routes/internal/internal_settings_routes.js';

describe('Internal Settings Routes [UNIT]', () => {
    let router;
    let mockSettingsService;
    let mockAuthorizationMiddleware;

    beforeEach(() => {
        mockSettingsService = {
            getPlatformSettings: vi.fn(),
            getSchema: vi.fn(),
            updatePlatformSettings: vi.fn()
        };
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };

        router = createInternalSettingsRouter({
            services: {
                settingsService: mockSettingsService
            },

            authorizationMiddleware: mockAuthorizationMiddleware
        });
    });

    const createMockReq = (overrides = {}) => ({
        params: {},
        body: {},
        query: {},
        headers: {},
        ...overrides
    });

    const createMockRes = () => {
        const res = {};
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        return res;
    };

    const getRouteHandler = (path, method = 'get') => {
        const layer = router.stack.find(s => s.route?.path === path && s.route?.methods[method]);
        if (!layer) throw new Error(`Route ${method.toUpperCase()} ${path} not found`);
        return layer.route.stack[layer.route.stack.length - 1].handle;
    };

    describe('GET /', () => {
        it('should fetch non-secret settings', async () => {
            const req = createMockReq();
            const res = createMockRes();
            const mockPlatformSettings = {
                site_name: 'g8e',
                api_key: 'secret-val'
            };
            const mockSchema = [
                { key: 'site_name', default: 'DefaultOps', secret: false, label: 'Site Name', section: 'general' },
                { key: 'api_key', default: '', secret: true, label: 'API Key', section: 'security' }
            ];

            mockSettingsService.getPlatformSettings.mockResolvedValue(mockPlatformSettings);
            mockSettingsService.getSchema.mockReturnValue(mockSchema);

            await getRouteHandler('/')(req, res);

            expect(mockSettingsService.getPlatformSettings).toHaveBeenCalled();
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                message: 'Settings fetched successfully',
                settings: {
                    site_name: {
                        value: 'g8e',
                        envLocked: false,
                        label: 'Site Name',
                        section: 'general'
                    }
                }
            }));
            // Secret setting should be omitted
            expect(res.json.mock.calls[0][0].settings).not.toHaveProperty('api_key');
        });

        it('should return 500 if settings service fails', async () => {
            const req = createMockReq();
            const res = createMockRes();

            mockSettingsService.getPlatformSettings.mockRejectedValue(new Error('DB failure'));

            await getRouteHandler('/')(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Failed to load settings'
            }));
        });
    });

    describe('PUT /', () => {
        it('should update settings successfully', async () => {
            const updates = { site_name: 'New Name' };
            const req = createMockReq({ body: { settings: updates } });
            const res = createMockRes();
            const mockResult = { success: true, saved: ['site_name'] };

            mockSettingsService.updatePlatformSettings.mockResolvedValue(mockResult);

            await getRouteHandler('/', 'put')(req, res);

            expect(mockSettingsService.updatePlatformSettings).toHaveBeenCalledWith(updates);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                saved: ['site_name']
            }));
        });

        it('should return 400 if settings object is missing or invalid', async () => {
            const invalidBodies = [
                {},
                { settings: 'not-an-object' },
                { settings: [] }
            ];

            for (const body of invalidBodies) {
                const req = createMockReq({ body });
                const res = createMockRes();

                await getRouteHandler('/', 'put')(req, res);

                expect(res.status).toHaveBeenCalledWith(400);
                expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                    error: 'settings object required'
                }));
            }
        });

        it('should return 500 if updatePlatformSettings fails', async () => {
            const req = createMockReq({ body: { settings: { k: 'v' } } });
            const res = createMockRes();

            mockSettingsService.updatePlatformSettings.mockRejectedValue(new Error('Save failed'));

            await getRouteHandler('/', 'put')(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Failed to save settings'
            }));
        });

        it('should handle errors during update', async () => {
            const req = createMockReq({ body: { settings: { k: 'v' } } });
            const res = createMockRes();

            mockSettingsService.updatePlatformSettings.mockRejectedValue(new Error('Unexpected error'));

            await getRouteHandler('/', 'put')(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Failed to save settings'
            }));
        });
    });
});
