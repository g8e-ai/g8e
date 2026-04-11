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
 * Authorization Middleware
 * 
 * Provides authorization checks to ensure authenticated users can only access
 * their own resources and internal endpoints are restricted to cluster-only access.
 */

import { logger } from '../utils/logger.js';
import { AuthenticationError, AuthorizationError, InternalServerError } from '../services/error_service.js';
import { AuthError, INTERNAL_AUTH_HEADER, DeviceLinkError } from '../constants/auth.js';
import crypto from 'crypto';

export function createAuthorizationMiddleware({ operatorService, settingsService }) {
    /**
     * Ensure request user matches resource owner
     * 
     * Validates that the user_id in the request (params, query, or body) matches
     * the authenticated session user_id. Prevents users from accessing other users' resources.
     */
    const requireOwnership = async (req, res, next) => {
        try {
            const sessionUserId = req.userId;
            
            if (!sessionUserId) {
                logger.warn('[AUTH] Ownership check failed - no authenticated session', {
                    endpoint: req.path,
                    ip: req.ip
                });
                throw new AuthenticationError(AuthError.REQUIRED);
            }
            
            // Extract requested user_id from various sources
            const requestUserId = req.params.userId || req.query.user_id || req.body.user_id;
            
            // If a user_id is specified in the request, it must match the session user
            if (requestUserId && requestUserId !== sessionUserId) {
                logger.warn('[AUTH] OWNERSHIP VIOLATION ATTEMPT', {
                    sessionUser: sessionUserId,
                    requestedUser: requestUserId,
                    endpoint: req.path,
                    method: req.method,
                    ip: req.ip,
                    userAgent: req.headers['user-agent']
                });
                throw new AuthorizationError(AuthError.FORBIDDEN_RESOURCE);
            }
            
            // Store authenticated user_id in request for handler use
            req.authenticatedUserId = sessionUserId;
            
            next();
        } catch (error) {
            logger.error('[AUTH] Ownership check error', {
                error: error.message,
                endpoint: req.path
            });
            next(error instanceof AuthenticationError || error instanceof AuthorizationError 
                ? error 
                : new InternalServerError(AuthError.AUTHORIZATION_CHECK_FAILED, { cause: error }));
        }
    };

    /**
     * Ensure Operator belongs to authenticated user
     * 
     * Validates that the operator_id in the request belongs to the authenticated user.
     * Prevents users from controlling or accessing other users' operators.
     */
    const requireOperatorOwnership = async (req, res, next) => {
        try {
            const sessionUserId = req.userId;
            
            if (!sessionUserId) {
                logger.warn('[AUTH] Operator ownership check failed - no authenticated session', {
                    endpoint: req.path,
                    operatorId: req.params.operatorId,
                    ip: req.ip
                });
                throw new AuthenticationError(AuthError.REQUIRED);
            }
            
            const operatorId = req.params.operatorId || req.body.operator_id;
            
            if (!operatorId) {
                throw new AuthenticationError(AuthError.OPERATOR_ID_REQUIRED);
            }
            
            const operator = await operatorService.getOperator(operatorId);
            
            if (!operator) {
                logger.warn('[AUTH] Operator ownership check - Operator not found', {
                    operatorId,
                    sessionUser: sessionUserId,
                    endpoint: req.path
                });
                throw new AuthenticationError(DeviceLinkError.OPERATOR_NOT_FOUND);
            }
            
            if (operator.user_id !== sessionUserId) {
                logger.warn('[AUTH] OPERATOR OWNERSHIP VIOLATION ATTEMPT', {
                    sessionUser: sessionUserId,
                    operatorId,
                    operatorOwner: operator.user_id,
                    endpoint: req.path,
                    method: req.method,
                    ip: req.ip,
                    userAgent: req.headers['user-agent']
                });
                throw new AuthorizationError(AuthError.FORBIDDEN_OPERATOR);
            }
            
            // Store Operator and authenticated user in request
            req.operator = operator;
            req.authenticatedUserId = sessionUserId;
            
            logger.info('[AUTH] Operator ownership validated', {
                operatorId,
                userId: sessionUserId,
                endpoint: req.path
            });
            
            next();
        } catch (error) {
            logger.error('[AUTH] Operator ownership check error', {
                error: error.message,
                operatorId: req.params.operatorId,
                endpoint: req.path
            });
            next(error instanceof AuthenticationError || error instanceof AuthorizationError 
                ? error 
                : new InternalServerError(AuthError.AUTHORIZATION_CHECK_FAILED, { cause: error }));
        }
    };

    /**
     * Restrict to internal Docker container calls only (VSE, g8e-pod)
     * 
     * All containers share INTERNAL_AUTH_TOKEN via the Docker network.
     * Localhost health checks are allowed without a token (Docker healthcheck runs inside the container).
     */
    const requireInternalOrigin = (req, res, next) => {
        const internalAuthToken = req.headers[INTERNAL_AUTH_HEADER];
        const expectedToken = settingsService?.getInternalAuthToken() || null;

        if (internalAuthToken && expectedToken) {
            const tokenBuffer = Buffer.from(internalAuthToken);
            const expectedBuffer = Buffer.from(expectedToken);
            if (tokenBuffer.length === expectedBuffer.length &&
                crypto.timingSafeEqual(tokenBuffer, expectedBuffer)) {
                return next();
            }
        }

        const ip = req.ip?.replace(/^::ffff:/, '') || req.ip;
        if ((ip === '127.0.0.1' || ip === '::1') && req.originalUrl.startsWith('/health')) {
            return next();
        }

        logger.warn('[AUTH] Internal endpoint access denied', {
            endpoint: req.path,
            method: req.method,
            ip: req.ip
        });

        throw new AuthorizationError(AuthError.FORBIDDEN_INTERNAL);
    };

    return {
        requireOwnership,
        requireOperatorOwnership,
        requireInternalOrigin
    };
}
