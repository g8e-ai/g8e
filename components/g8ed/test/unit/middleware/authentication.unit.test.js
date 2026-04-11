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
import { createAuthMiddleware } from '@g8ed/middleware/authentication.js';
import { AuthError, WEB_SESSION_ID_HEADER, BEARER_PREFIX, UserRole } from '@g8ed/constants/auth.js';
import { SessionType } from '@g8ed/constants/session.js';
import { AuthenticationError, AuthorizationError } from '@g8ed/services/error_service.js';

describe('Authentication Middleware', () => {
    let webSessionService;
    let setupService;
    let userService;
    let settingsService;
    let bindingService;
    let middleware;
    let req;
    let res;
    let next;

    beforeEach(() => {
        webSessionService = {
            validateSession: vi.fn()
        };
        setupService = {
            isFirstRun: vi.fn()
        };
        userService = {};
        settingsService = {
            getPlatformSettings: vi.fn()
        };
        bindingService = {
            resolveBoundOperators: vi.fn(),
            resolveBoundOperatorsForUser: vi.fn()
        };
        middleware = createAuthMiddleware({ 
            webSessionService, 
            setupService,
            userService,
            settingsService,
            bindingService
        });
        
        req = {
            headers: {
                host: 'localhost'
            },
            cookies: {},
            query: {},
            path: '/api/test',
            method: 'GET',
            ip: '127.0.0.1',
            socket: { remoteAddress: '127.0.0.1' }
        };
        res = {
            status: vi.fn().mockReturnThis(),
            json: vi.fn().mockReturnThis(),
            clearCookie: vi.fn().mockReturnThis(),
            redirect: vi.fn().mockReturnThis(),
            render: vi.fn().mockReturnThis()
        };
        next = vi.fn();
    });

    describe('requireAuth', () => {
        it('should throw AuthenticationError if session ID is in query params', async () => {
            req.query.web_session_id = 'some-session';
            await expect(middleware.requireAuth(req, res, next)).rejects.toThrow(AuthenticationError);
            await expect(middleware.requireAuth(req, res, next)).rejects.toThrow(AuthError.SESSION_ID_IN_QUERY_PARAM);
        });

        it('should throw AuthenticationError if session ID is in query params (camelCase)', async () => {
            req.query.webSessionId = 'some-session';
            await expect(middleware.requireAuth(req, res, next)).rejects.toThrow(AuthenticationError);
        });

        it('should extract session ID from cookie', async () => {
            req.cookies['web_session_id'] = 'cookie-session';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: SessionType.WEB,
                user_id: 'user-1'
            });

            await middleware.requireAuth(req, res, next);

            expect(webSessionService.validateSession).toHaveBeenCalledWith('cookie-session', expect.any(Object));
            expect(req.webSessionId).toBe('cookie-session');
            expect(next).toHaveBeenCalled();
        });

        it('should handle unexpected errors from webSessionService.validateSession', async () => {
            req.cookies['web_session_id'] = 'some-session';
            webSessionService.validateSession.mockRejectedValue(new Error('Internal DB error'));

            await expect(middleware.requireAuth(req, res, next)).rejects.toThrow(AuthenticationError);
            expect(res.clearCookie).toHaveBeenCalled();
        });

        it('should extract session ID from header', async () => {
            req.headers[WEB_SESSION_ID_HEADER] = 'header-session';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: SessionType.WEB,
                user_id: 'user-1'
            });

            await middleware.requireAuth(req, res, next);

            expect(req.webSessionId).toBe('header-session');
            expect(next).toHaveBeenCalled();
        });

        it('should extract session ID from Authorization header', async () => {
            req.headers.authorization = `${BEARER_PREFIX}auth-session`;
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: SessionType.WEB,
                user_id: 'user-1'
            });

            await middleware.requireAuth(req, res, next);

            expect(req.webSessionId).toBe('auth-session');
            expect(next).toHaveBeenCalled();
        });

        it('should throw AuthenticationError if no session ID is found', async () => {
            await expect(middleware.requireAuth(req, res, next)).rejects.toThrow(AuthenticationError);
            await expect(middleware.requireAuth(req, res, next)).rejects.toThrow(AuthError.REQUIRED);
        });

        it('should throw AuthenticationError if session is inactive', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ is_active: false });

            await expect(middleware.requireAuth(req, res, next)).rejects.toThrow(AuthenticationError);
            await expect(middleware.requireAuth(req, res, next)).rejects.toThrow(AuthError.INVALID_OR_EXPIRED_SESSION);
            expect(res.clearCookie).toHaveBeenCalled();
        });

        it('should throw AuthorizationError if session type is not WEB', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: 'OTHER'
            });

            await expect(middleware.requireAuth(req, res, next)).rejects.toThrow(AuthorizationError);
            await expect(middleware.requireAuth(req, res, next)).rejects.toThrow(AuthError.INVALID_SESSION_TYPE);
        });
    });

    describe('requireSuperAdmin', () => {
        it('should call next if user has SUPERADMIN role', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: SessionType.WEB,
                user_id: 'user-1',
                user_data: { roles: [UserRole.SUPERADMIN] }
            });

            await middleware.requireSuperAdmin(req, res, next);
            expect(next).toHaveBeenCalled();
        });

        it('should call next with AuthorizationError if user lacks superadmin role', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: SessionType.WEB,
                user_id: 'user-1',
                user_data: { roles: [UserRole.ADMIN] }
            });

            await middleware.requireSuperAdmin(req, res, next);
            expect(next).toHaveBeenCalledWith(expect.any(AuthorizationError));
            const error = next.mock.calls[0][0];
            expect(error.message).toBe(AuthError.SUPERADMIN_REQUIRED);
        });
    });

    describe('requireAdmin', () => {
        it('should call next if user has ADMIN role', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: SessionType.WEB,
                user_id: 'user-1',
                user_data: { roles: [UserRole.ADMIN] }
            });

            const adminMiddleware = middleware.requireAdmin;
            await adminMiddleware(req, res, next);
            expect(next).toHaveBeenCalled();
        });

        it('should call next if user has SUPERADMIN role (admin access)', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: SessionType.WEB,
                user_id: 'user-1',
                user_data: { roles: [UserRole.SUPERADMIN] }
            });

            const adminMiddleware = middleware.requireAdmin;
            await adminMiddleware(req, res, next);
            expect(next).toHaveBeenCalled();
        });

        it('should call next with AuthorizationError if user lacks admin role', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: SessionType.WEB,
                user_id: 'user-1',
                user_data: { roles: [UserRole.USER] }
            });

            const adminMiddleware = middleware.requireAdmin;
            await adminMiddleware(req, res, next);
            expect(next).toHaveBeenCalledWith(expect.any(AuthorizationError));
        });
    });

    describe('requirePageAuth', () => {
        it('should reject if session ID is in query params', async () => {
            req.query.web_session_id = 'some-session';
            const pageMiddleware = middleware.requirePageAuth();
            await pageMiddleware(req, res, next);
            expect(res.status).toHaveBeenCalledWith(404);
            expect(res.render).toHaveBeenCalledWith('404', expect.any(Object));
        });

        it('should redirect if no session ID in cookies', async () => {
            const pageMiddleware = middleware.requirePageAuth();
            await pageMiddleware(req, res, next);
            expect(res.redirect).toHaveBeenCalledWith('/');
        });

        it('should render 404 if no session and onFail is 404', async () => {
            const pageMiddleware = middleware.requirePageAuth({ onFail: '404' });
            await pageMiddleware(req, res, next);
            expect(res.status).toHaveBeenCalledWith(404);
            expect(res.render).toHaveBeenCalledWith('404', expect.any(Object));
        });

        it('should redirect if session is invalid', async () => {
            req.cookies['web_session_id'] = 'invalid-session';
            webSessionService.validateSession.mockResolvedValue(null);
            
            const pageMiddleware = middleware.requirePageAuth();
            await pageMiddleware(req, res, next);
            expect(res.redirect).toHaveBeenCalledWith('/');
            expect(res.clearCookie).toHaveBeenCalled();
        });

        it('should render 404 if session is invalid and onFail is 404', async () => {
            req.cookies['web_session_id'] = 'invalid-session';
            webSessionService.validateSession.mockResolvedValue(null);
            
            const pageMiddleware = middleware.requirePageAuth({ onFail: '404' });
            await pageMiddleware(req, res, next);
            expect(res.status).toHaveBeenCalledWith(404);
            expect(res.render).toHaveBeenCalledWith('404', expect.any(Object));
        });

        it('should redirect if session type is not WEB', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: 'OTHER'
            });
            
            const pageMiddleware = middleware.requirePageAuth();
            await pageMiddleware(req, res, next);
            expect(res.redirect).toHaveBeenCalledWith('/');
            expect(res.clearCookie).toHaveBeenCalled();
        });

        it('should call next if session is valid and type is WEB', async () => {
            req.cookies['web_session_id'] = 'valid-session';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: SessionType.WEB,
                user_id: 'user-1'
            });
            
            const pageMiddleware = middleware.requirePageAuth();
            await pageMiddleware(req, res, next);
            expect(next).toHaveBeenCalled();
            expect(req.userId).toBe('user-1');
        });
    });

    describe('requirePageAdmin', () => {
        it('should reject if session ID is in query params', async () => {
            req.query.web_session_id = 'some-session';
            const adminMiddleware = middleware.requirePageAdmin();
            await adminMiddleware(req, res, next);
            expect(res.status).toHaveBeenCalledWith(404);
            expect(res.render).toHaveBeenCalledWith('404', expect.any(Object));
        });

        it('should redirect if no session ID in cookies', async () => {
            const adminMiddleware = middleware.requirePageAdmin();
            await adminMiddleware(req, res, next);
            expect(res.redirect).toHaveBeenCalledWith('/');
        });

        it('should redirect if session is inactive', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ is_active: false });
            
            const adminMiddleware = middleware.requirePageAdmin();
            await adminMiddleware(req, res, next);
            expect(res.redirect).toHaveBeenCalledWith('/');
            expect(res.clearCookie).toHaveBeenCalled();
        });

        it('should redirect if session type is not WEB', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: 'OTHER'
            });
            
            const adminMiddleware = middleware.requirePageAdmin();
            await adminMiddleware(req, res, next);
            expect(res.redirect).toHaveBeenCalledWith('/');
            expect(res.clearCookie).toHaveBeenCalled();
        });

        it('should redirect if user lacks admin role', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: SessionType.WEB,
                user_id: 'user-1',
                user_data: { roles: [UserRole.USER] }
            });
            
            const adminMiddleware = middleware.requirePageAdmin({ redirectTo: '/denied' });
            await adminMiddleware(req, res, next);
            expect(res.redirect).toHaveBeenCalledWith('/denied');
        });

        it('should call next if user is SUPERADMIN', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: SessionType.WEB,
                user_id: 'user-1',
                user_data: { roles: [UserRole.SUPERADMIN] }
            });
            
            const adminMiddleware = middleware.requirePageAdmin();
            await adminMiddleware(req, res, next);
            expect(next).toHaveBeenCalled();
        });
    });

    describe('requireFirstRun', () => {
        it('should call next when platform_settings is null (fresh platform)', async () => {
            settingsService.getPlatformSettings.mockResolvedValue(null);
            await middleware.requireFirstRun(req, res, next);
            expect(next).toHaveBeenCalledWith();
        });

        it('should call next when setup_complete is false', async () => {
            settingsService.getPlatformSettings.mockResolvedValue({ setup_complete: false });
            await middleware.requireFirstRun(req, res, next);
            expect(next).toHaveBeenCalledWith();
        });

        it('should call next when setup_complete is undefined', async () => {
            settingsService.getPlatformSettings.mockResolvedValue({});
            await middleware.requireFirstRun(req, res, next);
            expect(next).toHaveBeenCalledWith();
        });

        it('should call next("route") when setup_complete is true', async () => {
            settingsService.getPlatformSettings.mockResolvedValue({ setup_complete: true });
            await middleware.requireFirstRun(req, res, next);
            expect(next).toHaveBeenCalledWith('route');
        });

        it('should call next with InternalServerError if settingsService fails', async () => {
            settingsService.getPlatformSettings.mockRejectedValue(new Error('DB Fail'));
            await middleware.requireFirstRun(req, res, next);
            expect(next).toHaveBeenCalledWith(expect.any(Object));
            const error = next.mock.calls[0][0];
            expect(error.message).toBe('Internal server error');
        });
    });

    describe('optionalAuth', () => {
        it('should call next if no session cookie', async () => {
            await middleware.optionalAuth(req, res, next);
            expect(next).toHaveBeenCalled();
            expect(req.session).toBeUndefined();
        });

        it('should attach session if valid cookie present', async () => {
            req.cookies['web_session_id'] = 'valid-session';
            const session = { 
                is_active: true, 
                session_type: SessionType.WEB,
                user_id: 'user-1'
            };
            webSessionService.validateSession.mockResolvedValue(session);

            await middleware.optionalAuth(req, res, next);

            expect(req.session).toBe(session);
            expect(req.userId).toBe('user-1');
            expect(next).toHaveBeenCalled();
        });

        it('should not attach session if session type is not WEB', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockResolvedValue({ 
                is_active: true, 
                session_type: 'OTHER'
            });

            await middleware.optionalAuth(req, res, next);
            expect(req.session).toBeUndefined();
            expect(next).toHaveBeenCalled();
        });

        it('should continue if validateSession throws', async () => {
            req.cookies['web_session_id'] = 'session-id';
            webSessionService.validateSession.mockRejectedValue(new Error('Fail'));

            await middleware.optionalAuth(req, res, next);
            expect(next).toHaveBeenCalled();
        });
    });

    describe('requireOperatorBinding', () => {
        beforeEach(() => {
            req.webSessionId = 'session-123';
            req.userId = 'user-123';
        });

        it('should resolve bound operators via webSessionId when web session is present', async () => {
            const boundOperators = [{ operator_id: 'op-1' }];
            bindingService.resolveBoundOperators.mockResolvedValue(boundOperators);

            await middleware.requireOperatorBinding(req, res, next);

            expect(bindingService.resolveBoundOperators).toHaveBeenCalledWith('session-123');
            expect(req.boundOperators).toBe(boundOperators);
            expect(next).toHaveBeenCalled();
        });

        it('should resolve bound operators via userId when OAuth Client ID auth (no web session)', async () => {
            req.webSessionId = null;
            const boundOperators = [{ operator_id: 'op-1' }];
            bindingService.resolveBoundOperatorsForUser.mockResolvedValue(boundOperators);

            await middleware.requireOperatorBinding(req, res, next);

            expect(bindingService.resolveBoundOperatorsForUser).toHaveBeenCalledWith('user-123');
            expect(req.boundOperators).toBe(boundOperators);
            expect(next).toHaveBeenCalled();
        });

        it('should throw AuthorizationError when no auth context (no webSessionId or userId)', async () => {
            req.webSessionId = null;
            req.userId = null;

            await middleware.requireOperatorBinding(req, res, next);

            expect(next).toHaveBeenCalledWith(expect.any(AuthorizationError));
            const error = next.mock.calls[0][0];
            expect(error.message).toBe(AuthError.ACCESS_DENIED);
        });

        it('should allow empty bound_operators array when requireOperatorBinding completes', async () => {
            bindingService.resolveBoundOperators.mockResolvedValue([]);

            await middleware.requireOperatorBinding(req, res, next);

            expect(req.boundOperators).toEqual([]);
            expect(next).toHaveBeenCalled();
        });

        it('should forward errors from bindingService', async () => {
            bindingService.resolveBoundOperators.mockRejectedValue(new Error('DB error'));

            await middleware.requireOperatorBinding(req, res, next);

            expect(next).toHaveBeenCalledWith(expect.any(Error));
        });
    });

    describe('requireAtLeastOneOperator', () => {
        it('should call next when operators are bound', async () => {
            const boundOperators = [{ operator_id: 'op-1' }];
            req.boundOperators = boundOperators;

            await middleware.requireAtLeastOneOperator(req, res, next);

            expect(next).toHaveBeenCalled();
            // Should not pass error
            expect(next.mock.calls[0][0]).toBeUndefined();
        });

        it('should throw AuthorizationError when no operators are bound', async () => {
            req.boundOperators = [];

            await middleware.requireAtLeastOneOperator(req, res, next);

            expect(next).toHaveBeenCalledWith(expect.any(AuthorizationError));
            const error = next.mock.calls[0][0];
            expect(error.message).toBe(AuthError.NO_OPERATOR_BOUND);
        });

        it('should throw AuthorizationError when boundOperators is undefined', async () => {
            req.boundOperators = undefined;

            await middleware.requireAtLeastOneOperator(req, res, next);

            expect(next).toHaveBeenCalledWith(expect.any(AuthorizationError));
        });
    });
});
