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
 * Cache Metrics - Track g8es KV Cache Performance
 * 
 * Monitors cache hit/miss ratios and estimates cost savings from reduced DB reads.
 * Used to measure effectiveness of the g8es KV caching strategy.
 */

import { logger } from './logger.js';

class CacheMetrics {
    constructor() {
        this.hits = 0;
        this.misses = 0;
        this.errors = 0;
        this.dbReads = 0;
        this.startTime = new Date();
        
        // Track metrics by cache type for detailed analysis
        this.metricsByType = new Map();
    }

    /**
     * Record cache hit (data found in g8es KV)
     */
    recordHit(cacheType = 'unknown') {
        this.hits++;
        this._updateTypeMetrics(cacheType, 'hit');
    }

    /**
     * Record cache miss (fallback to DB)
     */
    recordMiss(cacheType = 'unknown') {
        this.misses++;
        this.dbReads++;
        this._updateTypeMetrics(cacheType, 'miss');
    }

    /**
     * Record cache error
     */
    recordError(cacheType = 'unknown') {
        this.errors++;
        this._updateTypeMetrics(cacheType, 'error');
    }

    /**
     * Update metrics for specific cache type
     */
    _updateTypeMetrics(cacheType, operation) {
        if (!this.metricsByType.has(cacheType)) {
            this.metricsByType.set(cacheType, {
                hits: 0,
                misses: 0,
                errors: 0
            });
        }
        
        const typeMetrics = this.metricsByType.get(cacheType);
        if (operation === 'hit') typeMetrics.hits++;
        if (operation === 'miss') typeMetrics.misses++;
        if (operation === 'error') typeMetrics.errors++;
    }

    /**
     * Calculate cache hit ratio as percentage
     */
    getCacheHitRatio() {
        const total = this.hits + this.misses;
        return total === 0 ? 0 : parseFloat((this.hits / total * 100).toFixed(2));
    }

    /**
     * Calculate hit ratio for specific cache type
     */
    getCacheHitRatioByType(cacheType) {
        const metrics = this.metricsByType.get(cacheType);
        if (!metrics) return 0;
        
        const total = metrics.hits + metrics.misses;
        return total === 0 ? 0 : parseFloat((metrics.hits / total * 100).toFixed(2));
    }

    /**
     * Estimate cost savings from cache usage
     */
    calculateCostSavings() {
        const dbReadCost = 0.06 / 100000;
        const actualReads = this.dbReads;
        const potentialReads = this.hits + this.misses;
        const readsSaved = potentialReads - actualReads;
        
        return {
            readsSaved,
            actualReads,
            potentialReads,
            costSavedUSD: parseFloat((readsSaved * dbReadCost).toFixed(4)),
            projectedMonthlySavings: parseFloat((readsSaved * dbReadCost * 30).toFixed(2))
        };
    }

    /**
     * Get uptime in seconds
     */
    getUptimeSeconds() {
        return Math.floor((new Date().getTime() - this.startTime.getTime()) / 1000);
    }

    /**
     * Get comprehensive statistics
     */
    getStats() {
        const uptime = this.getUptimeSeconds();
        const costSavings = this.calculateCostSavings();
        
        return {
            overall: {
                hits: this.hits,
                misses: this.misses,
                errors: this.errors,
                dbReads: this.dbReads,
                hitRatio: this.getCacheHitRatio(),
                totalRequests: this.hits + this.misses,
                uptimeSeconds: uptime
            },
            byType: this._getStatsByType(),
            costSavings: {
                readsSaved: costSavings.readsSaved,
                actualDBReads: costSavings.actualReads,
                potentialDBReads: costSavings.potentialReads,
                costSavedUSD: costSavings.costSavedUSD,
                projectedMonthlySavingsUSD: costSavings.projectedMonthlySavings
            }
        };
    }

    /**
     * Get statistics grouped by cache type
     */
    _getStatsByType() {
        const statsByType = {};
        
        for (const [cacheType, metrics] of this.metricsByType.entries()) {
            statsByType[cacheType] = {
                hits: metrics.hits,
                misses: metrics.misses,
                errors: metrics.errors,
                hitRatio: this.getCacheHitRatioByType(cacheType),
                totalRequests: metrics.hits + metrics.misses
            };
        }
        
        return statsByType;
    }

    /**
     * Reset all metrics
     */
    reset() {
        this.hits = 0;
        this.misses = 0;
        this.errors = 0;
        this.dbReads = 0;
        this.startTime = new Date();
        this.metricsByType.clear();
        
        logger.info('[CACHE-METRICS] Metrics reset');
    }

    /**
     * Log current stats to console (for debugging)
     */
    logStats() {
        const stats = this.getStats();
        logger.info('[CACHE-METRICS] Current statistics', stats);
    }
}

// Export class for testing
export { CacheMetrics };

// Singleton instance
export const cacheMetrics = new CacheMetrics();
