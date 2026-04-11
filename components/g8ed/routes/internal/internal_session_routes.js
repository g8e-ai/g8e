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
 * g8ed Internal WebSession Routes
 * 
 * Internal HTTP endpoints for session validation.
 * Used by internal services to validate web sessions.
 * NOT exposed via public routes - only accessible from internal services.
 */

import express from 'express';
import { logger } from '../../utils/logger.js';
import { HTTP_G8E_SERVICE_HEADER } from '../../constants/headers.js';
import { SessionType } from '../../constants/session.js';
import { ErrorResponse, InternalSessionValidationResponse } from '../../models/response_models.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createInternalSessionRouter({ services, authorizationMiddleware }) {
    const { webSessionService, userService } = services;
    const { requireInternalOrigin } = authorizationMiddleware;
    const router = express.Router();

    /**
     * GET /api/internal/session/:sessionId
     * 
     * Validate a web session and return session data.
     */
    router.get('/:sessionId', requireInternalOrigin, async (req, res, next) => {
        try {
            const { sessionId } = req.params;
            const callingService = req.headers[HTTP_G8E_SERVICE_HEADER.toLowerCase()] || 'unknown';

            if (!sessionId) {
                return res.status(400).json(new ErrorResponse({
                    error: 'sessionId is required'
                }).forWire());
            }

            logger.info('[INTERNAL-SESSION] WebSession validation request', {
                sessionId: sessionId.substring(0, 12) + '...',
                callingService
            });

            const session = await webSessionService.validateSession(sessionId);

            if (!session) {
                logger.info('[INTERNAL-SESSION] WebSession not found', {
                    sessionId: sessionId.substring(0, 12) + '...'
                });
                return res.status(200).json(new ErrorResponse({
                    error: 'WebSession not found or expired'
                }).forWire());
            }

            if (session.session_type !== SessionType.WEB) {
                logger.info('[INTERNAL-SESSION] Invalid session type for console', {
                    sessionId: sessionId.substring(0, 12) + '...',
                    sessionType: session.session_type
                });
                return res.status(200).json(new ErrorResponse({
                    error: 'Invalid session type'
                }).forWire());
            }

            if (!session.is_active) {
                logger.info('[INTERNAL-SESSION] WebSession is inactive', {
                    sessionId: sessionId.substring(0, 12) + '...'
                });
                return res.status(200).json(new ErrorResponse({
                    error: 'WebSession is inactive'
                }).forWire());
            }

            let userRoles = [];
            
            if (session.user_id) {
                const user = await userService.getUser(session.user_id);
                if (user) {
                    userRoles = user.roles;
                }
            }

            logger.info('[INTERNAL-SESSION] WebSession validated successfully', {
                sessionId: sessionId.substring(0, 12) + '...',
                userId: session.user_id,
                email: session.user_data?.email,
                roles: userRoles,
                callingService
            });

            return res.json(new InternalSessionValidationResponse({
                success: true,
                message: 'WebSession validated successfully',
                session_id: session.id,
                user_id: session.user_id,
                valid: session.is_active,
                expires_at: session.expires_at,
                validation_details: {
                    organization_id: session.organization_id,
                    session_type: session.session_type,
                    is_active: session.is_active,
                    user_data: {
                        email: session.user_data?.email,
                        name: session.user_data?.name,
                        picture: session.user_data?.picture,
                        roles: userRoles
                    },
                    created_at: session.created_at,
                    expires_at: session.absolute_expires_at
                }
            }).forWire());
        } catch (error) {
            logger.error('[INTERNAL-SESSION] WebSession validation failed', {
                error: error.message,
                stack: error.stack,
                sessionId: req.params.sessionId?.substring(0, 12) + '...'
            });

            return res.status(500).json(new ErrorResponse({
                error: error.message || 'Failed to validate session'
            }).forWire());
        }
    });

    return router;
}
