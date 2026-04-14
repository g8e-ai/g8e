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

import { describe, it, expect, vi, beforeEach } from 'vitest';
import express from 'express';
import request from 'supertest';
import { createPasskeyRouter } from '@g8ed/routes/auth/passkey_routes.js';
import { PasskeyPaths } from '@g8ed/constants/api_paths.js';
import { ApiKeyError } from '@g8ed/constants/auth.js';

describe('PasskeyRoutes Unit Tests', () => {
    let app;
    let mockPasskeyAuthService;
    let mockUserService;
    let mockPostLoginService;
    let mockAuthMiddleware;
    let mockRateLimiters;
    let mockSetupService;

    beforeEach(() => {
        mockPasskeyAuthService = {
            generateRegistrationChallenge: vi.fn(),
            verifyRegistration: vi.fn(),
            generateAuthenticationChallenge: vi.fn(),
            verifyAuthentication: vi.fn()
        };
        mockUserService = {
            findUserByEmail: vi.fn(),
            getUser: vi.fn()
        };
        mockPostLoginService = {
            createSessionAndSetCookie: vi.fn(),
            onSuccessfulRegistration: vi.fn(),
            onSuccessfulLogin: vi.fn()
        };
        mockSetupService = {
            isFirstRun: vi.fn(),
            completeSetup: vi.fn()
        };

        mockAuthMiddleware = {
            requireAuth: (req, res, next) => {
                if (req.headers['x-test-auth'] === 'fail') {
                    return res.status(401).json({ success: false, error: 'Unauthorized' });
                }
                req.userId = req.headers['x-test-user-id'] || 'test-user-id';
                next();
            },
            requireFirstRun: async (req, res, next) => {
                const firstRun = await mockSetupService.isFirstRun();
                if (!firstRun) {
                    return next('route');
                }
                next();
            }
        };

        mockRateLimiters = {
            passkeyRateLimiter: (req, res, next) => next()
        };

        const router = createPasskeyRouter({
            services: {
                passkeyAuthService: mockPasskeyAuthService,
                userService: mockUserService,
                postLoginService: mockPostLoginService,
                setupService: mockSetupService
            },

            authMiddleware: mockAuthMiddleware,
            rateLimiters: mockRateLimiters
        });

        app = express();
        app.use(express.json());
        app.use('/api/auth/passkey', router);

        // Error handler
        app.use((err, req, res, next) => {
            if (err === 'route') return next();
            const status = typeof err.getHttpStatus === 'function' ? err.getHttpStatus() : (err.status || 500);
            res.status(status).json({
                error: {
                    message: err.message,
                    code: err.code
                }
            });
        });
    });

    describe(`POST ${PasskeyPaths.REGISTER_CHALLENGE}`, () => {
        it('issues challenge for valid user_id when authenticated', async () => {
            mockUserService.getUser.mockResolvedValue({ id: 'test-user-id' });
            mockPasskeyAuthService.generateRegistrationChallenge.mockResolvedValue({ challenge: 'abc' });

            const res = await request(app)
                .post('/api/auth/passkey/register-challenge')
                .set('x-test-user-id', 'test-user-id')
                .send({ user_id: 'test-user-id' });

            expect(res.status).toBe(200);
            expect(res.body.options.challenge).toBe('abc');
            expect(mockPasskeyAuthService.generateRegistrationChallenge).toHaveBeenCalled();
        });

        it('returns 403 if user_id does not match authenticated user', async () => {
            const res = await request(app)
                .post('/api/auth/passkey/register-challenge')
                .set('x-test-user-id', 'other-user')
                .send({ user_id: 'test-user-id' });

            expect(res.status).toBe(403);
            expect(res.body.error.message).toBe('Access denied');
        });

        it('returns 401 if not authenticated', async () => {
            const res = await request(app)
                .post('/api/auth/passkey/register-challenge')
                .set('x-test-auth', 'fail')
                .send({ user_id: 'test-user-id' });

            expect(res.status).toBe(401);
        });

        it('returns 404 if user not found', async () => {
            mockUserService.getUser.mockResolvedValue(null);

            const res = await request(app)
                .post('/api/auth/passkey/register-challenge')
                .set('x-test-user-id', 'non-existent')
                .send({ user_id: 'non-existent' });

            expect(res.status).toBe(404);
            expect(res.body.error.message).toContain('User not found');
        });
    });

    describe(`POST ${PasskeyPaths.REGISTER_VERIFY}`, () => {
        const attestationResponse = {
            id: 'cred-id',
            rawId: 'cred-id',
            type: 'public-key',
            response: {
                clientDataJSON: 'abc',
                attestationObject: 'def'
            }
        };

        it('setup flow: verifies and adds passkey, issuing session for first passkey during setup', async () => {
            const mockUser = { id: 'test-user-id', passkey_credentials: [] };
            const mockSession = { id: 'sess-id' };
            mockUserService.getUser.mockResolvedValue(mockUser);
            mockPasskeyAuthService.verifyRegistration.mockResolvedValue({ verified: true });
            mockPostLoginService.createSessionAndSetCookie.mockResolvedValue(mockSession);
            mockSetupService.isFirstRun.mockResolvedValue(true);

            const res = await request(app)
                .post('/api/auth/passkey/register-verify')
                .send({ 
                    user_id: 'test-user-id',
                    attestation_response: attestationResponse
                });

            expect(res.status).toBe(200);
            expect(res.body.session).toEqual(mockSession);
            expect(res.body.message).toBe('Setup complete');
            expect(mockPostLoginService.onSuccessfulRegistration).toHaveBeenCalledWith(mockUser, mockSession);
            expect(mockSetupService.completeSetup).toHaveBeenCalled();
        });

        it('setup flow: returns 403 if user already has passkeys', async () => {
            const mockUser = { id: 'test-user-id', passkey_credentials: [{ id: 'existing' }] };
            mockUserService.getUser.mockResolvedValue(mockUser);
            mockSetupService.isFirstRun.mockResolvedValue(true);

            const res = await request(app)
                .post('/api/auth/passkey/register-verify')
                .send({ 
                    user_id: 'test-user-id',
                    attestation_response: attestationResponse
                });

            expect(res.status).toBe(403);
            expect(res.body.error.message).toBe('Setup already complete');
        });

        it('initial registration flow: verifies and adds passkey for user with 0 credentials (post-setup)', async () => {
            const mockUser = { id: 'test-user-id', passkey_credentials: [] };
            const mockSession = { id: 'sess-id' };
            mockUserService.getUser.mockResolvedValue(mockUser);
            mockPasskeyAuthService.verifyRegistration.mockResolvedValue({ verified: true });
            mockPostLoginService.createSessionAndSetCookie.mockResolvedValue(mockSession);
            mockSetupService.isFirstRun.mockResolvedValue(false);

            const res = await request(app)
                .post('/api/auth/passkey/register-verify')
                .send({ 
                    user_id: 'test-user-id',
                    attestation_response: attestationResponse
                });

            expect(res.status).toBe(200);
            expect(res.body.session).toEqual(mockSession);
            expect(res.body.message).toBe('Passkey registered');
            expect(mockPostLoginService.onSuccessfulRegistration).toHaveBeenCalledWith(mockUser, mockSession);
        });

        it('authenticated flow: adds additional passkey for user with existing credentials', async () => {
            const mockUser = { id: 'test-user-id', passkey_credentials: [{ id: 'existing' }] };
            mockUserService.getUser.mockResolvedValue(mockUser);
            mockPasskeyAuthService.verifyRegistration.mockResolvedValue({ verified: true });
            mockSetupService.isFirstRun.mockResolvedValue(false);

            const res = await request(app)
                .post('/api/auth/passkey/register-verify')
                .set('x-test-user-id', 'test-user-id')
                .send({ 
                    user_id: 'test-user-id',
                    attestation_response: attestationResponse
                });

            expect(res.status).toBe(200);
            expect(res.body.session).toBeNull();
            expect(res.body.message).toBe('Passkey registered');
            expect(mockPostLoginService.createSessionAndSetCookie).not.toHaveBeenCalled();
        });

        it('authenticated flow: returns 403 if user_id does not match authenticated user', async () => {
            const mockUser = { id: 'test-user-id', passkey_credentials: [{ id: 'existing' }] };
            mockUserService.getUser.mockResolvedValue(mockUser);
            mockSetupService.isFirstRun.mockResolvedValue(false);

            const res = await request(app)
                .post('/api/auth/passkey/register-verify')
                .set('x-test-user-id', 'other-user')
                .send({ 
                    user_id: 'test-user-id',
                    attestation_response: attestationResponse
                });

            expect(res.status).toBe(403);
            expect(res.body.error.message).toBe('Access denied');
        });
    });

    describe(`POST ${PasskeyPaths.AUTH_CHALLENGE}`, () => {
        it('issues auth challenge for valid email', async () => {
            mockUserService.findUserByEmail.mockResolvedValue({ id: 'user-id' });
            mockPasskeyAuthService.generateAuthenticationChallenge.mockResolvedValue({ challenge: 'ghi' });

            const res = await request(app)
                .post('/api/auth/passkey/auth-challenge')
                .send({ email: 'user@example.com' });

            expect(res.status).toBe(200);
            expect(res.body.options.challenge).toBe('ghi');
        });

        it('returns 404 if user not found', async () => {
            mockUserService.findUserByEmail.mockResolvedValue(null);

            const res = await request(app)
                .post('/api/auth/passkey/auth-challenge')
                .send({ email: 'missing@example.com' });

            expect(res.status).toBe(404);
            expect(res.body.error.message).toBe('No account found for that email');
        });

        it('returns 400 if no passkeys registered', async () => {
            mockUserService.findUserByEmail.mockResolvedValue({ id: 'user-id' });
            mockPasskeyAuthService.generateAuthenticationChallenge.mockResolvedValue(null);

            const res = await request(app)
                .post('/api/auth/passkey/auth-challenge')
                .send({ email: 'nopasskey@example.com' });

            expect(res.status).toBe(400);
            expect(res.body.needs_setup).toBe(true);
        });
    });

    describe(`POST ${PasskeyPaths.AUTH_VERIFY}`, () => {
        it('successfully verifies and logs in', async () => {
            const mockUser = { id: 'user-id', email: 'user@example.com' };
            const mockSession = { id: 'sess-id' };
            mockUserService.findUserByEmail.mockResolvedValue(mockUser);
            mockPasskeyAuthService.verifyAuthentication.mockResolvedValue({ verified: true });
            mockPostLoginService.createSessionAndSetCookie.mockResolvedValue(mockSession);

            const res = await request(app)
                .post('/api/auth/passkey/auth-verify')
                .send({ 
                    email: 'user@example.com', 
                    assertion_response: {
                        id: 'cred-id',
                        rawId: 'cred-id',
                        type: 'public-key',
                        clientExtensionResults: {},
                        response: {
                            clientDataJSON: 'abc',
                            authenticatorData: 'def',
                            signature: 'ghi'
                        }
                    }
                });

            expect(res.status).toBe(200);
            expect(res.body.session).toEqual(mockSession);
            expect(mockPostLoginService.onSuccessfulLogin).toHaveBeenCalledWith(mockUser, mockSession);
        });

        it('returns 401 for failed verification', async () => {
            mockUserService.findUserByEmail.mockResolvedValue({ id: 'user-id' });
            mockPasskeyAuthService.verifyAuthentication.mockResolvedValue({ verified: false, error: 'Invalid' });

            const res = await request(app)
                .post('/api/auth/passkey/auth-verify')
                .send({ 
                    email: 'user@example.com',
                    assertion_response: {
                        id: 'cred-id',
                        rawId: 'cred-id',
                        type: 'public-key',
                        clientExtensionResults: {},
                        response: {
                            clientDataJSON: 'abc',
                            authenticatorData: 'def',
                            signature: 'ghi'
                        }
                    }
                });

            expect(res.status).toBe(401);
            expect(res.body.error.message).toBe('Invalid');
        });
    });
});
