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
import { InternalHttpClient } from '@vsod/services/clients/internal_http_client.js';
import { VSOHttpContext } from '@vsod/models/request_models.js';
import { VSOHeaders, HTTP_INTERNAL_AUTH_HEADER } from '@vsod/constants/headers.js';
import { SourceComponent } from '@vsod/constants/ai.js';
import {
    INTERNAL_HTTP_TIMEOUT_MS,
    VSE_INTERNAL_URL,
    NEW_CASE_ID
} from '@vsod/constants/http_client.js';
import { apiPaths, InternalApiPaths } from '@vsod/constants/api_paths.js';

function makeBootstrapService(token = null) {
    return {
        loadInternalAuthToken: vi.fn().mockReturnValue(token),
        getSslDir: vi.fn().mockReturnValue('/vsodb'),
        loadCaCertPath: vi.fn().mockReturnValue('/vsodb/ca.crt'),
    };
}

function makeSettingsService(overrides = {}) {
    return { vse_url: 'https://vse', ...overrides };
}

describe('InternalHttpClient', () => {
    let bootstrapService;
    let settingsService;
    let client;

    beforeEach(() => {
        vi.stubGlobal('fetch', vi.fn());
        vi.useFakeTimers();

        bootstrapService = makeBootstrapService('test-token');
        settingsService = makeSettingsService();
        client = new InternalHttpClient({ bootstrapService, settingsService });
    });

    afterEach(() => {
        vi.unstubAllGlobals();
        vi.useRealTimers();
        vi.clearAllMocks();
    });

    describe('constructor and resolution', () => {
        it('throws if bootstrapService is missing', () => {
            expect(() => new InternalHttpClient({ settingsService }))
                .toThrow('InternalHttpClient requires bootstrapService');
        });

        it('should resolve token from bootstrapService', () => {
            expect(client._resolveInternalAuthToken()).toBe('test-token');
            expect(bootstrapService.loadInternalAuthToken).toHaveBeenCalled();
        });

        it('should resolve service URL from settingsService or default', () => {
            expect(client._resolveServiceUrl('vse')).toBe('https://vse');

            const noSettingsClient = new InternalHttpClient({ bootstrapService });
            expect(noSettingsClient._resolveServiceUrl('vse')).toBe('https://vse');
        });
    });

    describe('buildVSOContextHeaders', () => {
        it('should throw if context is not VSOHttpContext', () => {
            expect(() => client.buildVSOContextHeaders({})).toThrow(/requires VSOHttpContext instance/);
        });

        it('should build headers for an existing case', () => {
            const context = new VSOHttpContext({
                web_session_id: 'ws-123',
                user_id: 'u-123',
                organization_id: 'o-123',
                case_id: 'c-123',
                investigation_id: 'i-123',
                execution_id: 'req-123'
            });

            const headers = client.buildVSOContextHeaders(context);

            expect(headers[VSOHeaders.WEB_SESSION_ID]).toBe('ws-123');
            expect(headers[VSOHeaders.USER_ID]).toBe('u-123');
            expect(headers[VSOHeaders.ORGANIZATION_ID]).toBe('o-123');
            expect(headers[VSOHeaders.CASE_ID]).toBe('c-123');
            expect(headers[VSOHeaders.INVESTIGATION_ID]).toBe('i-123');
            expect(headers[VSOHeaders.EXECUTION_ID]).toBe('req-123');
            expect(headers[VSOHeaders.SOURCE_COMPONENT]).toBe(SourceComponent.VSOD);
        });

        it('should set new case signals if case_id is missing', () => {
            const context = new VSOHttpContext({
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const headers = client.buildVSOContextHeaders(context);

            expect(headers[VSOHeaders.NEW_CASE]).toBe('true');
            expect(headers[VSOHeaders.CASE_ID]).toBe(NEW_CASE_ID);
            expect(headers[VSOHeaders.INVESTIGATION_ID]).toBe(NEW_CASE_ID);
        });

        it('should serialize bound operators', () => {
            const operators = [{ operator_id: 'op-1', operator_session_id: 'os-1' }];
            const context = new VSOHttpContext({
                web_session_id: 'ws-123',
                user_id: 'u-123',
                bound_operators: operators
            });

            const headers = client.buildVSOContextHeaders(context);
            expect(headers[VSOHeaders.BOUND_OPERATORS]).toBe(JSON.stringify(operators));
        });
    });

    describe('request', () => {
        it('should make a successful request with context headers', async () => {
            const mockData = { success: true, data: { id: '1' } };

            vi.mocked(fetch).mockResolvedValueOnce({
                ok: true,
                status: 200,
                json: async () => mockData,
            });

            const context = new VSOHttpContext({ web_session_id: 'ws-123', user_id: 'u-123' });
            const result = await client.request('vse', '/test', { vsoContext: context, method: 'POST', body: { foo: 'bar' } });

            expect(result).toEqual(mockData);
            expect(fetch).toHaveBeenCalledWith(expect.stringContaining('/test'), expect.objectContaining({
                method: 'POST',
                body: JSON.stringify({ foo: 'bar' }),
                headers: expect.objectContaining({
                    [HTTP_INTERNAL_AUTH_HEADER]: 'test-token',
                    [VSOHeaders.WEB_SESSION_ID]: 'ws-123'
                })
            }));
        });

        it('should handle 204 No Content', async () => {
            vi.mocked(fetch).mockResolvedValueOnce({
                ok: true,
                status: 204,
            });

            const result = await client.request('vse', '/test', { method: 'DELETE' });
            expect(result).toEqual({ success: true });
        });

        it('should throw on HTTP error', async () => {
            vi.mocked(fetch).mockResolvedValueOnce({
                ok: false,
                status: 500,
                text: async () => 'Internal Server Error',
            });

            await expect(client.request('vse', '/test')).rejects.toThrow('HTTP 500: Internal Server Error');
        });

        it('should timeout if request takes too long', async () => {
            vi.mocked(fetch).mockImplementationOnce(async (url, options) => {
                return new Promise((resolve, reject) => {
                    const timeout = setTimeout(() => {
                        resolve({ ok: true, status: 200, json: async () => ({}) });
                    }, INTERNAL_HTTP_TIMEOUT_MS * 2);

                    options.signal.addEventListener('abort', () => {
                        clearTimeout(timeout);
                        const error = new Error('The operation was aborted');
                        error.name = 'AbortError';
                        reject(error);
                    });
                });
            });

            const promise = client.request('vse', '/test');
            vi.advanceTimersByTime(INTERNAL_HTTP_TIMEOUT_MS + 100);

            await expect(promise).rejects.toThrow(/timeout/);
        });
    });

    describe('VSE endpoint methods', () => {
        const context = new VSOHttpContext({ 
            web_session_id: 'ws-123', 
            user_id: 'u-123' 
        });

        beforeEach(() => {
            vi.mocked(fetch).mockResolvedValue({
                ok: true,
                status: 200,
                json: async () => ({ success: true }),
            });
        });

        it('sendChatMessage should call VSE chat endpoint', async () => {
            await client.sendChatMessage({ message: 'hi' }, context);
            const path = apiPaths.vse.chat();
            expect(path).toBe('/api/internal/chat');
            expect(fetch).toHaveBeenCalledWith(expect.stringContaining(path), expect.objectContaining({
                method: 'POST',
                body: JSON.stringify({ message: 'hi' })
            }));
        });

        it('queryInvestigations should include query params', async () => {
            const params = new URLSearchParams({ status: 'active' });
            await client.queryInvestigations(params, context);
            const path = apiPaths.vse.investigations();
            expect(path).toBe('/api/internal/investigations');
            expect(fetch).toHaveBeenCalledWith(expect.stringContaining(path + '?status=active'), expect.any(Object));
        });

        it('getInvestigation should call specific investigation endpoint', async () => {
            await client.getInvestigation('inv-123', context);
            const path = apiPaths.vse.investigation('inv-123');
            expect(path).toBe('/api/internal/investigations/inv-123');
            expect(fetch).toHaveBeenCalledWith(expect.stringContaining(path), expect.any(Object));
        });

        it('deleteCase should call DELETE on case endpoint', async () => {
            await client.deleteCase('case-123', context);
            const path = apiPaths.vse.case('case-123');
            expect(path).toBe('/api/internal/cases/case-123');
            expect(fetch).toHaveBeenCalledWith(expect.stringContaining(path), expect.objectContaining({
                method: 'DELETE'
            }));
        });

        it('stopAIProcessing should call stop endpoint', async () => {
            await client.stopAIProcessing({ investigation_id: 'inv-123', reason: 'cancel', web_session_id: 'ws-123' }, context);
            const path = apiPaths.vse.chatStop();
            expect(path).toBe('/api/internal/chat/stop');
            expect(fetch).toHaveBeenCalledWith(expect.stringContaining(path), expect.objectContaining({
                method: 'POST',
                body: expect.stringContaining('inv-123')
            }));
        });

        it('healthCheck should check VSE health', async () => {
            vi.spyOn(client, 'request').mockResolvedValue({ success: true });
            const results = await client.healthCheck();
            expect(results.vse.status).toBe('healthy');
            expect(client.request).toHaveBeenCalledWith('vse', apiPaths.vse.health(), expect.any(Object));
        });
        
    });
});
