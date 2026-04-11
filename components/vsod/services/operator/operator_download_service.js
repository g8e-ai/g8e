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

import { logger } from '../../utils/logger.js';
import { PLATFORMS, OPERATOR_BINARY_BLOB_NAMESPACE } from '../../constants/service_config.js';
import { VSODB_HTTP_TIMEOUT_MS } from '../../constants/http_client.js';
import { HTTP_INTERNAL_AUTH_HEADER } from '../../constants/headers.js';

/**
 * OperatorDownloadService
 *
 * Owns all operator binary retrieval from VSODB's blob store.
 * VSOD is stateless — no local disk cache, no KV store involvement.
 *
 * Architecture:
 *   VSOD → GET  https://vsodb/blob/{ns}/{os}-{arch}      → VSODB blob store
 *   VSOD → GET  https://vsodb/blob/{ns}/{os}-{arch}/meta  → VSODB blob metadata (availability check)
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
        return `${this._listenUrl}/blob/${OPERATOR_BINARY_BLOB_NAMESPACE}/${os}-${arch}`;
    }

    _headers() {
        const headers = {};
        if (this._internalAuthToken) {
            headers[HTTP_INTERNAL_AUTH_HEADER] = this._internalAuthToken;
        }
        return headers;
    }

    /**
     * Fetch a binary from VSODB for the given platform.
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
            const timeoutId = setTimeout(() => controller.abort(), VSODB_HTTP_TIMEOUT_MS);
            let res;
            try {
                res = await fetch(url, { signal: controller.signal, headers: this._headers() });
            } finally {
                clearTimeout(timeoutId);
            }

            if (!res.ok) {
                logger.error(`[OPERATOR-DOWNLOAD-SERVICE] VSODB blob store returned ${res.status} for platform: ${platform}`, { url });
                throw new Error(`Operator binary not available for platform: ${platform}`);
            }

            const arrayBuf = await res.arrayBuffer();
            const buffer = Buffer.from(arrayBuf);
            logger.info(`[OPERATOR-DOWNLOAD-SERVICE] Fetched ${platform} binary from VSODB blob store`, {
                size_mb: (buffer.length / 1024 / 1024).toFixed(2),
            });
            return buffer;
        } catch (error) {
            if (error.message.startsWith('Operator binary not available')) {
                throw error;
            }
            logger.error(`[OPERATOR-DOWNLOAD-SERVICE] Failed to fetch binary from VSODB blob store`, { platform, error: error.message });
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
            const timeoutId = setTimeout(() => controller.abort(), VSODB_HTTP_TIMEOUT_MS);
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
