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
import { createMockInternalHttpClient } from '@test/mocks/internal-http-client.mock.js';
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
    let mockInternalHttpClient;

    beforeEach(async () => {
        vi.clearAllMocks();
        try {
            services = await getTestServices();
            userService = services.userService;
            mockInternalHttpClient = createMockInternalHttpClient();
            
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
            vi.spyOn(services.cacheAsideService, 'evictDocument');

            service = new PasskeyAuthService({
                userService,
                cacheAsideService: services.cacheAsideService,
                settingsService: makeSettingsService(),
                internalHttpClient: mockInternalHttpClient
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
            expect(() => new PasskeyAuthService({ cacheAsideService: services.cacheAsideService, settingsService: makeSettingsService(), internalHttpClient: mockInternalHttpClient }))
                .toThrow(/requires userService/);
        });

        it('throws when cacheAsideService is missing', () => {
            expect(() => new PasskeyAuthService({ userService, settingsService: makeSettingsService(), internalHttpClient: mockInternalHttpClient }))
                .toThrow(/requires cacheAsideService/);
        });

        it('throws when settingsService is missing', () => {
            expect(() => new PasskeyAuthService({ userService, cacheAsideService: services.cacheAsideService, internalHttpClient: mockInternalHttpClient }))
                .toThrow(/requires settingsService/);
        });

        it('throws when internalHttpClient is missing', () => {
            expect(() => new PasskeyAuthService({ userService, cacheAsideService: services.cacheAsideService, settingsService: makeSettingsService() }))
                .toThrow(/requires internalHttpClient/);
        });
    });

    describe('generateRegistrationChallenge', () => {
        it('calls substrate passkeyRegisterChallenge with correct user data', async () => {
            const user = makeUserDoc();
            mockInternalHttpClient.passkeyRegisterChallenge.mockResolvedValue({
                success: true,
                challenge: 'reg-ch'
            });

            const result = await service.generateRegistrationChallenge(makeReq(), user);

            expect(result.challenge).toBe('reg-ch');
            expect(mockInternalHttpClient.passkeyRegisterChallenge).toHaveBeenCalledWith(expect.objectContaining({
                user_id: user.id,
                email: user.email
            }));
        });
    });

    describe('verifyRegistration', () => {
        it('returns verified: true and evicts user cache on success', async () => {
            const user = makeUserDoc();
            mockInternalHttpClient.passkeyRegisterVerify.mockResolvedValue({
                success: true,
                credential: { id: 'new-id' }
            });

            const attestationResponse = {
                id: 'id',
                rawId: 'rawId',
                response: {
                    clientDataJSON: 'cdj',
                    attestationObject: 'ao',
                    transports: ['usb']
                }
            };

            const result = await service.verifyRegistration(makeReq(), user, attestationResponse);

            expect(result.verified).toBe(true);
            expect(mockInternalHttpClient.passkeyRegisterVerify).toHaveBeenCalledWith(expect.objectContaining({
                user_id: user.id,
                attestation_response: expect.objectContaining({
                    id: 'id'
                })
            }));
            expect(services.cacheAsideService.evictDocument).toHaveBeenCalledWith(Collections.USERS, user.id);
        });

        it('returns verified: false if substrate fails', async () => {
            const user = makeUserDoc();
            mockInternalHttpClient.passkeyRegisterVerify.mockResolvedValue({
                success: false,
                error: 'Substrate error'
            });

            const result = await service.verifyRegistration(makeReq(), user, { response: {} });

            expect(result.verified).toBe(false);
            expect(result.error).toBe('Substrate error');
        });
    });

    describe('generateAuthenticationChallenge', () => {
        it('calls substrate passkeyAuthChallenge and returns response', async () => {
            const user = makeUserDoc();
            mockInternalHttpClient.passkeyAuthChallenge.mockResolvedValue({
                success: true,
                challenge: 'auth-ch'
            });

            const result = await service.generateAuthenticationChallenge(makeReq(), user);

            expect(result.challenge).toBe('auth-ch');
            expect(mockInternalHttpClient.passkeyAuthChallenge).toHaveBeenCalledWith(expect.objectContaining({
                user_id: user.id,
                email: user.email
            }));
        });

        it('returns null if user has no passkeys (needs_setup)', async () => {
            const user = makeUserDoc();
            mockInternalHttpClient.passkeyAuthChallenge.mockResolvedValue({
                success: false,
                needs_setup: true
            });

            const result = await service.generateAuthenticationChallenge(makeReq(), user);

            expect(result).toBeNull();
        });
    });

    describe('verifyAuthentication', () => {
        it('returns verified: true and evicts user cache on success', async () => {
            const user = makeUserDoc();
            const assertionResponse = {
                id: 'cred-id',
                rawId: 'rawId',
                response: {
                    clientDataJSON: 'cdj',
                    authenticatorData: 'ad',
                    signature: 'sig',
                    userHandle: 'uh'
                }
            };

            mockInternalHttpClient.passkeyAuthVerify.mockResolvedValue({
                success: true,
                session_id: 'new-web-sess'
            });

            const result = await service.verifyAuthentication(makeReq(), user, assertionResponse);

            expect(result.verified).toBe(true);
            expect(result.session_id).toBe('new-web-sess');
            expect(mockInternalHttpClient.passkeyAuthVerify).toHaveBeenCalledWith(expect.objectContaining({
                user_id: user.id,
                assertion_response: expect.objectContaining({
                    id: 'cred-id'
                })
            }));
            expect(services.cacheAsideService.evictDocument).toHaveBeenCalledWith(Collections.USERS, user.id);
        });

        it('returns verified: false if substrate fails', async () => {
            const user = makeUserDoc();
            mockInternalHttpClient.passkeyAuthVerify.mockResolvedValue({
                success: false,
                error: 'Invalid signature'
            });

            const result = await service.verifyAuthentication(makeReq(), user, { id: 'id', response: {} });

            expect(result.verified).toBe(false);
            expect(result.error).toBe('Invalid signature');
        });
    });

    describe('Credential Management', () => {
        it('listCredentials returns credentials from substrate', async () => {
            const mockCredentials = [{ id: 'id1' }, { id: 'id2' }];
            mockInternalHttpClient.passkeyCredentials.mockResolvedValue({
                success: true,
                credentials: mockCredentials
            });

            const list = await service.listCredentials('uid');
            expect(list).toEqual(mockCredentials);
            expect(mockInternalHttpClient.passkeyCredentials).toHaveBeenCalledWith('uid');
        });

        it('revokeCredential calls substrate and evicts user cache', async () => {
            mockInternalHttpClient.passkeyRevokeCredential.mockResolvedValue({
                success: true,
                found: true,
                remaining: 1
            });

            const res = await service.revokeCredential('uid', 'id1');
            expect(res.remaining).toBe(1);
            expect(mockInternalHttpClient.passkeyRevokeCredential).toHaveBeenCalledWith('id1', 'uid');
            expect(services.cacheAsideService.evictDocument).toHaveBeenCalledWith(Collections.USERS, 'uid');
        });

        it('revokeAllCredentials still uses direct userService (app-layer responsibility)', async () => {
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
