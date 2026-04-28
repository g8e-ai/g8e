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
import { ErrorResponse, SSEPushResponse } from '../../models/response_models.js';
import { logger } from '../../utils/logger.js';
import { redactWebSessionId } from '../../utils/security.js';

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
     * POST /api/internal/sse/push
     *
     * Pure SSE transport. g8ee is the single writer for the operator document
     * (see OperatorDataService.update_operator_heartbeat), so this route MUST
     * NOT mutate the Operators collection on heartbeat pass-through. Doing so
     * would race with g8ee's authoritative write and clobber denormalized
     * fields (current_hostname, heartbeat_history) that g8ee maintains.
     */
    router.post('/push', requireInternalOrigin, async (req, res, next) => {
        try {
            const pushReq = SSEPushRequest.parse(req.body);

            const isHeartbeat = pushReq.event.type === 'g8e.v1.operator.heartbeat.received';
            if (isHeartbeat) {
                logger.info('[SSE-ROUTING] [HEARTBEAT] Heartbeat SSE push received from g8ee', {
                    webSessionId: redactWebSessionId(pushReq.web_session_id),
                    userId: pushReq.user_id,
                    eventType: pushReq.event.type,
                    operator_id: pushReq.event.payload?.operator_id,
                    operator_status: pushReq.event.payload?.status,
                    has_metrics: !!pushReq.event.payload?.metrics
                });
            } else {
                logger.info('[SSE-ROUTING] SSE push request received', {
                    webSessionId: redactWebSessionId(pushReq.web_session_id),
                    userId: pushReq.user_id,
                    eventType: pushReq.event.type
                });
            }

            logger.info(`[SESSION TRACE] g8ed received SSE push - web_session_id=${redactWebSessionId(pushReq.web_session_id)}, event_type=${pushReq.event.type}`);

            if (pushReq.web_session_id) {
                // SessionEvent: targeted delivery to a specific web session.
                if (isHeartbeat) {
                    logger.info('[SSE-ROUTING] [HEARTBEAT] Routing heartbeat to targeted web session', {
                        webSessionId: redactWebSessionId(pushReq.web_session_id),
                        operator_id: pushReq.event.payload?.operator_id
                    });
                }
                const delivered = await sseService.publishEvent(pushReq.web_session_id, pushReq.event, (status) => {
                    if (status.delivered) {
                        if (isHeartbeat) {
                            logger.info('[SSE-ROUTING] [HEARTBEAT] Heartbeat delivered to web session', {
                                webSessionId: redactWebSessionId(pushReq.web_session_id),
                                operator_id: pushReq.event.payload?.operator_id
                            });
                        } else {
                            logger.info('[INTERNAL-HTTP] SSE event delivered via callback', {
                                webSessionId: redactWebSessionId(pushReq.web_session_id),
                                eventType: pushReq.event.type
                            });
                        }
                    } else {
                        if (isHeartbeat) {
                            logger.warn('[SSE-ROUTING] [HEARTBEAT] Heartbeat delivery failed via callback', {
                                webSessionId: redactWebSessionId(pushReq.web_session_id),
                                operator_id: pushReq.event.payload?.operator_id
                            });
                        } else {
                            logger.warn('[INTERNAL-HTTP] SSE event delivery failed via callback', {
                                webSessionId: redactWebSessionId(pushReq.web_session_id),
                                eventType: pushReq.event.type
                            });
                        }
                    }
                });

                if (!delivered) {
                    // Targeted session not locally connected. A failed targeted
                    // push IS an error (the caller expected a specific session).
                    if (isHeartbeat) {
                        logger.warn('[SSE-ROUTING] [HEARTBEAT] Failed to publish heartbeat to targeted session (session not connected)', {
                            webSessionId: redactWebSessionId(pushReq.web_session_id),
                            userId: pushReq.user_id,
                            operator_id: pushReq.event.payload?.operator_id
                        });
                    } else {
                        logger.warn('[INTERNAL-HTTP] Failed to publish SSE event to targeted session', {
                            webSessionId: redactWebSessionId(pushReq.web_session_id),
                            userId: pushReq.user_id
                        });
                    }
                    return res.status(500).json(new ErrorResponse({
                        error: 'Failed to publish event'
                    }).forWire());
                }

                if (isHeartbeat) {
                    logger.info('[SSE-ROUTING] [HEARTBEAT] Heartbeat delivered via HTTP (targeted session)', {
                        webSessionId: redactWebSessionId(pushReq.web_session_id),
                        operator_id: pushReq.event.payload?.operator_id
                    });
                } else {
                    logger.info('[INTERNAL-HTTP] SSE event delivered via HTTP (targeted)', {
                        webSessionId: redactWebSessionId(pushReq.web_session_id),
                        eventType: pushReq.event.type
                    });
                }
                return res.json(new SSEPushResponse({ success: true, delivered: 1 }).forWire());
            }

            // BackgroundEvent: fan out to all locally connected sessions for this user.
            // SSEService maintains an in-memory userId -> sessions index, so this is
            // O(k) in the number of local sessions for that user with no cache lookup.
            // A zero count is the documented outcome when the user has no connected
            // sessions and is NOT an error - collapsing it into a 500 would mask real
            // g8ed outages when BackgroundEvent fan-out is routine.
            if (isHeartbeat) {
                logger.info('[SSE-ROUTING] [HEARTBEAT] Fanning out heartbeat to all user sessions', {
                    userId: pushReq.user_id,
                    operator_id: pushReq.event.payload?.operator_id
                });
            }
            const successCount = await sseService.publishToUser(pushReq.user_id, pushReq.event);
            if (isHeartbeat) {
                logger.info('[SSE-ROUTING] [HEARTBEAT] Heartbeat fanned out to user sessions', {
                    userId: pushReq.user_id,
                    successCount,
                    operator_id: pushReq.event.payload?.operator_id
                });
            } else {
                logger.info('[INTERNAL-HTTP] SSE event fanned out to user sessions', {
                    userId: pushReq.user_id,
                    successCount,
                    eventType: pushReq.event.type
                });
            }
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
