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

import { devLogger } from './dev-logger.js';
import {
    ComponentName,
    ComponentUrl,
    RequestTimeout,
    RetryConfig,
    RequestPath,
    WEB_SESSION_ID_HEADER,
    BEARER_PREFIX,
    CONTENT_TYPE_JSON,
    AUTHORIZATION_HEADER,
    COOKIE_HEADER,
    WEB_SESSION_COOKIE_KEY,
    API_KEY_HEADER,
    RATE_LIMIT_RESET_HEADER,
    RATE_LIMIT_FALLBACK_MESSAGE,
    HttpMethod,
    ServiceClientEvent,
} from '../constants/service-client-constants.js';

class RateLimitError extends Error {
    constructor(message, retryAfter = null) {
        super(message);
        this.name = 'RateLimitError';
        this.status = 429;
        this.retryAfter = retryAfter;
    }
}

const COMPONENT_URLS = {
    [ComponentName.G8EE]: ComponentUrl.G8EE,
    [ComponentName.G8ED]: ComponentUrl.G8ED,
    [ComponentName.G8ES]: ComponentUrl.G8ES,
};

class ServiceClient {
    constructor() {
        this.retryConfig = {
            maxRetries: RetryConfig.MAX_RETRIES,
            retryDelay: RetryConfig.RETRY_DELAY_MS,
            backoffMultiplier: RetryConfig.BACKOFF_MULTIPLIER,
            timeoutMs: RequestTimeout.DEFAULT_MS
        };

        this.currentEndpointIndex = {
            [ComponentName.G8EE]: 0,
            [ComponentName.G8ED]: 0,
        };

        this._authAccessor = null;

        this.configLoaded = false;
        this.initializeConfiguration();
    }

    registerAuthAccessor(accessor) {
        this._authAccessor = accessor;
    }



    getServiceEndpoints(componentName) {
        if (componentName === ComponentName.G8ED) {
            return [window.location.origin];
        }
        
        if (COMPONENT_URLS[componentName]) {
            return [COMPONENT_URLS[componentName]];
        }
        
        throw new Error(`Unknown component: ${componentName}`);
    }

    getAuthHeaders(componentName = null) {
        const headers = {};

        const auth = this._authAccessor ? this._authAccessor() : null;

        let webSessionId = auth?.getWebSessionId() ?? this.getWebSessionIdFromCookie();

        if (webSessionId) {
            headers[WEB_SESSION_ID_HEADER] = webSessionId;
            headers[AUTHORIZATION_HEADER] = `${BEARER_PREFIX}${webSessionId}`;
            headers[COOKIE_HEADER] = `${WEB_SESSION_COOKIE_KEY}=${webSessionId}`;

            const apiKey = auth?.getApiKey() ?? null;
            if (apiKey) {
                headers[API_KEY_HEADER] = apiKey;
            }
        } else if (auth) {
            devLogger.warn(`[ ServiceClient ] No session ID found for ${componentName || ComponentName.G8ED}`);
        }

        return headers;
    }


    getWebSessionIdFromCookie() {
        // Cannot read HttpOnly cookies from JavaScript - sent automatically by browser
        devLogger.log('[ ServiceClient ] WebSession managed via HttpOnly cookie (sent automatically)');
        return null;
    }

    async initializeConfiguration() {
        devLogger.log('[ServiceClient] Component URLs:', {
            [ComponentName.G8EE]: COMPONENT_URLS[ComponentName.G8EE],
            [ComponentName.G8ED]: window.location.origin,
        });
        this.configLoaded = true;
    }


    async sendRequest(componentName, path, options = {}) {
        const startTime = performance.now();
        const requestId = Math.random().toString(36).substring(2, 9);

        const mappedPath = path;

        const endpoints = this.getServiceEndpoints(componentName);
        const baseUrl = endpoints[0]; // Use primary endpoint only

        const url = `${baseUrl}${mappedPath}`;

        const authHeaders = this.getAuthHeaders(componentName);

        const mergedHeaders = {
            ...(typeof options.body === 'string' ? { 'Content-Type': CONTENT_TYPE_JSON } : {}),
            ...authHeaders,
            ...options.headers
        };

        const controller = new AbortController();
        const isAuthRequest = path.includes(RequestPath.AUTH_PREFIX);
        const isCaseCreation = path.includes(RequestPath.CASES_PREFIX) && (options.method === HttpMethod.POST || !options.method);
        const isChatCall = mappedPath.includes(RequestPath.CHAT_PREFIX);
        const timeoutMs = isAuthRequest ? RequestTimeout.AUTH_MS :
            isCaseCreation ? RequestTimeout.CASE_MS :
                isChatCall ? RequestTimeout.CHAT_MS :
                    this.retryConfig.timeoutMs;
        const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

        devLogger.log(`[ ServiceClient ] REQUEST [${requestId}] ${options.method || 'GET'} ${url}`);

        try {
            const response = await fetch(url, {
                ...options,
                headers: mergedHeaders,
                credentials: 'include',
                signal: controller.signal
            });

            const endTime = performance.now();
            const duration = Math.round(endTime - startTime);

            devLogger.log(`[ ServiceClient ] RESPONSE [${requestId}] ${response.status} ${duration}ms ${url}`);

            clearTimeout(timeoutId);

            if (!response.ok) {
                if (response.status === 429) {
                    const errorData = await response.clone().json().catch(() => ({}));
                    const retryAfter = response.headers.get(RATE_LIMIT_RESET_HEADER);
                    devLogger.warn('[ ServiceClient ] Rate limit exceeded:', errorData);
                    throw new RateLimitError(
                        errorData.message || errorData.error || RATE_LIMIT_FALLBACK_MESSAGE,
                        retryAfter ? parseInt(retryAfter, 10) : null
                    );
                }

                const errorBody = await Promise.resolve().then(() => response.json()).catch(() => null);
                const errorMessage = errorBody?.error || errorBody?.message || response.statusText || '';
                const err = new Error(`HTTP ${response.status}: ${errorMessage}`);
                err.response = response;
                err.errorData = errorBody;
                throw err;
            }

            return response;
        } catch (error) {
            const endTime = performance.now();
            const duration = Math.round(endTime - startTime);

            clearTimeout(timeoutId);

            devLogger.warn(`[ ServiceClient ] ERROR [${requestId}] ${url} ${duration}ms: ${error.message}`);

            throw error;
        }
    }

    async get(componentName, path, options = {}) {
        return this.sendRequest(componentName, path, { method: HttpMethod.GET, ...options });
    }

    async post(componentName, path, data = null, options = {}) {
        const requestOptions = {
            method: HttpMethod.POST,
            headers: { 'Content-Type': CONTENT_TYPE_JSON, ...options.headers },
            ...options
        };
        if (data) requestOptions.body = JSON.stringify(data);
        return this.sendRequest(componentName, path, requestOptions);
    }

    async put(componentName, path, data = null, options = {}) {
        const requestOptions = {
            method: HttpMethod.PUT,
            headers: { 'Content-Type': CONTENT_TYPE_JSON, ...options.headers },
            ...options
        };
        if (data) requestOptions.body = JSON.stringify(data);
        return this.sendRequest(componentName, path, requestOptions);
    }

    async delete(componentName, path, options = {}) {
        return this.sendRequest(componentName, path, { method: HttpMethod.DELETE, ...options });
    }

    async upload(componentName, path, formData, options = {}) {
        const requestOptions = {
            method: HttpMethod.POST,
            body: formData,
            ...options
        };
        return this.sendRequest(componentName, path, requestOptions);
    }
}

// Create global service client instance (with guard against duplicate initialization)
if (!window.serviceClient) {
    window.serviceClient = new ServiceClient();

    // Emit custom event to notify that serviceClient is ready
    devLogger.log('[ServiceClient] Initialized and ready');
    window.dispatchEvent(new CustomEvent(ServiceClientEvent.READY, {
        detail: {
            timestamp: Date.now(),
            serviceClient: window.serviceClient
        }
    }));
} else {
    devLogger.log('[ServiceClient] Already initialized, skipping duplicate initialization');
}

export { ServiceClient, RateLimitError };
