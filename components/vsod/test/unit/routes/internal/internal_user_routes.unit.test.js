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
import { createInternalUserRouter } from '@vsod/routes/internal/internal_user_routes.js';
import { UserRole, ApiKeyError } from '@vsod/constants/auth.js';
import { SessionEndReason } from '@vsod/constants/session.js';
import { ResourceNotFoundError } from '@vsod/services/error_service.js';

describe('Internal User Routes [UNIT]', () => {
    let router;
    let mockUserService;
    let mockWebSessionService;
    let mockPasskeyAuthService;
    let mockAuthorizationMiddleware;

    beforeEach(() => {
        mockUserService = {
            getUserStats: vi.fn(),
            listUsers: vi.fn(),
            findUserByEmail: vi.fn(),
            getUser: vi.fn(),
            createUser: vi.fn(),
            updateUser: vi.fn(),
            deleteUser: vi.fn()
        };
        mockWebSessionService = {
            invalidateAllUserSessions: vi.fn()
        };
        mockPasskeyAuthService = {
            listCredentials: vi.fn(),
            revokeCredential: vi.fn(),
            revokeAllCredentials: vi.fn()
        };
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };

        router = createInternalUserRouter({
            services: {
                userService: mockUserService,
                webSessionService: mockWebSessionService,
                passkeyAuthService: mockPasskeyAuthService
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

    describe('GET /stats', () => {
        it('should return user stats', async () => {
            const req = createMockReq();
            const res = createMockRes();
            const mockStats = { success: true, data: { total_users: 10 } };
            mockUserService.getUserStats.mockResolvedValue(mockStats);

            await getRouteHandler('/stats')(req, res);

            expect(mockUserService.getUserStats).toHaveBeenCalled();
            expect(res.json).toHaveBeenCalledWith(mockStats);
        });

        it('should handle errors', async () => {
            const req = createMockReq();
            const res = createMockRes();
            mockUserService.getUserStats.mockRejectedValue(new Error('Stats failed'));

            await getRouteHandler('/stats')(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({ error: 'Stats failed' }));
        });
    });

    describe('GET /', () => {
        it('should list users with default limit', async () => {
            const req = createMockReq();
            const res = createMockRes();
            const mockUsers = [{ id: 'u1', forWire: () => ({ id: 'u1' }) }];

            mockUserService.listUsers.mockResolvedValue(mockUsers);

            await getRouteHandler('/')(req, res);

            expect(mockUserService.listUsers).toHaveBeenCalled();
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                users: [{ id: 'u1' }],
                count: 1
            }));
        });

        it('should respect limit parameter within bounds', async () => {
            const req = createMockReq({ query: { limit: '50' } });
            const res = createMockRes();
            mockUserService.listUsers.mockResolvedValue([]);

            await getRouteHandler('/')(req, res);

            expect(mockUserService.listUsers).toHaveBeenCalledWith(50);
        });
    });

    describe('GET /email/:email', () => {
        it('should find user by email', async () => {
            const email = 'test@example.com';
            const req = createMockReq({ params: { email } });
            const res = createMockRes();
            const mockUser = { id: 'u1', email, forWire: () => ({ id: 'u1', email }) };

            mockUserService.findUserByEmail.mockResolvedValue(mockUser);

            await getRouteHandler('/email/:email')(req, res);

            expect(mockUserService.findUserByEmail).toHaveBeenCalledWith(email);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                user: { id: 'u1', email }
            }));
        });

        it('should return 404 if user not found', async () => {
            const req = createMockReq({ params: { email: 'notfound@example.com' } });
            const res = createMockRes();
            const mockNext = vi.fn();
            mockUserService.findUserByEmail.mockResolvedValue(null);

            await getRouteHandler('/email/:email')(req, res, mockNext);

            expect(mockNext).toHaveBeenCalledWith(expect.any(Error));
        });
    });

    describe('GET /:userId', () => {
        it('should find user by ID', async () => {
            const userId = 'u123';
            const req = createMockReq({ params: { userId } });
            const res = createMockRes();
            const mockUser = { id: userId, email: 't@e.com', forWire: () => ({ id: userId }) };

            mockUserService.getUser.mockResolvedValue(mockUser);

            await getRouteHandler('/:userId')(req, res);

            expect(mockUserService.getUser).toHaveBeenCalledWith(userId);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({ user: { id: userId } }));
        });

        it('should return 404 if not found', async () => {
            const req = createMockReq({ params: { userId: 'u999' } });
            const res = createMockRes();
            const mockNext = vi.fn();
            mockUserService.getUser.mockResolvedValue(null);

            await getRouteHandler('/:userId')(req, res, mockNext);

            expect(mockNext).toHaveBeenCalledWith(expect.any(Error));
        });
    });

    describe('POST /', () => {
        it('should create a new user', async () => {
            const body = { email: 'new@example.com', name: 'New User' };
            const req = createMockReq({ body });
            const res = createMockRes();
            const mockUser = { id: 'u_new', ...body, forWire: () => ({ id: 'u_new' }) };

            mockUserService.findUserByEmail.mockResolvedValue(null);
            mockUserService.createUser.mockResolvedValue(mockUser);

            await getRouteHandler('/', 'post')(req, res);

            expect(mockUserService.createUser).toHaveBeenCalledWith(expect.objectContaining({
                email: body.email,
                name: body.name
            }));
            expect(res.status).toHaveBeenCalledWith(201);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                user: { id: 'u_new' }
            }));
        });

        it('should return 409 if user already exists', async () => {
            const req = createMockReq({ body: { email: 'existing@example.com', name: 'Test User' } });
            const res = createMockRes();
            const mockNext = vi.fn();
            mockUserService.findUserByEmail.mockResolvedValue({ id: 'u1' });

            await getRouteHandler('/', 'post')(req, res, mockNext);

            expect(mockNext).toHaveBeenCalledWith(expect.any(Error));
        });

        it('should return 500 if validation fails', async () => {
            const req = createMockReq({ body: { email: 'invalid' } }); // Missing name
            const res = createMockRes();
            const mockNext = vi.fn();

            await getRouteHandler('/', 'post')(req, res, mockNext);

            expect(mockNext).toHaveBeenCalledWith(expect.any(Error));
        });
    });

    describe('PATCH /:userId/roles', () => {
        it('should update user roles (set)', async () => {
            const userId = 'u123';
            const body = { role: UserRole.ADMIN, action: 'set' };
            const req = createMockReq({ params: { userId }, body });
            const res = createMockRes();
            const mockUser = { id: userId, roles: [UserRole.USER], forWire: () => ({ id: userId }) };

            mockUserService.getUser.mockResolvedValue(mockUser);
            mockUserService.updateUser.mockResolvedValue({ ...mockUser, roles: [UserRole.ADMIN], forWire: () => ({}) });

            await getRouteHandler('/:userId/roles', 'patch')(req, res);

            expect(mockUserService.updateUser).toHaveBeenCalledWith(userId, { roles: [UserRole.ADMIN] });
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({ user: {} }));
        });

        it('should update user roles (add)', async () => {
            const userId = 'u123';
            const body = { role: UserRole.ADMIN, action: 'add' };
            const req = createMockReq({ params: { userId }, body });
            const res = createMockRes();
            const mockUser = { id: userId, roles: [UserRole.USER], forWire: () => ({ id: userId }) };

            mockUserService.getUser.mockResolvedValue(mockUser);
            mockUserService.updateUser.mockResolvedValue({ ...mockUser, roles: [UserRole.USER, UserRole.ADMIN], forWire: () => ({}) });

            await getRouteHandler('/:userId/roles', 'patch')(req, res);

            expect(mockUserService.updateUser).toHaveBeenCalledWith(userId, { roles: [UserRole.USER, UserRole.ADMIN] });
        });

        it('should update user roles (remove)', async () => {
            const userId = 'u123';
            const body = { role: UserRole.USER, action: 'remove' };
            const req = createMockReq({ params: { userId }, body });
            const res = createMockRes();
            const mockUser = { id: userId, roles: [UserRole.USER, UserRole.ADMIN], forWire: () => ({ id: userId }) };

            mockUserService.getUser.mockResolvedValue(mockUser);
            mockUserService.updateUser.mockResolvedValue({ ...mockUser, roles: [UserRole.ADMIN], forWire: () => ({}) });

            await getRouteHandler('/:userId/roles', 'patch')(req, res);

            expect(mockUserService.updateUser).toHaveBeenCalledWith(userId, { roles: [UserRole.ADMIN] });
        });

        it('should return 400 for invalid role', async () => {
            const req = createMockReq({ params: { userId: 'u1' }, body: { role: 'INVALID', action: 'set' } });
            const res = createMockRes();
            const mockNext = vi.fn();

            await getRouteHandler('/:userId/roles', 'patch')(req, res, mockNext);

            expect(mockNext).toHaveBeenCalledWith(expect.any(Error));
        });
    });

    describe('GET /:userId/passkeys', () => {
        it('should list user passkeys', async () => {
            const userId = 'u123';
            const req = createMockReq({ params: { userId } });
            const res = createMockRes();
            const mockCreds = [{ id: 'c1' }];

            mockPasskeyAuthService.listCredentials.mockResolvedValue(mockCreds);

            await getRouteHandler('/:userId/passkeys')(req, res);

            expect(mockPasskeyAuthService.listCredentials).toHaveBeenCalledWith(userId);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                message: 'Passkeys listed successfully',
                credentials: mockCreds,
                count: mockCreds.length
            }));
        });
    });

    describe('DELETE /:userId/passkeys/:credentialId', () => {
        it('should revoke a passkey', async () => {
            const userId = 'u123';
            const credentialId = 'c123';
            const req = createMockReq({ params: { userId, credentialId } });
            const res = createMockRes();

            mockPasskeyAuthService.revokeCredential.mockResolvedValue({ userExists: true, found: true, remaining: 0 });

            await getRouteHandler('/:userId/passkeys/:credentialId', 'delete')(req, res);

            expect(mockPasskeyAuthService.revokeCredential).toHaveBeenCalledWith(userId, credentialId);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({ 
                message: 'Passkey credential revoked successfully',
                user_id: userId, 
                credential_id: credentialId, 
                remaining: 0 
            }));
        });
    });

    describe('DELETE /:userId', () => {
        it('should delete a user and invalidate sessions', async () => {
            const userId = 'u123';
            const req = createMockReq({ params: { userId } });
            const res = createMockRes();

            mockUserService.getUser.mockResolvedValue({ id: userId, email: 't@e.com' });
            mockUserService.deleteUser.mockResolvedValue(true);

            await getRouteHandler('/:userId', 'delete')(req, res);

            expect(mockWebSessionService.invalidateAllUserSessions).toHaveBeenCalledWith(userId, SessionEndReason.USER_DELETED);
            expect(mockUserService.deleteUser).toHaveBeenCalledWith(userId);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({ 
                message: 'User deleted successfully',
                user_id: userId 
            }));
        });

        it('should return 404 if user does not exist', async () => {
            const req = createMockReq({ params: { userId: 'u999' } });
            const res = createMockRes();
            const mockNext = vi.fn();
            mockUserService.getUser.mockResolvedValue(null);

            await getRouteHandler('/:userId', 'delete')(req, res, mockNext);

            expect(mockNext).toHaveBeenCalledWith(expect.any(Error));
            expect(mockNext).toHaveBeenCalledWith(expect.objectContaining({
                message: 'User not found'
            }));
        });
    });
});
