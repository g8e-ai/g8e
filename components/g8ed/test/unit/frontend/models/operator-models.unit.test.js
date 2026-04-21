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

import { describe, it, expect } from 'vitest';
import { HeartbeatSnapshot } from '../../../../public/js/models/operator-models.js';

describe('HeartbeatSnapshot [FRONTEND UNIT]', () => {
    it('empty() returns snapshot with all nulls', () => {
        const snapshot = HeartbeatSnapshot.empty();
        expect(snapshot.timestamp).toBeNull();
        expect(snapshot.performance.cpu_percent).toBeNull();
        expect(snapshot.performance.memory_percent).toBeNull();
        expect(snapshot.performance.disk_percent).toBeNull();
        expect(snapshot.performance.network_latency).toBeNull();
        expect(snapshot.uptime.uptime_display).toBeNull();
        expect(snapshot.uptime.uptime_seconds).toBeNull();
    });

    it('parse() extracts metrics from nested structure', () => {
        const metrics = {
            timestamp: new Date('2026-01-01T00:00:00.000Z'),
            performance: {
                cpu_percent: 75.5,
                memory_percent: 60.2,
                disk_percent: 45.0,
                network_latency: 25,
            },
            uptime: {
                uptime_display: '2 days, 3 hours',
                uptime_seconds: 183600,
            },
        };
        const snapshot = HeartbeatSnapshot.parse(metrics);
        expect(snapshot.timestamp).toEqual(new Date('2026-01-01T00:00:00.000Z'));
        expect(snapshot.performance.cpu_percent).toBe(75.5);
        expect(snapshot.performance.memory_percent).toBe(60.2);
        expect(snapshot.performance.disk_percent).toBe(45.0);
        expect(snapshot.performance.network_latency).toBe(25);
        expect(snapshot.uptime.uptime_display).toBe('2 days, 3 hours');
        expect(snapshot.uptime.uptime_seconds).toBe(183600);
    });

    it('parse() handles missing metrics with null defaults', () => {
        const snapshot = HeartbeatSnapshot.parse({});
        expect(snapshot.performance.cpu_percent).toBeNull();
        expect(snapshot.performance.memory_percent).toBeNull();
        expect(snapshot.performance.disk_percent).toBeNull();
        expect(snapshot.performance.network_latency).toBeNull();
        expect(snapshot.uptime.uptime_display).toBeNull();
        expect(snapshot.uptime.uptime_seconds).toBeNull();
    });

    it('parse() extracts uptime fields from nested structure', () => {
        const metrics = {
            uptime: {
                uptime_display: '1h 30m',
                uptime_seconds: 5400,
            },
        };
        const snapshot = HeartbeatSnapshot.parse(metrics);
        expect(snapshot.uptime.uptime_display).toBe('1h 30m');
        expect(snapshot.uptime.uptime_seconds).toBe(5400);
    });

    it('parse() validates and creates valid HeartbeatSnapshot', () => {
        const raw = {
            timestamp: new Date('2026-01-01T00:00:00.000Z'),
            performance: {
                cpu_percent: 50.0,
                memory_percent: 75.0,
            },
        };
        const snapshot = HeartbeatSnapshot.parse(raw);
        expect(snapshot.performance.cpu_percent).toBe(50.0);
        expect(snapshot.performance.memory_percent).toBe(75.0);
        expect(snapshot.performance.disk_percent).toBeNull();
    });

    it('forWire() serializes Date to ISO string', () => {
        const timestamp = new Date('2026-01-01T00:00:00.000Z');
        const snapshot = HeartbeatSnapshot.parse({
            timestamp,
            performance: {
                cpu_percent: 50.0,
            },
        });
        const wire = snapshot.forWire();
        expect(wire.timestamp).toBe('2026-01-01T00:00:00.000Z');
        expect(wire.performance.cpu_percent).toBe(50.0);
    });
});
