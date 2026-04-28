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

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { OperatorStatus } from '@g8ed/public/js/constants/operator-constants.js';
import { EventType } from '@g8ed/public/js/constants/events.js';
import { TEMPLATE_FIXTURES, seedTemplates } from '@test/fixtures/templates.fixture.js';

let BindOperatorsMixin;
let operatorPanelService;
let templateLoader;
let devLogger;
let notificationService;
let showConfirmationModal;

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
        latest_heartbeat_snapshot: {
            system_identity: {
                hostname: 'test-host',
                os: 'Ubuntu 22.04',
            },
            network: {
                internal_ip: '192.168.1.100',
                public_ip: '203.0.113.1',
            },
        },
        bound_web_session_id: TEST_WEB_SESSION_ID,
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
                    result = result.replace(new RegExp(`\\{\\{!${key}\\}\\}`, 'g'), String(value ?? ''));
                    result = result.replace(new RegExp(`\\{\\{${key}\\}\\}`, 'g'), String(value ?? ''));
                }
                return result;
            }),
            renderTo: vi.fn(async () => {}),
        },
    }));

    vi.doMock('@g8ed/public/js/utils/ui-utils.js', () => ({
        showConfirmationModal: vi.fn(),
    }));

    vi.doMock('@g8ed/public/js/utils/notification-service.js', () => ({
        notificationService: {
            error: vi.fn(),
            info: vi.fn(),
            success: vi.fn(),
            warning: vi.fn(),
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

    const nsMod = await import('@g8ed/public/js/utils/notification-service.js');
    notificationService = nsMod.notificationService;

    const uiUtilsMod = await import('@g8ed/public/js/utils/ui-utils.js');
    showConfirmationModal = uiUtilsMod.showConfirmationModal;

    seedTemplates(templateLoader, [
        'operator-bind-single-overlay',
        'bind-all-confirmation-overlay',
        'unbind-all-confirmation-overlay',
        'bind-all-operator-item',
        'bind-result-feedback',
        'operator-bind-all-overlay',
        'operator-unbind-all-overlay',
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

        it('shows notificationService error when bind fails', async () => {
            const ctx = createMixinContext();
            operatorPanelService.bindOperator.mockResolvedValue({
                ok: false,
                json: async () => ({ error: 'Failed to bind' }),
            });

            await ctx.bindOperator(TEST_OPERATOR_ID);

            expect(notificationService.error).toHaveBeenCalledWith('Failed to bind operator: Failed to bind');
        });

        it('handles network errors with notificationService', async () => {
            const ctx = createMixinContext();
            operatorPanelService.bindOperator.mockRejectedValue(new Error('Network failure'));

            await ctx.bindOperator(TEST_OPERATOR_ID);

            expect(notificationService.error).toHaveBeenCalledWith('Failed to bind operator: Network failure');
        });
    });

    describe('unbindOperator', () => {
        it('calls operatorPanelService.unbindOperator with operator_id in body', async () => {
            const ctx = createMixinContext();
            const updateBindAllSpy = vi.spyOn(BindOperatorsMixin, 'updateBindAllButtonVisibility');
            const updateUnbindAllSpy = vi.spyOn(BindOperatorsMixin, 'updateUnbindAllButtonVisibility');
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];
            operatorPanelService.unbindOperator.mockResolvedValue({
                ok: true,
                json: async () => ({}),
            });

            await ctx.unbindOperator(TEST_OPERATOR_ID);

            expect(operatorPanelService.unbindOperator).toHaveBeenCalledWith({ operator_id: TEST_OPERATOR_ID });
            updateBindAllSpy.mockRestore();
            updateUnbindAllSpy.mockRestore();
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

        it('shows notificationService error when unbind fails', async () => {
            const ctx = createMixinContext();
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];
            operatorPanelService.unbindOperator.mockResolvedValue({
                ok: false,
                json: async () => ({ error: 'Failed to unbind' }),
            });

            await ctx.unbindOperator(TEST_OPERATOR_ID, false);

            expect(notificationService.error).toHaveBeenCalledWith('Failed to unbind operator: Failed to unbind');
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
        it('renders inline overlay with bind configuration', async () => {
            const ctx = createMixinContext();
            ctx.downloadCollapsibleContent = document.createElement('div');
            ctx.expandDownloadSection = vi.fn();

            await ctx._showBindSingleModal({
                operatorId: TEST_OPERATOR_ID,
                operator: createMockOperator(),
                mode: 'bind',
            });

            expect(ctx.expandDownloadSection).toHaveBeenCalled();
            expect(ctx.downloadCollapsibleContent.querySelector('.operator-bind-single-overlay')).toBeTruthy();
            const overlay = ctx.downloadCollapsibleContent.querySelector('.operator-bind-single-overlay');
            const titleEl = overlay.querySelector('[data-modal-title]');
            expect(titleEl.textContent).toBe('Bind Selected Operators');
            const confirmLabelEl = overlay.querySelector('[data-confirm-label]');
            expect(confirmLabelEl.textContent).toBe('Bind Operator');
        });

        it('renders inline overlay with unbind configuration', async () => {
            const ctx = createMixinContext();
            ctx.downloadCollapsibleContent = document.createElement('div');
            ctx.expandDownloadSection = vi.fn();

            await ctx._showBindSingleModal({
                operatorId: TEST_OPERATOR_ID,
                operator: createMockOperator(),
                mode: 'unbind',
            });

            const overlay = ctx.downloadCollapsibleContent.querySelector('.operator-bind-single-overlay');
            const titleEl = overlay.querySelector('[data-modal-title]');
            expect(titleEl.textContent).toBe('Unbind Operator from WebSession');
            const confirmLabelEl = overlay.querySelector('[data-confirm-label]');
            expect(confirmLabelEl.textContent).toBe('Unbind Operator');
        });

        it('renders inline overlay with unbind-stale configuration', async () => {
            const ctx = createMixinContext();
            ctx.downloadCollapsibleContent = document.createElement('div');
            ctx.expandDownloadSection = vi.fn();

            await ctx._showBindSingleModal({
                operatorId: TEST_OPERATOR_ID,
                operator: createMockOperator(),
                mode: 'unbind-stale',
            });

            const overlay = ctx.downloadCollapsibleContent.querySelector('.operator-bind-single-overlay');
            const titleEl = overlay.querySelector('[data-modal-title]');
            expect(titleEl.textContent).toBe('Unbind Stale Operator');
        });

        it('shows notificationService error when downloadCollapsibleContent not found', async () => {
            const ctx = createMixinContext();
            ctx.downloadCollapsibleContent = null;

            await ctx._showBindSingleModal({
                operatorId: TEST_OPERATOR_ID,
                operator: createMockOperator(),
                mode: 'bind',
            });

            expect(notificationService.error).toHaveBeenCalledWith('Unable to show bind overlay. Please try again.');
        });
    });

    describe('showBindAllConfirmationOverlay', () => {
        it('shows notificationService info when no active operators available to bind', () => {
            const ctx = createMixinContext();
            ctx.operators = [
                createMockOperator({ status: OperatorStatus.BOUND }),
            ];
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];

            ctx.showBindAllConfirmationOverlay();

            expect(notificationService.info).toHaveBeenCalledWith('No active operators available to bind. All active operators are already bound to this session.');
        });

        it('returns early when no active operators available', () => {
            const ctx = createMixinContext();
            ctx.operators = [createMockOperator({ status: OperatorStatus.BOUND })];
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];

            ctx.showBindAllConfirmationOverlay();

            expect(showConfirmationModal).not.toHaveBeenCalled();
        });

        it('renders inline overlay with active operators', async () => {
            const ctx = createMixinContext();
            ctx.operators = [
                createMockOperator({ operator_id: TEST_OPERATOR_ID, status: OperatorStatus.ACTIVE }),
                createMockOperator({ operator_id: TEST_OPERATOR_ID_2, status: OperatorStatus.BOUND }),
            ];
            ctx.boundOperatorIds = [TEST_OPERATOR_ID_2];
            ctx.operatorPanelWrapper = document.createElement('div');

            await ctx.showBindAllConfirmationOverlay();

            expect(ctx.operatorPanelWrapper.querySelector('.operator-bind-all-overlay')).toBeTruthy();
        });

        it('includes operator list in overlay', async () => {
            const ctx = createMixinContext();
            ctx.operators = [createMockOperator({ status: OperatorStatus.ACTIVE })];
            ctx.boundOperatorIds = [];
            ctx.operatorPanelWrapper = document.createElement('div');

            await ctx.showBindAllConfirmationOverlay();

            const overlay = ctx.operatorPanelWrapper.querySelector('.operator-bind-all-overlay');
            expect(overlay).toBeTruthy();
            expect(overlay.querySelector('#operator-bind-all-operators-list')).toBeTruthy();
        });
    });

    describe('executeBindAll', () => {
        beforeEach(() => {
            const overlay = document.createElement('div');
            overlay.className = 'operator-bind-all-overlay';
            overlay.id = 'operator-bind-all-overlay';
            overlay.innerHTML = `
                <button id="operator-bind-all-confirm-btn">Confirm</button>
                <button id="operator-bind-all-cancel-btn">Cancel</button>
                <div class="operator-bind-all-actions"></div>
                <div id="operator-bind-all-processing" class="initially-hidden"></div>
                <div id="operator-bind-all-feedback"></div>
            `;
            document.body.appendChild(overlay);
        });

        function addCheckboxesToOverlay(overlay, operatorIds) {
            operatorIds.forEach(id => {
                const checkbox = document.createElement('input');
                checkbox.type = 'checkbox';
                checkbox.className = 'operator-select-checkbox';
                checkbox.setAttribute('data-operator-id', id);
                checkbox.checked = true;
                overlay.appendChild(checkbox);
            });
        }

        it('calls operatorPanelService.bindAllOperators with operator IDs', async () => {
            vi.useFakeTimers();
            const ctx = createMixinContext();
            const overlay = document.querySelector('.operator-bind-all-overlay');
            const activeOperators = [createMockOperator(), createMockOperator({ operator_id: TEST_OPERATOR_ID_2 })];
            addCheckboxesToOverlay(overlay, [TEST_OPERATOR_ID, TEST_OPERATOR_ID_2]);
            operatorPanelService.bindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ bound_operator_ids: [TEST_OPERATOR_ID, TEST_OPERATOR_ID_2] }),
            });

            const p = ctx.executeBindAll(overlay, activeOperators);
            await vi.runAllTimersAsync();
            await p;

            expect(operatorPanelService.bindAllOperators).toHaveBeenCalledWith([TEST_OPERATOR_ID, TEST_OPERATOR_ID_2]);
            vi.useRealTimers();
        });

        it('adds bound operator IDs to boundOperatorIds', async () => {
            vi.useFakeTimers();
            const ctx = createMixinContext();
            const overlay = document.querySelector('.operator-bind-all-overlay');
            const activeOperators = [createMockOperator()];
            addCheckboxesToOverlay(overlay, [TEST_OPERATOR_ID]);
            operatorPanelService.bindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ bound_operator_ids: [TEST_OPERATOR_ID] }),
            });

            const p = ctx.executeBindAll(overlay, activeOperators);
            await vi.runAllTimersAsync();
            await p;

            expect(ctx.boundOperatorIds).toContain(TEST_OPERATOR_ID);
            vi.useRealTimers();
        });

        it('falls back to input operator IDs if response missing bound_operator_ids', async () => {
            vi.useFakeTimers();
            const ctx = createMixinContext();
            const overlay = document.querySelector('.operator-bind-all-overlay');
            const activeOperators = [createMockOperator()];
            addCheckboxesToOverlay(overlay, [TEST_OPERATOR_ID]);
            operatorPanelService.bindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({}),
            });

            const p = ctx.executeBindAll(overlay, activeOperators);
            await vi.runAllTimersAsync();
            await p;

            expect(ctx.boundOperatorIds).toContain(TEST_OPERATOR_ID);
            vi.useRealTimers();
        });

        it('shows error via templateLoader.renderTo on bind-all failure', async () => {
            vi.useFakeTimers();
            const ctx = createMixinContext();
            const overlay = document.querySelector('.operator-bind-all-overlay');
            const activeOperators = [createMockOperator()];
            addCheckboxesToOverlay(overlay, [TEST_OPERATOR_ID]);
            operatorPanelService.bindAllOperators.mockResolvedValue({
                ok: false,
                json: async () => ({ error: 'Bind all failed' }),
            });

            const p = ctx.executeBindAll(overlay, activeOperators).catch(() => {});
            await vi.runAllTimersAsync();
            await p;

            expect(templateLoader.renderTo).toHaveBeenCalledWith(
                expect.any(Object),
                'bind-result-feedback',
                expect.objectContaining({ resultClass: 'error' })
            );
            vi.useRealTimers();
        });

        it('binds only selected operators when some checkboxes are unchecked', async () => {
            vi.useFakeTimers();
            const ctx = createMixinContext();
            const overlay = document.querySelector('.operator-bind-all-overlay');
            const activeOperators = [createMockOperator(), createMockOperator({ operator_id: TEST_OPERATOR_ID_2 })];
            addCheckboxesToOverlay(overlay, [TEST_OPERATOR_ID, TEST_OPERATOR_ID_2]);
            const checkboxes = overlay.querySelectorAll('.operator-select-checkbox');
            checkboxes[1].checked = false;
            operatorPanelService.bindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ bound_operator_ids: [TEST_OPERATOR_ID] }),
            });

            const p = ctx.executeBindAll(overlay, activeOperators);
            await vi.runAllTimersAsync();
            await p;

            expect(operatorPanelService.bindAllOperators).toHaveBeenCalledWith([TEST_OPERATOR_ID]);
            vi.useRealTimers();
        });

        it('shows error when no operators are selected', async () => {
            vi.useFakeTimers();
            const ctx = createMixinContext();
            const overlay = document.querySelector('.operator-bind-all-overlay');
            const activeOperators = [createMockOperator()];
            addCheckboxesToOverlay(overlay, [TEST_OPERATOR_ID]);
            const checkboxes = overlay.querySelectorAll('.operator-select-checkbox');
            checkboxes[0].checked = false;

            const p = ctx.executeBindAll(overlay, activeOperators).catch(() => {});
            await vi.runAllTimersAsync();
            await p;

            expect(templateLoader.renderTo).toHaveBeenCalledWith(
                expect.any(Object),
                'bind-result-feedback',
                expect.objectContaining({ resultClass: 'error', message: 'No operators selected' })
            );
            vi.useRealTimers();
        });
    });

    describe('overlay lifecycle', () => {
        it('renders inline overlay in download section', async () => {
            const ctx = createMixinContext();
            ctx.operators = [createMockOperator({ status: OperatorStatus.ACTIVE })];
            ctx.boundOperatorIds = [];
            ctx.operatorPanelWrapper = document.createElement('div');

            await ctx.showBindAllConfirmationOverlay();

            expect(ctx.operatorPanelWrapper.querySelector('.operator-bind-all-overlay')).toBeTruthy();
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
        it('shows notificationService info when no bound operators', () => {
            const ctx = createMixinContext();
            ctx.operators = [createMockOperator({ status: OperatorStatus.ACTIVE })];
            ctx.boundOperatorIds = [];
            global.window = { authState: { getWebSessionId: () => TEST_WEB_SESSION_ID } };

            ctx.showUnbindAllConfirmationOverlay();

            expect(notificationService.info).toHaveBeenCalledWith('No operators are currently bound to this session.');
        });

        it('returns early when no bound operators', () => {
            const ctx = createMixinContext();
            ctx.operators = [createMockOperator({ status: OperatorStatus.ACTIVE })];
            ctx.boundOperatorIds = [];
            global.window = { authState: { getWebSessionId: () => TEST_WEB_SESSION_ID } };

            ctx.showUnbindAllConfirmationOverlay();

            expect(showConfirmationModal).not.toHaveBeenCalled();
        });

        it('renders inline overlay with bound operators', async () => {
            const ctx = createMixinContext();
            ctx.operators = [
                createMockOperator({ status: OperatorStatus.BOUND, bound_web_session_id: TEST_WEB_SESSION_ID }),
                createMockOperator({ operator_id: TEST_OPERATOR_ID_2, status: OperatorStatus.BOUND, bound_web_session_id: 'other_session' }),
            ];
            ctx.boundOperatorIds = [TEST_OPERATOR_ID, TEST_OPERATOR_ID_2];
            global.window = { authState: { getWebSessionId: () => TEST_WEB_SESSION_ID } };
            ctx.operatorPanelWrapper = document.createElement('div');

            await ctx.showUnbindAllConfirmationOverlay();

            expect(ctx.operatorPanelWrapper.querySelector('.operator-unbind-all-overlay')).toBeTruthy();
        });
    });

    describe('executeUnbindAll', () => {
        beforeEach(() => {
            const overlay = document.createElement('div');
            overlay.className = 'operator-unbind-all-overlay';
            overlay.id = 'operator-unbind-all-overlay';
            overlay.innerHTML = `
                <button id="operator-unbind-all-confirm-btn">Confirm</button>
                <button id="operator-unbind-all-cancel-btn">Cancel</button>
                <div class="operator-unbind-all-actions"></div>
                <div id="operator-unbind-all-processing" class="initially-hidden"></div>
                <div id="operator-unbind-all-feedback"></div>
            `;
            document.body.appendChild(overlay);
        });

        it('calls operatorPanelService.unbindAllOperators with operator IDs', async () => {
            vi.useFakeTimers();
            const ctx = createMixinContext();
            const overlay = document.querySelector('.operator-unbind-all-overlay');
            const boundOperators = [createMockOperator(), createMockOperator({ operator_id: TEST_OPERATOR_ID_2 })];
            operatorPanelService.unbindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ unbound_operator_ids: [TEST_OPERATOR_ID, TEST_OPERATOR_ID_2] }),
            });

            const p = ctx.executeUnbindAll(overlay, boundOperators);
            await vi.runAllTimersAsync();
            await p;

            expect(operatorPanelService.unbindAllOperators).toHaveBeenCalledWith([TEST_OPERATOR_ID, TEST_OPERATOR_ID_2]);
            vi.useRealTimers();
        });

        it('removes unbound operator IDs from boundOperatorIds', async () => {
            vi.useFakeTimers();
            const ctx = createMixinContext();
            const overlay = document.querySelector('.operator-unbind-all-overlay');
            ctx.boundOperatorIds = [TEST_OPERATOR_ID, TEST_OPERATOR_ID_2];
            const boundOperators = [createMockOperator(), createMockOperator({ operator_id: TEST_OPERATOR_ID_2 })];
            operatorPanelService.unbindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ unbound_operator_ids: [TEST_OPERATOR_ID] }),
            });

            const p = ctx.executeUnbindAll(overlay, boundOperators);
            await vi.runAllTimersAsync();
            await p;

            expect(ctx.boundOperatorIds).not.toContain(TEST_OPERATOR_ID);
            vi.useRealTimers();
        });

        it('sets status to OFFLINE and clears metrics when all operators unbound', async () => {
            vi.useFakeTimers();
            const ctx = createMixinContext();
            const overlay = document.querySelector('.operator-unbind-all-overlay');
            ctx.boundOperatorIds = [TEST_OPERATOR_ID];
            const boundOperators = [createMockOperator()];
            operatorPanelService.unbindAllOperators.mockResolvedValue({
                ok: true,
                json: async () => ({ unbound_operator_ids: [TEST_OPERATOR_ID] }),
            });

            const p = ctx.executeUnbindAll(overlay, boundOperators);
            await vi.runAllTimersAsync();
            await p;

            expect(ctx.updateStatus).toHaveBeenCalledWith(OperatorStatus.OFFLINE);
            expect(ctx.isConnected).toBe(false);
            expect(ctx.clearPanelMetrics).toHaveBeenCalled();
            vi.useRealTimers();
        });

        it('shows error via templateLoader.renderTo on unbind-all failure', async () => {
            vi.useFakeTimers();
            const ctx = createMixinContext();
            const overlay = document.querySelector('.operator-unbind-all-overlay');
            const boundOperators = [createMockOperator()];
            operatorPanelService.unbindAllOperators.mockResolvedValue({
                ok: false,
                json: async () => ({ error: 'Unbind all failed' }),
            });

            const p = ctx.executeUnbindAll(overlay, boundOperators).catch(() => {});
            await vi.runAllTimersAsync();
            await p;

            expect(templateLoader.renderTo).toHaveBeenCalledWith(
                expect.any(Object),
                'bind-result-feedback',
                expect.objectContaining({ resultClass: 'error' })
            );
            vi.useRealTimers();
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
            ctx.operators = [
                { ...createMockOperator(), operator_id: TEST_OPERATOR_ID, status: OperatorStatus.BOUND, bound_web_session_id: TEST_WEB_SESSION_ID },
                { ...createMockOperator(), operator_id: TEST_OPERATOR_ID_2, status: OperatorStatus.BOUND, bound_web_session_id: TEST_WEB_SESSION_ID },
            ];
            window.authState = { getWebSessionId: () => TEST_WEB_SESSION_ID };

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
            expect(result).toContain(operator.latest_heartbeat_snapshot.system_identity.hostname);
            expect(result).toContain(operator.latest_heartbeat_snapshot.system_identity.os);
            expect(result).toContain(operator.latest_heartbeat_snapshot.network.internal_ip);
        });

        it('uses defaults for missing latest_heartbeat_snapshot', () => {
            const ctx = createMixinContext();
            const operator = { operator_id: TEST_OPERATOR_ID, latest_heartbeat_snapshot: null };

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
            expect(result).toContain(operator.latest_heartbeat_snapshot.system_identity.hostname);
            expect(result).toContain(operator.latest_heartbeat_snapshot.system_identity.os);
            expect(result).toContain(operator.latest_heartbeat_snapshot.network.public_ip);
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
