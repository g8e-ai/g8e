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

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { CacheMetrics } from '../../../utils/cache_metrics.js';

describe('CacheMetrics', () => {
    let metrics;

    beforeEach(() => {
        metrics = new CacheMetrics();
        // Reset the start time for predictable tests
        metrics.startTime = new Date();
    });

    describe('recording metrics', () => {
        it('should record hits correctly', () => {
            metrics.recordHit('test-cache');
            expect(metrics.hits).toBe(1);
            expect(metrics.getStats().byType['test-cache'].hits).toBe(1);
        });

        it('should record misses correctly', () => {
            metrics.recordMiss('test-cache');
            expect(metrics.misses).toBe(1);
            expect(metrics.dbReads).toBe(1);
            expect(metrics.getStats().byType['test-cache'].misses).toBe(1);
        });

        it('should record errors correctly', () => {
            metrics.recordError('test-cache');
            expect(metrics.errors).toBe(1);
            expect(metrics.getStats().byType['test-cache'].errors).toBe(1);
        });

        it('should handle unknown cache types', () => {
            metrics.recordHit();
            expect(metrics.getStats().byType['unknown'].hits).toBe(1);
        });
    });

    describe('calculations', () => {
        it('should calculate hit ratio correctly', () => {
            expect(metrics.getCacheHitRatio()).toBe(0);
            
            metrics.recordHit('a');
            metrics.recordMiss('a');
            // 1 hit, 1 miss = 50%
            expect(metrics.getCacheHitRatio()).toBe(50);
            
            metrics.recordHit('b');
            // 2 hits, 1 miss = 66.67%
            expect(metrics.getCacheHitRatio()).toBe(66.67);
        });

        it('should calculate hit ratio by type correctly', () => {
            metrics.recordHit('a');
            metrics.recordMiss('a');
            metrics.recordHit('b');
            
            expect(metrics.getCacheHitRatioByType('a')).toBe(50);
            expect(metrics.getCacheHitRatioByType('b')).toBe(100);
            expect(metrics.getCacheHitRatioByType('nonexistent')).toBe(0);
        });

        it('should calculate cost savings correctly', () => {
            // 100,000 hits = 100,000 reads saved
            for (let i = 0; i < 100000; i++) {
                metrics.recordHit('test');
            }
            
            const savings = metrics.calculateCostSavings();
            expect(savings.readsSaved).toBe(100000);
            // dbReadCost = 0.06 / 100000 per read
            // 100,000 * (0.06 / 100000) = 0.06
            expect(savings.costSavedUSD).toBe(0.06);
            expect(savings.projectedMonthlySavings).toBe(1.8); // 0.06 * 30
        });
    });

    describe('state management', () => {
        it('should get comprehensive stats', () => {
            metrics.recordHit('cache1');
            metrics.recordMiss('cache2');
            metrics.recordError('cache1');
            
            const stats = metrics.getStats();
            expect(stats.overall.hits).toBe(1);
            expect(stats.overall.misses).toBe(1);
            expect(stats.overall.errors).toBe(1);
            expect(stats.byType['cache1'].hits).toBe(1);
            expect(stats.byType['cache2'].misses).toBe(1);
            expect(stats.costSavings).toBeDefined();
        });

        it('should reset metrics correctly', () => {
            metrics.recordHit('test');
            metrics.reset();
            
            expect(metrics.hits).toBe(0);
            expect(metrics.metricsByType.size).toBe(0);
        });

        it('should calculate uptime correctly', () => {
            const startTime = new Date();
            metrics.startTime = startTime;
            
            vi.useFakeTimers();
            vi.setSystemTime(startTime.getTime() + 10000);
            
            try {
                expect(metrics.getUptimeSeconds()).toBe(10);
            } finally {
                vi.useRealTimers();
            }
        });
    });
});
