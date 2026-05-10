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
import { g8esBlobClient } from '@g8ed/services/platform/g8es_blob_client.js';

vi.mock('@g8ed/utils/logger.js', () => ({
    logger: { info: vi.fn(), warn: vi.fn(), error: vi.fn(), debug: vi.fn() }
}));

const BASE_URL = 'https://g8es:9000';

function makeClient() {
    return new g8esBlobClient({ baseUrl: BASE_URL });
}

function mockFetchOk(body = {}, contentType = 'application/json') {
    return vi.fn().mockResolvedValue({
        ok:          true,
        status:      200,
        json:        async () => body,
        arrayBuffer: async () => Buffer.from(JSON.stringify(body)).buffer,
        text:        async () => JSON.stringify(body),
        headers:     { get: () => contentType },
    });
}

function mockFetchBinary(buf) {
    return vi.fn().mockResolvedValue({
        ok:          true,
        status:      200,
        arrayBuffer: async () => buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength),
        text:        async () => '',
        headers:     { get: () => 'image/png' },
    });
}

function mockFetchError(status, body = 'error') {
    return vi.fn().mockResolvedValue({
        ok:     false,
        status,
        text:   async () => body,
        json:   async () => ({ error: body }),
    });
}

describe('G8esBlobClient [UNIT]', () => {
    let client;

    beforeEach(() => {
        vi.clearAllMocks();
        client = makeClient();
    });

    // --- namespace / objectKey ---

    describe('namespace', () => {
        it('should prefix with att:', () => {
            expect(client.namespace('inv-001')).toBe('att:inv-001');
        });
    });

    describe('objectKey', () => {
        it('should build att:{invId}/{attId}', () => {
            expect(client.objectKey('inv-001', 'att-abc')).toBe('att:inv-001/att-abc');
        });
    });

    // --- putAttachment ---

    describe('putAttachment', () => {
        it('should PUT raw bytes to /blob/{ns}/{id} and return the object key', async () => {
            global.fetch = mockFetchOk({ status: 'ok' });
            const base64 = 'SGVsbG8gV29ybGQ=';

            const key = await client.putAttachment('inv-001', 'att-abc', base64, 'text/plain');

            expect(key).toBe('att:inv-001/att-abc');
            expect(global.fetch).toHaveBeenCalledOnce();
            const [url, opts] = global.fetch.mock.calls[0];
            expect(url).toContain('/blob/att%3Ainv-001/att-abc');
            expect(opts.method).toBe('PUT');
            expect(opts.headers['Content-Type']).toBe('text/plain');
            expect(Buffer.isBuffer(opts.body)).toBe(true);
            expect(opts.body.toString('base64')).toBe(base64);
        });

        it('should throw when base64Data is not a string', async () => {
            await expect(client.putAttachment('inv', 'att', 123, 'text/plain'))
                .rejects.toThrow('base64Data must be a string');
        });

        it('should throw on non-ok response', async () => {
            global.fetch = mockFetchError(500, 'internal error');
            await expect(client.putAttachment('inv', 'att', 'SGVs', 'text/plain'))
                .rejects.toThrow('500');
        });
    });

    // --- getAttachment ---

    describe('getAttachment', () => {
        it('should GET and return base64-encoded content', async () => {
            const content = Buffer.from('Hello World');
            global.fetch = mockFetchBinary(content);

            const result = await client.getAttachment('att:inv-001/att-abc');
            expect(result).toBe(content.toString('base64'));
            const [url] = global.fetch.mock.calls[0];
            expect(url).toContain('/blob/att%3Ainv-001/att-abc');
        });

        it('should throw not found on 404', async () => {
            global.fetch = mockFetchError(404, 'not found');
            await expect(client.getAttachment('att:inv-001/missing'))
                .rejects.toThrow('not found');
        });

        it('should throw on non-ok non-404 response', async () => {
            global.fetch = mockFetchError(503, 'service unavailable');
            await expect(client.getAttachment('att:inv-001/att-abc'))
                .rejects.toThrow('503');
        });

        it('should throw on malformed object key (no slash)', async () => {
            await expect(client.getAttachment('noseparator'))
                .rejects.toThrow('malformed object key');
        });
    });

    // --- deleteAttachment ---

    describe('deleteAttachment', () => {
        it('should DELETE /blob/{ns}/{id}', async () => {
            global.fetch = mockFetchOk({ deleted: 1 });
            await client.deleteAttachment('att:inv-001/att-abc');

            const [url, opts] = global.fetch.mock.calls[0];
            expect(url).toContain('/blob/att%3Ainv-001/att-abc');
            expect(opts.method).toBe('DELETE');
        });

        it('should not throw on 404', async () => {
            global.fetch = mockFetchError(404);
            await expect(client.deleteAttachment('att:inv-001/att-abc')).resolves.toBeUndefined();
        });

        it('should throw on non-ok non-404 response', async () => {
            global.fetch = mockFetchError(500);
            await expect(client.deleteAttachment('att:inv-001/att-abc')).rejects.toThrow('500');
        });
    });

    // --- deleteAttachmentsForInvestigation ---

    describe('deleteAttachmentsForInvestigation', () => {
        it('should DELETE /blob/{ns} and return deleted count', async () => {
            global.fetch = mockFetchOk({ deleted: 3 });
            const count = await client.deleteAttachmentsForInvestigation('inv-001');

            expect(count).toBe(3);
            const [url, opts] = global.fetch.mock.calls[0];
            expect(url).toContain('/blob/att%3Ainv-001');
            expect(opts.method).toBe('DELETE');
        });

        it('should return 0 when response deleted is not a number', async () => {
            global.fetch = mockFetchOk({ deleted: null });
            const count = await client.deleteAttachmentsForInvestigation('inv-001');
            expect(count).toBe(0);
        });

        it('should throw on non-ok response', async () => {
            global.fetch = mockFetchError(500);
            await expect(client.deleteAttachmentsForInvestigation('inv-001')).rejects.toThrow('500');
        });
    });

    // --- _parseObjectKey ---

    describe('_parseObjectKey', () => {
        it('should parse att:{invId}/{attId}', () => {
            const { ns, id } = client._parseObjectKey('att:inv-001/att-abc');
            expect(ns).toBe('att:inv-001');
            expect(id).toBe('att-abc');
        });

        it('should throw on key with no slash', () => {
            expect(() => client._parseObjectKey('noseparator'))
                .toThrow('malformed object key');
        });
    });
});
