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
import { ConnectionEstablishedEvent, KeepaliveEvent, OperatorListData } from '../../models/sse_models.js';
import { logger } from '../../utils/logger.js';
import { redactWebSessionId } from '../../utils/security.js';
import { EventType, SSE_KEEPALIVE_INTERVAL_MS } from '../../constants/events.js';
import { SystemHealth } from '../../constants/ai.js';
import { SSEHealthResponse } from '../../models/response_models.js';
import { SSEPaths } from '../../constants/api_paths.js';
import { getOperatorService } from '../../services/initialization.js';

export function createSSERouter({
    services,
    authMiddleware,
    authorizationMiddleware,
    rateLimiters
}) {
    const { sseService } = services;
    const { requireAuth } = authMiddleware;
    const { requireInternalOrigin } = authorizationMiddleware;
    const { sseRateLimiter } = rateLimiters;
    const router = express.Router();

    // Use a factory function to get OperatorService to avoid circular dependencies
    // during initialization, as OperatorService may depend on other services
    // that are still being initialized.
    const getResolvedOperatorService = () => {
        try {
            return getOperatorService();
        } catch (e) {
            logger.error('[G8ED-SSE] Failed to get OperatorService', { error: e.message });
            throw e;
        }
    };

    router.options(SSEPaths.EVENTS, (req, res) => {
        res.sendStatus(204);
    });

    router.get(SSEPaths.EVENTS, sseRateLimiter, requireAuth, async (req, res, next) => {
        // WebSession already validated by requireAuth middleware
        const connectionId = req.webSessionId;
        let resolvedOperatorService;
        try {
            resolvedOperatorService = getResolvedOperatorService();
        } catch (e) {
            res.status(500).end();
            return;
        }

        // CRITICAL: Set unlimited timeout for SSE connections
        // SSE should stay open indefinitely - only session expiration should limit them
        req.setTimeout(0);
        res.setTimeout(0);

        res.setHeader('Content-Type', 'text/event-stream');
        res.setHeader('Cache-Control', 'no-cache, no-transform');
        res.setHeader('Connection', 'keep-alive');
        res.setHeader('X-Accel-Buffering', 'no');

        // Write 200 status code
        res.status(200);

        res.flushHeaders();
        res.socket?.setNoDelay(true);

        // Register connection with SSE service
        let sseConnectionId;

        try {
            const result = await sseService.registerConnection(
                connectionId,
                res,
                {
                    ip: req.ip,
                    userAgent: req.headers['user-agent'],
                    remoteAddress: req.connection?.remoteAddress || req.ip
                }
            );
            sseConnectionId = result.connectionId;

            logger.info(`[G8ED-SSE] New SSE connection established`, {
                webSessionId: redactWebSessionId(connectionId),
                localConnections: result.localConnections,
                sessionConnections: result.sessionConnections,
                userAgent: req.headers['user-agent'],
                remoteAddress: req.connection?.remoteAddress || req.ip
            });

            await sseService.publishEvent(connectionId, new ConnectionEstablishedEvent({
                type: EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED,
                connectionId: connectionId,
                timestamp: now()
            }));
        } catch (err) {
            logger.error('[G8ED-SSE] Connection setup failed', { error: err.message, webSessionId: redactWebSessionId(connectionId) });
            res.end();
            return;
        }

        const organizationId = req.session?.organization_id || req.session?.user_data?.organization_id || null;
        
        // Push initial state (LLM config, Investigation list)
        sseService.pushInitialState(req.userId, connectionId, organizationId).catch(err => {
            logger.error('[G8ED-SSE] pushInitialState failed', { error: err.message });
        });

        // Sync operator session state
        resolvedOperatorService.syncSessionOnConnect(req.userId, connectionId).catch(err => {
            logger.error('[G8ED-SSE] syncSessionOnConnect failed', { error: err.message });
        });

        // Track cleanup state to prevent duplicate cleanup on simultaneous close/error events
        let cleanedUp = false;
        let keepaliveInterval;
        const connectionStartTime = Date.now();

        // Helper function to cleanup connection
        const cleanupConnection = async () => {
            if (cleanedUp) return;
            cleanedUp = true;

            const connectionDuration = Date.now() - connectionStartTime;

            sseService.unregisterConnection(connectionId, sseConnectionId);
            if (keepaliveInterval) {
                clearInterval(keepaliveInterval);
            }

            logger.info(`[G8ED-SSE] Connection cleaned up`, {
                webSessionId: redactWebSessionId(connectionId),
                connectionDuration: Math.floor(connectionDuration / 1000) + 's'
            });
        };

        // Send keepalive every 20 seconds to detect broken connections
        // Status transitions (ACTIVE→OFFLINE, BOUND→STALE) are handled by HeartbeatMonitorService
        keepaliveInterval = setInterval(async () => {
            if (sseService.hasLocalConnection(connectionId)) {
                try {
                    let operatorList = null;
                    try {
                        const rawOperatorList = await resolvedOperatorService.getUserOperators(req.userId);
                        operatorList = rawOperatorList ? OperatorListData.parse(rawOperatorList) : null;
                    } catch (e) {
                        logger.error(`[G8ED-SSE] Failed to fetch operator list for keepalive:`, e);
                    }

                    await sseService.publishEvent(connectionId, new KeepaliveEvent({
                        type: EventType.PLATFORM_SSE_KEEPALIVE_SENT,
                        timestamp: now(),
                        serverTime: Date.now(),
                        operator_list: operatorList
                    }));

                } catch (error) {
                    logger.error(`[G8ED-SSE] Keepalive failed for ${connectionId}:`, error);
                    cleanupConnection();
                }
            } else {
                clearInterval(keepaliveInterval);
            }
        }, SSE_KEEPALIVE_INTERVAL_MS);

        // Handle client disconnect
        req.on('close', () => {
            const connectionDuration = Date.now() - connectionStartTime;
            logger.info(`[G8ED-SSE] SSE connection closed`, {
                webSessionId: redactWebSessionId(connectionId),
                connectionDuration: Math.floor(connectionDuration / 1000) + 's'
            });
            cleanupConnection();
        });

        req.on('error', (error) => {
            const connectionDuration = Date.now() - connectionStartTime;
            const isQuickFailure = connectionDuration < 5000;
            const isIdleTimeout = error.code === 'ECONNRESET' && connectionDuration > 300000; // >5min
            
            logger.error(`[G8ED-SSE] SSE connection error`, {
                webSessionId: redactWebSessionId(connectionId),
                error: error.message,
                errorCode: error.code,
                connectionDuration: Math.floor(connectionDuration / 1000) + 's',
                isQuickFailure,
                isIdleTimeout,
                possibleCause: isIdleTimeout ? 'Load balancer timeout or network interruption' : 'Unknown'
            });
            cleanupConnection();
        });
    });


    router.get(SSEPaths.HEALTH, requireInternalOrigin, async (req, res) => {
        const stats = sseService.getStats();
        
        res.json(new SSEHealthResponse({
            status: sseService.isHealthy() ? SystemHealth.HEALTHY : SystemHealth.DEGRADED,
            service: 'g8ed_sse',
            timestamp: now(),
            ...stats,
            config: {
                keepaliveInterval: SSE_KEEPALIVE_INTERVAL_MS,
                timeout: 'unlimited'
            },
        }).forClient());
    });

    router.post(SSEPaths.CONFIG, requireAuth, async (req, res) => {
        try {
            await sseService._pushLLMConfig(req.userId, req.webSessionId);
            res.status(200).json({ status: 'success' });
        } catch (error) {
            logger.error('[G8ED-SSE] Failed to manually push LLM config', { error: error.message });
            res.status(500).json({ error: 'Failed to push config' });
        }
    });

    return router;
}
