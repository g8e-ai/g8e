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
import { createSettingsRouter } from '@g8ed/routes/platform/settings_routes.js';
import { SettingsPaths } from '@g8ed/constants/api_paths.js';

describe('Settings Routes [UNIT]', () => {
    let router;
    let mockSettingsService;
    let mockAuthMiddleware;
    let mockRateLimiters;

    beforeEach(() => {
        mockSettingsService = {
            getPlatformSettings: vi.fn(),
            getUserSettings: vi.fn(),
            getSchema: vi.fn(),
            getPageSections: vi.fn(),
            updateUserSettings: vi.fn()
        };
        mockAuthMiddleware = {
            requireAdmin: vi.fn((req, res, next) => {
                req.userId = 'admin_123';
                next();
            })
        };
        mockRateLimiters = {
            settingsRateLimiter: vi.fn((req, res, next) => next())
        };

        router = createSettingsRouter({
            services: {
                settingsService: mockSettingsService
            },

            authMiddleware: mockAuthMiddleware,
            rateLimiters: mockRateLimiters
        });
    });

    const createMockReq = (overrides = {}) => ({
        userId: 'admin_123',
        body: {},
        ...overrides
    });

    const createMockRes = () => {
        const res = {};
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        return res;
    };

    describe(`GET ${SettingsPaths.ROOT}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === SettingsPaths.ROOT && s.route?.methods?.get);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should fetch all settings', async () => {
            const mockPlatformSettings = { 'passkey_rp_name': 'g8e' };
            const mockUserSettings = { 'app_url': 'https://g8e.local' };
            const mockSchema = [
                { key: 'passkey_rp_name', default: 'g8e', secret: false },
                { key: 'app_url', default: 'https://localhost', secret: false }
            ];

            const mockSections = [
                { id: 'llm', label: 'LLM', icon: 'psychology' },
                { id: 'search', label: 'Web Search', icon: 'travel_explore' },
            ];

            mockSettingsService.getPlatformSettings.mockResolvedValue(mockPlatformSettings);
            mockSettingsService.getUserSettings.mockResolvedValue(mockUserSettings);
            mockSettingsService.getSchema.mockReturnValue(mockSchema);
            mockSettingsService.getPageSections.mockReturnValue(mockSections);

            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockSettingsService.getPlatformSettings).toHaveBeenCalled();
            expect(mockSettingsService.getUserSettings).toHaveBeenCalledWith('admin_123');
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                settings: expect.any(Array),
                sections: expect.any(Array)
            }));
            
            const responseData = res.json.mock.calls[0][0];
            expect(responseData.settings).toHaveLength(2);
            expect(responseData.settings.find(s => s.key === 'app_url').value).toBe('https://g8e.local');
            expect(responseData.sections).toHaveLength(2);
            expect(responseData.sections[0].id).toBe('llm');
        });

        it('should handle service errors', async () => {
            mockSettingsService.getPlatformSettings.mockRejectedValue(new Error('Service error'));

            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Failed to load settings'
            }));
        });
    });

    describe(`PUT ${SettingsPaths.ROOT}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === SettingsPaths.ROOT && s.route?.methods?.put);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should update settings successfully', async () => {
            const updates = { 'app_url': 'https://g8e.local' };
            const mockResult = { success: true, saved: ['app_url'] };
            mockSettingsService.updateUserSettings.mockResolvedValue(mockResult);

            const req = createMockReq({ body: { settings: updates } });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockSettingsService.updateUserSettings).toHaveBeenCalledWith('admin_123', updates);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                saved: ['app_url']
            }));
        });

        it('should reject invalid request body', async () => {
            const req = createMockReq({ body: { settings: 'invalid' } });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(400);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'settings object required'
            }));
        });

        it('should handle service update failure', async () => {
            mockSettingsService.updateUserSettings.mockRejectedValue(new Error('Write failed'));

            const req = createMockReq({ body: { settings: {} } });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Failed to save settings'
            }));
        });
    });
});
