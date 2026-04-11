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

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { UserRole } from '@g8ed/public/js/constants/auth-constants.js';

const FUTURE = new Date(Date.now() + 86400000).toISOString();
const PAST   = new Date(Date.now() - 1000).toISOString();

let webSessionService;
let WebSessionModel;

beforeEach(async () => {
    vi.resetModules();
    ({ webSessionService } = await import('@g8ed/public/js/utils/web-session-service.js'));
    ({ WebSessionModel }   = await import('@g8ed/public/js/models/session-model.js'));
    webSessionService.clearSession();
});

function makeSessionModel(overrides = {}) {
    const { user_data: userDataOverride, ...rest } = overrides;
    return WebSessionModel.parse({
        id: 'session-abc',
        user_id: 'user-xyz',
        is_active: true,
        expires_at: FUTURE,
        api_key: 'ak_test',
        user_data: {
            name: 'Alice',
            email: 'alice@example.com',
            roles: [UserRole.USER],
            ...userDataOverride,
        },
        ...rest,
    });
}

describe('WebSessionService — initial state [UNIT]', () => {
    it('getSession() returns null before any session is set', () => {
        expect(webSessionService.getSession()).toBeNull();
    });

    it('isAuthenticated() returns false with no session', () => {
        expect(webSessionService.isAuthenticated()).toBe(false);
    });

    it('getWebSessionId() returns null with no session', () => {
        expect(webSessionService.getWebSessionId()).toBeNull();
    });

    it('getApiKey() returns null with no session', () => {
        expect(webSessionService.getApiKey()).toBeNull();
    });

    it('hasRole() returns false with no session', () => {
        expect(webSessionService.hasRole(UserRole.ADMIN)).toBe(false);
    });

    it('isAdmin() returns false with no session', () => {
        expect(webSessionService.isAdmin()).toBe(false);
    });
});

describe('WebSessionService — setSession() [UNIT]', () => {
    it('stores the session and getSession() returns the exact same reference', () => {
        const session = makeSessionModel();
        webSessionService.setSession(session);
        expect(webSessionService.getSession()).toBe(session);
    });

    it('accepting null leaves the service in a cleared state', () => {
        webSessionService.setSession(makeSessionModel());
        webSessionService.setSession(null);
        expect(webSessionService.getSession()).toBeNull();
        expect(webSessionService.isAuthenticated()).toBe(false);
        expect(webSessionService.getWebSessionId()).toBeNull();
    });

    it('throws when passed a plain object instead of WebSessionModel', () => {
        expect(() => webSessionService.setSession({ id: 'fake' })).toThrow(
            'WebSessionService.setSession requires a WebSessionModel instance or null'
        );
    });

    it('throws when passed a string', () => {
        expect(() => webSessionService.setSession('session-id')).toThrow(
            'WebSessionService.setSession requires a WebSessionModel instance or null'
        );
    });

    it('throws when passed undefined', () => {
        expect(() => webSessionService.setSession(undefined)).toThrow(
            'WebSessionService.setSession requires a WebSessionModel instance or null'
        );
    });

    it('throws when passed an integer', () => {
        expect(() => webSessionService.setSession(42)).toThrow(
            'WebSessionService.setSession requires a WebSessionModel instance or null'
        );
    });

    it('does not change state when an invalid value is passed', () => {
        const session = makeSessionModel();
        webSessionService.setSession(session);
        expect(() => webSessionService.setSession({ id: 'fake' })).toThrow();
        expect(webSessionService.getSession()).toBe(session);
    });
});

describe('WebSessionService — clearSession() [UNIT]', () => {
    it('clears an active session — getSession() returns null', () => {
        webSessionService.setSession(makeSessionModel());
        webSessionService.clearSession();
        expect(webSessionService.getSession()).toBeNull();
    });

    it('clears an active session — all accessors return null/false', () => {
        webSessionService.setSession(makeSessionModel());
        webSessionService.clearSession();
        expect(webSessionService.isAuthenticated()).toBe(false);
        expect(webSessionService.getWebSessionId()).toBeNull();
        expect(webSessionService.getApiKey()).toBeNull();
        expect(webSessionService.hasRole(UserRole.USER)).toBe(false);
        expect(webSessionService.isAdmin()).toBe(false);
    });

    it('is idempotent — calling clearSession() twice does not throw', () => {
        webSessionService.setSession(makeSessionModel());
        webSessionService.clearSession();
        expect(() => webSessionService.clearSession()).not.toThrow();
        expect(webSessionService.getSession()).toBeNull();
    });

    it('is safe to call with no session ever set', () => {
        expect(() => webSessionService.clearSession()).not.toThrow();
        expect(webSessionService.getSession()).toBeNull();
    });
});

describe('WebSessionService — getWebSessionId() [UNIT]', () => {
    it('returns the session id when a session is set', () => {
        webSessionService.setSession(makeSessionModel());
        expect(webSessionService.getWebSessionId()).toBe('session-abc');
    });

    it('returns null after clearSession()', () => {
        webSessionService.setSession(makeSessionModel());
        webSessionService.clearSession();
        expect(webSessionService.getWebSessionId()).toBeNull();
    });
});

describe('WebSessionService — isAuthenticated() [UNIT]', () => {
    it('returns true for a valid active session', () => {
        webSessionService.setSession(makeSessionModel());
        expect(webSessionService.isAuthenticated()).toBe(true);
    });

    it('returns false for an inactive session (is_active: false)', () => {
        webSessionService.setSession(makeSessionModel({ is_active: false }));
        expect(webSessionService.isAuthenticated()).toBe(false);
    });

    it('returns false for an expired session (expires_at in the past)', () => {
        webSessionService.setSession(makeSessionModel({ expires_at: PAST }));
        expect(webSessionService.isAuthenticated()).toBe(false);
    });

    it('returns false for a session with no user_id', () => {
        webSessionService.setSession(makeSessionModel({ user_id: null }));
        expect(webSessionService.isAuthenticated()).toBe(false);
    });

    it('returns true when expires_at is absent (no expiry set)', () => {
        webSessionService.setSession(makeSessionModel({ expires_at: null }));
        expect(webSessionService.isAuthenticated()).toBe(true);
    });

    it('returns false after clearSession()', () => {
        webSessionService.setSession(makeSessionModel());
        webSessionService.clearSession();
        expect(webSessionService.isAuthenticated()).toBe(false);
    });

    it('reflects updated validity when session is replaced', () => {
        webSessionService.setSession(makeSessionModel());
        expect(webSessionService.isAuthenticated()).toBe(true);
        webSessionService.setSession(makeSessionModel({ is_active: false }));
        expect(webSessionService.isAuthenticated()).toBe(false);
    });
});

describe('WebSessionService — getApiKey() [UNIT]', () => {
    it('returns the api_key from the active session', () => {
        webSessionService.setSession(makeSessionModel());
        expect(webSessionService.getApiKey()).toBe('ak_test');
    });

    it('returns null when api_key is null on the session', () => {
        webSessionService.setSession(makeSessionModel({ api_key: null }));
        expect(webSessionService.getApiKey()).toBeNull();
    });

    it('returns null after clearSession()', () => {
        webSessionService.setSession(makeSessionModel());
        webSessionService.clearSession();
        expect(webSessionService.getApiKey()).toBeNull();
    });
});

describe('WebSessionService — hasRole() [UNIT]', () => {
    it('returns true when the session has the requested role', () => {
        webSessionService.setSession(makeSessionModel({ user_data: { roles: [UserRole.USER, UserRole.ADMIN] } }));
        expect(webSessionService.hasRole(UserRole.ADMIN)).toBe(true);
    });

    it('returns false when the session does not have the requested role', () => {
        webSessionService.setSession(makeSessionModel({ user_data: { roles: [UserRole.USER] } }));
        expect(webSessionService.hasRole(UserRole.ADMIN)).toBe(false);
    });

    it('returns false when roles is empty', () => {
        webSessionService.setSession(makeSessionModel({ user_data: { roles: [] } }));
        expect(webSessionService.hasRole(UserRole.USER)).toBe(false);
    });

    it('returns false for a role that is not present among multiple roles', () => {
        webSessionService.setSession(makeSessionModel({ user_data: { roles: [UserRole.USER, UserRole.ADMIN] } }));
        expect(webSessionService.hasRole(UserRole.SUPERADMIN)).toBe(false);
    });

    it('returns false after clearSession()', () => {
        webSessionService.setSession(makeSessionModel({ user_data: { roles: [UserRole.ADMIN] } }));
        webSessionService.clearSession();
        expect(webSessionService.hasRole(UserRole.ADMIN)).toBe(false);
    });
});

describe('WebSessionService — isAdmin() [UNIT]', () => {
    it('returns true for a session with admin role', () => {
        webSessionService.setSession(makeSessionModel({ user_data: { roles: [UserRole.ADMIN] } }));
        expect(webSessionService.isAdmin()).toBe(true);
    });

    it('returns true for a session with superadmin role', () => {
        webSessionService.setSession(makeSessionModel({ user_data: { roles: [UserRole.SUPERADMIN] } }));
        expect(webSessionService.isAdmin()).toBe(true);
    });

    it('returns true when session has both admin and superadmin roles', () => {
        webSessionService.setSession(makeSessionModel({ user_data: { roles: [UserRole.ADMIN, UserRole.SUPERADMIN] } }));
        expect(webSessionService.isAdmin()).toBe(true);
    });

    it('returns false for a plain user session', () => {
        webSessionService.setSession(makeSessionModel({ user_data: { roles: [UserRole.USER] } }));
        expect(webSessionService.isAdmin()).toBe(false);
    });

    it('returns false when roles is empty', () => {
        webSessionService.setSession(makeSessionModel({ user_data: { roles: [] } }));
        expect(webSessionService.isAdmin()).toBe(false);
    });

    it('returns false after clearSession()', () => {
        webSessionService.setSession(makeSessionModel({ user_data: { roles: [UserRole.ADMIN] } }));
        webSessionService.clearSession();
        expect(webSessionService.isAdmin()).toBe(false);
    });
});
