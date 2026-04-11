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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { createRequestTimestampMiddleware } from '@g8ed/middleware/request_timestamp.js';
import { KVKey } from '@g8ed/constants/kv_keys.js';
import { TIMESTAMP_WINDOW_SECONDS } from '@g8ed/constants/auth.js';

describe('RequestTimestamp Middleware', () => {
    let cacheAside;
    let middleware;
    let req;
    let res;
    let next;

    beforeEach(() => {
        cacheAside = {
            kvGet: vi.fn(),
            kvSetex: vi.fn().mockResolvedValue()
        };
        middleware = createRequestTimestampMiddleware({ cacheAsideService: cacheAside });
        
        req = {
            headers: {},
            path: '/api/test',
            method: 'POST',
            ip: '127.0.0.1'
        };
        res = {
            status: vi.fn().mockReturnThis(),
            json: vi.fn().mockReturnThis()
        };
        next = vi.fn();
        
        vi.useFakeTimers();
        vi.setSystemTime(new Date('2026-03-30T14:00:00Z'));
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    describe('requireRequestTimestamp', () => {
        it('should return 400 if X-Request-Timestamp is missing', async () => {
            const handler = middleware.requireRequestTimestamp();
            await handler(req, res, next);

            expect(res.status).toHaveBeenCalledWith(400);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Invalid request timestamp'
            }));
            expect(next).not.toHaveBeenCalled();
        });

        it('should return 400 if timestamp is outside the window (too old)', async () => {
            const oldTime = new Date(Date.now() - (TIMESTAMP_WINDOW_SECONDS + 10) * 1000).toISOString();
            req.headers['x-request-timestamp'] = oldTime;
            
            const handler = middleware.requireRequestTimestamp();
            await handler(req, res, next);

            expect(res.status).toHaveBeenCalledWith(400);
            expect(next).not.toHaveBeenCalled();
        });

        it('should return 400 if timestamp is outside the window (too new)', async () => {
            const futureTime = new Date(Date.now() + (TIMESTAMP_WINDOW_SECONDS + 10) * 1000).toISOString();
            req.headers['x-request-timestamp'] = futureTime;
            
            const handler = middleware.requireRequestTimestamp();
            await handler(req, res, next);

            expect(res.status).toHaveBeenCalledWith(400);
        });

        it('should call next if timestamp is within window', async () => {
            req.headers['x-request-timestamp'] = new Date().toISOString();
            
            const handler = middleware.requireRequestTimestamp();
            await handler(req, res, next);

            expect(next).toHaveBeenCalled();
            expect(req.requestTimestamp).toBeDefined();
        });

        it('should return 400 if nonce is replayed (detected via KV)', async () => {
            req.headers['x-request-timestamp'] = new Date().toISOString();
            req.headers['x-request-nonce'] = 'replayed-nonce';
            
            cacheAside.kvGet.mockResolvedValue('1'); // Nonce exists

            const handler = middleware.requireRequestTimestamp();
            await handler(req, res, next);

            expect(res.status).toHaveBeenCalledWith(400);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Request replay detected'
            }));
        });

        it('should mark nonce as used and call next if nonce is new', async () => {
            req.headers['x-request-timestamp'] = new Date().toISOString();
            req.headers['x-request-nonce'] = 'new-nonce';
            
            cacheAside.kvGet.mockResolvedValue(null);

            const handler = middleware.requireRequestTimestamp();
            await handler(req, res, next);

            expect(cacheAside.kvSetex).toHaveBeenCalledWith(
                KVKey.nonce('new-nonce'),
                expect.any(Number),
                '1'
            );
            expect(next).toHaveBeenCalled();
        });

        it('should return 400 if nonce is required but missing', async () => {
            req.headers['x-request-timestamp'] = new Date().toISOString();
            
            const handler = middleware.requireRequestTimestamp({ requireNonce: true });
            await handler(req, res, next);

            expect(res.status).toHaveBeenCalledWith(400);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Missing request nonce'
            }));
        });
    });

    describe('optionalRequestTimestamp', () => {
        it('should call next if timestamp is missing', async () => {
            const handler = middleware.optionalRequestTimestamp();
            await handler(req, res, next);
            expect(next).toHaveBeenCalled();
        });

        it('should validate if timestamp is provided', async () => {
            req.headers['x-request-timestamp'] = 'invalid-date';
            
            const handler = middleware.optionalRequestTimestamp();
            await handler(req, res, next);

            expect(req.requestTimestamp.valid).toBe(false);
            expect(next).toHaveBeenCalled(); // Optional doesn't reject
        });
    });
});
