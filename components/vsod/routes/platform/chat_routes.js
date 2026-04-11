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
import { now } from '../../models/base.js';
import { ChatHealthResponse, ChatMessageResponse, InvestigationListResponse, ErrorResponse, ChatActionResponse } from '../../models/response_models.js';
import { ChatMessageRequest, InvestigationQueryRequest, StopAIRequest } from '../../models/request_models.js';
import { logger } from '../../utils/logger.js';
import { redactWebSessionId } from '../../utils/security.js';
import { SystemHealth } from '../../constants/ai.js';
import { ChatPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.internalHttpClient - Internal HTTP client
 * @param {Object} options.bindingService - Binding service
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createChatRouter({
    services,
    authMiddleware,
    authorizationMiddleware,
    rateLimiters
}) {
    const { internalHttpClient, bindingService } = services;
    const { requireAuth, requireOperatorBinding } = authMiddleware;
    const { requireInternalOrigin } = authorizationMiddleware;
    const { chatRateLimiter, apiRateLimiter } = rateLimiters;
    const router = express.Router();

    router.post(ChatPaths.SEND, requireAuth, requireOperatorBinding, chatRateLimiter, async (req, res, next) => {
        try {
            const chatRequest = ChatMessageRequest.parse({
                ...req.body,
                web_session_id: req.webSessionId,
                user_id: req.userId
            });

            logger.info('[HTTP] Sending chat message to VSE', {
                caseId: req.vsoContext.case_id,
                investigationId: req.vsoContext.investigation_id,
                webSessionId: redactWebSessionId(req.vsoContext.web_session_id),
                messageLength: chatRequest.message.length
            });

            const response = await internalHttpClient.sendChatMessage(chatRequest.forWire(), req.vsoContext);

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

    router.get(ChatPaths.INVESTIGATIONS, requireAuth, requireOperatorBinding, apiRateLimiter, async (req, res, next) => {
        try {
            const queryRequest = InvestigationQueryRequest.parse(req.query);

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
            const investigations = await internalHttpClient.queryInvestigations(queryParams, req.vsoContext);

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

    router.get(ChatPaths.INVESTIGATION, requireAuth, requireOperatorBinding, apiRateLimiter, async (req, res, next) => {
        try {
            const investigation = await internalHttpClient.getInvestigation(req.params.investigationId, req.vsoContext);

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

    router.post(ChatPaths.STOP, requireAuth, requireOperatorBinding, apiRateLimiter, async (req, res, next) => {
        try {
            const stopRequest = StopAIRequest.parse({
                ...req.body,
                web_session_id: req.webSessionId
            });

            logger.info('[HTTP] Stopping AI processing via VSE', {
                investigationId: stopRequest.investigation_id,
                reason: stopRequest.reason,
                webSessionId: redactWebSessionId(req.webSessionId)
            });

            const response = await internalHttpClient.stopAIProcessing(stopRequest.forWire(), req.vsoContext);

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

    router.delete(ChatPaths.CASES, requireAuth, requireOperatorBinding, apiRateLimiter, async (req, res, next) => {
        try {
            logger.info('[HTTP] Deleting case via VSE', {
                caseId: req.params.caseId,
                userId: req.userId,
                webSessionId: redactWebSessionId(req.webSessionId)
            });

            await internalHttpClient.deleteCase(req.params.caseId, req.vsoContext);

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
                service: 'vsod-http-routes',
                status: SystemHealth.HEALTHY,
                internal_services: healthStatus,
                timestamp: now()
            }).forClient());

        } catch (error) {
            res.status(500).json(new ChatHealthResponse({
                service: 'vsod-http-routes',
                status: SystemHealth.UNHEALTHY,
                error: error.message,
                timestamp: now()
            }).forClient());
        }
    });

    return router;
}
