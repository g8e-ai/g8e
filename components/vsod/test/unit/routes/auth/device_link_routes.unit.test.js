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
import { createDeviceLinkRouter } from '@vsod/routes/auth/device_link_routes.js';
import { AuthPaths, DeviceLinkPaths } from '@vsod/constants/api_paths.js';
import { ApiKeyError, DeviceLinkError, WEB_SESSION_ID_HEADER } from '@vsod/constants/auth.js';

describe('DeviceLinkRoutes Unit Tests', () => {
    let app;
    let mockDeviceLinkService;
    let mockAuthMiddleware;
    let mockRateLimiters;

    beforeEach(() => {
        mockDeviceLinkService = {
            generateLink: vi.fn(),
            createLink: vi.fn(),
            listLinks: vi.fn(),
            revokeLink: vi.fn(),
            deleteLink: vi.fn(),
            registerDevice: vi.fn()
        };

        mockAuthMiddleware = {
            requireAuth: (req, res, next) => {
                req.userId = 'test-user-id';
                req.session = { user_data: { organization_id: 'test-org-id' } };
                next();
            }
        };

        const noopLimiter = (req, res, next) => next();
        mockRateLimiters = {
            deviceLinkRateLimiter: noopLimiter,
            deviceLinkGenerateLimiter: noopLimiter,
            deviceLinkCreateRateLimiter: noopLimiter,
            deviceLinkListRateLimiter: noopLimiter,
            deviceLinkRevokeRateLimiter: noopLimiter
        };

        const { authRouter, deviceLinkRouter, registerRouter } = createDeviceLinkRouter({
            services: {
                deviceLinkService: mockDeviceLinkService
            },

            authMiddleware: mockAuthMiddleware,
            rateLimiters: mockRateLimiters
        });

        app = express();
        app.use(express.json());
        
        // Setup routes similar to how they are mounted in the app
        app.use('/api/auth', authRouter);
        app.use('/api/device-links', deviceLinkRouter);
        app.use('/auth/link', registerRouter);

        // Error handler
        app.use((err, req, res, next) => {
            const status = typeof err.getHttpStatus === 'function' ? err.getHttpStatus() : (err.status || 500);
            res.status(status).json({
                success: false,
                error: err.message
            });
        });
    });

    describe(`POST ${AuthPaths.LINK_GENERATE}`, () => {
        it('successfully generates a link', async () => {
            const mockResult = {
                success: true,
                token: 'test-token',
                operator_command: 'g8e.operator bind --token test-token',
                expires_at: Date.now() + 3600000
            };
            mockDeviceLinkService.generateLink.mockResolvedValue(mockResult);

            const res = await request(app)
                .post('/api/auth/link/generate')
                .send({ operator_id: 'test-op-id' });

            expect(res.status).toBe(201);
            expect(res.body.success).toBe(true);
            expect(res.body.token).toBe('test-token');
            expect(mockDeviceLinkService.generateLink).toHaveBeenCalledWith(expect.objectContaining({
                user_id: 'test-user-id',
                organization_id: 'test-org-id',
                operator_id: 'test-op-id'
            }));
        });

        it('handles service failure', async () => {
            mockDeviceLinkService.generateLink.mockResolvedValue({ success: false, error: 'Failed' });

            const res = await request(app)
                .post('/api/auth/link/generate')
                .send({ operator_id: 'test-op-id' });

            expect(res.status).toBe(400);
            expect(res.body.error).toBe('Failed');
        });

        it('handles internal error', async () => {
            mockDeviceLinkService.generateLink.mockRejectedValue(new Error('Internal'));

            const res = await request(app)
                .post('/api/auth/link/generate')
                .send({ operator_id: 'test-op-id' });

            expect(res.status).toBe(500);
            expect(res.body.error).toBe('Internal');
        });
    });

    describe(`POST /api/device-links`, () => {
        it('successfully creates a link', async () => {
            const mockResult = {
                success: true,
                token: 'valid-token-12345',
                operator_command: 'cmd',
                name: 'test-link',
                max_uses: 1,
                expires_at: Date.now() + 3600000
            };
            mockDeviceLinkService.createLink.mockResolvedValue(mockResult);

            const res = await request(app)
                .post('/api/device-links')
                .send({ name: 'test-link', max_uses: 1, expires_in_hours: 1 });

            expect(res.status).toBe(201);
            expect(res.body.token).toBe('valid-token-12345');
            expect(mockDeviceLinkService.createLink).toHaveBeenCalledWith(expect.objectContaining({
                name: 'test-link',
                max_uses: 1,
                ttl_seconds: 3600
            }));
        });
    });

    describe(`GET /api/device-links`, () => {
        it('successfully lists links', async () => {
            const mockLinks = [{ token: 'valid-token-12345' }, { token: 'valid-token-67890' }];
            mockDeviceLinkService.listLinks.mockResolvedValue({ success: true, links: mockLinks });

            const res = await request(app).get('/api/device-links');

            expect(res.status).toBe(200);
            expect(res.body.links).toEqual(mockLinks);
        });
    });

    describe(`DELETE /api/device-links/:token`, () => {
        it('successfully revokes a link', async () => {
            mockDeviceLinkService.revokeLink.mockResolvedValue({ success: true });

            const res = await request(app).delete('/api/device-links/dlk_12345678901234567890123456789012');

            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(mockDeviceLinkService.revokeLink).toHaveBeenCalledWith('dlk_12345678901234567890123456789012', 'test-user-id');
        });

        it('successfully deletes a link when action=delete', async () => {
            mockDeviceLinkService.deleteLink.mockResolvedValue({ success: true });

            const res = await request(app).delete('/api/device-links/dlk_12345678901234567890123456789012?action=delete');

            expect(res.status).toBe(200);
            expect(res.body.success).toBe(true);
            expect(mockDeviceLinkService.deleteLink).toHaveBeenCalledWith('dlk_12345678901234567890123456789012', 'test-user-id');
        });

        it('returns 400 for invalid token format', async () => {
            const res = await request(app).delete('/api/device-links/invalid');

            expect(res.status).toBe(400);
            expect(res.body.error).toBe(DeviceLinkError.INVALID_TOKEN_FORMAT);
        });
    });

    describe(`POST /auth/link/:token/register`, () => {
        it('successfully registers a device', async () => {
            const mockResult = {
                success: true,
                operator_session_id: 'op-sess-id',
                operator_id: 'op-id'
            };
            mockDeviceLinkService.registerDevice.mockResolvedValue(mockResult);

            const res = await request(app)
                .post('/auth/link/dlk_12345678901234567890123456789012/register')
                .send({
                    hostname: 'test-host',
                    os: 'linux',
                    arch: 'amd64',
                    system_fingerprint: 'abcdef1234567890'
                });

            expect(res.status).toBe(200);
            expect(res.body.operator_session_id).toBe('op-sess-id');
            expect(mockDeviceLinkService.registerDevice).toHaveBeenCalledWith(
                'dlk_12345678901234567890123456789012',
                expect.objectContaining({
                    hostname: 'test-host',
                    os: 'linux',
                    arch: 'amd64',
                    system_fingerprint: 'abcdef1234567890'
                })
            );
        });

        it('returns 400 for invalid token format', async () => {
            const res = await request(app)
                .post('/auth/link/invalid/register')
                .send({ hostname: 'test', os: 'linux', arch: 'amd64', system_fingerprint: 'abc' });

            expect(res.status).toBe(400);
            expect(res.body.error).toBe(DeviceLinkError.INVALID_TOKEN_FORMAT);
        });
    });
});
