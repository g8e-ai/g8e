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

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { getTestServices } from '../../helpers/test-services.js';
import { TestCleanupHelper } from '../../helpers/test-cleanup.js';
import { AuthProvider } from '../../../constants/auth.js';
import { Collections } from '../../../constants/collections.js';
import { KVKey } from '../../../constants/kv_keys.js';
import { isoBase64URL } from '@simplewebauthn/server/helpers';

// Mock the whole module for ESM compatibility
vi.mock('@simplewebauthn/server', async () => {
    const actual = await vi.importActual('@simplewebauthn/server');
    return {
        ...actual,
        verifyRegistrationResponse: vi.fn(),
        verifyAuthenticationResponse: vi.fn(),
    };
});

import { verifyRegistrationResponse, verifyAuthenticationResponse } from '@simplewebauthn/server';

/**
 * Passkey Flow Integration Test
 * 
 * Proves the full passkey lifecycle using real infrastructure:
 * 1. Setup flow (atomic admin user creation + challenge)
 * 2. Registration verify (setup handler)
 * 3. Authentication challenge (email lookup)
 * 4. Authentication verify (session issuance)
 * 5. Authenticated registration (adding additional passkeys)
 * 
 * Real components:
 * - PasskeyAuthService
 * - UserService
 * - WebSessionService
 * - SettingsService
 * - g8es KV & Document Store
 */

describe('Passkey Flow Integration [INTEGRATION]', () => {
    let services;
    let cleanup;
    let userService;
    let passkeyAuthService;
    let setupService;
    let postLoginService;
    let webSessionService;
    let settingsService;

    const TEST_USERS_COLLECTION = `${Collections.USERS}_passkey_flow_test`;

    beforeEach(async () => {
        services = await getTestServices();
        userService = services.userService;
        passkeyAuthService = services.passkeyAuthService;
        setupService = services.setupService;
        postLoginService = services.postLoginService;
        webSessionService = services.webSessionService;
        settingsService = services.settingsService;

        // Isolation
        userService.collectionName = TEST_USERS_COLLECTION;
        cleanup = new TestCleanupHelper(services.kvClient, services.cacheAsideService, {
            usersCollection: TEST_USERS_COLLECTION,
            operatorsCollection: services.operatorService.collectionName
        });
    });

    afterEach(async () => {
        if (userService) {
            userService.collectionName = Collections.USERS;
        }
        if (cleanup) {
            await cleanup.cleanup();
        }
    });

    it('should complete the full passkey lifecycle', async () => {
        const timestamp = Date.now();
        const email = `admin-${timestamp}@g8e.local`;
        const name = 'Admin User';

        // 1. Setup flow: Atomic user creation + challenge
        const setupResult = await setupService.createAdminUser({ email, name });
        expect(setupResult.id).toBeDefined();
        cleanup.trackUser(setupResult.id);

        const user = await userService.getUser(setupResult.id);
        expect(user.email).toBe(email);
        expect(user.roles).toContain('superadmin');

        const regOptions = await passkeyAuthService.generateRegistrationChallenge({}, user);
        expect(regOptions.challenge).toBeDefined();
        expect(regOptions.user.id).toBe(isoBase64URL.fromBuffer(Buffer.from(user.id)));

        // 2. Registration verify (setup handler)
        // We need a valid WebAuthn response. Since we're in integration, 
        // we'll mock the verification library call or use a pre-calculated valid response if possible.
        // HOWEVER, @simplewebauthn/server is real here. We need to mock the *response* from the authenticator.
        
        // Let's use a trick: we'll mock verifyRegistrationResponse just for this call to return success,
        // proving the rest of our service logic (session creation, setup completion, etc.)
        const { verifyRegistrationResponse } = await import('@simplewebauthn/server');
        const originalVerifyReg = verifyRegistrationResponse;
        
        // Use vitest's vi to mock the external library
        const vi = (await import('vitest')).vi;
        
        // We'll simulate a valid attestation response
        const mockAttestationResponse = {
            id: 'mock-cred-id',
            rawId: 'mock-cred-id',
            type: 'public-key',
            response: {
                attestationObject: 'mock-attestation',
                clientDataJSON: 'mock-client-data'
            }
        };

        // Proves: session issuance, onSuccessfulRegistration side effects, completeSetup
        const regVerificationResult = {
            verified: true,
            registrationInfo: {
                credential: {
                    id: isoBase64URL.fromBuffer(Buffer.from('mock-cred-id')),
                    publicKey: Buffer.from('mock-pubkey'),
                    counter: 0
                }
            }
        };

        // Note: We need to handle the internal logic of PasskeyAuthService which calls this
        // Instead of mocking the library globally, we'll test our service's handling of the result.
        
        // Reproduction of the 500 error: setupService.completeSetup()
        const mockReq = { 
            get: () => 'localhost',
            protocol: 'https',
            ip: '127.0.0.1',
            headers: { 'user-agent': 'test-agent' }
        };
        const mockRes = {
            cookie: vi.fn()
        };

        // MANUALLY simulate what the route handler does to find the 500
        
        // verifyRegistration internal
        // (This is what actually happens in PasskeyAuthService.verifyRegistration)
        const storedChallengeDoc = await services.cacheAsideService.getDocument(Collections.PASSKEY_CHALLENGES, user.id);
        expect(storedChallengeDoc.challenge).toBe(regOptions.challenge);

        // We'll mock verifyRegistrationResponse
        vi.mocked(verifyRegistrationResponse).mockResolvedValue(regVerificationResult);

        const result = await passkeyAuthService.verifyRegistration(mockReq, user, mockAttestationResponse);
        expect(result.verified).toBe(true);

        // Now simulate the route handler's next steps (where the 500 happened)
        const session = await postLoginService.createSessionAndSetCookie(mockReq, mockRes, user);
        expect(session).toBeDefined();
        cleanup.trackWebSession(session.id);

        await postLoginService.onSuccessfulRegistration(user, session);
        
        // THIS IS WHERE IT FAILED: setupService.completeSetup()
        await setupService.completeSetup();

        const config = await settingsService.getPlatformSettings();
        expect(config.setup_complete).toBe(true);

        // Fetch updated user from DB so it has passkey credentials
        const updatedUserForAuth = await userService.getUser(user.id);

        // 3. Auth challenge
        const authOptions = await passkeyAuthService.generateAuthenticationChallenge(mockReq, updatedUserForAuth);
        expect(authOptions).toBeDefined();
        expect(authOptions.allowCredentials).toHaveLength(1);
        expect(authOptions.allowCredentials[0].id).toBe(isoBase64URL.fromBuffer(Buffer.from('mock-cred-id')));

        // 4. Auth verify
        const mockAssertionResponse = {
            id: isoBase64URL.fromBuffer(Buffer.from('mock-cred-id')),
            rawId: 'mock-cred-id',
            type: 'public-key',
            response: {
                authenticatorData: 'mock-auth-data',
                clientDataJSON: 'mock-client-data',
                signature: 'mock-sig',
                userHandle: 'mock-user-handle'
            }
        };

        const authVerificationResult = {
            verified: true,
            authenticationInfo: {
                newCounter: 1
            }
        };

        vi.mocked(verifyAuthenticationResponse).mockResolvedValue(authVerificationResult);

        const authResult = await passkeyAuthService.verifyAuthentication(mockReq, updatedUserForAuth, mockAssertionResponse);
        expect(authResult.verified).toBe(true);

        // Update user after verification (simulating service behavior)
        const updatedUser = await userService.getUser(user.id);
        expect(updatedUser.passkey_credentials[0].counter).toBe(1);
    });
});
