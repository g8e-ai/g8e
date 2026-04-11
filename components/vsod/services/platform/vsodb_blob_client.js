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
import { HTTP_INTERNAL_AUTH_HEADER } from '../../constants/headers.js';

const BLOB_ATTACHMENT_NAMESPACE_PREFIX = 'att';

/**
 * HTTP client for the vsodb /blob/ store.
 *
 * Attachments are stored as raw binary
 * in the vsodb SQLite blob table, keyed by namespace + id.
 *
 * Namespace per investigation: att:{investigationId}
 * Blob id: {attachmentId}
 * Object key (stored in KV record): att:{investigationId}/{attachmentId}
 */
export class VSODBBlobClient {
    /**
     * @param {object} [config]
     * @param {string} [config.baseUrl] - vsodb HTTP base URL
     * @param {string} [config.internalAuthToken] - Shared secret for VSODB authentication
     */
    constructor({ baseUrl, internalAuthToken = null } = {}) {
        this._baseUrl = baseUrl;
        this.internalAuthToken = internalAuthToken;
    }

    _headers(extraHeaders = {}) {
        const headers = { ...extraHeaders };
        if (this.internalAuthToken) {
            headers[HTTP_INTERNAL_AUTH_HEADER] = this.internalAuthToken;
        }
        return headers;
    }

    /**
     * Build the namespace used for a given investigation's attachments.
     * @param {string} investigationId
     * @returns {string}
     */
    namespace(investigationId) {
        return `${BLOB_ATTACHMENT_NAMESPACE_PREFIX}:${investigationId}`;
    }

    /**
     * Build the canonical object key stored in the KV attachment record.
     *
     * @param {string} investigationId
     * @param {string} attachmentId
     * @returns {string}
     */
    objectKey(investigationId, attachmentId) {
        return `${BLOB_ATTACHMENT_NAMESPACE_PREFIX}:${investigationId}/${attachmentId}`;
    }

    /**
     * Store a binary attachment from a base64 string.
     *
     * @param {string} investigationId
     * @param {string} attachmentId
     * @param {string} base64Data     - Raw base64 (no data URI prefix)
     * @param {string} contentType
     * @returns {Promise<string>}     - The object key
     */
    async putAttachment(investigationId, attachmentId, base64Data, contentType) {
        if (typeof base64Data !== 'string') {
            throw new Error('VSODBBlobClient.putAttachment: base64Data must be a string');
        }

        const ns     = this.namespace(investigationId);
        const buffer = Buffer.from(base64Data, 'base64');
        const url    = `${this._baseUrl}/blob/${encodeURIComponent(ns)}/${encodeURIComponent(attachmentId)}`;

        const res = await fetch(url, {
            method:  'PUT',
            headers: this._headers({ 'Content-Type': contentType }),
            body:    buffer,
        });

        if (!res.ok) {
            const text = await res.text().catch(() => '');
            throw new Error(`VSODBBlobClient.putAttachment failed: ${res.status} ${text}`);
        }

        const key = this.objectKey(investigationId, attachmentId);
        logger.info('[BLOB] Stored attachment', { key, size: buffer.length, contentType });
        return key;
    }

    /**
     * Retrieve an attachment as a base64 string.
     *
     * @param {string} objectKey - canonical object key from putAttachment
     * @returns {Promise<string>} base64-encoded content
     * @throws if the object does not exist or retrieval fails
     */
    async getAttachment(objectKey) {
        const { ns, id } = this._parseObjectKey(objectKey);
        const url = `${this._baseUrl}/blob/${encodeURIComponent(ns)}/${encodeURIComponent(id)}`;

        const res = await fetch(url, {
            headers: this._headers()
        });
        if (res.status === 404) {
            throw new Error(`VSODBBlobClient.getAttachment: not found: ${objectKey}`);
        }
        if (!res.ok) {
            const text = await res.text().catch(() => '');
            throw new Error(`VSODBBlobClient.getAttachment failed: ${res.status} ${text}`);
        }

        const buf = Buffer.from(await res.arrayBuffer());
        logger.info('[BLOB] Retrieved attachment', { objectKey, size: buf.length });
        return buf.toString('base64');
    }

    /**
     * Delete a single attachment object.
     *
     * @param {string} objectKey
     */
    async deleteAttachment(objectKey) {
        const { ns, id } = this._parseObjectKey(objectKey);
        const url = `${this._baseUrl}/blob/${encodeURIComponent(ns)}/${encodeURIComponent(id)}`;

        const res = await fetch(url, { 
            method: 'DELETE',
            headers: this._headers()
        });
        if (!res.ok && res.status !== 404) {
            const text = await res.text().catch(() => '');
            throw new Error(`VSODBBlobClient.deleteAttachment failed: ${res.status} ${text}`);
        }
        logger.info('[BLOB] Deleted attachment', { objectKey });
    }

    /**
     * Delete all attachment blobs for an investigation by deleting the entire namespace.
     *
     * @param {string} investigationId
     * @returns {Promise<number>} count of deleted blobs
     */
    async deleteAttachmentsForInvestigation(investigationId) {
        const ns  = this.namespace(investigationId);
        const url = `${this._baseUrl}/blob/${encodeURIComponent(ns)}`;

        const res = await fetch(url, { 
            method: 'DELETE',
            headers: this._headers()
        });
        if (!res.ok) {
            const text = await res.text().catch(() => '');
            throw new Error(`VSODBBlobClient.deleteAttachmentsForInvestigation failed: ${res.status} ${text}`);
        }

        const body = await res.json();
        const count = typeof body.deleted === 'number' ? body.deleted : 0;
        logger.info('[BLOB] Deleted attachments for investigation', { investigationId, count });
        return count;
    }

    /**
     * Parse a canonical object key back into namespace and blob id.
     * Format: att:{investigationId}/{attachmentId}
     *
     * @private
     */
    _parseObjectKey(objectKey) {
        const slash = objectKey.lastIndexOf('/');
        if (slash === -1) {
            throw new Error(`VSODBBlobClient: malformed object key: ${objectKey}`);
        }
        return {
            ns: objectKey.slice(0, slash),
            id: objectKey.slice(slash + 1),
        };
    }
}
