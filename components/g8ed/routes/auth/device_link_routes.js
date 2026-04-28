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

import { 
    CreateDeviceLinkRequest, 
    GenerateDeviceLinkRequest, 
    RegisterDeviceRequest,
    OperatorLinkRequest 
} from '../../models/request_models.js';
import express from 'express';
import { ApiKeyError, DeviceLinkError, DEVICE_LINK_TTL_SECONDS, WEB_SESSION_ID_HEADER } from '../../constants/auth.js';
import { logger } from '../../utils/logger.js';
import { isValidTokenFormat } from '../../services/auth/device_link_service.js';
import { 
    AuthenticationError, 
    AuthorizationError, 
    ValidationError, 
    ResourceNotFoundError, 
    InternalServerError,
    BusinessLogicError
} from '../../services/error_service.js';
import { DeviceLinkResponse, DeviceLinkListResponse, DeviceRegistrationResponse, SimpleSuccessResponse, ErrorResponse } from '../../models/response_models.js';
import { AuthPaths, DeviceLinkPaths } from '../../constants/api_paths.js';
import { extractClientIp } from '../../utils/request.js';
import { deviceLinkRateLimiter } from '../../middleware/rate-limit.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createDeviceLinkRouter({
    services,
    authMiddleware,
    rateLimiters
}) {
    const { deviceLinkService } = services;
    const { requireAuth } = authMiddleware;
    const {
        deviceLinkGenerateLimiter,
        deviceLinkCreateRateLimiter,
        deviceLinkListRateLimiter,
        deviceLinkRevokeRateLimiter
    } = rateLimiters;

    const authRouter = express.Router();

    authRouter.post(AuthPaths.LINK_GENERATE, requireAuth, deviceLinkGenerateLimiter, async (req, res, next) => {
        try {
            const generateReq = GenerateDeviceLinkRequest.parse(req.body);
            const user_id = req.userId;
            const organization_id = req.session?.user_data?.organization_id;
            const web_session_id = req.webSessionId || req.headers[WEB_SESSION_ID_HEADER];

            const result = await deviceLinkService.generateLink({
                user_id,
                organization_id,
                operator_id: generateReq.operator_id,
                web_session_id
            });

            if (!result.success) {
                throw new BusinessLogicError(result.error);
            }

            return res.status(201).json(new DeviceLinkResponse({
                success: true,
                token: result.token,
                operator_command: result.operator_command,
                expires_at: result.expires_at
            }).forClient());

        } catch (error) {
            logger.error('[DEVICE-LINK-ROUTES] Failed to generate link', { error: error.message });
            return next(error);
        }
    });

    const deviceLinkRouter = express.Router();

    deviceLinkRouter.post(DeviceLinkPaths.CREATE, requireAuth, deviceLinkCreateRateLimiter, async (req, res, next) => {
        try {
            const createReq = CreateDeviceLinkRequest.parse(req.body);
            const user_id = req.userId;
            const organization_id = req.session?.user_data?.organization_id;

            const result = await deviceLinkService.createLink({
                user_id,
                organization_id,
                name: createReq.name,
                max_uses: createReq.max_uses,
                ttl_seconds: createReq.expires_in_hours * 3600
            });

            if (!result.success) {
                throw new BusinessLogicError(result.error);
            }

            return res.status(201).json(new DeviceLinkResponse({
                success: true,
                token: result.token,
                operator_command: result.operator_command,
                name: result.name,
                max_uses: result.max_uses,
                expires_at: result.expires_at
            }).forClient());

        } catch (error) {
            logger.error('[DEVICE-LINK-ROUTES] Failed to create link', { error: error.message });
            return next(error);
        }
    });

    deviceLinkRouter.get(DeviceLinkPaths.LIST, requireAuth, deviceLinkListRateLimiter, async (req, res, next) => {
        try {
            const user_id = req.userId;

            const result = await deviceLinkService.listLinks(user_id);

            if (!result.success) {
                throw new InternalServerError(result.error);
            }

            return res.json(new DeviceLinkListResponse({
                success: true,
                links: result.links
            }).forClient());

        } catch (error) {
            logger.error('[DEVICE-LINK-ROUTES] Failed to list links', { error: error.message });
            return next(error);
        }
    });

    deviceLinkRouter.delete(DeviceLinkPaths.REVOKE, requireAuth, deviceLinkRevokeRateLimiter, async (req, res, next) => {
        try {
            const { token } = req.params;
            const user_id = req.userId;
            const action = req.query.action;

            if (!isValidTokenFormat(token)) {
                throw new ValidationError(DeviceLinkError.INVALID_TOKEN_FORMAT);
            }

            const result = action === 'delete'
                ? await deviceLinkService.deleteLink(token, user_id)
                : await deviceLinkService.revokeLink(token, user_id);

            if (!result.success) {
                throw new BusinessLogicError(result.error);
            }

            return res.json(new SimpleSuccessResponse({ 
                success: true,
                message: 'Link revoked successfully' 
            }).forClient());

        } catch (error) {
            logger.error('[DEVICE-LINK-ROUTES] Failed to process link deletion', { error: error.message });
            return next(error);
        }
    });

    const registerRouter = express.Router();

    registerRouter.post(DeviceLinkPaths.REGISTER, deviceLinkRateLimiter, async (req, res, next) => {
        try {
            const { token } = req.params;
            const registerReq = RegisterDeviceRequest.parse(req.body);

            if (!isValidTokenFormat(token)) {
                throw new ValidationError(DeviceLinkError.INVALID_TOKEN_FORMAT);
            }

            const ip_address = extractClientIp(req);

            const result = await deviceLinkService.registerDevice(token, {
                ...registerReq.forWire(),
                system_fingerprint: req.body.system_fingerprint,
                ip_address
            });

            if (!result.success) {
                throw new BusinessLogicError(result.error);
            }

            return res.json(new DeviceRegistrationResponse({
                success: true,
                operator_session_id: result.operator_session_id,
                operator_id: result.operator_id,
                api_key: result.api_key,
                operator_cert: result.operator_cert,
                operator_cert_key: result.operator_cert_key,
                session: result.session
            }).forClient());

        } catch (error) {
            logger.error('[DEVICE-LINK-ROUTES] Failed to register device', { error: error.message });
            return next(error);
        }
    });

    return { authRouter, deviceLinkRouter, registerRouter };
}
