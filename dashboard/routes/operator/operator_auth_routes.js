// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import express from 'express';
import { logger } from '../../utils/logger.js';
import { redactWebSessionId } from '../../utils/security.js';
import { OperatorAuthError, AuthError, BEARER_PREFIX } from '../../constants/auth.js';
import { ErrorResponse, OperatorSessionRefreshResponse } from '../../models/response_models.js';
import { AuthPaths } from '../../constants/api_paths.js';
import { ApiKeyError } from '../../constants/auth.js';

/**
 * @param {Object} options
 * @param {Object} options.operatorAuthService - OperatorAuthService instance
 * @param {Object} options.operatorSessionService - OperatorSessionService instance
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createOperatorAuthRouter({ operatorAuthService, operatorSessionService, rateLimiters, requestTimestampMiddleware }) {
    const { requireRequestTimestamp } = requestTimestampMiddleware;
    const { operatorAuthIpBackstopLimiter, operatorAuthRateLimiter, operatorRefreshRateLimiter } = rateLimiters;
    const router = express.Router();

    router.post(AuthPaths.OPERATOR_AUTH, operatorAuthIpBackstopLimiter, operatorAuthRateLimiter, requireRequestTimestamp(), async (req, res) => {
        logger.info('[OPERATOR-AUTH] VSA Operator authentication request received', {
            hasBody: !!req.body,
            hasBearerToken: !!(req.headers.authorization && req.headers.authorization.startsWith(BEARER_PREFIX)),
            hasSystemInfo: !!(req.body && req.body.system_info),
        });

        try {
            const result = await operatorAuthService.authenticateOperator({
                authorizationHeader: req.headers.authorization,
                body: req.body,
            });

            if (!result.success) {
                return res.status(result.statusCode).json(new ErrorResponse({
                    error: result.error,
                    message: result.message || null,
                    data: {
                        code: result.code,
                        key_type: result.key_type,
                        help: result.help,
                        status: result.status,
                        seconds_since_activity: result.seconds_since_activity,
                        existing_type: result.existing_type,
                        requested_type: result.requested_type,
                        stored_fingerprint_prefix: result.stored_fingerprint_prefix,
                        provided_fingerprint_prefix: result.provided_fingerprint_prefix,
                    }
                }).forClient());
            }

            return res.json(result.response.forClient());
        } catch (error) {
            logger.error('[OPERATOR-AUTH] Unexpected error during Operator authentication', {
                error: error.message,
                stack: error.stack,
            });
            return res.status(500).json(new ErrorResponse({
                error: ApiKeyError.INTERNAL_ERROR,
                message: error.message,
            }).forClient());
        }
    });

    router.post(AuthPaths.OPERATOR_REFRESH, operatorRefreshRateLimiter, async (req, res, next) => {
        try {
            const { operator_session_id } = req.body;

            if (!operator_session_id) {
                return res.status(400).json(new ErrorResponse({
                    error: OperatorAuthError.MISSING_OPERATOR_SESSION_ID,
                }).forClient());
            }

            const session = await operatorSessionService.validateSession(operator_session_id);

            if (!session) {
                return res.status(401).json(new ErrorResponse({
                    error: AuthError.INVALID_OR_EXPIRED_SESSION,
                }).forClient());
            }

            await operatorSessionService.refreshSession(operator_session_id, session);

            logger.info('[OPERATOR-AUTH] Operator session refreshed', {
                operatorSessionId: redactWebSessionId(operator_session_id),
                operator_id: session.operator_id,
            });

            return res.json(new OperatorSessionRefreshResponse({
                success: true,
                message: 'Session refreshed successfully',
                operator_id: session.operator_id,
                session: {
                    id: operator_session_id,
                    expires_at: session.expires_at,
                    operator_id: session.operator_id,
                    operator_status: session.operator_status,
                }
            }).forClient());
        } catch (error) {
            logger.error('[OPERATOR-AUTH] WebSession refresh failed', { error: error.message });
            return res.status(500).json(new ErrorResponse({
                error: OperatorAuthError.REFRESH_FAILED,
            }).forClient());
        }
    });

    return router;
}
