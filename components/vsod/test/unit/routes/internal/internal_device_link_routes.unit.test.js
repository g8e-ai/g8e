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
import { createInternalDeviceLinkRouter } from '@vsod/routes/internal/internal_device_link_routes.js';

describe('Internal Device Link Routes [UNIT]', () => {
    let router;
    let mockDeviceLinkService;
    let mockAuthorizationMiddleware;

    beforeEach(() => {
        mockDeviceLinkService = {
            listLinks: vi.fn(),
            createLink: vi.fn(),
            getLink: vi.fn(),
            deleteLink: vi.fn(),
            revokeLink: vi.fn()
        };
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };

        router = createInternalDeviceLinkRouter({
            services: {
                deviceLinkService: mockDeviceLinkService
            },

            authorizationMiddleware: mockAuthorizationMiddleware
        });
    });

    const createMockReq = (overrides = {}) => ({
        params: {},
        body: {},
        query: {},
        headers: {},
        ...overrides
    });

    const createMockRes = () => {
        const res = {};
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        return res;
    };

    const getRouteHandler = (path, method = 'get') => {
        const layer = router.stack.find(s => s.route?.path === path && s.route?.methods[method]);
        if (!layer) throw new Error(`Route ${method.toUpperCase()} ${path} not found`);
        return layer.route.stack[layer.route.stack.length - 1].handle;
    };

    describe('GET /user/:userId', () => {
        it('should list device links for a user', async () => {
            const userId = 'user_123';
            const req = createMockReq({ params: { userId } });
            const res = createMockRes();
            const mockLinks = [{ token: 'abc', name: 'Test' }];

            mockDeviceLinkService.listLinks.mockResolvedValue({ success: true, links: mockLinks });

            await getRouteHandler('/user/:userId')(req, res);

            expect(mockDeviceLinkService.listLinks).toHaveBeenCalledWith(userId);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                links: mockLinks
            }));
        });

        it('should return 400 if userId is missing', async () => {
            const req = createMockReq({ params: {} });
            const res = createMockRes();

            await getRouteHandler('/user/:userId')(req, res);

            expect(res.status).toHaveBeenCalledWith(400);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'userId is required'
            }));
        });

        it('should return 500 if service returns error', async () => {
            const req = createMockReq({ params: { userId: 'user_123' } });
            const res = createMockRes();

            mockDeviceLinkService.listLinks.mockResolvedValue({ success: false, error: 'Service error' });

            await getRouteHandler('/user/:userId')(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Service error'
            }));
        });
    });

    describe('POST /user/:userId', () => {
        it('should create a device link for a user', async () => {
            const userId = 'user_123';
            const body = { name: 'New Link', max_uses: 5, expires_in_hours: 24 };
            const req = createMockReq({ params: { userId }, body });
            const res = createMockRes();
            const mockResult = {
                success: true,
                token: 'tok_123',
                operator_command: 'g8e link tok_123',
                name: 'New Link',
                max_uses: 5,
                expires_at: '2026-03-31T00:00:00Z'
            };

            mockDeviceLinkService.createLink.mockResolvedValue(mockResult);

            await getRouteHandler('/user/:userId', 'post')(req, res);

            expect(mockDeviceLinkService.createLink).toHaveBeenCalledWith(expect.objectContaining({
                user_id: userId,
                name: body.name,
                max_uses: body.max_uses,
                ttl_seconds: 24 * 3600
            }));
            expect(res.status).toHaveBeenCalledWith(201);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                token: mockResult.token
            }));
        });

        it('should return 400 if creation fails', async () => {
            const req = createMockReq({ params: { userId: 'user_123' }, body: {} });
            const res = createMockRes();

            mockDeviceLinkService.createLink.mockResolvedValue({ success: false, error: 'Creation failed' });

            await getRouteHandler('/user/:userId', 'post')(req, res);

            expect(res.status).toHaveBeenCalledWith(400);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Creation failed'
            }));
        });
    });

    describe('DELETE /:token', () => {
        const token = 'dlk_12345678901234567890123456789012'; // Valid format: dlk_ + 32 chars

        it('should revoke a device link by default', async () => {
            const req = createMockReq({ params: { token } });
            const res = createMockRes();

            mockDeviceLinkService.getLink.mockResolvedValue({ success: true, data: { user_id: 'user_123' } });
            mockDeviceLinkService.revokeLink.mockResolvedValue({ success: true });

            await getRouteHandler('/:token', 'delete')(req, res);

            expect(mockDeviceLinkService.revokeLink).toHaveBeenCalledWith(token, 'user_123');
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                message: 'Device link revoked successfully'
            }));
        });

        it('should permanently delete a device link if action=delete', async () => {
            const req = createMockReq({ params: { token }, query: { action: 'delete' } });
            const res = createMockRes();

            mockDeviceLinkService.getLink.mockResolvedValue({ success: true, data: { user_id: 'user_123' } });
            mockDeviceLinkService.deleteLink.mockResolvedValue({ success: true });

            await getRouteHandler('/:token', 'delete')(req, res);

            expect(mockDeviceLinkService.deleteLink).toHaveBeenCalledWith(token, 'user_123');
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                message: 'Device link deleted successfully'
            }));
        });

        it('should return 400 for invalid token format', async () => {
            const req = createMockReq({ params: { token: 'short' } });
            const res = createMockRes();

            await getRouteHandler('/:token', 'delete')(req, res);

            expect(res.status).toHaveBeenCalledWith(400);
        });

        it('should return 404 if link not found', async () => {
            const req = createMockReq({ params: { token } });
            const res = createMockRes();

            mockDeviceLinkService.getLink.mockResolvedValue({ success: false, error: 'Not found' });

            await getRouteHandler('/:token', 'delete')(req, res);

            expect(res.status).toHaveBeenCalledWith(404);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Not found'
            }));
        });
    });
});
