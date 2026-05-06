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
import { ApiKeyService } from '@g8ed/services/auth/api_key_service.js';
import { ApiKeyDocument } from '@g8ed/models/auth_models.js';
import { ApiKeyStatus, ApiKeyError } from '@g8ed/constants/auth.js';
import { API_KEY_PREFIX } from '@g8ed/constants/operator_defaults.js';
import { G8eHttpContext } from '@g8ed/models/request_models.js';

describe('ApiKeyService [UNIT]', () => {
    let apiKeyDataService;
    let service;

    beforeEach(() => {
        apiKeyDataService = {
            makeDocId: vi.fn().mockReturnValue('doc123'),
            createKey: vi.fn().mockResolvedValue(),
            getKey: vi.fn(),
            updateKey: vi.fn().mockResolvedValue(),
            deleteKey: vi.fn().mockResolvedValue(),
        };
        service = new ApiKeyService({ apiKeyDataService });
    });

    describe('constructor', () => {
        it('throws if apiKeyDataService is missing', () => {
            expect(() => new ApiKeyService({})).toThrow('ApiKeyService requires apiKeyDataService');
        });
    });

    describe('generateRawKey', () => {
        it('delegates to internalHttpClient.generateApiKey when available', async () => {
            const mockKey = `${API_KEY_PREFIX}${'a'.repeat(64)}`;
            const mockInternalHttpClient = {
                generateApiKey: vi.fn().mockResolvedValue({ success: true, api_key: mockKey })
            };
            const serviceWithHttp = new ApiKeyService({ apiKeyDataService, internalHttpClient: mockInternalHttpClient });
            const context = G8eHttpContext.parse({ user_id: 'u1', organization_id: 'o1', case_id: 'test', investigation_id: 'inv', source_component: 'g8ed' });

            const key = await serviceWithHttp.generateRawKey(API_KEY_PREFIX, context);

            expect(key).toBe(mockKey);
            expect(mockInternalHttpClient.generateApiKey).toHaveBeenCalledWith(API_KEY_PREFIX, context);
        });

        it('throws error when internalHttpClient is not provided', async () => {
            const context = G8eHttpContext.parse({ user_id: 'u1', organization_id: 'o1', case_id: 'test', investigation_id: 'inv', source_component: 'g8ed' });
            await expect(service.generateRawKey(API_KEY_PREFIX, context)).rejects.toThrow('InternalHttpClient is required for key generation - g8ee must be reachable');
        });

        it('throws error when generateApiKey returns failure', async () => {
            const mockInternalHttpClient = {
                generateApiKey: vi.fn().mockResolvedValue({ success: false, error: 'g8ee unavailable' })
            };
            const serviceWithHttp = new ApiKeyService({ apiKeyDataService, internalHttpClient: mockInternalHttpClient });
            const context = G8eHttpContext.parse({ user_id: 'u1', organization_id: 'o1', case_id: 'test', investigation_id: 'inv', source_component: 'g8ed' });

            await expect(serviceWithHttp.generateRawKey(API_KEY_PREFIX, context)).rejects.toThrow('g8ee unreachable');
        });

        it('throws error when generateApiKey throws', async () => {
            const mockInternalHttpClient = {
                generateApiKey: vi.fn().mockRejectedValue(new Error('Network error'))
            };
            const serviceWithHttp = new ApiKeyService({ apiKeyDataService, internalHttpClient: mockInternalHttpClient });
            const context = G8eHttpContext.parse({ user_id: 'u1', organization_id: 'o1', case_id: 'test', investigation_id: 'inv', source_component: 'g8ed' });

            await expect(serviceWithHttp.generateRawKey(API_KEY_PREFIX, context)).rejects.toThrow('g8ee unreachable');
        });
    });

    describe('validateKey', () => {
        it('returns success and data for valid active key', async () => {
            const key = `${API_KEY_PREFIX}valid`;
            const doc = ApiKeyDocument.parse({
                user_id: 'u1',
                organization_id: 'o1',
                client_name: 'test',
                permissions: [],
                status: ApiKeyStatus.ACTIVE
            });
            apiKeyDataService.getKey.mockResolvedValue(doc);

            const result = await service.validateKey(key);
            expect(result.success).toBe(true);
            expect(result.data).toBeInstanceOf(ApiKeyDocument);
            expect(result.data.user_id).toBe('u1');
        });

        it('returns error if key is missing', async () => {
            const result = await service.validateKey(null);
            expect(result.success).toBe(false);
            expect(result.error).toBe('API key is required');
        });

        it('returns error if key format is invalid', async () => {
            const result = await service.validateKey('wrong_prefix_123');
            expect(result.success).toBe(false);
            expect(result.error).toBe(ApiKeyError.INVALID_KEY_FORMAT);
        });

        it('returns error if key not found', async () => {
            const key = `${API_KEY_PREFIX}missing`;
            apiKeyDataService.getKey.mockResolvedValue(null);

            const result = await service.validateKey(key);
            expect(result.success).toBe(false);
            expect(result.error).toBe('API key not found');
        });

        it('returns error if key is not active', async () => {
            const key = `${API_KEY_PREFIX}inactive`;
            const doc = ApiKeyDocument.parse({
                user_id: 'u1',
                organization_id: 'o1',
                client_name: 'test',
                permissions: [],
                status: ApiKeyStatus.REVOKED
            });
            apiKeyDataService.getKey.mockResolvedValue(doc);

            const result = await service.validateKey(key);
            expect(result.success).toBe(false);
            expect(result.error).toBe(`API key is ${ApiKeyStatus.REVOKED}`);
        });

        it('returns error if key is expired', async () => {
            const key = `${API_KEY_PREFIX}expired`;
            const past = new Date();
            past.setFullYear(past.getFullYear() - 1);
            
            const doc = ApiKeyDocument.parse({
                user_id: 'u1',
                organization_id: 'o1',
                client_name: 'test',
                permissions: [],
                status: ApiKeyStatus.ACTIVE,
                expires_at: past
            });
            apiKeyDataService.getKey.mockResolvedValue(doc);

            const result = await service.validateKey(key);
            expect(result.success).toBe(false);
            expect(result.error).toBe('API key has expired');
        });

        it('returns internal error on data service failure', async () => {
            apiKeyDataService.getKey.mockRejectedValue(new Error('DB Fail'));
            const result = await service.validateKey(`${API_KEY_PREFIX}any`);
            expect(result.success).toBe(false);
            expect(result.error).toBe('Internal validation error');
        });
    });

    describe('issueKey', () => {
        it('creates a new key document', async () => {
            const key = 'new_key';
            const params = { user_id: 'u1', organization_id: 'o1', client_name: 'test', permissions: [] };
            
            const result = await service.issueKey(key, params);
            
            expect(result.success).toBe(true);
            expect(apiKeyDataService.createKey).toHaveBeenCalledWith('doc123', expect.any(ApiKeyDocument));
        });

        it('returns error message on failure', async () => {
            apiKeyDataService.createKey.mockRejectedValue(new Error('Write Fail'));
            const result = await service.issueKey('k', { user_id: 'u1', client_name: 'test' });
            expect(result.success).toBe(false);
            expect(result.error).toBe('Write Fail');
        });
    });

    describe('recordUsage', () => {
        it('updates last_used_at timestamp', async () => {
            await service.recordUsage('key123');
            expect(apiKeyDataService.updateKey).toHaveBeenCalledWith('doc123', expect.objectContaining({
                last_used_at: expect.any(Date)
            }));
        });

        it('does not throw on failure (logged only)', async () => {
            apiKeyDataService.updateKey.mockRejectedValue(new Error('Fail'));
            await expect(service.recordUsage('k')).resolves.not.toThrow();
        });
    });

    describe('revokeKey', () => {
        it('deletes the key document', async () => {
            const result = await service.revokeKey('key123');
            expect(result.success).toBe(true);
            expect(apiKeyDataService.deleteKey).toHaveBeenCalledWith('doc123');
        });

        it('returns success: false on failure', async () => {
            apiKeyDataService.deleteKey.mockRejectedValue(new Error('Fail'));
            const result = await service.revokeKey('k');
            expect(result.success).toBe(false);
        });
    });
});
