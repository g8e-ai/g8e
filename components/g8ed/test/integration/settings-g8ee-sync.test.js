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
import express from 'express';
import request from 'supertest';
import { SettingsService } from '@g8ed/services/platform/settings_service.js';
import { Collections } from '@g8ed/constants/collections.js';
import { SETTINGS_DOC_ID } from '@g8ed/constants/service_config.js';
import { apiPaths } from '@g8ed/constants/api_paths.js';
import { getInternalHttpClient } from '@g8ed/services/clients/internal_http_client.js';

// Mock InternalHttpClient to verify the sync call to g8ee
const mockG8eeRequest = vi.fn();
vi.mock('@g8ed/services/clients/internal_http_client.js', () => ({
    getInternalHttpClient: vi.fn(() => ({
        request: mockG8eeRequest.mockResolvedValue({ success: true })
    }))
}));

describe('Settings -> g8ee Sync Integration [CROSS-SERVICE]', () => {
    let cacheAside;
    let settingsService;
    let internalHttpClient;

    beforeEach(() => {
        vi.clearAllMocks();

        cacheAside = {
            getDocument: vi.fn(),
            updateDocument: vi.fn(),
            createDocument: vi.fn(),
        };

        internalHttpClient = getInternalHttpClient();
        settingsService = new SettingsService({ cacheAsideService: cacheAside, internalHttpClient });
    });

    it('syncs user settings to g8ee when saved for a user', async () => {
        const userId = 'user-123';
        const updates = {
            llm_model: 'claude-sonnet-4-6',
            llm_temperature: '0.8'
        };

        // Mock DB success (existing doc with nested structure)
        cacheAside.getDocument.mockResolvedValue({
            user_id: userId,
            settings: { llm: {}, search: {}, eval_judge: {} },
        });
        cacheAside.updateDocument.mockResolvedValue({ success: true });

        const result = await settingsService.updateUserSettings(userId, updates);

        expect(result.success).toBe(true);
        expect(result.saved).toContain('llm_model');
        expect(result.saved).toContain('llm_temperature');

        // Verify DB write uses nested structure with user_id
        expect(cacheAside.updateDocument).toHaveBeenCalledWith(
            Collections.SETTINGS,
            `user_settings_${userId}`,
            expect.objectContaining({
                user_id: userId,
                settings: expect.objectContaining({
                    llm: expect.objectContaining({
                        llm_model: 'claude-sonnet-4-6',
                        llm_temperature: '0.8',
                    }),
                }),
            })
        );

        // Verify Sync to g8ee
        expect(mockG8eeRequest).toHaveBeenCalledWith(
            'g8ee',
            apiPaths.g8ee.settingsUser(),
            expect.objectContaining({
                method: 'PATCH',
                body: updates
            })
        );
    });

    it('syncs to platform settings and NOT g8ee when userId is null (setup context)', async () => {
        const updates = {
            g8e_internal_http_url: 'https://g8es:9000',
            https_port: '443'
        };

        // Mock DB success
        cacheAside.getDocument.mockResolvedValue({ settings: {} });
        cacheAside.updateDocument.mockResolvedValue({ success: true });

        const result = await settingsService.savePlatformSettings(updates);

        expect(result.success).toBe(true);

        // Verify DB write to SETTINGS
        expect(cacheAside.updateDocument).toHaveBeenCalledWith(
            Collections.SETTINGS,
            SETTINGS_DOC_ID,
            expect.objectContaining({
                settings: expect.objectContaining(updates)
            })
        );

        // Verify NO Sync to g8ee (g8ee loads platform settings directly from DB on boot/refresh)
        expect(mockG8eeRequest).not.toHaveBeenCalled();
    });
});
