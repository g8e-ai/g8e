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

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { KVCacheClient, KVOperationError } from '@g8ed/services/clients/g8es_kv_cache_client.js';
import { g8es_KV_CLIENT_STATUS_READY } from '@g8ed/constants/http_client.js';

describe('KVCacheClient', () => {
    const listenUrl = 'https://g8es:9000';
    const internalAuthToken = 'test-token';
    let client;
    let mockHttp;

    beforeEach(() => {
        client = new KVCacheClient({ listenUrl, internalAuthToken });
        mockHttp = client._http;
        vi.spyOn(mockHttp, 'get');
        vi.spyOn(mockHttp, 'put');
        vi.spyOn(mockHttp, 'post');
        vi.spyOn(mockHttp, 'delete');
        vi.spyOn(mockHttp, 'terminate');
        vi.spyOn(mockHttp, 'isTerminated');
    });

    describe('core KV operations', () => {
        it('get() should return value or null', async () => {
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: 'bar' });
            expect(await client.get('foo')).toBe('bar');

            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.get('foo')).toBe(null);

            vi.mocked(mockHttp.get).mockRejectedValueOnce(new Error('404'));
            expect(await client.get('foo')).toBe(null);
        });

        it('get() should URL-encode keys with special characters', async () => {
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: 'bar' });
            await client.get('cache.doc');
            expect(mockHttp.get).toHaveBeenCalledWith('/kv/cache.doc');

            await client.get('key*wildcard');
            expect(mockHttp.get).toHaveBeenCalledWith('/kv/key*wildcard');

            await client.get('user+id');
            expect(mockHttp.get).toHaveBeenCalledWith('/kv/user%2Bid');

            await client.get('array[0]');
            expect(mockHttp.get).toHaveBeenCalledWith('/kv/array%5B0%5D');

            await client.get('path/to/file');
            expect(mockHttp.get).toHaveBeenCalledWith('/kv/path%2Fto%2Ffile');

            await client.get('key with spaces');
            expect(mockHttp.get).toHaveBeenCalledWith('/kv/key%20with%20spaces');
        });

        it('set() should support EX, PX, and NX', async () => {
            vi.mocked(mockHttp.put).mockResolvedValue({ success: true });
            
            // Test EX
            await client.set('foo', 'bar', 'EX', '60');
            expect(mockHttp.put).toHaveBeenCalledWith('/kv/foo', JSON.stringify({ value: 'bar', ttl: 60 }));

            // Test PX (ms to s)
            await client.set('foo', 'bar', 'PX', '1500');
            expect(mockHttp.put).toHaveBeenCalledWith('/kv/foo', JSON.stringify({ value: 'bar', ttl: 2 }));

            // Test NX - key doesn't exist
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            const result1 = await client.set('newkey', 'val', 'NX');
            expect(result1).toBe('OK');
            expect(mockHttp.get).toHaveBeenCalledWith('/kv/newkey');

            // Test NX - key exists
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: 'exists' });
            const result2 = await client.set('oldkey', 'val', 'NX');
            expect(result2).toBe(null);
            expect(mockHttp.get).toHaveBeenCalledWith('/kv/oldkey');
        });

        it('set() should throw if value is not a string', async () => {
            await expect(client.set('key', 123)).rejects.toThrow('KVCacheClient.set: value must be a string');
        });

        it('set() should URL-encode keys with special characters', async () => {
            vi.mocked(mockHttp.put).mockResolvedValue({ success: true });
            
            await client.set('cache.doc', 'value');
            expect(mockHttp.put).toHaveBeenCalledWith('/kv/cache.doc', expect.stringContaining('"value":"value"'));

            await client.set('array[0]', 'value');
            expect(mockHttp.put).toHaveBeenCalledWith('/kv/array%5B0%5D', expect.any(String));

            await client.set('user+id', 'value');
            expect(mockHttp.put).toHaveBeenCalledWith('/kv/user%2Bid', expect.any(String));
        });

        it('del() should handle multiple keys and nested arrays', async () => {
            vi.mocked(mockHttp.delete).mockResolvedValue({ success: true });
            const count = await client.del('k1', ['k2', 'k3'], 'k4');
            expect(count).toBe(4);
            expect(mockHttp.delete).toHaveBeenCalledTimes(4);
        });

        it('del() should swallow errors and continue', async () => {
            vi.mocked(mockHttp.delete)
                .mockResolvedValueOnce({ success: true })
                .mockRejectedValueOnce(new Error('fail'))
                .mockResolvedValueOnce({ success: true });
            
            const count = await client.del('k1', 'k2', 'k3');
            expect(count).toBe(2);
        });

        it('keys() should return list of keys on success', async () => {
            vi.mocked(mockHttp.post).mockResolvedValueOnce({ keys: ['k1', 'k2'] });
            expect(await client.keys('k*')).toEqual(['k1', 'k2']);
            expect(mockHttp.post).toHaveBeenCalledWith('/kv/_keys', expect.stringContaining('"pattern":"k*"'));
        });

        it('keys() should default to wildcard pattern', async () => {
            vi.mocked(mockHttp.post).mockResolvedValueOnce({ keys: [] });
            await client.keys();
            expect(mockHttp.post).toHaveBeenCalledWith('/kv/_keys', expect.stringContaining('"pattern":"*"'));
        });

        it('keys() should throw KVOperationError on failure', async () => {
            const cause = new Error('connection refused');
            vi.mocked(mockHttp.post).mockRejectedValueOnce(cause);

            const err = await client.keys('test:*').catch(e => e);
            expect(err).toBeInstanceOf(KVOperationError);
            expect(err.operation).toBe('keys');
            expect(err.pattern).toBe('test:*');
            expect(err.cause).toBe(cause);
        });

        it('setex() should be a shortcut for set with EX', async () => {
            vi.mocked(mockHttp.put).mockResolvedValueOnce({ success: true });
            await client.setex('foo', 30, 'bar');
            expect(mockHttp.put).toHaveBeenCalledWith('/kv/foo', JSON.stringify({ value: 'bar', ttl: 30 }));
        });

        it('get_json/set_json should serialize/deserialize', async () => {
            const data = { a: 1, b: [2, 3] };
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify(data) });
            expect(await client.get_json('obj')).toEqual(data);

            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.get_json('missing')).toBe(null);

            vi.mocked(mockHttp.put).mockResolvedValue({ success: true });
            await client.set_json('obj', data, 60);
            expect(mockHttp.put).toHaveBeenCalledWith('/kv/obj', JSON.stringify({ value: JSON.stringify(data), ttl: 60 }));
            
            await client.set_json('obj2', data);
            expect(mockHttp.put).toHaveBeenCalledWith('/kv/obj2', JSON.stringify({ value: JSON.stringify(data), ttl: 0 }));
        });

        it('ping() should return PONG', async () => {
            vi.mocked(mockHttp.post).mockResolvedValueOnce({ keys: [] });
            expect(await client.ping()).toBe('PONG');
            expect(mockHttp.post).toHaveBeenCalledWith('/kv/_keys', expect.stringContaining('"pattern":"__ping__"'));
        });

        it('exists() should return 1 or 0', async () => {
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: 'val' });
            expect(await client.exists('foo')).toBe(1);

            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.exists('bar')).toBe(0);
        });

        it('incr/decr should work correctly', async () => {
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: '10' });
            vi.mocked(mockHttp.put).mockResolvedValue({ success: true });
            expect(await client.incr('c')).toBe(11);

            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: '10' });
            expect(await client.decr('c')).toBe(9);

            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.incr('new')).toBe(1);

            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: 'not-a-number' });
            expect(await client.incr('nan')).toBe(1);
        });

        it('expire() and ttl() should handle TTL operations', async () => {
            vi.mocked(mockHttp.put).mockResolvedValueOnce({ success: true });
            expect(await client.expire('foo', 100)).toBe(1);
            expect(mockHttp.put).toHaveBeenCalledWith('/kv/foo/_expire', JSON.stringify({ ttl: 100 }));

            vi.mocked(mockHttp.put).mockRejectedValueOnce(new Error('fail'));
            expect(await client.expire('bar', 100)).toBe(0);

            vi.mocked(mockHttp.get).mockResolvedValueOnce({ ttl: 50 });
            expect(await client.ttl('foo')).toBe(50);

            vi.mocked(mockHttp.get).mockRejectedValueOnce(new Error('fail'));
            expect(await client.ttl('bar')).toBe(-2);
        });
    });

    describe('Hash operations', () => {
        it('hset/hget/hgetall/hdel should manage JSON hash', async () => {
            // hset
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            vi.mocked(mockHttp.put).mockResolvedValue({ success: true });
            expect(await client.hset('h', 'f1', 'v1')).toBe(1);

            // hget
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ entries: { f1: 'v1' } }) });
            expect(await client.hget('h', 'f1')).toBe('v1');
            expect(await client.hget('h', 'f2')).toBe(null);

            // hgetall
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ entries: { f1: 'v1', f2: 'v2' } }) });
            expect(await client.hgetall('h')).toEqual({ f1: 'v1', f2: 'v2' });
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.hgetall('missing')).toBe(null);

            // hdel
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ entries: { f1: 'v1', f2: 'v2' } }) });
            expect(await client.hdel('h', 'f1', 'f3')).toBe(1);
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.hdel('missing', 'f1')).toBe(0);
        });
    });

    describe('List operations', () => {
        it('rpush/lpush/lrange/llen/ltrim should manage JSON list', async () => {
            vi.mocked(mockHttp.put).mockResolvedValue({ success: true });

            // rpush/lpush
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.rpush('l', 'b', 'c')).toBe(2);
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ items: ['b', 'c'] }) });
            expect(await client.lpush('l', 'a')).toBe(3);

            // llen
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ items: ['a', 'b', 'c'] }) });
            expect(await client.llen('l')).toBe(3);
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.llen('missing')).toBe(0);

            // lrange
            vi.mocked(mockHttp.get).mockResolvedValue({ value: JSON.stringify({ items: ['a', 'b', 'c'] }) });
            expect(await client.lrange('l', 0, 1)).toEqual(['a', 'b']);
            expect(await client.lrange('l', 1, -1)).toEqual(['b', 'c']);
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.lrange('missing', 0, -1)).toEqual([]);

            // ltrim
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ items: ['a', 'b', 'c'] }) });
            expect(await client.ltrim('l', 0, 1)).toBe('OK');
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.ltrim('missing', 0, -1)).toBe('OK');
        });
    });

    describe('Set operations', () => {
        it('sadd/srem/smembers/scard should manage JSON set', async () => {
            vi.mocked(mockHttp.put).mockResolvedValue({ success: true });

            // sadd
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.sadd('s', 'a', 'b', 'a')).toBe(2);

            // srem
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ members: ['a', 'b'] }) });
            expect(await client.srem('s', 'a', 'c')).toBe(1);
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.srem('missing', 'a')).toBe(0);

            // smembers
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ members: ['a', 'b'] }) });
            expect(await client.smembers('s')).toEqual(['a', 'b']);
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.smembers('missing')).toEqual([]);

            // scard
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ members: ['a', 'b'] }) });
            expect(await client.scard('s')).toBe(2);
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.scard('missing')).toBe(0);
        });
    });

    describe('Sorted set operations', () => {
        it('zadd/zrem/zrange/zrevrange should manage JSON sorted set', async () => {
            vi.mocked(mockHttp.put).mockResolvedValue({ success: true });

            // zadd
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.zadd('z', 10, 'm1')).toBe(1);
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ entries: [{ score: 10, member: 'm1' }] }) });
            expect(await client.zadd('z', 20, 'm1')).toBe(0); // Update score

            // zrem
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ entries: [{ score: 10, member: 'm1' }, { score: 20, member: 'm2' }] }) });
            expect(await client.zrem('z', 'm1', 'm3')).toBe(1);
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.zrem('missing', 'm1')).toBe(0);

            // zrange
            vi.mocked(mockHttp.get).mockResolvedValue({ value: JSON.stringify({ entries: [{ score: 10, member: 'm1' }, { score: 20, member: 'm2' }] }) });
            expect(await client.zrange('z', 0, 0)).toEqual(['m1']);
            expect(await client.zrange('z', 0, -1)).toEqual(['m1', 'm2']);
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.zrange('missing', 0, -1)).toEqual([]);

            // zrevrange
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ entries: [{ score: 10, member: 'm1' }, { score: 20, member: 'm2' }] }) });
            expect(await client.zrevrange('z', 0, -1)).toEqual(['m2', 'm1']);
        });
    });

    describe('Scan', () => {
        it('scan() should support MATCH and COUNT', async () => {
            vi.mocked(mockHttp.post).mockResolvedValueOnce({ cursor: 10, keys: ['k1'] });
            const result = await client.scan('0', 'MATCH', 'k*', 'COUNT', 5);
            expect(result).toEqual(['10', ['k1']]);
            expect(mockHttp.post).toHaveBeenCalledWith('/kv/_scan', expect.stringContaining('"cursor":0'));
            expect(mockHttp.post).toHaveBeenCalledWith('/kv/_scan', expect.stringContaining('"pattern":"k*"'));
            expect(mockHttp.post).toHaveBeenCalledWith('/kv/_scan', expect.stringContaining('"count":5'));

            const cause = new Error('fail');
            vi.mocked(mockHttp.post).mockRejectedValueOnce(cause);
            const err = await client.scan('0').catch(e => e);
            expect(err).toBeInstanceOf(KVOperationError);
            expect(err.operation).toBe('scan');
            expect(err.cause).toBe(cause);
        });
    });

    describe('Stream operations', () => {
        it('xadd/xrange should manage JSON stream', async () => {
            vi.mocked(mockHttp.put).mockResolvedValue({ success: true });

            // xadd
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            const id = await client.xadd('str', '*', 'f1', 'v1');
            expect(id).toBeDefined();

            // xrange
            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: JSON.stringify({ entries: [{ id: '1-0', fields: ['f1', 'v1'] }, { id: '2-0', fields: ['f2', 'v2'] }] }) });
            const range = await client.xrange('str', '1-0', '1-0');
            expect(range).toHaveLength(1);
            expect(range[0].id).toBe('1-0');

            vi.mocked(mockHttp.get).mockResolvedValueOnce({ value: null });
            expect(await client.xrange('missing', '-', '+')).toEqual([]);
        });
    });

    describe('Lifecycle and Utility', () => {
        it('should report status and handle termination', async () => {
            expect(client.status).toBe(G8es_KV_CLIENT_STATUS_READY);
            
            vi.mocked(mockHttp.isTerminated).mockReturnValue(false);
            await client.quit();
            expect(mockHttp.terminate).toHaveBeenCalled();

            await client.disconnect();
            expect(mockHttp.terminate).toHaveBeenCalledTimes(2);

            vi.mocked(mockHttp.isTerminated).mockReturnValue(true);
            await client.terminate();
            expect(mockHttp.terminate).toHaveBeenCalledTimes(2); // No extra call
            
            expect(client.isTerminated()).toBe(true);
        });
    });
});
