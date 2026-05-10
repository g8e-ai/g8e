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
import { WebSessionModel } from '@g8ed/public/js/models/session-model.js';
import { FrontendBaseModel } from '@g8ed/public/js/models/base.js';
import { UserRole } from '@g8ed/constants/auth.js';

const FUTURE = new Date(Date.now() + 86400000).toISOString();
const PAST   = new Date(Date.now() - 1000).toISOString();

function makeSession(overrides = {}) {
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
        },
        ...overrides,
    });
}

describe('WebSessionModel — parse() [UNIT]', () => {
    it('parse() returns an instance of WebSessionModel', () => {
        expect(makeSession()).toBeInstanceOf(WebSessionModel);
    });

    it('parse() returns an instance of FrontendBaseModel', () => {
        expect(makeSession()).toBeInstanceOf(FrontendBaseModel);
    });

    it('parse() strips unknown fields', () => {
        const m = WebSessionModel.parse({ unknown_field: 'drop' });
        expect(m).not.toHaveProperty('unknown_field');
    });

    it('parse() applies defaults for all missing fields', () => {
        const m = WebSessionModel.parse({}); 
        expect(m.id).toBeNull();
        expect(m.user_id).toBeNull();
        expect(m.is_active).toBe(false);
        expect(m.expires_at).toBeNull();
        expect(m.api_key).toBeNull();
    });

    it('parse() coerces is_active string "true" to boolean', () => {
        const m = WebSessionModel.parse({ is_active: 'true' });
        expect(m.is_active).toBe(true);
    });

    it('parse() normalises user_data to canonical shape', () => {
        const m = WebSessionModel.parse({ user_data: { name: 'Admin' } });
        expect(m.user_data.name).toBe('Admin');
        expect(m.user_data.email).toBeNull();
        expect(Array.isArray(m.user_data.roles)).toBe(true);
    });

    it('parse() defaults user_data.roles to []', () => {
        const m = WebSessionModel.parse({});
        expect(m.user_data.roles).toEqual([]);
    });

    it('parse() normalises api_key_info to canonical shape', () => {
        const m = WebSessionModel.parse({ api_key_info: { client_id: 'cid' } });
        expect(m.api_key_info.client_id).toBe('cid');
        expect(m.api_key_info.client_name).toBeNull();
        expect(m.api_key_info.scopes).toBeNull();
    });

    it('fromJSON() round-trips via parse()', () => {
        const original = makeSession();
        const restored = WebSessionModel.fromJSON(original.toJSON());
        expect(restored.id).toBe(original.id);
        expect(restored.user_id).toBe(original.user_id);
        expect(restored.user_data.email).toBe(original.user_data.email);
        expect(restored.api_key).toBe(original.api_key);
        expect(restored.isValid()).toBe(original.isValid());
    });
});

describe('WebSessionModel — forWire() [UNIT]', () => {
    it('forWire() returns a plain object', () => {
        const wire = makeSession().forWire();
        expect(typeof wire).toBe('object');
        expect(wire).not.toBeInstanceOf(WebSessionModel);
    });

    it('toJSON() delegates to forWire()', () => {
        const m = makeSession();
        expect(m.toJSON()).toEqual(m.forWire());
    });

    it('forWire() preserves id, user_id, api_key', () => {
        const wire = makeSession().forWire();
        expect(wire.id).toBe('session-abc');
        expect(wire.user_id).toBe('user-xyz');
        expect(wire.api_key).toBe('ak_test');
    });
});

describe('WebSessionModel — getDisplayName() [UNIT]', () => {
    it('returns user name when present', () => {
        expect(makeSession().getDisplayName()).toBe('Alice');
    });

    it('falls back to email when name is absent', () => {
        const m = WebSessionModel.parse({ user_data: { email: 'alice@example.com' } });
        expect(m.getDisplayName()).toBe('alice@example.com');
    });

    it('falls back to "User" when both name and email are absent', () => {
        expect(WebSessionModel.parse({ user_data: {} }).getDisplayName()).toBe('User');
    });

    it('falls back to "User" when user_data is absent', () => {
        expect(WebSessionModel.parse({}).getDisplayName()).toBe('User');
    });
});

describe('WebSessionModel — getEmail() [UNIT]', () => {
    it('returns the email from user_data', () => {
        expect(makeSession().getEmail()).toBe('alice@example.com');
    });

    it('returns null when user_data.email is absent', () => {
        const m = WebSessionModel.parse({ user_data: { name: 'Alice' } });
        expect(m.getEmail()).toBeNull();
    });

    it('returns null when user_data is absent', () => {
        expect(WebSessionModel.parse({}).getEmail()).toBeNull();
    });
});

describe('WebSessionModel — isValid() [UNIT]', () => {
    it('returns true for a fully valid session', () => {
        expect(makeSession().isValid()).toBe(true);
    });

    it('returns false when is_active is false', () => {
        expect(makeSession({ is_active: false }).isValid()).toBe(false);
    });

    it('returns false when user_id is absent', () => {
        expect(makeSession({ user_id: null }).isValid()).toBe(false);
    });

    it('returns false when session is expired', () => {
        expect(makeSession({ expires_at: PAST }).isValid()).toBe(false);
    });

    it('returns true when expires_at is absent', () => {
        expect(makeSession({ expires_at: null }).isValid()).toBe(true);
    });
});

describe('WebSessionModel — hasRole() / isAdmin() [UNIT]', () => {
    it('hasRole() returns true when role is present', () => {
        expect(makeSession({ user_data: { roles: [UserRole.USER, UserRole.ADMIN] } }).hasRole(UserRole.ADMIN)).toBe(true);
    });

    it('hasRole() returns false when role is absent', () => {
        expect(makeSession({ user_data: { roles: [UserRole.USER] } }).hasRole(UserRole.ADMIN)).toBe(false);
    });

    it('hasRole() returns false when roles is empty array', () => {
        expect(WebSessionModel.parse({}).hasRole(UserRole.ADMIN)).toBe(false);
    });

    it('isAdmin() returns true for admin role', () => {
        expect(makeSession({ user_data: { roles: [UserRole.ADMIN] } }).isAdmin()).toBe(true);
    });

    it('isAdmin() returns true for superadmin role', () => {
        expect(makeSession({ user_data: { roles: [UserRole.SUPERADMIN] } }).isAdmin()).toBe(true);
    });

    it('isAdmin() returns false for plain user role', () => {
        expect(makeSession({ user_data: { roles: [UserRole.USER] } }).isAdmin()).toBe(false);
    });
});

describe('WebSessionModel — getApiKey() / getApiScopes() / hasScope() [UNIT]', () => {
    it('getApiKey() returns the api_key value', () => {
        expect(makeSession().getApiKey()).toBe('ak_test');
    });

    it('getApiKey() returns null when api_key is absent', () => {
        expect(WebSessionModel.parse({}).getApiKey()).toBeNull();
    });

    it('getApiScopes() returns scopes array', () => {
        const m = WebSessionModel.parse({ api_key_info: { scopes: ['read', 'write'] } });
        expect(m.getApiScopes()).toEqual(['read', 'write']);
    });

    it('getApiScopes() returns null when absent', () => {
        expect(WebSessionModel.parse({}).getApiScopes()).toBeNull();
    });

    it('hasScope() returns true when scope is present', () => {
        const m = WebSessionModel.parse({ api_key_info: { scopes: ['read', 'write'] } });
        expect(m.hasScope('read')).toBe(true);
    });

    it('hasScope() returns false when scope is absent', () => {
        const m = WebSessionModel.parse({ api_key_info: { scopes: ['read'] } });
        expect(m.hasScope('write')).toBe(false);
    });

    it('hasScope() returns false when scopes is null', () => {
        expect(WebSessionModel.parse({}).hasScope('read')).toBe(false);
    });
});

describe('WebSessionModel — getExpiresAt() / getMinutesUntilExpiry() [UNIT]', () => {
    it('getExpiresAt() returns a Date when expires_at is set', () => {
        expect(makeSession().getExpiresAt()).toBeInstanceOf(Date);
    });

    it('getExpiresAt() returns null when expires_at is absent', () => {
        expect(WebSessionModel.parse({}).getExpiresAt()).toBeNull();
    });

    it('getMinutesUntilExpiry() returns a positive number for a future session', () => {
        const mins = makeSession().getMinutesUntilExpiry();
        expect(typeof mins).toBe('number');
        expect(mins).toBeGreaterThan(0);
    });

    it('getMinutesUntilExpiry() returns null when expires_at is absent', () => {
        expect(WebSessionModel.parse({}).getMinutesUntilExpiry()).toBeNull();
    });

    it('getMinutesUntilExpiry() returns negative for an expired session', () => {
        const mins = makeSession({ expires_at: PAST }).getMinutesUntilExpiry();
        expect(mins).toBeLessThan(0);
    });
});

describe('WebSessionModel — getAvatar() [UNIT]', () => {
    it('returns null since avatar is now a Material Symbol', () => {
        expect(makeSession().getAvatar()).toBeNull();
    });
});
