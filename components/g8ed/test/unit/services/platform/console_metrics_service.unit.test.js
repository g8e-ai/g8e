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

import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { ConsoleMetricsService } from '@g8ed/services/platform/console_metrics_service.js';
import { Collections } from '@g8ed/constants/collections.js';
import { OperatorStatus } from '@g8ed/constants/operator.js';
import { SystemHealth } from '@g8ed/constants/ai.js';

describe('ConsoleMetricsService [UNIT]', () => {
    let cacheAside;
    let internalHttpClient;
    let service;

    beforeEach(() => {
        vi.useFakeTimers();
        cacheAside = {
            queryDocuments: vi.fn(),
            getDocument: vi.fn(),
            kvScan: vi.fn(),
            kvScard: vi.fn(),
            kvGet: vi.fn(),
            kvTtl: vi.fn(),
        };
        internalHttpClient = {
            request: vi.fn(),
        };
        service = new ConsoleMetricsService({ cacheAsideService: cacheAside, internalHttpClient });
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    describe('getUserStats', () => {
        it('calculates activity distributions correctly', async () => {
            const now = new Date('2026-03-30T12:00:00Z');
            vi.setSystemTime(now);

            const users = [
                { id: 'u1', last_active: now.toISOString(), created_at: now.toISOString() }, // active today, new this week
                { id: 'u2', last_active: new Date(now.getTime() - 2 * 24 * 60 * 60 * 1000).toISOString() }, // active last week
                { id: 'u3', last_active: new Date(now.getTime() - 10 * 24 * 60 * 60 * 1000).toISOString() }, // active last month
                { id: 'u4', last_active: new Date(now.getTime() - 40 * 24 * 60 * 60 * 1000).toISOString() }, // inactive
            ];
            cacheAside.queryDocuments.mockResolvedValue(users);

            const stats = await service.getUserStats();

            expect(stats.total).toBe(4);
            expect(stats.activity.lastDay).toBe(1);
            expect(stats.activity.lastWeek).toBe(2);
            expect(stats.activity.lastMonth).toBe(3);
            expect(stats.newUsersLastWeek).toBe(1);
        });

        it('caches results', async () => {
            cacheAside.queryDocuments.mockResolvedValue([]);
            await service.getUserStats();
            await service.getUserStats();
            expect(cacheAside.queryDocuments).toHaveBeenCalledTimes(1);
        });
    });

    describe('getOperatorStats', () => {
        it('calculates health and distribution correctly', async () => {
            const operators = [
                { status: OperatorStatus.ACTIVE, latest_heartbeat_snapshot: { network_latency: 10, cpu_percent: 5, memory_percent: 20 } },
                { status: OperatorStatus.BOUND, latest_heartbeat_snapshot: { network_latency: 20, cpu_percent: 15, memory_percent: 30 } },
                { status: OperatorStatus.AVAILABLE },
                { status: OperatorStatus.OFFLINE }
            ];
            cacheAside.queryDocuments.mockResolvedValue(operators);

            const stats = await service.getOperatorStats();

            expect(stats.total).toBe(4);
            expect(stats.statusDistribution[OperatorStatus.ACTIVE]).toBe(1);
            expect(stats.health.healthy).toBe(2); // ACTIVE + BOUND
            expect(stats.health.avgLatencyMs).toBe(15);
            expect(stats.health.avgCpuPercent).toBe(10);
        });

        it('reads metrics from the nested OperatorHeartbeat shape (performance.*)', async () => {
            // Canonical persisted shape is g8ee OperatorHeartbeat with nested
            // performance.cpu_percent / memory_percent / network_latency.
            const operators = [
                {
                    status: OperatorStatus.ACTIVE,
                    latest_heartbeat_snapshot: {
                        performance: { cpu_percent: 40, memory_percent: 50, network_latency: 12 },
                    },
                },
                {
                    status: OperatorStatus.BOUND,
                    latest_heartbeat_snapshot: {
                        performance: { cpu_percent: 60, memory_percent: 70, network_latency: 18 },
                    },
                },
            ];
            cacheAside.queryDocuments.mockResolvedValue(operators);

            const stats = await service.getOperatorStats();

            expect(stats.health.healthy).toBe(2);
            expect(stats.health.avgLatencyMs).toBe(15);
            expect(stats.health.avgCpuPercent).toBe(50);
            expect(stats.health.avgMemoryPercent).toBe(60);
        });
    });

    describe('getSystemHealth', () => {
        it('reports healthy when both KV and DB are responsive', async () => {
            cacheAside.kvGet.mockResolvedValue('ok');
            cacheAside.getDocument.mockResolvedValue({ settings: {} });

            const health = await service.getSystemHealth();

            expect(health.overall).toBe(SystemHealth.HEALTHY);
            expect(health.g8es.status).toBe(SystemHealth.HEALTHY);
            expect(health.db.status).toBe(SystemHealth.HEALTHY);
        });

        it('reports degraded if one service fails', async () => {
            cacheAside.kvGet.mockRejectedValue(new Error('KV fail'));
            cacheAside.getDocument.mockResolvedValue({ settings: {} });

            const health = await service.getSystemHealth();

            expect(health.overall).toBe(SystemHealth.DEGRADED);
            expect(health.g8es.status).toBe(SystemHealth.UNHEALTHY);
        });
    });

    describe('getRealTimeMetrics', () => {
        it('returns process and cache metrics', async () => {
            const metrics = await service.getRealTimeMetrics();
            expect(metrics.timestamp).toBeDefined();
            expect(metrics.g8es.memoryUsed).toBeDefined();
            expect(metrics.cache).toBeDefined();
        });
    });
});
