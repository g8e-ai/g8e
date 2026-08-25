// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

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
    DropKeyError
} from '../../services/error_service.js';
import { ErrorResponse, UserMeResponse, UserDevLogsResponse, UserDropKeyRefreshResponse } from '../../models/response_models.js';
import { UserPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.userService - UserService instance
 * @param {Object} options.authMiddleware - Auth middleware object
 */
export function createUserRouter({ userService, authMiddleware }) {
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

    router.post(UserPaths.REFRESH_DROP_KEY, requireAuth, async (req, res, next) => {
        try {
            const user = await userService.getUser(req.userId);

            if (!user) {
                throw new ResourceNotFoundError(ApiKeyError.USER_NOT_FOUND);
            }

            const result = await userService.refreshUserDropKey(req.userId, user.organization_id);

            logger.info('[USER-API] drop_key refreshed', { userId: req.userId });

            return res.json(new UserDropKeyRefreshResponse({
                success: true,
                message: 'Drop key refreshed successfully',
                drop_key: result.api_key
            }).forClient());
        } catch (error) {
            logger.error('[USER-API] Error refreshing drop_key', { error: error.message });
            return next(error);
        }
    });

    return router;
}
