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
import { G8esDocumentClient, G8esFieldValuE } from '@g8ed/services/clients/g8es_document_client.js';
import { G8esHttpError } from '@g8ed/services/clients/g8es_http_client.js';
import { G8eBaseModel, F } from '@g8ed/models/base.js';

class TestModel extends G8eBaseModel {
    static fields = {
        name: { type: F.string },
        count: { type: F.number },
    };
}

describe('G8esDocumentClient', () => {
    const listenUrl = 'https://g8es:9000';
    const internalAuthToken = 'test-token';
    let client;
    let mockHttp;

    beforeEach(() => {
        client = new G8esDocumentClient({ listenUrl, internalAuthToken });
        mockHttp = client._http;
        vi.spyOn(mockHttp, 'get');
        vi.spyOn(mockHttp, 'put');
        vi.spyOn(mockHttp, 'post');
        vi.spyOn(mockHttp, 'patch');
        vi.spyOn(mockHttp, 'delete');
    });

    describe('FieldValue', () => {
        it('should generate server timestamp', () => {
            const ts = G8esFieldValuE.serverTimestamp();
            expect(ts).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/);
        });

        it('should create increment op', () => {
            expect(G8esFieldValuE.increment(5)).toEqual({ __op: 'increment', value: 5 });
        });

        it('should create arrayUnion op', () => {
            expect(G8esFieldValuE.arrayUnion('a', 'b')).toEqual({ __op: 'arrayUnion', values: ['a', 'b'] });
            expect(G8esFieldValuE.arrayUnion(['a', 'b'])).toEqual({ __op: 'arrayUnion', values: ['a', 'b'] });
        });

        it('should create arrayRemove op', () => {
            expect(G8esFieldValuE.arrayRemove('a')).toEqual({ __op: 'arrayRemove', values: ['a'] });
        });

        it('should create delete op', () => {
            expect(G8esFieldValuE.delete()).toEqual({ __op: 'delete' });
        });
    });

    describe('Document CRUD', () => {
        it('getDocument should return data on success', async () => {
            const mockData = { id: '1', name: 'test' };
            vi.mocked(mockHttp.get).mockResolvedValueOnce(mockData);

            const result = await client.getDocument('users', '1');
            expect(result).toEqual({ success: true, data: mockData, error: null });
        });

        it('getDocument should return null on 404', async () => {
            vi.mocked(mockHttp.get).mockRejectedValueOnce(new G8esHttpError('Not Found', 404));

            const result = await client.getDocument('users', '1');
            expect(result).toEqual({ success: true, data: null, error: 'Document not found' });
        });

        it('setDocument should handle models and field values', async () => {
            vi.mocked(mockHttp.put).mockResolvedValueOnce({ success: true });
            const model = new TestModel({ name: 'test', count: G8esFieldValuE.increment(1) });

            const result = await client.setDocument('users', '1', model);
            
            expect(result.success).toBe(true);
            const putBody = JSON.parse(vi.mocked(mockHttp.put).mock.calls[0][1]);
            expect(putBody.name).toBe('test');
            expect(putBody.count).toEqual({ __op: 'increment', value: 1 });
        });

        it('queryDocuments should format filters correctly', async () => {
            vi.mocked(mockHttp.post).mockResolvedValueOnce([{ id: '1' }]);
            
            const result = await client.queryDocuments('users', [
                { field: 'name', operator: '==', value: 'test' }
            ], 10);

            expect(result.success).toBe(true);
            expect(result.data).toHaveLength(1);
            const postBody = JSON.parse(vi.mocked(mockHttp.post).mock.calls[0][1]);
            expect(postBody.filters[0]).toEqual({ field: 'name', op: '==', value: 'test' });
            expect(postBody.limit).toBe(10);
        });

        it('updateDocument should use PATCH', async () => {
            vi.mocked(mockHttp.patch).mockResolvedValueOnce({ success: true });
            
            const result = await client.updateDocument('users', '1', { name: 'new' });
            
            expect(result.success).toBe(true);
            expect(mockHttp.patch).toHaveBeenCalledWith('/db/users/1', expect.any(String));
        });

        it('deleteDocument should return success', async () => {
            vi.mocked(mockHttp.delete).mockResolvedValueOnce({ success: true });
            
            const result = await client.deleteDocument('users', '1');
            expect(result).toEqual({ success: true, notFound: false, error: null });
        });
    });

    describe('Transactions', () => {
        it('runTransaction should read-modify-write', async () => {
            const existing = { id: '1', count: 10 };
            vi.mocked(mockHttp.get).mockResolvedValueOnce(existing);
            vi.mocked(mockHttp.put).mockResolvedValueOnce({ success: true });

            const result = await client.runTransaction('users', '1', async (data) => {
                return { ...data, count: data.count + 1 };
            });

            expect(result.success).toBe(true);
            expect(result.data.count).toBe(11);
            expect(mockHttp.put).toHaveBeenCalled();
        });
    });

    describe('Lifecycle', () => {
        it('should prevent operations when terminated', async () => {
            client.terminate();
            const result = await client.getDocument('users', '1');
            expect(result.success).toBe(false);
            expect(result.error).toBe('Client terminated');
        });
    });
});
