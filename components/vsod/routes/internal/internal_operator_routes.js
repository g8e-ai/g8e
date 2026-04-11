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

import { IntentRequest } from '../../models/request_models.js';
import express from 'express';
import { logger } from '../../utils/logger.js';
import { OperatorStatus } from '../../constants/operator.js';
import { DeviceLinkError } from '../../constants/auth.js';
import { OperatorDocument, OperatorWithSessionContext } from '../../models/operator_model.js';
import { ErrorResponse, OperatorListResponse, OperatorSlotsResponse } from '../../models/response_models.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createInternalOperatorRouter({ services, authorizationMiddleware }) {
    const { operatorService, g8eNodeOperatorService } = services;
    const { requireInternalOrigin } = authorizationMiddleware;
    const router = express.Router();

    /**
     * POST /api/internal/operators/:operatorId/refresh-key
     */
    router.post('/:operatorId/refresh-key', requireInternalOrigin, async (req, res, next) => {
        try {
            const { operatorId } = req.params;
            const { user_id } = req.body || {};

            if (!user_id) {
                return res.status(400).json(new ErrorResponse({
                    error: 'user_id is required in request body'
                }).forWire());
            }

            logger.info('[INTERNAL-HTTP] Operator API key refresh requested', {
                operator_id: operatorId,
                user_id
            });

            const result = await operatorService.refreshOperatorApiKey(operatorId, user_id);

            if (!result.success) {
                logger.warn('[INTERNAL-HTTP] Operator API key refresh failed', {
                    operator_id: operatorId,
                    error: result.message
                });
                return res.status(400).json(new ErrorResponse({
                    error: result.message
                }).forWire());
            }

            logger.info('[INTERNAL-HTTP] Operator API key refreshed', {
                old_operator_id: operatorId,
                new_operator_id: result.new_operator_id,
                slot_number: result.slot_number,
                user_id
            });

            return res.json({
                success: true,
                message: result.message,
                old_operator_id: operatorId,
                new_operator_id: result.new_operator_id,
                status: 'active'
            });

        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to refresh operator API key', {
                error: error.message,
                operator_id: req.params.operatorId
            });
            return res.status(500).json(new ErrorResponse({
                error: error.message || 'Failed to refresh operator API key'
            }).forWire());
        }
    });

    /**
     * POST /api/internal/operators/:operatorId/reset-cache
     */
    router.post('/:operatorId/reset-cache', requireInternalOrigin, async (req, res, next) => {
        try {
            const { operatorId: operator_id } = req.params;

            logger.info('[INTERNAL-HTTP] Operator reset request', {
                operator_id
            });

            // Reset Operator to fresh state (delete + recreate)
            const result = await operatorService.resetOperator(operator_id);

            if (!result.success) {
                logger.warn('[INTERNAL-HTTP] Operator reset failed', {
                    operator_id,
                    error: result.error
                });

                return res.status(404).json(new ErrorResponse({
                    error: result.error
                }).forWire());
            }

            logger.info('[INTERNAL-HTTP] Operator reset completed', {
                operator_id,
                status: result.operator?.status
            });

            return res.json({
                success: true,
                operator_id,
                status: result.operator?.status
            });

        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to reset operator', {
                error: error.message,
                stack: error.stack
            });

            return res.status(500).json(new ErrorResponse({
                error: error.message || 'Failed to reset operator'
            }).forWire());
        }
    });

    /**
     * POST /api/internal/operators/user/:userId/reauth
     */
    router.post('/user/:userId/reauth', requireInternalOrigin, async (req, res, next) => {
        const { userId } = req.params;

        logger.info('[INTERNAL-HTTP] g8e-pod operator reauth requested', { user_id: userId });

        try {
            const result = await g8eNodeOperatorService.relaunchG8ENodeOperatorForUser(userId);

            if (!result.success) {
                logger.warn('[INTERNAL-HTTP] g8e-pod operator reauth failed', {
                    user_id: userId,
                    error: result.error,
                });
                return res.status(404).json(new ErrorResponse({ error: result.error }).forWire());
            }

            logger.info('[INTERNAL-HTTP] g8e-pod operator reauth completed', {
                user_id: userId,
                operator_id: result.operator_id,
            });

            return res.json({ success: true, user_id: userId, operator_id: result.operator_id });
        } catch (error) {
            logger.error('[INTERNAL-HTTP] g8e-pod operator reauth error', {
                error: error.message,
                user_id: userId,
            });
            return res.status(500).json(new ErrorResponse({ error: error.message || 'Reauth failed' }).forWire());
        }
    });

    /**
     * GET /api/internal/operators/user/:userId
     */
    router.get('/user/:userId', requireInternalOrigin, async (req, res, next) => {
        try {
            const { userId } = req.params;

            logger.info('[INTERNAL-HTTP] Listing operators for user', { userId });

            const { operators, totalCount, activeCount } = await operatorService.getUserOperators(userId);

            const clientOperators = operators.map((op) => {
                const s = op.status ?? OperatorStatus.OFFLINE;
                const base = op instanceof OperatorDocument ? op.forClient() : op;
                return { ...base, status_display: s, status_class: s === OperatorStatus.OFFLINE ? 'inactive' : s.toLowerCase() };
            });

            logger.info('[INTERNAL-HTTP] Operators listed for user', {
                userId,
                totalCount,
                activeCount
            });

            return res.json(new OperatorListResponse({
                success: true,
                data: clientOperators,
                total_count: totalCount,
                active_count: activeCount
            }).forWire());
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to list operators for user', {
                error: error.message,
                userId: req.params.userId
            });

            return res.status(500).json(new ErrorResponse({
                error: error.message || 'Failed to list operators'
            }).forWire());
        }
    });

    /**
     * POST /api/internal/operators/user/:userId/initialize-slots
     */
    router.post('/user/:userId/initialize-slots', requireInternalOrigin, async (req, res, next) => {
        try {
            const { userId } = req.params;
            const { organization_id } = req.body;

            logger.info('[INTERNAL-HTTP] Ensuring operator slots for user', {
                userId,
                organization_id
            });

            const slotIds = await operatorService.initializeOperatorSlots(
                userId,
                organization_id || userId
            );

            return res.json(new OperatorSlotsResponse({
                success: true,
                operator_ids: slotIds,
                count: slotIds.length
            }).forWire());
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to ensure operator slots', {
                error: error.message,
                userId: req.params.userId
            });

            return res.status(500).json(new ErrorResponse({
                error: error.message || 'Failed to ensure operator slots'
            }).forWire());
        }
    });

    /**
     * GET /api/internal/operators/:operatorId/status
     */
    router.get('/:operatorId/status', requireInternalOrigin, async (req, res, next) => {
        try {
            const { operatorId } = req.params;

            // Direct DB/Cache lookup for the operator document.
            // This is used by VSE for bootstrap/validation.
            const operator = await operatorService.getOperator(operatorId);

            if (!operator) {
                return res.status(404).json(new ErrorResponse({
                    error: DeviceLinkError.OPERATOR_NOT_FOUND,
                }).forWire());
            }

            logger.info('[INTERNAL-HTTP] Operator document retrieved for status check', {
                operator_id: operatorId,
                status: operator.status
            });

            return res.json((operator.forWire ? operator.forWire() : operator));
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to get Operator status', {
                error: error.message,
                operator_id: req.params.operatorId
            });

            return res.status(500).json(new ErrorResponse({
                error: error.message || 'Failed to get Operator status'
            }).forWire());
        }
    });

    /**
     * GET /api/internal/operators/:operatorId
     */
    router.get('/:operatorId', requireInternalOrigin, async (req, res, next) => {
        try {
            const { operatorId } = req.params;

            const operator = await operatorService.getOperator(operatorId);

            if (!operator) {
                return res.status(404).json(new ErrorResponse({
                    error: DeviceLinkError.OPERATOR_NOT_FOUND,
                }).forWire());
            }

            return res.json((operator instanceof OperatorDocument ? operator.forWire() : operator));
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to get operator', {
                error: error.message,
                operator_id: req.params.operatorId
            });
            return res.status(500).json(new ErrorResponse({
                error: 'Failed to retrieve operator',
            }).forWire());
        }
    });

    /**
     * GET /api/internal/operators/:operatorId/with-session-context
     */
    router.get('/:operatorId/with-session-context', requireInternalOrigin, async (req, res, next) => {
        try {
            const { operatorId } = req.params;

            const operatorWithContext = await operatorService.getOperatorWithSessionContext(operatorId);

            if (!operatorWithContext) {
                return res.status(404).json(new ErrorResponse({
                    error: DeviceLinkError.OPERATOR_NOT_FOUND,
                }).forWire());
            }

            return res.json((operatorWithContext instanceof OperatorWithSessionContext
                ? operatorWithContext.forWire()
                : operatorWithContext));
        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to get Operator with session context', {
                error: error.message,
                operator_id: req.params.operatorId
            });
            return res.status(500).json(new ErrorResponse({
                error: 'Failed to retrieve Operator with session context',
            }).forWire());
        }
    });

    /**
     * POST /api/internal/operators/:operatorId/grant-intent
     */
    router.post('/:operatorId/grant-intent', requireInternalOrigin, async (req, res, next) => {
        try {
            const { operatorId } = req.params;
            const { intent } = IntentRequest.parse(req.body);

            logger.info('[INTERNAL-HTTP] Granting intent to operator', {
                operator_id: operatorId,
                intent
            });

            const result = await operatorService.grantIntent(operatorId, intent);

            if (!result.success) {
                return res.status(400).json(new ErrorResponse({
                    error: result.error || 'Failed to grant intent'
                }).forWire());
            }

            return res.json({
                success: true,
                operator_id: operatorId,
                granted_intents: result.granted_intents
            });

        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to grant intent', {
                error: error.message,
                operator_id: req.params.operatorId
            });
            return res.status(500).json(new ErrorResponse({
                error: error.message || 'Failed to grant intent'
            }).forWire());
        }
    });

    /**
     * POST /api/internal/operators/:operatorId/revoke-intent
     */
    router.post('/:operatorId/revoke-intent', requireInternalOrigin, async (req, res, next) => {
        try {
            const { operatorId } = req.params;
            const { intent } = IntentRequest.parse(req.body);

            logger.info('[INTERNAL-HTTP] Revoking intent from operator', {
                operator_id: operatorId,
                intent
            });

            const result = await operatorService.revokeIntent(operatorId, intent);

            if (!result.success) {
                return res.status(400).json(new ErrorResponse({
                    error: result.error || 'Failed to revoke intent'
                }).forWire());
            }

            return res.json({
                success: true,
                operator_id: operatorId,
                granted_intents: result.granted_intents
            });

        } catch (error) {
            logger.error('[INTERNAL-HTTP] Failed to revoke intent', {
                error: error.message,
                operator_id: req.params.operatorId
            });
            return res.status(500).json(new ErrorResponse({
                error: error.message || 'Failed to revoke intent'
            }).forWire());
        }
    });

    return router;
}
