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
import { USER_SETTINGS, validateUserSettings } from '@g8ed/models/settings_model.js';
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

        it('returns user-specific settings flattened from nested structure', async () => {
            const userDocId = `${USER_SETTINGS_DOC_PREFIX}user-1`;
            await cacheAside.updateDocument(Collections.SETTINGS, userDocId, {
                user_id: 'user-1',
                settings: {
                    llm: { primary_provider: LLMProvider.OPENAI, primary_model: 'gpt-4o' },
                    search: {},
                    eval_judge: {},
                },
            });
            const settings = await service.getUserSettings('user-1');
            expect(settings).toEqual({ llm_primary_provider: LLMProvider.OPENAI, llm_model: 'gpt-4o' });
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
            const result = await service.savePlatformSettings({ https_port: '443', llm_primary_provider: LLMProvider.OPENAI });
            expect(result.saved).toContain('https_port');
            expect(result.saved).not.toContain('llm_primary_provider');
        });

        it('refuses to overwrite writeOnce bootstrap secrets already set in DB', async () => {
            // Simulate g8eo SecretManager having bootstrapped secrets.
            await cacheAside.updateDocument(Collections.SETTINGS, SETTINGS_DOC_ID, {
                settings: {
                    internal_auth_token:    'bootstrapped-token',
                    session_encryption_key: 'bootstrapped-key',
                },
            });

            const result = await service.savePlatformSettings({
                internal_auth_token:    'malicious-ui-token',
                session_encryption_key: 'malicious-ui-key',
                https_port:             '444',
            });

            expect(result.success).toBe(true);
            expect(result.saved).toEqual(['https_port']);
            expect(result.skipped).toEqual(
                expect.arrayContaining(['internal_auth_token', 'session_encryption_key'])
            );

            // Bootstrap secrets must be untouched; only https_port advanced.
            const doc = await cacheAside.getDocument(Collections.SETTINGS, SETTINGS_DOC_ID);
            expect(doc.settings.internal_auth_token).toBe('bootstrapped-token');
            expect(doc.settings.session_encryption_key).toBe('bootstrapped-key');
            expect(doc.settings.https_port).toBe('444');
        });

        it('allows writeOnce keys on first write when DB is empty', async () => {
            const result = await service.savePlatformSettings({
                internal_auth_token: 'first-write-token',
            });
            expect(result.saved).toContain('internal_auth_token');
            expect(result.skipped).toEqual([]);
            const doc = await cacheAside.getDocument(Collections.SETTINGS, SETTINGS_DOC_ID);
            expect(doc.settings.internal_auth_token).toBe('first-write-token');
        });
    });

    describe('updateUserSettings', () => {
        it('throws if userId is missing', async () => {
            await expect(service.updateUserSettings(null, { llm_primary_provider: LLMProvider.OPENAI })).rejects.toThrow('userId is required');
        });

        it('merges with existing user settings in nested structure', async () => {
            const userDocId = `${USER_SETTINGS_DOC_PREFIX}user-1`;
            await cacheAside.updateDocument(Collections.SETTINGS, userDocId, {
                user_id: 'user-1',
                settings: {
                    llm: { primary_provider: LLMProvider.OPENAI },
                    search: {},
                    eval_judge: {},
                },
            });
            await service.updateUserSettings('user-1', { llm_model: 'gpt-4o' });

            const doc = await cacheAside.getDocument(Collections.SETTINGS, userDocId);
            expect(doc.settings.llm).toEqual(
                expect.objectContaining({ primary_provider: LLMProvider.OPENAI, primary_model: 'gpt-4o' })
            );
            expect(doc.user_id).toBe('user-1');
        });


        it('writes user_id at document level', async () => {
            await service.updateUserSettings('user-1', { llm_primary_provider: LLMProvider.GEMINI });
            const userDocId = `${USER_SETTINGS_DOC_PREFIX}user-1`;
            const doc = await cacheAside.getDocument(Collections.SETTINGS, userDocId);
            expect(doc.user_id).toBe('user-1');
        });
    });

    describe('Metadata accessors', () => {
        it('returns USER_SETTINGS schema', () => {
            expect(service.getSchema()).toBe(USER_SETTINGS);
        });
    });
});

describe('validateUserSettings cross-field validation [UNIT]', () => {
    describe('LLM provider credential validation', () => {
        it('passes when primary provider has required API key', () => {
            const result = validateUserSettings({
                llm_primary_provider: LLMProvider.ANTHROPIC,
                anthropic_api_key: 'sk-ant-123'
            });
            expect(result.errors).toHaveLength(0);
        });

        it('fails when primary provider is set without required API key', () => {
            const result = validateUserSettings({
                llm_primary_provider: LLMProvider.ANTHROPIC
            });
            expect(result.errors).toContain('anthropic_api_key is required when anthropic is set as primary provider');
        });

        it('fails when primary provider is set with empty API key', () => {
            const result = validateUserSettings({
                llm_primary_provider: LLMProvider.ANTHROPIC,
                anthropic_api_key: ''
            });
            expect(result.errors).toContain('anthropic_api_key is required when anthropic is set as primary provider');
        });

        it('fails when primary provider is set with whitespace-only API key', () => {
            const result = validateUserSettings({
                llm_primary_provider: LLMProvider.ANTHROPIC,
                anthropic_api_key: '   '
            });
            expect(result.errors).toContain('anthropic_api_key is required when anthropic is set as primary provider');
        });

        it('passes when assistant provider has required API key', () => {
            const result = validateUserSettings({
                llm_assistant_provider: LLMProvider.OPENAI,
                openai_api_key: 'sk-123'
            });
            expect(result.errors).toHaveLength(0);
        });

        it('fails when assistant provider is set without required API key', () => {
            const result = validateUserSettings({
                llm_assistant_provider: LLMProvider.OPENAI
            });
            expect(result.errors).toContain('openai_api_key is required when openai is set as assistant provider');
        });

        it('validates both primary and assistant providers', () => {
            const result = validateUserSettings({
                llm_primary_provider: LLMProvider.GEMINI,
                llm_assistant_provider: LLMProvider.OLLAMA,
                gemini_api_key: 'gemini-key',
                ollama_endpoint: '127.0.0.1:11434'
            });
            expect(result.errors).toHaveLength(0);
        });

        it('fails when either provider is missing credentials', () => {
            const result = validateUserSettings({
                llm_primary_provider: LLMProvider.GEMINI,
                llm_assistant_provider: LLMProvider.OLLAMA,
                gemini_api_key: 'gemini-key'
            });
            expect(result.errors).toContain('ollama_endpoint is required when ollama is set as assistant provider');
        });

        it('passes when provider fields are empty or not set', () => {
            const result = validateUserSettings({
                llm_primary_provider: '',
                llm_assistant_provider: ''
            });
            expect(result.errors).toHaveLength(0);
        });

        it('passes when provider fields are not present in updates', () => {
            const result = validateUserSettings({});
            expect(result.errors).toHaveLength(0);
        });
    });

    describe('Vertex AI Search credential validation', () => {
        it('passes when search is disabled', () => {
            const result = validateUserSettings({
                vertex_search_enabled: false
            });
            expect(result.errors).toHaveLength(0);
        });

        it('passes when search is enabled with all required fields', () => {
            const result = validateUserSettings({
                vertex_search_enabled: true,
                vertex_search_project_id: 'my-project',
                vertex_search_engine_id: 'my-engine',
                vertex_search_api_key: 'AIza123'
            });
            expect(result.errors).toHaveLength(0);
        });

        it('fails when search is enabled without project_id', () => {
            const result = validateUserSettings({
                vertex_search_enabled: true,
                vertex_search_engine_id: 'my-engine',
                vertex_search_api_key: 'AIza123'
            });
            expect(result.errors).toContain('vertex_search_project_id is required when vertex_search_enabled is true');
        });

        it('fails when search is enabled without engine_id', () => {
            const result = validateUserSettings({
                vertex_search_enabled: true,
                vertex_search_project_id: 'my-project',
                vertex_search_api_key: 'AIza123'
            });
            expect(result.errors).toContain('vertex_search_engine_id is required when vertex_search_enabled is true');
        });

        it('fails when search is enabled without api_key', () => {
            const result = validateUserSettings({
                vertex_search_enabled: true,
                vertex_search_project_id: 'my-project',
                vertex_search_engine_id: 'my-engine'
            });
            expect(result.errors).toContain('vertex_search_api_key is required when vertex_search_enabled is true');
        });

        it('fails when search is enabled with empty required fields', () => {
            const result = validateUserSettings({
                vertex_search_enabled: true,
                vertex_search_project_id: '',
                vertex_search_engine_id: '',
                vertex_search_api_key: ''
            });
            expect(result.errors.length).toBeGreaterThan(0);
        });
    });

    describe('Combined validation scenarios', () => {
        it('validates both provider and search requirements together', () => {
            const result = validateUserSettings({
                llm_primary_provider: LLMProvider.ANTHROPIC,
                anthropic_api_key: 'sk-ant-123',
                vertex_search_enabled: true,
                vertex_search_project_id: 'my-project',
                vertex_search_engine_id: 'my-engine',
                vertex_search_api_key: 'AIza123'
            });
            expect(result.errors).toHaveLength(0);
        });

        it('reports multiple validation errors', () => {
            const result = validateUserSettings({
                llm_primary_provider: LLMProvider.ANTHROPIC,
                vertex_search_enabled: true
            });
            expect(result.errors.length).toBeGreaterThan(1);
            expect(result.errors).toContain('anthropic_api_key is required when anthropic is set as primary provider');
        });
    });
});
