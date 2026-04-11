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
 * VSODBDocumentClient — VSODB Document Store HTTP client.
 * 
 * Purpose-built client for VSODB document store operations (/db/...).
 * Mirrors g8ee's DBClient (components/g8ee/app/db/client.py) in scope
 * and responsibility — document CRUD only, no KV, no pub/sub.
 * 
 * VSODB endpoints:
 *   GET    /db/{collection}/{id}       - get document
 *   PUT    /db/{collection}/{id}       - set (create/replace) document
 *   PATCH  /db/{collection}/{id}       - update (merge) document
 *   DELETE /db/{collection}/{id}       - delete document
 *   POST   /db/{collection}/_query     - query documents
 */

import { randomUUID } from 'crypto';
import { VSODBHttpClient, VSODBHttpError } from './vsodb_http_client.js';
import { VSOBaseModel } from '../../models/base.js';
import { logger } from '../../utils/logger.js';
import { nowISOString } from '../../utils/timestamp.js';

const LOG_PREFIX = '[VSODB-DOC]';
const CLIENT_TERMINATED_ERROR = 'Client terminated';

const FieldOp = {
    INCREMENT: 'increment',
    ARRAY_UNION: 'arrayUnion',
    ARRAY_REMOVE: 'arrayRemove',
    DELETE: 'delete',
    SERVER_TIMESTAMP: '__SERVER_TIMESTAMP__',
};

class VSODBFieldValue {
    static serverTimestamp() {
        return nowISOString();
    }

    static increment(n) {
        return { __op: FieldOp.INCREMENT, value: n };
    }

    static arrayUnion(...elements) {
        const values = elements.length === 1 && Array.isArray(elements[0]) ? elements[0] : elements;
        return { __op: FieldOp.ARRAY_UNION, values };
    }

    static arrayRemove(...elements) {
        const values = elements.length === 1 && Array.isArray(elements[0]) ? elements[0] : elements;
        return { __op: FieldOp.ARRAY_REMOVE, values };
    }

    static delete() {
        return { __op: FieldOp.DELETE };
    }
}

function isFieldValueOp(val) {
    return val && typeof val === 'object' && '__op' in val;
}

class VSODBDocumentClient {
    static FieldValue = VSODBFieldValue;

    /**
     * @param {object} config
     * @param {string} config.listenUrl - Base URL of VSODB (e.g. $G8E_INTERNAL_HTTP_URL)
     * @param {string} [config.internalAuthToken] - Shared secret for VSODB authentication
     * @param {string} [config.caCertPath] - Path to CA certificate for TLS verification
     */
    constructor({ listenUrl, internalAuthToken = null, caCertPath = null } = {}) {
        this._http = new VSODBHttpClient({ listenUrl, component: 'VSODB-DOC', internalAuthToken, caCertPath });
    }

    get FieldValue() {
        return VSODBDocumentClient.FieldValue;
    }

    // =========================================================================
    // Document CRUD
    // =========================================================================

    async getDocument(collection, documentId) {
        if (this._http.isTerminated()) return { success: false, data: null, error: CLIENT_TERMINATED_ERROR };

        try {
            const data = await this._http.get(`/db/${collection}/${documentId}`);
            return { success: true, data, error: null };
        } catch (error) {
            if (error instanceof VSODBHttpError && error.status === 404) {
                return { success: true, data: null, error: 'Document not found' };
            }
            logger.error(`${LOG_PREFIX} getDocument failed: ${error.message}`);
            return { success: false, data: null, error: error.message };
        }
    }

    async setDocument(collection, documentId, data) {
        if (this._http.isTerminated()) return { success: false, error: CLIENT_TERMINATED_ERROR };

        try {
            const flat = data instanceof VSOBaseModel ? data.forDB() : data;
            const resolved = this._resolveFieldValues(flat);
            await this._http.put(`/db/${collection}/${documentId}`, JSON.stringify(resolved));
            return { success: true, error: null };
        } catch (error) {
            logger.error(`${LOG_PREFIX} setDocument failed: ${error.message}`);
            return { success: false, error: error.message };
        }
    }

    async queryDocuments(collection, filters = [], limit = null) {
        if (this._http.isTerminated()) return { success: false, data: [], error: CLIENT_TERMINATED_ERROR };

        try {
            const body = { filters: this._convertFilters(filters) };
            if (limit) body.limit = limit;

            const data = await this._http.post(`/db/${collection}/_query`, JSON.stringify(body));
            return { success: true, data: Array.isArray(data) ? data : [], error: null };
        } catch (error) {
            logger.error(`${LOG_PREFIX} queryDocuments failed: ${error.message}`);
            return { success: false, data: [], error: error.message };
        }
    }

    async queryDocumentsOrdered(collection, filters = [], orderBy = null, limit = null) {
        if (this._http.isTerminated()) return { success: false, data: [], error: CLIENT_TERMINATED_ERROR };

        try {
            const body = { filters: this._convertFilters(filters) };
            if (orderBy) {
                const dir = (orderBy.direction || 'asc').toUpperCase();
                body.order_by = `${orderBy.field} ${dir === 'DESC' ? 'DESC' : 'ASC'}`;
            }
            if (limit) body.limit = limit;

            const data = await this._http.post(`/db/${collection}/_query`, JSON.stringify(body));
            return { success: true, data: Array.isArray(data) ? data : [], error: null };
        } catch (error) {
            logger.error(`${LOG_PREFIX} queryDocumentsOrdered failed: ${error.message}`);
            return { success: false, data: [], error: error.message };
        }
    }

    async updateDocument(collection, documentId, updates) {
        if (this._http.isTerminated()) return { success: false, data: null, error: CLIENT_TERMINATED_ERROR };

        try {
            const flat = updates instanceof VSOBaseModel ? updates.forDB() : updates;
            const resolved = this._resolveFieldValues(flat);
            const data = await this._http.patch(`/db/${collection}/${documentId}`, JSON.stringify(resolved));
            return { success: true, data, error: null };
        } catch (error) {
            logger.error(`${LOG_PREFIX} updateDocument failed: ${error.message}`);
            return { success: false, data: null, error: error.message };
        }
    }

    async createDocument(collection, data) {
        if (this._http.isTerminated()) return { success: false, id: null, error: CLIENT_TERMINATED_ERROR };

        try {
            const id = data.id || randomUUID();
            const resolved = this._resolveFieldValues(data);
            await this._http.put(`/db/${collection}/${id}`, JSON.stringify(resolved));
            return { success: true, id, error: null };
        } catch (error) {
            logger.error(`${LOG_PREFIX} createDocument failed: ${error.message}`);
            return { success: false, id: null, error: error.message };
        }
    }

    async deleteDocument(collection, documentId) {
        if (this._http.isTerminated()) return { success: false, notFound: false, error: CLIENT_TERMINATED_ERROR };

        try {
            await this._http.delete(`/db/${collection}/${documentId}`);
            return { success: true, notFound: false, error: null };
        } catch (error) {
            const notFound = error instanceof VSODBHttpError && error.status === 404;
            if (!notFound) {
                logger.error(`${LOG_PREFIX} deleteDocument failed: ${error.message}`);
            }
            return { success: false, notFound, error: error.message };
        }
    }

    async runTransaction(collection, documentId, updateFn) {
        if (this._http.isTerminated()) return { success: false, data: null, error: CLIENT_TERMINATED_ERROR };

        try {
            const { data: existingData } = await this.getDocument(collection, documentId);
            const newData = await updateFn(existingData, !!existingData);

            if (newData !== null && newData !== undefined) {
                await this.setDocument(collection, documentId, newData);
            }

            return { success: true, data: newData, error: null };
        } catch (error) {
            logger.error(`${LOG_PREFIX} runTransaction failed: ${error.message}`);
            return { success: false, data: null, error: error.message };
        }
    }

    // =========================================================================
    // Lifecycle
    // =========================================================================

    async terminate() {
        this._http.terminate();
        logger.info(`${LOG_PREFIX} Client terminated`);
    }

    isTerminated() {
        return this._http.isTerminated();
    }

    async waitForReady(maxRetries, delayMs) {
        return this._http.waitForReady(maxRetries, delayMs);
    }

    // =========================================================================
    // Internal helpers
    // =========================================================================

    _resolveFieldValues(data) {
        if (!data || typeof data !== 'object') return data;
        const resolved = {};
        const ts = nowISOString();

        for (const [k, v] of Object.entries(data)) {
            if (v === FieldOp.SERVER_TIMESTAMP) {
                resolved[k] = ts;
            } else if (isFieldValueOp(v)) {
                if (v.__op === FieldOp.DELETE) {
                    resolved[k] = null;
                } else if (v.__op === FieldOp.INCREMENT) {
                    resolved[k] = v;
                } else if (v.__op === FieldOp.ARRAY_UNION || v.__op === FieldOp.ARRAY_REMOVE) {
                    resolved[k] = v;
                }
            } else if (typeof v === 'object' && v !== null && typeof v.toISOString === 'function') {
                resolved[k] = v.toISOString();
            } else {
                resolved[k] = v;
            }
        }
        return resolved;
    }

    _convertFilters(filters) {
        return filters.map(f => ({
            field: f.field,
            op: f.operator || '==',
            value: f.value,
        }));
    }
}

export { VSODBDocumentClient, VSODBFieldValue };
