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

import { describe, it, expect, beforeEach } from 'vitest';
import { ApiKeyDataService } from '@vsod/services/auth/api_key_data_service.js';
import { Collections } from '@vsod/constants/collections.js';
import { ApiKeyDocument } from '@vsod/models/auth_models.js';
import { createMockCacheAside } from '@test/mocks/cache-aside.mock.js';

describe('ApiKeyDataService [UNIT]', () => {
    let cacheAside;
    let service;

    beforeEach(() => {
        cacheAside = createMockCacheAside();
        service = new ApiKeyDataService({ cacheAsideService: cacheAside });
    });

    describe('constructor', () => {
        it('throws if cacheAsideService is missing', () => {
            expect(() => new ApiKeyDataService({})).toThrow('ApiKeyDataService requires cacheAsideService');
        });
    });

    describe('makeDocId', () => {
        it('generates a deterministic 64-character hex hash', () => {
            const key = 'g8e_test_key_123';
            const id1 = service.makeDocId(key);
            const id2 = service.makeDocId(key);
            
            expect(id1).toBe(id2);
            expect(id1).toMatch(/^[a-f0-9]{64}$/);
        });
    });

    describe('createKey', () => {
        it('calls cacheAside.createDocument', async () => {
            const doc = { user_id: 'u1' };
            await service.createKey('doc1', doc);
            expect(cacheAside.createDocument).toHaveBeenCalledWith(Collections.API_KEYS, 'doc1', doc);
            
            // Proves it actually hit the mock store
            const stored = await cacheAside.getDocument(Collections.API_KEYS, 'doc1');
            expect(stored.user_id).toBe('u1');
        });
    });

    describe('getKey', () => {
        it('returns parsed ApiKeyDocument', async () => {
            const raw = { user_id: 'u1', client_name: 'test', permissions: [] };
            cacheAside._seedDoc(Collections.API_KEYS, 'doc1', raw);
            
            const result = await service.getKey('doc1');
            expect(result).toBeInstanceOf(ApiKeyDocument);
            expect(result.user_id).toBe('u1');
        });

        it('returns null if not found', async () => {
            const result = await service.getKey('doc1');
            expect(result).toBeNull();
        });
    });

    describe('updateKey', () => {
        it('calls cacheAside.updateDocument', async () => {
            cacheAside._seedDoc(Collections.API_KEYS, 'doc1', { user_id: 'u1', status: 'active' });
            
            const updates = { status: 'revoked' };
            await service.updateKey('doc1', updates);
            expect(cacheAside.updateDocument).toHaveBeenCalledWith(Collections.API_KEYS, 'doc1', updates);
            
            const updated = await cacheAside.getDocument(Collections.API_KEYS, 'doc1');
            expect(updated.status).toBe('revoked');
        });
    });

    describe('deleteKey', () => {
        it('calls cacheAside.deleteDocument', async () => {
            cacheAside._seedDoc(Collections.API_KEYS, 'doc1', { user_id: 'u1' });
            
            await service.deleteKey('doc1');
            expect(cacheAside.deleteDocument).toHaveBeenCalledWith(Collections.API_KEYS, 'doc1');
            
            const doc = await cacheAside.getDocument(Collections.API_KEYS, 'doc1');
            expect(doc).toBeNull();
        });

        it('does not throw if not found', async () => {
            await expect(service.deleteKey('doc1')).resolves.not.toThrow();
        });
    });
});
