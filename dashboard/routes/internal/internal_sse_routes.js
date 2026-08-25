// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * g8ed Internal SSE Routes
 * 
 * Internal HTTP endpoints for SSE event delivery from VSE.
 * NOT exposed via public routes - only accessible from internal services.
 */

import express from 'express';
import { SSEPushRequest } from '../../models/request_models.js';
import { VSEPassthroughEvent } from '../../models/sse_models.js';
import { OperatorListUpdatedEvent } from '../../models/operator_model.js';
import { ErrorResponse, SimpleSuccessResponse } from '../../models/response_models.js';
import { logger } from '../../utils/logger.js';
import { redactWebSessionId } from '../../utils/security.js';
import { EventType } from '../../constants/events.js';

/**
 * @param {Object} options
 * @param {Object} options.sseService - SSEService instance
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 * @param {Object} options.operatorService - OperatorService instance
 */
export function createInternalSSERouter({ sseService, authorizationMiddleware, operatorService }) {
    const { requireInternalOrigin } = authorizationMiddleware;
    const router = express.Router();

    /**
     * Normalize citation_num values in a CHAT_CITATIONS_READY event to sequential 1-based integers.
     * VSE emits non-sequential citation_num values (e.g. 10, 20, 30). The frontend expects
     * sequential 1-based values. Returns a new event object — does not mutate the input.
     */
    function normalizeCitationNums(event) {
        const sources = event?.grounding_metadata?.sources;
        if (!Array.isArray(sources) || sources.length === 0) {
            return event;
        }
        return {
            ...event,
            grounding_metadata: {
                ...event.grounding_metadata,
                sources: sources.map((source, index) => ({ ...source, citation_num: index + 1 })),
            },
        };
    }

    /**
     * POST /api/internal/sse/push
     */
    router.post('/push', requireInternalOrigin, async (req, res, next) => {
        try {
            const pushReq = SSEPushRequest.parse(req.body);

            logger.info('[INTERNAL-HTTP] SSE push request received', {
                webSessionId: redactWebSessionId(pushReq.web_session_id),
                eventType: pushReq.event.type
            });

            logger.info(`[SESSION TRACE] g8ed received SSE push - web_session_id=${redactWebSessionId(pushReq.web_session_id)}, event_type=${pushReq.event.type}`);

            const targetWebSessionId = pushReq.web_session_id;

            let finalEvent;
            
            // Special handling for OPERATOR_PANEL_LIST_UPDATED: replace VSE's single-operator payload
            // with g8ed's full operator list for the frontend
            if (pushReq.event.type === EventType.OPERATOR_PANEL_LIST_UPDATED) {
                try {
                    const operatorList = await operatorService.getUserOperators(pushReq.user_id);
                    finalEvent = new OperatorListUpdatedEvent(operatorList);
                    logger.info('[INTERNAL-HTTP] Replaced VSE operator payload with full operator list', {
                        webSessionId: redactWebSessionId(pushReq.web_session_id),
                        operatorCount: operatorList.operators?.length || 0
                    });
                } catch (err) {
                    logger.error('[INTERNAL-HTTP] Failed to get operator list for OPERATOR_PANEL_LIST_UPDATED', {
                        webSessionId: redactWebSessionId(pushReq.web_session_id),
                        error: err.message
                    });
                    // Fallback to original event if we can't get the operator list
                    finalEvent = new VSEPassthroughEvent({ _payload: normalizedEvent });
                }
            } else {
                const normalizedEvent = pushReq.event.type === EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED
                    ? normalizeCitationNums(pushReq.event)
                    : pushReq.event;
                finalEvent = new VSEPassthroughEvent({ _payload: normalizedEvent });
            }

            // Forward to SSE service for delivery
            const published = await sseService.publishEvent(targetWebSessionId, finalEvent);

            if (published) {
                logger.info('[INTERNAL-HTTP] SSE event delivered via HTTP', {
                    webSessionId: redactWebSessionId(pushReq.web_session_id),
                    eventType: pushReq.event.type
                });
                
                return res.json(new SimpleSuccessResponse({
                    success: true,
                    message: 'Event delivered'
                }).forWire());
            } else {
                logger.warn('[INTERNAL-HTTP] Failed to publish SSE event', {
                    webSessionId: redactWebSessionId(pushReq.web_session_id)
                });
                return res.status(500).json(new ErrorResponse({
                    error: 'Failed to publish event'
                }).forWire());
            }

        } catch (error) {
            logger.error('[INTERNAL-HTTP] SSE push failed', {
                error: error.message
            });
            return res.status(500).json(new ErrorResponse({
                error: error.message
            }).forWire());
        }
    });

    return router;
}
