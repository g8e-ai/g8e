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
import { MockEventBus, MockElement } from '@test/mocks/mock-browser-env.js';
import { EventType } from '@g8ed/public/js/constants/events.js';
import { OperatorStatus } from '@g8ed/public/js/constants/operator-constants.js';

let TerminalOperatorMixin;

function createMixinContext(overrides = {}) {
    const ctx = Object.create(null);
    
    Object.getOwnPropertyNames(TerminalOperatorMixin.prototype).forEach(name => {
        ctx[name] = TerminalOperatorMixin.prototype[name].bind(ctx);
    });
    
    ctx.eventBus = new MockEventBus();
    ctx.hostnameElement = new MockElement('span', 'terminal-hostname');
    ctx.promptElement = new MockElement('span', 'terminal-prompt');
    ctx.updateInputState = vi.fn();
    ctx.appendSystemMessage = vi.fn();
    ctx.setUser = vi.fn();
    ctx.enable = vi.fn();
    ctx.disable = vi.fn();
    ctx.focus = vi.fn();
    ctx.denyAllPendingApprovals = vi.fn();
    ctx.handleApprovalRequest = vi.fn();
    ctx.handleCommandExecutionEvent = vi.fn();
    ctx.handleIntentResult = vi.fn();
    
    Object.assign(ctx, overrides);
    return ctx;
}

beforeEach(async () => {
    vi.resetModules();
    const mod = await import('@g8ed/public/js/components/anchored-terminal-operator.js');
    TerminalOperatorMixin = mod.TerminalOperatorMixin;
});

afterEach(() => {
    vi.restoreAllMocks();
});

describe('TerminalOperatorMixin [UNIT - jsdom]', () => {
    
    describe('initOperatorState', () => {
        it('initializes operator state to unbound', () => {
            const ctx = createMixinContext();
            ctx.initOperatorState();
            
            expect(ctx.isOperatorBound).toBe(false);
            expect(ctx.boundOperator).toBe(null);
        });

        it('resets state when called on already bound operator', () => {
            const ctx = createMixinContext();
            ctx.isOperatorBound = true;
            ctx.boundOperator = { operator_id: 'op_123' };
            
            ctx.initOperatorState();
            
            expect(ctx.isOperatorBound).toBe(false);
            expect(ctx.boundOperator).toBe(null);
        });
    });

    describe('bindEventBusListeners', () => {
        it('binds OPERATOR_STATUS_UPDATED_BOUND listener', () => {
            const ctx = createMixinContext({ setOperatorBound: vi.fn() });
            
            ctx.bindEventBusListeners();
            
            const testOperator = { operator_id: 'op_123', name: 'test-op' };
            ctx.eventBus.emit(EventType.OPERATOR_STATUS_UPDATED_BOUND, { operator: testOperator });
            
            expect(ctx.setOperatorBound).toHaveBeenCalledWith(testOperator);
        });

        it('does not call setOperatorBound when operator data is missing', () => {
            const ctx = createMixinContext({ setOperatorBound: vi.fn() });
            
            ctx.bindEventBusListeners();
            
            ctx.eventBus.emit(EventType.OPERATOR_STATUS_UPDATED_BOUND, {});
            
            expect(ctx.setOperatorBound).not.toHaveBeenCalled();
        });

        it('binds all unbound status listeners to setOperatorUnbound', () => {
            const ctx = createMixinContext({ setOperatorUnbound: vi.fn() });
            
            ctx.bindEventBusListeners();
            
            const unboundStatuses = [
                EventType.OPERATOR_STATUS_UPDATED_ACTIVE,
                EventType.OPERATOR_STATUS_UPDATED_AVAILABLE,
                EventType.OPERATOR_STATUS_UPDATED_UNAVAILABLE,
                EventType.OPERATOR_STATUS_UPDATED_OFFLINE,
                EventType.OPERATOR_STATUS_UPDATED_STALE,
                EventType.OPERATOR_STATUS_UPDATED_STOPPED,
                EventType.OPERATOR_STATUS_UPDATED_TERMINATED,
            ];
            
            for (const statusEvent of unboundStatuses) {
                ctx.eventBus.emit(statusEvent);
            }
            
            expect(ctx.setOperatorUnbound).toHaveBeenCalledTimes(unboundStatuses.length);
        });

        it('binds OPERATOR_PANEL_LIST_UPDATED listener', () => {
            const ctx = createMixinContext();
            ctx.handleOperatorListUpdate = vi.fn();
            
            ctx.bindEventBusListeners();
            
            const testData = { operators: [] };
            ctx.eventBus.emit(EventType.OPERATOR_PANEL_LIST_UPDATED, testData);
            
            expect(ctx.handleOperatorListUpdate).toHaveBeenCalledWith(testData);
        });

        it('binds OPERATOR_BOUND listener', () => {
            const ctx = createMixinContext();
            ctx.handleOperatorBound = vi.fn();
            
            ctx.bindEventBusListeners();
            
            const testData = { operator: { operator_id: 'op_123' } };
            ctx.eventBus.emit(EventType.OPERATOR_BOUND, testData);
            
            expect(ctx.handleOperatorBound).toHaveBeenCalledWith(testData);
        });

        it('binds OPERATOR_UNBOUND listener', () => {
            const ctx = createMixinContext();
            ctx.handleOperatorUnbound = vi.fn();
            
            ctx.bindEventBusListeners();
            
            ctx.eventBus.emit(EventType.OPERATOR_UNBOUND, {});
            
            expect(ctx.handleOperatorUnbound).toHaveBeenCalled();
        });

        it('binds all approval request listeners to handleApprovalRequest', () => {
            const ctx = createMixinContext();
            ctx.handleApprovalRequest = vi.fn();
            
            ctx.bindEventBusListeners();
            
            const approvalEvents = [
                EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED,
                EventType.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED,
                EventType.OPERATOR_INTENT_APPROVAL_REQUESTED,
            ];
            
            for (const approvalEvent of approvalEvents) {
                const testData = { approval_id: 'apr_123' };
                ctx.eventBus.emit(approvalEvent, testData);
            }
            
            expect(ctx.handleApprovalRequest).toHaveBeenCalledTimes(approvalEvents.length);
        });

        it('binds all command execution event listeners', () => {
            const ctx = createMixinContext();
            ctx.handleCommandExecutionEvent = vi.fn();
            
            ctx.bindEventBusListeners();
            
            const commandEvents = [
                EventType.OPERATOR_COMMAND_APPROVAL_PREPARING,
                EventType.OPERATOR_COMMAND_STARTED,
                EventType.OPERATOR_COMMAND_OUTPUT_RECEIVED,
                EventType.OPERATOR_COMMAND_COMPLETED,
                EventType.OPERATOR_COMMAND_FAILED,
                EventType.OPERATOR_COMMAND_APPROVAL_GRANTED,
                EventType.OPERATOR_COMMAND_APPROVAL_REJECTED,
                EventType.OPERATOR_FILE_EDIT_STARTED,
                EventType.OPERATOR_FILE_EDIT_COMPLETED,
                EventType.OPERATOR_FILE_EDIT_FAILED,
                EventType.OPERATOR_FILE_EDIT_APPROVAL_GRANTED,
                EventType.OPERATOR_FILE_EDIT_APPROVAL_REJECTED,
            ];
            
            for (const cmdEvent of commandEvents) {
                const testData = { execution_id: 'exe_123' };
                ctx.eventBus.emit(cmdEvent, testData);
            }
            
            expect(ctx.handleCommandExecutionEvent).toHaveBeenCalledTimes(commandEvents.length);
        });

        it('passes eventType to handleCommandExecutionEvent', () => {
            const ctx = createMixinContext();
            ctx.handleCommandExecutionEvent = vi.fn();
            
            ctx.bindEventBusListeners();
            
            const testData = { execution_id: 'exe_123' };
            ctx.eventBus.emit(EventType.OPERATOR_COMMAND_STARTED, testData);
            
            expect(ctx.handleCommandExecutionEvent).toHaveBeenCalledWith({
                ...testData,
                eventType: EventType.OPERATOR_COMMAND_STARTED,
            });
        });

        it('binds all intent result listeners', () => {
            const ctx = createMixinContext();
            ctx.handleIntentResult = vi.fn();
            
            ctx.bindEventBusListeners();
            
            const intentEvents = [
                EventType.OPERATOR_INTENT_GRANTED,
                EventType.OPERATOR_INTENT_DENIED,
                EventType.OPERATOR_INTENT_REVOKED,
                EventType.OPERATOR_INTENT_APPROVAL_GRANTED,
                EventType.OPERATOR_INTENT_APPROVAL_REJECTED,
            ];
            
            for (const intentEvent of intentEvents) {
                const testData = { intent_id: 'int_123' };
                ctx.eventBus.emit(intentEvent, testData);
            }
            
            expect(ctx.handleIntentResult).toHaveBeenCalledTimes(intentEvents.length);
        });

        it('passes eventType to handleIntentResult', () => {
            const ctx = createMixinContext();
            ctx.handleIntentResult = vi.fn();
            
            ctx.bindEventBusListeners();
            
            const testData = { intent_id: 'int_123' };
            ctx.eventBus.emit(EventType.OPERATOR_INTENT_GRANTED, testData);
            
            expect(ctx.handleIntentResult).toHaveBeenCalledWith({
                ...testData,
                eventType: EventType.OPERATOR_INTENT_GRANTED,
            });
        });

        it('binds OPERATOR_TERMINAL_APPROVAL_DENIED listener', () => {
            const ctx = createMixinContext();
            ctx.denyAllPendingApprovals = vi.fn();
            
            ctx.bindEventBusListeners();
            
            const testData = { reason: 'Session expired', statusMessage: 'Unauthorized' };
            ctx.eventBus.emit(EventType.OPERATOR_TERMINAL_APPROVAL_DENIED, testData);
            
            expect(ctx.denyAllPendingApprovals).toHaveBeenCalledWith('Session expired', 'Unauthorized');
        });

        it('binds OPERATOR_TERMINAL_AUTH_STATE_CHANGED listener for authenticated state', () => {
            const ctx = createMixinContext();
            ctx.bindEventBusListeners();
            
            const testUser = { user_id: 'user_123', email: 'test@example.com' };
            ctx.eventBus.emit(EventType.OPERATOR_TERMINAL_AUTH_STATE_CHANGED, {
                isAuthenticated: true,
                user: testUser,
            });
            
            expect(ctx.setUser).toHaveBeenCalledWith(testUser);
            expect(ctx.enable).toHaveBeenCalled();
            expect(ctx.focus).toHaveBeenCalled();
        });

        it('binds OPERATOR_TERMINAL_AUTH_STATE_CHANGED listener for unauthenticated state', () => {
            const ctx = createMixinContext();
            ctx.bindEventBusListeners();
            
            ctx.eventBus.emit(EventType.OPERATOR_TERMINAL_AUTH_STATE_CHANGED, {
                isAuthenticated: false,
                user: null,
            });
            
            expect(ctx.setUser).toHaveBeenCalledWith(null);
            expect(ctx.disable).toHaveBeenCalled();
        });

        it('sets _eventsBound flag to prevent duplicate binding', () => {
            const ctx = createMixinContext();
            
            ctx.bindEventBusListeners();
            const initialListenerCount = ctx.eventBus.getListenerCount(EventType.OPERATOR_STATUS_UPDATED_BOUND);
            
            ctx.bindEventBusListeners();
            const finalListenerCount = ctx.eventBus.getListenerCount(EventType.OPERATOR_STATUS_UPDATED_BOUND);
            
            expect(finalListenerCount).toBe(initialListenerCount);
        });

        it('returns early if eventBus is not set', () => {
            const ctx = createMixinContext();
            ctx.eventBus = null;
            ctx._eventsBound = false;
            
            expect(() => ctx.bindEventBusListeners()).not.toThrow();
            expect(ctx._eventsBound).toBe(false);
        });
    });

    describe('handleOperatorListUpdate', () => {
        it('sets operator bound when operator has BOUND status', () => {
            const ctx = createMixinContext({ setOperatorBound: vi.fn() });
            
            const testData = {
                operators: [
                    { operator_id: 'op_1', status: OperatorStatus.BOUND, name: 'bound-op' },
                    { operator_id: 'op_2', status: OperatorStatus.AVAILABLE, name: 'available-op' },
                ],
            };
            
            ctx.handleOperatorListUpdate(testData);
            
            expect(ctx.setOperatorBound).toHaveBeenCalledWith(
                expect.objectContaining({ operator_id: 'op_1', status: OperatorStatus.BOUND })
            );
        });

        it('sets operator bound when operator has is_bound flag', () => {
            const ctx = createMixinContext({ setOperatorBound: vi.fn() });
            
            const testData = {
                operators: [
                    { operator_id: 'op_1', is_bound: true, name: 'bound-op' },
                ],
            };
            
            ctx.handleOperatorListUpdate(testData);
            
            expect(ctx.setOperatorBound).toHaveBeenCalledWith(
                expect.objectContaining({ operator_id: 'op_1', is_bound: true })
            );
        });

        it('sets operator unbound when no bound operator found', () => {
            const ctx = createMixinContext({ setOperatorUnbound: vi.fn() });
            
            const testData = {
                operators: [
                    { operator_id: 'op_1', status: OperatorStatus.AVAILABLE },
                    { operator_id: 'op_2', status: OperatorStatus.ACTIVE },
                ],
            };
            
            ctx.handleOperatorListUpdate(testData);
            
            expect(ctx.setOperatorUnbound).toHaveBeenCalled();
        });

        it('sets operator unbound when operators array is empty', () => {
            const ctx = createMixinContext({ setOperatorUnbound: vi.fn() });
            
            ctx.handleOperatorListUpdate({ operators: [] });
            
            expect(ctx.setOperatorUnbound).toHaveBeenCalled();
        });

        it('does nothing when data is null', () => {
            const ctx = createMixinContext({ setOperatorBound: vi.fn(), setOperatorUnbound: vi.fn() });
            
            ctx.handleOperatorListUpdate(null);
            
            expect(ctx.setOperatorBound).not.toHaveBeenCalled();
            expect(ctx.setOperatorUnbound).not.toHaveBeenCalled();
        });

        it('does nothing when operators array is missing', () => {
            const ctx = createMixinContext({ setOperatorBound: vi.fn(), setOperatorUnbound: vi.fn() });
            
            ctx.handleOperatorListUpdate({});
            
            expect(ctx.setOperatorBound).not.toHaveBeenCalled();
            expect(ctx.setOperatorUnbound).not.toHaveBeenCalled();
        });

        it('does nothing when operators is not an array', () => {
            const ctx = createMixinContext({ setOperatorBound: vi.fn(), setOperatorUnbound: vi.fn() });
            
            ctx.handleOperatorListUpdate({ operators: 'not-an-array' });
            
            expect(ctx.setOperatorBound).not.toHaveBeenCalled();
            expect(ctx.setOperatorUnbound).not.toHaveBeenCalled();
        });
    });

    describe('handleOperatorBound', () => {
        it('calls setOperatorBound with operator from data', () => {
            const ctx = createMixinContext({ setOperatorBound: vi.fn() });
            
            const testOperator = { operator_id: 'op_123', name: 'test-op' };
            ctx.handleOperatorBound({ operator: testOperator });
            
            expect(ctx.setOperatorBound).toHaveBeenCalledWith(testOperator);
        });

        it('does nothing when data is null', () => {
            const ctx = createMixinContext({ setOperatorBound: vi.fn() });
            
            ctx.handleOperatorBound(null);
            
            expect(ctx.setOperatorBound).not.toHaveBeenCalled();
        });

        it('does nothing when operator is missing from data', () => {
            const ctx = createMixinContext({ setOperatorBound: vi.fn() });
            
            ctx.handleOperatorBound({});
            
            expect(ctx.setOperatorBound).not.toHaveBeenCalled();
        });
    });

    describe('handleOperatorUnbound', () => {
        it('calls setOperatorUnbound', () => {
            const ctx = createMixinContext({ setOperatorUnbound: vi.fn() });
            
            ctx.handleOperatorUnbound();
            
            expect(ctx.setOperatorUnbound).toHaveBeenCalled();
        });
    });

    describe('setOperatorBound', () => {
        it('sets isOperatorBound to true', () => {
            const ctx = createMixinContext();
            const testOperator = { operator_id: 'op_123', name: 'test-op' };
            
            ctx.setOperatorBound(testOperator);
            
            expect(ctx.isOperatorBound).toBe(true);
        });

        it('stores bound operator', () => {
            const ctx = createMixinContext();
            const testOperator = { operator_id: 'op_123', name: 'test-op' };
            
            ctx.setOperatorBound(testOperator);
            
            expect(ctx.boundOperator).toEqual(testOperator);
        });

        it('updates hostnameElement with hostname from system_info', () => {
            const ctx = createMixinContext();
            const testOperator = {
                operator_id: 'op_123',
                name: 'test-op',
                system_info: { hostname: 'test-host' },
            };
            
            ctx.setOperatorBound(testOperator);
            
            expect(ctx.hostnameElement.textContent).toBe('test-host');
        });

        it('updates hostnameElement with operator name when system_info.hostname is missing', () => {
            const ctx = createMixinContext();
            const testOperator = {
                operator_id: 'op_123',
                name: 'test-op',
                system_info: {},
            };
            
            ctx.setOperatorBound(testOperator);
            
            expect(ctx.hostnameElement.textContent).toBe('test-op');
        });

        it('updates hostnameElement with "operator" fallback when name is missing', () => {
            const ctx = createMixinContext();
            const testOperator = {
                operator_id: 'op_123',
                system_info: {},
            };
            
            ctx.setOperatorBound(testOperator);
            
            expect(ctx.hostnameElement.textContent).toBe('operator');
        });

        it('does not update hostnameElement when element is null', () => {
            const ctx = createMixinContext();
            ctx.hostnameElement = null;
            const testOperator = {
                operator_id: 'op_123',
                name: 'test-op',
                system_info: { hostname: 'test-host' },
            };
            
            expect(() => ctx.setOperatorBound(testOperator)).not.toThrow();
        });

        it('updates promptElement with current_user from system_info', () => {
            const ctx = createMixinContext();
            const testOperator = {
                operator_id: 'op_123',
                system_info: { current_user: 'ubuntu' },
            };
            
            ctx.setOperatorBound(testOperator);
            
            expect(ctx.promptElement.textContent).toBe('ubuntu$');
        });

        it('updates promptElement with "$" when current_user is "$"', () => {
            const ctx = createMixinContext();
            const testOperator = {
                operator_id: 'op_123',
                system_info: { current_user: '$' },
            };
            
            ctx.setOperatorBound(testOperator);
            
            expect(ctx.promptElement.textContent).toBe('$');
        });

        it('updates promptElement with "$" when current_user is missing', () => {
            const ctx = createMixinContext();
            const testOperator = {
                operator_id: 'op_123',
                system_info: {},
            };
            
            ctx.setOperatorBound(testOperator);
            
            expect(ctx.promptElement.textContent).toBe('$');
        });

        it('does not update promptElement when element is null', () => {
            const ctx = createMixinContext();
            ctx.promptElement = null;
            const testOperator = {
                operator_id: 'op_123',
                system_info: { current_user: 'ubuntu' },
            };
            
            expect(() => ctx.setOperatorBound(testOperator)).not.toThrow();
        });

        it('calls updateInputState', () => {
            const ctx = createMixinContext();
            const testOperator = { operator_id: 'op_123' };
            
            ctx.setOperatorBound(testOperator);
            
            expect(ctx.updateInputState).toHaveBeenCalled();
        });

        it('calls appendSystemMessage with connection message', () => {
            const ctx = createMixinContext();
            const testOperator = { operator_id: 'op_123', name: 'test-op' };
            
            ctx.setOperatorBound(testOperator);
            
            expect(ctx.appendSystemMessage).toHaveBeenCalledWith('Connected to test-op');
        });

        it('calls appendSystemMessage with "operator" fallback when name is missing', () => {
            const ctx = createMixinContext();
            const testOperator = { operator_id: 'op_123' };
            
            ctx.setOperatorBound(testOperator);
            
            expect(ctx.appendSystemMessage).toHaveBeenCalledWith('Connected to operator');
        });

        it('does nothing if already bound to same operator', () => {
            const ctx = createMixinContext();
            const testOperator = { operator_id: 'op_123', name: 'test-op' };
            
            ctx.isOperatorBound = true;
            ctx.boundOperator = { operator_id: 'op_123', name: 'test-op' };
            ctx.updateInputState = vi.fn();
            ctx.appendSystemMessage = vi.fn();
            
            ctx.setOperatorBound(testOperator);
            
            expect(ctx.updateInputState).not.toHaveBeenCalled();
            expect(ctx.appendSystemMessage).not.toHaveBeenCalled();
        });

        it('updates operator when bound to different operator', () => {
            const ctx = createMixinContext();
            const newOperator = { operator_id: 'op_456', name: 'new-op' };
            
            ctx.isOperatorBound = true;
            ctx.boundOperator = { operator_id: 'op_123', name: 'old-op' };
            
            ctx.setOperatorBound(newOperator);
            
            expect(ctx.boundOperator).toEqual(newOperator);
            expect(ctx.appendSystemMessage).toHaveBeenCalledWith('Connected to new-op');
        });
    });

    describe('setOperatorUnbound', () => {
        it('sets isOperatorBound to false', () => {
            const ctx = createMixinContext();
            ctx.isOperatorBound = true;
            
            ctx.setOperatorUnbound();
            
            expect(ctx.isOperatorBound).toBe(false);
        });

        it('clears boundOperator', () => {
            const ctx = createMixinContext();
            ctx.boundOperator = { operator_id: 'op_123' };
            
            ctx.setOperatorUnbound();
            
            expect(ctx.boundOperator).toBe(null);
        });

        it('clears hostnameElement textContent', () => {
            const ctx = createMixinContext();
            ctx.hostnameElement.textContent = 'test-host';
            
            ctx.setOperatorUnbound();
            
            expect(ctx.hostnameElement.textContent).toBe('');
        });

        it('does not error when hostnameElement is null', () => {
            const ctx = createMixinContext();
            ctx.hostnameElement = null;
            
            expect(() => ctx.setOperatorUnbound()).not.toThrow();
        });

        it('resets promptElement to "$"', () => {
            const ctx = createMixinContext();
            ctx.promptElement.textContent = 'ubuntu$';
            
            ctx.setOperatorUnbound();
            
            expect(ctx.promptElement.textContent).toBe('$');
        });

        it('does not error when promptElement is null', () => {
            const ctx = createMixinContext();
            ctx.promptElement = null;
            
            expect(() => ctx.setOperatorUnbound()).not.toThrow();
        });

        it('calls updateInputState', () => {
            const ctx = createMixinContext();
            
            ctx.setOperatorUnbound();
            
            expect(ctx.updateInputState).toHaveBeenCalled();
        });
    });
});
