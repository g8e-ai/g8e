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
import { PasskeyPaths } from '../../constants/api_paths.js';
import { PasskeyAuthChallengeRequest, PasskeyAuthVerifyRequest, AttestationResponseJSON, PasskeyRegisterVerifyRequest, PasskeyRegisterChallengeRequest } from '../../models/request_models.js';
import { 
    AuthenticationError, 
    AuthorizationError, 
    ValidationError, 
    ResourceNotFoundError, 
    InternalServerError,
    BusinessLogicError
} from '../../services/error_service.js';
import { PasskeyRegisterChallengeResponse, PasskeyAuthChallengeResponse, PasskeyVerifyResponse, SimpleSuccessResponse } from '../../models/response_models.js';
import { ApiKeyError } from '../../constants/auth.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createPasskeyRouter({ services, authMiddleware, rateLimiters }) {
    const { passkeyAuthService, userService, postLoginService, setupService } = services;
    const { requireAuth, requireFirstRun } = authMiddleware;
    const { passkeyRateLimiter } = rateLimiters;
    const router = express.Router();

    // ---------------------------------------------------------------------------
    // POST /register-challenge
    //   Body: { user_id }
    //   → generates challenge against existing user record
    //   → returns { success, options }
    //   SECURITY: Requires authenticated session. Setup flow gets challenge atomically via /api/setup/user.
    // ---------------------------------------------------------------------------

    router.post(PasskeyPaths.REGISTER_CHALLENGE, passkeyRateLimiter, requireAuth, async (req, res, next) => {
        try {
            const challengeReq = PasskeyRegisterChallengeRequest.parse(req.body);

            // SECURITY: Only allow generating challenge for oneself
            if (challengeReq.user_id !== req.userId) {
                throw new AuthorizationError('Access denied');
            }

            const user = await userService.getUser(challengeReq.user_id);
            if (!user) {
                throw new ResourceNotFoundError('User not found');
            }

            const options = await passkeyAuthService.generateRegistrationChallenge(req, user);

            logger.info('[PASSKEY-ROUTES] Registration challenge issued', { userId: user.id });
            return res.json(new PasskeyRegisterChallengeResponse({ message: 'Challenge issued', options }).forClient());
        } catch (error) {
            logger.error('[PASSKEY-ROUTES] Register challenge failed', { error: error.message });
            return next(error);
        }
    });

    // ---------------------------------------------------------------------------
    // POST /register-verify
    //   Body: { user_id, attestation_response }
    //   → verifies WebAuthn against existing user, appends credential
    //   → returns { success, session? }
    // ---------------------------------------------------------------------------

    // Setup flow: Only allowed if first-run setup is active
    router.post(PasskeyPaths.REGISTER_VERIFY, passkeyRateLimiter, requireFirstRun, async (req, res, next) => {
        try {
            const verifyReq = PasskeyRegisterVerifyRequest.parse(req.body);
            const user = await userService.getUser(verifyReq.user_id);
            if (!user) {
                throw new ResourceNotFoundError('User not found');
            }

            // SECURITY: Setup flow only allows registration for the first user if they have no passkeys yet
            if (user.passkey_credentials.length > 0) {
                throw new AuthorizationError('Setup already complete');
            }

            const result = await passkeyAuthService.verifyRegistration(req, user, verifyReq.attestation_response);

            if (!result.verified) {
                logger.warn('[PASSKEY-ROUTES] Setup registration verification failed', { userId: user.id, error: result.error });
                throw new ValidationError(result.error);
            }

            // Issue session and mark setup complete
            const session = await postLoginService.createSessionAndSetCookie(req, res, user);
            await postLoginService.onSuccessfulRegistration(user, session);
            await setupService.completeSetup();

            logger.info('[PASSKEY-ROUTES] Setup complete: first passkey registered, session created', { userId: user.id });

            return res.json(new PasskeyVerifyResponse({ 
                success: true,
                message: 'Setup complete',
                session 
            }).forClient());
        } catch (error) {
            logger.error('[PASSKEY-ROUTES] Setup verify failed', { error: error.message });
            return next(error);
        }
    });

    // Post-setup flow: First passkey registration for a new user (no session yet)
    router.post(PasskeyPaths.REGISTER_VERIFY, passkeyRateLimiter, async (req, res, next) => {
        try {
            const verifyReq = PasskeyRegisterVerifyRequest.parse(req.body);
            const user = await userService.getUser(verifyReq.user_id);
            if (!user) {
                throw new ResourceNotFoundError('User not found');
            }

            // SECURITY: Only allow this non-session path if the user has NO passkeys yet.
            // This prevents an attacker from hijacking an account that already has a passkey.
            if (user.passkey_credentials.length > 0) {
                return next(); // Fall through to requireAuth handler
            }

            const result = await passkeyAuthService.verifyRegistration(req, user, verifyReq.attestation_response);

            if (!result.verified) {
                logger.warn('[PASSKEY-ROUTES] Initial registration verification failed', { userId: user.id, error: result.error });
                throw new ValidationError(result.error);
            }

            // Registration successful - create session immediately for the new user
            const session = await postLoginService.createSessionAndSetCookie(req, res, user);
            await postLoginService.onSuccessfulRegistration(user, session);

            logger.info('[PASSKEY-ROUTES] Initial passkey registered and session created', { userId: user.id });

            return res.json(new PasskeyVerifyResponse({ 
                success: true,
                message: 'Passkey registered',
                session 
            }).forClient());
        } catch (error) {
            logger.error('[PASSKEY-ROUTES] Initial register verify failed', { error: error.message });
            return next(error);
        }
    });

    // Post-setup flow: Requires authenticated session to add additional passkeys
    router.post(PasskeyPaths.REGISTER_VERIFY, passkeyRateLimiter, requireAuth, async (req, res, next) => {
        try {
            const verifyReq = PasskeyRegisterVerifyRequest.parse(req.body);

            // SECURITY: Only allow adding passkeys to the authenticated user's account
            if (verifyReq.user_id !== req.userId) {
                throw new AuthorizationError('Access denied');
            }

            const user = await userService.getUser(verifyReq.user_id);
            if (!user) {
                throw new ResourceNotFoundError('User not found');
            }

            const result = await passkeyAuthService.verifyRegistration(req, user, verifyReq.attestation_response);

            if (!result.verified) {
                logger.warn('[PASSKEY-ROUTES] Registration verification failed', { userId: user.id, error: result.error });
                throw new ValidationError(result.error);
            }

            logger.info('[PASSKEY-ROUTES] Additional passkey added', { userId: user.id });

            return res.json(new PasskeyVerifyResponse({ 
                success: true,
                message: 'Passkey registered'
            }).forClient());
        } catch (error) {
            logger.error('[PASSKEY-ROUTES] Register verify failed', { error: error.message });
            return next(error);
        }
    });

    router.post(PasskeyPaths.AUTH_CHALLENGE, passkeyRateLimiter, async (req, res, next) => {
        try {
            let challengeReq;
            try {
                challengeReq = PasskeyAuthChallengeRequest.parse(req.body);
            } catch (err) {
                throw new ValidationError(err.message);
            }

            const user = await userService.findUserByEmail(challengeReq.email);
            if (!user) {
                logger.warn('[PASSKEY-ROUTES] Auth challenge: no account found for email', { email: challengeReq.email });
                throw new ResourceNotFoundError('No account found for that email');
            }

            const options = await passkeyAuthService.generateAuthenticationChallenge(req, user);

            if (!options) {
                logger.warn('[PASSKEY-ROUTES] Auth challenge: no passkeys registered for user', { email: challengeReq.email });
                return res.status(400).json(new PasskeyAuthChallengeResponse({ 
                    success: false, 
                    message: 'No passkeys registered for this account',
                    needs_setup: true,
                    user_id: user.id
                }).forClient());
            }

            logger.info('[PASSKEY-ROUTES] Auth challenge issued', { userId: user.id });
            return res.json(new PasskeyAuthChallengeResponse({ success: true, message: 'Auth challenge issued', options }).forClient());
        } catch (error) {
            logger.error('[PASSKEY-ROUTES] Auth challenge failed', { error: error.message });
            return next(error);
        }
    });

    router.post(PasskeyPaths.AUTH_VERIFY, passkeyRateLimiter, async (req, res, next) => {
        try {
            let verifyReq;
            try {
                verifyReq = PasskeyAuthVerifyRequest.parse(req.body);
            } catch (err) {
                logger.error('[PASSKEY-ROUTES] Auth verify validation failed', { error: err.message });
                throw new ValidationError(err.message);
            }

            const user = await userService.findUserByEmail(verifyReq.email);
            if (!user) {
                throw new ResourceNotFoundError('No account found for that email');
            }

            const result = await passkeyAuthService.verifyAuthentication(req, user, verifyReq.assertion_response);

            if (!result.verified) {
                logger.warn('[PASSKEY-ROUTES] Auth verify failed', { email: verifyReq.email, error: result.error });
                throw new AuthenticationError(result.error);
            }

            const session = await postLoginService.createSessionAndSetCookie(req, res, user);
            await postLoginService.onSuccessfulLogin(user, session);

            logger.info('[PASSKEY-ROUTES] Auth verify success, session created', { userId: user.id });
            return res.json(new PasskeyVerifyResponse({ success: true, message: 'Authentication successful', session }).forClient());
        } catch (error) {
            logger.error('[PASSKEY-ROUTES] Auth verify failed', { error: error.message });
            return next(error);
        }
    });

    return router;
}
