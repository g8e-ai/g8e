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

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { CacheAsideService } from '@vsod/services/cache/cache_aside_service.js';
import { Collections } from '@vsod/constants/collections.js';
import { SourceComponent } from '@vsod/constants/ai.js';
import { VSOBaseModel, F } from '@vsod/models/base.js';
import { KVOperationError } from '@vsod/services/clients/vsodb_kv_cache_client.js';

// Minimal model for testing
class TestModel extends VSOBaseModel {
    static fields = {
        id: { type: F.string, required: true },
        name: { type: F.string, required: true }
    };
}

describe('CacheAsideService', () => {
    let service;
    let mockListenClient;
    let mockDbClient;

    beforeEach(() => {
        mockListenClient = {
            get_json: vi.fn(),
            set_json: vi.fn(),
            del: vi.fn(),
            keys: vi.fn(),
            exists: vi.fn(),
            ttl: vi.fn(),
            expire: vi.fn()
        };
        mockDbClient = {
            getDocument: vi.fn(),
            setDocument: vi.fn(),
            updateDocument: vi.fn(),
            deleteDocument: vi.fn(),
            queryDocuments: vi.fn()
        };
        service = new CacheAsideService(mockListenClient, mockDbClient, SourceComponent.VSOD);
    });

    describe('createDocument', () => {
        it('should write to DB then warm the cache', async () => {
            const docId = 'doc-1';
            const data = { id: docId, name: 'Test' };
            mockDbClient.setDocument.mockResolvedValue({ success: true });
            mockListenClient.keys.mockResolvedValue([]); // For query cache invalidation

            const result = await service.createDocument(Collections.USERS, docId, data);

            expect(result.success).toBe(true);
            expect(mockDbClient.setDocument).toHaveBeenCalledWith(Collections.USERS, docId, data);
            expect(mockListenClient.set_json).toHaveBeenCalledWith(expect.any(String), data, expect.any(Number));
        });

        it('should handle DB failure and not warm cache', async () => {
            mockDbClient.setDocument.mockResolvedValue({ success: false, error: 'DB Error' });
            
            const result = await service.createDocument(Collections.USERS, 'id', {});

            expect(result.success).toBe(false);
            expect(mockListenClient.set_json).not.toHaveBeenCalled();
        });
    });

    describe('getDocument', () => {
        it('should return from cache on HIT', async () => {
            const cachedData = { id: '1', name: 'Cached' };
            mockListenClient.get_json.mockResolvedValue(cachedData);

            const result = await service.getDocument(Collections.USERS, '1');

            expect(result).toEqual(cachedData);
            expect(mockDbClient.getDocument).not.toHaveBeenCalled();
        });

        it('should read from DB and warm cache on MISS', async () => {
            const dbData = { id: '1', name: 'DB' };
            mockListenClient.get_json.mockResolvedValue(null);
            mockDbClient.getDocument.mockResolvedValue({ success: true, data: dbData });

            const result = await service.getDocument(Collections.USERS, '1');

            expect(result).toEqual(dbData);
            expect(mockListenClient.set_json).toHaveBeenCalledWith(expect.any(String), dbData, expect.any(Number));
        });

        it('should return null if not in DB', async () => {
            mockListenClient.get_json.mockResolvedValue(null);
            mockDbClient.getDocument.mockResolvedValue({ success: true, data: null });

            const result = await service.getDocument(Collections.USERS, '1');

            expect(result).toBeNull();
        });
    });

    describe('updateDocument', () => {
        it('should update DB and invalidate cache', async () => {
            mockDbClient.updateDocument.mockResolvedValue({ success: true });
            mockListenClient.keys.mockResolvedValue([]);

            const result = await service.updateDocument(Collections.USERS, '1', { name: 'New' });

            expect(result.success).toBe(true);
            expect(mockListenClient.del).toHaveBeenCalled();
        });
    });

    describe('deleteDocument', () => {
        it('should delete from DB and invalidate cache', async () => {
            mockDbClient.deleteDocument.mockResolvedValue({ success: true });
            mockListenClient.keys.mockResolvedValue([]);

            const result = await service.deleteDocument(Collections.USERS, '1');

            expect(result.success).toBe(true);
            expect(mockListenClient.del).toHaveBeenCalled();
        });
    });

    describe('_invalidateQueryCache', () => {
        it('should delete all matching query keys on success', async () => {
            mockListenClient.keys.mockResolvedValue(['query:k1', 'query:k2']);
            mockDbClient.setDocument.mockResolvedValue({ success: true });

            await service.createDocument(Collections.USERS, 'doc-1', { id: 'doc-1', name: 'Test' });

            expect(mockListenClient.keys).toHaveBeenCalledWith(expect.stringContaining('query'));
            expect(mockListenClient.del).toHaveBeenCalledWith('query:k1');
            expect(mockListenClient.del).toHaveBeenCalledWith('query:k2');
        });

        it('should not propagate KVOperationError from keys() — write still succeeds', async () => {
            mockListenClient.keys.mockRejectedValue(
                new KVOperationError('keys', 'g8e:cache:query:users:*', new Error('connection refused'))
            );
            mockDbClient.setDocument.mockResolvedValue({ success: true });

            const result = await service.createDocument(Collections.USERS, 'doc-1', { id: 'doc-1', name: 'Test' });

            expect(result.success).toBe(true);
        });

        it('should not propagate KVOperationError from keys() on updateDocument', async () => {
            mockListenClient.keys.mockRejectedValue(
                new KVOperationError('keys', 'g8e:cache:query:users:*', new Error('timeout'))
            );
            mockDbClient.updateDocument.mockResolvedValue({ success: true });

            const result = await service.updateDocument(Collections.USERS, '1', { name: 'New' });

            expect(result.success).toBe(true);
        });
    });

    describe('queryDocuments', () => {
        it('should return from query cache on HIT', async () => {
            const cachedResults = [{ id: '1' }];
            // getQueryResult uses set_json internally for cache check
            mockListenClient.get_json.mockResolvedValue(cachedResults);

            const result = await service.queryDocuments(Collections.USERS, []);

            expect(result).toEqual(cachedResults);
            expect(mockDbClient.queryDocuments).not.toHaveBeenCalled();
        });

        it('should query DB and cache results on MISS', async () => {
            const dbResults = [{ id: '1' }];
            mockListenClient.get_json.mockResolvedValue(null);
            mockDbClient.queryDocuments.mockResolvedValue({ success: true, data: dbResults });

            const result = await service.queryDocuments(Collections.USERS, []);

            expect(result).toEqual(dbResults);
            expect(mockListenClient.set_json).toHaveBeenCalled();
        });

        it('should NOT cache empty query results', async () => {
            const dbResults = [];
            mockListenClient.get_json.mockResolvedValue(null);
            mockDbClient.queryDocuments.mockResolvedValue({ success: true, data: dbResults });

            const result = await service.queryDocuments(Collections.USERS, []);

            expect(result).toEqual(dbResults);
            expect(mockListenClient.set_json).not.toHaveBeenCalled();
        });
    });
});
