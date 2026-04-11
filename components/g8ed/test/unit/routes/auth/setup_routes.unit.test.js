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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import express from 'express';
import request from 'supertest';
import { createSetupRouter } from '@g8ed/routes/auth/setup_routes.js';
import { SetupPaths } from '@g8ed/constants/api_paths.js';
import { USER_SETTINGS } from '@g8ed/models/settings_model.js';
import { UserRole } from '@g8ed/constants/auth.js';

describe('SetupRoutes Unit Tests', () => {
    let app;
    let mockSettingsService;
    let mockSetupService;
    let mockPasskeyAuthService;
    let mockRateLimiters;

    beforeEach(() => {
        mockSettingsService = {
            getSettingsForUI: vi.fn(),
            saveSettings: vi.fn(),
            updateUserSettings: vi.fn().mockResolvedValue({ success: true }),
            getPlatformSettings: vi.fn().mockResolvedValue({}),
            getSchema: vi.fn().mockReturnValue(USER_SETTINGS),
            savePlatformSettings: vi.fn().mockResolvedValue({ success: true })
        };
        mockSetupService = {
            isFirstRun: vi.fn().mockResolvedValue(true),
            derivePasskeyFields: vi.fn().mockReturnValue({
                passkey_rp_id: 'localhost',
                passkey_origin: 'https://localhost'
            }),
            createAdminUser: vi.fn(),
            performFirstRunSetup: vi.fn(),
            completeSetup: vi.fn()
        };
        mockPasskeyAuthService = {
            generateRegistrationChallenge: vi.fn(),
            verifyRegistrationResponse: vi.fn()
        };
        mockRateLimiters = {
            passkeyRateLimiter: vi.fn((req, res, next) => next())
        };

        const router = createSetupRouter({
            services: {
                settingsService: mockSettingsService,
                setupService: mockSetupService,
                passkeyAuthService: mockPasskeyAuthService
            },

            rateLimiters: mockRateLimiters
        });

        app = express();
        app.use(express.json());
        
        // Mock render
        app.set('view engine', 'ejs');
        app.engine('ejs', (path, options, callback) => callback(null, 'rendered-setup'));
        
        // Setup routes similar to how they are mounted in the app
        app.use('/', router);

        // Error handler
        app.use((err, req, res, next) => {
            const status = typeof err.getHttpStatus === 'function' ? err.getHttpStatus() : (err.status || 500);
            const response = {
                success: false,
                error: err.message,
                code: err.code
            };
            res.status(status).json(response);
        });
    });

    afterEach(() => {
        vi.clearAllMocks();
    });

    describe(`GET ${SetupPaths.WIZARD}`, () => {
        it('renders setup page on first run', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(true);
            const res = await request(app).get('/setup');
            expect(res.status).toBe(200);
            expect(res.text).toBe('rendered-setup');
        });

        it('redirects to / if not first run', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(false);
            const res = await request(app).get('/setup');
            expect(res.status).toBe(302);
            expect(res.header.location).toBe('/');
        });

        it('redirects to / when setup service throws error', async () => {
            mockSetupService.isFirstRun.mockRejectedValue(new Error('Service unavailable'));
            const res = await request(app).get('/setup');
            expect(res.status).toBe(302);
            expect(res.header.location).toBe('/');
        });

        it('redirects to / when setup service throws network error', async () => {
            const networkError = new Error('Network timeout');
            networkError.code = 'ETIMEDOUT';
            mockSetupService.isFirstRun.mockRejectedValue(networkError);
            const res = await request(app).get('/setup');
            expect(res.status).toBe(302);
            expect(res.header.location).toBe('/');
        });

        it('handles database connection errors gracefully', async () => {
            const dbError = new Error('Connection refused');
            dbError.code = 'ECONNREFUSED';
            mockSetupService.isFirstRun.mockRejectedValue(dbError);
            const res = await request(app).get('/setup');
            expect(res.status).toBe(302);
            expect(res.header.location).toBe('/');
        });

        it('handles isFirstRun returning non-boolean values', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(undefined);
            const res = await request(app).get('/setup');
            expect(res.status).toBe(302);
            expect(res.header.location).toBe('/');
        });

        it('handles isFirstRun returning null', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(null);
            const res = await request(app).get('/setup');
            expect(res.status).toBe(302);
            expect(res.header.location).toBe('/');
        });

        it('preserves request headers and context', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(true);
            const res = await request(app)
                .get('/setup')
                .set('X-Forwarded-Host', 'external.com')
                .set('X-Forwarded-Proto', 'https')
                .set('User-Agent', 'test-browser');
            
            expect(res.status).toBe(200);
            expect(res.text).toBe('rendered-setup');
        });

        it('handles requests with query parameters', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(true);
            const res = await request(app)
                .get('/setup?theme=dark&lang=en')
                .set('Accept-Language', 'en-US');
            
            expect(res.status).toBe(200);
            expect(res.text).toBe('rendered-setup');
        });
    });

    describe('Route Structure and Dependencies', () => {
        it('requires all dependencies', () => {
            expect(() => {
                createSetupRouter({});
            }).toThrow();

            expect(() => {
                createSetupRouter({
                    services: {
                        settingsService: mockSettingsService,
                        setupService: mockSetupService
                    }
                });
            }).toThrow();

            expect(() => {
                createSetupRouter({
                    services: {
                        settingsService: mockSettingsService,
                        setupService: mockSetupService,
                        passkeyAuthService: mockPasskeyAuthService
                    }
                });
            }).toThrow();

            expect(() => {
                createSetupRouter({
                    services: {
                        settingsService: mockSettingsService,
                        setupService: mockSetupService,
                        passkeyAuthService: mockPasskeyAuthService
                    },

                    rateLimiters: mockRateLimiters
                });
            }).not.toThrow();
        });

        it('creates router with correct structure', () => {
            const router = createSetupRouter({
                services: {
                    settingsService: mockSettingsService,
                    setupService: mockSetupService,
                    passkeyAuthService: mockPasskeyAuthService
                },

                rateLimiters: mockRateLimiters
            });

            expect(router).toBeDefined();
            expect(typeof router).toBe('function');
            expect(router.stack).toBeDefined();
            expect(Array.isArray(router.stack)).toBe(true);
        });

        it('contains setup wizard route', () => {
            const router = createSetupRouter({
                services: {
                    settingsService: mockSettingsService,
                    setupService: mockSetupService,
                    passkeyAuthService: mockPasskeyAuthService
                },

                rateLimiters: mockRateLimiters
            });

            const setupRoute = router.stack.find(
                layer => layer.route?.path === SetupPaths.WIZARD
            );
            
            expect(setupRoute).toBeDefined();
            expect(setupRoute.route).toBeDefined();
            expect(setupRoute.route.methods).toHaveProperty('get');
        });

        it('has correct path constant', () => {
            expect(SetupPaths.WIZARD).toBe('/setup');
        });
    });

    describe('Middleware Integration', () => {
        it('integrates with Express middleware chain', () => {
            const middlewareApp = express();
            const router = createSetupRouter({
                services: {
                    settingsService: mockSettingsService,
                    setupService: mockSetupService,
                    passkeyAuthService: mockPasskeyAuthService
                },

                rateLimiters: mockRateLimiters
            });
            
            expect(() => {
                middlewareApp.use('/test', router);
            }).not.toThrow();
        });

        it('uses rate limiter middleware', () => {
            const customRateLimiters = {
                passkeyRateLimiter: vi.fn((req, res, next) => next())
            };

            expect(() => {
                createSetupRouter({
                    services: {
                        settingsService: mockSettingsService,
                        setupService: mockSetupService,
                        passkeyAuthService: mockPasskeyAuthService
                    },

                    rateLimiters: customRateLimiters
                });
            }).not.toThrow();
        });
    });

    describe('Error Handling', () => {
        it('handles service timeout errors', async () => {
            const timeoutError = new Error('Operation timed out');
            timeoutError.code = 'TIMEOUT';
            mockSetupService.isFirstRun.mockRejectedValue(timeoutError);
            
            const res = await request(app).get('/setup');
            expect(res.status).toBe(302);
            expect(res.header.location).toBe('/');
        });

        it('handles permission errors', async () => {
            const permError = new Error('Permission denied');
            permError.code = 'EACCES';
            mockSetupService.isFirstRun.mockRejectedValue(permError);
            
            const res = await request(app).get('/setup');
            expect(res.status).toBe(302);
            expect(res.header.location).toBe('/');
        });

        it('handles validation errors', async () => {
            const validError = new Error('Invalid configuration');
            validError.code = 'VALIDATION_ERROR';
            mockSetupService.isFirstRun.mockRejectedValue(validError);
            
            const res = await request(app).get('/setup');
            expect(res.status).toBe(302);
            expect(res.header.location).toBe('/');
        });

        it('handles unexpected errors', async () => {
            mockSetupService.isFirstRun.mockRejectedValue(new Error('Unexpected error'));
            
            const res = await request(app).get('/setup');
            expect(res.status).toBe(302);
            expect(res.header.location).toBe('/');
        });
    });

    describe('Template Rendering', () => {
        it('renders setup template without additional data', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(true);
            
            const res = await request(app).get('/setup');
            expect(res.status).toBe(200);
            expect(res.text).toBe('rendered-setup');
        });

        it('handles template rendering errors', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(true);
            
            app.engine('ejs', (path, options, callback) => {
                callback(new Error('Template not found'));
            });
            
            const res = await request(app).get('/setup');
            expect(res.status).toBe(500);
            expect(res.body.success).toBe(false);
            expect(res.body.error).toBe('Template not found');
        });
    });

    describe('Service Interaction', () => {
        it('calls setupService.isFirstRun exactly once', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(true);
            
            await request(app).get('/setup');
            
            expect(mockSetupService.isFirstRun).toHaveBeenCalledTimes(1);
        });

        it('does not call other services during GET request', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(true);
            
            await request(app).get('/setup');
            
            expect(mockSettingsService.getPlatformSettings).not.toHaveBeenCalled();
            expect(mockPasskeyAuthService.generateRegistrationChallenge).not.toHaveBeenCalled();
            expect(mockSetupService.performFirstRunSetup).not.toHaveBeenCalled();
        });

        it('handles service returning promises', async () => {
            mockSetupService.isFirstRun.mockReturnValue(Promise.resolve(true));
            
            const res = await request(app).get('/setup');
            expect(res.status).toBe(200);
            expect(res.text).toBe('rendered-setup');
        });
    });

    describe('Request Context', () => {
        it('handles requests with different user agents', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(true);
            
            const agents = [
                'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36',
                'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36',
                'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36'
            ];

            for (const agent of agents) {
                const res = await request(app)
                    .get('/setup')
                    .set('User-Agent', agent);
                
                expect(res.status).toBe(200);
                expect(res.text).toBe('rendered-setup');
            }
        });

        it('handles requests with different IP addresses', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(true);
            
            const ips = ['127.0.0.1', '192.168.1.1', '10.0.0.1', '::1'];

            for (const ip of ips) {
                const res = await request(app)
                    .get('/setup')
                    .set('X-Forwarded-For', ip);
                
                expect(res.status).toBe(200);
                expect(res.text).toBe('rendered-setup');
            }
        });

        it('handles requests with forwarded headers', async () => {
            mockSetupService.isFirstRun.mockResolvedValue(true);
            
            const res = await request(app)
                .get('/setup')
                .set('X-Forwarded-Host', 'external.example.com')
                .set('X-Forwarded-Proto', 'https')
                .set('X-Forwarded-For', '203.0.113.1');
            
            expect(res.status).toBe(200);
            expect(res.text).toBe('rendered-setup');
        });
    });

    describe('Async Operation Handling', () => {
        it('handles delayed service responses', async () => {
            mockSetupService.isFirstRun.mockImplementation(
                () => new Promise(resolve => setTimeout(() => resolve(true), 100))
            );
            
            const res = await request(app).get('/setup');
            expect(res.status).toBe(200);
            expect(res.text).toBe('rendered-setup');
        });

        it('handles promise rejection without unhandled rejection', async () => {
            mockSetupService.isFirstRun.mockRejectedValue(new Error('Async error'));
            
            // Should not cause unhandled promise rejection
            const res = await request(app).get('/setup');
            expect(res.status).toBe(302);
            expect(res.header.location).toBe('/');
        });
    });
});
