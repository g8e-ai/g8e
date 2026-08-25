// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.


import express from 'express';
import { now } from '../../models/base.js';
import { ChatHealthResponse, ChatMessageResponse, InvestigationListResponse, ErrorResponse, ChatActionResponse } from '../../models/response_models.js';
import { ChatMessageRequest, InvestigationQueryRequest, VSOHttpContext, StopAIRequest } from '../../models/request_models.js';
import { logger } from '../../utils/logger.js';
import { redactWebSessionId } from '../../utils/security.js';
import { SystemHealth } from '../../constants/ai.js';
import { ChatPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.internalHttpClient - InternalHttpClient instance
 * @param {Object} options.bindingService - BoundSessionsService instance
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createChatRouter({
    internalHttpClient,
    bindingService,
    authMiddleware,
    authorizationMiddleware,
    rateLimiters
}) {
    const { requireAuth } = authMiddleware;
    const { requireInternalOrigin } = authorizationMiddleware;
    const { chatRateLimiter, apiRateLimiter } = rateLimiters;
    const router = express.Router();

    router.post(ChatPaths.SEND, requireAuth, chatRateLimiter, async (req, res, next) => {
        try {
            const chatRequest = ChatMessageRequest.parse({
                ...req.body,
                web_session_id: req.webSessionId,
                user_id: req.userId
            });

            const vsoContext = VSOHttpContext.parse({
                web_session_id: req.webSessionId,
                user_id: req.userId,
                organization_id: req.session.organization_id || req.session.user_data?.organization_id || null,
                case_id: chatRequest.case_id,
                investigation_id: chatRequest.investigation_id,
                bound_operators: await bindingService.resolveBoundOperators(req.webSessionId),
                execution_id: `req_chat_send_${now().getTime()}`
            });

            logger.info('[HTTP] Sending chat message to VSE', {
                caseId: chatRequest.case_id,
                investigationId: chatRequest.investigation_id,
                webSessionId: redactWebSessionId(vsoContext.web_session_id),
                messageLength: chatRequest.message.length
            });

            const response = await internalHttpClient.sendChatMessage(chatRequest.forWire(), vsoContext);

            res.json(new ChatMessageResponse({
                success: response.success,
                data: response.data || response,
                error: response.error || null
            }).forClient());

        } catch (error) {
            logger.error('[HTTP] Chat send failed', { error: error.message });
            res.status(500).json(new ChatMessageResponse({
                success: false,
                error: error.message
            }).forClient());
        }
    });

    router.get(ChatPaths.INVESTIGATIONS, requireAuth, apiRateLimiter, async (req, res, next) => {
        try {
            const queryRequest = InvestigationQueryRequest.parse(req.query);

            const vsoContext = VSOHttpContext.parse({
                web_session_id: req.webSessionId,
                user_id: req.userId,
                organization_id: req.session.organization_id || req.session.user_data?.organization_id || null,
                case_id: queryRequest.case_id,
                investigation_id: queryRequest.investigation_id,
                bound_operators: await bindingService.resolveBoundOperators(req.webSessionId),
                execution_id: `req_investigations_${now().getTime()}`
            });

            logger.info('[HTTP] Querying investigations from VSE', {
                userId: req.userId,
                caseId: queryRequest.case_id,
                webSessionId: redactWebSessionId(req.webSessionId)
            });

            const queryParams = new URLSearchParams();
            Object.entries(queryRequest.forClient()).forEach(([key, value]) => {
                if (value !== null && value !== undefined) {
                    queryParams.append(key, value);
                }
            });
            const investigations = await internalHttpClient.queryInvestigations(queryParams, vsoContext);

            res.json(new InvestigationListResponse({
                success: true,
                investigations: Array.isArray(investigations) ? investigations : [],
                count: Array.isArray(investigations) ? investigations.length : 0
            }).forClient());

        } catch (error) {
            logger.error('[HTTP] Investigation query failed', { error: error.message });
            res.status(500).json(new ChatMessageResponse({
                success: false,
                error: error.message
            }).forClient());
        }
    });

    router.get(ChatPaths.INVESTIGATION, requireAuth, apiRateLimiter, async (req, res, next) => {
        try {
            const vsoContext = VSOHttpContext.parse({
                web_session_id: req.webSessionId,
                user_id: req.userId,
                organization_id: req.session.organization_id || req.session.user_data?.organization_id || null,
                investigation_id: req.params.investigationId,
                case_id: req.query.case_id || null,
                bound_operators: await bindingService.resolveBoundOperators(req.webSessionId),
                execution_id: `req_investigation_get_${now().getTime()}`
            });
            const investigation = await internalHttpClient.getInvestigation(req.params.investigationId, vsoContext);

            logger.info('[HTTP] Getting investigation from VSE', {
                investigationId: req.params.investigationId,
                webSessionId: redactWebSessionId(req.webSessionId)
            });

            res.json(new ChatMessageResponse({
                success: true,
                data: investigation
            }).forClient());

        } catch (error) {
            logger.error('[HTTP] Investigation get failed', { error: error.message });
            res.status(500).json(new ChatMessageResponse({
                success: false,
                error: error.message
            }).forClient());
        }
    });

    router.post(ChatPaths.STOP, requireAuth, apiRateLimiter, async (req, res, next) => {
        try {
            const stopRequest = StopAIRequest.parse({
                ...req.body,
                web_session_id: req.webSessionId
            });

            const vsoContext = VSOHttpContext.parse({
                web_session_id: req.webSessionId,
                user_id: req.userId,
                organization_id: req.session.organization_id || req.session.user_data?.organization_id || null,
                investigation_id: stopRequest.investigation_id,
                bound_operators: await bindingService.resolveBoundOperators(req.webSessionId),
                execution_id: `req_stop_ai_${now().getTime()}`
            });

            logger.info('[HTTP] Stopping AI processing via VSE', {
                investigationId: stopRequest.investigation_id,
                reason: stopRequest.reason,
                webSessionId: redactWebSessionId(req.webSessionId)
            });

            const response = await internalHttpClient.stopAIProcessing(stopRequest.forWire(), vsoContext);

            res.json(new ChatActionResponse({
                success: response.success,
                message: response.data?.message || 'AI processing stopped',
                data: {
                    investigation_id: stopRequest.investigation_id,
                    was_active: response.data?.was_active
                }
            }).forClient());

        } catch (error) {
            logger.error('[HTTP] Stop AI processing failed', { error: error.message });
            res.status(500).json(new ErrorResponse({
                error: error.message
            }).forClient());
        }
    });

    router.delete(ChatPaths.CASES, requireAuth, apiRateLimiter, async (req, res, next) => {
        try {
            const vsoContext = VSOHttpContext.parse({
                web_session_id: req.webSessionId,
                user_id: req.userId,
                organization_id: req.session.organization_id || req.session.user_data?.organization_id || null,
                case_id: req.params.caseId,
                bound_operators: await bindingService.resolveBoundOperators(req.webSessionId),
                execution_id: `req_case_delete_${now().getTime()}`
            });

            logger.info('[HTTP] Deleting case via VSE', {
                caseId: req.params.caseId,
                userId: req.userId,
                webSessionId: redactWebSessionId(req.webSessionId)
            });

            await internalHttpClient.deleteCase(req.params.caseId, vsoContext);

            res.status(204).send();

        } catch (error) {
            logger.error('[HTTP] Case deletion failed', { error: error.message });
            res.status(500).json(new ErrorResponse({
                error: error.message
            }).forClient());
        }
    });

    router.get(ChatPaths.HEALTH, requireInternalOrigin, async (req, res, next) => {
        try {
            const healthStatus = await internalHttpClient.healthCheck();

            res.json(new ChatHealthResponse({
                service: 'g8ed-http-routes',
                status: SystemHealth.HEALTHY,
                internal_services: healthStatus,
                timestamp: now()
            }).forClient());

        } catch (error) {
            res.status(500).json(new ChatHealthResponse({
                service: 'g8ed-http-routes',
                status: SystemHealth.UNHEALTHY,
                error: error.message,
                timestamp: now()
            }).forClient());
        }
    });

    return router;
}
