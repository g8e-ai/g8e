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
 * KV Mock for Testing
 *
 * In-memory g8es KV simulation that faithfully mirrors the real
 * KVCacheClient contract:
 *   - set() requires a string value — throws on non-string input
 *   - All compound structures (rpush, sadd, zadd, incr, decr) store JSON
 *     strings exactly as the real client does via forKV()
 *   - TTL expiry is enforced on get/exists
 *
 * Usage:
 *   const kv = createKVMock();
 *   vi.spyOn(kv, 'setex');           // spyable when call assertions are needed
 *   vi.spyOn(kv, 'rpush');
 *   expect(kv.setex).toHaveBeenCalledTimes(1);
 */

export class KVMock {
    constructor() {
        this.store = new Map();
        this.ttls = new Map();
        this.status = 'ready';
    }

    _isExpired(key) {
        if (!this.ttls.has(key)) return false;
        if (Date.now() > this.ttls.get(key)) {
            this.store.delete(key);
            this.ttls.delete(key);
            return true;
        }
        return false;
    }

    async get(key) {
        if (this._isExpired(key)) return null;
        return this.store.get(key) ?? null;
    }

    async set(key, value, ...args) {
        if (typeof value !== 'string') {
            throw new Error(`KVMock.set: value must be a string, got ${typeof value}`);
        }
        let nx = false;
        for (let i = 0; i < args.length; i++) {
            if (typeof args[i] === 'string') {
                const flag = args[i].toUpperCase();
                if (flag === 'EX' && args[i + 1] !== undefined) {
                    this.ttls.set(key, Date.now() + parseInt(args[i + 1], 10) * 1000);
                    i++;
                } else if (flag === 'PX' && args[i + 1] !== undefined) {
                    this.ttls.set(key, Date.now() + parseInt(args[i + 1], 10));
                    i++;
                } else if (flag === 'NX') {
                    nx = true;
                }
            }
        }
        if (nx && this.store.has(key) && !this._isExpired(key)) return null;
        this.store.set(key, value);
        return 'OK';
    }

    async setex(key, seconds, value) {
        if (typeof value !== 'string') {
            throw new Error(`KVMock.setex: value must be a string, got ${typeof value}`);
        }
        this.store.set(key, value);
        this.ttls.set(key, Date.now() + seconds * 1000);
        return 'OK';
    }

    async del(...keys) {
        let count = 0;
        for (const key of keys.flat()) {
            if (this.store.has(key)) {
                this.store.delete(key);
                this.ttls.delete(key);
                count++;
            }
        }
        return count;
    }

    async exists(key) {
        if (this._isExpired(key)) return 0;
        return this.store.has(key) ? 1 : 0;
    }

    async expire(key, seconds) {
        if (!this.store.has(key) || this._isExpired(key)) return 0;
        this.ttls.set(key, Date.now() + seconds * 1000);
        return 1;
    }

    async ttl(key) {
        if (!this.store.has(key)) return -2;
        if (!this.ttls.has(key)) return -1;
        const secondsLeft = Math.floor((this.ttls.get(key) - Date.now()) / 1000);
        return secondsLeft > 0 ? secondsLeft : -2;
    }

    async incr(key) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : '0';
        const next = (parseInt(raw, 10) || 0) + 1;
        this.store.set(key, next.toString());
        return next;
    }

    async decr(key) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : '0';
        const next = (parseInt(raw, 10) || 0) - 1;
        this.store.set(key, next.toString());
        return next;
    }

    async keys(pattern) {
        const regexPattern = (pattern || '*')
            .replace(/[.+^${}()|[\]\\]/g, '\\$&')
            .replace(/\*/g, '.*')
            .replace(/\?/g, '.');
        const regex = new RegExp(`^${regexPattern}$`);
        return Array.from(this.store.keys()).filter(k => !this._isExpired(k) && regex.test(k));
    }

    async scan(cursor, ...args) {
        let pattern = '*';
        let count = 100;
        for (let i = 0; i < args.length; i++) {
            if (typeof args[i] === 'string') {
                const flag = args[i].toUpperCase();
                if (flag === 'MATCH' && args[i + 1]) { pattern = args[i + 1]; i++; }
                else if (flag === 'COUNT' && args[i + 1] !== undefined) { count = parseInt(args[i + 1], 10); i++; }
            }
        }
        const all = await this.keys(pattern);
        const start = parseInt(cursor, 10) || 0;
        const end = Math.min(start + count, all.length);
        const nextCursor = end >= all.length ? '0' : end.toString();
        return [nextCursor, all.slice(start, end)];
    }

    // -------------------------------------------------------------------------
    // Lists — stored as JSON strings, matching real KVCacheClient
    // -------------------------------------------------------------------------

    async rpush(key, ...values) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        const list = raw ? JSON.parse(raw) : [];
        list.push(...values.flat());
        this.store.set(key, JSON.stringify(list));
        return list.length;
    }

    async lpush(key, ...values) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        const list = raw ? JSON.parse(raw) : [];
        list.unshift(...values.flat());
        this.store.set(key, JSON.stringify(list));
        return list.length;
    }

    async lrange(key, start, stop) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        if (!raw) return [];
        const list = JSON.parse(raw);
        if (stop === -1) return list.slice(start);
        return list.slice(start, stop + 1);
    }

    async llen(key) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        if (!raw) return 0;
        return JSON.parse(raw).length;
    }

    async ltrim(key, start, stop) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        if (!raw) return 'OK';
        const list = JSON.parse(raw);
        const trimmed = stop === -1 ? list.slice(start) : list.slice(start, stop + 1);
        this.store.set(key, JSON.stringify(trimmed));
        return 'OK';
    }

    // -------------------------------------------------------------------------
    // Sets — stored as JSON strings, matching real KVCacheClient
    // -------------------------------------------------------------------------

    async sadd(key, ...members) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        const set = raw ? JSON.parse(raw) : [];
        let added = 0;
        for (const m of members.flat()) {
            if (!set.includes(m)) { set.push(m); added++; }
        }
        this.store.set(key, JSON.stringify(set));
        return added;
    }

    async srem(key, ...members) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        if (!raw) return 0;
        const set = JSON.parse(raw);
        const toRemove = members.flat();
        const filtered = set.filter(m => !toRemove.includes(m));
        this.store.set(key, JSON.stringify(filtered));
        return set.length - filtered.length;
    }

    async smembers(key) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        if (!raw) return [];
        return JSON.parse(raw);
    }

    async scard(key) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        if (!raw) return 0;
        return JSON.parse(raw).length;
    }

    // -------------------------------------------------------------------------
    // Sorted sets — stored as JSON strings, matching real KVCacheClient
    // -------------------------------------------------------------------------

    async zadd(key, score, member) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        const entries = raw ? JSON.parse(raw) : [];
        const idx = entries.findIndex(e => e.member === member);
        if (idx >= 0) { entries[idx].score = score; } else { entries.push({ score, member }); }
        entries.sort((a, b) => a.score - b.score);
        this.store.set(key, JSON.stringify(entries));
        return idx < 0 ? 1 : 0;
    }

    async zrem(key, ...members) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        if (!raw) return 0;
        const entries = JSON.parse(raw);
        const toRemove = members.flat();
        const filtered = entries.filter(e => !toRemove.includes(e.member));
        this.store.set(key, JSON.stringify(filtered));
        return entries.length - filtered.length;
    }

    async zrange(key, start, stop) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        if (!raw) return [];
        const entries = JSON.parse(raw);
        const slice = stop === -1 ? entries.slice(start) : entries.slice(start, stop + 1);
        return slice.map(e => e.member);
    }

    async zrevrange(key, start, stop) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        if (!raw) return [];
        const reversed = [...JSON.parse(raw)].reverse();
        const slice = stop === -1 ? reversed.slice(start) : reversed.slice(start, stop + 1);
        return slice.map(e => e.member);
    }

    // -------------------------------------------------------------------------
    // Streams — stored as JSON strings, matching real KVCacheClient
    // -------------------------------------------------------------------------

    async xadd(key, id, ...fieldValues) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        const stream = raw ? JSON.parse(raw) : [];
        const entryId = id === '*' ? `${Date.now()}-${stream.length}` : id;
        const fields = [];
        for (let i = 0; i < fieldValues.length; i += 2) {
            fields.push(fieldValues[i], fieldValues[i + 1]);
        }
        stream.push([entryId, fields]);
        this.store.set(key, JSON.stringify(stream));
        return entryId;
    }

    async xrange(key, start, end) {
        const raw = this.store.has(key) && !this._isExpired(key) ? this.store.get(key) : null;
        if (!raw) return [];
        return JSON.parse(raw).filter(entry => {
            const entryId = entry[0];
            return (start === '-' || entryId >= start) && (end === '+' || entryId <= end);
        });
    }

    // -------------------------------------------------------------------------
    // JSON helpers — mirror KVCacheClient.set_json / get_json
    // -------------------------------------------------------------------------

    async set_json(key, value, ex = null) {
        const serialized = JSON.stringify(value);
        if (ex !== null) {
            return this.setex(key, ex, serialized);
        }
        return this.set(key, serialized);
    }

    async get_json(key) {
        const raw = await this.get(key);
        return raw !== null ? JSON.parse(raw) : null;
    }

    // -------------------------------------------------------------------------
    // CacheAsideService kv* passthrough API
    // These delegate to the raw methods above so KVMock can be used directly
    // as a cacheAside mock in unit tests.
    // -------------------------------------------------------------------------

    async kvGet(key)                    { return this.get(key); }
    async kvSet(key, value, ...args)    { return this.set(key, value, ...args); }
    async kvDel(...keys)                { return this.del(...keys); }
    async kvSetex(key, ttl, value)      { return this.setex(key, ttl, value); }
    async kvGetJson(key)                { return this.get_json(key); }
    async kvSetJson(key, value, ttl)    { return this.set_json(key, value, ttl); }
    async kvTtl(key)                    { return this.ttl(key); }
    async kvExpire(key, seconds)        { return this.expire(key, seconds); }
    async kvExists(key)                 { return this.exists(key); }
    async kvIncr(key)                   { return this.incr(key); }
    async kvDecr(key)                   { return this.decr(key); }
    async kvSadd(key, ...members)       { return this.sadd(key, ...members); }
    async kvSrem(key, ...members)       { return this.srem(key, ...members); }
    async kvSmembers(key)               { return this.smembers(key); }
    async kvScard(key)                  { return this.scard(key); }
    async kvRpush(key, ...values)       { return this.rpush(key, ...values); }
    async kvLrange(key, start, stop)    { return this.lrange(key, start, stop); }
    async kvZadd(key, score, member)    { return this.zadd(key, score, member); }
    async kvZrem(key, ...members)       { return this.zrem(key, ...members); }
    async kvZrange(key, start, stop)    { return this.zrange(key, start, stop); }
    async kvZrevrange(key, start, stop) { return this.zrevrange(key, start, stop); }
    async kvScan(cursor, ...args)       { return this.scan(cursor, ...args); }

    // -------------------------------------------------------------------------
    // Lifecycle / test helpers
    // -------------------------------------------------------------------------

    async quit() { return 'OK'; }
    async disconnect() { return 'OK'; }
    isTerminated() { return false; }

    clear() {
        this.store.clear();
        this.ttls.clear();
    }
}

export function createKVMock() {
    return new KVMock();
}
