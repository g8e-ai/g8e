// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import express from 'express';
import { ErrorResponse, OperatorApiKeyResponse, OperatorRefreshKeyResponse } from '../../models/response_models.js';
import { logger } from '../../utils/logger.js';
import { OperatorPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.operatorService - OperatorDataService instance
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createOperatorApiKeyRouter({
    operatorService,
    authMiddleware,
    authorizationMiddleware
}) {
    const { requireAuth } = authMiddleware;
    const { requireOperatorOwnership } = authorizationMiddleware;
    const router = express.Router();

    router.get(OperatorPaths.API_KEY, requireAuth, requireOperatorOwnership, async (req, res, next) => {
        try {
            const operator = req.operator;

            const apiKey = operator.api_key ?? null;
            if (!apiKey) {
                return res.status(404).json(new ErrorResponse({
                    error: 'No API key found for this operator'
                }).forClient());
            }

            res.json(new OperatorApiKeyResponse({
                success: true,
                operator_id: operator.operator_id,
                api_key: apiKey
            }).forClient());

        } catch (error) {
            logger.error('[OPERATOR-API-KEY] Failed to fetch API key', {
                error: error.message,
                operator_id: req.params.operatorId
            });
            res.status(500).json(new ErrorResponse({
                error: 'Failed to fetch API key'
            }).forClient());
        }
    });

    router.post(OperatorPaths.REFRESH_API_KEY, requireAuth, async (req, res, next) => {
        try {
            const { operatorId } = req.params;

            if (!operatorId) {
                return res.status(400).json(new ErrorResponse({
                    error: 'operator_id is required'
                }).forClient());
            }

            const userId = req.userId;

            const result = await operatorService.refreshOperatorApiKey(operatorId, userId);

            if (!result.success) {
                return res.status(result.message.includes('Unauthorized') ? 403 : 400).json(new ErrorResponse({
                    error: result.message
                }).forClient());
            }

            logger.info('[OPERATOR-REFRESH-KEY] API key refreshed - old Operator terminated, new created', {
                old_operator_id: operatorId,
                new_operator_id: result.new_operator_id,
                slot_number: result.slot_number,
                user_id: userId
            });

            res.json(new OperatorRefreshKeyResponse({
                success: true,
                message: result.message,
                old_operator_id: operatorId,
                new_operator_id: result.new_operator_id,
                slot_number: result.slot_number,
                new_api_key: result.new_api_key
            }).forClient());

        } catch (error) {
            logger.error('[OPERATOR-REFRESH-KEY] Failed to refresh API key', {
                error: error.message,
                operator_id: req.params.operatorId
            });
            res.status(500).json(new ErrorResponse({
                error: 'Failed to refresh API key'
            }).forClient());
        }
    });

    return router;
}
