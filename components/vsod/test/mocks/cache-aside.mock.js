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
 * MockCacheAsideService — unit-test stand-in for CacheAsideService.
 *
 * Mirrors the real CacheAsideService contract exactly:
 *   createDocument, updateDocument, deleteDocument,
 *   getDocument, getQueryResult, setQueryResult
 *
 * Backed by two in-memory stores:
 *   _db  — simulates VSODB document store (authoritative)
 *   _kv  — simulates VSODB KV cache
 *
 * Every public method is a vi.fn() spy so callers can assert on it.
 * The implementations faithfully reproduce the real cache-aside contract:
 *   CREATE: DB write first → KV set_json (cache warmed)
 *   UPDATE: DB write first → KV del (cache INVALIDATED — not written through)
 *   READ:   KV get → miss: DB read + KV set_json
 *   DELETE: DB delete → KV del; notFound → KV del + success:false/notFound:true
 *
 * Factory:
 *   createMockCacheAside(initialDocs?)
 *     initialDocs: { [collection]: { [id]: data } }
 *
 * Test helpers (not part of the real API):
 *   _db        - Map of collection → Map of id → data
 *   _kv        - Map of key → serialised string
 *   _reset()   - clear both stores and reset all spy call counts
 *   _seedDoc(collection, id, data)  - plant a doc in the DB store only
 *   _seedKV(key, value)             - plant a raw value in KV
 *   _getKV(key)                     - read a raw KV value
 */

import { vi } from 'vitest';
import { KVKey, KVKeyPrefix } from '@vsod/constants/kv_keys.js';
import { Collections } from '@vsod/constants/collections.js';
import crypto from 'crypto';

export class MockCacheAsideService {
    constructor(initialDocs = {}) {
        this._db = new Map();
        this._kv = new Map();

        for (const [collection, docs] of Object.entries(initialDocs)) {
            const colMap = new Map();
            for (const [id, data] of Object.entries(docs)) {
                colMap.set(id, JSON.parse(JSON.stringify(data)));
            }
            this._db.set(collection, colMap);
        }

        this.createDocument = vi.fn(this._createDocument.bind(this));
        this.updateDocument = vi.fn(this._updateDocument.bind(this));
        this.deleteDocument = vi.fn(this._deleteDocument.bind(this));
        this.getDocument    = vi.fn(this._getDocument.bind(this));
        this.getQueryResult = vi.fn(this._getQueryResult.bind(this));
        this.setQueryResult = vi.fn(this._setQueryResult.bind(this));
        this.queryDocuments = vi.fn(this._queryDocuments.bind(this));
        this.kvGetJson      = vi.fn(this._kvGetJson.bind(this));
        this.kvSetJson      = vi.fn(this._kvSetJson.bind(this));
        this.kvDel          = vi.fn(this._kvDel.bind(this));
        this.kvGet          = vi.fn(this._kvGet.bind(this));
        this.kvSet          = vi.fn(this._kvSet.bind(this));
        this.kvExists       = vi.fn(this._kvExists.bind(this));
        this.kvExpire       = vi.fn(this._kvExpire.bind(this));
        this.kvTtl          = vi.fn(this._kvTtl.bind(this));
        this.kvIncr         = vi.fn(this._kvIncr.bind(this));
        this.kvDecr         = vi.fn(this._kvDecr.bind(this));
        this.kvSadd         = vi.fn(this._kvSadd.bind(this));
        this.kvSrem         = vi.fn(this._kvSrem.bind(this));
        this.kvSmembers     = vi.fn(this._kvSmembers.bind(this));
        this.kvScard        = vi.fn(this._kvScard.bind(this));
        this.kvRpush        = vi.fn(this._kvRpush.bind(this));
        this.kvLrange       = vi.fn(this._kvLrange.bind(this));
        this.kvZadd         = vi.fn(this._kvZadd.bind(this));
        this.kvZrem         = vi.fn(this._kvZrem.bind(this));
        this.kvZrange       = vi.fn(this._kvZrange.bind(this));
        this.kvZrevrange    = vi.fn(this._kvZrevrange.bind(this));
        this.kvScan         = vi.fn(this._kvScan.bind(this));
        this.kvSetex        = vi.fn(this._kvSetex.bind(this));
        this.evictDocument  = vi.fn(this._evictDocument.bind(this));
    }

    _colMap(collection) {
        if (!this._db.has(collection)) this._db.set(collection, new Map());
        return this._db.get(collection);
    }

    _makeKey(collection, documentId) {
        if (collection === Collections.WEB_SESSIONS)      return KVKey.webSessionKey(documentId);
        if (collection === Collections.OPERATOR_SESSIONS) return KVKey.operatorSessionKey(documentId);
        return KVKey.doc(collection, documentId);
    }

    async _createDocument(collection, documentId, data, ttl = null) {
        const col = this._colMap(collection);
        const flat = JSON.parse(JSON.stringify(data));
        col.set(documentId, flat);
        this._kv.set(this._makeKey(collection, documentId), JSON.stringify(flat));
        await this._invalidateQueryCache(collection);
        return { success: true, documentId, cached: true };
    }

    async _updateDocument(collection, documentId, data, merge = true) {
        const col = this._colMap(collection);
        const existing = col.get(documentId) || {};
        const updated = merge ? this._deepMerge(existing, data) : JSON.parse(JSON.stringify(data));
        col.set(documentId, updated);
        this._kv.delete(this._makeKey(collection, documentId));
        await this._invalidateQueryCache(collection);
        return { success: true, documentId };
    }

    _deepMerge(target, patch) {
        const result = JSON.parse(JSON.stringify(target));
        for (const [key, value] of Object.entries(patch)) {
            if (value !== null && typeof value === 'object' && !Array.isArray(value) &&
                result[key] !== null && typeof result[key] === 'object' && !Array.isArray(result[key])) {
                result[key] = this._deepMerge(result[key], value);
            } else {
                result[key] = JSON.parse(JSON.stringify(value));
            }
        }
        return result;
    }

    async _deleteDocument(collection, documentId) {
        const col = this._colMap(collection);
        const kvKey = this._makeKey(collection, documentId);
        this._kv.delete(kvKey);
        await this._invalidateQueryCache(collection);

        if (!col.has(documentId)) {
            return { success: false, notFound: true, error: 'document not found' };
        }

        col.delete(documentId);
        return { success: true, notFound: false, documentId };
    }

    async _getDocument(collection, documentId) {
        const kvKey = this._makeKey(collection, documentId);
        const cached = this._kv.get(kvKey);
        if (cached !== undefined) {
            return JSON.parse(cached);
        }

        const col = this._colMap(collection);
        if (!col.has(documentId)) {
            return null;
        }

        const data = col.get(documentId);
        this._kv.set(kvKey, JSON.stringify(data));
        return JSON.parse(JSON.stringify(data));
    }

    async _getQueryResult(collection, queryParams) {
        const queryStr = JSON.stringify(queryParams, Object.keys(queryParams).sort());
        const filterHash = crypto.createHash('md5').update(queryStr).digest('hex');
        const key = KVKey.query(collection, filterHash);
        const cached = this._kv.get(key);
        return cached !== undefined ? JSON.parse(cached) : null;
    }

    async _setQueryResult(collection, queryParams, results, ttl = 300) {
        if (!Array.isArray(results)) {
            throw new Error('CacheAsideService.setQueryResult requires an array of results');
        }
        const queryStr = JSON.stringify(queryParams, Object.keys(queryParams).sort());
        const filterHash = crypto.createHash('md5').update(queryStr).digest('hex');
        const key = KVKey.query(collection, filterHash);
        this._kv.set(key, JSON.stringify(results));
        return true;
    }

    async _queryDocuments(collection, filters = [], limit = null) {
        const col = this._colMap(collection);
        let results = Array.from(col.values()).map(v => JSON.parse(JSON.stringify(v)));
        if (limit !== null) {
            results = results.slice(0, limit);
        }
        return results;
    }

    async _kvGetJson(key) {
        const raw = this._kv.get(key);
        return raw !== undefined ? JSON.parse(raw) : null;
    }

    async _kvSetJson(key, value, ttl = null) {
        this._kv.set(key, JSON.stringify(value));
        return true;
    }

    async _kvDel(...keys) {
        for (const key of keys) {
            this._kv.delete(key);
        }
        return keys.length;
    }

    async _kvGet(key) {
        return this._kv.get(key) ?? null;
    }

    async _kvSet(key, value) {
        this._kv.set(key, value);
        return true;
    }

    async _kvExists(key) {
        return this._kv.has(key) ? 1 : 0;
    }

    async _kvSetex(key, ttl, value) {
        this._kv.set(key, value);
        return 'OK';
    }

    async _kvExpire(key, seconds) {
        return 1;
    }

    async _kvTtl(key) {
        return 3600;
    }

    async _kvIncr(key) {
        const val = parseInt(this._kv.get(key) || '0', 10);
        const newVal = val + 1;
        this._kv.set(key, newVal.toString());
        return newVal;
    }

    async _kvDecr(key) {
        const val = parseInt(this._kv.get(key) || '0', 10);
        const newVal = val - 1;
        this._kv.set(key, newVal.toString());
        return newVal;
    }

    async _kvSadd(key, ...members) {
        let set = this._kv.get(key);
        if (!set) {
            set = new Set();
            this._kv.set(key, set);
        }
        if (!(set instanceof Set)) {
            // Handle if it was stored as string previously
            if (typeof set === 'string') {
                await this._kvDel(key);
                set = new Set();
                this._kv.set(key, set);
            } else {
                throw new Error('WRONGTYPE');
            }
        }
        let added = 0;
        for (const m of members) {
            if (!set.has(m)) {
                set.add(m);
                added++;
            }
        }
        return added;
    }

    async _kvSrem(key, ...members) {
        const set = this._kv.get(key);
        if (!set) return 0;
        if (!(set instanceof Set)) throw new Error('WRONGTYPE');
        let removed = 0;
        for (const m of members) {
            if (set.delete(m)) removed++;
        }
        return removed;
    }

    async _kvSmembers(key) {
        const set = this._kv.get(key);
        if (!set) return [];
        if (!(set instanceof Set)) throw new Error('WRONGTYPE');
        return Array.from(set);
    }

    async _kvScard(key) {
        const set = this._kv.get(key);
        if (!set) return 0;
        if (!(set instanceof Set)) throw new Error('WRONGTYPE');
        return set.size;
    }

    async _kvRpush(key, ...values) {
        let list = this._kv.get(key);
        if (!list) {
            list = [];
            this._kv.set(key, list);
        }
        if (!Array.isArray(list)) throw new Error('WRONGTYPE');
        list.push(...values);
        return list.length;
    }

    async _kvLrange(key, start, stop) {
        const list = this._kv.get(key);
        if (!list) return [];
        if (!Array.isArray(list)) throw new Error('WRONGTYPE');
        // Simplified range handling
        const s = start < 0 ? list.length + start : start;
        const e = stop < 0 ? list.length + stop : stop;
        return list.slice(s, e + 1);
    }

    async _kvZadd(key, score, member) {
        let map = this._kv.get(key);
        if (!map) {
            map = new Map();
            this._kv.set(key, map);
        }
        if (!(map instanceof Map)) throw new Error('WRONGTYPE');
        map.set(member, score);
        return 1;
    }

    async _kvZrem(key, ...members) {
        const map = this._kv.get(key);
        if (!map) return 0;
        if (!(map instanceof Map)) throw new Error('WRONGTYPE');
        let removed = 0;
        for (const m of members) {
            if (map.delete(m)) removed++;
        }
        return removed;
    }

    async _kvZrange(key, start, stop) {
        const map = this._kv.get(key);
        if (!map) return [];
        if (!(map instanceof Map)) throw new Error('WRONGTYPE');
        return Array.from(map.entries())
            .sort((a, b) => a[1] - b[1])
            .map(e => e[0]);
    }

    async _kvZrevrange(key, start, stop) {
        const map = this._kv.get(key);
        if (!map) return [];
        if (!(map instanceof Map)) throw new Error('WRONGTYPE');
        return Array.from(map.entries())
            .sort((a, b) => b[1] - a[1])
            .map(e => e[0]);
    }

    async _kvScan(cursor, ...args) {
        // Find 'MATCH' pattern if present
        let pattern = '*';
        const matchIdx = args.indexOf('MATCH');
        if (matchIdx !== -1 && args[matchIdx + 1]) {
            pattern = args[matchIdx + 1];
        }

        const regex = new RegExp('^' + pattern.replace(/\*/g, '.*') + '$');
        const keys = Array.from(this._kv.keys()).filter(k => regex.test(k));
        return ['0', keys];
    }

    async _evictDocument(collection, documentId) {
        const key = this._makeKey(collection, documentId);
        this._kv.delete(key);
    }

    async _invalidateQueryCache(collection) {
        const pattern = KVKey.query(collection, '*');
        const regex = new RegExp('^' + pattern.replace(/\*/g, '.*') + '$');
        for (const key of this._kv.keys()) {
            if (regex.test(key)) {
                this._kv.delete(key);
            }
        }
    }

    _reset() {
        this._db.clear();
        this._kv.clear();
        this.createDocument.mockClear();
        this.updateDocument.mockClear();
        this.deleteDocument.mockClear();
        this.getDocument.mockClear();
        this.evictDocument.mockClear();
        this.getQueryResult.mockClear();
        this.setQueryResult.mockClear();
        this.queryDocuments.mockClear();
        this.kvGetJson.mockClear();
        this.kvSetJson.mockClear();
        this.kvDel.mockClear();
        this.kvGet.mockClear();
        this.kvSet.mockClear();
        this.kvSetex.mockClear();
        this.kvExists.mockClear();
        this.kvExpire.mockClear();
        this.kvTtl.mockClear();
        this.kvIncr.mockClear();
        this.kvDecr.mockClear();
        this.kvSadd.mockClear();
        this.kvSrem.mockClear();
        this.kvSmembers.mockClear();
        this.kvScard.mockClear();
        this.kvRpush.mockClear();
        this.kvLrange.mockClear();
        this.kvZadd.mockClear();
        this.kvZrem.mockClear();
        this.kvZrange.mockClear();
        this.kvZrevrange.mockClear();
        this.kvScan.mockClear();
    }

    _seedDoc(collection, documentId, data) {
        this._colMap(collection).set(documentId, JSON.parse(JSON.stringify(data)));
    }

    _seedKV(key, value) {
        this._kv.set(key, typeof value === 'string' ? value : JSON.stringify(value));
    }

    _getKV(key) {
        const raw = this._kv.get(key);
        return raw !== undefined ? JSON.parse(raw) : undefined;
    }
}

/**
 * Create a MockCacheAsideService instance.
 *
 * @param {Object} [initialDocs] - Optional seed data: { collection: { id: data } }
 * @returns {MockCacheAsideService}
 */
export function createMockCacheAside(initialDocs = {}) {
    return new MockCacheAsideService(initialDocs);
}
