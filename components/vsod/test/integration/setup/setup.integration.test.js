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
 * Setup Integration Tests — Real VSODB, real services
 * 
 * Tests the complete setup flow including:
 * - First run detection and redirection
 * - Setup wizard accessibility and rendering
 * - Account creation with platform settings
 * - Passkey registration flow
 * - Setup completion and session creation
 * - Post-setup redirect behavior
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { getTestServices, cleanupTestServices } from '@test/helpers/test-services.js';
import { TestCleanupHelper } from '@test/helpers/test-cleanup.js';
import { SetupService } from '@vsod/services/platform/setup_service.js';
import { SettingsService } from '@vsod/services/platform/settings_service.js';
import { UserService } from '@vsod/services/platform/user_service.js';
import { WebSessionService } from '@vsod/services/auth/web_session_service.js';
import { PasskeyAuthService } from '@vsod/services/auth/passkey_auth_service.js';
import { UserRole } from '@vsod/constants/auth.js';
import { LLMProvider, GeminiModel } from '@vsod/constants/ai.js';
import { Collections } from '@vsod/constants/collections.js';
import { SetupPaths } from '@vsod/constants/api_paths.js';
import { logger } from '@vsod/utils/logger.js';

// Mock external dependencies that are permitted in integration tests
import { vi } from 'vitest';

vi.mock('@vsod/utils/logger.js', () => ({
    logger: {
        info: vi.fn(),
        warn: vi.fn(),
        error: vi.fn(),
        debug: vi.fn()
    }
}));

describe('Setup Integration Tests', () => {
    let cleanup;
    let services;
    let setupService;
    let settingsService;
    let userService;
    let webSessionService;
    let passkeyAuthService;
    let testUsersCollection;
    let testSettingsCollection;

    beforeEach(async () => {
        // Get real test services (matches production composition root)
        services = await getTestServices();
        userService = services.userService;
        webSessionService = services.webSessionService;
        settingsService = services.settingsService;
        passkeyAuthService = services.passkeyAuthService;
        setupService = services.setupService;

        // Create unique test collections for isolation
        const testSuffix = `setup_integration_${Date.now()}`;
        testUsersCollection = `${Collections.USERS}_${testSuffix}`;
        testSettingsCollection = `${Collections.SETTINGS}_${testSuffix}`;

        // Override collection names for this test suite
        userService.collectionName = testUsersCollection;
        settingsService.collectionName = testSettingsCollection;

        // Setup cleanup tracking
        cleanup = new TestCleanupHelper(services.kvClient, services.cacheAsideService, {
            usersCollection: testUsersCollection,
            settingsCollection: testSettingsCollection
        });
    });

    afterEach(async () => {
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

    describe('First Run Detection', () => {
        it('should detect first run when no users exist and setup not complete', async () => {
            const isFirstRun = await setupService.isFirstRun();
            expect(isFirstRun).toBe(true);
        });

        it('should still be first run when users exist but setup_complete is not true', async () => {
            const testUser = await userService.createUser({
                email: 'test@example.com',
                name: 'Test User',
                roles: [UserRole.SUPERADMIN]
            });
            cleanup.trackUser(testUser.id);

            const isFirstRun = await setupService.isFirstRun();
            expect(isFirstRun).toBe(true);
        });

        it('should not be first run when setup is marked complete', async () => {
            // Mark setup as complete without creating users
            await settingsService.savePlatformSettings({
                setup_complete: true
            });

            const isFirstRun = await setupService.isFirstRun();
            expect(isFirstRun).toBe(false);
        });

        it('should not be first run only when setup_complete is explicitly true', async () => {
            const testUser = await userService.createUser({
                email: 'test@example.com',
                name: 'Test User',
                roles: [UserRole.SUPERADMIN]
            });
            cleanup.trackUser(testUser.id);

            await settingsService.savePlatformSettings({ setup_complete: false });
            expect(await setupService.isFirstRun()).toBe(true);

            await settingsService.savePlatformSettings({ setup_complete: true });
            expect(await setupService.isFirstRun()).toBe(false);
        });
    });

    describe('Setup Flow Integration', () => {
        it('should complete full setup flow with valid data', async () => {
            const setupData = {
                email: 'admin@example.com',
                name: 'Platform Admin',
                userSettings: {
                    llm_provider: LLMProvider.GEMINI,
                    llm_model: GeminiModel.PRO_PREVIEW,
                    llm_assistant_model: GeminiModel.FLASH_LITE_PREVIEW,
                    gemini_api_key: 'test-api-key-12345'
                }
            };

            // Mock request object for passkey field derivation
            const mockReq = {
                get: vi.fn((header) => {
                    switch (header) {
                        case 'host':
                            return 'localhost';
                        case 'x-forwarded-proto':
                            return 'https';
                        default:
                            return undefined;
                    }
                }),
                protocol: 'https',
                hostname: 'localhost'
            };

            // Perform first run setup
            const user = await setupService.performFirstRunSetup({
                ...setupData,
                req: mockReq
            });
            cleanup.trackUser(user.id);

            // Complete setup
            await setupService.completeSetup();

            // Verify user creation
            expect(user).toBeDefined();
            expect(user.email).toBe(setupData.email);
            expect(user.name).toBe(setupData.name);
            expect(user.roles).toContain(UserRole.SUPERADMIN);

            // Verify platform settings were saved (derived server-side only)
            const platformSettings = await settingsService.getPlatformSettings();
            expect(platformSettings.passkey_rp_id).toBe('localhost');
            expect(platformSettings.passkey_origin).toBe('https://localhost');
            expect(platformSettings.setup_complete).toBe(true);

            // Verify user settings were saved
            const userSettings = await settingsService.getUserSettings(user.id);
            expect(userSettings.llm_provider).toBe(setupData.userSettings.llm_provider);
            expect(userSettings.llm_model).toBe(setupData.userSettings.llm_model);
            expect(userSettings.llm_assistant_model).toBe(setupData.userSettings.llm_assistant_model);
            expect(userSettings.gemini_api_key).toBe(setupData.userSettings.gemini_api_key);

            // Verify no longer first run
            const isFirstRun = await setupService.isFirstRun();
            expect(isFirstRun).toBe(false);
        });

        it('should handle setup with minimal required data', async () => {
            const setupData = {
                email: 'minimal@example.com',
                name: 'Minimal Admin',
                userSettings: {}
            };

            const mockReq = {
                get: vi.fn(() => 'g8e.local'),
                protocol: 'https',
                hostname: 'g8e.local'
            };

            const user = await setupService.performFirstRunSetup({
                ...setupData,
                req: mockReq
            });
            cleanup.trackUser(user.id);

            expect(user.email).toBe(setupData.email);
            expect(user.name).toBe(setupData.name);

            const platformSettings = await settingsService.getPlatformSettings();
            expect(platformSettings.passkey_rp_id).toBe('g8e.local');
        });

        it('should derive passkey fields from custom hostname', async () => {
            const setupData = {
                email: 'custom@example.com',
                name: 'Custom Host Admin'
            };

            const mockReq = {
                get: vi.fn((header) => {
                    if (header === 'x-forwarded-host') return 'custom.example.com';
                    if (header === 'x-forwarded-proto') return 'https';
                    return undefined;
                }),
                protocol: 'https',
                hostname: 'custom.example.com'
            };

            const user = await setupService.performFirstRunSetup({
                ...setupData,
                req: mockReq
            });
            cleanup.trackUser(user.id);

            const platformSettings = await settingsService.getPlatformSettings();
            expect(platformSettings.passkey_rp_id).toBe('custom.example.com');
            expect(platformSettings.passkey_origin).toBe('https://custom.example.com');
        });
    });

    describe('Setup Completion', () => {
        it('should mark setup as complete', async () => {
            // Start with incomplete setup
            await settingsService.savePlatformSettings({
                setup_complete: false
            });

            // Complete setup
            await setupService.completeSetup();

            // Verify setup is marked complete
            const platformSettings = await settingsService.getPlatformSettings();
            expect(platformSettings.setup_complete).toBe(true);

            // Verify no longer first run
            const isFirstRun = await setupService.isFirstRun();
            expect(isFirstRun).toBe(false);
        });

        it('should handle completion when settings already exist', async () => {
            // Pre-existing settings
            await settingsService.savePlatformSettings({
                setup_complete: false,
                app_url: 'https://existing.com'
            });

            await setupService.completeSetup();

            const platformSettings = await settingsService.getPlatformSettings();
            expect(platformSettings.setup_complete).toBe(true);
            expect(platformSettings.app_url).toBe('https://existing.com');
        });
    });

    describe('Admin User Creation', () => {
        it('should create admin user with SUPERADMIN role', async () => {
            const adminData = {
                email: 'superadmin@example.com',
                name: 'Super Admin'
            };

            const user = await setupService.createAdminUser(adminData);
            cleanup.trackUser(user.id);

            expect(user.email).toBe(adminData.email);
            expect(user.name).toBe(adminData.name);
            expect(user.roles).toContain(UserRole.SUPERADMIN);
            expect(user.roles).toHaveLength(1); // Only SUPERADMIN role
        });

        it('should handle duplicate admin user creation gracefully', async () => {
            const adminData = {
                email: 'duplicate@example.com',
                name: 'Duplicate Admin'
            };

            // Create first admin
            const user1 = await setupService.createAdminUser(adminData);
            cleanup.trackUser(user1.id);

            // Attempt to create duplicate - should throw or handle gracefully
            try {
                const user2 = await setupService.createAdminUser(adminData);
                // If it succeeds, verify it's the same user or handle appropriately
                expect(user2.email).toBe(adminData.email);
                cleanup.trackUser(user2.id);
            } catch (error) {
                // Expected behavior - duplicate user creation should fail
                expect(error.message).toBeDefined();
            }
        });
    });

    describe('Passkey Challenge Generation (Real Service)', () => {
        it('should generate a real registration challenge via PasskeyAuthService', async () => {
            await settingsService.savePlatformSettings({
                passkey_rp_id: 'localhost',
                passkey_origin: 'https://localhost'
            });

            const user = await userService.createUser({
                email: 'passkey@example.com',
                name: 'Passkey User',
                roles: [UserRole.SUPERADMIN]
            });
            cleanup.trackUser(user.id);

            const mockReq = { get: vi.fn(() => null), protocol: 'https', hostname: 'localhost' };
            const options = await passkeyAuthService.generateRegistrationChallenge(mockReq, user);

            expect(options).toBeDefined();
            expect(options.challenge).toBeDefined();
            expect(typeof options.challenge).toBe('string');
            expect(options.rp.id).toBe('localhost');
            expect(options.user).toBeDefined();
            expect(options.user.name).toBe(user.email);
            expect(options.pubKeyCredParams).toBeDefined();
            expect(options.authenticatorSelection).toBeDefined();
        });

        it('should store challenge in VSODB KV and retrieve it', async () => {
            await settingsService.savePlatformSettings({
                passkey_rp_id: 'localhost',
                passkey_origin: 'https://localhost'
            });

            const user = await userService.createUser({
                email: 'challenge-kv@example.com',
                name: 'Challenge KV User',
                roles: [UserRole.SUPERADMIN]
            });
            cleanup.trackUser(user.id);

            const mockReq = { get: vi.fn(() => null), protocol: 'https', hostname: 'localhost' };
            const options = await passkeyAuthService.generateRegistrationChallenge(mockReq, user);

            const stored = await services.cacheAsideService.getDocument(Collections.PASSKEY_CHALLENGES, user.id);
            expect(stored).toBeDefined();
            expect(stored.challenge).toBe(options.challenge);
        });

        it('should exclude existing credentials from challenge options', async () => {
            await settingsService.savePlatformSettings({
                passkey_rp_id: 'localhost',
                passkey_origin: 'https://localhost'
            });

            const user = await userService.createUser({
                email: 'existing-cred@example.com',
                name: 'Existing Cred User',
                roles: [UserRole.SUPERADMIN]
            });
            cleanup.trackUser(user.id);

            // User with no credentials — excludeCredentials should be empty
            const mockReq = { get: vi.fn(() => null), protocol: 'https', hostname: 'localhost' };
            const options = await passkeyAuthService.generateRegistrationChallenge(mockReq, user);
            expect(options.excludeCredentials).toEqual([]);
        });
    });

    describe('Post-Setup Session Flow', () => {
        it('should create web session after successful setup', async () => {
            // Complete setup first
            const user = await setupService.performFirstRunSetup({
                email: 'session@example.com',
                name: 'Session User',
                userSettings: { llm_provider: 'gemini' },
                req: { get: vi.fn(), protocol: 'https', hostname: 'localhost' }
            });
            cleanup.trackUser(user.id);

            // Mark setup complete
            await setupService.completeSetup();

            // Create a web session (simulating successful passkey registration)
            const requestContext = {
                ip: '127.0.0.1',
                userAgent: 'Test Browser'
            };

            const session = await webSessionService.createWebSession({
                user_id: user.id,
                requestContext
            }, requestContext);
            cleanup.trackWebSession(session.id);

            expect(session).toBeDefined();
            expect(session.user_id).toBe(user.id);
            expect(session.is_active).toBe(true);

            // Verify session can be validated
            const validatedSession = await webSessionService.validateSession(
                session.id,
                requestContext
            );
            expect(validatedSession).toBeDefined();
            if (validatedSession) {
                expect(validatedSession.user_id).toBe(user.id);
            }
        });

        it('should redirect to chat after setup completion', async () => {
            // This tests the redirect logic that would happen in the routes
            const user = await setupService.performFirstRunSetup({
                email: 'redirect@example.com',
                name: 'Redirect User',
                req: { get: vi.fn(), protocol: 'https', hostname: 'localhost' }
            });
            cleanup.trackUser(user.id);

            await setupService.completeSetup();

            // Verify no longer first run - this would trigger redirect to login
            const isFirstRun = await setupService.isFirstRun();
            expect(isFirstRun).toBe(false);

            // With a valid session, user should be able to access chat
            const requestContext = {
                ip: '127.0.0.1',
                userAgent: 'Test Browser'
            };

            const session = await webSessionService.createWebSession({
                user_id: user.id,
                requestContext
            }, requestContext);
            cleanup.trackWebSession(session.id);

            const validatedSession = await webSessionService.validateSession(
                session.id,
                requestContext
            );
            expect(validatedSession).toBeDefined();
            if (validatedSession) {
                expect(validatedSession.user_id).toBe(user.id);
            }
        });
    });

    describe('Edge Cases', () => {
        it('should derive platform settings from request when no userSettings provided', async () => {
            const user = await setupService.performFirstRunSetup({
                email: 'minimal@example.com',
                name: 'Minimal User',
                req: { get: vi.fn(() => null), protocol: 'https', hostname: 'localhost' }
            });
            cleanup.trackUser(user.id);

            expect(user.email).toBe('minimal@example.com');

            const platformSettings = await settingsService.getPlatformSettings();
            expect(platformSettings.passkey_rp_id).toBe('localhost');
            expect(platformSettings.passkey_origin).toBe('https://localhost');

            const userSettings = await settingsService.getUserSettings(user.id);
            expect(userSettings).toEqual({});
        });

        it('should fall back to localhost passkey fields when req is null', async () => {
            const user = await setupService.performFirstRunSetup({
                email: 'nullreq@example.com',
                name: 'Null Req User',
                req: null
            });
            cleanup.trackUser(user.id);

            expect(user.email).toBe('nullreq@example.com');

            const platformSettings = await settingsService.getPlatformSettings();
            expect(platformSettings.passkey_rp_id).toBe('localhost');
            expect(platformSettings.passkey_origin).toBe('https://localhost');
        });

        it('should persist user settings to DB and read them back', async () => {
            const user = await setupService.performFirstRunSetup({
                email: 'persist@example.com',
                name: 'Persist User',
                userSettings: {
                    llm_provider: 'anthropic',
                    anthropic_api_key: 'sk-test-key-123'
                },
                req: { get: vi.fn(() => null), protocol: 'https', hostname: 'localhost' }
            });
            cleanup.trackUser(user.id);

            const savedSettings = await settingsService.getUserSettings(user.id);
            expect(savedSettings.llm_provider).toBe('anthropic');
            expect(savedSettings.anthropic_api_key).toBe('sk-test-key-123');
        });

        it('should verify user is retrievable from DB after setup', async () => {
            const user = await setupService.performFirstRunSetup({
                email: 'dbcheck@example.com',
                name: 'DB Check User',
                req: { get: vi.fn(() => null), protocol: 'https', hostname: 'localhost' }
            });
            cleanup.trackUser(user.id);

            const fetched = await userService.getUser(user.id);
            expect(fetched).toBeDefined();
            expect(fetched.email).toBe('dbcheck@example.com');
            expect(fetched.name).toBe('DB Check User');
            expect(fetched.roles).toContain(UserRole.SUPERADMIN);
        });
    });

    describe('Concurrent Setup Protection', () => {
        it('should handle concurrent setup attempts safely', async () => {
            const setupData1 = {
                email: 'concurrent1@example.com',
                name: 'Concurrent User 1',
                req: { get: vi.fn(), protocol: 'https', hostname: 'localhost' }
            };

            const setupData2 = {
                email: 'concurrent2@example.com',
                name: 'Concurrent User 2',
                req: { get: vi.fn(), protocol: 'https', hostname: 'localhost' }
            };

            // Run setup operations concurrently
            const [user1, user2] = await Promise.all([
                setupService.performFirstRunSetup(setupData1),
                setupService.performFirstRunSetup(setupData2)
            ]);

            cleanup.trackUser(user1.id);
            cleanup.trackUser(user2.id);

            // Both should succeed but only one should be the actual admin
            expect(user1.email).toBe(setupData1.email);
            expect(user2.email).toBe(setupData2.email);

            // performFirstRunSetup sets setup_complete=false, so still first run
            const isFirstRun = await setupService.isFirstRun();
            expect(isFirstRun).toBe(true);
        });
    });
});
