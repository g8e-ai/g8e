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
import { LLMProvider, GeminiModel, OpenAIModel } from '@g8ed/constants/ai.js';
import { SettingsService } from '@g8ed/services/platform/settings_service.js';
import { Collections } from '@g8ed/constants/collections.js';
import { SETTINGS_DOC_ID, USER_SETTINGS_DOC_PREFIX } from '@g8ed/constants/service_config.js';

describe('Settings Service - User Settings Overlay [INTEGRATION]', () => {
    let cacheAside;
    let settingsService;

    beforeEach(() => {
        vi.clearAllMocks();

        cacheAside = {
            getDocument: vi.fn(),
            updateDocument: vi.fn(),
        };

        // We don't need real HTTP client for these logic tests
        settingsService = new SettingsService({ cacheAsideService: cacheAside, internalHttpClient: null });
    });

    it('returns platform settings when no user_id is provided', async () => {
        const platformSettings = {
            llm_primary_provider: LLMProvider.GEMINI,
            llm_model: GeminiModel.PRO_PREVIEW
        };

        cacheAside.getDocument.mockImplementation((coll, id) => {
            if (coll === Collections.SETTINGS && id === SETTINGS_DOC_ID) {
                return Promise.resolve({ settings: platformSettings });
            }
            return Promise.resolve(null);
        });

        const result = await settingsService.getPlatformSettings();
        expect(result.llm_primary_provider).toBe(LLMProvider.GEMINI);
        expect(result.llm_model).toBe(GeminiModel.PRO_PREVIEW);
    });

    it('returns user-specific settings flattened from nested structure', async () => {
        const userId = 'user-abc';
        const nestedSettings = {
            llm: { provider: LLMProvider.OPENAI, llm_model: OpenAIModel.GPT_4O },
            search: {},
            eval_judge: {},
        };

        cacheAside.getDocument.mockImplementation((coll, id) => {
            if (coll === Collections.SETTINGS && id === `${USER_SETTINGS_DOC_PREFIX}${userId}`) {
                return Promise.resolve({ user_id: userId, settings: nestedSettings });
            }
            return Promise.resolve(null);
        });

        const result = await settingsService.getUserSettings(userId);
        expect(result.llm_primary_provider).toBe(LLMProvider.OPENAI);
        expect(result.llm_model).toBe(OpenAIModel.GPT_4O);
    });

    it('persists settings in nested structure with user_id', async () => {
        const userId = 'user-abc';
        const updates = {
            llm_temperature: '0.5'
        };

        cacheAside.getDocument.mockResolvedValue({
            user_id: userId,
            settings: {
                llm: { provider: LLMProvider.GEMINI },
                search: {},
                eval_judge: {},
            },
        });
        cacheAside.updateDocument.mockResolvedValue({ success: true });

        await settingsService.updateUserSettings(userId, updates);

        expect(cacheAside.updateDocument).toHaveBeenCalledWith(
            Collections.SETTINGS,
            `${USER_SETTINGS_DOC_PREFIX}${userId}`,
            expect.objectContaining({
                user_id: userId,
                settings: expect.objectContaining({
                    llm: expect.objectContaining({
                        provider: LLMProvider.GEMINI,
                        llm_temperature: '0.5',
                    }),
                }),
            })
        );
    });
});
