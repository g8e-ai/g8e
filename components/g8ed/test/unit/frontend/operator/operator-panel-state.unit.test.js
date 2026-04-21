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

// Regression coverage for OperatorPanel state-mutation handlers
// (_onHeartbeat, _onStatusUpdated). Bugs fixed together with the JS
// OperatorSlot model renaming its identifier to `operator_id` (canonical
// wire field per shared/models/wire/operator_slot.json):
//
//   1. _onHeartbeat was matching slots by the (non-existent) `operator_id`
//      against slots whose field was named `id`, so findIndex always
//      returned -1 and latest_heartbeat_snapshot was never populated on
//      the panel — every row rendered with blank CPU/Memory/Disk/etc.
//
//   2. _onStatusUpdated only mutated aggregate counts; it never updated
//      the matching slot's status, so status.updated.{active,offline,...}
//      events had no effect on individual operator badges until a full
//      list refresh.

let OperatorPanel;
let HeartbeatSnapshot;
let OperatorStatus;

beforeEach(async () => {
    vi.resetModules();

    vi.doMock('@g8ed/public/js/utils/dev-logger.js', () => ({
        devLogger: { log: vi.fn(), warn: vi.fn(), error: vi.fn() },
    }));
    vi.doMock('@g8ed/public/js/utils/operator-session-service.js', () => ({
        operatorSessionService: { setBoundOperators: vi.fn() },
    }));
    vi.doMock('@g8ed/public/js/utils/notification-service.js', () => ({
        notificationService: { info: vi.fn(), error: vi.fn() },
    }));
    vi.doMock('@g8ed/public/js/utils/template-loader.js', () => ({
        templateLoader: { preload: vi.fn(), cache: new Map(), replace: vi.fn() },
    }));

    global.window = {
        ...(global.window || {}),
        authState: { getState: () => ({ isAuthenticated: true }) },
    };

    ({ OperatorPanel } = await import('@g8ed/public/js/components/operator-panel.js'));
    ({ HeartbeatSnapshot } = await import('@g8ed/public/js/models/operator-models.js'));
    ({ OperatorStatus }    = await import('@g8ed/public/js/constants/operator-constants.js'));
});

function createPanel(initialOperators) {
    const eventBus = { on: vi.fn(), off: vi.fn(), emit: vi.fn() };
    const panel = new OperatorPanel(eventBus);
    panel._operators = initialOperators;
    panel._isRendered = false;
    return panel;
}

describe('OperatorPanel._onHeartbeat [UNIT - PURE LOGIC]', () => {
    it('merges heartbeat metrics into the matching slot by operator_id', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE, latest_heartbeat_snapshot: null },
            { operator_id: 'op-2', status: OperatorStatus.AVAILABLE, latest_heartbeat_snapshot: null },
        ]);

        panel._onHeartbeat({
            type: 'g8e.v1.operator.heartbeat.received',
            operator_id: 'op-1',
            data: {
                status: OperatorStatus.ACTIVE,
                metrics: {
                    timestamp: '2026-04-21T21:30:10.683Z',
                    performance: { cpu_percent: 42.1, memory_percent: 60.3, disk_percent: 55.2, network_latency: 12 },
                    system_identity: { hostname: 'host-a', os: 'linux' },
                },
            },
        });

        const updated = panel._operators[0];
        expect(updated.latest_heartbeat_snapshot).toBeInstanceOf(HeartbeatSnapshot);
        expect(updated.latest_heartbeat_snapshot.performance.cpu_percent).toBe(42.1);
        expect(updated.latest_heartbeat_snapshot.performance.memory_percent).toBe(60.3);
        expect(updated.latest_heartbeat_snapshot.performance.disk_percent).toBe(55.2);
        expect(updated.latest_heartbeat_snapshot.performance.network_latency).toBe(12);
        expect(updated.latest_heartbeat_snapshot.system_identity.hostname).toBe('host-a');
        expect(updated.status).toBe(OperatorStatus.ACTIVE);
        expect(updated.last_heartbeat).toBe('2026-04-21T21:30:10.683Z');

        // Unrelated slot untouched
        expect(panel._operators[1].latest_heartbeat_snapshot).toBeNull();
    });

    it('leaves operators untouched when no operator_id matches', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE, latest_heartbeat_snapshot: null },
        ]);

        panel._onHeartbeat({
            type: 'g8e.v1.operator.heartbeat.received',
            operator_id: 'other',
            data: { metrics: {} },
        });

        expect(panel._operators[0].latest_heartbeat_snapshot).toBeNull();
    });

    it('ignores heartbeats when not authenticated', () => {
        global.window.authState = { getState: () => ({ isAuthenticated: false }) };

        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE, latest_heartbeat_snapshot: null },
        ]);

        panel._onHeartbeat({
            type: 'g8e.v1.operator.heartbeat.received',
            operator_id: 'op-1',
            data: { metrics: { performance: { cpu_percent: 99 } } },
        });

        expect(panel._operators[0].latest_heartbeat_snapshot).toBeNull();
    });
});

describe('OperatorPanel._onStatusUpdated [UNIT - PURE LOGIC]', () => {
    it('applies status, status_display, status_class, and bound_web_session_id to the matching slot', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE, status_display: 'AVAILABLE', status_class: 'available', bound_web_session_id: null },
        ]);

        panel._onStatusUpdated({
            type: 'g8e.v1.operator.status.updated.bound',
            data: {
                operator_id: 'op-1',
                status: OperatorStatus.BOUND,
                web_session_id: 'ws-42',
            },
        });

        const updated = panel._operators[0];
        expect(updated.status).toBe(OperatorStatus.BOUND);
        expect(updated.status_display).toBe(String(OperatorStatus.BOUND).toUpperCase());
        expect(updated.status_class).toBe(String(OperatorStatus.BOUND).toLowerCase());
        expect(updated.bound_web_session_id).toBe('ws-42');
    });

    it('preserves existing bound_web_session_id when event omits web_session_id', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.BOUND, bound_web_session_id: 'ws-existing' },
        ]);

        panel._onStatusUpdated({
            type: 'g8e.v1.operator.status.updated.active',
            data: {
                operator_id: 'op-1',
                status: OperatorStatus.ACTIVE,
            },
        });

        expect(panel._operators[0].status).toBe(OperatorStatus.ACTIVE);
        expect(panel._operators[0].bound_web_session_id).toBe('ws-existing');
    });

    it('does not mutate any slot when operator_id does not match', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);

        panel._onStatusUpdated({
            type: 'g8e.v1.operator.status.updated.active',
            data: {
                operator_id: 'missing',
                status: OperatorStatus.ACTIVE,
            },
        });

        expect(panel._operators[0].status).toBe(OperatorStatus.AVAILABLE);
    });

    it('updates aggregate counts from event payload', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);

        panel._onStatusUpdated({
            type: 'g8e.v1.operator.status.updated.active',
            data: {
                operator_id: 'op-1',
                status: OperatorStatus.ACTIVE,
                total_count: 7,
                active_count: 3,
            },
        });

        expect(panel._totalOperatorCount).toBe(7);
        expect(panel._activeOperatorCount).toBe(3);
    });
});
