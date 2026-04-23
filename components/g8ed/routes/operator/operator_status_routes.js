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
import { ErrorResponse } from '../../models/response_models.js';
import { OperatorDocument, OperatorSlot } from '../../models/operator_model.js';
import { logger } from '../../utils/logger.js';
import { sessionIdTag } from '../../utils/session_log.js';
import { OperatorPaths } from '../../constants/api_paths.js';
import { OperatorStatus } from '../../constants/operator.js';

// NOTE: Frontend operator selection mechanism is currently disabled
// The frontend no longer allows users to select a single bound operator for metrics display
// This was disabled because the UX for selecting one operator out of a list of bound operators needs improvement
// To re-enable when UX is better established:
// 1. Uncomment the click handler in operator-list-mixin.js that calls _selectMetricsOperator
// 2. Uncomment the _applyDefaultMetricsSelection call in operator-list-mixin.js
// 3. Uncomment the automatic selection on bind in operator-bind-mixin.js
// The backend here supports operator selection via the selectedMetricsOperatorId pattern
// See: operator-panel.js (heartbeat and status_updated handlers) and operator-list-mixin.js (_selectMetricsOperator)

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createOperatorStatusRouter({
    services,
    authMiddleware,
    authorizationMiddleware
}) {
    const { operatorService, g8eNodeOperatorService, internalHttpClient } = services;
    const { requireAuth } = authMiddleware;
    const { requireOperatorOwnership } = authorizationMiddleware;
    const router = express.Router();

    router.post(OperatorPaths.G8E_GATEWAY_REAUTH, requireAuth, async (req, res, next) => {
        const userId = req.userId;

        logger.info('[g8ep-REAUTH] g8ep operator reauth requested', { user_id: userId });

        try {
            const result = await g8eNodeOperatorService.relaunchG8ENodeOperatorForUser(userId);

            if (!result.success) {
                logger.warn('[g8ep-REAUTH] g8ep operator reauth failed', {
                    user_id: userId,
                    error: result.error,
                });
                return res.status(404).json(new ErrorResponse({ error: result.error }).forClient());
            }

            logger.info('[g8ep-REAUTH] g8ep operator reauth completed', {
                user_id: userId,
                operator_id: result.operator_id,
            });

            return res.json({
                success: true,
                message: 'g8ep reauth initiated',
                id: result.id
            });

        } catch (error) {
            logger.error('[g8ep-REAUTH] g8ep operator reauth error', {
                user_id: userId,
                error: error.message,
            });
            return res.status(500).json(new ErrorResponse({ error: error.message || 'Reauth failed' }).forClient());
        }
    });

    router.get(OperatorPaths.DETAILS, requireAuth, requireOperatorOwnership, async (req, res, next) => {
        try {
            const operator = req.operator;
            const operatorSlot = OperatorSlot.fromOperator(operator);

            return res.json(operatorSlot.forClient());

        } catch (error) {
            logger.error('[OPERATOR-STATUS] Failed to get Operator details', {
                error: error.message,
                operator_id: req.params.operatorId
            });
            res.status(500).json(new ErrorResponse({
                error: 'Failed to retrieve Operator details'
            }).forClient());
        }
    });

    router.post(OperatorPaths.STOP, requireAuth, requireOperatorOwnership, async (req, res, next) => {
        try {
            const { operatorId } = req.params;
            const webSessionId = req.webSessionId;
            const userId = req.userId;

            const operator = req.operator;
            const operatorSessionId = operator.operator_session_id;

            if (!operatorSessionId) {
                return res.status(400).json(new ErrorResponse({
                    error: 'Operator has no active session'
                }).forClient());
            }

            logger.info('[OPERATOR-STOP] Relaying stop command to g8ee', {
                operator_id: operatorId,
                user_id: userId,
                operator_session_id_tag: sessionIdTag(operatorSessionId)
            });

            try {
                await operatorService.relayStopCommandToG8ee(req.g8eContext);

                logger.info('[OPERATOR-STOP] Stop command relayed successfully to g8ee', {
                    operator_id: operatorId
                });

                res.json({
                    success: true,
                    message: 'Stop command relayed to orchestrator'
                });

            } catch (g8eeError) {
                logger.error('[OPERATOR-STOP] Failed to relay stop command to g8ee', {
                    error: g8eeError.message,
                    operator_id: operatorId
                });
                throw new Error(`Failed to relay stop command: ${g8eeError.message}`);
            }

        } catch (error) {
            logger.error('[OPERATOR-STOP] Failed to process stop request', {
                error: error.message,
                operator_id: req.params?.operatorId
            });
            res.status(500).json(new ErrorResponse({
                error: 'Failed to process stop request'
            }).forClient());
        }
    });

    return router;
}
