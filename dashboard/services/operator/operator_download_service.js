// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { logger } from '../../utils/logger.js';
import { PLATFORMS, OPERATOR_BINARY_BLOB_NAMESPACE } from '../../constants/service_config.js';
import { g8edB_HTTP_TIMEOUT_MS } from '../../constants/http_client.js';
import { ApiPaths } from '../../constants/api_paths.js';
import { HTTP_INTERNAL_AUTH_HEADER } from '../../constants/headers.js';

/**
 * OperatorDownloadService
 *
 * Owns all operator binary retrieval from g8edB's blob store.
 * g8ed is stateless — no local disk cache, no KV store involvement.
 *
 * Architecture:
 *   g8ed → GET  https://g8edb/blob/{ns}/{os}-{arch}      → g8edB blob store
 *   g8ed → GET  https://g8edb/blob/{ns}/{os}-{arch}/meta  → g8edB blob metadata (availability check)
 */
class OperatorDownloadService {
    constructor(listenUrl, internalAuthToken) {
        if (!listenUrl) {
            throw new Error('OperatorDownloadService requires listenUrl');
        }
        this._listenUrl = listenUrl.replace(/\/$/, '');
        this._internalAuthToken = internalAuthToken || null;
    }

    _blobUrl(os, arch) {
        return `${this._listenUrl}${ApiPaths.blobs.object(OPERATOR_BINARY_BLOB_NAMESPACE, `${os}-${arch}`)}`;
    }

    _headers() {
        const headers = {};
        if (this._internalAuthToken) {
            headers[HTTP_INTERNAL_AUTH_HEADER] = this._internalAuthToken;
        }
        return headers;
    }

    /**
     * Fetch a binary from g8edB for the given platform.
     *
     * @param {string} os
     * @param {string} arch
     * @returns {Promise<Buffer>}
     * @throws {Error} 'Operator binary not available for platform: {os}/{arch}' on any failure
     */
    async getBinary(os, arch) {
        const platform = `${os}/${arch}`;
        const url = this._blobUrl(os, arch);

        try {
            const controller = new AbortController();
            const timeoutId = setTimeout(() => controller.abort(), g8edB_HTTP_TIMEOUT_MS);
            let res;
            try {
                res = await fetch(url, { signal: controller.signal, headers: this._headers() });
            } finally {
                clearTimeout(timeoutId);
            }

            if (!res.ok) {
                logger.error(`[OPERATOR-DOWNLOAD-SERVICE] g8edB blob store returned ${res.status} for platform: ${platform}`, { url });
                throw new Error(`Operator binary not available for platform: ${platform}`);
            }

            const arrayBuf = await res.arrayBuffer();
            const buffer = Buffer.from(arrayBuf);
            logger.info(`[OPERATOR-DOWNLOAD-SERVICE] Fetched ${platform} binary from g8edB blob store`, {
                size_mb: (buffer.length / 1024 / 1024).toFixed(2),
            });
            return buffer;
        } catch (error) {
            if (error.message.startsWith('Operator binary not available')) {
                throw error;
            }
            logger.error(`[OPERATOR-DOWNLOAD-SERVICE] Failed to fetch binary from g8edB blob store`, { platform, error: error.message });
            throw new Error(`Operator binary not available for platform: ${platform}`);
        }
    }

    /**
     * Check whether a binary is available for a given platform without downloading it.
     *
     * @param {string} os
     * @param {string} arch
     * @returns {Promise<boolean>}
     */
    async hasBinary(os, arch) {
        const url = `${this._blobUrl(os, arch)}/meta`;
        try {
            const controller = new AbortController();
            const timeoutId = setTimeout(() => controller.abort(), g8edB_HTTP_TIMEOUT_MS);
            let res;
            try {
                res = await fetch(url, { signal: controller.signal, headers: this._headers() });
            } finally {
                clearTimeout(timeoutId);
            }
            return res.ok;
        } catch {
            return false;
        }
    }

    /**
     * Return availability status for every platform defined in PLATFORMS.
     *
     * @returns {Promise<Record<string, { available: boolean }>>}
     */
    async getPlatformAvailability() {
        const info = {};
        for (const { os, arch } of PLATFORMS) {
            const platformKey = `${os}/${arch}`;
            const available = await this.hasBinary(os, arch);
            info[platformKey] = { available };
        }
        return info;
    }
}

export { OperatorDownloadService };
