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
 * g8ed Internal Device Link Routes
 *
 * Internal HTTP endpoints for device link management.
 * NOT exposed via public routes - only accessible from internal services.
 *
 * Routes (mounted at /api/internal/device-links):
 * - GET  /user/:userId          - List device links for a user
 * - POST /user/:userId          - Create a device link for a user
 * - DELETE /:token              - Revoke a device link (default)
 * - DELETE /:token?action=delete - Permanently delete a device link
 * - POST /operator-link         - Generate a single-operator handshake link
 */

import express from 'express';
import { logger } from '../../utils/logger.js';
import { isValidTokenFormat } from '../../services/auth/device_link_service.js';
import { OperatorLinkRequest } from '../../models/request_models.js';
import { ErrorResponse, DeviceLinkResponse, DeviceLinkListResponse, SimpleSuccessResponse } from '../../models/response_models.js';
import { AuthPaths, DeviceLinkPaths } from '../../constants/api_paths.js';
import { DEVICE_LINK_TTL_SECONDS, DeviceLinkError, DeviceLinkSuccess, DEFAULT_DEVICE_LINK_MAX_USES } from '../../constants/auth.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createInternalDeviceLinkRouter({ services, authorizationMiddleware }) {
    const { deviceLinkService } = services;
    const { requireInternalOrigin, requireInternalOrUserAuth } = authorizationMiddleware;
    const router = express.Router();

    /**
     * GET /api/internal/device-links/user/:userId
     *
     * List all active device links for a user.
     *
     * SECURITY: INTERNAL ONLY - cluster-only access
     */
    router.get('/user/:userId', requireInternalOrUserAuth, async (req, res, next) => {
        const { userId } = req.params;

        if (!userId) {
            return res.status(400).json(new ErrorResponse({ error: 'userId is required' }).forWire());
        }

        try {
            const result = await deviceLinkService.listLinks(userId);

            if (!result.success) {
                return res.status(500).json(new ErrorResponse({ error: result.error }).forWire());
            }

            logger.info('[INTERNAL-HTTP] Device links listed', {
                userId,
                count: result.links.length
            });

            return res.json(new DeviceLinkListResponse({ success: true, links: result.links }).forWire());

        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to list device links', {
                error: error.message,
                userId
            });
            return res.status(500).json(new ErrorResponse({ error: error.message }).forWire());
        }
    });

    /**
     * POST /api/internal/device-links/user/:userId
     *
     * Create a device link for a user.
     *
     * Body: { name?, max_uses?, expires_in_hours? }
     *
     * SECURITY: INTERNAL ONLY - cluster-only access
     */
    router.post('/user/:userId', requireInternalOrUserAuth, async (req, res, next) => {
        const { userId } = req.params;
        const { name, max_uses, expires_in_hours } = req.body;

        if (!userId) {
            return res.status(400).json(new ErrorResponse({ error: 'userId is required' }).forWire());
        }

        const ttl_seconds = expires_in_hours ? expires_in_hours * 3600 : DEVICE_LINK_TTL_SECONDS;

        try {
            const result = await deviceLinkService.createLink({
                user_id: userId,
                organization_id: userId,
                name,
                max_uses: max_uses !== undefined ? max_uses : DEFAULT_DEVICE_LINK_MAX_USES,
                ttl_seconds
            });

            if (!result.success) {
                return res.status(400).json(new ErrorResponse({ error: result.error }).forWire());
            }

            logger.info('[INTERNAL-HTTP] Device link created', {
                userId,
                token_prefix: result.token.substring(0, 25) + '...',
                max_uses: result.max_uses
            });

            return res.status(201).json(new DeviceLinkResponse({
                success: true,
                token: result.token,
                operator_command: result.operator_command,
                name: result.name,
                max_uses: result.max_uses,
                expires_at: result.expires_at
            }).forWire());

        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to create device link', {
                error: error.message,
                userId
            });
            return res.status(500).json(new ErrorResponse({ error: error.message }).forWire());
        }
    });

    /**
     * DELETE /api/internal/device-links/:token
     *
     * Revoke a device link (default) or permanently delete it (?action=delete).
     * Ownership is verified against the token's stored user_id.
     *
     * Query params:
     *   action=delete  — permanently delete (only allowed when not ACTIVE)
     *   (default)      — revoke (marks as REVOKED, short TTL retained)
     *
     * SECURITY: INTERNAL ONLY - cluster-only access
     */
    router.delete('/:token', requireInternalOrUserAuth, async (req, res, next) => {
        const { token } = req.params;
        const action = req.query.action;

        if (!isValidTokenFormat(token)) {
            return res.status(400).json(new ErrorResponse({ error: DeviceLinkError.INVALID_TOKEN_FORMAT }).forWire());
        }

        try {
            const linkResult = await deviceLinkService.getLink(token);
            if (!linkResult.success) {
                return res.status(404).json(new ErrorResponse({ error: linkResult.error }).forWire());
            }

            const user_id = linkResult.data.user_id;

            const result = action === 'delete'
                ? await deviceLinkService.deleteLink(token, user_id)
                : await deviceLinkService.revokeLink(token, user_id);

            if (!result.success) {
                return res.status(400).json(new ErrorResponse({ error: result.error }).forWire());
            }

            logger.info('[INTERNAL-HTTP] Device link processed', {
                action: action || 'revoke',
                token_prefix: token.substring(0, 20) + '...'
            });

            return res.json(new SimpleSuccessResponse({ success: true, message: action === 'delete' ? DeviceLinkSuccess.DELETED : DeviceLinkSuccess.REVOKED }).forWire());
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to process device link', {
                error: error.message,
                token_prefix: token.substring(0, 20) + '...'
            });
            return res.status(500).json(new ErrorResponse({ error: error.message }).forWire());
        }
    });

    /**
     * POST /api/internal/device-links/operator-link
     *
     * Generate a single-operator handshake link (dlk_ token).
     *
     * Body: { user_id, organization_id, operator_id, web_session_id }
     *
     * SECURITY: INTERNAL ONLY - cluster-only access
     */
    router.post('/operator-link', requireInternalOrUserAuth, async (req, res, next) => {
        try {
            const generateReq = OperatorLinkRequest.parse(req.body);

            const result = await deviceLinkService.generateLink({
                user_id: generateReq.user_id,
                organization_id: generateReq.organization_id,
                operator_id: generateReq.operator_id,
                web_session_id: generateReq.web_session_id
            });

            if (!result.success) {
                return res.status(400).json(new ErrorResponse({ error: result.error }).forWire());
            }

            logger.info('[INTERNAL-HTTP] Operator device link generated', {
                userId: generateReq.user_id,
                operatorId: generateReq.operator_id,
                token_prefix: result.token.substring(0, 25) + '...'
            });

            return res.status(201).json(new DeviceLinkResponse({
                success: true,
                token: result.token,
                operator_command: result.operator_command,
                expires_at: result.expires_at
            }).forWire());

        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to generate operator device link', {
                error: error.message,
                body: req.body
            });
            return res.status(500).json(new ErrorResponse({ error: error.message }).forWire());
        }
    });

    return router;
}
