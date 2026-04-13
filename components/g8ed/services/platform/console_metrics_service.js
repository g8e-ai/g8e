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
import { G8eBaseModel, F, now, addSeconds } from '../../models/base.js';
import { cacheMetrics } from '../../utils/cache_metrics.js';
import { OperatorStatus, OperatorType } from '../../constants/operator.js';
import { ConversationStatus } from '../../constants/chat.js';
import { SystemHealth } from '../../constants/ai.js';
import { Collections } from '../../constants/collections.js';
import { KVKey, KVScanPattern } from '../../constants/kv_keys.js';
import {
    CONSOLE_METRICS_CACHE_TTL_MS,
    CONSOLE_METRICS_WINDOW_1_DAY_SECONDS,
    CONSOLE_METRICS_WINDOW_7_DAYS_SECONDS,
    CONSOLE_METRICS_WINDOW_30_DAYS_SECONDS,
} from '../../constants/service_config.js';

class MetricsSnapshot extends G8eBaseModel {
    static fields = {
        timestamp:  { type: F.date, default: () => now() },
        users:      { type: F.any,  default: null },
        operators:  { type: F.any,  default: null },
        sessions:   { type: F.any,  default: null },
        cache:      { type: F.any,  default: null },
        system:     { type: F.any,  default: null },
    };
}

class RealTimeMetrics extends G8eBaseModel {
    static fields = {
        timestamp: { type: F.date, default: () => now() },
        g8es:     { type: F.any,  default: null },
        cache:     { type: F.any,  default: null },
    };
}

class DateQueryValue extends G8eBaseModel {
    static fields = {
        value: { type: F.date, required: true },
    };

    forDB() {
        return super.forDB().value;
    }
}

class ConsoleMetricsService {
    /**
     * @param {Object} options
     * @param {Object} options.cacheAsideService - CacheAsideService instance
     * @param {Object} [options.internalHttpClient] - InternalHttpClient instance
     */
    constructor({ cacheAsideService, internalHttpClient = null }) {
        this._cache_aside = cacheAsideService;
        this.internalHttpClient = internalHttpClient;
        this.metricsCache = new Map();
        this.cacheTTL = CONSOLE_METRICS_CACHE_TTL_MS;
    }

    async _getCachedMetric(key, computeFn) {
        const cached = this.metricsCache.get(key);
        if (cached && Date.now() - cached.timestamp < this.cacheTTL) {
            return cached.data;
        }

        try {
            const data = await computeFn();
            this.metricsCache.set(key, { data, timestamp: Date.now() });
            return data;
        } catch (error) {
            logger.error(`[CONSOLE-METRICS] Failed to compute metric: ${key}`, { error: error.message });
            return cached?.data || null;
        }
    }

    async getPlatformOverview() {
        const [
            userStats,
            operatorStats,
            sessionStats,
            cacheStats,
            systemHealth
        ] = await Promise.all([
            this.getUserStats(),
            this.getOperatorStats(),
            this.getSessionStats(),
            this.getCacheStats(),
            this.getSystemHealth()
        ]);

        return new MetricsSnapshot({
            timestamp: now(),
            users: userStats,
            operators: operatorStats,
            sessions: sessionStats,
            cache: cacheStats,
            system: systemHealth,
        }).forWire();
    }

    async getUserStats() {
        return this._getCachedMetric('user_stats', async () => {
            // Get all users
            const users = await this._cache_aside.queryDocuments(Collections.USERS, []);
            const ts = now();
            const thirtyDaysAgo = addSeconds(ts, -CONSOLE_METRICS_WINDOW_30_DAYS_SECONDS);
            const sevenDaysAgo = addSeconds(ts, -CONSOLE_METRICS_WINDOW_7_DAYS_SECONDS);
            const oneDayAgo = addSeconds(ts, -CONSOLE_METRICS_WINDOW_1_DAY_SECONDS);

            let activeLastDay = 0;
            let activeLastWeek = 0;
            let activeLastMonth = 0;
            let newUsersLastWeek = 0;

            for (const user of users) {
                const lastActive = user.last_active ? new Date(user.last_active) : 
                                   user.updated_at ? new Date(user.updated_at) : null;
                
                if (lastActive) {
                    if (lastActive >= oneDayAgo) activeLastDay++;
                    if (lastActive >= sevenDaysAgo) activeLastWeek++;
                    if (lastActive >= thirtyDaysAgo) activeLastMonth++;
                }

                const createdAt = user.created_at ? new Date(user.created_at) : null;
                if (createdAt && createdAt >= sevenDaysAgo) {
                    newUsersLastWeek++;
                }

            }

            return {
                total: users.length,
                activity: {
                    lastDay: activeLastDay,
                    lastWeek: activeLastWeek,
                    lastMonth: activeLastMonth
                },
                newUsersLastWeek
            };
        });
    }

    async getOperatorStats() {
        return this._getCachedMetric('operator_stats', async () => {
            const operators = await this._cache_aside.queryDocuments(Collections.OPERATORS, []);

            const statusDistribution = {
                [OperatorStatus.AVAILABLE]: 0,
                [OperatorStatus.ACTIVE]: 0,
                [OperatorStatus.BOUND]: 0,
                [OperatorStatus.OFFLINE]: 0,
                [OperatorStatus.STALE]: 0,
                [OperatorStatus.STOPPED]: 0,
                [OperatorStatus.TERMINATED]: 0,
                [OperatorStatus.UNAVAILABLE]: 0
            };

            const typeDistribution = {
                [OperatorType.SYSTEM]: 0,
                [OperatorType.CLOUD]: 0
            };

            let healthyCount = 0;
            let totalLatency = 0;
            let latencyCount = 0;
            let avgCpu = 0;
            let avgMemory = 0;
            let metricsCount = 0;

            for (const op of operators) {
                const status = op.status || OperatorStatus.OFFLINE;
                if (statusDistribution.hasOwnProperty(status)) {
                    statusDistribution[status]++;
                }

                const type = op.operator_type || OperatorType.SYSTEM;
                if (typeDistribution.hasOwnProperty(type)) {
                    typeDistribution[type]++;
                }

                const heartbeat = op.latest_heartbeat_snapshot;
                if (heartbeat) {
                    if (status === OperatorStatus.ACTIVE || status === OperatorStatus.BOUND) {
                        healthyCount++;
                    }
                    
                    if (heartbeat.network_latency) {
                        totalLatency += heartbeat.network_latency;
                        latencyCount++;
                    }

                    if (heartbeat.cpu_percent !== undefined) {
                        avgCpu += heartbeat.cpu_percent;
                        metricsCount++;
                    }

                    if (heartbeat.memory_percent !== undefined) {
                        avgMemory += heartbeat.memory_percent;
                    }
                }
            }

            return {
                total: operators.length,
                statusDistribution,
                typeDistribution,
                health: {
                    healthy: healthyCount,
                    unhealthy: operators.length - healthyCount - statusDistribution[OperatorStatus.AVAILABLE] - statusDistribution[OperatorStatus.TERMINATED],
                    avgLatencyMs: latencyCount > 0 ? Math.round(totalLatency / latencyCount) : 0,
                    avgCpuPercent: metricsCount > 0 ? Math.round(avgCpu / metricsCount) : 0,
                    avgMemoryPercent: metricsCount > 0 ? Math.round(avgMemory / metricsCount) : 0
                }
            };
        });
    }

    async _scanKeys(cacheAside, pattern) {
        const keys = [];
        let cursor = '0';
        do {
            const result = await cacheAside.kvScan(cursor, 'MATCH', pattern, 'COUNT', 100);
            cursor = result[0];
            keys.push(...result[1]);
        } while (cursor !== '0');
        return keys;
    }

    async getSessionStats() {
        return this._getCachedMetric('session_stats', async () => {
            try {
                const webSessions = await this._scanKeys(this._cache_aside, KVScanPattern.scanWebSessions());
                const operatorSessions = await this._scanKeys(this._cache_aside, KVScanPattern.scanOperatorSessions());

                const boundOperatorKeys = await this._scanKeys(this._cache_aside, KVScanPattern.allSessionWebBinds());
                let totalBoundOperators = 0;
                for (const key of boundOperatorKeys) {
                    const count = await this._cache_aside.kvScard(key);
                    totalBoundOperators += count;
                }

                return {
                    web: webSessions.length,
                    operator: operatorSessions.length,
                    total: webSessions.length + operatorSessions.length,
                    boundOperators: totalBoundOperators
                };
            } catch (error) {
                logger.error('[CONSOLE-METRICS] Failed to get session stats', { error: error.message });
                return { error: 'Failed to fetch session data' };
            }
        });
    }

    getCacheStats() {
        const stats = cacheMetrics.getStats();
        return {
            overall: stats.overall,
            byType: stats.byType,
            costSavings: stats.costSavings
        };
    }

    async getSystemHealth() {
        const health = {
            g8es: { status: SystemHealth.UNKNOWN, latencyMs: null },
            db: { status: SystemHealth.UNKNOWN, latencyMs: null },
            overall: SystemHealth.UNKNOWN
        };

        try {
            const kvStart = Date.now();
            await this._cache_aside.kvGet('__health_check__');
            health.g8es = {
                status: SystemHealth.HEALTHY,
                latencyMs: Date.now() - kvStart
            };
        } catch (error) {
            health.g8es = { status: SystemHealth.UNHEALTHY, error: error.message };
        }

        try {
            const dbStart = Date.now();
            await this._cache_aside.getDocument(Collections.SETTINGS, 'platform_settings');
            health.db = {
                status: SystemHealth.HEALTHY,
                latencyMs: Date.now() - dbStart
            };
        } catch (error) {
            health.db = { status: SystemHealth.UNHEALTHY, error: error.message };
        }

        health.overall = (health.g8es.status === SystemHealth.HEALTHY && health.db.status === SystemHealth.HEALTHY)
            ? SystemHealth.HEALTHY
            : SystemHealth.DEGRADED;

        return health;
    }

    async getLoginAuditStats() {
        return this._getCachedMetric('login_audit_stats', async () => {
            const oneDayAgo = addSeconds(now(), -CONSOLE_METRICS_WINDOW_1_DAY_SECONDS);
            
            const events = await this._cache_aside.queryDocuments(Collections.LOGIN_AUDIT, [
                { field: 'timestamp', operator: '>=', value: new DateQueryValue({ value: oneDayAgo }).forDB() }
            ]);
            
            const stats = {
                total: events.length,
                successful: 0,
                failed: 0,
                locked: 0,
                anomalies: 0,
                byHour: {}
            };

            for (const event of events) {
                switch (event.event_type) {
                    case 'login_success':
                        stats.successful++;
                        break;
                    case 'login_failed':
                        stats.failed++;
                        break;
                    case 'account_locked':
                        stats.locked++;
                        break;
                    case 'login_anomaly':
                        stats.anomalies++;
                        break;
                }

                // Group by hour
                if (event.timestamp) {
                    const hour = new Date(event.timestamp).getHours();
                    stats.byHour[hour] = (stats.byHour[hour] || 0) + 1;
                }
            }

            return stats;
        });
    }

    async getAIUsageStats() {
        return this._getCachedMetric('ai_usage_stats', async () => {
            const sevenDaysAgo = addSeconds(now(), -CONSOLE_METRICS_WINDOW_7_DAYS_SECONDS);
            
            const investigations = await this._cache_aside.queryDocuments(Collections.INVESTIGATIONS, [
                { field: 'created_at', operator: '>=', value: new DateQueryValue({ value: sevenDaysAgo }).forDB() }
            ]);

            return {
                totalInvestigations: investigations.length,
                activeInvestigations: investigations.filter(i => i.status === ConversationStatus.ACTIVE).length,
                completedInvestigations: investigations.filter(i => i.status === ConversationStatus.COMPLETED).length
            };
        });
    }

    async getRealTimeMetrics() {
        try {
            const mem = process.memoryUsage();
            const formatBytes = (bytes) => {
                if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}K`;
                return `${(bytes / (1024 * 1024)).toFixed(2)}M`;
            };

            return new RealTimeMetrics({
                timestamp: now(),
                g8es: {
                    memoryUsed: formatBytes(mem.heapUsed),
                    memoryPeak: formatBytes(mem.heapTotal),
                },
                cache: this.getCacheStats(),
            }).forWire();
        } catch (error) {
            logger.error('[CONSOLE-METRICS] Failed to get real-time metrics', { error: error.message });
            return { error: error.message };
        }
    }

    clearCache() {
        this.metricsCache.clear();
        logger.info('[CONSOLE-METRICS] Metrics cache cleared');
    }

    async getComponentHealth() {
        const components = {};

        components.g8ed = {
            name: 'g8ed',
            status: SystemHealth.HEALTHY,
            latencyMs: 0,
            details: {
                uptime: Math.floor(process.uptime()),
                memoryMb: Math.round(process.memoryUsage().heapUsed / 1024 / 1024),
                pid: process.pid,
            }
        };

        try {
            const kvStart = Date.now();
            await this._cache_aside.kvGet('__console_health_check__');
            components.g8es_kv = { name: 'g8es KV', status: SystemHealth.HEALTHY, latencyMs: Date.now() - kvStart };
        } catch (error) {
            components.g8es_kv = { name: 'g8es KV', status: SystemHealth.UNHEALTHY, latencyMs: null, error: error.message };
        }

        try {
            const dbStart = Date.now();
            await this._cache_aside.getDocument(Collections.SETTINGS, 'platform_settings');
            components.g8es_db = { name: 'g8es DB', status: SystemHealth.HEALTHY, latencyMs: Date.now() - dbStart };
        } catch (error) {
            components.g8es_db = { name: 'g8es DB', status: SystemHealth.UNHEALTHY, latencyMs: null, error: error.message };
        }

        try {
            const g8eeStart = Date.now();
            const g8eeHealth = await this.internalHttpClient.request('g8ee', '/health', { method: 'GET' });
            components.g8ee = {
                name: 'g8ee',
                status: g8eeHealth?.status === SystemHealth.HEALTHY ? SystemHealth.HEALTHY : SystemHealth.DEGRADED,
                latencyMs: Date.now() - g8eeStart,
                details: { reported_status: g8eeHealth?.status }
            };
        } catch (error) {
            components.g8ee = { name: 'g8ee', status: SystemHealth.UNHEALTHY, latencyMs: null, error: error.message };
        }

        const allHealthy = Object.values(components).every(c => c.status === SystemHealth.HEALTHY);
        const anyUnhealthy = Object.values(components).some(c => c.status === SystemHealth.UNHEALTHY);
        const overall = allHealthy ? SystemHealth.HEALTHY : anyUnhealthy ? SystemHealth.UNHEALTHY : SystemHealth.DEGRADED;

        return { overall, timestamp: now(), components };
    }

    async scanKV(pattern, cursor, count) {
        const [nextCursor, keys] = await this._cache_aside.kvScan(cursor, 'MATCH', pattern, 'COUNT', count);
        return { cursor: nextCursor, keys, count: keys.length };
    }

    async getKVKey(key) {
        const [value, ttl] = await Promise.all([this._cache_aside.kvGet(key), this._cache_aside.kvTtl(key)]);
        return { key, value, ttl, exists: value !== null };
    }

    async queryCollection(collection, limit) {
        const documents = await this._cache_aside.queryDocuments(collection, [], limit);
        return { collection, documents, count: documents.length };
    }
}

export { ConsoleMetricsService };
