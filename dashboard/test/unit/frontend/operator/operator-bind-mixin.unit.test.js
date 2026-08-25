// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { OperatorStatus } from '@g8ed/public/js/constants/operator-constants.js';
import { EventType } from '@g8ed/public/js/constants/events.js';
import { TEMPLATE_FIXTURES, seedTemplates } from '@test/fixtures/templates.fixture.js';

let BindOperatorsMixin;
let operatorPanelService;
let templateLoader;
let devLogger;

const TEST_OPERATOR_ID = 'op_test_123';
const TEST_OPERATOR_ID_2 = 'op_test_456';
const TEST_WEB_SESSION_ID = 'ws_test_789';

function createMixinContext(overrides = {}) {
    const ctx = Object.create(null);
    Object.assign(ctx, BindOperatorsMixin);
    ctx.operators = [];
    ctx.boundOperatorIds = [];
    ctx.eventBus = {
        emit: vi.fn(),
        on: vi.fn(),
        off: vi.fn(),
    };
    ctx.selectedMetricsOperatorId = null;
    ctx.isConnected = false;
    ctx.updateMetrics = vi.fn();
    ctx.updateStatus = vi.fn();
    ctx.clearPanelMetrics = vi.fn();
    ctx.bindAllBtn = null;
    ctx.unbindAllBtn = null;
    ctx._escapeHtml = (text) => {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    };
    Object.assign(ctx, overrides);
    return ctx;
}

function createMockOperator(overrides = {}) {
    return {
        operator_id: TEST_OPERATOR_ID,
        status: OperatorStatus.ACTIVE,
        system_info: {
            hostname: 'test-host',
            os: 'Ubuntu 22.04',
            private_ip: '192.168.1.100',
            public_ip: '203.0.113.1',
        },
        web_session_id: TEST_WEB_SESSION_ID,
        ...overrides,
    };
}

beforeEach(async () => {
    vi.resetModules();
    document.body.innerHTML = '';

    vi.doMock('@g8ed/public/js/utils/dev-logger.js', () => ({
        devLogger: { log: vi.fn(), error: vi.fn(), warn: vi.fn() },
    }));

    vi.doMock('@g8ed/public/js/utils/operator-panel-service.js', () => ({
        operatorPanelService: {
            bindOperator: vi.fn(),
            unbindOperator: vi.fn(),
            bindAllOperators: vi.fn(),
            unbindAllOperators: vi.fn(),
        },
    }));

    vi.doMock('@g8ed/public/js/utils/template-loader.js', () => ({
        templateLoader: {
            cache: new Map(),
            seed: vi.fn((name, html) => {
                templateLoader.cache.set(name, html);
            }),
            replace: vi.fn((template, vars) => {
                let result = template;
                for (const [key, value] of Object.entries(vars)) {
                    result = result.replace(new RegExp(`\\{\\{\\{${key}\\}\\}\\}`, 'g'), value ?? '');
                    result = result.replace(new RegExp(`\\{\\{${key}\\}\\}`, 'g'), String(value ?? ''));
                }
                return result;
            }),
        },
    }));

    const mod = await import('@g8ed/public/js/components/operator-bind-mixin.js');
    BindOperatorsMixin = mod.BindOperatorsMixin;

    const opsMod = await import('@g8ed/public/js/utils/operator-panel-service.js');
    operatorPanelService = opsMod.operatorPanelService;

    const tlMod = await import('@g8ed/public/js/utils/template-loader.js');
    templateLoader = tlMod.templateLoader;

    const dlMod = await import('@g8ed/public/js/utils/dev-logger.js');
    devLogger = dlMod.devLogger;

    seedTemplates(templateLoader, [
        'bind-single-confirmation-overlay',
        'bind-all-confirmation-overlay',
        'unbind-all-confirmation-overlay',
        'bind-all-operator-item',
        'bind-result-feedback',
    ]);
});

afterEach(() => {
    vi.restoreAllMocks();
    document.body.innerHTML = '';
});

describe('BindOperatorsMixin [UNIT - jsdom]', () => {

    describe('bindOperator', () => {
        it('calls operatorPanelService.bindOperator with operator ID', async () => {
            const ctx = createMixinContext();
            const updateBindAllSpy = vi.spyOn(BindOperatorsMixin, 'updateBindAllButtonVisibility');
            const updateUnbindAllSpy = vi.spyOn(BindOperatorsMixin, 'updateUnbindAllButtonVisibility');
            operatorPanelService.bindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({ operator: createMockOperator() }),
            });

            await ctx.bindOperator(TEST_OPERATOR_ID);

            expect(operatorPanelService.bindOperator).toHaveBeenCalledWith(TEST_OPERATOR_ID);
            updateBindAllSpy.mockRestore();
            updateUnbindAllSpy.mockRestore();
        });

        it('adds operator ID to boundOperatorIds on successful bind', async () => {
            const ctx = createMixinContext();
            operatorPanelService.bindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({ operator: createMockOperator() }),
            });

            await ctx.bindOperator(TEST_OPERATOR_ID);

            expect(ctx.boundOperatorIds).toContain(TEST_OPERATOR_ID);
        });

        it('emits OPERATOR_BOUND event', async () => {
            const ctx = createMixinContext();
            const mockOperator = createMockOperator();
            operatorPanelService.bindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({ operator: mockOperator }),
            });

            await ctx.bindOperator(TEST_OPERATOR_ID);

            expect(ctx.eventBus.emit).toHaveBeenCalledWith(EventType.OPERATOR_BOUND, {
                operator_id: TEST_OPERATOR_ID,
                operator: mockOperator,
            });
        });

        it('calls updateBindAllButtonVisibility after successful bind', async () => {
            const ctx = createMixinContext();
            const spy = vi.spyOn(ctx, 'updateBindAllButtonVisibility');
            operatorPanelService.bindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({ operator: createMockOperator() }),
            });

            await ctx.bindOperator(TEST_OPERATOR_ID);

            expect(spy).toHaveBeenCalled();
            spy.mockRestore();
        });

        it('calls updateUnbindAllButtonVisibility after successful bind', async () => {
            const ctx = createMixinContext();
            const spy = vi.spyOn(ctx, 'updateUnbindAllButtonVisibility');
            operatorPanelService.bindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({ operator: createMockOperator() }),
            });

            await ctx.bindOperator(TEST_OPERATOR_ID);

            expect(spy).toHaveBeenCalled();
            spy.mockRestore();
        });

        it('sets selectedMetricsOperatorId if not set', async () => {
            const ctx = createMixinContext();
            operatorPanelService.bindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({ operator: createMockOperator() }),
            });

            await ctx.bindOperator(TEST_OPERATOR_ID);

            expect(ctx.selectedMetricsOperatorId).toBe(TEST_OPERATOR_ID);
        });

        it('calls updateMetrics with operator on successful bind', async () => {
            const ctx = createMixinContext();
            const mockOperator = createMockOperator();
            operatorPanelService.bindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({ operator: mockOperator }),
            });

            await ctx.bindOperator(TEST_OPERATOR_ID);

            expect(ctx.updateMetrics).toHaveBeenCalledWith(mockOperator);
        });

        it('calls updateStatus with operator status on successful bind', async () => {
            const ctx = createMixinContext();
            operatorPanelService.bindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({ operator: createMockOperator() }),
            });

            await ctx.bindOperator(TEST_OPERATOR_ID);

            expect(ctx.updateStatus).toHaveBeenCalledWith(OperatorStatus.ACTIVE);
        });

        it('alerts when bind fails', async () => {
            const ctx = createMixinContext();
            operatorPanelService.bindOperator.mockResolvedValue({
                ok: false,
                json: async () => ({ error: 'Failed to bind' }),
            });
            global.alert = vi.fn();

            await ctx.bindOperator(TEST_OPERATOR_ID);

            expect(global.alert).toHaveBeenCalledWith('Failed to bind operator: Failed to bind');
        });

        it('handles network errors', async () => {
            const ctx = createMixinContext();
            operatorPanelService.bindOperator.mockRejectedValue(new Error('Network failure'));
            global.alert = vi.fn();

            await ctx.bindOperator(TEST_OPERATOR_ID);

            expect(global.alert).toHaveBeenCalledWith('Failed to bind operator: Network failure');
        });
    });

    describe('unbindOperator', () => {
        it('calls operatorPanelService.unbindOperator with empty body when forceWithOperatorId is false', async () => {
            const ctx = createMixinContext();
            const updateBindAllSpy = vi.spyOn(BindOperatorsMixin, 'updateBindAllButtonVisibility');
            const updateUnbindAllSpy = vi.spyOn(BindOperatorsMixin, 'updateUnbindAllButtonVisibility');
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];
            operatorPanelService.unbindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({}),
            });

            await ctx.unbindOperator(TEST_OPERATOR_ID, false);

            expect(operatorPanelService.unbindOperator).toHaveBeenCalledWith({});
            updateBindAllSpy.mockRestore();
            updateUnbindAllSpy.mockRestore();
        });

        it('calls operatorPanelService.unbindOperator with operator_id in body when forceWithOperatorId is true', async () => {
            const ctx = createMixinContext();
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];
            operatorPanelService.unbindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({}),
            });

            await ctx.unbindOperator(TEST_OPERATOR_ID, true);

            expect(operatorPanelService.unbindOperator).toHaveBeenCalledWith({ operator_id: TEST_OPERATOR_ID });
        });

        it('removes operator ID from boundOperatorIds on successful unbind', async () => {
            const ctx = createMixinContext();
            ctx.boundOperatorIds = [TEST_OPERATOR_ID, TEST_OPERATOR_ID_2];
            operatorPanelService.unbindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({}),
            });

            await ctx.unbindOperator(TEST_OPERATOR_ID, false);

            expect(ctx.boundOperatorIds).not.toContain(TEST_OPERATOR_ID);
            expect(ctx.boundOperatorIds).toContain(TEST_OPERATOR_ID_2);
        });

        it('calls updateBindAllButtonVisibility after successful unbind', async () => {
            const ctx = createMixinContext();
            const spy = vi.spyOn(ctx, 'updateBindAllButtonVisibility');
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];
            operatorPanelService.unbindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({}),
            });

            await ctx.unbindOperator(TEST_OPERATOR_ID, false);

            expect(spy).toHaveBeenCalled();
            spy.mockRestore();
        });

        it('calls updateUnbindAllButtonVisibility after successful unbind', async () => {
            const ctx = createMixinContext();
            const spy = vi.spyOn(ctx, 'updateUnbindAllButtonVisibility');
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];
            operatorPanelService.unbindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({}),
            });

            await ctx.unbindOperator(TEST_OPERATOR_ID, false);

            expect(spy).toHaveBeenCalled();
            spy.mockRestore();
        });

        it('sets status to OFFLINE and clears metrics when last operator unbound', async () => {
            const ctx = createMixinContext();
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];
            operatorPanelService.unbindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({}),
            });

            await ctx.unbindOperator(TEST_OPERATOR_ID, false);

            expect(ctx.updateStatus).toHaveBeenCalledWith(OperatorStatus.OFFLINE);
            expect(ctx.isConnected).toBe(false);
            expect(ctx.clearPanelMetrics).toHaveBeenCalled();
        });

        it('alerts when unbind fails', async () => {
            const ctx = createMixinContext();
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];
            operatorPanelService.unbindOperator.mockResolvedValue({
                ok: false,
                json: async () => ({ error: 'Failed to unbind' }),
            });
            global.alert = vi.fn();

            await ctx.unbindOperator(TEST_OPERATOR_ID, false);

            expect(global.alert).toHaveBeenCalledWith('Failed to unbind operator: Failed to unbind');
        });
    });

    describe('bindOperatorWithConfirmation', () => {
        it('calls _showBindSingleModal with bind mode', async () => {
            const ctx = createMixinContext();
            ctx.operators = [createMockOperator()];
            const showModalSpy = vi.spyOn(ctx, '_showBindSingleModal').mockResolvedValue();

            await ctx.bindOperatorWithConfirmation(TEST_OPERATOR_ID);

            expect(showModalSpy).toHaveBeenCalledWith({
                operatorId: TEST_OPERATOR_ID,
                operator: ctx.operators[0],
                mode: 'bind',
            });
        });
    });

    describe('unbindOperatorWithConfirmation', () => {
        it('calls _showBindSingleModal with unbind mode when isStale is false', async () => {
            const ctx = createMixinContext();
            ctx.operators = [createMockOperator()];
            const showModalSpy = vi.spyOn(ctx, '_showBindSingleModal').mockResolvedValue();

            await ctx.unbindOperatorWithConfirmation(TEST_OPERATOR_ID, false);

            expect(showModalSpy).toHaveBeenCalledWith({
                operatorId: TEST_OPERATOR_ID,
                operator: ctx.operators[0],
                mode: 'unbind',
            });
        });

        it('calls _showBindSingleModal with unbind-stale mode when isStale is true', async () => {
            const ctx = createMixinContext();
            ctx.operators = [createMockOperator()];
            const showModalSpy = vi.spyOn(ctx, '_showBindSingleModal').mockResolvedValue();

            await ctx.unbindOperatorWithConfirmation(TEST_OPERATOR_ID, true);

            expect(showModalSpy).toHaveBeenCalledWith({
                operatorId: TEST_OPERATOR_ID,
                operator: ctx.operators[0],
                mode: 'unbind-stale',
            });
        });
    });

    describe('_showBindSingleModal', () => {
        it('resolves immediately if template not found', async () => {
            const ctx = createMixinContext();
            templateLoader.cache.delete('bind-single-confirmation-overlay');

            const result = await ctx._showBindSingleModal({
                operatorId: TEST_OPERATOR_ID,
                operator: createMockOperator(),
                mode: 'bind',
            });

            expect(result).toBeUndefined();
        });
    });

    describe('showBindAllConfirmationOverlay', () => {
        it('shows alert when no active operators available to bind', () => {
            const ctx = createMixinContext();
            ctx.operators = [
                createMockOperator({ status: OperatorStatus.BOUND }),
            ];
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];
            global.alert = vi.fn();

            ctx.showBindAllConfirmationOverlay();

            expect(global.alert).toHaveBeenCalledWith('No active operators available to bind. All active operators are already bound to this session.');
        });

        it('returns early when no active operators available', () => {
            const ctx = createMixinContext();
            ctx.operators = [createMockOperator({ status: OperatorStatus.BOUND })];
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];

            const result = ctx.showBindAllConfirmationOverlay();

            expect(result).toBeUndefined();
        });

        it('returns early if template not found', () => {
            const ctx = createMixinContext();
            templateLoader.cache.delete('bind-all-confirmation-overlay');

            const result = ctx.showBindAllConfirmationOverlay();

            expect(result).toBeUndefined();
        });

        it('filters active operators that are not bound', () => {
            const ctx = createMixinContext();
            ctx.operators = [
                createMockOperator({ operator_id: TEST_OPERATOR_ID, status: OperatorStatus.ACTIVE }),
                createMockOperator({ operator_id: TEST_OPERATOR_ID_2, status: OperatorStatus.BOUND }),
            ];
            ctx.boundOperatorIds = [TEST_OPERATOR_ID_2];

            ctx.showBindAllConfirmationOverlay();

            expect(ctx.bindAllOverlay).toBeDefined();
        });

        it('adds overlay to document body', () => {
            const ctx = createMixinContext();
            ctx.operators = [createMockOperator({ status: OperatorStatus.ACTIVE })];
            ctx.boundOperatorIds = [];

            ctx.showBindAllConfirmationOverlay();

            expect(document.body.contains(ctx.bindAllOverlay)).toBe(true);
        });
    });

    describe('executeBindAll', () => {
        beforeEach(() => {
            const overlay = document.createElement('div');
            overlay.className = 'bind-all-confirmation-overlay';
            overlay.innerHTML = `
                <button data-action="confirm">Confirm</button>
                <button data-action="cancel">Cancel</button>
                <div class="bind-all-actions"></div>
                <div data-processing-indicator class="initially-hidden"></div>
            `;
            document.body.appendChild(overlay);
        });

        it('calls operatorPanelService.bindAllOperators with operator IDs', async () => {
            const ctx = createMixinContext();
            const overlay = document.querySelector('.bind-all-confirmation-overlay');
            const activeOperators = [createMockOperator(), createMockOperator({ operator_id: TEST_OPERATOR_ID_2 })];
            operatorPanelService.bindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ bound_operator_ids: [TEST_OPERATOR_ID, TEST_OPERATOR_ID_2] }),
            });
            vi.useFakeTimers();

            await ctx.executeBindAll(overlay, activeOperators);
            vi.runAllTimers();

            expect(operatorPanelService.bindAllOperators).toHaveBeenCalledWith([TEST_OPERATOR_ID, TEST_OPERATOR_ID_2]);
            vi.useRealTimers();
        });

        it('adds bound operator IDs to boundOperatorIds', async () => {
            const ctx = createMixinContext();
            const overlay = document.querySelector('.bind-all-confirmation-overlay');
            const activeOperators = [createMockOperator()];
            operatorPanelService.bindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ bound_operator_ids: [TEST_OPERATOR_ID] }),
            });
            vi.useFakeTimers();

            await ctx.executeBindAll(overlay, activeOperators);
            vi.runAllTimers();

            expect(ctx.boundOperatorIds).toContain(TEST_OPERATOR_ID);
            vi.useRealTimers();
        });

        it('falls back to input operator IDs if response missing bound_operator_ids', async () => {
            const ctx = createMixinContext();
            const overlay = document.querySelector('.bind-all-confirmation-overlay');
            const activeOperators = [createMockOperator()];
            operatorPanelService.bindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({}),
            });
            vi.useFakeTimers();

            await ctx.executeBindAll(overlay, activeOperators);
            vi.runAllTimers();

            expect(ctx.boundOperatorIds).toContain(TEST_OPERATOR_ID);
            vi.useRealTimers();
        });

        it('shows error message on bind-all failure', async () => {
            const ctx = createMixinContext();
            const overlay = document.querySelector('.bind-all-confirmation-overlay');
            const activeOperators = [createMockOperator()];
            operatorPanelService.bindAllOperators.mockResolvedValue({
                ok: false,
                json: async () => ({ error: 'Bind all failed' }),
            });
            vi.useFakeTimers();

            await ctx.executeBindAll(overlay, activeOperators);
            vi.runAllTimers();

            const actionsContainer = overlay.querySelector('.bind-all-actions');
            expect(actionsContainer.innerHTML).toContain('Failed to bind operators');
            vi.useRealTimers();
        });
    });

    describe('closeBindAllOverlay', () => {
        it('removes overlay from DOM', async () => {
            const ctx = createMixinContext();
            ctx.bindAllOverlay = document.createElement('div');
            ctx.bindAllOverlay.className = 'bind-all-confirmation-overlay';
            document.body.appendChild(ctx.bindAllOverlay);
            vi.useFakeTimers();

            ctx.closeBindAllOverlay();
            vi.runAllTimers();

            expect(document.body.contains(ctx.bindAllOverlay)).toBe(false);
            vi.useRealTimers();
        });

        it('does nothing if bindAllOverlay is null', () => {
            const ctx = createMixinContext();
            ctx.bindAllOverlay = null;

            expect(() => ctx.closeBindAllOverlay()).not.toThrow();
        });
    });

    describe('updateBindAllButtonVisibility', () => {
        beforeEach(() => {
            const btn = document.createElement('button');
            btn.id = 'bind-all-btn';
            btn.className = 'initially-hidden';
            const span = document.createElement('span');
            btn.appendChild(span);
            document.body.appendChild(btn);
        });

        it('shows button when unbound active operators exist', () => {
            const ctx = createMixinContext();
            ctx.operators = [
                createMockOperator({ status: OperatorStatus.ACTIVE }),
                createMockOperator({ operator_id: TEST_OPERATOR_ID_2, status: OperatorStatus.ACTIVE }),
            ];
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];

            ctx.updateBindAllButtonVisibility();

            const btn = document.getElementById('bind-all-btn');
            expect(btn.classList.contains('initially-hidden')).toBe(false);
        });

        it('hides button when no unbound active operators', () => {
            const ctx = createMixinContext();
            ctx.operators = [
                createMockOperator({ status: OperatorStatus.ACTIVE }),
            ];
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];

            ctx.updateBindAllButtonVisibility();

            const btn = document.getElementById('bind-all-btn');
            expect(btn.classList.contains('initially-hidden')).toBe(true);
        });

        it('does nothing if button not found in DOM', () => {
            const ctx = createMixinContext();
            document.getElementById('bind-all-btn').remove();
            ctx.operators = [createMockOperator({ status: OperatorStatus.ACTIVE })];
            ctx.boundOperatorIds = [];

            expect(() => ctx.updateBindAllButtonVisibility()).not.toThrow();
        });
    });

    describe('showUnbindAllConfirmationOverlay', () => {
        it('shows alert when no bound operators', () => {
            const ctx = createMixinContext();
            ctx.operators = [createMockOperator({ status: OperatorStatus.ACTIVE })];
            ctx.boundOperatorIds = [];
            global.window = { authState: { getWebSessionId: () => TEST_WEB_SESSION_ID } };
            global.alert = vi.fn();

            ctx.showUnbindAllConfirmationOverlay();

            expect(global.alert).toHaveBeenCalledWith('No operators are currently bound to this session.');
        });

        it('returns early when no bound operators', () => {
            const ctx = createMixinContext();
            ctx.operators = [createMockOperator({ status: OperatorStatus.ACTIVE })];
            ctx.boundOperatorIds = [];
            global.window = { authState: { getWebSessionId: () => TEST_WEB_SESSION_ID } };

            const result = ctx.showUnbindAllConfirmationOverlay();

            expect(result).toBeUndefined();
        });

        it('returns early if template not found', () => {
            const ctx = createMixinContext();
            templateLoader.cache.delete('unbind-all-confirmation-overlay');
            ctx.operators = [createMockOperator({ status: OperatorStatus.BOUND })];
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];
            global.window = { authState: { getWebSessionId: () => TEST_WEB_SESSION_ID } };

            const result = ctx.showUnbindAllConfirmationOverlay();

            expect(result).toBeUndefined();
        });

        it('filters bound operators for current web session', () => {
            const ctx = createMixinContext();
            ctx.operators = [
                createMockOperator({ status: OperatorStatus.BOUND, web_session_id: TEST_WEB_SESSION_ID }),
                createMockOperator({ operator_id: TEST_OPERATOR_ID_2, status: OperatorStatus.BOUND, web_session_id: 'other_session' }),
            ];
            ctx.boundOperatorIds = [TEST_OPERATOR_ID, TEST_OPERATOR_ID_2];
            global.window = { authState: { getWebSessionId: () => TEST_WEB_SESSION_ID } };

            ctx.showUnbindAllConfirmationOverlay();

            expect(ctx.unbindAllOverlay).toBeDefined();
        });
    });

    describe('executeUnbindAll', () => {
        beforeEach(() => {
            const overlay = document.createElement('div');
            overlay.className = 'unbind-all-confirmation-overlay';
            overlay.innerHTML = `
                <button data-action="confirm">Confirm</button>
                <button data-action="cancel">Cancel</button>
                <div class="bind-all-actions"></div>
                <div data-processing-indicator class="initially-hidden"></div>
            `;
            document.body.appendChild(overlay);
        });

        it('calls operatorPanelService.unbindAllOperators with operator IDs', async () => {
            const ctx = createMixinContext();
            const overlay = document.querySelector('.unbind-all-confirmation-overlay');
            const boundOperators = [createMockOperator(), createMockOperator({ operator_id: TEST_OPERATOR_ID_2 })];
            operatorPanelService.unbindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ unbound_operator_ids: [TEST_OPERATOR_ID, TEST_OPERATOR_ID_2] }),
            });
            vi.useFakeTimers();

            await ctx.executeUnbindAll(overlay, boundOperators);
            vi.runAllTimers();

            expect(operatorPanelService.unbindAllOperators).toHaveBeenCalledWith([TEST_OPERATOR_ID, TEST_OPERATOR_ID_2]);
            vi.useRealTimers();
        });

        it('removes unbound operator IDs from boundOperatorIds', async () => {
            const ctx = createMixinContext();
            const overlay = document.querySelector('.unbind-all-confirmation-overlay');
            ctx.boundOperatorIds = [TEST_OPERATOR_ID, TEST_OPERATOR_ID_2];
            const boundOperators = [createMockOperator(), createMockOperator({ operator_id: TEST_OPERATOR_ID_2 })];
            operatorPanelService.unbindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ unbound_operator_ids: [TEST_OPERATOR_ID] }),
            });
            vi.useFakeTimers();

            await ctx.executeUnbindAll(overlay, boundOperators);
            vi.runAllTimers();

            expect(ctx.boundOperatorIds).not.toContain(TEST_OPERATOR_ID);
            vi.useRealTimers();
        });

        it('sets status to OFFLINE and clears metrics when all operators unbound', async () => {
            const ctx = createMixinContext();
            const overlay = document.querySelector('.unbind-all-confirmation-overlay');
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];
            const boundOperators = [createMockOperator()];
            operatorPanelService.unbindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ unbound_operator_ids: [TEST_OPERATOR_ID] }),
            });
            vi.useFakeTimers();

            await ctx.executeUnbindAll(overlay, boundOperators);
            vi.runAllTimers();

            expect(ctx.updateStatus).toHaveBeenCalledWith(OperatorStatus.OFFLINE);
            expect(ctx.isConnected).toBe(false);
            expect(ctx.clearPanelMetrics).toHaveBeenCalled();
            vi.useRealTimers();
        });

        it('calls updateBindAllButtonVisibility after success', async () => {
            const ctx = createMixinContext();
            const overlay = document.querySelector('.unbind-all-confirmation-overlay');
            const boundOperators = [createMockOperator()];
            const updateSpy = vi.spyOn(ctx, 'updateBindAllButtonVisibility');
            operatorPanelService.unbindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ unbound_operator_ids: [TEST_OPERATOR_ID] }),
            });
            vi.useFakeTimers();

            await ctx.executeUnbindAll(overlay, boundOperators);
            vi.runAllTimers();

            expect(updateSpy).toHaveBeenCalled();
            updateSpy.mockRestore();
            vi.useRealTimers();
        });

        it('calls updateUnbindAllButtonVisibility after success', async () => {
            const ctx = createMixinContext();
            const overlay = document.querySelector('.unbind-all-confirmation-overlay');
            const boundOperators = [createMockOperator()];
            const updateSpy = vi.spyOn(ctx, 'updateUnbindAllButtonVisibility');
            operatorPanelService.unbindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ unbound_operator_ids: [TEST_OPERATOR_ID] }),
            });
            vi.useFakeTimers();

            await ctx.executeUnbindAll(overlay, boundOperators);
            vi.runAllTimers();

            expect(updateSpy).toHaveBeenCalled();
            updateSpy.mockRestore();
            vi.useRealTimers();
        });

        it('shows error message on unbind-all failure', async () => {
            const ctx = createMixinContext();
            const overlay = document.querySelector('.unbind-all-confirmation-overlay');
            const boundOperators = [createMockOperator()];
            operatorPanelService.unbindAllOperators.mockResolvedValue({
                ok: false,
                json: async () => ({ error: 'Unbind all failed' }),
            });
            vi.useFakeTimers();

            await ctx.executeUnbindAll(overlay, boundOperators);
            vi.runAllTimers();

            const actionsContainer = overlay.querySelector('.bind-all-actions');
            expect(actionsContainer.innerHTML).toContain('Failed to unbind operators');
            vi.useRealTimers();
        });
    });

    describe('closeUnbindAllOverlay', () => {
        it('removes overlay from DOM', async () => {
            const ctx = createMixinContext();
            ctx.unbindAllOverlay = document.createElement('div');
            ctx.unbindAllOverlay.className = 'unbind-all-confirmation-overlay';
            document.body.appendChild(ctx.unbindAllOverlay);
            vi.useFakeTimers();

            ctx.closeUnbindAllOverlay();
            vi.runAllTimers();

            expect(document.body.contains(ctx.unbindAllOverlay)).toBe(false);
            vi.useRealTimers();
        });

        it('does nothing if unbindAllOverlay is null', () => {
            const ctx = createMixinContext();
            ctx.unbindAllOverlay = null;

            expect(() => ctx.closeUnbindAllOverlay()).not.toThrow();
        });
    });

    describe('updateUnbindAllButtonVisibility', () => {
        beforeEach(() => {
            const btn = document.createElement('button');
            btn.id = 'unbind-all-btn';
            btn.className = 'initially-hidden';
            const span = document.createElement('span');
            btn.appendChild(span);
            document.body.appendChild(btn);
        });

        it('shows button when bound operators exist', () => {
            const ctx = createMixinContext();
            ctx.boundOperatorIds = [TEST_OPERATOR_ID, TEST_OPERATOR_ID_2];

            ctx.updateUnbindAllButtonVisibility();

            const btn = document.getElementById('unbind-all-btn');
            expect(btn.classList.contains('initially-hidden')).toBe(false);
        });

        it('hides button when no bound operators', () => {
            const ctx = createMixinContext();
            ctx.boundOperatorIds = [];

            ctx.updateUnbindAllButtonVisibility();

            const btn = document.getElementById('unbind-all-btn');
            expect(btn.classList.contains('initially-hidden')).toBe(true);
        });

        it('does nothing if button not found in DOM', () => {
            const ctx = createMixinContext();
            document.getElementById('unbind-all-btn').remove();
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];

            expect(() => ctx.updateUnbindAllButtonVisibility()).not.toThrow();
        });
    });

    describe('_createBindAllOperatorItem', () => {
        it('replaces template with operator data', () => {
            const ctx = createMixinContext();
            const operator = createMockOperator();

            const result = ctx._createBindAllOperatorItem(operator);

            expect(result).toContain(operator.operator_id);
            expect(result).toContain(operator.system_info.hostname);
            expect(result).toContain(operator.system_info.os);
            expect(result).toContain(operator.system_info.private_ip);
        });

        it('uses defaults for missing system_info', () => {
            const ctx = createMixinContext();
            const operator = { operator_id: TEST_OPERATOR_ID, system_info: null };

            const result = ctx._createBindAllOperatorItem(operator);

            expect(result).toContain('Unknown');
        });
    });

    describe('_createUnbindAllOperatorItem', () => {
        it('replaces template with operator data', () => {
            const ctx = createMixinContext();
            const operator = createMockOperator();

            const result = ctx._createUnbindAllOperatorItem(operator);

            expect(result).toContain(operator.operator_id);
            expect(result).toContain(operator.system_info.hostname);
            expect(result).toContain(operator.system_info.os);
            expect(result).toContain(operator.system_info.public_ip);
        });

        it('includes stale status class for stale operators', () => {
            const ctx = createMixinContext();
            const operator = createMockOperator({ status: OperatorStatus.STALE });

            const result = ctx._createUnbindAllOperatorItem(operator);

            expect(result).toContain('unbind-all-operator-status-stale');
        });

        it('does not include stale status class for bound operators', () => {
            const ctx = createMixinContext();
            const operator = createMockOperator({ status: OperatorStatus.BOUND });

            const result = ctx._createUnbindAllOperatorItem(operator);

            expect(result).not.toContain('unbind-all-operator-status-stale');
        });
    });

    describe('_renderFeedback', () => {
        it('replaces template with feedback data', () => {
            const ctx = createMixinContext();

            const result = ctx._renderFeedback('success', 'check_circle', 'Operation successful');

            expect(result).toContain('success');
            expect(result).toContain('check_circle');
            expect(result).toContain('Operation successful');
        });
    });

    describe('_escapeHtml', () => {
        it('escapes HTML special characters', () => {
            const ctx = createMixinContext();

            expect(ctx._escapeHtml('<script>alert(xss)</script>')).toBe('&lt;script&gt;alert(xss)&lt;/script&gt;');
        });

        it('escapes ampersands', () => {
            const ctx = createMixinContext();

            expect(ctx._escapeHtml('A & B')).toBe('A &amp; B');
        });

        it('handles null input', () => {
            const ctx = createMixinContext();

            expect(ctx._escapeHtml(null)).toBe('');
        });

        it('handles undefined input', () => {
            const ctx = createMixinContext();

            expect(ctx._escapeHtml(undefined)).toBe('');
        });

        it('passes through safe strings unchanged', () => {
            const ctx = createMixinContext();

            expect(ctx._escapeHtml('safe string 123')).toBe('safe string 123');
        });
    });
});
