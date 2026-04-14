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
import { redactWebSessionId } from '../../utils/security.js';
import { OperatorAuthError, AuthError, BEARER_PREFIX } from '../../constants/auth.js';
import { ErrorResponse, OperatorAuthResponse, OperatorSessionRefreshResponse } from '../../models/response_models.js';
import { AuthPaths } from '../../constants/api_paths.js';
import { ApiKeyError } from '../../constants/auth.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.rateLimiters - Rate limiter objects
 * @param {Object} options.requestTimestampMiddleware - Request timestamp middleware object
 */
export function createOperatorAuthRouter({ services, rateLimiters, requestTimestampMiddleware }) {
    const { operatorAuthService, operatorSessionService } = services;
    const { requireRequestTimestamp } = requestTimestampMiddleware;
    const { operatorAuthIpBackstopLimiter, operatorAuthRateLimiter, operatorRefreshRateLimiter } = rateLimiters;
    const router = express.Router();

    router.post(AuthPaths.OPERATOR_AUTH, operatorAuthIpBackstopLimiter, operatorAuthRateLimiter, requireRequestTimestamp(), async (req, res) => {
        logger.info('[OPERATOR-AUTH] g8eo Operator authentication request received', {
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
