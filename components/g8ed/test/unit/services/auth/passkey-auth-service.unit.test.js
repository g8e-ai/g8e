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

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { PasskeyAuthService, PASSKEY_CHALLENGE_TTL_SECONDS } from '@g8ed/services/auth/passkey_auth_service.js';
import { Collections } from '@g8ed/constants/collections.js';
import { HTTP_X_FORWARDED_HOST_HEADER, HTTP_X_FORWARDED_PROTO_HEADER } from '@g8ed/constants/headers.js';
import { AuthProvider } from '@g8ed/constants/auth.js';
import { getTestServices } from '@test/helpers/test-services.js';
import { TestCleanupHelper } from '@test/helpers/test-cleanup.js';
import { makeUserDoc, makePasskeyCredential } from '@test/fixtures/users.fixture.js';

// --- Helpers ---

function makeSettingsService(overrides = {}) {
    return {
        getPlatformSettings: vi.fn().mockResolvedValue({
            passkey_rp_id:   'localhost',
            passkey_origin:  'https://localhost',
            passkey_rp_name: 'g8e',
            ...overrides,
        }),
    };
}

function makeReq(overrides = {}) {
    return {
        hostname: 'localhost',
        protocol: 'https',
        get: vi.fn((h) => {
            if (h === 'host') return 'localhost';
            return null;
        }),
        ...overrides,
    };
}

// --- Mocks ---

vi.mock('@g8ed/utils/logger.js', () => ({
    logger: { info: vi.fn(), warn: vi.fn(), error: vi.fn(), debug: vi.fn() }
}));

vi.mock('@simplewebauthn/server', () => ({
    generateRegistrationOptions:   vi.fn(),
    verifyRegistrationResponse:    vi.fn(),
    generateAuthenticationOptions: vi.fn(),
    verifyAuthenticationResponse:  vi.fn(),
}));

vi.mock('@simplewebauthn/server/helpers', () => ({
    isoBase64URL: {
        fromBuffer: vi.fn((buf) => Buffer.from(buf).toString('base64url')),
    },
}));

import {
    generateRegistrationOptions,
    verifyRegistrationResponse,
    generateAuthenticationOptions,
    verifyAuthenticationResponse,
} from '@simplewebauthn/server';

// --- Tests ---

describe('PasskeyAuthService [UNIT]', () => {
    let services;
    let userService;
    let service;
    let cleanup;

    beforeEach(async () => {
        vi.clearAllMocks();
        try {
            services = await getTestServices();
            userService = services.userService;
            
            cleanup = new TestCleanupHelper(services.kvClient, services.cacheAsideService, {
                operatorsCollection: services.operatorService.collectionName
            });

            // Use real services from getTestServices() but mock the userService methods we need
            vi.spyOn(userService, 'getUser');
            vi.spyOn(userService, 'updateUser').mockResolvedValue({ success: true });
            vi.spyOn(userService, 'updateLastLogin').mockResolvedValue({ success: true });

            // Spy on cacheAsideService (which is a real CacheAsideService in getTestServices)
            vi.spyOn(services.cacheAsideService, 'createDocument');
            vi.spyOn(services.cacheAsideService, 'getDocument');
            vi.spyOn(services.cacheAsideService, 'deleteDocument');

            service = new PasskeyAuthService({
                userService,
                cacheAsideService: services.cacheAsideService,
                settingsService: makeSettingsService()
            });
        } catch (error) {
            console.error('Failed to initialize test services:', error);
            throw error;
        }
    });

    afterEach(async () => {
        if (cleanup) {
            await cleanup.cleanup();
        }
    });

    describe('constructor', () => {
        it('throws when userService is missing', () => {
            expect(() => new PasskeyAuthService({ cacheAsideService: services.cacheAsideService, settingsService: makeSettingsService() }))
                .toThrow(/requires userService/);
        });

        it('throws when cacheAsideService is missing', () => {
            expect(() => new PasskeyAuthService({ userService, settingsService: makeSettingsService() }))
                .toThrow(/requires cacheAsideService/);
        });

        it('throws when settingsService is missing', () => {
            expect(() => new PasskeyAuthService({ userService, cacheAsideService: services.cacheAsideService }))
                .toThrow(/requires settingsService/);
        });
    });

    describe('RP ID resolution', () => {
        it('uses settings passkey_rp_id when set (and not localhost)', async () => {
            const settingsService = makeSettingsService({ passkey_rp_id: 'g8e.local' });
            const svc = new PasskeyAuthService({ userService, cacheAsideService: services.cacheAsideService, settingsService });
            const user = makeUserDoc();
            generateRegistrationOptions.mockResolvedValueOnce({ challenge: 'ch' });

            await svc.generateRegistrationChallenge(makeReq({ hostname: 'wrong.ai' }), user);

            const call = generateRegistrationOptions.mock.calls[0][0];
            expect(call.rpID).toBe('g8e.local');
        });

        it('falls back to x-forwarded-host when settings is localhost', async () => {
            const req = makeReq({
                get: vi.fn((h) => h === HTTP_X_FORWARDED_HOST_HEADER ? 'forwarded.ai:8443' : null)
            });
            generateRegistrationOptions.mockResolvedValueOnce({ challenge: 'ch' });

            await service.generateRegistrationChallenge(req, makeUserDoc());

            const call = generateRegistrationOptions.mock.calls[0][0];
            expect(call.rpID).toBe('forwarded.ai');
        });

        it('falls back to req.hostname when settings is localhost and no header', async () => {
            const req = makeReq({ hostname: 'req-host.local' });
            generateRegistrationOptions.mockResolvedValueOnce({ challenge: 'ch' });

            await service.generateRegistrationChallenge(req, makeUserDoc());

            const call = generateRegistrationOptions.mock.calls[0][0];
            expect(call.rpID).toBe('req-host.local');
        });
    });

    describe('Origin resolution', () => {
        it('uses settings passkey_origin when set (and not localhost)', async () => {
            const settingsService = makeSettingsService({ passkey_origin: 'https://g8e.local' });
            const svc = new PasskeyAuthService({ userService, cacheAsideService: services.cacheAsideService, settingsService });
            
            services.cacheAsideService.getDocument.mockResolvedValueOnce({ challenge: 'stored-ch' });
            verifyRegistrationResponse.mockResolvedValueOnce({
                verified: true,
                registrationInfo: { credential: { id: 'id', publicKey: new Uint8Array([1]), counter: 0 } }
            });

            await svc.verifyRegistration(makeReq(), makeUserDoc(), {});

            const call = verifyRegistrationResponse.mock.calls[0][0];
            expect(call.expectedOrigin).toBe('https://g8e.local');
        });

        it('builds origin from request when settings is localhost', async () => {
            const req = makeReq({
                protocol: 'https',
                get: vi.fn((h) => {
                    if (h === HTTP_X_FORWARDED_PROTO_HEADER) return 'https';
                    if (h === HTTP_X_FORWARDED_HOST_HEADER) return 'forwarded.ai';
                    return null;
                })
            });
            services.cacheAsideService.getDocument.mockResolvedValueOnce({ challenge: 'stored-ch' });
            verifyRegistrationResponse.mockResolvedValueOnce({
                verified: true,
                registrationInfo: { credential: { id: 'id', publicKey: new Uint8Array([1]), counter: 0 } }
            });

            await service.verifyRegistration(req, makeUserDoc(), {});

            const call = verifyRegistrationResponse.mock.calls[0][0];
            expect(call.expectedOrigin).toBe('https://forwarded.ai');
        });
    });

    describe('generateRegistrationChallenge', () => {
        it('calls generateRegistrationOptions with correct user data and Buffer userID', async () => {
            const user = makeUserDoc();
            generateRegistrationOptions.mockResolvedValueOnce({ challenge: 'reg-ch' });

            await service.generateRegistrationChallenge(makeReq(), user);

            expect(generateRegistrationOptions).toHaveBeenCalledWith(expect.objectContaining({
                userName: user.email,
                userDisplayName: user.name,
                userID: expect.any(Buffer)
            }));
        });

        it('stores challenge in PASSKEY_CHALLENGES collection with correct TTL', async () => {
            const user = makeUserDoc();
            generateRegistrationOptions.mockResolvedValueOnce({ challenge: 'stored-ch' });

            await service.generateRegistrationChallenge(makeReq(), user);

            expect(services.cacheAsideService.createDocument).toHaveBeenCalledWith(
                Collections.PASSKEY_CHALLENGES,
                user.id,
                { challenge: 'stored-ch' },
                PASSKEY_CHALLENGE_TTL_SECONDS
            );
        });

        it('excludes existing credentials', async () => {
            const cred = makePasskeyCredential();
            const user = makeUserDoc({ passkey_credentials: [cred] });
            generateRegistrationOptions.mockResolvedValueOnce({ challenge: 'ch' });

            await service.generateRegistrationChallenge(makeReq(), user);

            const call = generateRegistrationOptions.mock.calls[0][0];
            expect(call.excludeCredentials).toHaveLength(1);
            expect(call.excludeCredentials[0].id).toBeDefined();
        });
    });

    describe('verifyRegistration', () => {
        it('returns verified: false if challenge is missing', async () => {
            services.cacheAsideService.getDocument.mockResolvedValueOnce(null);
            const result = await service.verifyRegistration(makeReq(), makeUserDoc(), {});
            expect(result.verified).toBe(false);
            expect(result.error).toMatch(/expired/);
        });

        it('saves new credential and marks user as PASSKEY provider on success', async () => {
            const user = makeUserDoc();
            services.cacheAsideService.getDocument.mockResolvedValueOnce({ challenge: 'valid-ch' });
            verifyRegistrationResponse.mockResolvedValueOnce({
                verified: true,
                registrationInfo: {
                    credential: { id: 'new-id', publicKey: new Uint8Array([1, 2, 3]), counter: 0 }
                }
            });

            const result = await service.verifyRegistration(makeReq(), user, { response: { transports: ['usb'] } });

            expect(result.verified).toBe(true);
            expect(services.cacheAsideService.deleteDocument).toHaveBeenCalledWith(Collections.PASSKEY_CHALLENGES, user.id);
            expect(userService.updateUser).toHaveBeenCalledWith(user.id, expect.objectContaining({
                provider: AuthProvider.PASSKEY,
                passkey_credentials: expect.arrayContaining([
                    expect.objectContaining({ id: 'new-id', transports: ['usb'] })
                ])
            }));
        });
    });

    describe('generateAuthenticationChallenge', () => {
        it('returns null if user has no passkeys', async () => {
            const result = await service.generateAuthenticationChallenge(makeReq(), makeUserDoc({ passkey_credentials: [] }));
            expect(result).toBeNull();
        });

        it('calls generateAuthenticationOptions and stores challenge in PASSKEY_CHALLENGES collection', async () => {
            const user = makeUserDoc({ passkey_credentials: [makePasskeyCredential()] });
            generateAuthenticationOptions.mockResolvedValueOnce({ challenge: 'auth-ch' });

            const result = await service.generateAuthenticationChallenge(makeReq(), user);

            expect(result.challenge).toBe('auth-ch');
            expect(services.cacheAsideService.createDocument).toHaveBeenCalledWith(
                Collections.PASSKEY_CHALLENGES,
                user.id,
                { challenge: 'auth-ch' },
                PASSKEY_CHALLENGE_TTL_SECONDS
            );
        });
    });

    describe('verifyAuthentication', () => {
        it('returns verified: false if challenge missing', async () => {
            services.cacheAsideService.getDocument.mockResolvedValueOnce(null);
            const result = await service.verifyAuthentication(makeReq(), makeUserDoc(), { id: 'some-id' });
            expect(result.verified).toBe(false);
        });

        it('returns verified: false if credential not found on user', async () => {
            services.cacheAsideService.getDocument.mockResolvedValueOnce({ challenge: 'ch' });
            const result = await service.verifyAuthentication(makeReq(), makeUserDoc({ passkey_credentials: [] }), { id: 'unknown' });
            expect(result.verified).toBe(false);
            expect(result.error).toMatch(/not recognized/);
        });

        it('updates counter and last_used_at on success', async () => {
            const cred = makePasskeyCredential({ id: 'my-id', counter: 10 });
            const user = makeUserDoc({ passkey_credentials: [cred] });
            services.cacheAsideService.getDocument.mockResolvedValueOnce({ challenge: 'ch' });
            verifyAuthenticationResponse.mockResolvedValueOnce({
                verified: true,
                authenticationInfo: { newCounter: 11 }
            });

            const result = await service.verifyAuthentication(makeReq(), user, { id: 'my-id' });

            expect(result.verified).toBe(true);
            expect(services.cacheAsideService.deleteDocument).toHaveBeenCalledWith(Collections.PASSKEY_CHALLENGES, user.id);
            const update = userService.updateUser.mock.calls[0][1];
            expect(update.passkey_credentials[0].counter).toBe(11);
            expect(update.passkey_credentials[0].last_used_at).toBeInstanceOf(Date);
            expect(userService.updateLastLogin).toHaveBeenCalledWith(user.id);
        });
    });

    describe('Credential Management', () => {
        it('listCredentials returns public info only', async () => {
            const cred = makePasskeyCredential({ id: 'id1', public_key: 'secret' });
            userService.getUser.mockResolvedValueOnce(makeUserDoc({ passkey_credentials: [cred] }));

            const list = await service.listCredentials('uid');
            expect(list[0].id).toBe('id1');
            expect(list[0].public_key).toBeUndefined();
        });

        it('revokeCredential removes one', async () => {
            const cred1 = makePasskeyCredential({ id: 'id1' });
            const cred2 = makePasskeyCredential({ id: 'id2' });
            userService.getUser.mockResolvedValueOnce(makeUserDoc({ id: 'uid', passkey_credentials: [cred1, cred2] }));

            const res = await service.revokeCredential('uid', 'id1');
            expect(res.remaining).toBe(1);
            expect(userService.updateUser).toHaveBeenCalledWith('uid', {
                passkey_credentials: [expect.objectContaining({ id: 'id2' })]
            });
        });

        it('revokeAllCredentials clears array', async () => {
            userService.getUser.mockResolvedValueOnce(makeUserDoc({ 
                id: 'uid', 
                passkey_credentials: [makePasskeyCredential(), makePasskeyCredential()] 
            }));
            const res = await service.revokeAllCredentials('uid');
            expect(res.revoked).toBe(2);
            expect(userService.updateUser).toHaveBeenCalledWith('uid', { passkey_credentials: [] });
        });
    });
});
