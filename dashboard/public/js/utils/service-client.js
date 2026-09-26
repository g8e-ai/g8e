// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { devLogger } from './dev-logger.js';
import {
    ServiceName,
    RequestTimeout,
    RetryConfig,
    RATE_LIMIT_RESET_HEADER,
    RATE_LIMIT_FALLBACK_MESSAGE,
    HttpMethod,
    CONTENT_TYPE_JSON,
} from '../constants/service-client-constants.js';

class RateLimitError extends Error {
    constructor(message, retryAfter = null) {
        super(message);
        this.name = 'RateLimitError';
        this.status = 429;
        this.retryAfter = retryAfter;
    }
}

class ServiceClient {
    constructor() {
        this.retryConfig = {
            maxRetries: RetryConfig.MAX_RETRIES,
            retryDelay: RetryConfig.RETRY_DELAY_MS,
            backoffMultiplier: RetryConfig.BACKOFF_MULTIPLIER,
            timeoutMs: RequestTimeout.DEFAULT_MS,
        };
        this.configLoaded = false;
        this.initializeConfiguration();
    }

    getServiceEndpoints(serviceName) {
        if (serviceName !== ServiceName.GATEWAY) {
            throw new Error(`Unknown service: ${serviceName}. Browser APIs must use ServiceName.GATEWAY`);
        }
        if (!window.G8E_GATEWAY_URL) {
            throw new Error('Gateway origin not configured: window.G8E_GATEWAY_URL is unset (expected /g8e-config.js to set it from G8E_GATEWAY_URL)');
        }
        return [window.G8E_GATEWAY_URL];
    }

    async initializeConfiguration() {
        devLogger.log('[ServiceClient] Gateway URL:', window.G8E_GATEWAY_URL ?? '(unset — /g8e-config.js not loaded)');
        this.configLoaded = true;
    }

    async sendRequest(serviceName, path, options = {}) {
        const startTime = performance.now();
        const requestId = Math.random().toString(36).substring(2, 9);
        const baseUrl = this.getServiceEndpoints(serviceName)[0];
        const url = `${baseUrl}${path}`;

        const mergedHeaders = {
            ...(typeof options.body === 'string' ? { 'Content-Type': CONTENT_TYPE_JSON } : {}),
            ...options.headers,
        };

        const controller = new AbortController();
        const isAuthRequest = path.includes('/auth/passkeys/');
        const isCaseRequest = path.includes('/cases');
        const isChatCall = path.includes('/chat');
        const timeoutMs = isAuthRequest ? RequestTimeout.AUTH_MS
            : isCaseRequest ? RequestTimeout.CASE_MS
                : isChatCall ? RequestTimeout.CHAT_MS
                    : this.retryConfig.timeoutMs;
        const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

        devLogger.log(`[ ServiceClient ] REQUEST [${requestId}] ${options.method || 'GET'} ${url}`);

        try {
            const response = await fetch(url, {
                ...options,
                headers: mergedHeaders,
                credentials: 'include',
                signal: controller.signal,
            });

            const duration = Math.round(performance.now() - startTime);
            devLogger.log(`[ ServiceClient ] RESPONSE [${requestId}] ${response.status} ${duration}ms ${url}`);
            clearTimeout(timeoutId);

            if (!response.ok) {
                if (response.status === 429) {
                    const errorData = await response.clone().json().catch(() => ({}));
                    const retryAfter = response.headers.get(RATE_LIMIT_RESET_HEADER);
                    throw new RateLimitError(
                        errorData.message || errorData.error || RATE_LIMIT_FALLBACK_MESSAGE,
                        retryAfter ? parseInt(retryAfter, 10) : null,
                    );
                }

                const errorBody = await response.json().catch(() => null);
                const errorMessage = errorBody?.error || errorBody?.message || response.statusText || '';
                const err = new Error(`HTTP ${response.status}: ${errorMessage}`);
                err.response = response;
                err.errorData = errorBody;
                throw err;
            }

            return response;
        } catch (error) {
            clearTimeout(timeoutId);
            const duration = Math.round(performance.now() - startTime);
            devLogger.error(`[ ServiceClient ] ERROR [${requestId}] ${duration}ms ${url}`, error);
            throw error;
        }
    }

    get(serviceName, path, options = {}) {
        return this.sendRequest(serviceName, path, { ...options, method: HttpMethod.GET });
    }

    post(serviceName, path, body, options = {}) {
        return this.sendRequest(serviceName, path, {
            ...options,
            method: HttpMethod.POST,
            body: body !== undefined ? JSON.stringify(body) : undefined,
        });
    }

    put(serviceName, path, body, options = {}) {
        return this.sendRequest(serviceName, path, {
            ...options,
            method: HttpMethod.PUT,
            body: body !== undefined ? JSON.stringify(body) : undefined,
        });
    }

    delete(serviceName, path, options = {}) {
        return this.sendRequest(serviceName, path, { ...options, method: HttpMethod.DELETE });
    }
}

export const serviceClient = new ServiceClient();
window.serviceClient = serviceClient;
export { RateLimitError };
