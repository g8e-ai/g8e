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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
    HeartbeatMonitorService,
    resolveHeartbeatTransition,
} from '../../../../services/operator/heartbeat_monitor_service.js';
import { OperatorStatus, operatorStatusToEventType } from '../../../../constants/operator.js';
import { EventType } from '../../../../constants/events.js';

const THRESHOLD_SECONDS = 60;

function makeOperator(overrides = {}) {
    return {
        operator_id: 'op-1',
        user_id: 'user-1',
        status: OperatorStatus.BOUND,
        last_heartbeat: new Date(Date.now() - 10_000),
        system_info: { hostname: 'node-02' },
        system_fingerprint: 'fp-1',
        ...overrides,
    };
}

describe('resolveHeartbeatTransition', () => {
    it('transitions BOUND -> STALE when stale', () => {
        expect(resolveHeartbeatTransition(OperatorStatus.BOUND, true)).toBe(OperatorStatus.STALE);
    });

    it('transitions ACTIVE -> OFFLINE when stale', () => {
        expect(resolveHeartbeatTransition(OperatorStatus.ACTIVE, true)).toBe(OperatorStatus.OFFLINE);
    });

    it('recovers STALE -> BOUND when fresh', () => {
        expect(resolveHeartbeatTransition(OperatorStatus.STALE, false)).toBe(OperatorStatus.BOUND);
    });

    it('recovers OFFLINE -> ACTIVE when fresh', () => {
        expect(resolveHeartbeatTransition(OperatorStatus.OFFLINE, false)).toBe(OperatorStatus.ACTIVE);
    });

    it('returns null when no transition is required', () => {
        expect(resolveHeartbeatTransition(OperatorStatus.BOUND, false)).toBeNull();
        expect(resolveHeartbeatTransition(OperatorStatus.ACTIVE, false)).toBeNull();
        expect(resolveHeartbeatTransition(OperatorStatus.STALE, true)).toBeNull();
        expect(resolveHeartbeatTransition(OperatorStatus.OFFLINE, true)).toBeNull();
    });

    it('ignores terminal/unbound statuses', () => {
        expect(resolveHeartbeatTransition(OperatorStatus.AVAILABLE, true)).toBeNull();
        expect(resolveHeartbeatTransition(OperatorStatus.TERMINATED, true)).toBeNull();
        expect(resolveHeartbeatTransition(OperatorStatus.STOPPED, true)).toBeNull();
    });
});

describe('HeartbeatMonitorService.tick', () => {
    let operatorDataService;
    let sseService;
    let service;

    beforeEach(() => {
        operatorDataService = {
            queryOperators: vi.fn().mockResolvedValue([]),
            updateOperator: vi.fn().mockResolvedValue({ success: true }),
        };
        sseService = {
            publishToUser: vi.fn().mockResolvedValue(1),
        };
        service = new HeartbeatMonitorService({
            operatorDataService,
            sseService,
            thresholdSeconds: THRESHOLD_SECONDS,
            intervalMs: 1_000_000, // irrelevant, tick() is invoked directly
        });
    });

    afterEach(() => {
        service.stop();
        vi.restoreAllMocks();
    });

    it('transitions a stale BOUND operator to STALE and fans out SSE', async () => {
        const op = makeOperator({
            status: OperatorStatus.BOUND,
            last_heartbeat: new Date(Date.now() - (THRESHOLD_SECONDS + 30) * 1000),
        });
        operatorDataService.queryOperators.mockResolvedValue([op]);

        await service.tick();

        expect(operatorDataService.updateOperator).toHaveBeenCalledWith(
            'op-1',
            expect.objectContaining({ status: OperatorStatus.STALE }),
        );
        expect(sseService.publishToUser).toHaveBeenCalledWith(
            'user-1',
            expect.objectContaining({
                type: operatorStatusToEventType(OperatorStatus.STALE),
            }),
        );
        const event = sseService.publishToUser.mock.calls[0][1];
        expect(event.type).toBe(EventType.OPERATOR_STATUS_UPDATED_STALE);
        expect(event.data.operator_id).toBe('op-1');
        expect(event.data.status).toBe(OperatorStatus.STALE);
    });

    it('transitions a stale ACTIVE operator to OFFLINE', async () => {
        operatorDataService.queryOperators.mockResolvedValue([makeOperator({
            status: OperatorStatus.ACTIVE,
            last_heartbeat: new Date(Date.now() - (THRESHOLD_SECONDS + 10) * 1000),
        })]);

        await service.tick();

        expect(operatorDataService.updateOperator).toHaveBeenCalledWith(
            'op-1',
            expect.objectContaining({ status: OperatorStatus.OFFLINE }),
        );
    });

    it('recovers STALE -> BOUND when heartbeat is fresh', async () => {
        operatorDataService.queryOperators.mockResolvedValue([makeOperator({
            status: OperatorStatus.STALE,
            last_heartbeat: new Date(Date.now() - 5_000),
        })]);

        await service.tick();

        expect(operatorDataService.updateOperator).toHaveBeenCalledWith(
            'op-1',
            expect.objectContaining({ status: OperatorStatus.BOUND }),
        );
    });

    it('recovers OFFLINE -> ACTIVE when heartbeat is fresh', async () => {
        operatorDataService.queryOperators.mockResolvedValue([makeOperator({
            status: OperatorStatus.OFFLINE,
            last_heartbeat: new Date(Date.now() - 5_000),
        })]);

        await service.tick();

        expect(operatorDataService.updateOperator).toHaveBeenCalledWith(
            'op-1',
            expect.objectContaining({ status: OperatorStatus.ACTIVE }),
        );
    });

    it('does not touch operators without a last_heartbeat', async () => {
        operatorDataService.queryOperators.mockResolvedValue([makeOperator({
            status: OperatorStatus.BOUND,
            last_heartbeat: null,
        })]);

        await service.tick();

        expect(operatorDataService.updateOperator).not.toHaveBeenCalled();
        expect(sseService.publishToUser).not.toHaveBeenCalled();
    });

    it('ignores operators whose status is not monitored', async () => {
        operatorDataService.queryOperators.mockResolvedValue([
            makeOperator({ status: OperatorStatus.AVAILABLE, last_heartbeat: new Date(0) }),
            makeOperator({ operator_id: 'op-2', status: OperatorStatus.TERMINATED, last_heartbeat: new Date(0) }),
            makeOperator({ operator_id: 'op-3', status: OperatorStatus.STOPPED, last_heartbeat: new Date(0) }),
        ]);

        await service.tick();

        expect(operatorDataService.updateOperator).not.toHaveBeenCalled();
    });

    it('skips SSE fan-out but still persists when publish fails', async () => {
        sseService.publishToUser.mockRejectedValue(new Error('sse down'));
        operatorDataService.queryOperators.mockResolvedValue([makeOperator({
            status: OperatorStatus.BOUND,
            last_heartbeat: new Date(Date.now() - (THRESHOLD_SECONDS + 10) * 1000),
        })]);

        await expect(service.tick()).resolves.toBeUndefined();
        expect(operatorDataService.updateOperator).toHaveBeenCalled();
    });

    it('coalesces concurrent ticks', async () => {
        let resolveQuery;
        operatorDataService.queryOperators.mockImplementation(
            () => new Promise(resolve => { resolveQuery = resolve; }),
        );

        const first = service.tick();
        const second = service.tick();
        resolveQuery([]);
        await Promise.all([first, second]);

        expect(operatorDataService.queryOperators).toHaveBeenCalledTimes(1);
    });
});

describe('HeartbeatMonitorService lifecycle', () => {
    it('requires operatorDataService and sseService', () => {
        expect(() => new HeartbeatMonitorService({ sseService: {} })).toThrow(/operatorDataService/);
        expect(() => new HeartbeatMonitorService({ operatorDataService: {} })).toThrow(/sseService/);
    });

    it('start() is idempotent and stop() clears the timer', () => {
        const svc = new HeartbeatMonitorService({
            operatorDataService: { queryOperators: vi.fn().mockResolvedValue([]), updateOperator: vi.fn() },
            sseService: { publishToUser: vi.fn() },
            intervalMs: 1_000_000,
        });
        svc.start();
        const timer = svc._timer;
        svc.start();
        expect(svc._timer).toBe(timer);
        svc.stop();
        expect(svc._timer).toBeNull();
    });
});
