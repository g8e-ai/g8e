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
import { cspNonce } from '@vsod/middleware/csp_nonce.js';

describe('CSP Nonce Middleware', () => {
    let req;
    let res;
    let next;

    beforeEach(() => {
        req = {};
        res = {
            locals: {}
        };
        next = vi.fn();
    });

    it('should generate a base64 nonce and attach it to res.locals', () => {
        cspNonce(req, res, next);

        expect(res.locals.cspNonce).toBeDefined();
        expect(typeof res.locals.cspNonce).toBe('string');
        // Base64 encoded 16 bytes should be around 22-24 characters
        expect(res.locals.cspNonce.length).toBeGreaterThanOrEqual(22);
        expect(next).toHaveBeenCalled();
    });

    it('should generate unique nonces for subsequent calls', () => {
        const res1 = { locals: {} };
        const res2 = { locals: {} };
        
        cspNonce(req, res1, next);
        cspNonce(req, res2, next);

        expect(res1.locals.cspNonce).not.toBe(res2.locals.cspNonce);
    });
});
