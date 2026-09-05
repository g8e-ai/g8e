// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import express from 'express';
import {
    BindOperatorsResponse,
    ErrorResponse,
    UnbindOperatorsResponse,
} from '../../models/response_models.js';
import { BindOperatorsRequest, UnbindOperatorsRequest } from '../../models/request_models.js';
import { logger } from '../../utils/logger.js';
import { OperatorPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.BindOperatorsService - BindOperatorsService instance
 * @param {Object} options.authMiddleware - Auth middleware object
 */
export function createBindOperatorsRouter({
    BindOperatorsService,
    authMiddleware
}) {
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

            const result = await BindOperatorsService.bindOperators(bindReq);

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

            const result = await BindOperatorsService.bindOperators(bindReq);

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

            const result = await BindOperatorsService.unbindOperators(unbindReq);

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

            const result = await BindOperatorsService.unbindOperators(unbindReq);

            return res.status(result.statusCode).json(new UnbindOperatorsResponse(result).forClient());
        } catch (error) {
            logger.error('[OPERATOR-UNBIND-ALL] Failed to unbind operators', { error: error.message });
            return res.status(500).json(new ErrorResponse({ error: 'Failed to unbind operators' }).forClient());
        }
    });

    return router;
}
