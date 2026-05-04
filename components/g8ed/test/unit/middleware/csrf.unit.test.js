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

import { describe, it, expect, vi } from 'vitest';
import { createCsrfProtection } from '../../../middleware/csrf.js';

describe('CSRF Middleware', () => {
    it('should set a CSRF cookie if not present', () => {
        const middleware = createCsrfProtection();
        const req = {
            method: 'GET',
            path: '/test',
            cookies: {},
            headers: {}
        };
        const res = {
            cookie: vi.fn(),
            locals: {}
        };
        const next = vi.fn();

        middleware(req, res, next);

        expect(res.cookie).toHaveBeenCalledWith('g8e_csrf_token', expect.any(String), expect.any(Object));
        expect(res.locals.csrfToken).toBeDefined();
        expect(next).toHaveBeenCalled();
    });

    it('should skip validation for GET requests', () => {
        const middleware = createCsrfProtection();
        const req = {
            method: 'GET',
            path: '/test',
            cookies: { g8e_csrf_token: 'valid-token' },
            headers: {}
        };
        const res = {
            cookie: vi.fn(),
            locals: {}
        };
        const next = vi.fn();

        middleware(req, res, next);

        expect(next).toHaveBeenCalledWith();
    });

    it('should fail POST requests without a token', () => {
        const middleware = createCsrfProtection();
        const req = {
            method: 'POST',
            path: '/test',
            cookies: { g8e_csrf_token: 'valid-token' },
            headers: {},
            body: {}
        };
        const res = {
            cookie: vi.fn(),
            locals: {}
        };
        const next = vi.fn();

        middleware(req, res, next);

        expect(next).toHaveBeenCalledWith(expect.any(Error));
        const error = next.mock.calls[0][0];
        expect(error.message).toBe('Invalid or missing CSRF token');
    });

    it('should pass POST requests with a valid header token', () => {
        const middleware = createCsrfProtection();
        const req = {
            method: 'POST',
            path: '/test',
            cookies: { g8e_csrf_token: 'valid-token' },
            headers: { 'x-csrf-token': 'valid-token' },
            body: {}
        };
        const res = {
            cookie: vi.fn(),
            locals: {}
        };
        const next = vi.fn();

        middleware(req, res, next);

        expect(next).toHaveBeenCalledWith();
    });

    it('should skip validation if x-skip-csrf header is present in test mode', () => {
        const middleware = createCsrfProtection({ isTest: true });
        const req = {
            method: 'POST',
            path: '/test',
            cookies: { g8e_csrf_token: 'valid-token' },
            headers: { 'x-skip-csrf': 'true' },
            body: {}
        };
        const res = {
            cookie: vi.fn(),
            locals: {}
        };
        const next = vi.fn();

        middleware(req, res, next);

        expect(next).toHaveBeenCalledWith();
    });
});
