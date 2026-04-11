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
import { logger } from '../../utils/logger.js';
import { SourceComponent, SystemHealth } from '../../constants/ai.js';
import { now } from '../../models/base.js';
import { MetricsHealthResponse, ErrorResponse } from '../../models/response_models.js';
import { MetricsPaths } from '../../constants/api_paths.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createMetricsRouter({
    services,
    authorizationMiddleware
}) {
    const { cacheAsideService } = services;
    const { requireInternalOrigin } = authorizationMiddleware;
    const router = express.Router();

    router.get(MetricsPaths.HEALTH, requireInternalOrigin, async (req, res, next) => {
        try {
            // Check VSODB KV health via cache-aside
            let kvHealthy = false;
            try {
                await cacheAsideService.kvGet('__health_check__');
                kvHealthy = true;
            } catch (e) {
                logger.error('[METRICS] VSODB KV health check failed', { error: e.message });
            }
            
            const isHealthy = kvHealthy;
            
            res.status(isHealthy ? 200 : 503).json(new MetricsHealthResponse({
                success: isHealthy,
                status: isHealthy ? SystemHealth.HEALTHY : SystemHealth.DEGRADED,
                service: SourceComponent.VSOD,
                vsodb: {
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
