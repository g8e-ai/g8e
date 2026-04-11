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
import { createAuthRouter } from '@g8ed/routes/auth/auth_routes.js';
import { AuthPaths } from '@g8ed/constants/api_paths.js';
import { UserRole } from '@g8ed/constants/auth.js';
import { LLMProvider } from '@g8ed/constants/ai.js';
import { SESSION_COOKIE_NAME } from '@g8ed/constants/session.js';
import { ApiKeyError } from '@g8ed/constants/auth.js';
import { 
    AuthenticationError, 
    AuthorizationError, 
    ValidationError, 
    BusinessLogicError,
    ResourceNotFoundError
} from '@g8ed/services/error_service.js';

describe('AuthRoutes Unit Tests', () => {
    let app;
    let mockWebSessionService;
    let mockUserService;
    let mockLoginSecurityService;
    let mockSetupService;
    let mockPasskeyAuthService;
    let mockAuthMiddleware;

    beforeEach(() => {
        mockWebSessionService = {
            endSession: vi.fn().mockResolvedValue(true)
        };
        mockUserService = {
            findUserByEmail: vi.fn(),
            createUser: vi.fn(),
            getUser: vi.fn()
        };
        mockLoginSecurityService = {
            getLockedAccounts: vi.fn(),
            auditAdminAccess: vi.fn(),
            unlockAccount: vi.fn(),
            isAccountLocked: vi.fn(),
            getFailedAttemptStatus: vi.fn()
        };
        mockSetupService = {
            isFirstRun: vi.fn().mockResolvedValue(false),
            createAdminUser: vi.fn(),
            performFirstRunSetup: vi.fn()
        };
        mockPasskeyAuthService = {
            generateRegistrationChallenge: vi.fn()
        };
        mockAuthMiddleware = {
            requireAuth: (req, res, next) => {
                req.userId = 'test-user-id';
                req.session = { user_id: 'test-user-id', user_data: { email: 'test@example.com' } };
                next();
            },
            requireAdmin: (req, res, next) => {
                req.userId = 'admin-user-id';
                req.session = { user_id: 'admin-user-id', user_data: { email: 'admin@example.com' } };
                next();
            }
        };

        const router = createAuthRouter({
            services: {
                webSessionService: mockWebSessionService,
                userService: mockUserService,
                loginSecurityService: mockLoginSecurityService,
                setupService: mockSetupService,
                passkeyAuthService: mockPasskeyAuthService
            },

            authMiddleware: mockAuthMiddleware,

            rateLimiters: {
                authRateLimiter: (req, res, next) => next()
            }
        });

        app = express();
        app.use(express.json());
        // Simple cookie parser shim for tests
        app.use((req, res, next) => {
            req.cookies = req.headers.cookie ? Object.fromEntries(
                req.headers.cookie.split('; ').map(c => c.split('='))
            ) : {};
            next();
        });
        app.use('/api/auth', router);
        
        // Error handler
        app.use((err, req, res, next) => {
            const status = typeof err.getHttpStatus === 'function' ? err.getHttpStatus() : (err.status || 500);
            const response = {
                error: {
                    message: err.message,
                    code: err.code
                }
            };
            res.status(status).json(response);
        });
    });

    describe(`GET ${AuthPaths.WEB_SESSION}`, () => {
        it('returns session data when authenticated', async () => {
            const res = await request(app).get('/api/auth/web-session');
            expect(res.status).toBe(200);
            expect(res.body.authenticated).toBe(true);
            expect(res.body.session.user_id).toBe('test-user-id');
        });
    });

    describe(`POST ${AuthPaths.LOGOUT}`, () => {
        it('ends session and clears cookie', async () => {
            const res = await request(app)
                .post('/api/auth/logout')
                .set('Cookie', [`${SESSION_COOKIE_NAME}=test-session-id`]);

            expect(res.status).toBe(200);
            expect(mockWebSessionService.endSession).toHaveBeenCalledWith('test-session-id');
            expect(res.headers['set-cookie'][0]).toContain(`${SESSION_COOKIE_NAME}=;`);
        });

        it('handles internal error during logout', async () => {
            mockWebSessionService.endSession.mockRejectedValue(new Error('DB Fail'));
            const res = await request(app)
                .post('/api/auth/logout')
                .set('Cookie', [`${SESSION_COOKIE_NAME}=test-session-id`]);

            expect(res.status).toBe(500);
            expect(res.body.error.message).toBe(ApiKeyError.INTERNAL_ERROR);
        });
    });

    describe(`POST ${AuthPaths.REGISTER}`, () => {
        describe('Response Model Shape (UserRegisterResponse)', () => {
            it('returns { message, user_id, challenge_options } with no success field', async () => {
                mockUserService.findUserByEmail.mockResolvedValue(null);
                mockUserService.createUser.mockResolvedValue({ id: 'new-user-id', email: 'new@example.com' });
                mockPasskeyAuthService.generateRegistrationChallenge.mockResolvedValue({ challenge: 'c' });

                const res = await request(app)
                    .post('/api/auth/register')
                    .send({ email: 'new@example.com', name: 'New User' });

                expect(res.status).toBe(201);
                expect(res.body).toHaveProperty('user_id');
                expect(res.body).toHaveProperty('message');
                expect(res.body).toHaveProperty('challenge_options');
                expect(res.body.success).toBeUndefined();
            });

            it('first-run response also has no success field', async () => {
                mockSetupService.isFirstRun.mockResolvedValue(true);
                mockSetupService.performFirstRunSetup.mockResolvedValue({ id: 'a1', email: 'a@b.com' });
                mockPasskeyAuthService.generateRegistrationChallenge.mockResolvedValue({ challenge: 'c' });

                const res = await request(app)
                    .post('/api/auth/register')
                    .send({ email: 'a@b.com', name: 'Admin' });

                expect(res.status).toBe(201);
                expect(res.body.success).toBeUndefined();
                expect(res.body.user_id).toBe('a1');
            });
        });

        describe('Standard Registration', () => {
            it('creates user with passkey challenge', async () => {
                mockUserService.findUserByEmail.mockResolvedValue(null);
                mockUserService.createUser.mockResolvedValue({ id: 'new-user-id', email: 'new@example.com' });
                mockPasskeyAuthService.generateRegistrationChallenge.mockResolvedValue({ challenge: 'atomic-challenge' });

                const res = await request(app)
                    .post('/api/auth/register')
                    .send({ email: 'new@example.com', name: 'New User' });

                expect(res.status).toBe(201);
                expect(res.body.user_id).toBe('new-user-id');
                expect(res.body.challenge_options.challenge).toBe('atomic-challenge');
                expect(mockPasskeyAuthService.generateRegistrationChallenge).toHaveBeenCalled();
                expect(mockSetupService.performFirstRunSetup).not.toHaveBeenCalled();
            });

            it('rejects duplicate email with 409', async () => {
                mockUserService.findUserByEmail.mockResolvedValue({ id: 'existing' });

                const res = await request(app)
                    .post('/api/auth/register')
                    .send({ email: 'existing@example.com' });

                expect(res.status).toBe(409);
                expect(res.body.error.code).toBe('USER_ALREADY_EXISTS');
            });
        });

        describe('First-Run Administrative Setup', () => {
            it('delegates to performFirstRunSetup with email, name, userSettings, and req', async () => {
                mockSetupService.isFirstRun.mockResolvedValue(true);
                mockSetupService.performFirstRunSetup.mockResolvedValue({ id: 'admin-id', email: 'admin@g8e.local' });
                mockPasskeyAuthService.generateRegistrationChallenge.mockResolvedValue({ challenge: 'setup-challenge' });

                const userSettings = { llm_provider: LLMProvider.GEMINI };

                const res = await request(app)
                    .post('/api/auth/register')
                    .send({ email: 'admin@g8e.local', name: 'Admin', settings: userSettings });

                expect(res.status).toBe(201);
                expect(res.body.message).toBe('Administrative setup initialized');
                expect(mockSetupService.performFirstRunSetup).toHaveBeenCalledWith({
                    email: 'admin@g8e.local',
                    name: 'Admin',
                    userSettings,
                    req: expect.any(Object)
                });
            });

            it('does not call findUserByEmail or createUser (setup service handles it)', async () => {
                mockSetupService.isFirstRun.mockResolvedValue(true);
                mockSetupService.performFirstRunSetup.mockResolvedValue({ id: 'a1', email: 'a@b.com' });
                mockPasskeyAuthService.generateRegistrationChallenge.mockResolvedValue({ challenge: 'c' });

                await request(app)
                    .post('/api/auth/register')
                    .send({ email: 'a@b.com', name: 'A' });

                expect(mockUserService.findUserByEmail).not.toHaveBeenCalled();
                expect(mockUserService.createUser).not.toHaveBeenCalled();
            });
        });

        describe('Input Validation', () => {
            it('rejects missing email with 400', async () => {
                const res = await request(app)
                    .post('/api/auth/register')
                    .send({ name: 'New User' });

                expect(res.status).toBe(400);
                expect(res.body.error.message).toBe('email is required');
            });

            it('rejects email without @ with 400', async () => {
                const res = await request(app)
                    .post('/api/auth/register')
                    .send({ email: 'not-an-email' });

                expect(res.status).toBe(400);
                expect(res.body.error.message).toBe('Invalid email format');
            });
        });

        describe('Input Sanitization', () => {
            it('trims and lowercases email', async () => {
                mockUserService.findUserByEmail.mockResolvedValue(null);
                mockUserService.createUser.mockResolvedValue({ id: 'u1', email: 'test@example.com' });
                mockPasskeyAuthService.generateRegistrationChallenge.mockResolvedValue({ challenge: 'c' });

                await request(app)
                    .post('/api/auth/register')
                    .send({ email: '  TEST@Example.COM  ', name: 'Test' });

                expect(mockUserService.findUserByEmail).toHaveBeenCalledWith('test@example.com');
            });

            it('strips script tags from name', async () => {
                mockUserService.findUserByEmail.mockResolvedValue(null);
                mockUserService.createUser.mockResolvedValue({ id: 'u1', email: 'a@b.com' });
                mockPasskeyAuthService.generateRegistrationChallenge.mockResolvedValue({ challenge: 'c' });

                await request(app)
                    .post('/api/auth/register')
                    .send({ email: 'a@b.com', name: '<script>alert(1)</script>Admin' });

                const createCall = mockUserService.createUser.mock.calls[0][0];
                expect(createCall.name).not.toContain('<script>');
                expect(createCall.name).toContain('Admin');
            });

            it('defaults name to email prefix when name is absent', async () => {
                mockUserService.findUserByEmail.mockResolvedValue(null);
                mockUserService.createUser.mockResolvedValue({ id: 'u1', email: 'alice@corp.com' });
                mockPasskeyAuthService.generateRegistrationChallenge.mockResolvedValue({ challenge: 'c' });

                await request(app)
                    .post('/api/auth/register')
                    .send({ email: 'alice@corp.com' });

                const createCall = mockUserService.createUser.mock.calls[0][0];
                expect(createCall.name).toBe('alice');
            });
        });
    });

    describe(`GET ${AuthPaths.ADMIN_LOCKED_ACCOUNTS}`, () => {
        it('returns locked accounts for admin', async () => {
            const locked = [{ identifier: 'user@example.com', locked_at: Date.now() }];
            mockLoginSecurityService.getLockedAccounts.mockResolvedValue(locked);

            const res = await request(app).get('/api/auth/admin/locked-accounts');

            expect(res.status).toBe(200);
            expect(res.body.locked_accounts).toEqual(locked);
            expect(mockLoginSecurityService.auditAdminAccess).toHaveBeenCalled();
        });
    });

    describe(`POST ${AuthPaths.ADMIN_UNLOCK_ACCOUNT}`, () => {
        it('successfully unlocks an account', async () => {
            mockLoginSecurityService.unlockAccount.mockResolvedValue({ success: true });

            const res = await request(app)
                .post('/api/auth/admin/unlock-account')
                .send({ identifier: 'user@example.com' });

            expect(res.status).toBe(200);
            expect(mockLoginSecurityService.auditAdminAccess).toHaveBeenCalled();
        });

        it('throws ResourceNotFoundError if account not found', async () => {
            mockLoginSecurityService.unlockAccount.mockResolvedValue({ success: false, error: 'Not found' });

            const res = await request(app)
                .post('/api/auth/admin/unlock-account')
                .send({ identifier: 'missing@example.com' });

            expect(res.status).toBe(404);
            expect(res.body.error.message).toBe('Not found');
        });
    });

    describe(`GET ${AuthPaths.ADMIN_ACCOUNT_STATUS}`, () => {
        it('returns account status for admin', async () => {
            mockUserService.getUser.mockResolvedValue({ email: 'user@example.com' });
            mockLoginSecurityService.isAccountLocked.mockResolvedValue({ locked: true, locked_at: 123, failed_attempts: 5 });
            mockLoginSecurityService.getFailedAttemptStatus.mockResolvedValue({ attempts: 5, requires_captcha: true });

            const res = await request(app).get('/api/auth/admin/account-status/test-user');

            expect(res.status).toBe(200);
            expect(res.body.locked).toBe(true);
            expect(res.body.failed_attempts).toBe(5);
            expect(res.body.requires_captcha).toBe(true);
        });

        it('throws ResourceNotFoundError if user missing', async () => {
            mockUserService.getUser.mockResolvedValue(null);

            const res = await request(app).get('/api/auth/admin/account-status/missing');

            expect(res.status).toBe(404);
            expect(res.body.error.message).toBe('User not found');
        });
    });
});
