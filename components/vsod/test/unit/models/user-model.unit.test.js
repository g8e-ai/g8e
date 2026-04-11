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
import { UserDocument, PasskeyCredential } from '@vsod/models/user_model.js';
import { UserRole, AuthProvider } from '@vsod/constants/auth.js';

function makeBaseUser(overrides = {}) {
    return {
        id: 'user-001',
        email: 'test@example.com',
        ...overrides,
    };
}

describe('UserDocument [UNIT]', () => {
    // -------------------------------------------------------------------------
    // Required fields
    // -------------------------------------------------------------------------

    describe('required fields', () => {
        it('throws when id is missing', () => {
            expect(() => UserDocument.parse({ email: 'test@example.com' })).toThrow();
        });

        it('throws when email is missing', () => {
            expect(() => UserDocument.parse({ id: 'user-001' })).toThrow();
        });

        it('parses with only required fields', () => {
            const doc = UserDocument.parse(makeBaseUser());
            expect(doc.id).toBe('user-001');
            expect(doc.email).toBe('test@example.com');
        });
    });

    // -------------------------------------------------------------------------
    // Defaults
    // -------------------------------------------------------------------------

    describe('defaults', () => {
        it('defaults name to null', () => {
            expect(UserDocument.parse(makeBaseUser()).name).toBeNull();
        });

        it('defaults passkey_credentials to empty array', () => {
            expect(UserDocument.parse(makeBaseUser()).passkey_credentials).toEqual([]);
        });

        it('defaults passkey_challenge to null', () => {
            expect(UserDocument.parse(makeBaseUser()).passkey_challenge).toBeNull();
        });

        it('defaults g8e_key to null', () => {
            expect(UserDocument.parse(makeBaseUser()).g8e_key).toBeNull();
        });

        it('defaults organization_id to null', () => {
            expect(UserDocument.parse(makeBaseUser()).organization_id).toBeNull();
        });

        it('defaults roles to [UserRole.USER]', () => {
            const doc = UserDocument.parse(makeBaseUser());
            expect(doc.roles).toEqual([UserRole.USER]);
        });

        it('defaults operator_id to null', () => {
            expect(UserDocument.parse(makeBaseUser()).operator_id).toBeNull();
        });

        it('defaults operator_status to null', () => {
            expect(UserDocument.parse(makeBaseUser()).operator_status).toBeNull();
        });

        it('defaults provider to AuthProvider.PASSKEY', () => {
            expect(UserDocument.parse(makeBaseUser()).provider).toBe(AuthProvider.PASSKEY);
        });

        it('defaults sessions to empty array', () => {
            expect(UserDocument.parse(makeBaseUser()).sessions).toEqual([]);
        });

        it('defaults profile_picture to null', () => {
            expect(UserDocument.parse(makeBaseUser()).profile_picture).toBeNull();
        });

        it('defaults dev_logs_enabled to true', () => {
            expect(UserDocument.parse(makeBaseUser()).dev_logs_enabled).toBe(true);
        });
    });

    // -------------------------------------------------------------------------
    // Field assignment
    // -------------------------------------------------------------------------

    describe('field assignment', () => {
        it('assigns all provided fields', () => {
            const doc = UserDocument.parse({
                id: 'user-002',
                email: 'admin@example.com',
                name: 'Admin User',
                roles: [UserRole.SUPERADMIN],
                organization_id: 'org-001',
                operator_id: 'op-001',
                operator_status: 'active',
                provider: AuthProvider.PASSKEY,
                dev_logs_enabled: false,
            });

            expect(doc.name).toBe('Admin User');
            expect(doc.roles).toContain(UserRole.SUPERADMIN);
            expect(doc.organization_id).toBe('org-001');
            expect(doc.operator_id).toBe('op-001');
            expect(doc.dev_logs_enabled).toBe(false);
        });

        it('coerces dev_logs_enabled from truthy string', () => {
            const doc = UserDocument.parse({ ...makeBaseUser(), dev_logs_enabled: 'true' });
            expect(doc.dev_logs_enabled).toBe(true);
        });

        it('coerces dev_logs_enabled from falsy string', () => {
            const doc = UserDocument.parse({ ...makeBaseUser(), dev_logs_enabled: 'false' });
            expect(doc.dev_logs_enabled).toBe(false);
        });

        it('parses passkey_credentials as PasskeyCredential instances', () => {
            const cred = {
                id: 'cred-001',
                public_key: 'base64pubkey',
                counter: 0,
                transports: ['usb'],
            };
            const doc = UserDocument.parse({ ...makeBaseUser(), passkey_credentials: [cred] });
            expect(doc.passkey_credentials).toHaveLength(1);
            expect(doc.passkey_credentials[0]).toBeInstanceOf(PasskeyCredential);
            expect(doc.passkey_credentials[0].id).toBe('cred-001');
        });
    });

    // -------------------------------------------------------------------------
    // forClient
    // -------------------------------------------------------------------------

    describe('forClient', () => {
        it('omits passkey_credentials', () => {
            const doc = UserDocument.parse({
                ...makeBaseUser(),
                passkey_credentials: [{ id: 'cred-001', public_key: 'key', counter: 0 }],
            });
            const client = doc.forClient();
            expect(client.passkey_credentials).toBeUndefined();
        });

        it('omits passkey_challenge', () => {
            const doc = UserDocument.parse({ ...makeBaseUser(), passkey_challenge: 'challenge-abc' });
            expect(doc.forClient().passkey_challenge).toBeUndefined();
        });

        it('omits passkey_challenge_expires_at', () => {
            const doc = UserDocument.parse({ ...makeBaseUser(), passkey_challenge_expires_at: new Date() });
            expect(doc.forClient().passkey_challenge_expires_at).toBeUndefined();
        });

        it('omits g8e_key', () => {
            const doc = UserDocument.parse({ ...makeBaseUser(), g8e_key: 'g8e_secret' });
            expect(doc.forClient().g8e_key).toBeUndefined();
        });

        it('omits sessions', () => {
            const doc = UserDocument.parse({ ...makeBaseUser(), sessions: ['session-001'] });
            expect(doc.forClient().sessions).toBeUndefined();
        });

        it('retains id, email, name, roles, organization_id', () => {
            const doc = UserDocument.parse({
                id: 'user-client-001',
                email: 'client@example.com',
                name: 'Client User',
                roles: [UserRole.ADMIN],
                organization_id: 'org-client',
            });
            const client = doc.forClient();
            expect(client.id).toBe('user-client-001');
            expect(client.email).toBe('client@example.com');
            expect(client.name).toBe('Client User');
            expect(client.roles).toContain(UserRole.ADMIN);
            expect(client.organization_id).toBe('org-client');
        });

        it('retains dev_logs_enabled', () => {
            const doc = UserDocument.parse({ ...makeBaseUser(), dev_logs_enabled: false });
            expect(doc.forClient().dev_logs_enabled).toBe(false);
        });

        it('does not mutate the original model', () => {
            const doc = UserDocument.parse({ ...makeBaseUser(), passkey_credentials: [{ id: 'c', public_key: 'k', counter: 0 }] });
            doc.forClient();
            expect(doc.passkey_credentials).toHaveLength(1);
        });
    });

    // -------------------------------------------------------------------------
    // forDB / forWire round-trip
    // -------------------------------------------------------------------------

    describe('forDB / forWire round-trip', () => {
        it('round-trips through forDB and parse', () => {
            const original = UserDocument.parse({
                id: 'user-rt-001',
                email: 'roundtrip@example.com',
                name: 'RT User',
                roles: [UserRole.SUPERADMIN],
            });

            const restored = UserDocument.parse(original.forDB());

            expect(restored.id).toBe(original.id);
            expect(restored.email).toBe(original.email);
            expect(restored.name).toBe(original.name);
            expect(restored.roles).toEqual(original.roles);
        });
    });
});

// -------------------------------------------------------------------------
// PasskeyCredential
// -------------------------------------------------------------------------

describe('PasskeyCredential [UNIT]', () => {
    it('throws on missing required id', () => {
        expect(() => PasskeyCredential.parse({ public_key: 'key', counter: 0 })).toThrow();
    });

    it('throws on missing required public_key', () => {
        expect(() => PasskeyCredential.parse({ id: 'cred-001', counter: 0 })).toThrow();
    });

    it('throws on missing required counter', () => {
        expect(() => PasskeyCredential.parse({ id: 'cred-001', public_key: 'key' })).toThrow();
    });

    it('parses with required fields', () => {
        const cred = PasskeyCredential.parse({ id: 'cred-001', public_key: 'base64key', counter: 5 });
        expect(cred.id).toBe('cred-001');
        expect(cred.public_key).toBe('base64key');
        expect(cred.counter).toBe(5);
    });

    it('defaults transports to empty array', () => {
        const cred = PasskeyCredential.parse({ id: 'cred-001', public_key: 'key', counter: 0 });
        expect(cred.transports).toEqual([]);
    });

    it('defaults last_used_at to null', () => {
        const cred = PasskeyCredential.parse({ id: 'cred-001', public_key: 'key', counter: 0 });
        expect(cred.last_used_at).toBeNull();
    });

    it('defaults created_at to current time', () => {
        const before = Date.now();
        const cred = PasskeyCredential.parse({ id: 'cred-001', public_key: 'key', counter: 0 });
        const after = Date.now();
        expect(cred.created_at).toBeInstanceOf(Date);
        expect(cred.created_at.getTime()).toBeGreaterThanOrEqual(before);
        expect(cred.created_at.getTime()).toBeLessThanOrEqual(after);
    });

    it('accepts transports array', () => {
        const cred = PasskeyCredential.parse({ id: 'cred-001', public_key: 'key', counter: 0, transports: ['usb', 'nfc'] });
        expect(cred.transports).toEqual(['usb', 'nfc']);
    });
});
