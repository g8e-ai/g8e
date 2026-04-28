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
import {
    HeartbeatSnapshot,
    OperatorSlot,
    OperatorListUpdatedEvent,
    OperatorStatusUpdatedEvent,
    HeartbeatSSEEnvelope,
} from '../../../../public/js/models/operator-models.js';

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


describe('OperatorSlot [FRONTEND UNIT]', () => {
    it('parse() requires operator_id', () => {
        expect(() => OperatorSlot.parse({})).toThrow();
    });

    it('parse() validates operator_id is required', () => {
        const slot = OperatorSlot.parse({ operator_id: 'op-123' });
        expect(slot.operator_id).toBe('op-123');
        expect(slot.name).toBeNull();
        expect(slot.status).toBeNull();
        expect(slot.status_class).toBe('inactive');
    });

    it('parse() extracts all operator fields', () => {
        const slot = OperatorSlot.parse({
            operator_id: 'op-123',
            name: 'test-operator',
            status: 'active',
            status_display: 'ACTIVE',
            status_class: 'active',
            bound_web_session_id: 'ws-456',
            is_g8ep: false,
            first_deployed: '2026-01-01T00:00:00.000Z',
            claimed_at: '2026-01-01T12:00:00.000Z',
            last_heartbeat: '2026-01-02T00:00:00.000Z',
            latest_heartbeat_snapshot: {
                system_identity: {
                    hostname: 'test-host',
                    os: 'linux',
                },
            },
        });
        expect(slot.operator_id).toBe('op-123');
        expect(slot.name).toBe('test-operator');
        expect(slot.status).toBe('active');
        expect(slot.status_display).toBe('ACTIVE');
        expect(slot.status_class).toBe('active');
        expect(slot.bound_web_session_id).toBe('ws-456');
        expect(slot.is_g8ep).toBe(false);
        expect(slot.first_deployed).toEqual(new Date('2026-01-01T00:00:00.000Z'));
        expect(slot.claimed_at).toEqual(new Date('2026-01-01T12:00:00.000Z'));
        expect(slot.last_heartbeat).toEqual(new Date('2026-01-02T00:00:00.000Z'));
        expect(slot.latest_heartbeat_snapshot.system_identity.hostname).toBe('test-host');
        expect(slot.latest_heartbeat_snapshot.system_identity.os).toBe('linux');
    });
});

describe('OperatorListUpdatedEvent [FRONTEND UNIT]', () => {
    // These models represent the INNER `data` payload the browser receives after
    // sse-connection-manager strips the `{ type, data }` transport envelope.
    // See operator-models.js for the contract note.

    it('parse() extracts list event fields from flat payload', () => {
        const event = OperatorListUpdatedEvent.parse({
            operators: [
                { operator_id: 'op-1', status: 'active' },
                { operator_id: 'op-2', status: 'available' },
            ],
            total_count: 2,
            active_count: 1,
            used_slots: 1,
            max_slots: 5,
        });
        expect(event.operators).toHaveLength(2);
        expect(event.operators[0]).toBeInstanceOf(OperatorSlot);
        expect(event.operators[0].operator_id).toBe('op-1');
        expect(event.operators[1].operator_id).toBe('op-2');
        expect(event.total_count).toBe(2);
        expect(event.active_count).toBe(1);
        expect(event.used_slots).toBe(1);
        expect(event.max_slots).toBe(5);
    });

    it('parse() parses operators array through OperatorSlot', () => {
        const event = OperatorListUpdatedEvent.parse({
            operators: [
                { operator_id: 'op-1', status: 'active' },
            ],
        });
        expect(event.operators[0]).toBeInstanceOf(OperatorSlot);
    });

    it('parse() handles empty payload with defaults', () => {
        const event = OperatorListUpdatedEvent.parse({});
        expect(event.operators).toEqual([]);
        expect(event.total_count).toBe(0);
        expect(event.active_count).toBe(0);
        expect(event.used_slots).toBe(0);
        expect(event.max_slots).toBe(0);
    });
});

describe('OperatorStatusUpdatedEvent [FRONTEND UNIT]', () => {
    it('parse() requires operator_id and status', () => {
        expect(() => OperatorStatusUpdatedEvent.parse({})).toThrow();
        expect(() => OperatorStatusUpdatedEvent.parse({ operator_id: 'op-1' })).toThrow();
    });

    it('parse() extracts flat status payload', () => {
        const event = OperatorStatusUpdatedEvent.parse({
            operator_id: 'op-123',
            status: 'active',
            hostname: 'test-host',
            total_count: 5,
            active_count: 2,
        });
        expect(event.operator_id).toBe('op-123');
        expect(event.status).toBe('active');
        expect(event.hostname).toBe('test-host');
        expect(event.total_count).toBe(5);
        expect(event.active_count).toBe(2);
    });
});

describe('HeartbeatSSEEnvelope [FRONTEND UNIT]', () => {
    it('parse() requires operator_id and status', () => {
        expect(() => HeartbeatSSEEnvelope.parse({})).toThrow();
        expect(() => HeartbeatSSEEnvelope.parse({ operator_id: 'op-1' })).toThrow();
    });

    it('parse() extracts envelope fields and coerces metrics into HeartbeatSnapshot', () => {
        const env = HeartbeatSSEEnvelope.parse({
            operator_id: 'op-123',
            status: 'active',
            metrics: {
                timestamp: '2026-01-01T00:00:00.000Z',
                performance: { cpu_percent: 50 },
            },
        });
        expect(env.operator_id).toBe('op-123');
        expect(env.status).toBe('active');
        expect(env.metrics).toBeInstanceOf(HeartbeatSnapshot);
        expect(env.metrics.performance.cpu_percent).toBe(50);
        expect(env.metrics.timestamp).toEqual(new Date('2026-01-01T00:00:00.000Z'));
    });

    it('parse() defaults metrics to an empty HeartbeatSnapshot when omitted', () => {
        const env = HeartbeatSSEEnvelope.parse({
            operator_id: 'op-123',
            status: 'active',
        });
        expect(env.metrics).toBeInstanceOf(HeartbeatSnapshot);
        expect(env.metrics.performance.cpu_percent).toBeNull();
    });
});
