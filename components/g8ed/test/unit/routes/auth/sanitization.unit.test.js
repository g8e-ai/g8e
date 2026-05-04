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
import { createAuthRouter } from '../../../../routes/auth/auth_routes.js';

describe('Auth Routes - Sanitization', () => {
    let app;
    let mockServices;
    let mockAuthMiddleware;

    beforeEach(() => {
        mockServices = {
            webSessionService: {},
            userService: {
                findUserByEmail: vi.fn().mockResolvedValue(null),
                createUser: vi.fn().mockImplementation((data) => Promise.resolve({ id: 'user-123', ...data }))
            },
            loginSecurityService: {},
            setupService: {
                isFirstRun: vi.fn().mockResolvedValue(false),
                performFirstRunSetup: vi.fn()
            },
            passkeyAuthService: {
                generateRegistrationChallenge: vi.fn().mockResolvedValue({ challenge: 'abc' })
            }
        };

        mockAuthMiddleware = {
            requireAuth: (req, res, next) => next(),
            requireAdmin: (req, res, next) => next()
        };

        app = express();
        app.use(express.json());
        app.use('/api/auth', createAuthRouter({ 
            services: mockServices, 
            authMiddleware: mockAuthMiddleware 
        }));
    });

    it('should strip HTML tags from the name field', async () => {
        const payload = {
            email: 'test@example.com',
            name: 'John <script>alert(1)</script>Doe'
        };

        const response = await request(app)
            .post('/api/auth/register')
            .send(payload);

        expect(response.status).toBe(201);
        expect(mockServices.userService.createUser).toHaveBeenCalledWith(expect.objectContaining({
            name: 'John alert(1)Doe'
        }));
    });

    it('should strip reappearing HTML tags using a loop', async () => {
        const payload = {
            email: 'test@example.com',
            name: 'John <scr<script>ipt>Doe'
        };

        const response = await request(app)
            .post('/api/auth/register')
            .send(payload);

        expect(response.status).toBe(201);
        expect(mockServices.userService.createUser).toHaveBeenCalledWith(expect.objectContaining({
            name: 'John Doe'
        }));
    });

    it('should strip other HTML tags', async () => {
        const payload = {
            email: 'test@example.com',
            name: '<b>John</b> <i>Doe</i>'
        };

        const response = await request(app)
            .post('/api/auth/register')
            .send(payload);

        expect(response.status).toBe(201);
        expect(mockServices.userService.createUser).toHaveBeenCalledWith(expect.objectContaining({
            name: 'John Doe'
        }));
    });
});
