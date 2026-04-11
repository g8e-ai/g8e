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

import express from 'express';
import { logger } from '../../utils/logger.js';

import { 
    ValidationError, 
    ResourceNotFoundError, 
    InternalServerError,
    BusinessLogicError
} from '../../services/error_service.js';
import { SimpleSuccessResponse, WebSessionResponse, LockedAccountsResponse, AccountStatusResponse, UserRegisterResponse } from '../../models/response_models.js';
import { ApiKeyError } from '../../constants/auth.js';
import { redactWebSessionId } from '../../utils/security.js';
import { SESSION_COOKIE_NAME, COOKIE_SAME_SITE } from '../../constants/session.js';
import { AuthPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createAuthRouter({ services, authMiddleware, rateLimiters }) {
    const { webSessionService, userService, loginSecurityService, setupService, passkeyAuthService } = services;
    const { requireAuth, requireAdmin } = authMiddleware;
    const { authRateLimiter } = rateLimiters;
    const router = express.Router();

    router.get(AuthPaths.WEB_SESSION, requireAuth, (req, res) => {
        res.json(new WebSessionResponse({
            success: true,
            authenticated: true,
            session: req.session,
        }).forClient());
    });

    router.post(AuthPaths.LOGOUT, requireAuth, async (req, res, next) => {
        try {
            const web_session_id = req.cookies[SESSION_COOKIE_NAME];

            if (web_session_id) {
                logger.info('[AUTH] Processing logout', {
                    webSessionId: redactWebSessionId(web_session_id)
                });

                await webSessionService.endSession(web_session_id);
            }

            res.clearCookie(SESSION_COOKIE_NAME, {
                httpOnly: true,
                secure: true,
                sameSite: COOKIE_SAME_SITE,
                path: '/'
            });

            logger.info('[AUTH] Logout successful');
            res.json(new SimpleSuccessResponse({ success: true, message: 'Logged out successfully' }).forClient());

        } catch (error) {
            logger.error('[AUTH] Error during logout', { error: error.message });
            next(new InternalServerError(ApiKeyError.INTERNAL_ERROR, { cause: error }));
        }
    });

    router.post(AuthPaths.REGISTER, authRateLimiter, async (req, res, next) => {
        try {
            const isFirstRun = await setupService.isFirstRun();
            let { email, name, settings: userSettings } = req.body;

            if (!email) {
                throw new ValidationError('email is required');
            }

            // Sanitize email: trim and lowercase
            email = email.trim().toLowerCase();

            if (!email.includes('@')) {
                throw new ValidationError('Invalid email format');
            }

            // Sanitize name: trim and remove potential script tags (basic XSS protection for names)
            if (name) {
                name = name.trim().replace(/<script\b[^>]*>([\s\S]*?)<\/script>/gim, "");
            } else {
                name = email.split('@')[0];
            }

            // --- FIRST RUN SETUP LOGIC ---
            if (isFirstRun) {
                logger.info('[AUTH-SETUP] Processing first-run administrative setup');
                const user = await setupService.performFirstRunSetup({ email, name, userSettings, req });
                logger.info('[AUTH-SETUP] Initial superadmin created', { userId: user.id, email: user.email });

                // 4. Generate passkey challenge
                const options = await passkeyAuthService.generateRegistrationChallenge(req, user);

                return res.status(201).json(new UserRegisterResponse({
                    message: 'Administrative setup initialized',
                    user_id: user.id,
                    challenge_options: options
                }).forClient());
            }

            // --- STANDARD REGISTRATION LOGIC ---
            const existing = await userService.findUserByEmail(email);
            if (existing) {
                throw new BusinessLogicError('An account with that email already exists', {
                    code: 'USER_ALREADY_EXISTS',
                    category: 'conflict'
                });
            }

            const user = await userService.createUser({ email, name });
            logger.info('[AUTH] User registered', { userId: user.id, email: user.email });

            // Atomic: Generate passkey challenge immediately for the new user
            const options = await passkeyAuthService.generateRegistrationChallenge(req, user);

            return res.status(201).json(new UserRegisterResponse({ 
                message: 'User registered', 
                user_id: user.id,
                challenge_options: options
            }).forClient());
        } catch (error) {
            logger.error('[AUTH] Register error', { error: error.message });
            return next(error);
        }
    });

    router.get(AuthPaths.ADMIN_LOCKED_ACCOUNTS, requireAdmin, async (req, res, next) => {
        try {
            const lockedAccounts = await loginSecurityService.getLockedAccounts();
            loginSecurityService.auditAdminAccess({ action: 'view_locked_accounts', userId: req.userId, userEmail: req.session?.user_data?.email, ip: req.ip, userAgent: req.headers['user-agent'], path: req.path, method: req.method, queryParams: Object.keys(req.query).length > 0 ? req.query : null, metadata: { count: lockedAccounts.length } });
            logger.info('[AUTH-ADMIN] Locked accounts retrieved', {
                adminUserId: req.session.user_id,
                count: lockedAccounts.length
            });
            res.json(new LockedAccountsResponse({ success: true, locked_accounts: lockedAccounts, count: lockedAccounts.length }).forClient());
        } catch (error) {
            logger.error('[AUTH-ADMIN] Error retrieving locked accounts', { error: error.message });
            next(new InternalServerError('Failed to retrieve locked accounts', { cause: error }));
        }
    });

    router.post(AuthPaths.ADMIN_UNLOCK_ACCOUNT, requireAdmin, async (req, res, next) => {
        try {
            const { identifier } = req.body;
            if (!identifier) {
                throw new ValidationError('identifier is required');
            }
            const result = await loginSecurityService.unlockAccount(identifier, req.userId);
            if (!result.success) {
                throw new ResourceNotFoundError(result.error || 'Account not found');
            }
            loginSecurityService.auditAdminAccess({ action: 'unlock_account', userId: req.userId, userEmail: req.session?.user_data?.email, ip: req.ip, userAgent: req.headers['user-agent'], path: req.path, method: req.method, queryParams: null, metadata: { identifier } });
            logger.info('[AUTH-ADMIN] Account unlocked', { adminUserId: req.session.user_id, identifier });
            res.json(new SimpleSuccessResponse({ success: true, message: 'Account unlocked successfully' }).forClient());
        } catch (error) {
            logger.error('[AUTH-ADMIN] Error on unlock-account', { error: error.message });
            return next(error);
        }
    });

    router.get(AuthPaths.ADMIN_ACCOUNT_STATUS, requireAdmin, async (req, res, next) => {
        try {
            const { userId } = req.params;
            const user = await userService.getUser(userId);
            if (!user) {
                throw new ResourceNotFoundError('User not found');
            }
            const identifier = user.email;
            const lockStatus = await loginSecurityService.isAccountLocked(identifier);
            const attemptStatus = await loginSecurityService.getFailedAttemptStatus(identifier);
            loginSecurityService.auditAdminAccess({ action: 'check_account_status', userId: req.userId, userEmail: req.session?.user_data?.email, ip: req.ip, userAgent: req.headers['user-agent'], path: req.path, method: req.method, queryParams: null, metadata: { locked: lockStatus.locked } });
            res.json(new AccountStatusResponse({
                success: true,
                locked: lockStatus.locked,
                locked_at: lockStatus.locked_at || null,
                failed_attempts: lockStatus.failed_attempts || attemptStatus.attempts,
                requires_captcha: attemptStatus.requires_captcha
            }).forClient());
        } catch (error) {
            logger.error('[AUTH-ADMIN] Error checking account status', { error: error.message });
            return next(error);
        }
    });

    return router;
}
