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

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
    RATE_LIMIT_FALLBACK_MESSAGE,
    RATE_LIMIT_RESET_HEADER,
    WEB_SESSION_ID_HEADER,
    BEARER_PREFIX,
    CONTENT_TYPE_JSON,
    AUTHORIZATION_HEADER,
    COOKIE_HEADER,
    WEB_SESSION_COOKIE_KEY,
    API_KEY_HEADER,
    ComponentName,
    ComponentUrl,
    RequestTimeout,
    RetryConfig,
    RequestPath,
    HttpMethod,
    ServiceClientEvent,
    HttpStatus,
    MAX_ATTACHMENT_SIZE,
    MAX_TOTAL_ATTACHMENT_SIZE,
    MAX_ATTACHMENT_FILES,
} from '@g8ed/public/js/constants/service-client-constants.js';
import { ComponentName } from '@g8ed/public/js/models/investigation-models.js';

let ServiceClient;
let RateLimitError;

beforeEach(async () => {
    vi.resetModules();

    window.serviceClient = undefined;
    window.location = { origin: 'https://localhost' };

    vi.stubGlobal('performance', { now: vi.fn(() => 0) });

    ({ ServiceClient, RateLimitError } = await import('@g8ed/public/js/utils/service-client.js'));
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
});

function makeOkResponse(overrides = {}) {
    return {
        ok: true,
        status: 200,
        headers: { get: vi.fn(() => null) },
        ...overrides,
    };
}

function make429Response({ body = {}, retryAfterValue = null } = {}) {
    return {
        ok: false,
        status: 429,
        statusText: 'Too Many Requests',
        headers: { get: vi.fn((name) => name === RATE_LIMIT_RESET_HEADER ? retryAfterValue : null) },
        clone: () => ({ json: async () => body }),
    };
}

function makeErrorResponse(status, statusText, body = null) {
    return {
        ok: false,
        status,
        statusText,
        headers: { get: vi.fn(() => null) },
        json: async () => body,
    };
}

describe('service-client-constants contract [UNIT]', () => {
    it('ComponentName values are correct', () => {
        expect(ComponentName.G8EE).toBe('g8ee');
        expect(ComponentName.G8ED).toBe('g8ed');
    });

    it('ComponentUrl.G8EE is the expected internal URL', () => {
        expect(ComponentUrl.G8EE).toBe('https://g8ee');
    });

    it('RequestTimeout values are positive integers', () => {
        expect(RequestTimeout.AUTH_MS).toBeGreaterThan(0);
        expect(RequestTimeout.CASE_MS).toBeGreaterThan(0);
        expect(RequestTimeout.CHAT_MS).toBeGreaterThan(0);
        expect(RequestTimeout.DEFAULT_MS).toBeGreaterThan(0);
    });

    it('AUTH_MS is shorter than CASE_MS and CHAT_MS', () => {
        expect(RequestTimeout.AUTH_MS).toBeLessThan(RequestTimeout.CASE_MS);
        expect(RequestTimeout.AUTH_MS).toBeLessThan(RequestTimeout.CHAT_MS);
    });

    it('RetryConfig values are positive', () => {
        expect(RetryConfig.MAX_RETRIES).toBeGreaterThan(0);
        expect(RetryConfig.RETRY_DELAY_MS).toBeGreaterThan(0);
        expect(RetryConfig.BACKOFF_MULTIPLIER).toBeGreaterThan(1);
    });

    it('RequestPath prefixes start with /', () => {
        expect(RequestPath.AUTH_PREFIX).toMatch(/^\//);
        expect(RequestPath.CASES_PREFIX).toMatch(/^\//);
        expect(RequestPath.CHAT_PREFIX).toMatch(/^\//);
    });

    it('HttpMethod values are uppercase HTTP verb strings', () => {
        expect(HttpMethod.GET).toBe('GET');
        expect(HttpMethod.POST).toBe('POST');
        expect(HttpMethod.PUT).toBe('PUT');
        expect(HttpMethod.DELETE).toBe('DELETE');
    });

    it('ServiceClientEvent.READY is the correct event name string', () => {
        expect(ServiceClientEvent.READY).toBe('serviceClientReady');
    });

    it('HTTP header name constants are correct', () => {
        expect(WEB_SESSION_ID_HEADER).toBe('x-session-id');
        expect(BEARER_PREFIX).toBe('Bearer ');
        expect(CONTENT_TYPE_JSON).toBe('application/json');
        expect(AUTHORIZATION_HEADER).toBe('Authorization');
        expect(COOKIE_HEADER).toBe('Cookie');
        expect(WEB_SESSION_COOKIE_KEY).toBe('web_session_id');
        expect(API_KEY_HEADER).toBe('X-API-Key');
        expect(RATE_LIMIT_RESET_HEADER).toBe('RateLimit-Reset');
    });

    it('HttpStatus codes are correct', () => {
        expect(HttpStatus.UNAUTHORIZED).toBe(401);
        expect(HttpStatus.FORBIDDEN).toBe(403);
        expect(HttpStatus.NOT_FOUND).toBe(404);
        expect(HttpStatus.INTERNAL_ERROR).toBe(500);
    });

    it('RATE_LIMIT_FALLBACK_MESSAGE is a non-empty string', () => {
        expect(typeof RATE_LIMIT_FALLBACK_MESSAGE).toBe('string');
        expect(RATE_LIMIT_FALLBACK_MESSAGE.length).toBeGreaterThan(0);
    });

    it('RATE_LIMIT_FALLBACK_MESSAGE is the expected text', () => {
        expect(RATE_LIMIT_FALLBACK_MESSAGE).toBe('Too many requests. Please try again later.');
    });

    it('attachment limit constants are positive numbers', () => {
        expect(MAX_ATTACHMENT_SIZE).toBeGreaterThan(0);
        expect(MAX_TOTAL_ATTACHMENT_SIZE).toBeGreaterThan(MAX_ATTACHMENT_SIZE);
        expect(MAX_ATTACHMENT_FILES).toBeGreaterThan(0);
    });
});

describe('window.serviceClient initialization [UNIT - jsdom]', () => {
    it('sets window.serviceClient on module evaluation', () => {
        expect(window.serviceClient).toBeDefined();
        expect(typeof window.serviceClient.get).toBe('function');
        expect(typeof window.serviceClient.post).toBe('function');
        expect(typeof window.serviceClient.put).toBe('function');
        expect(typeof window.serviceClient.delete).toBe('function');
        expect(typeof window.serviceClient.upload).toBe('function');
        expect(typeof window.serviceClient.sendRequest).toBe('function');
    });

    it('does not overwrite an existing window.serviceClient on re-evaluation', async () => {
        const sentinel = { sentinel: true };
        window.serviceClient = sentinel;

        await import('@g8ed/public/js/utils/service-client.js');

        expect(window.serviceClient).toBe(sentinel);
    });

    it('dispatches serviceClientReady event on initialization', async () => {
        vi.resetModules();
        window.serviceClient = undefined;

        const dispatched = [];
        window.addEventListener(ServiceClientEvent.READY, (e) => dispatched.push(e));

        await import('@g8ed/public/js/utils/service-client.js');

        expect(dispatched).toHaveLength(1);
        expect(dispatched[0].type).toBe(ServiceClientEvent.READY);
        expect(dispatched[0].detail.serviceClient).toBe(window.serviceClient);
        expect(typeof dispatched[0].detail.timestamp).toBe('number');
    });

    it('does not dispatch serviceClientReady event when serviceClient already exists', async () => {
        window.serviceClient = { existing: true };

        const dispatched = [];
        window.addEventListener(ServiceClientEvent.READY, (e) => dispatched.push(e));

        await import('@g8ed/public/js/utils/service-client.js');

        expect(dispatched).toHaveLength(0);
    });

    it('configLoaded is set to true after initializeConfiguration', async () => {
        expect(window.serviceClient.configLoaded).toBe(true);
    });

    it('retryConfig is initialized from RetryConfig constants', () => {
        expect(window.serviceClient.retryConfig.maxRetries).toBe(RetryConfig.MAX_RETRIES);
        expect(window.serviceClient.retryConfig.retryDelay).toBe(RetryConfig.RETRY_DELAY_MS);
        expect(window.serviceClient.retryConfig.backoffMultiplier).toBe(RetryConfig.BACKOFF_MULTIPLIER);
        expect(window.serviceClient.retryConfig.timeoutMs).toBe(RequestTimeout.DEFAULT_MS);
    });
});

describe('RateLimitError [UNIT - jsdom]', () => {
    it('is an instance of Error', () => {
        expect(new RateLimitError('test')).toBeInstanceOf(Error);
    });

    it('name is "RateLimitError"', () => {
        expect(new RateLimitError('test').name).toBe('RateLimitError');
    });

    it('status is 429', () => {
        expect(new RateLimitError('test').status).toBe(429);
    });

    it('message is set from constructor argument', () => {
        expect(new RateLimitError('Too many requests').message).toBe('Too many requests');
    });

    it('retryAfter defaults to null when not provided', () => {
        expect(new RateLimitError('test').retryAfter).toBeNull();
    });

    it('retryAfter is set when provided', () => {
        expect(new RateLimitError('test', 42).retryAfter).toBe(42);
    });

    it('retryAfter is null when explicitly passed null', () => {
        expect(new RateLimitError('test', null).retryAfter).toBeNull();
    });

    it('has a stack trace', () => {
        expect(new RateLimitError('test').stack).toBeDefined();
    });
});

describe('ServiceClient.getServiceEndpoints [UNIT - jsdom]', () => {
    let client;

    beforeEach(() => {
        client = new ServiceClient();
    });

    it('returns window.location.origin for g8ed', () => {
        const endpoints = client.getServiceEndpoints(ComponentName.G8ED);
        expect(endpoints).toEqual(['https://localhost']);
    });

    it('reflects window.location.origin change for g8ed', () => {
        window.location = { origin: 'https://g8e.local' };
        const endpoints = client.getServiceEndpoints(ComponentName.G8ED);
        expect(endpoints).toEqual(['https://g8e.local']);
    });

    it('returns ComponentUrl.G8EE for g8ee', () => {
        const endpoints = client.getServiceEndpoints(ComponentName.G8EE);
        expect(endpoints).toEqual([ComponentUrl.G8EE]);
    });

    it('throws for an unknown component name', () => {
        expect(() => client.getServiceEndpoints('unknown-component')).toThrow('Unknown component: unknown-component');
    });
});

describe('ServiceClient.getAuthHeaders [UNIT - jsdom]', () => {
    let client;

    beforeEach(() => {
        client = new ServiceClient();
    });

    it('does not include Content-Type (set by sendRequest, not auth layer)', () => {
        const headers = client.getAuthHeaders();
        expect(headers['Content-Type']).toBeUndefined();
    });

    it('sets Authorization, Cookie, and session ID header when accessor provides a session ID', () => {
        client.registerAuthAccessor(() => ({ getWebSessionId: () => 'sess_abc123', getApiKey: () => null }));

        const headers = client.getAuthHeaders(ComponentName.G8ED);

        expect(headers[WEB_SESSION_ID_HEADER]).toBe('sess_abc123');
        expect(headers[AUTHORIZATION_HEADER]).toBe(`${BEARER_PREFIX}sess_abc123`);
        expect(headers[COOKIE_HEADER]).toBe(`${WEB_SESSION_COOKIE_KEY}=sess_abc123`);
    });

    it('includes API key header when accessor provides both session ID and API key', () => {
        client.registerAuthAccessor(() => ({ getWebSessionId: () => 'sess_abc123', getApiKey: () => 'key_xyz' }));

        const headers = client.getAuthHeaders(ComponentName.G8ED);

        expect(headers[API_KEY_HEADER]).toBe('key_xyz');
    });

    it('omits API key header when accessor has no API key', () => {
        client.registerAuthAccessor(() => ({ getWebSessionId: () => 'sess_abc123', getApiKey: () => null }));

        const headers = client.getAuthHeaders(ComponentName.G8ED);

        expect(headers[API_KEY_HEADER]).toBeUndefined();
    });

    it('omits auth headers when no accessor is registered (HttpOnly cookie path)', () => {
        const headers = client.getAuthHeaders(ComponentName.G8ED);

        expect(headers[WEB_SESSION_ID_HEADER]).toBeUndefined();
        expect(headers[AUTHORIZATION_HEADER]).toBeUndefined();
        expect(headers[COOKIE_HEADER]).toBeUndefined();
        expect(headers[API_KEY_HEADER]).toBeUndefined();
    });

    it('omits auth headers when accessor returns no session ID', () => {
        client.registerAuthAccessor(() => ({ getWebSessionId: () => null, getApiKey: () => null }));

        const headers = client.getAuthHeaders(ComponentName.G8ED);

        expect(headers[AUTHORIZATION_HEADER]).toBeUndefined();
        expect(headers[COOKIE_HEADER]).toBeUndefined();
    });

    it('Bearer token is prefixed with the BEARER_PREFIX constant value', () => {
        client.registerAuthAccessor(() => ({ getWebSessionId: () => 'sess_test', getApiKey: () => null }));

        const headers = client.getAuthHeaders(ComponentName.G8ED);

        expect(headers[AUTHORIZATION_HEADER]).toMatch(new RegExp(`^${BEARER_PREFIX}`));
    });

    it('Cookie header uses WEB_SESSION_COOKIE_KEY as the cookie name', () => {
        client.registerAuthAccessor(() => ({ getWebSessionId: () => 'sess_test', getApiKey: () => null }));

        const headers = client.getAuthHeaders(ComponentName.G8ED);

        expect(headers[COOKIE_HEADER]).toBe(`${WEB_SESSION_COOKIE_KEY}=sess_test`);
    });
});

describe('ServiceClient.sendRequest — URL and fetch [UNIT - jsdom]', () => {
    let client;

    beforeEach(() => {
        client = new ServiceClient();
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(makeOkResponse()));
    });

    it('constructs URL from g8ed origin + path', async () => {
        await client.sendRequest(ComponentName.G8ED, '/api/test');
        expect(fetch).toHaveBeenCalledWith(
            'https://localhost/api/test',
            expect.any(Object)
        );
    });

    it('constructs URL from g8ee origin + path', async () => {
        await client.sendRequest(ComponentName.G8EE, '/api/internal/health');
        expect(fetch).toHaveBeenCalledWith(
            `${ComponentUrl.G8EE}/api/internal/health`,
            expect.any(Object)
        );
    });

    it('passes credentials: include to fetch', async () => {
        await client.sendRequest(ComponentName.G8ED, '/api/test', { method: HttpMethod.GET });
        const fetchOptions = fetch.mock.calls[0][1];
        expect(fetchOptions.credentials).toBe('include');
    });

    it('merges caller-provided headers over auth headers', async () => {
        client.registerAuthAccessor(() => ({ getWebSessionId: () => 'sess_123', getApiKey: () => null }));
        const overrideHeader = { 'X-Custom': 'custom-value' };

        await client.sendRequest(ComponentName.G8ED, '/api/test', {
            method: HttpMethod.GET,
            headers: overrideHeader,
        });

        const fetchOptions = fetch.mock.calls[0][1];
        expect(fetchOptions.headers['X-Custom']).toBe('custom-value');
        expect(fetchOptions.headers[AUTHORIZATION_HEADER]).toBeDefined();
    });

    it('caller-provided header overrides auth header of same name', async () => {
        client.registerAuthAccessor(() => ({ getWebSessionId: () => 'sess_123', getApiKey: () => null }));

        await client.sendRequest(ComponentName.G8ED, '/api/test', {
            method: HttpMethod.GET,
            headers: { [AUTHORIZATION_HEADER]: 'Bearer override' },
        });

        const fetchOptions = fetch.mock.calls[0][1];
        expect(fetchOptions.headers[AUTHORIZATION_HEADER]).toBe('Bearer override');
    });

    it('passes an AbortSignal to fetch', async () => {
        await client.sendRequest(ComponentName.G8ED, '/api/test', { method: HttpMethod.GET });
        const fetchOptions = fetch.mock.calls[0][1];
        expect(fetchOptions.signal).toBeDefined();
    });

    it('returns the fetch response on success', async () => {
        const mockResponse = makeOkResponse({ status: 200 });
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse));

        const result = await client.sendRequest(ComponentName.G8ED, '/api/test', { method: HttpMethod.GET });
        expect(result).toBe(mockResponse);
    });

    it('does not set Content-Type when body is absent', async () => {
        await client.sendRequest(ComponentName.G8ED, '/api/test', { method: HttpMethod.GET });
        const fetchOptions = fetch.mock.calls[0][1];
        expect(fetchOptions.headers['Content-Type']).toBeUndefined();
    });

    it('sets Content-Type application/json when body is a string', async () => {
        await client.sendRequest(ComponentName.G8ED, '/api/test', {
            method: HttpMethod.POST,
            body: JSON.stringify({ key: 'val' }),
        });
        const fetchOptions = fetch.mock.calls[0][1];
        expect(fetchOptions.headers['Content-Type']).toBe(CONTENT_TYPE_JSON);
    });

    it('does not set Content-Type when body is FormData', async () => {
        await client.sendRequest(ComponentName.G8ED, '/api/test', {
            method: HttpMethod.POST,
            body: new FormData(),
        });
        const fetchOptions = fetch.mock.calls[0][1];
        expect(fetchOptions.headers['Content-Type']).toBeUndefined();
    });

    it('caller-provided Content-Type is preserved when body is a string', async () => {
        await client.sendRequest(ComponentName.G8ED, '/api/test', {
            method: HttpMethod.POST,
            body: JSON.stringify({ key: 'val' }),
            headers: { 'Content-Type': 'text/plain' },
        });
        const fetchOptions = fetch.mock.calls[0][1];
        expect(fetchOptions.headers['Content-Type']).toBe('text/plain');
    });
});

describe('ServiceClient.sendRequest — timeout selection [UNIT - jsdom]', () => {
    let client;
    let capturedSignal;

    function makeHangingFetch() {
        return vi.fn().mockImplementation((_url, opts) => {
            capturedSignal = opts.signal;
            return new Promise((_resolve, reject) => {
                opts.signal.addEventListener('abort', () => {
                    reject(new DOMException('The operation was aborted.', 'AbortError'));
                }, { once: true });
            });
        });
    }

    beforeEach(() => {
        capturedSignal = null;
        vi.useFakeTimers();
        client = new ServiceClient();
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    async function assertTimeoutMs(path, method, expectedTimeoutMs) {
        vi.stubGlobal('fetch', makeHangingFetch());
        const req = client.sendRequest(ComponentName.G8ED, path, { method });
        await Promise.resolve();

        expect(capturedSignal).not.toBeNull();
        expect(capturedSignal.aborted).toBe(false);

        vi.advanceTimersByTime(expectedTimeoutMs - 1);
        await Promise.resolve();
        expect(capturedSignal.aborted).toBe(false);

        vi.advanceTimersByTime(1);
        await Promise.resolve();
        expect(capturedSignal.aborted).toBe(true);

        await req.catch(() => {});
    }

    it('uses AUTH_MS timeout for auth paths', async () => {
        await assertTimeoutMs(
            `${RequestPath.AUTH_PREFIX}login`,
            HttpMethod.POST,
            RequestTimeout.AUTH_MS
        );
    });

    it('uses CASE_MS timeout for POST to cases path', async () => {
        await assertTimeoutMs(
            `${RequestPath.CASES_PREFIX}/new`,
            HttpMethod.POST,
            RequestTimeout.CASE_MS
        );
    });

    it('does NOT use CASE_MS timeout for GET to cases path (uses DEFAULT_MS)', async () => {
        await assertTimeoutMs(
            `${RequestPath.CASES_PREFIX}/abc`,
            HttpMethod.GET,
            RequestTimeout.DEFAULT_MS
        );
    });

    it('uses CHAT_MS timeout for chat paths', async () => {
        await assertTimeoutMs(
            `${RequestPath.CHAT_PREFIX}stream`,
            HttpMethod.POST,
            RequestTimeout.CHAT_MS
        );
    });

    it('uses DEFAULT_MS timeout for generic paths', async () => {
        await assertTimeoutMs(
            '/api/user/me',
            HttpMethod.GET,
            RequestTimeout.DEFAULT_MS
        );
    });

    it('abort causes sendRequest to throw', async () => {
        vi.stubGlobal('fetch', makeHangingFetch());
        const req = client.sendRequest(ComponentName.G8ED, '/api/user/me', { method: HttpMethod.GET });
        await Promise.resolve();

        vi.advanceTimersByTime(RequestTimeout.DEFAULT_MS);
        await expect(req).rejects.toMatchObject({ name: 'AbortError' });
    });
});

describe('ServiceClient.sendRequest — error handling [UNIT - jsdom]', () => {
    let client;

    beforeEach(() => {
        client = new ServiceClient();
    });

    it('throws RateLimitError on 429', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(make429Response({ body: { error: 'slow down' } })));

        await expect(
            client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET })
        ).rejects.toBeInstanceOf(RateLimitError);
    });

    it('uses error field from 429 body as RateLimitError message', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            make429Response({ body: { error: 'Too many messages. Please slow down.' } })
        ));

        await expect(
            client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET })
        ).rejects.toMatchObject({ message: 'Too many messages. Please slow down.' });
    });

    it('uses message field from 429 body when error field is absent', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            make429Response({ body: { message: 'Custom message from server' } })
        ));

        await expect(
            client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET })
        ).rejects.toMatchObject({ message: 'Custom message from server' });
    });

    it('prefers message field over error field in 429 body', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            make429Response({ body: { error: 'error field', message: 'message wins' } })
        ));

        await expect(
            client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET })
        ).rejects.toMatchObject({ message: 'message wins' });
    });

    it('falls back to RATE_LIMIT_FALLBACK_MESSAGE when 429 body is empty', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(make429Response({ body: {} })));

        await expect(
            client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET })
        ).rejects.toMatchObject({ message: RATE_LIMIT_FALLBACK_MESSAGE });
    });

    it('falls back to RATE_LIMIT_FALLBACK_MESSAGE when 429 body JSON parse fails', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
            ok: false,
            status: 429,
            statusText: 'Too Many Requests',
            headers: { get: vi.fn(() => null) },
            clone: () => ({ json: async () => { throw new Error('invalid json'); } }),
        }));

        await expect(
            client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET })
        ).rejects.toMatchObject({ message: RATE_LIMIT_FALLBACK_MESSAGE });
    });

    it('sets retryAfter from RATE_LIMIT_RESET_HEADER when present', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            make429Response({ body: {}, retryAfterValue: '30' })
        ));

        let thrown;
        try {
            await client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET });
        } catch (err) {
            thrown = err;
        }

        expect(thrown).toBeInstanceOf(RateLimitError);
        expect(thrown.retryAfter).toBe(30);
    });

    it('sets retryAfter to null when RATE_LIMIT_RESET_HEADER is absent', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(make429Response({ body: {} })));

        let thrown;
        try {
            await client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET });
        } catch (err) {
            thrown = err;
        }

        expect(thrown).toBeInstanceOf(RateLimitError);
        expect(thrown.retryAfter).toBeNull();
    });

    it('calls response.headers.get with RATE_LIMIT_RESET_HEADER constant', async () => {
        const getHeader = vi.fn(() => null);
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
            ok: false,
            status: 429,
            statusText: 'Too Many Requests',
            headers: { get: getHeader },
            clone: () => ({ json: async () => ({}) }),
        }));

        await client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET }).catch(() => {});

        expect(getHeader).toHaveBeenCalledWith(RATE_LIMIT_RESET_HEADER);
    });

    it('throws a plain Error for non-429 HTTP errors', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            makeErrorResponse(HttpStatus.FORBIDDEN, 'Forbidden')
        ));

        await expect(
            client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET })
        ).rejects.not.toBeInstanceOf(RateLimitError);
    });

    it('error message includes HTTP status code for non-429 errors', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            makeErrorResponse(HttpStatus.FORBIDDEN, 'Forbidden')
        ));

        await expect(
            client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET })
        ).rejects.toMatchObject({ message: 'HTTP 403: Forbidden' });
    });

    it('attaches response and errorData to thrown Error for non-429 errors', async () => {
        const errorBody = { error: 'not allowed' };
        const mockResponse = {
            ok: false,
            status: HttpStatus.FORBIDDEN,
            statusText: 'Forbidden',
            headers: { get: vi.fn(() => null) },
            json: async () => errorBody,
        };
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse));

        let thrown;
        try {
            await client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET });
        } catch (err) {
            thrown = err;
        }

        expect(thrown.response).toBe(mockResponse);
        expect(thrown.errorData).toEqual(errorBody);
    });

    it('uses error field from non-429 body in error message', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
            ok: false,
            status: HttpStatus.FORBIDDEN,
            statusText: 'Forbidden',
            headers: { get: vi.fn(() => null) },
            json: async () => ({ error: 'access denied' }),
        }));

        await expect(
            client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET })
        ).rejects.toMatchObject({ message: 'HTTP 403: access denied' });
    });

    it('uses message field from non-429 body when error field is absent', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
            ok: false,
            status: HttpStatus.FORBIDDEN,
            statusText: 'Forbidden',
            headers: { get: vi.fn(() => null) },
            json: async () => ({ message: 'forbidden message' }),
        }));

        await expect(
            client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET })
        ).rejects.toMatchObject({ message: 'HTTP 403: forbidden message' });
    });

    it('falls back to statusText when non-429 body has no error or message', async () => {
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
            makeErrorResponse(HttpStatus.NOT_FOUND, 'Not Found', {})
        ));

        await expect(
            client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET })
        ).rejects.toMatchObject({ message: 'HTTP 404: Not Found' });
    });

    it('rethrows network errors', async () => {
        const networkErr = new TypeError('Failed to fetch');
        vi.stubGlobal('fetch', vi.fn().mockRejectedValue(networkErr));

        await expect(
            client.sendRequest(ComponentName.G8ED, '/api/endpoint', { method: HttpMethod.GET })
        ).rejects.toThrow('Failed to fetch');
    });
});

describe('ServiceClient.get [UNIT - jsdom]', () => {
    let client;

    beforeEach(() => {
        client = new ServiceClient();
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(makeOkResponse()));
    });

    it('issues a GET request', async () => {
        await client.get(ComponentName.G8ED, '/api/user/me');
        const opts = fetch.mock.calls[0][1];
        expect(opts.method).toBe(HttpMethod.GET);
    });

    it('passes extra options through to sendRequest', async () => {
        await client.get(ComponentName.G8ED, '/api/user/me', { headers: { 'X-Test': 'yes' } });
        const opts = fetch.mock.calls[0][1];
        expect(opts.headers['X-Test']).toBe('yes');
    });
});

describe('ServiceClient.post [UNIT - jsdom]', () => {
    let client;

    beforeEach(() => {
        client = new ServiceClient();
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(makeOkResponse()));
    });

    it('issues a POST request', async () => {
        await client.post(ComponentName.G8ED, '/api/chat/stream', { message: 'hello' });
        const opts = fetch.mock.calls[0][1];
        expect(opts.method).toBe(HttpMethod.POST);
    });

    it('serializes data as JSON body', async () => {
        const data = { key: 'value' };
        await client.post(ComponentName.G8ED, '/api/chat/stream', data);
        const opts = fetch.mock.calls[0][1];
        expect(opts.body).toBe(JSON.stringify(data));
    });

    it('sets Content-Type to CONTENT_TYPE_JSON', async () => {
        await client.post(ComponentName.G8ED, '/api/chat/stream', { key: 'val' });
        const opts = fetch.mock.calls[0][1];
        expect(opts.headers['Content-Type']).toBe(CONTENT_TYPE_JSON);
    });

    it('omits body when data is null', async () => {
        await client.post(ComponentName.G8ED, '/api/chat/stop', null);
        const opts = fetch.mock.calls[0][1];
        expect(opts.body).toBeUndefined();
    });
});

describe('ServiceClient.put [UNIT - jsdom]', () => {
    let client;

    beforeEach(() => {
        client = new ServiceClient();
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(makeOkResponse()));
    });

    it('issues a PUT request', async () => {
        await client.put(ComponentName.G8ED, '/api/settings', { theme: 'dark' });
        const opts = fetch.mock.calls[0][1];
        expect(opts.method).toBe(HttpMethod.PUT);
    });

    it('serializes data as JSON body', async () => {
        const data = { theme: 'dark' };
        await client.put(ComponentName.G8ED, '/api/settings', data);
        const opts = fetch.mock.calls[0][1];
        expect(opts.body).toBe(JSON.stringify(data));
    });

    it('sets Content-Type to CONTENT_TYPE_JSON', async () => {
        await client.put(ComponentName.G8ED, '/api/settings', { theme: 'dark' });
        const opts = fetch.mock.calls[0][1];
        expect(opts.headers['Content-Type']).toBe(CONTENT_TYPE_JSON);
    });

    it('omits body when data is null', async () => {
        await client.put(ComponentName.G8ED, '/api/settings', null);
        const opts = fetch.mock.calls[0][1];
        expect(opts.body).toBeUndefined();
    });
});

describe('ServiceClient.delete [UNIT - jsdom]', () => {
    let client;

    beforeEach(() => {
        client = new ServiceClient();
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(makeOkResponse()));
    });

    it('issues a DELETE request', async () => {
        await client.delete(ComponentName.G8ED, '/api/chat/cases/case_123');
        const opts = fetch.mock.calls[0][1];
        expect(opts.method).toBe(HttpMethod.DELETE);
    });

    it('constructs correct URL', async () => {
        await client.delete(ComponentName.G8ED, '/api/chat/cases/case_123');
        expect(fetch).toHaveBeenCalledWith(
            'https://localhost/api/chat/cases/case_123',
            expect.any(Object)
        );
    });
});

describe('ServiceClient.upload [UNIT - jsdom]', () => {
    let client;

    beforeEach(() => {
        client = new ServiceClient();
        vi.stubGlobal('fetch', vi.fn().mockResolvedValue(makeOkResponse()));
    });

    it('issues a POST request', async () => {
        const formData = new FormData();
        await client.upload(ComponentName.G8ED, '/api/upload', formData);
        const opts = fetch.mock.calls[0][1];
        expect(opts.method).toBe(HttpMethod.POST);
    });

    it('passes FormData as body', async () => {
        const formData = new FormData();
        formData.append('file', new Blob(['content'], { type: 'text/plain' }), 'test.txt');
        await client.upload(ComponentName.G8ED, '/api/upload', formData);
        const opts = fetch.mock.calls[0][1];
        expect(opts.body).toBe(formData);
    });

    it('does not set Content-Type header so browser can set multipart boundary', async () => {
        const formData = new FormData();
        await client.upload(ComponentName.G8ED, '/api/upload', formData);
        const opts = fetch.mock.calls[0][1];
        expect(opts.headers?.['Content-Type']).toBeUndefined();
    });

    it('preserves caller-provided Content-Type header when explicitly set', async () => {
        const formData = new FormData();
        await client.upload(ComponentName.G8ED, '/api/upload', formData, {
            headers: { 'Content-Type': 'multipart/form-data' },
        });
        const opts = fetch.mock.calls[0][1];
        expect(opts.headers?.['Content-Type']).toBe('multipart/form-data');
    });
});
