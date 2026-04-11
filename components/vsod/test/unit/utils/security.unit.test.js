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

import { describe, it, expect } from 'vitest';
import { getCookieDomain, clearSessionCookies, redactWebSessionId, EMAIL_REGEX } from '../../../utils/security.js';
import { SESSION_COOKIE_NAME, COOKIE_SAME_SITE } from '../../../constants/session.js';

describe('security utils', () => {
    describe('EMAIL_REGEX', () => {
        it('should validate correct emails', () => {
            expect(EMAIL_REGEX.test('test@example.com')).toBe(true);
            expect(EMAIL_REGEX.test('user.name+tag@domain.co.uk')).toBe(true);
        });

        it('should reject invalid emails', () => {
            expect(EMAIL_REGEX.test('invalid-email')).toBe(false);
            expect(EMAIL_REGEX.test('@domain.com')).toBe(false);
            expect(EMAIL_REGEX.test('user@')).toBe(false);
        });
    });

    describe('getCookieDomain', () => {
        it('should return undefined for localhost', () => {
            const req = { hostname: 'localhost' };
            expect(getCookieDomain(req)).toBeUndefined();
        });

        it('should return undefined for IP addresses', () => {
            const req = { hostname: '127.0.0.1' };
            expect(getCookieDomain(req)).toBeUndefined();
            
            const req2 = { hostname: '192.168.1.1' };
            expect(getCookieDomain(req2)).toBeUndefined();
        });

        it('should return .localhost for console.localhost', () => {
            const req = { hostname: 'console.localhost' };
            expect(getCookieDomain(req)).toBe('.localhost');
        });

        it('should handle req.get for host', () => {
            const req = { get: (name) => name === 'host' ? 'console.localhost:3000' : null };
            expect(getCookieDomain(req)).toBe('.localhost');
        });

        it('should return undefined for other domains (host-only)', () => {
            const req = { hostname: 'example.com' };
            expect(getCookieDomain(req)).toBeUndefined();
        });
    });

    describe('clearSessionCookies', () => {
        it('should clear cookies with and without domain', () => {
            const req = { hostname: 'console.localhost' };
            const cleared = [];
            const res = {
                clearCookie: (name, options) => cleared.push({ name, options })
            };

            clearSessionCookies(res, req);

            // Should clear:
            // 1. Host-only
            // 2. .console.localhost
            // 3. .localhost
            expect(cleared).toHaveLength(3);
            expect(cleared[0].name).toBe(SESSION_COOKIE_NAME);
            expect(cleared[0].options.domain).toBeUndefined();
            
            expect(cleared[1].options.domain).toBe('.console.localhost');
            expect(cleared[2].options.domain).toBe('.localhost');
        });

        it('should handle parent domains', () => {
            const req = { hostname: 'sub.console.localhost' };
            const cleared = [];
            const res = {
                clearCookie: (name, options) => cleared.push({ name, options })
            };

            clearSessionCookies(res, req);

            // Should clear:
            // 1. Host-only
            // 2. .sub.console.localhost
            // 3. .console.localhost
            // 4. .localhost
            expect(cleared).toHaveLength(4);
            expect(cleared[1].options.domain).toBe('.sub.console.localhost');
            expect(cleared[2].options.domain).toBe('.console.localhost');
            expect(cleared[3].options.domain).toBe('.localhost');
        });
    });

    describe('redactWebSessionId', () => {
        it('should redact long session IDs', () => {
            const longId = 'sess_1234567890abcdef1234567890';
            const redacted = redactWebSessionId(longId);
            expect(redacted).toBe('sess_1234567890abcdef1234...');
            expect(redacted.length).toBeLessThan(longId.length);
        });

        it('should not redact short session IDs', () => {
            const shortId = 'sess_123';
            expect(redactWebSessionId(shortId)).toBe(shortId);
        });

        it('should handle invalid inputs', () => {
            expect(redactWebSessionId(null)).toBe('[invalid]');
            expect(redactWebSessionId(undefined)).toBe('[invalid]');
            expect(redactWebSessionId(123)).toBe('[invalid]');
        });
    });
});
