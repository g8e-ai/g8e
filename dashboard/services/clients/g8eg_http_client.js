// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * g8egHttpClient — Purpose-built HTTP client for g8eg.
 * 
 * Shared base for g8egDocumentClient and KVCacheClient.
 * Provides timeout, error logging, and auth header propagation
 * for all HTTP calls to g8eg (Operator --listen mode).
 * 
 * Architecture (from docs/architecture/storage-data-flows.md):
 *   g8ed -> g8eg uses HTTP ($G8E_INTERNAL_HTTP_URL) for KV, document store.
 *   DB operations are never routed over WebSocket.
 */

import { readFileSync, existsSync } from 'node:fs';
import { Agent } from 'undici';
import { logger } from '../../utils/logger.js';
import { g8eg_HTTP_TIMEOUT_MS } from '../../constants/http_client.js';
import { HTTP_INTERNAL_AUTH_HEADER, HTTP_CONTENT_TYPE_HEADER } from '../../constants/headers.js';

class g8egHttpError extends Error {
    constructor(message, status) {
        super(message);
        this.name = 'g8egHttpError';
        this.status = status;
    }
}

class g8egHttpClient {
    /**
     * @param {object} config
     * @param {string} config.listenUrl - Base URL of g8eg (e.g. $G8E_INTERNAL_HTTP_URL)
     * @param {string} [config.component] - Client component name for log prefixes
     * @param {string} [config.internalAuthToken] - Shared secret for g8eg authentication
     * @param {string} [config.clientCertPath] - Path to the mTLS client cert (PEM). When
     *   provided alongside `clientKeyPath`, an undici `Agent` is constructed with
     *   `connect: { cert, key, ca }` and passed as the `dispatcher` option on each
     *   `fetch` call so the dashboard presents its enrolled app cert and verifies the
     *   gateway's CA. The internal auth token header is still sent when set (coexistence).
     * @param {string} [config.clientKeyPath] - Path to the mTLS client private key (PEM).
     * @param {string} [config.caCertPath] - Path to the gateway CA bundle (PEM) for
     *   verifying the gateway's presented cert.
     */
    constructor({ listenUrl, component = 'g8eg-HTTP', internalAuthToken = null, clientCertPath = null, clientKeyPath = null, caCertPath = null } = {}) {
        if (!listenUrl) {
            throw new Error('g8egHttpClient: listenUrl is required');
        }
        this.listenUrl = listenUrl.replace(/\/$/, '');
        this.component = component;
        this.internalAuthToken = internalAuthToken;
        this._terminated = false;
        this._dispatcher = this._buildDispatcher(clientCertPath, clientKeyPath, caCertPath);
    }

    /**
     * Build an undici `Agent` dispatcher for mTLS when both cert and key paths
     * are provided and exist on disk. Returns `null` when mTLS config is absent
     * (non-mTLS deployment falls back to plain HTTP + internal auth token).
     *
     * @param {string|null} certPath
     * @param {string|null} keyPath
     * @param {string|null} caPath
     * @returns {import('undici').Agent | null}
     */
    _buildDispatcher(certPath, keyPath, caPath) {
        if (!certPath || !keyPath) {
            return null;
        }
        if (!existsSync(certPath) || !existsSync(keyPath)) {
            logger.warn(`[${this.component}] mTLS cert/key not found on disk (cert=${certPath}, key=${keyPath}); falling back to non-mTLS`);
            return null;
        }
        const connect = {
            cert: readFileSync(certPath, 'utf8'),
            key: readFileSync(keyPath, 'utf8'),
        };
        if (caPath && existsSync(caPath)) {
            connect.ca = readFileSync(caPath, 'utf8');
        }
        return new Agent({ connect });
    }

    _headers() {
        const headers = { [HTTP_CONTENT_TYPE_HEADER]: 'application/json' };
        if (this.internalAuthToken) {
            headers[HTTP_INTERNAL_AUTH_HEADER] = this.internalAuthToken;
        }
        return headers;
    }

    /**
     * Make an HTTP request to g8eg with timeout and structured error handling.
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
        const timeoutId = setTimeout(() => timeoutController.abort(), g8eg_HTTP_TIMEOUT_MS);

        try {
            const fetchOptions = {
                method,
                ...options,
                signal: timeoutController.signal,
                headers: { ...this._headers(), ...options.headers },
            };
            if (this._dispatcher) {
                fetchOptions.dispatcher = this._dispatcher;
            }

            const res = await fetch(url, fetchOptions);
            clearTimeout(timeoutId);

            const text = await res.text();
            let data;
            try {
                data = JSON.parse(text);
            } catch {
                throw new g8egHttpError(`g8eg returned non-JSON response: ${text}`, res.status);
            }

            if (!res.ok) {
                const errMsg = data.error;
                if (res.status === 404) {
                    logger.info(`[${this.component}] ${method} ${path} failed: ${errMsg || `HTTP ${res.status}`}`);
                } else {
                    logger.error(`[${this.component}] ${method} ${path} failed: ${errMsg || `HTTP ${res.status}`}`);
                }
                throw new g8egHttpError(errMsg || `HTTP ${res.status}`, res.status);
            }
            return data;
        } catch (error) {
            clearTimeout(timeoutId);

            if (error instanceof g8egHttpError) {
                throw error;
            }

            if (error.name === 'AbortError') {
                logger.error(`[${this.component}] ${method} ${path} timeout after ${g8eg_HTTP_TIMEOUT_MS}ms`);
                throw new Error(`g8eg request timeout: ${method} ${path} after ${g8eg_HTTP_TIMEOUT_MS}ms`);
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
            throw new Error(`g8egHttpClient.put: body must be a pre-serialized JSON string, got ${typeof body}`);
        }
        return this.request('PUT', path, { ...options, body });
    }

    /**
     * Convenience: PATCH request with pre-serialized JSON body
     */
    async patch(path, body, options = {}) {
        if (typeof body !== 'string') {
            throw new Error(`g8egHttpClient.patch: body must be a pre-serialized JSON string, got ${typeof body}`);
        }
        return this.request('PATCH', path, { ...options, body });
    }

    /**
     * Convenience: POST request with pre-serialized JSON body
     */
    async post(path, body, options = {}) {
        if (typeof body !== 'string') {
            throw new Error(`g8egHttpClient.post: body must be a pre-serialized JSON string, got ${typeof body}`);
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

export { g8egHttpClient, g8egHttpError };
