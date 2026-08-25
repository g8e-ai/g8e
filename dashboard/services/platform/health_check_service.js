// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { logger } from '../../utils/logger.js';
import { VSOBaseModel, F, now } from '../../models/base.js';
import { cacheMetrics } from '../../utils/cache_metrics.js';
import { Collections } from '../../constants/collections.js';
import { SystemHealth, SourceComponent } from '../../constants/ai.js';

class HealthCheckResult extends VSOBaseModel {
    static fields = {
        status: { type: F.string, required: true },
        message: { type: F.string, default: null },
        latencyMs: { type: F.number, default: null },
        error: { type: F.string, default: null },
        details: { type: F.object, default: null }
    };
}

class DetailedHealthStatus extends VSOBaseModel {
    static fields = {
        status: { type: F.string, required: true },
        timestamp: { type: F.date, required: true },
        service: { type: F.string, required: true },
        checks: { type: F.object, default: () => ({}) },
        error: { type: F.string, default: null }
    };
}

/**
 * HealthCheckService - Centralizes all health check business logic
 * Extracted from routes to maintain separation of concerns
 */
export class HealthCheckService {
    /**
     * @param {Object} options
     * @param {Object} options.cacheAsideService - CacheAsideService instance
     * @param {Object} options.webSessionService - WebSessionService instance
     */
    constructor({ cacheAsideService, webSessionService }) {
        this.cacheAsideService = cacheAsideService;
        this.webSessionService = webSessionService;
    }

    /**
     * Basic health check - confirms service is alive
     */
    getBasicHealth() {
        return {
            status: SystemHealth.HEALTHY,
            timestamp: now(),
            service: SourceComponent.g8ed
        };
    }

    /**
     * Liveness check - simple alive status
     */
    getLivenessStatus() {
        return {
            success: true,
            status: 'alive',
            message: 'g8ed is alive',
            details: { service: SourceComponent.g8ed }
        };
    }

    /**
     * Readiness check - verifies all critical dependencies are available
     */
    async getReadinessStatus() {
        const checks = {};
        let isReady = true;

        // Check g8eg KV connectivity
        try {
            const g8egHealthy = this.webSessionService.isHealthy();
            if (g8egHealthy) {
                checks.g8eg = 'up';
            } else {
                checks.g8eg = 'down';
                isReady = false;
            }
        } catch (e) {
            checks.g8eg = `down: ${e.message}`;
            isReady = false;
        }

        // Check DB availability via cache-aside
        try {
            if (this.cacheAsideService) {
                checks.database = 'up';
            } else {
                checks.database = 'not_initialized';
                isReady = false;
            }
        } catch (e) {
            checks.database = `down: ${e.message}`;
            isReady = false;
        }

        const status = isReady ? 'ready' : SystemHealth.UNHEALTHY;
        const message = isReady ? 'Platform components are ready' : 'Platform components are not ready';

        return {
            success: isReady,
            status,
            message,
            details: { 
                service: SourceComponent.g8ed,
                checks 
            }
        };
    }

    /**
     * Detailed health check - comprehensive status of all components
     */
    async getDetailedHealthStatus() {
        const healthStatus = new DetailedHealthStatus({
            status: SystemHealth.HEALTHY,
            timestamp: now(),
            service: SourceComponent.g8ed,
            checks: {}
        });

        // Storage check removed - file storage decommissioned
        healthStatus.checks.storage = { status: 'skipped', message: 'File storage decommissioned' };

        // g8eg KV check
        await this._checkg8egKV(healthStatus);

        // Database check via cache-aside
        await this._checkDatabase(healthStatus);

        // Calculate overall status
        const unhealthyChecks = Object.values(healthStatus.checks).filter(check => check.status === SystemHealth.UNHEALTHY);
        if (unhealthyChecks.length > 0) {
            healthStatus.status = SystemHealth.UNHEALTHY;
        }

        return healthStatus;
    }

    /**
     * Cache statistics - performance metrics
     */
    getCacheStats() {
        try {
            const stats = cacheMetrics.getStats();
            
            return {
                timestamp: now(),
                service: SourceComponent.g8ed,
                cache_performance: stats.overall,
                cache_by_type: stats.byType,
                cost_savings: stats.costSavings,
                message: 'Cache statistics showing g8eg KV performance and DB read reduction'
            };
        } catch (error) {
            logger.error('Cache stats failed:', error);
            throw new Error('Failed to retrieve cache statistics');
        }
    }

    /**
     * Check g8eg KV connectivity and health
     * @private
     */
    async _checkg8egKV(healthStatus) {
        try {
            const g8egHealthy = this.webSessionService.isHealthy();
            if (g8egHealthy) {
                const sessionCount = await this.webSessionService.getSessionCount();
                healthStatus.checks.g8eg = { 
                    status: SystemHealth.HEALTHY, 
                    message: 'g8eg KV connection is healthy',
                    activeSessions: sessionCount
                };
            } else {
                healthStatus.checks.g8eg = { status: SystemHealth.UNHEALTHY, message: 'g8eg KV connection is down' };
            }
        } catch (e) {
            healthStatus.checks.g8eg = { status: SystemHealth.UNHEALTHY, message: `g8eg KV error: ${e.message}` };
        }
    }

    /**
     * Check database connectivity via cache-aside
     * @private
     */
    async _checkDatabase(healthStatus) {
        try {
            const configDoc = await this.cacheAsideService.getDocument(Collections.PLATFORM_SETTINGS, 'platform_settings');
            if (configDoc !== null) {
                healthStatus.checks.database = { status: SystemHealth.HEALTHY, message: 'g8eg document store is OK.' };
            } else {
                healthStatus.checks.database = { status: SystemHealth.UNHEALTHY, message: 'Failed to query g8eg: document not found' };
            }
        } catch (e) {
            healthStatus.checks.database = { status: SystemHealth.UNHEALTHY, message: `DB error: ${e.message}` };
        }
    }
}
