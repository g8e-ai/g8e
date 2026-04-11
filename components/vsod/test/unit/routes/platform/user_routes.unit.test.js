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
import { createUserRouter } from '@vsod/routes/platform/user_routes.js';
import { UserPaths } from '@vsod/constants/api_paths.js';
import { ApiKeyError } from '@vsod/constants/auth.js';
import { ValidationError, ResourceNotFoundError, G8eKeyError } from '@vsod/services/error_service.js';

describe('User Routes [UNIT]', () => {
    let router;
    let mockUserService;
    let mockAuthMiddleware;

    beforeEach(() => {
        mockUserService = {
            getUser: vi.fn(),
            updateUser: vi.fn(),
            refreshUserG8eKey: vi.fn()
        };
        mockAuthMiddleware = {
            requireAuth: vi.fn((req, res, next) => next()),
            requireAdmin: vi.fn((req, res, next) => next())
        };

        router = createUserRouter({
            services: {
                userService: mockUserService
            },

            authMiddleware: mockAuthMiddleware
        });
    });

    const createMockReq = (overrides = {}) => ({
        userId: 'user_123',
        body: {},
        ...overrides
    });

    const createMockRes = () => {
        const res = {};
        res.status = vi.fn().mockReturnValue(res);
        res.json = vi.fn().mockReturnValue(res);
        return res;
    };

    describe(`GET ${UserPaths.ME}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === UserPaths.ME);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should fetch and return current user', async () => {
            const mockUser = {
                id: 'user_123',
                email: 'test@example.com',
                forClient: vi.fn().mockReturnValue({ id: 'user_123', email: 'test@example.com' })
            };
            mockUserService.getUser.mockResolvedValue(mockUser);

            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockUserService.getUser).toHaveBeenCalledWith('user_123');
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                id: 'user_123',
                email: 'test@example.com'
            }));
        });

        it('should return 404 if user not found', async () => {
            mockUserService.getUser.mockResolvedValue(null);

            const req = createMockReq();
            const res = createMockRes();
            const mockNext = vi.fn();

            await getRoute()(req, res, mockNext);

            expect(mockNext).toHaveBeenCalledWith(expect.any(Error));
            expect(mockNext).toHaveBeenCalledWith(expect.objectContaining({
                message: 'User not found'
            }));
        });
    });

    describe(`PATCH ${UserPaths.DEV_LOGS}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === UserPaths.DEV_LOGS);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should update dev logs enabled status', async () => {
            mockUserService.updateUser.mockResolvedValue({ dev_logs_enabled: true });

            const req = createMockReq({ body: { enabled: true } });
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockUserService.updateUser).toHaveBeenCalledWith('user_123', { dev_logs_enabled: true });
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                message: 'Dev logs enabled',
                dev_logs_enabled: true
            }));
        });

        it('should reject non-boolean enabled parameter', async () => {
            const req = createMockReq({ body: { enabled: 'not-boolean' } });
            const res = createMockRes();
            const mockNext = vi.fn();

            await getRoute()(req, res, mockNext);

            expect(mockNext).toHaveBeenCalledWith(expect.any(ValidationError));
            expect(mockNext).toHaveBeenCalledWith(expect.objectContaining({
                message: 'enabled (boolean) is required'
            }));
        });
    });

    describe(`POST ${UserPaths.REFRESH_G8E_KEY}`, () => {
        const getRoute = () => {
            const layer = router.stack.find(s => s.route?.path === UserPaths.REFRESH_G8E_KEY);
            return layer.route.stack[layer.route.stack.length - 1].handle;
        };

        it('should refresh g8e_key successfully', async () => {
            const mockUser = {
                id: 'user_123',
                email: 'test@example.com',
                organization_id: 'org_123'
            };
            mockUserService.getUser.mockResolvedValue(mockUser);
            mockUserService.refreshUserG8eKey.mockResolvedValue({
                success: true,
                api_key: 'dpk_new_key_1234567890abcdef'
            });

            const req = createMockReq();
            const res = createMockRes();

            await getRoute()(req, res);

            expect(mockUserService.getUser).toHaveBeenCalledWith('user_123');
            expect(mockUserService.refreshUserG8eKey).toHaveBeenCalledWith('user_123', 'org_123');
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                message: 'g8e key refreshed successfully',
                g8e_key: 'dpk_new_key_1234567890abcdef'
            }));
        });

        it('should return 404 if user not found', async () => {
            mockUserService.getUser.mockResolvedValue(null);

            const req = createMockReq();
            const res = createMockRes();
            const mockNext = vi.fn();

            await getRoute()(req, res, mockNext);

            expect(mockNext).toHaveBeenCalledWith(expect.any(ResourceNotFoundError));
        });

        it('should return error if refresh fails', async () => {
            const mockUser = {
                id: 'user_123',
                email: 'test@example.com',
                organization_id: 'org_123'
            };
            mockUserService.getUser.mockResolvedValue(mockUser);
            mockUserService.refreshUserG8eKey.mockRejectedValue(new G8eKeyError('Failed to issue new key'));

            const req = createMockReq();
            const res = createMockRes();
            const mockNext = vi.fn();

            await getRoute()(req, res, mockNext);

            expect(mockNext).toHaveBeenCalledWith(expect.any(G8eKeyError));
            expect(mockNext).toHaveBeenCalledWith(expect.objectContaining({
                message: 'Failed to issue new key'
            }));
        });
    });
});
