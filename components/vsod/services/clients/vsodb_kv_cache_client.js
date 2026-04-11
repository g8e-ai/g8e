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
 * KVCacheClient — VSODB KV Store HTTP client.
 *
 * Purpose-built client for VSODB KV store operations (/kv/...).
 * Handles all key-value, hash, list, set, sorted set, and stream operations.
 * No document store, no pub/sub — those are separate clients.
 *
 * VSODB endpoints:
 *   GET    /kv/{key}           - get value
 *   PUT    /kv/{key}           - set value (with optional TTL)
 *   DELETE /kv/{key}           - delete key
 *   POST   /kv/_keys           - list keys by pattern
 *   PUT    /kv/{key}/_expire   - set TTL
 *   GET    /kv/{key}/_ttl      - get remaining TTL
 *   POST   /kv/_scan           - cursor-based scan
 *
 * CONCURRENCY WARNING
 * VSODB exposes no atomic increment, compare-and-swap, or server-side scripting.
 * All compound operations in this client (incr, decr, set-NX, hset, hdel, rpush,
 * lpush, ltrim, sadd, srem, zadd, zrem, xadd) are implemented as read-modify-write
 * cycles over HTTP and are NOT atomic under concurrent access.
 *
 * Callers that require mutual exclusion must use the distributed lock pattern
 * (set key lockValue 'PX' ttlMs 'NX' with retry) and accept that the NX check
 * itself has a TOCTOU window. For use-count enforcement and similar counters,
 * over- or under-counts are possible under high concurrency; implement
 * compensating logic on failure paths rather than relying on exact counts.
 */

import { VSODBHttpClient } from './vsodb_http_client.js';
import { logger } from '../../utils/logger.js';
import { VSODB_KV_CLIENT_STATUS_READY, KV_SCAN_DEFAULT_COUNT } from '../../constants/http_client.js';
import { KVHash, KVList, KVSet, KVSortedSet, KVStream } from '../../models/kv_models.js';

class KVOperationError extends Error {
    constructor(operation, pattern, cause) {
        super(`KV ${operation} failed for pattern "${pattern}": ${cause.message}`);
        this.name = 'KVOperationError';
        this.operation = operation;
        this.pattern = pattern;
        this.cause = cause;
    }
}

class KVCacheClient {
    /**
     * @param {object} config
     * @param {string} config.listenUrl - Base URL of VSODB (e.g. $G8E_INTERNAL_HTTP_URL)
     * @param {string} [config.internalAuthToken] - Shared secret for VSODB authentication
     * @param {string} [config.caCertPath] - Path to CA certificate for TLS verification
     */
    constructor({ listenUrl, internalAuthToken = null, caCertPath = null } = {}) {
        this._http = new VSODBHttpClient({ listenUrl, component: 'VSODB-KV', internalAuthToken, caCertPath });
    }

    // =========================================================================
    // Core KV operations
    // =========================================================================

    async get(key) {
        try {
            const data = await this._http.get(`/kv/${encodeURIComponent(key)}`);
            return data.value !== undefined && data.value !== null ? data.value : null;
        } catch {
            return null;
        }
    }

    async set(key, value, ...args) {
        if (typeof value !== 'string') {
            throw new Error(`KVCacheClient.set: value must be a string, got ${typeof value}`);
        }
        let ttl = 0;
        let nx = false;
        for (let i = 0; i < args.length; i++) {
            if (typeof args[i] === 'string') {
                const flag = args[i].toUpperCase();
                if (flag === 'EX' && args[i + 1] !== undefined) {
                    ttl = parseInt(args[i + 1], 10);
                    i++;
                } else if (flag === 'PX' && args[i + 1] !== undefined) {
                    ttl = Math.ceil(parseInt(args[i + 1], 10) / 1000);
                    i++;
                } else if (flag === 'NX') {
                    nx = true;
                }
            }
        }
        if (nx) {
            const existing = await this.get(key);
            if (existing !== null) return null;
        }
        await this._http.put(`/kv/${encodeURIComponent(key)}`, JSON.stringify({ value, ttl }));
        return 'OK';
    }

    async del(...keys) {
        let count = 0;
        for (const key of keys.flat()) {
            try {
                await this._http.delete(`/kv/${encodeURIComponent(key)}`);
                count++;
            } catch {}
        }
        return count;
    }

    async keys(pattern) {
        const p = pattern || '*';
        try {
            const data = await this._http.post('/kv/_keys', JSON.stringify({ pattern: p }));
            return data.keys;
        } catch (error) {
            throw new KVOperationError('keys', p, error);
        }
    }

    async setex(key, seconds, value) {
        return this.set(key, value, 'EX', seconds);
    }

    _serialize(value) {
        return JSON.stringify(value);
    }

    _deserialize(raw) {
        return JSON.parse(raw);
    }

    async get_json(key) {
        const raw = await this.get(key);
        if (raw === null) return null;
        return this._deserialize(raw);
    }

    async set_json(key, value, ex = null) {
        const serialized = this._serialize(value);
        if (ex !== null) {
            return this.set(key, serialized, 'EX', ex);
        }
        return this.set(key, serialized);
    }

    async ping() {
        await this._http.post('/kv/_keys', JSON.stringify({ pattern: '__ping__', count: 1 }));
        return 'PONG';
    }

    get status() {
        return VSODB_KV_CLIENT_STATUS_READY;
    }

    async exists(key) {
        const val = await this.get(key);
        return val !== null ? 1 : 0;
    }

    async incr(key) {
        const existing = await this.get(key);
        const current = existing !== null ? parseInt(existing, 10) || 0 : 0;
        const next = current + 1;
        await this.set(key, next.toString());
        return next;
    }

    async decr(key) {
        const existing = await this.get(key);
        const current = existing !== null ? parseInt(existing, 10) || 0 : 0;
        const next = current - 1;
        await this.set(key, next.toString());
        return next;
    }

    async expire(key, seconds) {
        try {
            await this._http.put(`/kv/${encodeURIComponent(key)}/_expire`, JSON.stringify({ ttl: seconds }));
            return 1;
        } catch {
            return 0;
        }
    }

    async ttl(key) {
        try {
            const data = await this._http.get(`/kv/${encodeURIComponent(key)}/_ttl`);
            return data.ttl;
        } catch {
            return -2;
        }
    }

    // =========================================================================
    // Hash operations (stored as JSON in KV)
    // NOT atomic: concurrent writes to the same key will race.
    // =========================================================================

    async _getHash(key) {
        const raw = await this.get(key);
        if (!raw) return KVHash.fromRaw(null);
        return KVHash.fromRaw(this._deserialize(raw));
    }

    async hset(key, field, value) {
        const hash = await this._getHash(key);
        hash.set(field, value);
        await this.set(key, this._serialize(hash.forKV()));
        return 1;
    }

    async hget(key, field) {
        const hash = await this._getHash(key);
        return hash.get(field);
    }

    async hgetall(key) {
        const raw = await this.get(key);
        if (!raw) return null;
        return KVHash.fromRaw(this._deserialize(raw)).entries;
    }

    async hdel(key, ...fields) {
        const raw = await this.get(key);
        if (!raw) return 0;
        const hash = KVHash.fromRaw(this._deserialize(raw));
        const count = hash.del(fields.flat());
        await this.set(key, this._serialize(hash.forKV()));
        return count;
    }

    // =========================================================================
    // List operations (stored as JSON arrays in KV)
    // NOT atomic: concurrent rpush/lpush/ltrim to the same key will race.
    // =========================================================================

    async _getList(key) {
        const raw = await this.get(key);
        if (!raw) return KVList.fromRaw(null);
        return KVList.fromRaw(this._deserialize(raw));
    }

    async rpush(key, ...values) {
        const list = await this._getList(key);
        list.items.push(...values.flat());
        await this.set(key, this._serialize(list.forKV()));
        return list.items.length;
    }

    async lpush(key, ...values) {
        const list = await this._getList(key);
        list.items.unshift(...values.flat());
        await this.set(key, this._serialize(list.forKV()));
        return list.items.length;
    }

    async lrange(key, start, stop) {
        const raw = await this.get(key);
        if (!raw) return [];
        const list = KVList.fromRaw(this._deserialize(raw));
        if (stop === -1) return list.items.slice(start);
        return list.items.slice(start, stop + 1);
    }

    async llen(key) {
        const raw = await this.get(key);
        if (!raw) return 0;
        return KVList.fromRaw(this._deserialize(raw)).items.length;
    }

    async ltrim(key, start, stop) {
        const raw = await this.get(key);
        if (!raw) return 'OK';
        const list = KVList.fromRaw(this._deserialize(raw));
        list.items = stop === -1 ? list.items.slice(start) : list.items.slice(start, stop + 1);
        await this.set(key, this._serialize(list.forKV()));
        return 'OK';
    }

    // =========================================================================
    // Set operations (stored as JSON arrays in KV, deduplicated)
    // NOT atomic: concurrent sadd/srem to the same key will race.
    // =========================================================================

    async _getSet(key) {
        const raw = await this.get(key);
        if (!raw) return KVSet.fromRaw(null);
        return KVSet.fromRaw(this._deserialize(raw));
    }

    async sadd(key, ...members) {
        const set = await this._getSet(key);
        const added = set.add(members.flat());
        await this.set(key, this._serialize(set.forKV()));
        return added;
    }

    async srem(key, ...members) {
        const raw = await this.get(key);
        if (!raw) return 0;
        const set = KVSet.fromRaw(this._deserialize(raw));
        const removed = set.remove(members.flat());
        await this.set(key, this._serialize(set.forKV()));
        return removed;
    }

    async smembers(key) {
        const raw = await this.get(key);
        if (!raw) return [];
        return KVSet.fromRaw(this._deserialize(raw)).members;
    }

    async scard(key) {
        const raw = await this.get(key);
        if (!raw) return 0;
        return KVSet.fromRaw(this._deserialize(raw)).members.length;
    }

    // =========================================================================
    // Sorted set operations (stored as JSON array of {score, member} in KV)
    // NOT atomic: concurrent zadd/zrem to the same key will race.
    // =========================================================================

    async _getZSet(key) {
        const raw = await this.get(key);
        if (!raw) return KVSortedSet.fromRaw(null);
        return KVSortedSet.fromRaw(this._deserialize(raw));
    }

    async zadd(key, score, member) {
        const zset = await this._getZSet(key);
        const added = zset.upsert(score, member);
        await this.set(key, this._serialize(zset.forKV()));
        return added;
    }

    async zrem(key, ...members) {
        const raw = await this.get(key);
        if (!raw) return 0;
        const zset = KVSortedSet.fromRaw(this._deserialize(raw));
        const removed = zset.remove(members.flat());
        await this.set(key, this._serialize(zset.forKV()));
        return removed;
    }

    async zrange(key, start, stop) {
        const raw = await this.get(key);
        if (!raw) return [];
        const zset = KVSortedSet.fromRaw(this._deserialize(raw));
        const slice = stop === -1 ? zset.entries.slice(start) : zset.entries.slice(start, stop + 1);
        return slice.map(e => e.member);
    }

    async zrevrange(key, start, stop) {
        const raw = await this.get(key);
        if (!raw) return [];
        const zset = KVSortedSet.fromRaw(this._deserialize(raw));
        const reversed = [...zset.entries].reverse();
        const slice = stop === -1 ? reversed.slice(start) : reversed.slice(start, stop + 1);
        return slice.map(e => e.member);
    }

    // =========================================================================
    // Scan
    // =========================================================================

    async scan(cursor, ...args) {
        let pattern = '*';
        let count = KV_SCAN_DEFAULT_COUNT;
        for (let i = 0; i < args.length; i++) {
            if (typeof args[i] === 'string') {
                const flag = args[i].toUpperCase();
                if (flag === 'MATCH' && args[i + 1]) {
                    pattern = args[i + 1];
                    i++;
                } else if (flag === 'COUNT' && args[i + 1] !== undefined) {
                    count = parseInt(args[i + 1], 10);
                    i++;
                }
            }
        }
        try {
            const data = await this._http.post('/kv/_scan', JSON.stringify({
                cursor: parseInt(cursor, 10) || 0, pattern, count
            }));
            return [data.cursor.toString(), data.keys];
        } catch (error) {
            throw new KVOperationError('scan', pattern, error);
        }
    }

    // =========================================================================
    // Stream operations (emulated using JSON arrays in KV)
    // NOT atomic: concurrent xadd to the same key can produce duplicate IDs
    // when using '*' (auto-ID is derived from stream.length at read time).
    // =========================================================================

    async _getStream(key) {
        const raw = await this.get(key);
        if (!raw) return KVStream.fromRaw(null);
        return KVStream.fromRaw(this._deserialize(raw));
    }

    async xadd(key, id, ...fieldValues) {
        const stream = await this._getStream(key);
        const fields = [];
        for (let i = 0; i < fieldValues.length; i += 2) {
            fields.push(fieldValues[i], fieldValues[i + 1]);
        }
        const entryId = stream.append(id, fields);
        await this.set(key, this._serialize(stream.forKV()));
        return entryId;
    }

    async xrange(key, start, end) {
        const raw = await this.get(key);
        if (!raw) return [];
        return KVStream.fromRaw(this._deserialize(raw)).range(start, end);
    }

    // =========================================================================
    // Lifecycle
    // =========================================================================

    async quit() {
        return this.terminate();
    }

    async disconnect() {
        return this.terminate();
    }

    async terminate() {
        if (this._http.isTerminated()) return;
        this._http.terminate();
        logger.info('[VSODB-KV] Client terminated');
    }

    isTerminated() {
        return this._http.isTerminated();
    }

    async waitForReady(maxRetries, delayMs) {
        return this._http.waitForReady(maxRetries, delayMs);
    }
}

export { KVCacheClient, KVOperationError };
