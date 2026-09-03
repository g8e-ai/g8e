// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import express from 'express';
import { ErrorResponse } from '../../models/response_models.js';
import { VSOHttpContext } from '../../models/request_models.js';
import { logger } from '../../utils/logger.js';
import { OperatorPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.operatorService - OperatorDataService instance
 * @param {Object} options.sseService - SSEService instance
 * @param {Object} options.BindOperatorsService - BindOperatorsService instance
 * @param {Object} options.internalHttpClient - InternalHttpClient instance
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createOperatorStatusRouter({
    operatorService,
    authMiddleware,
    authorizationMiddleware
}) {
    const { requireAuth } = authMiddleware;
    const { requireOperatorOwnership } = authorizationMiddleware;
    const router = express.Router();

    router.get(OperatorPaths.DETAILS, requireAuth, requireOperatorOwnership, async (req, res, next) => {
        try {
            const operator = req.operator;
            const status = operator.status;
            const enhancedOperator = {
                ...operator.forClient(),
                status_display: status,
                status_class:   status,
            };

            return res.json(enhancedOperator);

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

            logger.info('[OPERATOR-STOP] Relaying stop command to VSE', {
                operator_id: operatorId,
                user_id: userId,
                operator_session_id: operatorSessionId.substring(0, 12) + '...'
            });

            const vsoContext = VSOHttpContext.parse({
                web_session_id: webSessionId,
                user_id: userId,
                operator_id: operatorId,
                operator_session_id: operatorSessionId,
            });

            try {
                await operatorService.relayStopCommandToVse(vsoContext);

                logger.info('[OPERATOR-STOP] Stop command relayed successfully to VSE', {
                    operator_id: operatorId
                });

                res.json({
                    success: true,
                    message: 'Stop command relayed to orchestrator'
                });

            } catch (vseError) {
                logger.error('[OPERATOR-STOP] Failed to relay stop command to VSE', {
                    error: vseError.message,
                    operator_id: operatorId
                });
                throw new Error(`Failed to relay stop command: ${vseError.message}`, { cause: vseError });
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
