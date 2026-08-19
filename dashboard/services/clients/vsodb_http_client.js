// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * g8edBHttpClient — Purpose-built HTTP client for g8edB.
 * 
 * Shared base for g8edBDocumentClient and KVCacheClient.
 * Provides timeout, error logging, and auth header propagation
 * for all HTTP calls to g8edB (Operator --listen mode).
 * 
 * Architecture (from docs/architecture/storage-data-flows.md):
 *   g8ed -> g8edB uses HTTP ($DROPOPS_INTERNAL_HTTP_URL) for KV, document store.
 *   DB operations are never routed over WebSocket.
 */

import { logger } from '../../utils/logger.js';
import { g8edB_HTTP_TIMEOUT_MS } from '../../constants/http_client.js';
import { HTTP_INTERNAL_AUTH_HEADER, HTTP_CONTENT_TYPE_HEADER } from '../../constants/headers.js';

class g8edBHttpError extends Error {
    constructor(message, status) {
        super(message);
        this.name = 'g8edBHttpError';
        this.status = status;
    }
}

class g8edBHttpClient {
    /**
     * @param {object} config
     * @param {string} config.listenUrl - Base URL of g8edB (e.g. $DROPOPS_INTERNAL_HTTP_URL)
     * @param {string} [config.component] - Client component name for log prefixes
     * @param {string} [config.internalAuthToken] - Shared secret for g8edB authentication
     */
    constructor({ listenUrl, component = 'g8edB-HTTP', internalAuthToken = null } = {}) {
        if (!listenUrl) {
            throw new Error('g8edBHttpClient: listenUrl is required');
        }
        this.listenUrl = listenUrl.replace(/\/$/, '');
        this.component = component;
        this.internalAuthToken = internalAuthToken;
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
     * Make an HTTP request to g8edB with timeout and structured error handling.
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
        const timeoutId = setTimeout(() => timeoutController.abort(), g8edB_HTTP_TIMEOUT_MS);

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
                throw new g8edBHttpError(`g8edB returned non-JSON response: ${text}`, res.status);
            }

            if (!res.ok) {
                const errMsg = data.error;
                if (res.status === 404) {
                    logger.info(`[${this.component}] ${method} ${path} failed: ${errMsg || `HTTP ${res.status}`}`);
                } else {
                    logger.error(`[${this.component}] ${method} ${path} failed: ${errMsg || `HTTP ${res.status}`}`);
                }
                throw new g8edBHttpError(errMsg || `HTTP ${res.status}`, res.status);
            }
            return data;
        } catch (error) {
            clearTimeout(timeoutId);

            if (error instanceof g8edBHttpError) {
                throw error;
            }

            if (error.name === 'AbortError') {
                logger.error(`[${this.component}] ${method} ${path} timeout after ${g8edB_HTTP_TIMEOUT_MS}ms`);
                throw new Error(`g8edB request timeout: ${method} ${path} after ${g8edB_HTTP_TIMEOUT_MS}ms`);
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
            throw new Error(`g8edBHttpClient.put: body must be a pre-serialized JSON string, got ${typeof body}`);
        }
        return this.request('PUT', path, { ...options, body });
    }

    /**
     * Convenience: PATCH request with pre-serialized JSON body
     */
    async patch(path, body, options = {}) {
        if (typeof body !== 'string') {
            throw new Error(`g8edBHttpClient.patch: body must be a pre-serialized JSON string, got ${typeof body}`);
        }
        return this.request('PATCH', path, { ...options, body });
    }

    /**
     * Convenience: POST request with pre-serialized JSON body
     */
    async post(path, body, options = {}) {
        if (typeof body !== 'string') {
            throw new Error(`g8edBHttpClient.post: body must be a pre-serialized JSON string, got ${typeof body}`);
        }
        return this.request('POST', path, { ...options, body });
    }

    /**
     * Convenience: DELETE request
     */
    async delete(path, options = {}) {
        return this.request('DELETE', path, options);
    }

    isTerminated() {
        return this._terminated;
    }

    terminate() {
        this._terminated = true;
    }
}

export { g8edBHttpClient, g8edBHttpError };
