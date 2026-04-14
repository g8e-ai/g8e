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
import { createInternalSessionRouter } from '@g8ed/routes/internal/internal_session_routes.js';
import { SessionType } from '@g8ed/constants/session.js';
import { HTTP_G8E_SERVICE_HEADER } from '@g8ed/constants/headers.js';
import { UserRole } from '@g8ed/constants/auth.js';

describe('Internal Session Routes [UNIT]', () => {
    let router;
    let mockWebSessionService;
    let mockUserService;
    let mockAuthorizationMiddleware;

    beforeEach(() => {
        mockWebSessionService = {
            validateSession: vi.fn()
        };
        mockUserService = {
            getUser: vi.fn()
        };
        mockAuthorizationMiddleware = {
            requireInternalOrigin: vi.fn((req, res, next) => next())
        };

        router = createInternalSessionRouter({
            services: {
                webSessionService: mockWebSessionService,
                userService: mockUserService
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

    describe('GET /:sessionId', () => {
        const sessionId = 'sess_123456789012';

        it('should validate a web session and return session data', async () => {
            const req = createMockReq({ 
                params: { sessionId },
                headers: { [HTTP_G8E_SERVICE_HEADER.toLowerCase()]: 'g8ee' }
            });
            const res = createMockRes();
            const mockSession = {
                id: sessionId,
                user_id: 'user_123',
                organization_id: 'org_123',
                session_type: SessionType.WEB,
                is_active: true,
                user_data: { email: 'test@example.com', name: 'Test User' },
                created_at: '2026-03-31T00:00:00Z',
                absolute_expires_at: '2026-03-31T01:00:00Z'
            };

            mockWebSessionService.validateSession.mockResolvedValue(mockSession);
            mockUserService.getUser.mockResolvedValue({ id: 'user_123', roles: [UserRole.ADMIN] });

            await getRouteHandler('/:sessionId')(req, res);

            expect(mockWebSessionService.validateSession).toHaveBeenCalledWith(sessionId);
            expect(mockUserService.getUser).toHaveBeenCalledWith('user_123');
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                success: true,
                message: 'WebSession validated successfully',
                session_id: 'sess_123456789012',
                user_id: 'user_123',
                valid: true,
                validation_details: expect.objectContaining({
                    user_data: expect.objectContaining({
                        roles: [UserRole.ADMIN]
                    })
                })
            }));
        });

        it('should return 400 if sessionId is missing', async () => {
            const req = createMockReq({ params: {} });
            const res = createMockRes();

            await getRouteHandler('/:sessionId')(req, res);

            expect(res.status).toHaveBeenCalledWith(400);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'sessionId is required'
            }));
        });

        it('should return 200 with error if session not found', async () => {
            const req = createMockReq({ params: { sessionId } });
            const res = createMockRes();

            mockWebSessionService.validateSession.mockResolvedValue(null);

            await getRouteHandler('/:sessionId')(req, res);

            expect(res.status).toHaveBeenCalledWith(200);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'WebSession not found or expired'
            }));
        });

        it('should return 200 with error if session type is invalid', async () => {
            const req = createMockReq({ params: { sessionId } });
            const res = createMockRes();

            mockWebSessionService.validateSession.mockResolvedValue({
                session_type: 'INVALID'
            });

            await getRouteHandler('/:sessionId')(req, res);

            expect(res.status).toHaveBeenCalledWith(200);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Invalid session type'
            }));
        });

        it('should return 200 with error if session is inactive', async () => {
            const req = createMockReq({ params: { sessionId } });
            const res = createMockRes();

            mockWebSessionService.validateSession.mockResolvedValue({
                session_type: SessionType.WEB,
                is_active: false
            });

            await getRouteHandler('/:sessionId')(req, res);

            expect(res.status).toHaveBeenCalledWith(200);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'WebSession is inactive'
            }));
        });

        it('should handle errors during validation', async () => {
            const req = createMockReq({ params: { sessionId } });
            const res = createMockRes();

            mockWebSessionService.validateSession.mockRejectedValue(new Error('Validation error'));

            await getRouteHandler('/:sessionId')(req, res);

            expect(res.status).toHaveBeenCalledWith(500);
            expect(res.json).toHaveBeenCalledWith(expect.objectContaining({
                error: 'Validation error'
            }));
        });
    });
});
