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
import {
    BindOperatorsResponse,
    UnbindOperatorsResponse,
    ErrorResponse,
} from '../../models/response_models.js';
import { BindOperatorsRequest, UnbindOperatorsRequest } from '../../models/request_models.js';
import { logger } from '../../utils/logger.js';
import { OperatorPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authMiddleware - Auth middleware object
 */
export function createBindOperatorsRouter({
    services,
    authMiddleware
}) {
    const { bindOperatorsService } = services;
    const { requireAuth } = authMiddleware;
    const router = express.Router();

    router.post(OperatorPaths.BIND, requireAuth, async (req, res, next) => {
        try {
            const operatorIds = req.body.operator_ids || (req.body.operator_id ? [req.body.operator_id] : []);
            const bindReq = BindOperatorsRequest.parse({
                ...req.body,
                operator_ids: operatorIds,
                web_session_id: req.webSessionId,
                user_id: req.userId
            });

            const result = await bindOperatorsService.bindOperators(bindReq);

            return res.status(result.statusCode).json(new BindOperatorsResponse(result).forClient());
        } catch (error) {
            logger.error('[OPERATOR-BIND] Failed to bind operator', { error: error.message, operator_id: req.body?.operator_id });
            return res.status(500).json(new ErrorResponse({ error: 'Failed to bind operator' }).forClient());
        }
    });

    router.post(OperatorPaths.BIND_ALL, requireAuth, async (req, res, next) => {
        try {
            const bindReq = BindOperatorsRequest.parse({
                ...req.body,
                web_session_id: req.webSessionId,
                user_id: req.userId
            });

            const result = await bindOperatorsService.bindOperators(bindReq);

            return res.status(result.statusCode).json(new BindOperatorsResponse(result).forClient());
        } catch (error) {
            logger.error('[OPERATOR-BIND-ALL] Failed to bind operators', { error: error.message });
            return res.status(500).json(new ErrorResponse({ error: 'Failed to bind operators' }).forClient());
        }
    });

    router.post(OperatorPaths.UNBIND, requireAuth, async (req, res, next) => {
        try {
            const operatorIds = req.body.operator_ids || (req.body.operator_id ? [req.body.operator_id] : []);
            const unbindReq = UnbindOperatorsRequest.parse({
                ...req.body,
                operator_ids: operatorIds,
                web_session_id: req.webSessionId,
                user_id: req.userId
            });

            const result = await bindOperatorsService.unbindOperators(unbindReq);

            return res.status(result.statusCode).json(new UnbindOperatorsResponse(result).forClient());
        } catch (error) {
            logger.error('[OPERATOR-UNBIND] Failed to unbind operator', { error: error.message });
            return res.status(500).json(new ErrorResponse({ error: 'Failed to unbind operator' }).forClient());
        }
    });

    router.post(OperatorPaths.UNBIND_ALL, requireAuth, async (req, res, next) => {
        try {
            const operatorIds = req.body.operator_ids || [];
            const unbindReq = UnbindOperatorsRequest.parse({
                ...req.body,
                operator_ids: operatorIds,
                web_session_id: req.webSessionId,
                user_id: req.userId
            });

            const result = await bindOperatorsService.unbindOperators(unbindReq);

            return res.status(result.statusCode).json(new UnbindOperatorsResponse(result).forClient());
        } catch (error) {
            logger.error('[OPERATOR-UNBIND-ALL] Failed to unbind operators', { error: error.message });
            return res.status(500).json(new ErrorResponse({ error: 'Failed to unbind operators' }).forClient());
        }
    });

    return router;
}
