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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { VSODBHttpClient, VSODBHttpError } from '@vsod/services/clients/vsodb_http_client.js';
import { VSODB_HTTP_TIMEOUT_MS } from '@vsod/constants/http_client.js';
import { HTTP_INTERNAL_AUTH_HEADER, HTTP_CONTENT_TYPE_HEADER } from '@vsod/constants/headers.js';
import { logger } from '@vsod/utils/logger.js';

vi.mock('@vsod/utils/logger.js', () => ({
    logger: { info: vi.fn(), warn: vi.fn(), error: vi.fn(), debug: vi.fn() }
}));

describe('VSODBHttpClient', () => {
    const listenUrl = 'https://vsodb:9000';
    const internalAuthToken = 'test-token';
    let client;

    beforeEach(() => {
        vi.stubGlobal('fetch', vi.fn());
        vi.useFakeTimers();
        client = new VSODBHttpClient({ listenUrl, internalAuthToken });
    });

    afterEach(() => {
        vi.unstubAllGlobals();
        vi.useRealTimers();
    });

    describe('constructor', () => {
        it('should throw if listenUrl is missing', () => {
            expect(() => new VSODBHttpClient({})).toThrow('VSODBHttpClient: listenUrl is required');
        });

        it('should strip trailing slash from listenUrl', () => {
            const clientWithSlash = new VSODBHttpClient({ listenUrl: 'https://vsodb:9000/' });
            expect(clientWithSlash.listenUrl).toBe('https://vsodb:9000');
        });

        it('should set default component name', () => {
            expect(client.component).toBe('VSODB-HTTP');
        });
    });

    describe('request', () => {
        beforeEach(() => {
            vi.mocked(logger.info).mockClear();
            vi.mocked(logger.error).mockClear();
        });

        it('should make a successful request', async () => {
            const mockResponse = { success: true };
            vi.mocked(fetch).mockResolvedValueOnce({
                ok: true,
                status: 200,
                text: async () => JSON.stringify(mockResponse),
            });

            const result = await client.request('GET', '/test');

            expect(result).toEqual(mockResponse);
            expect(fetch).toHaveBeenCalledWith(`${listenUrl}/test`, expect.objectContaining({
                method: 'GET',
                headers: {
                    [HTTP_CONTENT_TYPE_HEADER]: 'application/json',
                    [HTTP_INTERNAL_AUTH_HEADER]: internalAuthToken,
                },
            }));
        });

        it('should throw VSODBHttpError on non-JSON response', async () => {
            vi.mocked(fetch).mockResolvedValueOnce({
                ok: true,
                status: 200,
                text: async () => 'not json',
            });

            await expect(client.request('GET', '/test'))
                .rejects.toThrow(VSODBHttpError);
        });

        it('should throw VSODBHttpError on non-ok response', async () => {
            const errorResponse = { error: 'Not Found' };
            vi.mocked(fetch).mockResolvedValueOnce({
                ok: false,
                status: 404,
                text: async () => JSON.stringify(errorResponse),
            });

            await expect(client.request('GET', '/test'))
                .rejects.toThrow('Not Found');
        });

        it('should timeout if request takes too long', async () => {
            vi.mocked(fetch).mockImplementationOnce(async (url, options) => {
                return new Promise((resolve, reject) => {
                    const timeout = setTimeout(() => {
                        resolve({
                            ok: true,
                            status: 200,
                            text: async () => JSON.stringify({ success: true }),
                        });
                    }, VSODB_HTTP_TIMEOUT_MS * 2);

                    options.signal.addEventListener('abort', () => {
                        clearTimeout(timeout);
                        const error = new Error('The operation was aborted');
                        error.name = 'AbortError';
                        reject(error);
                    });
                });
            });

            const requestPromise = client.request('GET', '/test');
            
            vi.advanceTimersByTime(VSODB_HTTP_TIMEOUT_MS + 100);

            await expect(requestPromise).rejects.toThrow(/timeout/);
        });

        it('should throw error if client is terminated', async () => {
            client.terminate();
            await expect(client.request('GET', '/test')).rejects.toThrow('Client terminated');
        });

        it('should log 404 at info level only, not error', async () => {
            vi.mocked(fetch).mockResolvedValueOnce({
                ok: false,
                status: 404,
                text: async () => JSON.stringify({ error: 'key not found' }),
            });

            await expect(client.request('GET', '/kv/some-key')).rejects.toThrow(VSODBHttpError);

            expect(logger.info).toHaveBeenCalledTimes(1);
            expect(logger.info).toHaveBeenCalledWith(expect.stringContaining('key not found'));
            expect(logger.error).not.toHaveBeenCalled();
        });

        it('should log non-404 HTTP errors at error level only once', async () => {
            vi.mocked(fetch).mockResolvedValueOnce({
                ok: false,
                status: 500,
                text: async () => JSON.stringify({ error: 'internal error' }),
            });

            await expect(client.request('GET', '/kv/some-key')).rejects.toThrow(VSODBHttpError);

            expect(logger.error).toHaveBeenCalledTimes(1);
            expect(logger.error).toHaveBeenCalledWith(expect.stringContaining('internal error'));
        });

        it('should log network failures once at error level', async () => {
            vi.mocked(fetch).mockRejectedValueOnce(new TypeError('fetch failed'));

            await expect(client.request('GET', '/kv/some-key')).rejects.toThrow(TypeError);

            expect(logger.error).toHaveBeenCalledTimes(1);
        });

        it('should not double-log non-JSON responses', async () => {
            vi.mocked(fetch).mockResolvedValueOnce({
                ok: true,
                status: 200,
                text: async () => 'not json',
            });

            await expect(client.request('GET', '/test')).rejects.toThrow(VSODBHttpError);

            expect(logger.error).not.toHaveBeenCalled();
        });
    });

    describe('convenience methods', () => {
        beforeEach(() => {
            vi.mocked(fetch).mockResolvedValue({
                ok: true,
                status: 200,
                text: async () => JSON.stringify({ success: true }),
            });
        });

        it('get() should make GET request', async () => {
            await client.get('/test');
            expect(fetch).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ method: 'GET' }));
        });

        it('put() should make PUT request with body', async () => {
            const body = JSON.stringify({ foo: 'bar' });
            await client.put('/test', body);
            expect(fetch).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ method: 'PUT', body }));
        });

        it('put() should throw if body is not a string', async () => {
            await expect(client.put('/test', { foo: 'bar' })).rejects.toThrow(/body must be a pre-serialized JSON string/);
        });

        it('patch() should make PATCH request with body', async () => {
            const body = JSON.stringify({ foo: 'bar' });
            await client.patch('/test', body);
            expect(fetch).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ method: 'PATCH', body }));
        });

        it('post() should make POST request with body', async () => {
            const body = JSON.stringify({ foo: 'bar' });
            await client.post('/test', body);
            expect(fetch).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ method: 'POST', body }));
        });

        it('delete() should make DELETE request', async () => {
            await client.delete('/test');
            expect(fetch).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ method: 'DELETE' }));
        });
    });

    describe('lifecycle', () => {
        it('should report termination status correctly', () => {
            expect(client.isTerminated()).toBe(false);
            client.terminate();
            expect(client.isTerminated()).toBe(true);
        });
    });
});
