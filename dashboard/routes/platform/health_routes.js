// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import express from 'express';
import { logger } from '../../utils/logger.js';
import { HealthResponse, CacheStatsResponse, SimpleStatusResponse, ErrorResponse } from '../../models/response_models.js';
import { SystemHealth } from '../../constants/ai.js';
import { HealthPaths } from '../../constants/api_paths.js';
import { HealthCheckService } from '../../services/platform/health_check_service.js';

/**
 * @param {Object} options
 * @param {Object} options.cacheAsideService - CacheAsideService instance
 * @param {Object} options.webSessionService - WebSessionService instance
 * @param {Object} options.authorizationMiddleware - Authorization middleware object
 */
export function createHealthRouter({ cacheAsideService, webSessionService, authorizationMiddleware }) {
    const { requireInternalOrigin } = authorizationMiddleware;
    const healthCheckService = new HealthCheckService({ cacheAsideService, webSessionService });
    const router = express.Router();

    router.get(HealthPaths.ROOT, async (req, res) => {
        const healthData = healthCheckService.getBasicHealth();
        res.json(new HealthResponse(healthData).forClient());
    });

    router.get(HealthPaths.LIVE, async (req, res) => {
        const livenessData = healthCheckService.getLivenessStatus();
        res.json(new SimpleStatusResponse(livenessData).forClient());
    });

    router.get(HealthPaths.STORE, requireInternalOrigin, async (req, res) => {
        try {
            const readinessData = await healthCheckService.getReadinessStatus();
            const statusCode = readinessData.success ? 200 : 503;
            res.status(statusCode).json(new SimpleStatusResponse(readinessData).forClient());
        } catch (error) {
            logger.error('Readiness check failed:', error);
            res.status(503).json(new SimpleStatusResponse({
                success: false,
                status: 'error',
                message: 'Readiness check failed',
                details: { error: error.message }
            }).forClient());
        }
    });

    router.get(HealthPaths.DETAILS, requireInternalOrigin, async (req, res, next) => {
        try {
            const healthStatus = await healthCheckService.getDetailedHealthStatus();
            const statusCode = healthStatus.status === SystemHealth.HEALTHY ? 200 : 503;
            res.status(statusCode).json(new HealthResponse(healthStatus).forClient());
        } catch (error) {
            logger.error('Health check failed:', error);
            res.status(503).json(new HealthResponse({
                status: SystemHealth.UNHEALTHY,
                timestamp: new Date().toISOString(),
                service: 'g8ed',
                error: error.message
            }).forClient());
        }
    });

    router.get(HealthPaths.CACHE_STATS, requireInternalOrigin, async (req, res) => {
        try {
            const stats = healthCheckService.getCacheStats();
            res.json(new CacheStatsResponse(stats).forClient());
        } catch (error) {
            logger.error('Cache stats failed:', error);
            res.status(500).json(new ErrorResponse({
                error: 'Failed to retrieve cache statistics',
                message: error.message
            }).forClient());
        }
    });

    return router;
}
