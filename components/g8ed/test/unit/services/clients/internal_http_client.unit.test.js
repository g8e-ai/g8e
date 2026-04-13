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
import { InternalHttpClient } from '@g8ed/services/clients/internal_http_client.js';
import { G8eHttpContext } from '@g8ed/models/request_models.js';
import { G8eHeaders, HTTP_INTERNAL_AUTH_HEADER } from '@g8ed/constants/headers.js';
import { SourceComponent } from '@g8ed/constants/ai.js';
import {
    INTERNAL_HTTP_TIMEOUT_MS,
    G8EE_INTERNAL_URL,
    NEW_CASE_ID
} from '@g8ed/constants/http_client.js';
import { apiPaths, InternalApiPaths } from '@g8ed/constants/api_paths.js';

function makeBootstrapService(token = null) {
    return {
        loadInternalAuthToken: vi.fn().mockReturnValue(token),
        getSslDir: vi.fn().mockReturnValue('/g8es'),
        loadCaCertPath: vi.fn().mockReturnValue('/g8es/ca.crt'),
    };
}

function makeSettingsService(overrides = {}) {
    return { g8ee_url: 'https://g8ee', ...overrides };
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

        it('should resolve component URL from settingsService or default', () => {
            expect(client._resolveComponentUrl('g8ee')).toBe('https://g8ee');

            const noSettingsClient = new InternalHttpClient({ bootstrapService });
            expect(noSettingsClient._resolveComponentUrl('g8ee')).toBe('https://g8ee');
        });
    });

    describe('buildG8eContextHeaders', () => {
        it('should throw if context is not G8eHttpContext', () => {
            expect(() => client.buildG8eContextHeaders({})).toThrow(/requires G8eHttpContext instance/);
        });

        it('should build headers for an existing case', () => {
            const context = new G8eHttpContext({
                web_session_id: 'ws-123',
                user_id: 'u-123',
                organization_id: 'o-123',
                case_id: 'c-123',
                investigation_id: 'i-123',
                execution_id: 'req-123'
            });

            const headers = client.buildG8eContextHeaders(context);

            expect(headers[G8eHeaders.WEB_SESSION_ID]).toBe('ws-123');
            expect(headers[G8eHeaders.USER_ID]).toBe('u-123');
            expect(headers[G8eHeaders.ORGANIZATION_ID]).toBe('o-123');
            expect(headers[G8eHeaders.CASE_ID]).toBe('c-123');
            expect(headers[G8eHeaders.INVESTIGATION_ID]).toBe('i-123');
            expect(headers[G8eHeaders.EXECUTION_ID]).toBe('req-123');
            expect(headers[G8eHeaders.SOURCE_COMPONENT]).toBe(SourceComponent.G8ED);
        });

        it('should set new case signals if case_id is missing', () => {
            const context = new G8eHttpContext({
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const headers = client.buildG8eContextHeaders(context);

            expect(headers[G8eHeaders.NEW_CASE]).toBe('true');
            expect(headers[G8eHeaders.CASE_ID]).toBe(NEW_CASE_ID);
            expect(headers[G8eHeaders.INVESTIGATION_ID]).toBe(NEW_CASE_ID);
        });

        it('should serialize bound operators', () => {
            const operators = [{ operator_id: 'op-1', operator_session_id: 'os-1' }];
            const context = new G8eHttpContext({
                web_session_id: 'ws-123',
                user_id: 'u-123',
                bound_operators: operators
            });

            const headers = client.buildG8eContextHeaders(context);
            expect(headers[G8eHeaders.BOUND_OPERATORS]).toBe(JSON.stringify(operators));
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

            const context = new G8eHttpContext({ web_session_id: 'ws-123', user_id: 'u-123' });
            const result = await client.request('g8ee', '/test', { g8eContext: context, method: 'POST', body: { foo: 'bar' } });

            expect(result).toEqual(mockData);
            expect(fetch).toHaveBeenCalledWith(expect.stringContaining('/test'), expect.objectContaining({
                method: 'POST',
                body: JSON.stringify({ foo: 'bar' }),
                headers: expect.objectContaining({
                    [HTTP_INTERNAL_AUTH_HEADER]: 'test-token',
                    [G8eHeaders.WEB_SESSION_ID]: 'ws-123'
                })
            }));
        });

        it('should handle 204 No Content', async () => {
            vi.mocked(fetch).mockResolvedValueOnce({
                ok: true,
                status: 204,
            });

            const result = await client.request('g8ee', '/test', { method: 'DELETE' });
            expect(result).toEqual({ success: true });
        });

        it('should throw on HTTP error', async () => {
            vi.mocked(fetch).mockResolvedValueOnce({
                ok: false,
                status: 500,
                text: async () => 'Internal Server Error',
            });

            await expect(client.request('g8ee', '/test')).rejects.toThrow('HTTP 500: Internal Server Error');
        });
    });

    describe('g8ee endpoint methods', () => {
        const context = new G8eHttpContext({ 
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

        it('sendChatMessage should call g8ee chat endpoint', async () => {
            await client.sendChatMessage({ message: 'hi' }, context);
            const path = apiPaths.g8ee.chat();
            expect(path).toBe('/api/internal/chat');
            expect(fetch).toHaveBeenCalledWith(expect.stringContaining(path), expect.objectContaining({
                method: 'POST',
                body: JSON.stringify({ message: 'hi' })
            }));
        });

        it('deleteCase should call DELETE on case endpoint', async () => {
            await client.deleteCase('case-123', context);
            const path = apiPaths.g8ee.case('case-123');
            expect(path).toBe('/api/internal/cases/case-123');
            expect(fetch).toHaveBeenCalledWith(expect.stringContaining(path), expect.objectContaining({
                method: 'DELETE'
            }));
        });

        it('stopAIProcessing should call stop endpoint', async () => {
            await client.stopAIProcessing({ investigation_id: 'inv-123', reason: 'cancel', web_session_id: 'ws-123' }, context);
            const path = apiPaths.g8ee.chatStop();
            expect(path).toBe('/api/internal/chat/stop');
            expect(fetch).toHaveBeenCalledWith(expect.stringContaining(path), expect.objectContaining({
                method: 'POST',
                body: expect.stringContaining('inv-123')
            }));
        });

        it('healthCheck should check g8ee health', async () => {
            vi.spyOn(client, 'request').mockResolvedValue({ success: true });
            const results = await client.healthCheck();
            expect(results.g8ee.status).toBe('healthy');
            expect(client.request).toHaveBeenCalledWith('g8ee', apiPaths.g8ee.health(), expect.any(Object));
        });
        
    });
});
