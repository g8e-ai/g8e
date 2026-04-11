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
import { ApiKeyError } from '../../constants/auth.js';
import { 
    AuthenticationError, 
    AuthorizationError, 
    ValidationError, 
    ResourceNotFoundError, 
    InternalServerError,
    BusinessLogicError,
    G8eKeyError
} from '../../services/error_service.js';
import { ErrorResponse, UserMeResponse, UserDevLogsResponse, UserG8eKeyRefreshResponse } from '../../models/response_models.js';
import { UserPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authMiddleware - Auth middleware object
 */
export function createUserRouter({ services, authMiddleware }) {
    const { userService } = services;
    const { requireAuth, requireAdmin } = authMiddleware;
    const router = express.Router();

    router.get(UserPaths.ME, requireAuth, async (req, res, next) => {
        try {
            const user = await userService.getUser(req.userId);

            if (!user) {
                throw new ResourceNotFoundError(ApiKeyError.USER_NOT_FOUND);
            }

            return res.json(new UserMeResponse(user.forClient()).forClient());
        } catch (error) {
            logger.error('[USER-API] Error fetching user', {
                error: error.message
            });
            return next(error);
        }
    });

    router.patch(UserPaths.DEV_LOGS, requireAdmin, async (req, res, next) => {
        const { enabled } = req.body ?? {};

        if (typeof enabled !== 'boolean') {
            return next(new ValidationError('enabled (boolean) is required'));
        }

        try {
            const user = await userService.updateUser(req.userId, { dev_logs_enabled: enabled });

            logger.info('[USER-API] dev_logs_enabled updated', { userId: req.userId, enabled });

            return res.json(new UserDevLogsResponse({ 
                message: `Dev logs ${enabled ? 'enabled' : 'disabled'}`, 
                dev_logs_enabled: user.dev_logs_enabled 
            }).forClient());
        } catch (error) {
            logger.error('[USER-API] Error updating dev_logs_enabled', { error: error.message });
            return next(error);
        }
    });

    router.post(UserPaths.REFRESH_G8E_KEY, requireAuth, async (req, res, next) => {
        try {
            const user = await userService.getUser(req.userId);

            if (!user) {
                throw new ResourceNotFoundError(ApiKeyError.USER_NOT_FOUND);
            }

            const result = await userService.refreshUserG8eKey(req.userId, user.organization_id);

            logger.info('[USER-API] g8e_key refreshed', { userId: req.userId });

            return res.json(new UserG8eKeyRefreshResponse({
                success: true,
                message: 'g8e key refreshed successfully',
                g8e_key: result.api_key
            }).forClient());
        } catch (error) {
            logger.error('[USER-API] Error refreshing g8e_key', { error: error.message });
            return next(error);
        }
    });

    return router;
}
