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

/**
 * g8esHttpClient — Purpose-built HTTP client for g8es.
 * 
 * Shared base for g8esDocumentClient and KVCacheClient.
 * Provides timeout, error logging, and auth header propagation
 * for all HTTP calls to g8es (Operator --listen mode).
 * 
 * Architecture (from docs/architecture/storage-data-flows.md):
 *   g8ed -> g8es uses HTTP ($G8E_INTERNAL_HTTP_URL) for KV, document store.
 *   DB operations are never routed over WebSocket.
 */

import { logger } from '../../utils/logger.js';
import { g8es_HTTP_TIMEOUT_MS } from '../../constants/http_client.js';
import { HTTP_INTERNAL_AUTH_HEADER, HTTP_CONTENT_TYPE_HEADER } from '../../constants/headers.js';

class g8esHttpError extends Error {
    constructor(message, status) {
        super(message);
        this.name = 'G8esHttpError';
        this.status = status;
    }
}

class g8esHttpClient {
    /**
     * @param {object} config
     * @param {string} config.listenUrl - Base URL of g8es (e.g. $G8E_INTERNAL_HTTP_URL)
     * @param {string} [config.component] - Client component name for log prefixes
     * @param {string} [config.internalAuthToken] - Shared secret for g8es authentication
     * @param {string} [config.caCertPath] - Path to CA certificate for TLS verification
     */
    constructor({ listenUrl, component = 'G8E-HTTP', internalAuthToken = null, caCertPath = null } = {}) {
        if (!listenUrl) {
            throw new Error('G8esHttpClient: listenUrl is required');
        }
        this.listenUrl = listenUrl.replace(/\/$/, '');
        this.component = component;
        this.internalAuthToken = internalAuthToken;
        this.caCertPath = caCertPath;
        this._terminated = false;
    }

    _headers() {
        const headers = { [HTTP_CONTENT_TYPE_HEADER]: 'application/json' };
        if (this.internalAuthToken) {
            headers[HTTP_INTERNAL_AUTH_HEADER] = this.internalAuthToken;
        }
        return headers;
    }

    /**
     * Make an HTTP request to g8es with timeout and structured error handling.
     *
     * @param {string} method - HTTP method
     * @param {string} path - URL path (e.g. /db/collection/id or /kv/key)
     * @param {object} [options] - Additional fetch options (body, headers)
     * @returns {Promise<any>} Parsed JSON response
     * @throws {Error} On timeout, HTTP error, or network failure
     */
    async request(method, path, options = {}) {
        if (this._terminated) {
            throw new Error('Client terminated');
        }

        const url = `${this.listenUrl}${path}`;
        const timeoutController = new AbortController();
        const timeoutId = setTimeout(() => timeoutController.abort(), g8es_HTTP_TIMEOUT_MS);

        try {
            const fetchOptions = {
                method,
                ...options,
                signal: timeoutController.signal,
                headers: { ...this._headers(), ...options.headers },
            };

            const res = await fetch(url, fetchOptions);
            clearTimeout(timeoutId);

            const text = await res.text();
            let data;
            try {
                data = JSON.parse(text);
            } catch {
                throw new g8esHttpError(`g8es returned non-JSON response: ${text}`, res.status);
            }

            if (!res.ok) {
                const errMsg = data.error;
                if (res.status === 404) {
                    logger.info(`[${this.component}] ${method} ${path} failed: ${errMsg || `HTTP ${res.status}`}`);
                } else {
                    logger.error(`[${this.component}] ${method} ${path} failed: ${errMsg || `HTTP ${res.status}`}`);
                }
                throw new g8esHttpError(errMsg || `HTTP ${res.status}`, res.status);
            }
            return data;
        } catch (error) {
            clearTimeout(timeoutId);

            if (error instanceof g8esHttpError) {
                throw error;
            }

            if (error.name === 'AbortError') {
                logger.error(`[${this.component}] ${method} ${path} timeout after ${G8ES_HTTP_TIMEOUT_MS}ms`);
                throw new Error(`g8es request timeout: ${method} ${path} after ${G8ES_HTTP_TIMEOUT_MS}ms`);
            }

            logger.error(`[${this.component}] ${method} ${path} failed`, {
                url,
                error: error.message,
            });
            throw error;
        }
    }

    /**
     * Convenience: GET request
     */
    async get(path, options = {}) {
        return this.request('GET', path, options);
    }

    /**
     * Convenience: PUT request with pre-serialized JSON body
     */
    async put(path, body, options = {}) {
        if (typeof body !== 'string') {
            throw new Error(`G8esHttpClient.put: body must be a pre-serialized JSON string, got ${typeof body}`);
        }
        return this.request('PUT', path, { ...options, body });
    }

    /**
     * Convenience: PATCH request with pre-serialized JSON body
     */
    async patch(path, body, options = {}) {
        if (typeof body !== 'string') {
            throw new Error(`G8esHttpClient.patch: body must be a pre-serialized JSON string, got ${typeof body}`);
        }
        return this.request('PATCH', path, { ...options, body });
    }

    /**
     * Convenience: POST request with pre-serialized JSON body
     */
    async post(path, body, options = {}) {
        if (typeof body !== 'string') {
            throw new Error(`G8esHttpClient.post: body must be a pre-serialized JSON string, got ${typeof body}`);
        }
        return this.request('POST', path, { ...options, body });
    }

    /**
     * Convenience: DELETE request
     */
    async delete(path, options = {}) {
        return this.request('DELETE', path, options);
    }

    /**
     * Check health of the upstream service.
     * @returns {Promise<boolean>}
     */
    async healthCheck() {
        try {
            const data = await this.get('/health');
            return data && (data.status === 'ok' || data.status === 'healthy');
        } catch (error) {
            logger.warn(`[${this.component}] Health check failed`, {
                url: this.listenUrl,
                error: error.message
            });
            return false;
        }
    }

    /**
     * Wait for upstream service to become healthy.
     * @param {number} maxRetries
     * @param {number} delayMs
     * @returns {Promise<void>}
     * @throws {Error} If service does not become healthy within retries
     */
    async waitForReady(maxRetries = 30, delayMs = 1000) {
        logger.info(`[${this.component}] Waiting for service to become healthy...`, {
            url: this.listenUrl,
            maxRetries,
            delayMs
        });

        for (let i = 0; i < maxRetries; i++) {
            if (await this.healthCheck()) {
                logger.info(`[${this.component}] Service is healthy`, { url: this.listenUrl });
                return;
            }
            
            if (i < maxRetries - 1) {
                await new Promise(resolve => setTimeout(resolve, delayMs));
            }
        }

        throw new Error(`[${this.component}] Service failed to become healthy after ${maxRetries} attempts: ${this.listenUrl}`);
    }

    isTerminated() {
        return this._terminated;
    }

    terminate() {
        this._terminated = true;
    }
}

export { g8esHttpClient, g8esHttpError };
