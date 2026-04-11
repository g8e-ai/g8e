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
 * VSOD Internal User Routes
 * 
 * Internal HTTP endpoints for user queries (read-only).
 * NOT exposed via public routes - only accessible from internal services.
 */

import { CreateUserRequest, UpdateUserRolesRequest } from '../../models/request_models.js';
import express from 'express';
import { logger } from '../../utils/logger.js';
import { SessionEndReason } from '../../constants/session.js';
import { ApiKeyError, UserRole } from '../../constants/auth.js';
import { ErrorResponse, InternalUserListResponse, InternalUserResponse, PasskeyListResponse, PasskeyRevokeResponse, PasskeyRevokeAllResponse, UserDeleteResponse } from '../../models/response_models.js';
import { 
    AuthenticationError, 
    AuthorizationError, 
    ValidationError, 
    ResourceNotFoundError, 
    InternalServerError,
    BusinessLogicError
} from '../../services/error_service.js';
import {
    USER_STATS_QUERY_LIMIT,
    USER_LIST_DEFAULT_LIMIT,
    USER_LIST_MAX_LIMIT,
    USER_LIST_MIN_LIMIT,
} from '../../constants/service_config.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createInternalUserRouter({ services, authorizationMiddleware }) {
    const { userService, webSessionService, passkeyAuthService } = services;
    const { requireInternalOrigin } = authorizationMiddleware;
    const router = express.Router();

    /**
     * GET /api/internal/users/stats
     */
    router.get('/stats', requireInternalOrigin, async (req, res, next) => {
        try {
            logger.info('[INTERNAL-HTTP] Getting user stats');

            const stats = await userService.getUserStats(USER_STATS_QUERY_LIMIT);

            logger.info('[INTERNAL-HTTP] User stats generated', {
                total: stats.data.total_users
            });

            return res.json(stats);
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to get user stats', {
                error: error.message
            });

            if (next) {
                return next(error);
            } else {
                return res.status(500).json(new ErrorResponse({ error: error.message }).forWire());
            }
        }
    });

    /**
     * GET /api/internal/users
     */
    router.get('/', requireInternalOrigin, async (req, res, next) => {
        try {
            const { limit: limitParam } = req.query;

            let limit = parseInt(limitParam, 10) || USER_LIST_DEFAULT_LIMIT;
            if (limit > USER_LIST_MAX_LIMIT) limit = USER_LIST_MAX_LIMIT;
            if (limit < USER_LIST_MIN_LIMIT) limit = USER_LIST_MIN_LIMIT;

            logger.info('[INTERNAL-HTTP] Listing users', { limit });

            const users = await userService.listUsers(limit);

            logger.info('[INTERNAL-HTTP] Users listed', { count: users.length });

            return res.json(new InternalUserListResponse({
                success: true,
                users: users.map(u => u.forWire()),
                count: users.length,
            }).forWire());
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to list users', {
                error: error.message
            });

            return next(error);
        }
    });

    /**
     * GET /api/internal/users/email/:email
     */
    router.get('/email/:email', requireInternalOrigin, async (req, res, next) => {
        try {
            const { email } = req.params;

            if (!email) {
                throw new ValidationError('email is required');
            }

            logger.info('[INTERNAL-HTTP] User lookup by email', { email });

            const user = await userService.findUserByEmail(email);

            if (!user) {
                throw new ResourceNotFoundError(ApiKeyError.USER_NOT_FOUND);
            }

            logger.info('[INTERNAL-HTTP] User found by email', {
                userId: user.id,
                email: user.email
            });

            return res.json(new InternalUserResponse({
                user: user.forWire()
            }).forWire());
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to get user by email', {
                error: error.message,
                email: req.params.email
            });

            return next(error);
        }
    });

    /**
     * GET /api/internal/users/:userId
     */
    router.get('/:userId', requireInternalOrigin, async (req, res, next) => {
        try {
            const { userId } = req.params;

            if (!userId) {
                throw new ValidationError('userId is required');
            }

            logger.info('[INTERNAL-HTTP] User lookup by ID', { userId });

            const user = await userService.getUser(userId);

            if (!user) {
                throw new ResourceNotFoundError(ApiKeyError.USER_NOT_FOUND);
            }

            logger.info('[INTERNAL-HTTP] User found', {
                userId,
                email: user.email
            });

            return res.json(new InternalUserResponse({
                user: user.forWire()
            }).forWire());
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to get user by ID', {
                error: error.message,
                userId: req.params.userId
            });

            return next(error);
        }
    });

    /**
     * POST /api/internal/users
     */
    router.post('/', requireInternalOrigin, async (req, res, next) => {
        try {
            const userReq = CreateUserRequest.parse(req.body);

            const existing = await userService.findUserByEmail(userReq.email);
            if (existing) {
                throw new BusinessLogicError('User already exists with that email', { category: 'conflict' });
            }

            logger.info('[INTERNAL-HTTP] Creating user', { email: userReq.email });

            const user = await userService.createUser({
                email: userReq.email,
                name: userReq.name,
                roles: userReq.roles || [UserRole.USER],
            });

            logger.info('[INTERNAL-HTTP] User created', { userId: user.id, email: user.email });

            return res.status(201).json(new InternalUserResponse({ 
                user: user.forWire() 
            }).forWire());
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to create user', { error: error.message });
            return next(error);
        }
    });

    /**
     * PATCH /api/internal/users/:userId/roles
     */
    router.patch('/:userId/roles', requireInternalOrigin, async (req, res, next) => {
        try {
            const { userId } = req.params;
            const rolesReq = UpdateUserRolesRequest.parse(req.body);

            const validRoles = Object.values(UserRole);
            if (!validRoles.includes(rolesReq.role)) {
                throw new ValidationError(`Invalid role: ${rolesReq.role}. Valid roles: ${validRoles.join(', ')}`);
            }

            if (!['set', 'add', 'remove'].includes(rolesReq.action)) {
                throw new ValidationError("action must be 'set', 'add', or 'remove'");
            }

            const user = await userService.getUser(userId);
            if (!user) {
                throw new ResourceNotFoundError(ApiKeyError.USER_NOT_FOUND);
            }

            const currentRoles = user.roles || [];
            let newRoles;
            if (rolesReq.action === 'set') {
                newRoles = [rolesReq.role];
            } else if (rolesReq.action === 'add') {
                newRoles = [...new Set([...currentRoles, rolesReq.role])];
            } else {
                newRoles = currentRoles.filter(r => r !== rolesReq.role);
            }

            logger.info('[INTERNAL-HTTP] Updating user roles', { userId, action: rolesReq.action, role: rolesReq.role, newRoles });

            const updated = await userService.updateUser(userId, { roles: newRoles });

            logger.info('[INTERNAL-HTTP] User roles updated', { userId, newRoles });

            return res.json(new InternalUserResponse({ 
                user: updated.forWire() 
            }).forWire());
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to update user roles', { error: error.message, userId: req.params.userId });
            return next(error);
        }
    });

    /**
     * GET /api/internal/users/:userId/passkeys
     */
    router.get('/:userId/passkeys', requireInternalOrigin, async (req, res, next) => {
        try {
            const { userId } = req.params;

            const credentials = await passkeyAuthService.listCredentials(userId);
            if (credentials === null) {
                throw new ResourceNotFoundError(ApiKeyError.USER_NOT_FOUND);
            }

            return res.json(new PasskeyListResponse({ 
                message: 'Passkeys listed successfully',
                user_id: userId, 
                credentials, 
                count: credentials.length 
            }).forWire());
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to list passkeys', { error: error.message, userId: req.params.userId });
            return next(error);
        }
    });

    /**
     * DELETE /api/internal/users/:userId/passkeys/:credentialId
     */
    router.delete('/:userId/passkeys/:credentialId', requireInternalOrigin, async (req, res, next) => {
        try {
            const { userId, credentialId } = req.params;

            const result = await passkeyAuthService.revokeCredential(userId, credentialId);

            if (!result.userExists) {
                throw new ResourceNotFoundError(ApiKeyError.USER_NOT_FOUND);
            }
            if (!result.found) {
                throw new ResourceNotFoundError('Credential not found');
            }

            logger.info('[INTERNAL-HTTP] Passkey credential revoked', { userId, credentialId: credentialId.substring(0, 12) + '...' });

            return res.json(new PasskeyRevokeResponse({ 
                message: 'Passkey credential revoked successfully',
                user_id: userId, 
                credential_id: credentialId, 
                remaining: result.remaining 
            }).forWire());
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to revoke passkey', { error: error.message, userId: req.params.userId });
            return next(error);
        }
    });

    /**
     * DELETE /api/internal/users/:userId/passkeys
     */
    router.delete('/:userId/passkeys', requireInternalOrigin, async (req, res, next) => {
        try {
            const { userId } = req.params;

            const result = await passkeyAuthService.revokeAllCredentials(userId);

            if (!result.userExists) {
                throw new ResourceNotFoundError(ApiKeyError.USER_NOT_FOUND);
            }

            logger.info('[INTERNAL-HTTP] All passkey credentials revoked', { userId, count: result.revoked });

            return res.json(new PasskeyRevokeAllResponse({ 
                message: 'All passkey credentials revoked successfully',
                user_id: userId, 
                revoked: result.revoked 
            }).forWire());
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to revoke all passkeys', { error: error.message, userId: req.params.userId });
            return next(error);
        }
    });

    /**
     * DELETE /api/internal/users/:userId
     */
    router.delete('/:userId', requireInternalOrigin, async (req, res, next) => {
        try {
            const { userId } = req.params;

            const user = await userService.getUser(userId);
            if (!user) {
                throw new ResourceNotFoundError(ApiKeyError.USER_NOT_FOUND);
            }

            logger.info('[INTERNAL-HTTP] Deleting user', { userId, email: user.email });

            await webSessionService.invalidateAllUserSessions(userId, SessionEndReason.USER_DELETED);

            const deleted = await userService.deleteUser(userId);
            if (!deleted) {
                throw new InternalServerError('Failed to delete user');
            }

            logger.info('[INTERNAL-HTTP] User deleted', { userId });

            return res.json(new UserDeleteResponse({ 
                message: 'User deleted successfully',
                user_id: userId 
            }).forWire());

        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to delete user', { error: error.message, userId: req.params.userId });
            return next(error);
        }
    });

    return router;
}
