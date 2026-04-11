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
import { createMockCacheAside } from '@test/mocks/cache-aside.mock.js';
import { SettingsService } from '@g8ed/services/platform/settings_service.js';
import { USER_SETTINGS } from '@g8ed/models/settings_model.js';
import { Collections } from '@g8ed/constants/collections.js';
import { SETTINGS_DOC_ID, USER_SETTINGS_DOC_PREFIX } from '@g8ed/constants/service_config.js';
import { LLMProvider } from '@g8ed/constants/ai.js';

describe('SettingsService [UNIT]', () => {
    let cacheAside;
    let service;

    beforeEach(async () => {
        vi.resetModules();
        vi.clearAllMocks();
        cacheAside = createMockCacheAside();
        service = new SettingsService({ cacheAsideService: cacheAside });
    });

    describe('constructor', () => {
        it('throws if cacheAsideService is missing', () => {
            expect(() => new SettingsService({})).toThrow('SettingsService requires a cacheAsideService instance');
        });

        it('initializes with cacheAside', () => {
            const svc = new SettingsService({ cacheAsideService: cacheAside });
            expect(svc._cache_aside).toBe(cacheAside);
        });
    });

    describe('getPlatformSettings', () => {
        it('returns empty object if document does not exist', async () => {
            const settings = await service.getPlatformSettings();
            expect(settings).toEqual({});
        });

        it('returns settings from the document', async () => {
            await cacheAside.updateDocument(Collections.SETTINGS, SETTINGS_DOC_ID, {
                settings: { https_port: '443' }
            });
            const settings = await service.getPlatformSettings();
            expect(settings).toEqual({ https_port: '443' });
        });
    });

    describe('getUserSettings', () => {
        it('returns empty object if userId is missing', async () => {
            const settings = await service.getUserSettings(null);
            expect(settings).toEqual({});
        });

        it('returns empty object if document does not exist', async () => {
            const settings = await service.getUserSettings('user-1');
            expect(settings).toEqual({});
        });

        it('returns user-specific settings', async () => {
            const userDocId = `${USER_SETTINGS_DOC_PREFIX}user-1`;
            await cacheAside.updateDocument(Collections.SETTINGS, userDocId, {
                settings: { llm_provider: LLMProvider.OPENAI }
            });
            const settings = await service.getUserSettings('user-1');
            expect(settings).toEqual({ llm_provider: LLMProvider.OPENAI });
        });
    });

    describe('savePlatformSettings', () => {
        it('creates a new document if it does not exist', async () => {
            const result = await service.savePlatformSettings({ https_port: '444' });
            expect(result.success).toBe(true);
            expect(result.saved).toContain('https_port');

            const doc = await cacheAside.getDocument(Collections.SETTINGS, SETTINGS_DOC_ID);
            expect(doc.settings.https_port).toBe('444');
        });

        it('merges with existing settings', async () => {
            await cacheAside.updateDocument(Collections.SETTINGS, SETTINGS_DOC_ID, {
                settings: { http_port: '80' }
            });
            await service.savePlatformSettings({ https_port: '443' });
            
            const doc = await cacheAside.getDocument(Collections.SETTINGS, SETTINGS_DOC_ID);
            expect(doc.settings).toEqual({ http_port: '80', https_port: '443' });
        });

        it('throws on DB failure', async () => {
            vi.spyOn(cacheAside, 'getDocument').mockResolvedValue({ settings: {} });
            vi.spyOn(cacheAside, 'updateDocument').mockResolvedValue({ success: false, error: 'DB Error' });
            await expect(service.savePlatformSettings({ https_port: '443' })).rejects.toThrow('DB Error');
        });

        it('filters out non-platform settings', async () => {
            const result = await service.savePlatformSettings({ https_port: '443', llm_provider: LLMProvider.OPENAI });
            expect(result.saved).toContain('https_port');
            expect(result.saved).not.toContain('llm_provider');
        });
    });

    describe('updateUserSettings', () => {
        it('throws if userId is missing', async () => {
            await expect(service.updateUserSettings(null, { llm_provider: LLMProvider.OPENAI })).rejects.toThrow('userId is required');
        });

        it('merges with existing user settings', async () => {
            const userDocId = `${USER_SETTINGS_DOC_PREFIX}user-1`;
            await cacheAside.updateDocument(Collections.SETTINGS, userDocId, {
                settings: { llm_provider: LLMProvider.OPENAI }
            });
            await service.updateUserSettings('user-1', { llm_model: 'gpt-4o' });

            const doc = await cacheAside.getDocument(Collections.SETTINGS, userDocId);
            expect(doc.settings).toEqual({ llm_provider: LLMProvider.OPENAI, llm_model: 'gpt-4o' });
        });

        it('validates settings against schema', async () => {
            const result = await service.updateUserSettings('user-1', { 
                llm_temperature: '2.5', // Invalid (> 2.0)
                llm_provider: LLMProvider.OPENAI  // Valid
            });
            expect(result.saved).toContain('llm_provider');
            expect(result.saved).not.toContain('llm_temperature');
        });
    });

    describe('Metadata accessors', () => {
        it('returns USER_SETTINGS schema', () => {
            expect(service.getSchema()).toBe(USER_SETTINGS);
        });
    });
});
