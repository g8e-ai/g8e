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
// @vitest-environment jsdom

import { describe, it, expect } from 'vitest';
import { AuthResponseModel } from '@vsod/public/js/models/auth-response-model.js';
import { WebSessionModel } from '@vsod/public/js/models/session-model.js';
import { FrontendBaseModel } from '@vsod/public/js/models/base.js';
import { UserRole } from '@vsod/constants/auth.js';

const FUTURE = new Date(Date.now() + 86400000).toISOString();

function makeValidSessionRaw(overrides = {}) {
    return {
        id: 'session-abc',
        user_id: 'user-xyz',
        is_active: true,
        expires_at: FUTURE,
        user_data: { roles: [UserRole.USER] },
        ...overrides,
    };
}

describe('AuthResponseModel — parse() [UNIT]', () => {
    it('parse() returns an AuthResponseModel instance', () => {
        expect(AuthResponseModel.parse({ success: true })).toBeInstanceOf(AuthResponseModel);
    });

    it('parse() returns an instance of FrontendBaseModel', () => {
        expect(AuthResponseModel.parse({ success: true })).toBeInstanceOf(FrontendBaseModel);
    });

    it('success defaults to false', () => {
        expect(AuthResponseModel.parse({}).success).toBe(false);
    });

    it('session defaults to null', () => {
        expect(AuthResponseModel.parse({ success: true }).session).toBeNull();
    });

    it('message defaults to error string when success is false', () => {
        const m = AuthResponseModel.parse({ success: false });
        expect(m.message).toBe('An unknown authentication error occurred.');
    });

    it('message defaults to empty string when success is true', () => {
        const m = AuthResponseModel.parse({ success: true });
        expect(m.message).toBe('');
    });

    it('message is preserved when explicitly provided', () => {
        const m = AuthResponseModel.parse({ success: false, message: 'Invalid credentials' });
        expect(m.message).toBe('Invalid credentials');
    });

    it('success coerced from string "true"', () => {
        const m = AuthResponseModel.parse({ success: 'true' });
        expect(m.success).toBe(true);
    });

    it('strips unknown fields', () => {
        const m = AuthResponseModel.parse({ success: true, extra: 'drop' });
        expect(m).not.toHaveProperty('extra');
    });

    it('parses nested session into WebSessionModel instance', () => {
        const m = AuthResponseModel.parse({ success: true, session: makeValidSessionRaw() });
        expect(m.session).toBeInstanceOf(WebSessionModel);
    });

    it('nested session preserves user_id', () => {
        const m = AuthResponseModel.parse({ success: true, session: makeValidSessionRaw() });
        expect(m.session.user_id).toBe('user-xyz');
    });

    it('nested session strips unknown fields', () => {
        const m = AuthResponseModel.parse({ success: true, session: makeValidSessionRaw({ bogus: 'drop' }) });
        expect(m.session).not.toHaveProperty('bogus');
    });
});

describe('AuthResponseModel — isAuthenticated [UNIT]', () => {
    it('returns true when success=true and session is valid', () => {
        const m = AuthResponseModel.parse({ success: true, session: makeValidSessionRaw() });
        expect(m.isAuthenticated).toBe(true);
    });

    it('returns false when success=false', () => {
        const m = AuthResponseModel.parse({ success: false, session: makeValidSessionRaw() });
        expect(m.isAuthenticated).toBe(false);
    });

    it('returns false when session is null', () => {
        const m = AuthResponseModel.parse({ success: true });
        expect(m.isAuthenticated).toBe(false);
    });

    it('returns false when session is expired', () => {
        const PAST = new Date(Date.now() - 1000).toISOString();
        const m = AuthResponseModel.parse({
            success: true,
            session: makeValidSessionRaw({ expires_at: PAST }),
        });
        expect(m.isAuthenticated).toBe(false);
    });

    it('returns false when session has no user_id', () => {
        const m = AuthResponseModel.parse({
            success: true,
            session: makeValidSessionRaw({ user_id: null }),
        });
        expect(m.isAuthenticated).toBe(false);
    });
});

describe('AuthResponseModel — forWire() [UNIT]', () => {
    it('forWire() returns a plain object', () => {
        const wire = AuthResponseModel.parse({ success: true }).forWire();
        expect(typeof wire).toBe('object');
        expect(wire).not.toBeInstanceOf(AuthResponseModel);
    });

    it('forWire() with nested session serializes session to plain object', () => {
        const m = AuthResponseModel.parse({ success: true, session: makeValidSessionRaw() });
        const wire = m.forWire();
        expect(wire.session).not.toBeInstanceOf(WebSessionModel);
        expect(typeof wire.session).toBe('object');
        expect(wire.session.user_id).toBe('user-xyz');
    });

    it('forWire() has no Date instances', () => {
        function findDates(obj) {
            if (obj instanceof Date) return true;
            if (Array.isArray(obj)) return obj.some(findDates);
            if (obj !== null && typeof obj === 'object') return Object.values(obj).some(findDates);
            return false;
        }
        const m = AuthResponseModel.parse({ success: true, session: makeValidSessionRaw() });
        expect(findDates(m.forWire())).toBe(false);
    });
});
