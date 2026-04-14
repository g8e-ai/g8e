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

/**
 * Setup Flow End-to-End Tests — Complete user journey simulation
 * 
 * Tests the complete setup flow from first run detection through
 * account creation, passkey registration, and platform access.
 * Uses real HTTP requests against a live g8ed instance.
 */

import { describe, it, expect, beforeAll, afterAll, beforeEach, afterEach } from 'vitest';
import request from 'supertest';
import path from 'path';
import { fileURLToPath } from 'url';
import { getTestServices, cleanupTestServices } from '@test/helpers/test-services.js';
import { TestCleanupHelper } from '@test/helpers/test-cleanup.js';
import { UserRole } from '@g8ed/constants/auth.js';
import { LLMProvider, GeminiModel } from '@g8ed/constants/ai.js';
import { Collections } from '@g8ed/constants/collections.js';
import { createAuthMiddleware } from '@g8ed/middleware/authentication.js';
import { createAuthorizationMiddleware } from '@g8ed/middleware/authorization.js';
import { createApiKeyMiddleware } from '@g8ed/middleware/api_key_auth.js';
import { createG8edApp } from '@g8ed/app_factory.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Mock external dependencies
import { vi } from 'vitest';

// Mock WebAuthn for passkey testing
vi.mock('@simplewebauthn/server', () => ({
    generateRegistrationOptions: vi.fn(() => ({
        challenge: 'test-challenge-123',
        user: {
            id: 'test-user-id',
            name: 'test@example.com',
            displayName: 'Test User'
        },
        rp: {
            name: 'g8e',
            id: 'localhost'
        },
        pubKeyCredParams: [{ alg: -7, type: 'public-key' }],
        excludeCredentials: [],
        authenticatorSelection: {
            residentKey: 'preferred',
            userVerification: 'preferred'
        },
        timeout: 60000,
        attestation: 'direct'
    })),
    verifyRegistrationResponse: vi.fn(() => ({
        verified: true,
        registrationInfo: {
            credential: {
                id: 'test-credential-id',
                publicKey: Buffer.from('test-public-key'),
                counter: 0
            }
        }
    }))
}));

describe('Setup Flow End-to-End Tests', () => {
    let app;
    let services;
    let cleanup;
    let userService;
    let settingsService;
    let webSessionService;
    let testUsersCollection;
    let testSettingsCollection;
    let rateLimitCount = 0;

    beforeAll(async () => {
        // Get real test services
        services = await getTestServices();
        const {
            userService: us,
            webSessionService: wss,
            settingsService: ss,
            setupService,
            passkeyAuthService,
            kvClient,
            cacheAside
        } = services;

        userService = us;
        webSessionService = wss;
        settingsService = ss;
        
        // Store test collection names for cleanup
        testUsersCollection = userService.collectionName;
        testSettingsCollection = settingsService.collectionName;

        // Mock services and middleware dependencies
        const mockRateLimiters = {
            settingsRateLimiter: (req, res, next) => next(),
            apiRateLimiter: (req, res, next) => next(),
            globalPublicRateLimiter: (req, res, next) => next(),
            sseRateLimiter: (req, res, next) => next(),
            auditRateLimiter: (req, res, next) => next(),
            consoleRateLimiter: (req, res, next) => next(),
            passkeyRateLimiter: (req, res, next) => next(),
            chatRateLimiter: (req, res, next) => next(),
            authRateLimiter: (req, res, next) => {
                rateLimitCount++;
                // Check if it's a registration request and increment counter
                const isRegister = (req.originalUrl || req.url || '').endsWith('/register');
                if (rateLimitCount > 5 && isRegister && req.method === 'POST') {
                    return res.status(429).json({ success: false, error: 'Too many requests' });
                }
                next();
            },
            // Device link rate limiters
            deviceLinkRateLimiter: (req, res, next) => next(),
            deviceLinkGenerateLimiter: (req, res, next) => next(),
            deviceLinkCreateRateLimiter: (req, res, next) => next(),
            deviceLinkListRateLimiter: (req, res, next) => next(),
            deviceLinkRevokeRateLimiter: (req, res, next) => next(),
            // Operator rate limiters
            operatorAuthIpBackstopLimiter: (req, res, next) => next(),
            operatorAuthRateLimiter: (req, res, next) => next(),
            operatorRefreshRateLimiter: (req, res, next) => next()
        };

        const authMiddleware = createAuthMiddleware({
            userService,
            webSessionService,
            apiKeyService: services.apiKeyService,
            globalPublicRateLimiter: mockRateLimiters.globalPublicRateLimiter,
            settingsService,
            bindingService: services.bindingService
        });

        const authorizationMiddleware = createAuthorizationMiddleware({
            operatorService: services.operatorService,
            settingsService
        });

        const apiKeyMiddleware = createApiKeyMiddleware({
            apiKeyService: services.apiKeyService,
            userService
        });

        // Create error handler middleware for tests
        const errorHandlerMiddleware = (err, req, res, next) => {
            if (process.env.VITEST) {
                console.log('[TEST-ERROR-HANDLER] Caught error:', err.name, err.message);
            }
            
            // Standard g8e Error Response
            const status = typeof err.getHttpStatus === 'function' ? err.getHttpStatus() : 500;
            const body = {
                error: {
                    message: err.message,
                    code: err.code || 'UNEXPECTED_ERROR',
                    category: err.category || 'internal',
                    severity: err.severity || 'medium'
                }
            };
            
            return res.status(status).json(body);
        };

        // Create the Express app using the factory (same as production)
        app = createG8edApp({
            services,
            rateLimiters: mockRateLimiters,
            authMiddleware,
            authorizationMiddleware,
            apiKeyMiddleware,
            requestTimestampMiddleware: { 
            requireRequestTimestamp: () => (req, res, next) => next() 
        },
            errorHandlerMiddleware,
            settings: { cors: { allowed_origins: ['http://localhost:3000'] } },
            versionInfo: { version: 'test-version' },
            isTest: true,
            viewsPath: path.join(__dirname, '../../../views'),
            publicPath: path.join(__dirname, '../../../public')
        });

        // Add a mock chat page renderer for tests
        app.get('/chat', authMiddleware.requirePageAuth({ onFail: 'redirect', redirectTo: '/' }), (req, res) => {
            res.send('<html><body>chat</body></html>');
        });

        // Setup cleanup tracking
        cleanup = new TestCleanupHelper(kvClient, cacheAside, {
            usersCollection: userService.collectionName,
            settingsCollection: settingsService.collectionName
        });
    });

    afterAll(async () => {
        // Restore original collection names
        if (userService) {
            userService.collectionName = Collections.USERS;
        }
        if (settingsService) {
            settingsService.collectionName = Collections.SETTINGS;
        }
        
        // Clean up all test data
        if (cleanup) {
            await cleanup.cleanup();
        }
        
        // Clean up test services
        await cleanupTestServices();
    });

    beforeEach(async () => {
        // Ensure clean state before each test
        await cleanup.cleanup();
        
        // Additional cleanup: ensure no users exist that could affect first run detection
        const existingUsers = await userService.listUsers();
        for (const user of existingUsers) {
            await userService.deleteUser(user.id);
        }
        
        // Ensure platform settings are in first run state
        await settingsService.savePlatformSettings({ setup_complete: false });
        
        rateLimitCount = 0;
    });

    afterEach(async () => {
        // Clean up any test data created during test
        await cleanup.cleanup();
    });

    describe('First Run Detection and Setup Access', () => {
        it('should redirect to setup when accessing root on first run', async () => {
            // Ensure we're in first run state
            const isFirstRun = await services.setupService.isFirstRun();
            expect(isFirstRun).toBe(true);

            // Access root - should redirect to setup
            const response = await request(app)
                .get('/')
                .timeout(5000);

            expect(response.status).toBe(302);
            expect(response.headers.location).toBe('/setup');
        });

        it('should render setup page when accessing /setup on first run', async () => {
            const response = await request(app)
                .get('/setup')
                .timeout(5000);

            expect(response.status).toBe(200);
            expect(response.headers['content-type']).toMatch(/text\/html/);
            expect(response.text).toContain('g8e - Platform Setup');
            expect(response.text).toContain('wizard-steps');
        });

        it('should redirect to login when setup is complete', async () => {
            // Complete setup by creating a user and marking setup complete
            const user = await userService.createUser({
                email: 'admin@example.com',
                name: 'Admin User',
                roles: [UserRole.SUPERADMIN]
            });
            cleanup.trackUser(user.id);

            await settingsService.savePlatformSettings({
                setup_complete: true
            });
            cleanup.trackKVPattern(`${testSettingsCollection}:*`);
            cleanup.trackDBDoc(testSettingsCollection, 'platform_settings');

            // Access setup - should redirect to login
            const response = await request(app)
                .get('/setup')
                .timeout(5000);

            expect(response.status).toBe(302);
            expect(response.headers.location).toBe('/');
        });
    });

    describe('Setup Wizard Flow', () => {
        it('should load setup page with correct structure', async () => {
            const response = await request(app)
                .get('/setup')
                .timeout(5000);

            expect(response.text).toContain('data-step="1"');
            expect(response.text).toContain('data-step="2"');
            expect(response.text).toContain('data-step="3"');
            expect(response.text).toContain('data-step="4"');

            // Check for key elements
            expect(response.text).toContain('account_email');
            expect(response.text).toContain('finish-btn');

            // Platform hostname step should NOT exist
            expect(response.text).not.toContain('platform_host');
            expect(response.text).not.toContain('data-step="5"');
            expect(response.text).not.toContain('data-step="6"');
        });

        it('should include all AI provider options', async () => {
            const response = await request(app)
                .get('/setup')
                .timeout(5000);

            // Check for AI provider cards
            expect(response.text).toContain('data-provider="gemini"');
            expect(response.text).toContain('data-provider="anthropic"');
            expect(response.text).toContain('data-provider="openai"');
            expect(response.text).toContain('data-provider="ollama"');
        });

        it('should include web search configuration', async () => {
            const response = await request(app)
                .get('/setup')
                .timeout(5000);

            // Check for search provider options
            expect(response.text).toContain('search_provider');
            expect(response.text).toContain('search-config-google');
            expect(response.text).toContain('google_project_id');
            expect(response.text).toContain('vertex_ai_search_app_id');
        });
    });

    describe('Account Creation Integration', () => {
        it('should create admin account via registration endpoint', async () => {
            const userData = {
                email: 'newadmin@example.com',
                name: 'New Admin',
                settings: {
                    llm_primary_provider: LLMProvider.GEMINI,
                    llm_model: GeminiModel.PRO_PREVIEW,
                    llm_assistant_model: GeminiModel.FLASH_PREVIEW,
                    gemini_api_key: 'test-api-key-12345'
                }
            };

            const response = await request(app)
                .post('/api/auth/register')
                .send(userData)
                .timeout(5000);

            expect(response.status).toBe(201);
            expect(response.body.success).toBeUndefined();
            expect(response.body.user_id).toBeDefined();
            expect(response.body.challenge_options).toBeDefined();

            // Track created user for cleanup
            cleanup.trackUser(response.body.user_id);

            // Verify user was created in database
            const user = await userService.getUser(response.body.user_id);
            expect(user).toBeDefined();
            expect(user.email).toBe(userData.email);
            expect(user.name).toBe(userData.name);
            expect(user.roles).toContain(UserRole.SUPERADMIN);

            // Verify platform settings were derived server-side
            const platformSettings = await settingsService.getPlatformSettings();
            expect(platformSettings.passkey_rp_id).toBeDefined();
            expect(platformSettings.passkey_origin).toBeDefined();

            // Verify user settings were saved
            const userSettings = await settingsService.getUserSettings(response.body.user_id);
            expect(userSettings.llm_primary_provider).toBe(userData.settings.llm_primary_provider);
            expect(userSettings.llm_model).toBe(userData.settings.llm_model);
        });

        it('should handle duplicate email registration during first-run idempotently', async () => {
            const userData = {
                email: 'duplicate@example.com',
                name: 'First User',
                settings: { llm_primary_provider: 'gemini', gemini_api_key: 'test-key' }
            };

            const first = await request(app)
                .post('/api/auth/register')
                .send(userData)
                .timeout(5000);

            expect(first.status).toBe(201);
            cleanup.trackUser(first.body.user_id);

            // Second register with same email during first-run uses find-or-create (idempotent)
            const second = await request(app)
                .post('/api/auth/register')
                .send(userData)
                .timeout(5000);

            expect(second.status).toBe(201);
            expect(second.body.user_id).toBe(first.body.user_id);
        });

        it('should reject duplicate email registration after setup is complete', async () => {
            const userData = {
                email: 'postsetup@example.com',
                name: 'Post-Setup User',
                settings: { llm_primary_provider: 'gemini', gemini_api_key: 'test-key' }
            };

            // Complete setup first
            const reg = await request(app)
                .post('/api/auth/register')
                .send(userData)
                .timeout(5000);
            cleanup.trackUser(reg.body.user_id);
            await services.setupService.completeSetup();

            // Now try to register same email — standard path rejects duplicates
            const dup = await request(app)
                .post('/api/auth/register')
                .send(userData)
                .timeout(5000);

            expect(dup.status).toBe(409);
            expect(dup.body.error).toBeDefined();
            expect(dup.body.error.code).toBe('USER_ALREADY_EXISTS');
        });

        it('should validate required fields in registration', async () => {
            const invalidData = {
                email: '',
                name: 'Test User',
                settings: {}
            };

            const response = await request(app)
                .post('/api/auth/register')
                .send(invalidData)
                .timeout(5000);

            expect(response.status).toBeGreaterThanOrEqual(400);
            expect(response.body.error).toBeDefined();
            expect(response.body.error.message).toBeDefined();
            expect(response.body.success).toBeUndefined();
        });
    });

    describe('Passkey Registration Flow', () => {
        it('should generate passkey challenge after user registration', async () => {
            // First register a user
            const userData = {
                email: 'passkey@example.com',
                name: 'Passkey User',
                settings: {
                    llm_primary_provider: 'gemini',
                    gemini_api_key: 'test-key'
                }
            };

            const registerResponse = await request(app)
                .post('/api/auth/register')
                .send(userData)
                .timeout(5000);

            expect(registerResponse.status).toBe(201);
            expect(registerResponse.body.challenge_options).toBeDefined();

            const userId = registerResponse.body.user_id;
            cleanup.trackUser(userId);

            // Verify challenge structure
            const challenge = registerResponse.body.challenge_options;
            expect(challenge.challenge).toBeDefined();
            expect(challenge.user).toBeDefined();
            expect(challenge.rp).toBeDefined();
            expect(challenge.pubKeyCredParams).toBeDefined();
        });

        it('should complete passkey registration', async () => {
            // Register user first
            const userData = {
                email: 'complete@example.com',
                name: 'Complete User',
                settings: {
                    llm_primary_provider: 'gemini',
                    gemini_api_key: 'test-key'
                }
            };

            const registerResponse = await request(app)
                .post('/api/auth/register')
                .send(userData)
                .timeout(5000);

            const userId = registerResponse.body.user_id;
            cleanup.trackUser(userId);

            // Mock passkey verification response
            const mockCredential = {
                id: 'test-credential-id',
                rawId: 'dGVzdC1jcmVkZW50aWFsLWlk',
                type: 'public-key',
                response: {
                    attestationObject: 'dGVzdC1hdHRlc3RhdGlvbg',
                    clientDataJSON: 'dGVzdC1jbGllbnQtZGF0YQ'
                }
            };

            const verifyResponse = await request(app)
                .post('/api/auth/passkey/register-verify')
                .send({
                    user_id: userId,
                    attestation_response: mockCredential
                })
                .timeout(5000);

            expect(verifyResponse.status).toBe(200);

            // PasskeyVerifyResponse shape: { success, message, session }
            expect(verifyResponse.body).toHaveProperty('message');
            expect(verifyResponse.body).toHaveProperty('session');
            expect(verifyResponse.body.success).toBe(true);
            expect(verifyResponse.body.message).toBe('Setup complete');
            expect(verifyResponse.body.session).toBeDefined();
            expect(verifyResponse.body.session.id).toBeDefined();

            // Verify session was created and is valid
            const session = await webSessionService.validateSession(
                verifyResponse.body.session.id,
                { ip: '127.0.0.1', userAgent: 'test' }
            );
            expect(session).toBeDefined();
            expect(session.user_id).toBe(userId);

            // Verify setup was marked complete after passkey registration
            const isFirstRun = await services.setupService.isFirstRun();
            expect(isFirstRun).toBe(false);
        });
    });

    describe('Post-Setup Access', () => {
        it('should allow access to chat after successful setup', async () => {
            // Complete full setup flow
            const userData = {
                email: 'chataccess@example.com',
                name: 'Chat Access User',
                settings: {
                    llm_primary_provider: 'gemini',
                    gemini_api_key: 'test-key'
                }
            };

            const registerResponse = await request(app)
                .post('/api/auth/register')
                .send(userData)
                .timeout(5000);

            const userId = registerResponse.body.user_id;
            cleanup.trackUser(userId);

            // Complete passkey registration
            const mockCredential = {
                id: 'chat-credential-id',
                rawId: 'dGVzdC1jcmVkZW50aWFsLWlk',
                type: 'public-key',
                response: {
                    attestationObject: 'dGVzdC1hdHRlc3RhdGlvbg',
                    clientDataJSON: 'dGVzdC1jbGllbnQtZGF0YQ'
                }
            };

            const verifyResponse = await request(app)
                .post('/api/auth/passkey/register-verify')
                .send({
                    user_id: userId,
                    attestation_response: mockCredential
                })
                .timeout(5000);

            expect(verifyResponse.status).toBe(200);
            expect(verifyResponse.body.session?.id).toBeDefined();

            // Use the session to access chat
            const sessionId = verifyResponse.body.session.id;
            const chatResponse = await request(app)
                .get('/chat')
                .set('Cookie', `web_session_id=${sessionId}`)
                .timeout(5000);

            expect(chatResponse.status).toBe(200);
            expect(chatResponse.text).toContain('chat'); // Should contain chat interface
        });

        it('should redirect setup to login after completion', async () => {
            // Mark setup as complete
            await settingsService.savePlatformSettings({
                setup_complete: true
            });
            cleanup.trackKVPattern(`${testSettingsCollection}:*`);
            cleanup.trackDBDoc(testSettingsCollection, 'platform_settings');

            // Try to access setup page
            const response = await request(app)
                .get('/setup')
                .timeout(5000);

            expect(response.status).toBe(302);
            expect(response.headers.location).toBe('/');
        });
    });

    describe('Error Handling and Edge Cases', () => {
        it('should handle malformed registration requests', async () => {
            const malformedData = {
                email: 'invalid-email',
            };

            const response = await request(app)
                .post('/api/auth/register')
                .send(malformedData)
                .timeout(5000);

            expect(response.status).toBe(400);
            expect(response.body.error).toBeDefined();
            expect(response.body.error.message).toBe('Invalid email format');
            expect(response.body.success).toBeUndefined();
        });

        it('should handle invalid passkey verification data', async () => {
            const userData = {
                email: 'invalidpasskey@example.com',
                name: 'Invalid Passkey User',
                settings: {
                    llm_primary_provider: 'gemini',
                    gemini_api_key: 'test-key'
                }
            };

            const registerResponse = await request(app)
                .post('/api/auth/register')
                .send(userData)
                .timeout(5000);

            const userId = registerResponse.body.user_id;
            cleanup.trackUser(userId);

            // Send invalid credential data
            const invalidCredential = {
                id: '',
                rawId: '',
                type: 'invalid-type',
                response: {}
            };

            const response = await request(app)
                .post('/api/auth/passkey/register-verify')
                .send({
                    user_id: userId,
                    attestation_response: invalidCredential
                })
                .timeout(5000);

            expect(response.status).toBeGreaterThanOrEqual(400);
            expect(response.body.error).toBeDefined();
        });

        it('should serialize concurrent same-email registrations during first-run (no duplicates)', async () => {
            const userData = {
                email: 'concurrent@example.com',
                name: 'Concurrent User',
                settings: {
                    llm_primary_provider: 'gemini',
                    gemini_api_key: 'test-key'
                }
            };

            const promises = Array(3).fill().map(() =>
                request(app)
                    .post('/api/auth/register')
                    .send(userData)
                    .timeout(5000)
            );

            const responses = await Promise.all(promises);

            // All should succeed — the setup lock serializes them
            for (const response of responses) {
                expect(response.status).toBe(201);
            }

            // All must return the same user_id — the lock prevents duplicate creation
            const userIds = new Set(responses.map(r => r.body.user_id));
            expect(userIds.size).toBe(1);

            cleanup.trackUser(responses[0].body.user_id);
        });
    });

    describe('Security and Validation', () => {
        it('should sanitize input data properly', async () => {
            const userData = {
                email: '  sanitize@example.com  ',
                name: '  <script>alert("xss")</script>  ',
                settings: {
                    llm_primary_provider: 'gemini',
                    gemini_api_key: '  test-key-with-spaces  '
                }
            };

            const response = await request(app)
                .post('/api/auth/register')
                .send(userData)
                .timeout(5000);

            if (response.status === 201) {
                cleanup.trackUser(response.body.user_id);
                
                // Verify data was sanitized
                const user = await userService.getUser(response.body.user_id);
                expect(user.email).toBe('sanitize@example.com'); // Whitespace trimmed
                expect(user.name).not.toContain('<script>'); // XSS sanitized
            }
        });

        it('should enforce rate limiting on registration endpoint', async () => {
            const userData = {
                email: 'ratelimit@example.com',
                name: 'Rate Limit User',
                settings: {
                    llm_primary_provider: 'gemini',
                    gemini_api_key: 'test-key'
                }
            };

            // Send multiple rapid requests
            const promises = Array(10).fill().map((_, i) =>
                request(app)
                    .post('/api/auth/register')
                    .send({
                        ...userData,
                        email: `ratelimit${i}@example.com`
                    })
                    .timeout(10000)
            );

            const responses = await Promise.all(promises);

            // Some requests should be rate limited
            const rateLimitedCount = responses.filter(r => r.status === 429).length;
            expect(rateLimitedCount).toBeGreaterThan(0);

            // Track any successful users
            for (const response of responses) {
                if (response.status === 201 && response.body.user_id) {
                    cleanup.trackUser(response.body.user_id);
                }
            }
        });
    });
});
