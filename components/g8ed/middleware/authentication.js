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
 * Authentication Middleware
 * 
 * Centralized authentication and authorization middleware for g8ed.
 * Provides session validation, user authentication, and role-based access control.
 */

import { logger } from '../utils/logger.js';
import { AuthenticationError, AuthorizationError, InternalServerError } from '../services/error_service.js';
import { ErrorCode } from '../constants/errors.js';
import { UserRole, WEB_SESSION_ID_HEADER, BEARER_PREFIX, AuthError } from '../constants/auth.js';
import { SessionType } from '../constants/session.js';
import { clearSessionCookies } from '../utils/security.js';

export function createAuthMiddleware({ webSessionService, setupService, userService, settingsService, bindingService }) {
    /**
     * Require authenticated session
     */
    const requireAuth = async (req, res, next) => {
        // SECURITY: Hard reject if WEB session ID is sent in URL query params
        if (req.query?.web_session_id || req.query?.webSessionId) {
            logger.warn('[AUTH-MIDDLEWARE] SECURITY: Rejected request with web session ID in URL', {
                ip: req.ip,
                path: req.path,
                userAgent: req.headers['user-agent']
            });
            throw new AuthenticationError(AuthError.SESSION_ID_IN_QUERY_PARAM, {
                code: ErrorCode.AUTH_ERROR,
                details: { reason: 'session_id_in_query' }
            });
        }

        // Check multiple sources for session ID (in order of preference)
        let webSessionId = null;
        
        // 1. Secure cookie (production)
        if (req.cookies?.['web_session_id']) {
            webSessionId = req.cookies['web_session_id'];
        }
        // 2. x-session-id header (testing/internal)
        else if (req.headers[WEB_SESSION_ID_HEADER]) {
            webSessionId = req.headers[WEB_SESSION_ID_HEADER];
        }
        // 3. Authorization Bearer token
        else if (req.headers.authorization?.startsWith(BEARER_PREFIX)) {
            webSessionId = req.headers.authorization.substring(BEARER_PREFIX.length);
        }

        if (!webSessionId) {
            throw new AuthenticationError(AuthError.REQUIRED);
        }
        
        // Build request context for session binding validation
        const requestContext = {
            ip: req.ip || req.headers['x-forwarded-for'] || req.socket.remoteAddress,
            userAgent: req.headers['user-agent']
        };

        let session;
        try {
            session = await webSessionService.validateSession(webSessionId, requestContext);
        } catch (error) {
            logger.error('[AUTH-MIDDLEWARE] validateSession threw unexpectedly', {
                error: error.message,
                path: req.path,
                ip: req.ip
            });
            clearSessionCookies(res, req);
            throw new AuthenticationError(AuthError.INVALID_OR_EXPIRED_SESSION, { cause: error });
        }

        if (!session || !session.is_active) {
            // Clear ALL session cookie variants (host-only and domain-scoped)
            clearSessionCookies(res, req);
            throw new AuthenticationError(AuthError.INVALID_OR_EXPIRED_SESSION);
        }

        // SECURITY: Only allow fully authenticated WEB sessions
        if (session.session_type !== SessionType.WEB) {
            logger.warn('[AUTH-MIDDLEWARE] Invalid session type', {
                sessionType: session.session_type,
                path: req.path,
                ip: req.ip
            });
            throw new AuthorizationError(AuthError.INVALID_SESSION_TYPE, {
                code: 'INVALID_SESSION_TYPE'
            });
        }

        // Attach session to request
        req.session = session;
        req.webSessionId = webSessionId;
        req.userId = session.user_id;

        return next();
    };

    /**
     * Require admin role
     */
    const requireAdmin = async (req, res, next) => {
        try {
            await requireAuth(req, res, async () => {
                const roles = req.session.user_data?.roles;
                if (!Array.isArray(roles) || (!roles.includes(UserRole.ADMIN) && !roles.includes(UserRole.SUPERADMIN))) {
                    throw new AuthorizationError(AuthError.ACCESS_DENIED);
                }
                next();
            });
        } catch (error) {
            next(error);
        }
    };

    /**
     * Require superadmin role
     * More restrictive than requireAdmin - only superadmin role allowed
     */
    const requireSuperAdmin = async (req, res, next) => {
        try {
            await requireAuth(req, res, async () => {
                const roles = req.session.user_data?.roles;
                if (!Array.isArray(roles) || !roles.includes(UserRole.SUPERADMIN)) {
                    logger.warn('[AUTH-MIDDLEWARE] Superadmin access denied', {
                        userId: req.userId,
                        roles,
                        path: req.path
                    });
                    throw new AuthorizationError(AuthError.SUPERADMIN_REQUIRED);
                }
                next();
            });
        } catch (error) {
            next(error);
        }
    };

    /**
     * Require authenticated session for web pages (HTML responses)
     * Unlike requireAuth which returns JSON, this middleware:
     * - Redirects to landing page when not authenticated
     * - Renders 404 for security-sensitive pages (hides existence)
     * - Properly validates session type
     * 
     * @param {Object} options - Configuration options
     * @param {string} options.onFail - 'redirect' (default) or '404'
     * @param {string} options.redirectTo - URL to redirect to (default: '/')
     */
    const requirePageAuth = (options = {}) => {
        const { onFail = 'redirect', redirectTo = '/' } = options;
        
        return async (req, res, next) => {
            // SECURITY: Hard reject if WEB session ID is sent in URL query params
            if (req.query?.web_session_id || req.query?.webSessionId) {
                logger.warn('[PAGE-AUTH] SECURITY: Rejected request with web session ID in URL', {
                    ip: req.ip,
                    path: req.path,
                    userAgent: req.headers['user-agent']
                });
                return res.status(404).render('404', {
                    message: 'Page not found'
                });
            }

            const webSessionId = req.cookies?.['web_session_id'];
            
            if (!webSessionId) {
                if (onFail === '404') {
                    return res.status(404).render('404', {
                        message: 'Page not found'
                    });
                }
                return res.redirect(redirectTo);
            }
            
            // Build request context for session binding validation
            const requestContext = {
                ip: req.ip || req.headers['x-forwarded-for'] || req.socket.remoteAddress,
                userAgent: req.headers['user-agent']
            };

            const session = await webSessionService.validateSession(webSessionId, requestContext);

            if (!session || !session.is_active) {
                clearSessionCookies(res, req);
                if (onFail === '404') {
                    return res.status(404).render('404', {
                        message: 'Page not found'
                    });
                }
                return res.redirect(redirectTo);
            }

            // SECURITY: Only allow fully authenticated WEB sessions
            if (session.session_type !== SessionType.WEB) {
                logger.warn('[PAGE-AUTH] Invalid session type for page access', {
                    sessionType: session.session_type,
                    path: req.path,
                    ip: req.ip
                });
                clearSessionCookies(res, req);
                if (onFail === '404') {
                    return res.status(404).render('404', {
                        message: 'Page not found'
                    });
                }
                return res.redirect(redirectTo);
            }
            
            // Attach session to request
            req.session = session;
            req.webSessionId = webSessionId;
            req.userId = session.user_id;

            return next();
        };
    };

    /**
     * Require admin role for web pages (HTML responses)
     * Combines authentication check with admin role verification
     * 
     * @param {Object} options - Configuration options
     * @param {string} options.redirectTo - URL to redirect non-admins to (default: '/chat')
     */
    const requirePageAdmin = (options = {}) => {
        const { redirectTo = '/chat' } = options;
        
        return async (req, res, next) => {
            // SECURITY: Hard reject if WEB session ID is sent in URL query params
            if (req.query?.web_session_id || req.query?.webSessionId) {
                logger.warn('[PAGE-AUTH] SECURITY: Rejected admin request with web session ID in URL', {
                    ip: req.ip,
                    path: req.path
                });
                return res.status(404).render('404', {
                    message: 'Page not found'
                });
            }

            const webSessionId = req.cookies?.['web_session_id'];
            
            if (!webSessionId) {
                return res.redirect('/');
            }
            
            // Build request context for session binding validation
            const requestContext = {
                ip: req.ip || req.headers['x-forwarded-for'] || req.socket.remoteAddress,
                userAgent: req.headers['user-agent']
            };

            const session = await webSessionService.validateSession(webSessionId, requestContext);

            if (!session || !session.is_active) {
                clearSessionCookies(res, req);
                return res.redirect('/');
            }

            // SECURITY: Only allow fully authenticated WEB sessions
            if (session.session_type !== SessionType.WEB) {
                logger.warn('[PAGE-AUTH] Invalid session type for admin page access', {
                    sessionType: session.session_type,
                    path: req.path,
                    ip: req.ip
                });
                clearSessionCookies(res, req);
                return res.redirect('/');
            }
            
            // Check for admin role (admin or superadmin)
            const roles = session.user_data?.roles;
            if (!Array.isArray(roles) || (!roles.includes(UserRole.ADMIN) && !roles.includes(UserRole.SUPERADMIN))) {
                logger.warn('[PAGE-AUTH] Admin page access denied - admin role required', {
                    userId: session.user_id,
                    roles,
                    path: req.path
                });
                return res.redirect(redirectTo);
            }

            // Attach session to request
            req.session = session;
            req.webSessionId = webSessionId;
            req.userId = session.user_id;

            return next();
        };
    };

    /**
     * Require first-run setup mode.
     * Calls next('route') when setup is already complete, allowing Express to fall
     * through to the next route handler on the same path (the requireAuth handler).
     * Used on dual-handler registration routes where the setup-flow handler is
     * registered first and the add-passkey handler is registered second.
     */
    const requireFirstRun = async (req, res, next) => {
        try {
            const platform_settings = await settingsService.getPlatformSettings();
            if (platform_settings?.setup_complete === true) {
                return next('route');
            }
            next();
        } catch (error) {
            logger.error('[AUTH-MIDDLEWARE] requireFirstRun check failed', { error: error.message });
            next(new InternalServerError('Internal server error', { cause: error }));
        }
    };

    /**
     * Optional authentication - attaches session if present but doesn't require it
     * SECURITY: Only attaches fully authenticated WEB sessions
     */
    const optionalAuth = async (req, res, next) => {
        const webSessionId = req.cookies?.['web_session_id'];

        if (webSessionId) {
            try {
                // Build request context for session binding validation
                const requestContext = {
                    ip: req.ip || req.headers['x-forwarded-for'] || req.socket.remoteAddress,
                    userAgent: req.headers['user-agent']
                };
                
                const session = await webSessionService.validateSession(webSessionId, requestContext);
                // SECURITY: Only attach fully authenticated WEB sessions
                if (session && session.is_active && session.session_type === SessionType.WEB) {
                    req.session = session;
                    req.webSessionId = webSessionId;
                    req.userId = session.user_id;
                }
            } catch (error) {
                logger.warn('[AUTH-MIDDLEWARE] Optional auth failed', {
                    error: error.message
                });
            }
        }

        next();
    };

    /**
     * Require operator binding - resolves bound operators after authentication
     * 
     * This middleware must run AFTER requireAuth or OAuth Client ID authentication.
     * It resolves bound operators based on the authentication method:
     * - Web session auth: uses req.webSessionId
     * - OAuth Client ID auth: uses req.userId (no web session)
     */
    const requireOperatorBinding = async (req, res, next) => {
        try {
            let boundOperators = [];
            
            // Resolve bound operators based on authentication method
            if (req.webSessionId) {
                // Web session auth
                boundOperators = await bindingService.resolveBoundOperators(req.webSessionId);
            } else if (req.userId) {
                // OAuth Client ID auth (no web session)
                boundOperators = await bindingService.resolveBoundOperatorsForUser(req.userId);
            } else {
                // No authentication context - this shouldn't happen if middleware is ordered correctly
                logger.error('[AUTH-MIDDLEWARE] requireOperatorBinding called without auth context', {
                    path: req.path
                });
                throw new AuthorizationError(AuthError.ACCESS_DENIED, {
                    code: 'NO_AUTH_CONTEXT'
                });
            }
            
            // Attach to request
            req.boundOperators = boundOperators;
            
            next();
        } catch (error) {
            next(error);
        }
    };

    /**
     * Require at least one bound operator - returns 400 if no operators are bound
     * 
     * This middleware must run AFTER requireOperatorBinding.
     */
    const requireAtLeastOneOperator = async (req, res, next) => {
        if (!req.boundOperators || req.boundOperators.length === 0) {
            logger.warn('[AUTH-MIDDLEWARE] No operator session found for user', {
                userId: req.userId,
                webSessionId: req.webSessionId,
                path: req.path
            });
            return next(new AuthorizationError(AuthError.NO_OPERATOR_BOUND, {
                code: 'NO_OPERATOR_BOUND'
            }));
        }
        next();
    };

    return {
        requireAuth,
        requireAdmin,
        requireSuperAdmin,
        requirePageAuth,
        requirePageAdmin,
        requireFirstRun,
        optionalAuth,
        requireOperatorBinding,
        requireAtLeastOneOperator
    };
}
