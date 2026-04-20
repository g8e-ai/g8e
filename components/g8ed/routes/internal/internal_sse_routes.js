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

/**
 * g8ed Internal SSE Routes
 * 
 * Internal HTTP endpoints for SSE event delivery from G8EE.
 * NOT exposed via public routes - only accessible from internal services.
 */

import express from 'express';
import { SSEPushRequest } from '../../models/request_models.js';
import { G8eePassthroughEvent } from '../../models/sse_models.js';
import { ErrorResponse, SSEPushResponse } from '../../models/response_models.js';
import { logger } from '../../utils/logger.js';
import { redactWebSessionId } from '../../utils/security.js';
import { EventType } from '../../constants/events.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createInternalSSERouter({ services, authorizationMiddleware }) {
    const { sseService } = services;
    const { requireInternalOrigin } = authorizationMiddleware;
    const router = express.Router();

    /**
     * Normalize citation_num values in a CHAT_CITATIONS_READY event to sequential 1-based integers.
     * g8ee emits non-sequential citation_num values (e.g. 10, 20, 30). The frontend expects
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
                userId: pushReq.user_id,
                eventType: pushReq.event.type
            });

            logger.info(`[SESSION TRACE] g8ed received SSE push - web_session_id=${redactWebSessionId(pushReq.web_session_id)}, event_type=${pushReq.event.type}`);

            const normalizedEvent = pushReq.event.type === EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED
                ? normalizeCitationNums(pushReq.event)
                : pushReq.event;
            const finalEvent = new G8eePassthroughEvent({ _payload: normalizedEvent });

            if (pushReq.web_session_id) {
                // SessionEvent: targeted delivery to a specific web session.
                const delivered = await sseService.publishEvent(pushReq.web_session_id, finalEvent, (status) => {
                    if (status.delivered) {
                        logger.info('[INTERNAL-HTTP] SSE event delivered via callback', {
                            webSessionId: redactWebSessionId(pushReq.web_session_id),
                            eventType: pushReq.event.type
                        });
                    } else {
                        logger.warn('[INTERNAL-HTTP] SSE event delivery failed via callback', {
                            webSessionId: redactWebSessionId(pushReq.web_session_id),
                            eventType: pushReq.event.type
                        });
                    }
                });

                if (!delivered) {
                    // Targeted session not locally connected. A failed targeted
                    // push IS an error (the caller expected a specific session).
                    logger.warn('[INTERNAL-HTTP] Failed to publish SSE event to targeted session', {
                        webSessionId: redactWebSessionId(pushReq.web_session_id),
                        userId: pushReq.user_id
                    });
                    return res.status(500).json(new ErrorResponse({
                        error: 'Failed to publish event'
                    }).forWire());
                }

                logger.info('[INTERNAL-HTTP] SSE event delivered via HTTP (targeted)', {
                    webSessionId: redactWebSessionId(pushReq.web_session_id),
                    eventType: pushReq.event.type
                });
                return res.json(new SSEPushResponse({ success: true, delivered: 1 }).forWire());
            }

            // BackgroundEvent: fan out to all locally connected sessions for this user.
            // SSEService maintains an in-memory userId -> sessions index, so this is
            // O(k) in the number of local sessions for that user with no cache lookup.
            // A zero count is the documented outcome when the user has no connected
            // sessions and is NOT an error - collapsing it into a 500 would mask real
            // g8ed outages when BackgroundEvent fan-out is routine.
            const successCount = await sseService.publishToUser(pushReq.user_id, finalEvent);
            logger.info('[INTERNAL-HTTP] SSE event fanned out to user sessions', {
                userId: pushReq.user_id,
                successCount,
                eventType: pushReq.event.type
            });
            return res.json(new SSEPushResponse({ success: true, delivered: successCount }).forWire());

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
