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
// (_onHeartbeat, _onStatusUpdated, heartbeat buffering).
//
// Payloads here mirror what sse-connection-manager actually emits on the
// eventBus: the INNER `data` body only, with the `{ type, data }` transport
// envelope already stripped. For heartbeat this is the flat envelope shape
// from shared/models/wire/heartbeat_sse.json (operator_id, status, metrics).
// For status-updated this is the flat OperatorStatusUpdatedEvent shape.

let OperatorPanel;
let HeartbeatSnapshot;
let OperatorStatus;
let computeLiveness;
let computeHealthClass;
let Liveness;

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
    ({ computeLiveness, computeHealthClass, Liveness } = await import('@g8ed/public/js/components/operator-list-mixin.js'));
});

function createPanel(initialOperators) {
    const eventBus = { on: vi.fn(), off: vi.fn(), emit: vi.fn() };
    const panel = new OperatorPanel(eventBus);
    panel.operators = initialOperators;
    panel._isRendered = false;
    panel._heartbeatDirty = false;
    panel._fullRenderTimerId = null;
    panel.displayOperators = vi.fn();
    panel._patchOperatorCard = vi.fn();
    panel.updateMetrics = vi.fn();
    panel.updateStatus = vi.fn();
    panel.updatePanelStatusFromOperatorCounts = vi.fn();
    panel.updateBindAllButtonVisibility = vi.fn();
    panel.updateUnbindAllButtonVisibility = vi.fn();
    return panel;
}

describe('OperatorPanel._onHeartbeat [UNIT - PURE LOGIC]', () => {
    it('merges heartbeat metrics into the matching slot by operator_id', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE, latest_heartbeat_snapshot: null },
            { operator_id: 'op-2', status: OperatorStatus.AVAILABLE, latest_heartbeat_snapshot: null },
        ]);

        panel._onHeartbeat({
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
            metrics: {
                timestamp: '2026-04-21T21:30:10.683Z',
                performance: { cpu_percent: 42.1, memory_percent: 60.3, disk_percent: 55.2, network_latency: 12 },
                system_identity: { hostname: 'host-a', os: 'linux' },
            },
        });

        const updated = panel.operators[0];
        expect(updated.latest_heartbeat_snapshot).toBeInstanceOf(HeartbeatSnapshot);
        expect(updated.latest_heartbeat_snapshot.performance.cpu_percent).toBe(42.1);
        expect(updated.latest_heartbeat_snapshot.performance.memory_percent).toBe(60.3);
        expect(updated.latest_heartbeat_snapshot.performance.disk_percent).toBe(55.2);
        expect(updated.latest_heartbeat_snapshot.performance.network_latency).toBe(12);
        expect(updated.latest_heartbeat_snapshot.system_identity.hostname).toBe('host-a');
        expect(updated.status).toBe(OperatorStatus.ACTIVE);
        expect(updated.status_display).toBe(String(OperatorStatus.ACTIVE).toUpperCase());
        expect(updated.status_class).toBe(String(OperatorStatus.ACTIVE).toLowerCase());

        // Unrelated slot untouched
        expect(panel.operators[1].latest_heartbeat_snapshot).toBeNull();
    });

    it('sets _heartbeatDirty = true and calls _patchOperatorCard, not displayOperators', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE, latest_heartbeat_snapshot: null },
        ]);

        panel._onHeartbeat({
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
            metrics: {
                timestamp: '2026-04-21T21:30:10.683Z',
                performance: { cpu_percent: 42.1 },
            },
        });

        expect(panel._heartbeatDirty).toBe(true);
        expect(panel._patchOperatorCard).toHaveBeenCalledWith('op-1');
        expect(panel.displayOperators).not.toHaveBeenCalled();
    });

    it('updates metrics panel for selected operator on heartbeat', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.ACTIVE, latest_heartbeat_snapshot: null },
        ]);
        panel.selectedMetricsOperatorId = 'op-1';

        panel._onHeartbeat({
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
            metrics: {
                timestamp: '2026-04-21T21:30:10.683Z',
                performance: { cpu_percent: 42.1 },
            },
        });

        expect(panel.updateMetrics).toHaveBeenCalled();
        expect(panel.updateStatus).toHaveBeenCalledWith(OperatorStatus.ACTIVE);
    });

    it('leaves operators untouched when no operator_id matches', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE, latest_heartbeat_snapshot: null },
        ]);

        panel._onHeartbeat({
            operator_id: 'other',
            status: OperatorStatus.ACTIVE,
            metrics: {},
        });

        expect(panel.operators[0].latest_heartbeat_snapshot).toBeNull();
    });

    it('processes heartbeats even when not authenticated', () => {
        global.window.authState = { getState: () => ({ isAuthenticated: false }) };

        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE, latest_heartbeat_snapshot: null },
        ]);

        panel._onHeartbeat({
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
            metrics: { performance: { cpu_percent: 99 } },
        });

        expect(panel.operators[0].latest_heartbeat_snapshot).not.toBeNull();
        expect(panel.operators[0].latest_heartbeat_snapshot.performance.cpu_percent).toBe(99);
    });
});

describe('OperatorPanel._onStatusUpdated [UNIT - PURE LOGIC]', () => {
    it('applies status, status_display, status_class, and bound_web_session_id to the matching slot', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE, status_display: 'AVAILABLE', status_class: 'available', bound_web_session_id: null },
        ]);

        panel._onStatusUpdated({
            operator_id: 'op-1',
            status: OperatorStatus.BOUND,
            web_session_id: 'ws-42',
        });

        const updated = panel.operators[0];
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
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
        });

        expect(panel.operators[0].status).toBe(OperatorStatus.ACTIVE);
        expect(panel.operators[0].bound_web_session_id).toBe('ws-existing');
    });

    it('does not mutate any slot when operator_id does not match', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);

        panel._onStatusUpdated({
            operator_id: 'missing',
            status: OperatorStatus.ACTIVE,
        });

        expect(panel.operators[0].status).toBe(OperatorStatus.AVAILABLE);
    });

    it('updates aggregate counts from event payload', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);

        panel._onStatusUpdated({
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
            total_count: 7,
            active_count: 3,
        });

        expect(panel.totalOperatorCount).toBe(7);
        expect(panel.activeOperatorCount).toBe(3);
    });

    it('clears _heartbeatDirty after immediate render', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);
        panel._isRendered = true;
        panel._heartbeatDirty = true;

        panel._onStatusUpdated({
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
        });

        expect(panel._heartbeatDirty).toBe(false);
    });
});

describe('OperatorPanel.heartbeat buffering [UNIT - PURE LOGIC]', () => {
    it('_onListUpdated clears _heartbeatDirty after immediate render', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);
        panel._isRendered = true;
        panel._heartbeatDirty = true;

        panel._onListUpdated({
            operators: panel.operators,
            total_count: 1,
            active_count: 1,
            used_slots: 0,
            max_slots: 1,
        });

        expect(panel._heartbeatDirty).toBe(false);
    });

    it('scheduled full render with _heartbeatDirty = true triggers render and clears flag', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.ACTIVE },
        ]);
        panel._isRendered = true;
        panel._heartbeatDirty = true;
        panel.isCollapsed = false;

        panel._triggerRender({ cause: 'scheduled' });

        expect(panel.displayOperators).toHaveBeenCalled();
        expect(panel._heartbeatDirty).toBe(false);
    });

    it('scheduled full render with _heartbeatDirty = false is a no-op', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.ACTIVE },
        ]);
        panel._isRendered = true;
        panel._heartbeatDirty = false;
        panel.isCollapsed = false;

        panel._triggerRender({ cause: 'scheduled' });

        expect(panel.displayOperators).toHaveBeenCalled();
        expect(panel._heartbeatDirty).toBe(false);
    });
});

describe('OperatorPanel.state buffering during render [UNIT - PURE LOGIC]', () => {
    it('_triggerRender sets flag when not rendered, returns early', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);
        panel._isRendered = false;
        panel._statePendingDuringRender = false;

        panel._triggerRender({ cause: 'list_updated' });

        expect(panel._statePendingDuringRender).toBe(true);
        expect(panel.displayOperators).not.toHaveBeenCalled();
    });

    it('_triggerRender applies state immediately when already rendered', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);
        panel._isRendered = true;
        panel._statePendingDuringRender = false;

        panel._triggerRender({ cause: 'list_updated' });

        expect(panel.displayOperators).toHaveBeenCalled();
        expect(panel._statePendingDuringRender).toBe(false);
    });

    it('multiple state updates during render set flag each time, state accumulates', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);
        panel._isRendered = false;
        panel._statePendingDuringRender = false;

        panel._onListUpdated({
            operators: [{ operator_id: 'op-1', status: OperatorStatus.ACTIVE }],
            total_count: 1,
            active_count: 1,
            used_slots: 0,
            max_slots: 1,
        });

        expect(panel._statePendingDuringRender).toBe(true);
        expect(panel.operators[0].status).toBe(OperatorStatus.ACTIVE);
        expect(panel.displayOperators).not.toHaveBeenCalled();

        panel._onStatusUpdated({
            operator_id: 'op-1',
            status: OperatorStatus.BOUND,
            web_session_id: 'ws-42',
        });

        expect(panel._statePendingDuringRender).toBe(true);
        expect(panel.operators[0].status).toBe(OperatorStatus.BOUND);
        expect(panel.operators[0].bound_web_session_id).toBe('ws-42');
        expect(panel.displayOperators).not.toHaveBeenCalled();
    });

    it('after render completes, buffered state is applied with render_complete cause', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);
        panel._isRendered = false;
        panel._statePendingDuringRender = false;

        panel._onListUpdated({
            operators: [{ operator_id: 'op-1', status: OperatorStatus.ACTIVE }],
            total_count: 1,
            active_count: 1,
            used_slots: 0,
            max_slots: 1,
        });

        expect(panel._statePendingDuringRender).toBe(true);

        panel._isRendered = true;
        panel._triggerRender({ cause: 'render_complete' });

        expect(panel.displayOperators).toHaveBeenCalled();
        expect(panel._statePendingDuringRender).toBe(false);
        expect(panel.operators[0].status).toBe(OperatorStatus.ACTIVE);
    });

    it('flag is cleared after applying buffered state', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);
        panel._isRendered = false;
        panel._statePendingDuringRender = false;

        panel._onListUpdated({
            operators: panel.operators,
            total_count: 1,
            active_count: 1,
            used_slots: 0,
            max_slots: 1,
        });

        expect(panel._statePendingDuringRender).toBe(true);

        panel._isRendered = true;
        panel._triggerRender({ cause: 'render_complete' });

        expect(panel._statePendingDuringRender).toBe(false);
    });

    it('stores pending list update data when arriving before render completes', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);
        panel._isRendered = false;
        panel._pendingListUpdateData = null;

        const listData = {
            operators: [{ operator_id: 'op-1', status: OperatorStatus.ACTIVE }],
            total_count: 1,
            active_count: 1,
            used_slots: 0,
            max_slots: 1,
        };

        panel._onListUpdated(listData);

        expect(panel._pendingListUpdateData).toEqual(listData);
        expect(panel._statePendingDuringRender).toBe(true);
    });

    it('does not store pending list update data when already rendered', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);
        panel._isRendered = true;
        panel._pendingListUpdateData = null;

        const listData = {
            operators: [{ operator_id: 'op-1', status: OperatorStatus.ACTIVE }],
            total_count: 1,
            active_count: 1,
            used_slots: 0,
            max_slots: 1,
        };

        panel._onListUpdated(listData);

        expect(panel._pendingListUpdateData).toBeNull();
    });

    it('clears pending list update data after render completes', () => {
        const panel = createPanel([
            { operator_id: 'op-1', status: OperatorStatus.AVAILABLE },
        ]);
        panel._isRendered = false;
        panel._pendingListUpdateData = null;

        const listData = {
            operators: [{ operator_id: 'op-1', status: OperatorStatus.ACTIVE }],
            total_count: 1,
            active_count: 1,
            used_slots: 0,
            max_slots: 1,
        };

        panel._onListUpdated(listData);
        expect(panel._pendingListUpdateData).toEqual(listData);

        panel._isRendered = true;
        panel._triggerRender({ cause: 'render_complete' });
        panel._pendingListUpdateData = null;

        expect(panel._pendingListUpdateData).toBeNull();
    });
});

describe('computeLiveness [UNIT - PURE LOGIC]', () => {
    it('returns NONE when latest_heartbeat_snapshot is missing', () => {
        const operator = { operator_id: 'op-1', status: OperatorStatus.ACTIVE, latest_heartbeat_snapshot: null };
        const result = computeLiveness(operator);
        expect(result.state).toBe(Liveness.NONE);
        expect(result.ageSeconds).toBeNull();
    });

    it('returns LIVE for age ≤ 30s', () => {
        const nowMs = Date.now();
        const operator = {
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
            latest_heartbeat_snapshot: { timestamp: new Date(nowMs - 15000).toISOString() },
        };
        const result = computeLiveness(operator, nowMs);
        expect(result.state).toBe(Liveness.LIVE);
        expect(result.ageSeconds).toBe(15);
    });

    it('returns DEGRADED for age 30–90s', () => {
        const nowMs = Date.now();
        const operator = {
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
            latest_heartbeat_snapshot: { timestamp: new Date(nowMs - 60000).toISOString() },
        };
        const result = computeLiveness(operator, nowMs);
        expect(result.state).toBe(Liveness.DEGRADED);
        expect(result.ageSeconds).toBe(60);
    });

    it('returns STALE for age > 90s', () => {
        const nowMs = Date.now();
        const operator = {
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
            latest_heartbeat_snapshot: { timestamp: new Date(nowMs - 120000).toISOString() },
        };
        const result = computeLiveness(operator, nowMs);
        expect(result.state).toBe(Liveness.STALE);
        expect(result.ageSeconds).toBe(120);
    });
});

describe('computeHealthClass [UNIT - PURE LOGIC]', () => {
    it('returns muted when liveness is NONE, regardless of operator.status === active', () => {
        const operator = { operator_id: 'op-1', status: OperatorStatus.ACTIVE, latest_heartbeat_snapshot: null };
        const result = computeHealthClass(operator, 'good', 'good', 'good', 'good');
        expect(result).toBe('muted');
    });

    it('returns muted when liveness is STALE', () => {
        const nowMs = Date.now();
        const operator = {
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
            latest_heartbeat_snapshot: { timestamp: new Date(nowMs - 120000).toISOString() },
        };
        const result = computeHealthClass(operator, 'good', 'good', 'good', 'good');
        expect(result).toBe('muted');
    });

    it('returns loaded when liveness is DEGRADED and metrics are good', () => {
        const nowMs = Date.now();
        const operator = {
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
            latest_heartbeat_snapshot: { timestamp: new Date(nowMs - 60000).toISOString() },
        };
        const result = computeHealthClass(operator, 'good', 'good', 'good', 'good');
        expect(result).toBe('loaded');
    });

    it('returns healthy only when liveness is LIVE and metrics are good', () => {
        const nowMs = Date.now();
        const operator = {
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
            latest_heartbeat_snapshot: { timestamp: new Date(nowMs - 15000).toISOString() },
        };
        const result = computeHealthClass(operator, 'good', 'good', 'good', 'good');
        expect(result).toBe('healthy');
    });

    it('returns crit when liveness is LIVE but metrics include crit', () => {
        const nowMs = Date.now();
        const operator = {
            operator_id: 'op-1',
            status: OperatorStatus.ACTIVE,
            latest_heartbeat_snapshot: { timestamp: new Date(nowMs - 15000).toISOString() },
        };
        const result = computeHealthClass(operator, 'crit', 'good', 'good', 'good');
        expect(result).toBe('crit');
    });
});
