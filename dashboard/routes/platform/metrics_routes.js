// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import express from 'express';
import { logger } from '../../utils/logger.js';
import { SourceComponent, SystemHealth } from '../../constants/ai.js';
import { now } from '../../models/base.js';
import { MetricsHealthResponse, ErrorResponse } from '../../models/response_models.js';
import { MetricsPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.cacheAsideService - CacheAsideService instance
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createMetricsRouter({
    cacheAsideService,
    authorizationMiddleware
}) {
    const { requireInternalOrigin } = authorizationMiddleware;
    const router = express.Router();

    router.get(MetricsPaths.HEALTH, requireInternalOrigin, async (req, res, next) => {
        try {
            // Check g8eg KV health via cache-aside
            let kvHealthy = false;
            try {
                await cacheAsideService.kvGet('__health_check__');
                kvHealthy = true;
            } catch (e) {
                logger.error('[METRICS] g8eg KV health check failed', { error: e.message });
            }
            
            const isHealthy = kvHealthy;
            
            res.status(isHealthy ? 200 : 503).json(new MetricsHealthResponse({
                success: isHealthy,
                status: isHealthy ? SystemHealth.HEALTHY : SystemHealth.DEGRADED,
                service: SourceComponent.g8ed,
                g8eg: {
                    healthy: kvHealthy
                },
                timestamp: now()
            }).forClient());
        } catch (error) {
            logger.error('[METRICS] Health check failed', { error: error.message });
            res.status(503).json(new ErrorResponse({
                error: error.message
            }).forClient());
        }
    });

    return router;
}
