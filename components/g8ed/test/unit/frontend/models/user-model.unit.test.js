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
import { UserDocument } from '@g8ed/public/js/models/user-model.js';
import { FrontendIdentifiableModel, FrontendBaseModel } from '@g8ed/public/js/models/base.js';
import { UserRole, AuthProvider } from '@g8ed/public/js/constants/auth-constants.js';

function makeUser(overrides = {}) {
    return UserDocument.parse({
        id:              'user-abc',
        email:           'alice@example.com',
        name:            'Alice',
        organization_id: 'org-abc',
        roles:           [UserRole.USER],
        ...overrides,
    });
}

describe('UserDocument — parse() [UNIT]', () => {
    it('parse() returns a UserDocument instance', () => {
        expect(makeUser()).toBeInstanceOf(UserDocument);
    });

    it('parse() returns a FrontendIdentifiableModel instance', () => {
        expect(makeUser()).toBeInstanceOf(FrontendIdentifiableModel);
    });

    it('parse() returns a FrontendBaseModel instance', () => {
        expect(makeUser()).toBeInstanceOf(FrontendBaseModel);
    });

    it('parse() requires id', () => {
        expect(() => UserDocument.parse({ email: 'a@b.com' })).toThrow();
    });

    it('parse() requires email', () => {
        expect(() => UserDocument.parse({ id: 'user-1' })).toThrow();
    });

    it('parse() strips unknown fields', () => {
        const m = UserDocument.parse({ id: 'u1', email: 'a@b.com', secret: 'drop' });
        expect(m).not.toHaveProperty('secret');
    });

    it('parse() defaults name to null', () => {
        expect(UserDocument.parse({ id: 'u1', email: 'a@b.com' }).name).toBeNull();
    });

    it('parse() defaults roles to [UserRole.USER]', () => {
        expect(UserDocument.parse({ id: 'u1', email: 'a@b.com' }).roles).toEqual([UserRole.USER]);
    });

    it('parse() defaults provider to AuthProvider.LOCAL', () => {
        expect(UserDocument.parse({ id: 'u1', email: 'a@b.com' }).provider).toBe(AuthProvider.LOCAL);
    });

    it('parse() defaults operator_id to null', () => {
        expect(UserDocument.parse({ id: 'u1', email: 'a@b.com' }).operator_id).toBeNull();
    });

    it('parse() defaults operator_status to null', () => {
        expect(UserDocument.parse({ id: 'u1', email: 'a@b.com' }).operator_status).toBeNull();
    });

    it('parse() coerces last_login string to Date', () => {
        const iso = new Date(Date.now() - 3600000).toISOString();
        const m = UserDocument.parse({ id: 'u1', email: 'a@b.com', last_login: iso });
        expect(m.last_login).toBeInstanceOf(Date);
    });

    it('parse() preserves roles array', () => {
        const m = UserDocument.parse({ id: 'u1', email: 'a@b.com', roles: [UserRole.ADMIN, UserRole.USER] });
        expect(m.roles).toEqual([UserRole.ADMIN, UserRole.USER]);
    });

    it('parse() sets password_hash absent — field not on model', () => {
        const m = UserDocument.parse({ id: 'u1', email: 'a@b.com', password_hash: 'hash' });
        expect(m).not.toHaveProperty('password_hash');
    });

    it('parse() sets g8e_key absent — field not on model', () => {
        const m = UserDocument.parse({ id: 'u1', email: 'a@b.com', g8e_key: 'key' });
        expect(m).not.toHaveProperty('g8e_key');
    });
});

describe('UserDocument — forWire() [UNIT]', () => {
    it('forWire() returns a plain object', () => {
        const wire = makeUser().forWire();
        expect(typeof wire).toBe('object');
        expect(wire).not.toBeInstanceOf(UserDocument);
    });

    it('forWire() serializes last_login Date to ISO string', () => {
        const iso = new Date(Date.now() - 3600000).toISOString();
        const m = UserDocument.parse({ id: 'u1', email: 'a@b.com', last_login: iso });
        const wire = m.forWire();
        expect(typeof wire.last_login).toBe('string');
    });

    it('forWire() preserves id, email, name', () => {
        const wire = makeUser().forWire();
        expect(wire.id).toBe('user-abc');
        expect(wire.email).toBe('alice@example.com');
        expect(wire.name).toBe('Alice');
    });

    it('forWire() preserves roles', () => {
        const wire = makeUser({ roles: [UserRole.ADMIN] }).forWire();
        expect(wire.roles).toEqual([UserRole.ADMIN]);
    });
});

describe('UserDocument — getDisplayName() [UNIT]', () => {
    it('returns name when present', () => {
        expect(makeUser({ name: 'Alice' }).getDisplayName()).toBe('Alice');
    });

    it('falls back to email prefix when name is absent', () => {
        const m = UserDocument.parse({ id: 'u1', email: 'admin@example.com' });
        expect(m.getDisplayName()).toBe('admin');
    });

    it('falls back to "User" when email is empty after split', () => {
        const m = UserDocument.parse({ id: 'u1', email: '@example.com' });
        expect(m.getDisplayName()).toBe('User');
    });
});

describe('UserDocument — hasRole() / isAdmin() [UNIT]', () => {
    it('hasRole() returns true when role is present', () => {
        expect(makeUser({ roles: [UserRole.USER, UserRole.ADMIN] }).hasRole(UserRole.ADMIN)).toBe(true);
    });

    it('hasRole() returns false when role is absent', () => {
        expect(makeUser({ roles: [UserRole.USER] }).hasRole(UserRole.ADMIN)).toBe(false);
    });

    it('hasRole() returns false when roles is empty', () => {
        expect(makeUser({ roles: [] }).hasRole(UserRole.ADMIN)).toBe(false);
    });

    it('isAdmin() returns true for admin role', () => {
        expect(makeUser({ roles: [UserRole.ADMIN] }).isAdmin()).toBe(true);
    });

    it('isAdmin() returns true for superadmin role', () => {
        expect(makeUser({ roles: [UserRole.SUPERADMIN] }).isAdmin()).toBe(true);
    });

    it('isAdmin() returns false for plain user role', () => {
        expect(makeUser({ roles: [UserRole.USER] }).isAdmin()).toBe(false);
    });

    it('hasAnyRole() returns true when at least one role matches', () => {
        expect(makeUser({ roles: [UserRole.USER] }).hasAnyRole([UserRole.ADMIN, UserRole.USER])).toBe(true);
    });

    it('hasAnyRole() returns false when no roles match', () => {
        expect(makeUser({ roles: [UserRole.USER] }).hasAnyRole([UserRole.ADMIN, UserRole.SUPERADMIN])).toBe(false);
    });
});

describe('UserDocument — getAvatar() [UNIT]', () => {
    it('returns null when profile_picture is absent (avatar is now a Material Symbol)', () => {
        expect(makeUser().getAvatar()).toBeNull();
    });

    it('returns profile_picture when set', () => {
        expect(makeUser({ profile_picture: '/uploads/pic.png' }).getAvatar()).toBe('/uploads/pic.png');
    });
});

describe('UserDocument — dev_logs_enabled [UNIT]', () => {
    it('defaults to true when not provided', () => {
        expect(makeUser().dev_logs_enabled).toBe(true);
    });

    it('parses true correctly', () => {
        expect(makeUser({ dev_logs_enabled: true }).dev_logs_enabled).toBe(true);
    });

    it('parses false correctly', () => {
        expect(makeUser({ dev_logs_enabled: false }).dev_logs_enabled).toBe(false);
    });

    it('coerces string "true" to boolean true', () => {
        expect(makeUser({ dev_logs_enabled: 'true' }).dev_logs_enabled).toBe(true);
    });

    it('coerces string "false" to boolean false', () => {
        expect(makeUser({ dev_logs_enabled: 'false' }).dev_logs_enabled).toBe(false);
    });

    it('is included in forWire() output', () => {
        const wire = makeUser({ dev_logs_enabled: true }).forWire();
        expect(wire).toHaveProperty('dev_logs_enabled', true);
    });

    it('strips unknown fields but preserves dev_logs_enabled', () => {
        const m = UserDocument.parse({ id: 'u1', email: 'a@b.com', dev_logs_enabled: true, unknown_field: 'drop' });
        expect(m.dev_logs_enabled).toBe(true);
        expect(m).not.toHaveProperty('unknown_field');
    });
});
