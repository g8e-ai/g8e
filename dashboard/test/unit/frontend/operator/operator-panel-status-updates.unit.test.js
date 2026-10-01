// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { OperatorStatus } from '@g8ed/public/js/constants/operator-constants.js';
import { EventType } from '@g8ed/public/js/constants/events.js';

let OperatorPanel;
let operatorPanelService;

const OPERATOR_ID = 'op_status_1';
const OTHER_OPERATOR_ID = 'op_status_2';

function createEventBus() {
    const handlers = new Map();
    return {
        on: vi.fn((type, handler) => handlers.set(type, handler)),
        off: vi.fn(),
        emit: vi.fn(),
        deliver(type, data) {
            handlers.get(type)(data);
        },
    };
}

function createPanel(operators) {
    const eventBus = createEventBus();
    const panel = new OperatorPanel(eventBus);
    panel._operators = operators;
    panel._totalOperatorCount = operators.length;
    panel._activeOperatorCount = operators.filter(
        op => op.status === OperatorStatus.ACTIVE || op.status === OperatorStatus.BOUND
    ).length;
    panel._applyOperatorState = vi.fn();
    return { panel, eventBus };
}

function operator(id, status) {
    return { operator_id: id, status, name: id };
}

beforeEach(async () => {
    vi.resetModules();

    vi.doMock('@g8ed/public/js/utils/dev-logger.js', () => ({
        devLogger: { log: vi.fn(), error: vi.fn(), warn: vi.fn() },
    }));
    vi.doMock('@g8ed/public/js/utils/operator-session-service.js', () => ({
        operatorSessionService: { setBoundOperators: vi.fn() },
    }));
    vi.doMock('@g8ed/public/js/utils/operator-panel-service.js', () => ({
        operatorPanelService: {
            listOperators: vi.fn().mockResolvedValue({ operators: [] }),
        },
    }));

    ({ OperatorPanel } = await import('@g8ed/public/js/components/operator-panel.js'));
    ({ operatorPanelService } = await import('@g8ed/public/js/utils/operator-panel-service.js'));
});

afterEach(() => {
    vi.restoreAllMocks();
});

describe('OperatorPanel status.updated handling [UNIT - jsdom]', () => {

    describe.each([
        ['stale', EventType.OPERATOR_STATUS_UPDATED_STALE, OperatorStatus.STALE],
        ['stopped', EventType.OPERATOR_STATUS_UPDATED_STOPPED, OperatorStatus.STOPPED],
        ['terminated', EventType.OPERATOR_STATUS_UPDATED_TERMINATED, OperatorStatus.TERMINATED],
        ['offline', EventType.OPERATOR_STATUS_UPDATED_OFFLINE, OperatorStatus.OFFLINE],
    ])('%s event', (_name, eventType, status) => {

        it('updates the status of the matching operator and leaves the others alone', () => {
            const { panel, eventBus } = createPanel([
                operator(OPERATOR_ID, OperatorStatus.ACTIVE),
                operator(OTHER_OPERATOR_ID, OperatorStatus.ACTIVE),
            ]);

            eventBus.deliver(eventType, { operator_id: OPERATOR_ID, status, name: OPERATOR_ID });

            expect(panel._operators.find(op => op.operator_id === OPERATOR_ID).status).toBe(status);
            expect(panel._operators.find(op => op.operator_id === OTHER_OPERATOR_ID).status).toBe(OperatorStatus.ACTIVE);
        });

        it('drops the operator from the active count and re-renders', () => {
            const { panel, eventBus } = createPanel([
                operator(OPERATOR_ID, OperatorStatus.ACTIVE),
                operator(OTHER_OPERATOR_ID, OperatorStatus.ACTIVE),
            ]);

            eventBus.deliver(eventType, { operator_id: OPERATOR_ID, status });

            expect(panel._totalOperatorCount).toBe(2);
            expect(panel._activeOperatorCount).toBe(1);
            expect(panel._applyOperatorState).toHaveBeenCalledWith({ cause: 'status_updated' });
        });
    });

    it('returns a recovered operator to the active count', () => {
        const { panel, eventBus } = createPanel([operator(OPERATOR_ID, OperatorStatus.STALE)]);

        eventBus.deliver(EventType.OPERATOR_STATUS_UPDATED_ACTIVE, {
            operator_id: OPERATOR_ID,
            status: OperatorStatus.ACTIVE,
        });

        expect(panel._operators[0].status).toBe(OperatorStatus.ACTIVE);
        expect(panel._activeOperatorCount).toBe(1);
    });

    it('re-fetches the list for an operator the panel does not hold', () => {
        const { panel, eventBus } = createPanel([operator(OTHER_OPERATOR_ID, OperatorStatus.ACTIVE)]);

        eventBus.deliver(EventType.OPERATOR_STATUS_UPDATED_STALE, {
            operator_id: OPERATOR_ID,
            status: OperatorStatus.STALE,
        });

        expect(operatorPanelService.listOperators).toHaveBeenCalledTimes(1);
        expect(panel._operators).toHaveLength(1);
    });

    it('marks the panel disconnected when a bound operator goes stale', () => {
        const { panel, eventBus } = createPanel([operator(OPERATOR_ID, OperatorStatus.BOUND)]);
        panel._isConnected = true;
        panel._lastHeartbeat = Date.now();

        eventBus.deliver(EventType.OPERATOR_STATUS_UPDATED_STALE, {
            operator_id: OPERATOR_ID,
            status: OperatorStatus.STALE,
        });

        expect(panel._isConnected).toBe(false);
        expect(panel._lastHeartbeat).toBeNull();
    });

    it('keeps the panel connected when an unbound operator goes stale', () => {
        const { panel, eventBus } = createPanel([operator(OPERATOR_ID, OperatorStatus.ACTIVE)]);
        panel._isConnected = true;

        eventBus.deliver(EventType.OPERATOR_STATUS_UPDATED_STALE, {
            operator_id: OPERATOR_ID,
            status: OperatorStatus.STALE,
        });

        expect(panel._isConnected).toBe(true);
    });

    it('replaces the operator when the event carries the full operator_data', () => {
        const { panel, eventBus } = createPanel([operator(OPERATOR_ID, OperatorStatus.ACTIVE)]);
        const updated = { operator_id: OPERATOR_ID, status: OperatorStatus.STOPPED, name: 'renamed' };

        eventBus.deliver(EventType.OPERATOR_STATUS_UPDATED_STOPPED, {
            operator_id: OPERATOR_ID,
            operator_data: updated,
        });

        expect(panel._operators[0]).toEqual(updated);
        expect(operatorPanelService.listOperators).not.toHaveBeenCalled();
    });
});
