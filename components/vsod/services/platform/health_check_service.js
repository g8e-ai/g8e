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
        this._cacheAside = cacheAsideService;
        this._webSessionService = webSessionService;
    }

    /**
     * Basic health check - confirms service is alive
     */
    getBasicHealth() {
        return {
            status: SystemHealth.HEALTHY,
            timestamp: now(),
            service: SourceComponent.VSOD
        };
    }

    /**
     * Liveness check - simple alive status
     */
    getLivenessStatus() {
        return {
            success: true,
            status: 'alive',
            message: 'VSOD is alive',
            details: { service: SourceComponent.VSOD }
        };
    }

    /**
     * Readiness check - verifies all critical dependencies are available
     */
    async getReadinessStatus() {
        const checks = {};
        let isReady = true;

        // Check VSODB KV connectivity
        try {
            const vsodbHealthy = this._webSessionService.isHealthy();
            if (vsodbHealthy) {
                checks.vsodb = 'up';
            } else {
                checks.vsodb = 'down';
                isReady = false;
            }
        } catch (e) {
            checks.vsodb = `down: ${e.message}`;
            isReady = false;
        }

        // Check DB availability via cache-aside
        try {
            if (this._cacheAside) {
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
                service: SourceComponent.VSOD,
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
            service: SourceComponent.VSOD,
            checks: {}
        });

        // Storage check removed - file storage decommissioned
        healthStatus.checks.storage = { status: 'skipped', message: 'File storage decommissioned' };

        // VSODB KV check
        await this._checkVSODBKV(healthStatus);

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
                service: SourceComponent.VSOD,
                cache_performance: stats.overall,
                cache_by_type: stats.byType,
                cost_savings: stats.costSavings,
                message: 'Cache statistics showing VSODB KV performance and DB read reduction'
            };
        } catch (error) {
            logger.error('Cache stats failed:', error);
            throw new Error('Failed to retrieve cache statistics');
        }
    }

    /**
     * Check VSODB KV connectivity and health
     * @private
     */
    async _checkVSODBKV(healthStatus) {
        try {
            const vsodbHealthy = this._webSessionService.isHealthy();
            if (vsodbHealthy) {
                const sessionCount = await this._webSessionService.getSessionCount();
                healthStatus.checks.vsodb = { 
                    status: SystemHealth.HEALTHY, 
                    message: 'VSODB KV connection is healthy',
                    activeSessions: sessionCount
                };
            } else {
                healthStatus.checks.vsodb = { status: SystemHealth.UNHEALTHY, message: 'VSODB KV connection is down' };
            }
        } catch (e) {
            healthStatus.checks.vsodb = { status: SystemHealth.UNHEALTHY, message: `VSODB KV error: ${e.message}` };
        }
    }

    /**
     * Check database connectivity via cache-aside
     * @private
     */
    async _checkDatabase(healthStatus) {
        try {
            const configDoc = await this._cacheAside.getDocument(Collections.PLATFORM_SETTINGS, 'platform_settings');
            if (configDoc !== null) {
                healthStatus.checks.database = { status: SystemHealth.HEALTHY, message: 'VSODB document store is OK.' };
            } else {
                healthStatus.checks.database = { status: SystemHealth.UNHEALTHY, message: 'Failed to query VSODB: document not found' };
            }
        } catch (e) {
            healthStatus.checks.database = { status: SystemHealth.UNHEALTHY, message: `DB error: ${e.message}` };
        }
    }
}
