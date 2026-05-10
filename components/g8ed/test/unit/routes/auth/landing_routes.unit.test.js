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
import { createLandingRouter } from '@g8ed/routes/auth/landing_routes.js';
import { SESSION_COOKIE_NAME } from '@g8ed/constants/session.js';

describe('LandingRoutes Unit Tests', () => {
    let app;
    let mockWebSessionService;
    let mockUserService;
    let mockSetupService;

    beforeEach(() => {
        mockWebSessionService = {
            validateSession: vi.fn()
        };
        mockUserService = {};
        mockSetupService = {
            isFirstRun: vi.fn().mockResolvedValue(false)
        };

        const router = createLandingRouter({
            services: {
                webSessionService: mockWebSessionService,
                userService: mockUserService,
                setupService: mockSetupService
            }
        });

        app = express();
        // Simple cookie parser shim
        app.use((req, res, next) => {
            req.cookies = req.headers.cookie ? Object.fromEntries(
                req.headers.cookie.split('; ').map(c => c.split('='))
            ) : {};
            next();
        });
        
        // Mock render
        app.set('view engine', 'ejs'); // just to satisfy express
        app.engine('ejs', (path, options, callback) => callback(null, 'rendered-view'));
        
        app.use('/', router);
    });

    it('redirects to /chat if an active session exists', async () => {
        mockWebSessionService.validateSession.mockResolvedValue({
            is_active: true,
            user_id: 'test-user-id'
        });

        const res = await request(app)
            .get('/')
            .set('Cookie', [`${SESSION_COOKIE_NAME}=valid-session`]);

        expect(res.status).toBe(302);
        expect(res.header.location).toBe('/chat');
        expect(mockWebSessionService.validateSession).toHaveBeenCalledWith('valid-session', expect.any(Object));
    });

    it('redirects to /setup if setupService says it is first run', async () => {
        mockSetupService.isFirstRun.mockResolvedValue(true);

        const res = await request(app).get('/');

        expect(res.status).toBe(302);
        expect(res.header.location).toBe('/setup');
        expect(mockSetupService.isFirstRun).toHaveBeenCalled();
    });

    it('renders login page if no session exists and not first run', async () => {
        mockSetupService.isFirstRun.mockResolvedValue(false);

        const res = await request(app).get('/');

        expect(res.status).toBe(200);
        expect(res.text).toContain('Login');
    });

    it('renders login page if session is invalid and not first run', async () => {
        mockWebSessionService.validateSession.mockResolvedValue(null);
        mockSetupService.isFirstRun.mockResolvedValue(false);

        const res = await request(app)
            .get('/')
            .set('Cookie', [`${SESSION_COOKIE_NAME}=invalid-session`]);

        expect(res.status).toBe(200);
        expect(res.text).toContain('Login');
    });

    it('handles session validation error and proceeds to first run check', async () => {
        mockWebSessionService.validateSession.mockRejectedValue(new Error('DB Fail'));
        mockSetupService.isFirstRun.mockResolvedValue(false);

        const res = await request(app)
            .get('/')
            .set('Cookie', [`${SESSION_COOKIE_NAME}=some-session`]);

        expect(res.status).toBe(200);
        expect(res.text).toContain('Login');
    });
});
