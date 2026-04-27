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
import { ErrorResponse, OperatorApiKeyResponse } from '../../models/response_models.js';
import { logger } from '../../utils/logger.js';
import { OperatorPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createOperatorApiKeyRouter({
    services,
    authMiddleware,
    authorizationMiddleware
}) {
    const { operatorService } = services;
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
                id: operator.id,
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
            const webSessionId = req.webSessionId;

            const result = await operatorService.refreshOperatorApiKey(operatorId, userId, webSessionId, null);

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

            res.json(result.forClient());

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
