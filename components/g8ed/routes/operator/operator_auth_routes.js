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
import { operatorAuthRateLimiter, operatorAuthIpBackstopLimiter } from '../../middleware/rate-limit.js';
import { G8eHttpContext } from '../../models/request_models.js';
import { now } from '../../models/base.js';
import { SourceComponent } from '../../constants/ai.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.rateLimiters - Rate limiter objects
 * @param {Object} options.requestTimestampMiddleware - Request timestamp middleware object
 */
export function createOperatorAuthRouter({ services, rateLimiters, requestTimestampMiddleware }) {
    const { operatorService, cliSessionService } = services;
    const { requireRequestTimestamp } = requestTimestampMiddleware;
    const { operatorRefreshRateLimiter } = rateLimiters;
    const router = express.Router();

    // Operator auth uses Bearer token, not web session - create minimal context
    router.use((req, res, next) => {
        const rawPath = req.originalUrl ? req.originalUrl.split('?')[0] : req.path;
        req.g8eContext = G8eHttpContext.parse({
            web_session_id: null,
            user_id: null,
            organization_id: null,
            case_id: req.body?.case_id || req.query?.case_id || req.params?.caseId || null,
            investigation_id: req.body?.investigation_id || req.query?.investigation_id || req.params?.investigationId || null,
            task_id: req.body?.task_id || req.query?.task_id || req.params?.taskId || null,
            bound_operators: [],
            execution_id: `req_${rawPath.replace(/\//g, '_').replace(/^_/, '')}_${now().getTime()}`,
            source_component: SourceComponent.G8ED
        });
        next();
    });

    router.post(AuthPaths.OPERATOR_AUTH, operatorAuthIpBackstopLimiter, operatorAuthRateLimiter, requireRequestTimestamp(), async (req, res) => {
        logger.info('[OPERATOR-AUTH] g8eo Operator authentication request received', {
            hasBody: !!req.body,
            hasBearerToken: !!(req.headers.authorization && req.headers.authorization.startsWith(BEARER_PREFIX)),
            hasSystemInfo: !!(req.body && req.body.system_info),
        });

        try {
            const result = await operatorService.relayAuthenticateOperatorToG8ee({
                ...req.body,
                authorization_header: req.headers.authorization,
            }, req.g8eContext);

            if (!result.success) {
                return res.status(result.statusCode || 401).json(new ErrorResponse({
                    error: result.error,
                    message: result.message || null,
                    data: result.data || {}
                }).forClient());
            }

            return res.json(new OperatorAuthResponse(result.response || result).forClient());
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

            const result = await operatorService.relayRefreshOperatorSessionToG8ee(operator_session_id, req.g8eContext);

            if (!result.success) {
                return res.status(result.statusCode || 401).json(new ErrorResponse({
                    error: result.error || AuthError.INVALID_OR_EXPIRED_SESSION,
                }).forClient());
            }

            logger.info('[OPERATOR-AUTH] Operator session refreshed via g8ee', {
                operatorSessionId: redactWebSessionId(operator_session_id),
                operator_id: result.operator_id,
            });

            return res.json(new OperatorSessionRefreshResponse({
                success: true,
                message: 'Session refreshed successfully',
                operator_id: result.operator_id,
                session: result.session
            }).forClient());
        } catch (error) {
            logger.error('[OPERATOR-AUTH] WebSession refresh failed', { error: error.message });
            return res.status(500).json(new ErrorResponse({
                error: OperatorAuthError.REFRESH_FAILED,
            }).forClient());
        }
    });

    router.post(AuthPaths.OPERATOR_VALIDATE, async (req, res) => {
        try {
            const { operator_session_id } = req.body;

            if (!operator_session_id) {
                return res.status(400).json({ success: false, valid: false, error: 'Missing session_id' });
            }

            let result = null;
            let sessionType = null;

            // Try CLI session first (CLI sessions have cli_session_ prefix)
            if (operator_session_id.startsWith('cli_session_')) {
                const session = await cliSessionService.validateSession(operator_session_id, { ip: req.ip });
                if (session) {
                    result = { success: true, valid: true, user_id: session.user_id, operator_id: null };
                }
                sessionType = 'CLI';
            } else {
                // Try operator session via g8ee
                result = await operatorService.relayValidateOperatorSessionToG8ee(operator_session_id, req.g8eContext);
                sessionType = 'OPERATOR';
            }

            if (!result || !result.valid) {
                logger.info('[OPERATOR-AUTH] Session validation failed', {
                    sessionId: redactWebSessionId(operator_session_id),
                    sessionType,
                });
                return res.status(401).json({ success: false, valid: false, error: 'Invalid or expired session' });
            }

            logger.info('[OPERATOR-AUTH] Session validated successfully', {
                sessionId: redactWebSessionId(operator_session_id),
                sessionType,
                user_id: result.user_id,
            });

            return res.json({
                success: true,
                valid: true,
                session_type: sessionType,
                user_id: result.user_id,
                operator_id: result.operator_id || null,
            });
        } catch (error) {
            logger.error('[OPERATOR-AUTH] Session validation error', { error: error.message });
            return res.status(500).json({ success: false, valid: false, error: 'Internal server error' });
        }
    });

    return router;
}
