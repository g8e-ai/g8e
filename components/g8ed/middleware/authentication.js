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
import { G8eHttpContext } from '../models/request_models.js';

export function createAuthMiddleware({ webSessionService, setupService, userService, settingsService, bindingService, operatorService, deviceLinkService }) {
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
        // 2. x-session-id header (testing/internal - only enabled in non-production)
        else if (process.env.NODE_ENV !== 'production' && req.headers[WEB_SESSION_ID_HEADER]) {
            webSessionId = req.headers[WEB_SESSION_ID_HEADER];
        }
        // 3. Authorization Bearer token (testing/internal - only enabled in non-production)
        else if (process.env.NODE_ENV !== 'production' && req.headers.authorization?.startsWith(BEARER_PREFIX)) {
            webSessionId = req.headers.authorization.substring(BEARER_PREFIX.length);
        }

        // Check for Operator Session ID (evals/cli use this)
        const operatorSessionId = req.headers['x-g8e-operator-session-id'] || req.headers['X-G8E-Operator-Session-ID'];
        if (operatorSessionId && operatorService) {
            try {
                // Validate the operator session against g8ee (authority)
                // Use a minimal g8eContext for validation
                const g8eContext = G8eHttpContext.parse({ source_component: 'g8ed' });
                const result = await operatorService.relayValidateOperatorSessionToG8ee(operatorSessionId, g8eContext);
                
                if (result && result.valid) {
                    req.operatorSessionId = operatorSessionId;
                    req.operatorId = result.operator_id;
                    req.userId = result.user_id;
                    // For operator sessions, we don't have a full WebSession object, 
                    // but we can mock enough for downstream usage if needed.
                    req.session = {
                        user_id: result.user_id,
                        is_active: true,
                        session_type: 'OPERATOR'
                    };
                    return next();
                }
            } catch (error) {
                logger.warn('[AUTH-MIDDLEWARE] Operator session validation failed', {
                    operatorSessionId: redactWebSessionId(operatorSessionId),
                    error: error.message
                });
            }
        }

        // Check for Device Link Token (evals use this)
        const deviceToken = req.headers['x-g8e-device-token'] || req.headers['X-G8E-Device-Token'];
        if (deviceToken && deviceLinkService) {
            try {
                const linkResult = await deviceLinkService.getLink(deviceToken);
                if (linkResult.success) {
                    const linkData = linkResult.data;
                    req.userId = linkData.user_id;
                    req.organizationId = linkData.organization_id;
                    req.deviceToken = deviceToken;
                    // For device link auth, we mock a session
                    req.session = {
                        user_id: linkData.user_id,
                        organization_id: linkData.organization_id,
                        is_active: true,
                        session_type: 'DEVICE_LINK'
                    };
                    return next();
                }
            } catch (error) {
                logger.warn('[AUTH-MIDDLEWARE] Device link validation failed', {
                    deviceToken: redactWebSessionId(deviceToken),
                    error: error.message
                });
            }
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
            // Allow OPERATOR or DEVICE_LINK sessions if they were already handled above
            if (req.operatorSessionId || req.deviceToken) {
                return next();
            }
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
        let webSessionId = null;

        // 1. Secure cookie (production)
        if (req.cookies?.['web_session_id']) {
            webSessionId = req.cookies['web_session_id'];
        }
        // 2. x-session-id header (testing/internal - only enabled in non-production)
        else if (process.env.NODE_ENV !== 'production' && req.headers[WEB_SESSION_ID_HEADER]) {
            webSessionId = req.headers[WEB_SESSION_ID_HEADER];
        }
        // 3. Authorization Bearer token (testing/internal - only enabled in non-production)
        else if (process.env.NODE_ENV !== 'production' && req.headers.authorization?.startsWith(BEARER_PREFIX)) {
            webSessionId = req.headers.authorization.substring(BEARER_PREFIX.length);
        }

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
            if (req.webSessionId && typeof req.webSessionId === 'string' && req.webSessionId.length > 0) {
                // Web session auth
                boundOperators = await bindingService.resolveBoundOperators(req.webSessionId);
            } else if (req.operatorSessionId && req.operatorId) {
                // Use the explicit operator session from auth (evals/cli flow)
                boundOperators = [{
                    operator_id: req.operatorId,
                    operator_session_id: req.operatorSessionId
                }];
            } else if (req.deviceToken && deviceLinkService && operatorService) {
                // Use operators associated with the device link (evals flow)
                const linkResult = await deviceLinkService.getLink(req.deviceToken);
                if (linkResult.success && linkResult.data.claims) {
                    boundOperators = await Promise.all(linkResult.data.claims.map(async claim => {
                        const operator = await operatorService.getOperator(claim.operator_id);
                        return {
                            operator_id: claim.operator_id,
                            operator_session_id: operator?.operator_session_id
                        };
                    }));
                    // Filter out any where we couldn't find an operator session
                    boundOperators = boundOperators.filter(o => o.operator_session_id);
                }
            } else if (req.userId && typeof req.userId === 'string' && req.userId.length > 0) {
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
