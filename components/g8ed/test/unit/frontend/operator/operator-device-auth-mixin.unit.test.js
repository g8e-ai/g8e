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

let OperatorDeviceAuthMixin;
let operatorPanelService;
let mockDevLogger;
let notificationService;

const TEST_TOKEN = 'auth_token_abc123def456';
const TEST_OPERATOR_ID = 'op_test_operator_id';
const TEST_EXPIRES_AT = new Date(Date.now() + 30000).toISOString();

function buildMockOperatorCard(operatorId) {
    const card = document.createElement('div');
    card.className = 'operator-item';
    card.dataset.operatorId = operatorId;
    card.innerHTML = `
        <div class="operator-item-info">
            <div class="operator-item-hostname">test-hostname</div>
        </div>
    `;
    return card;
}

function buildMockDrawerList() {
    const drawerList = document.createElement('div');
    drawerList.className = 'operator-drawer-list';
    return drawerList;
}

function createMixinContext(overrides = {}) {
    const ctx = Object.create(null);
    Object.assign(ctx, OperatorDeviceAuthMixin);
    ctx.drawerList = buildMockDrawerList();
    ctx._pendingAuthRequests = new Map();
    ctx._escapeHtml = (text) => {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    };
    Object.assign(ctx, overrides);
    return ctx;
}

beforeEach(async () => {
    vi.resetModules();
    vi.useFakeTimers();

    global.alert = vi.fn();

    mockDevLogger = { log: vi.fn(), error: vi.fn(), warn: vi.fn() };

    vi.doMock('@g8ed/public/js/utils/dev-logger.js', () => ({
        devLogger: mockDevLogger,
    }));

    vi.doMock('@g8ed/public/js/utils/operator-panel-service.js', () => ({
        operatorPanelService: {
            authorizeDevice: vi.fn(),
            rejectDevice: vi.fn(),
        },
    }));

    vi.doMock('@g8ed/public/js/utils/notification-service.js', () => ({
        notificationService: {
            error: vi.fn(),
            info: vi.fn(),
            success: vi.fn(),
            warning: vi.fn(),
        },
    }));

    const mod = await import('@g8ed/public/js/components/operator-device-auth-mixin.js');
    OperatorDeviceAuthMixin = mod.OperatorDeviceAuthMixin;

    const opsMod = await import('@g8ed/public/js/utils/operator-panel-service.js');
    operatorPanelService = opsMod.operatorPanelService;

    const nsMod = await import('@g8ed/public/js/utils/notification-service.js');
    notificationService = nsMod.notificationService;
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
});

describe('OperatorDeviceAuthMixin [UNIT - jsdom]', () => {

    describe('handleDevicePendingAuthorization', () => {
        it('shows pending auth UI on operator card when card exists', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device.local' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            expect(card.classList.contains('pending-device-auth')).toBe(true);
            const hostnameEl = card.querySelector('.operator-item-hostname');
            expect(hostnameEl.textContent).toBe('test-device.local');
            expect(hostnameEl.classList.contains('pending-auth-hostname')).toBe(true);
        });

        it('escapes HTML in hostname to prevent XSS', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const maliciousHostname = '<script>alert("xss")</script>';
            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: maliciousHostname },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            const hostnameEl = card.querySelector('.operator-item-hostname');
            expect(hostnameEl.innerHTML).not.toContain('<script>');
            expect(hostnameEl.innerHTML).toContain('&lt;script&gt;');
        });

        it('uses "Unknown device" when hostname is missing', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: {},
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            const hostnameEl = card.querySelector('.operator-item-hostname');
            expect(hostnameEl.textContent).toBe('Unknown device');
        });

        it('uses "Unknown device" when device_info is missing', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            const hostnameEl = card.querySelector('.operator-item-hostname');
            expect(hostnameEl.textContent).toBe('Unknown device');
        });

        it('creates approve and deny buttons with correct data attributes', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            const approveBtn = card.querySelector('.device-auth-approve-btn');
            expect(approveBtn).toBeTruthy();
            expect(approveBtn.dataset.token).toBe(TEST_TOKEN);
            expect(approveBtn.dataset.operatorId).toBe(TEST_OPERATOR_ID);
            expect(approveBtn.innerHTML).toContain('check');
            expect(approveBtn.innerHTML).toContain('Approve');

            const denyBtn = card.querySelector('.device-auth-deny-btn');
            expect(denyBtn).toBeTruthy();
            expect(denyBtn.dataset.token).toBe(TEST_TOKEN);
            expect(denyBtn.dataset.operatorId).toBe(TEST_OPERATOR_ID);
            expect(denyBtn.innerHTML).toContain('close');
            expect(denyBtn.innerHTML).toContain('Deny');
        });

        it('binds click handlers to approve and deny buttons', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            const authorizeSpy = vi.spyOn(ctx, 'authorizeDevice');
            const rejectSpy = vi.spyOn(ctx, 'rejectDevice');

            ctx.handleDevicePendingAuthorization(data);

            const approveBtn = card.querySelector('.device-auth-approve-btn');
            const denyBtn = card.querySelector('.device-auth-deny-btn');

            approveBtn.click();
            expect(authorizeSpy).toHaveBeenCalledWith(TEST_TOKEN, TEST_OPERATOR_ID);

            denyBtn.click();
            expect(rejectSpy).toHaveBeenCalledWith(TEST_TOKEN, TEST_OPERATOR_ID);
        });

        it('clears existing pending auth UI before showing new one', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data1 = {
                token: 'old_token',
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'old-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data1);

            const oldAuthContainer = card.querySelector('.device-auth-inline-container');
            expect(oldAuthContainer).toBeTruthy();

            const data2 = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'new-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data2);

            const newAuthContainer = card.querySelector('.device-auth-inline-container');
            expect(newAuthContainer).toBeTruthy();
            expect(card.querySelectorAll('.device-auth-inline-container')).toHaveLength(1);
        });

        it('stores pending auth request in Map with timeout', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            expect(ctx._pendingAuthRequests.has(TEST_OPERATOR_ID)).toBe(true);
            const pending = ctx._pendingAuthRequests.get(TEST_OPERATOR_ID);
            expect(pending.token).toBe(TEST_TOKEN);
            expect(pending.device_info).toEqual({ hostname: 'test-device' });
            expect(pending.timeout).toBeTruthy();
        });

        it('sets timeout to clear UI on expiry', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const clearSpy = vi.spyOn(ctx, 'clearInlineAuthUI');

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            vi.advanceTimersByTime(30000);

            expect(clearSpy).toHaveBeenCalledWith(TEST_OPERATOR_ID, TEST_TOKEN, false);
        });

        it('does not set timeout if expires_at is in the past', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const pastExpiry = new Date(Date.now() - 10000).toISOString();
            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: pastExpiry,
            };

            ctx.handleDevicePendingAuthorization(data);

            const pending = ctx._pendingAuthRequests.get(TEST_OPERATOR_ID);
            expect(pending.timeout).toBeNull();
        });

        it('logs warning when operator card not found', () => {
            const ctx = createMixinContext();

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            expect(mockDevLogger.warn).toHaveBeenCalledWith(
                '[OPERATOR] Operator card not found for pending auth:',
                TEST_OPERATOR_ID
            );
        });

        it('does not modify DOM when operator card not found', () => {
            const ctx = createMixinContext();

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            expect(ctx._pendingAuthRequests.has(TEST_OPERATOR_ID)).toBe(false);
        });

        it('appends auth container after item-info when present', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            const itemInfo = card.querySelector('.operator-item-info');
            const authContainer = card.querySelector('.device-auth-inline-container');
            expect(itemInfo.nextSibling).toBe(authContainer);
        });

        it('appends auth container to card when item-info is missing', () => {
            const ctx = createMixinContext();
            const card = document.createElement('div');
            card.className = 'operator-item';
            card.dataset.operatorId = TEST_OPERATOR_ID;
            card.innerHTML = '<div>Some content</div>';
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            const authContainer = card.querySelector('.device-auth-inline-container');
            expect(authContainer).toBeTruthy();
            expect(card.lastChild).toBe(authContainer);
        });
    });

    describe('_clearPendingAuthUI', () => {
        it('clears timeout from pending request', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            const pending = ctx._pendingAuthRequests.get(TEST_OPERATOR_ID);
            const clearTimeoutSpy = vi.spyOn(window, 'clearTimeout');

            ctx._clearPendingAuthUI(TEST_OPERATOR_ID);

            expect(clearTimeoutSpy).toHaveBeenCalledWith(pending.timeout);
        });

        it('removes pending auth request from Map', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);
            expect(ctx._pendingAuthRequests.has(TEST_OPERATOR_ID)).toBe(true);

            ctx._clearPendingAuthUI(TEST_OPERATOR_ID);

            expect(ctx._pendingAuthRequests.has(TEST_OPERATOR_ID)).toBe(false);
        });

        it('removes pending-device-auth class from operator card', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);
            expect(card.classList.contains('pending-device-auth')).toBe(true);

            ctx._clearPendingAuthUI(TEST_OPERATOR_ID);

            expect(card.classList.contains('pending-device-auth')).toBe(false);
        });

        it('removes auth container from DOM', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);
            expect(card.querySelector('.device-auth-inline-container')).toBeTruthy();

            ctx._clearPendingAuthUI(TEST_OPERATOR_ID);

            expect(card.querySelector('.device-auth-inline-container')).toBeFalsy();
        });

        it('removes pending-auth-hostname class from hostname element', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);
            const hostnameEl = card.querySelector('.operator-item-hostname');
            expect(hostnameEl.classList.contains('pending-auth-hostname')).toBe(true);

            ctx._clearPendingAuthUI(TEST_OPERATOR_ID);

            expect(hostnameEl.classList.contains('pending-auth-hostname')).toBe(false);
        });

        it('handles missing operator card gracefully', () => {
            const ctx = createMixinContext();

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            expect(() => ctx._clearPendingAuthUI(TEST_OPERATOR_ID)).not.toThrow();
        });

        it('handles null timeout gracefully', () => {
            const ctx = createMixinContext();
            ctx._pendingAuthRequests.set(TEST_OPERATOR_ID, { token: TEST_TOKEN, device_info: {}, timeout: null });

            const clearTimeoutSpy = vi.spyOn(window, 'clearTimeout');

            expect(() => ctx._clearPendingAuthUI(TEST_OPERATOR_ID)).not.toThrow();
            expect(clearTimeoutSpy).not.toHaveBeenCalled();
        });
    });

    describe('clearInlineAuthUI', () => {
        it('initializes _pendingAuthRequests Map if not exists', () => {
            const ctx = createMixinContext();
            delete ctx._pendingAuthRequests;

            ctx.clearInlineAuthUI(TEST_OPERATOR_ID);

            expect(ctx._pendingAuthRequests).toBeInstanceOf(Map);
        });

        it('clears UI when expectedToken matches', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            ctx.clearInlineAuthUI(TEST_OPERATOR_ID, TEST_TOKEN);

            expect(ctx._pendingAuthRequests.has(TEST_OPERATOR_ID)).toBe(false);
            expect(card.classList.contains('pending-device-auth')).toBe(false);
        });

        it('does not clear UI when expectedToken does not match', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            ctx.clearInlineAuthUI(TEST_OPERATOR_ID, 'different_token');

            expect(ctx._pendingAuthRequests.has(TEST_OPERATOR_ID)).toBe(true);
            expect(card.classList.contains('pending-device-auth')).toBe(true);
        });

        it('clears UI when expectedToken is null (no validation)', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            ctx.clearInlineAuthUI(TEST_OPERATOR_ID, null);

            expect(ctx._pendingAuthRequests.has(TEST_OPERATOR_ID)).toBe(false);
        });

        it('clears timeout when shouldClearTimeout is true', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            const pending = ctx._pendingAuthRequests.get(TEST_OPERATOR_ID);
            const clearTimeoutSpy = vi.spyOn(window, 'clearTimeout');

            ctx.clearInlineAuthUI(TEST_OPERATOR_ID, TEST_TOKEN, true);

            expect(clearTimeoutSpy).toHaveBeenCalledWith(pending.timeout);
        });

        it('does not clear timeout when shouldClearTimeout is false', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            const clearTimeoutSpy = vi.spyOn(window, 'clearTimeout');

            ctx.clearInlineAuthUI(TEST_OPERATOR_ID, TEST_TOKEN, false);

            expect(clearTimeoutSpy).not.toHaveBeenCalled();
        });

        it('handles missing operator card gracefully', () => {
            const ctx = createMixinContext();

            expect(() => ctx.clearInlineAuthUI(TEST_OPERATOR_ID)).not.toThrow();
        });

        it('logs clearance action', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            ctx.clearInlineAuthUI(TEST_OPERATOR_ID);

            expect(mockDevLogger.log).toHaveBeenCalledWith(
                '[OPERATOR] Cleared inline auth UI:',
                { operatorId: TEST_OPERATOR_ID }
            );
        });
    });

    describe('authorizeDevice', () => {
        it('calls operatorPanelService.authorizeDevice with token', async () => {
            const ctx = createMixinContext();

            operatorPanelService.authorizeDevice.mockResolvedValue({
                ok: true,
                json: async () => ({ success: true }),
            });

            await ctx.authorizeDevice(TEST_TOKEN, TEST_OPERATOR_ID);

            expect(operatorPanelService.authorizeDevice).toHaveBeenCalledWith(TEST_TOKEN);
        });

        it('clears UI on successful authorization', async () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            operatorPanelService.authorizeDevice.mockResolvedValue({
                ok: true,
                json: async () => ({ success: true }),
            });

            await ctx.authorizeDevice(TEST_TOKEN, TEST_OPERATOR_ID);

            expect(ctx._pendingAuthRequests.has(TEST_OPERATOR_ID)).toBe(false);
            expect(card.classList.contains('pending-device-auth')).toBe(false);
        });

        it('shows notificationService error on failed authorization', async () => {
            const ctx = createMixinContext();

            operatorPanelService.authorizeDevice.mockResolvedValue({
                ok: false,
                json: async () => ({ error: 'Invalid token' }),
            });

            await ctx.authorizeDevice(TEST_TOKEN, TEST_OPERATOR_ID);

            expect(notificationService.error).toHaveBeenCalledWith('Failed to authorize device: Invalid token');
        });

        it('shows notificationService error with generic error message when error field missing', async () => {
            const ctx = createMixinContext();

            operatorPanelService.authorizeDevice.mockResolvedValue({
                ok: false,
                json: async () => ({}),
            });

            await ctx.authorizeDevice(TEST_TOKEN, TEST_OPERATOR_ID);

            expect(notificationService.error).toHaveBeenCalledWith('Failed to authorize device: Unknown error');
        });

        it('shows notificationService error on network error', async () => {
            const ctx = createMixinContext();

            operatorPanelService.authorizeDevice.mockRejectedValue(new Error('Network error'));

            await ctx.authorizeDevice(TEST_TOKEN, TEST_OPERATOR_ID);

            expect(notificationService.error).toHaveBeenCalledWith('Failed to authorize device: Network error');
        });

        it('logs success on successful authorization', async () => {
            const ctx = createMixinContext();

            operatorPanelService.authorizeDevice.mockResolvedValue({
                ok: true,
                json: async () => ({ success: true }),
            });

            await ctx.authorizeDevice(TEST_TOKEN, TEST_OPERATOR_ID);

            expect(mockDevLogger.log).toHaveBeenCalledWith(
                '[OPERATOR] Device authorized:',
                { operatorId: TEST_OPERATOR_ID, token: expect.stringContaining('...') }
            );
        });

        it('logs error on failed authorization', async () => {
            const ctx = createMixinContext();

            operatorPanelService.authorizeDevice.mockRejectedValue(new Error('Network error'));

            await ctx.authorizeDevice(TEST_TOKEN, TEST_OPERATOR_ID);

            expect(mockDevLogger.error).toHaveBeenCalledWith(
                '[OPERATOR] Failed to authorize device:',
                expect.any(Error)
            );
        });
    });

    describe('rejectDevice', () => {
        it('calls operatorPanelService.rejectDevice with token', async () => {
            const ctx = createMixinContext();

            operatorPanelService.rejectDevice.mockResolvedValue({
                ok: true,
            });

            await ctx.rejectDevice(TEST_TOKEN, TEST_OPERATOR_ID);

            expect(operatorPanelService.rejectDevice).toHaveBeenCalledWith(TEST_TOKEN);
        });

        it('clears UI after rejection', async () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            operatorPanelService.rejectDevice.mockResolvedValue({
                ok: true,
            });

            await ctx.rejectDevice(TEST_TOKEN, TEST_OPERATOR_ID);

            expect(ctx._pendingAuthRequests.has(TEST_OPERATOR_ID)).toBe(false);
            expect(card.classList.contains('pending-device-auth')).toBe(false);
        });

        it('logs success on successful rejection', async () => {
            const ctx = createMixinContext();

            operatorPanelService.rejectDevice.mockResolvedValue({
                ok: true,
            });

            await ctx.rejectDevice(TEST_TOKEN, TEST_OPERATOR_ID);

            expect(mockDevLogger.log).toHaveBeenCalledWith(
                '[OPERATOR] Device rejected:',
                { operatorId: TEST_OPERATOR_ID, token: expect.stringContaining('...') }
            );
        });

        it('logs error on failed rejection', async () => {
            const ctx = createMixinContext();

            operatorPanelService.rejectDevice.mockRejectedValue(new Error('Network error'));

            await ctx.rejectDevice(TEST_TOKEN, TEST_OPERATOR_ID);

            expect(mockDevLogger.error).toHaveBeenCalledWith(
                '[OPERATOR] Failed to reject device:',
                expect.any(Error)
            );
        });

        it('clears UI even when rejection fails', async () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            operatorPanelService.rejectDevice.mockRejectedValue(new Error('Network error'));

            await ctx.rejectDevice(TEST_TOKEN, TEST_OPERATOR_ID);

            expect(ctx._pendingAuthRequests.has(TEST_OPERATOR_ID)).toBe(false);
            expect(card.classList.contains('pending-device-auth')).toBe(false);
        });
    });

    describe('handleDeviceAuthorized', () => {
        it('clears UI when operator_id is present', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            const eventData = { operator_id: TEST_OPERATOR_ID };

            ctx.handleDeviceAuthorized(eventData);

            expect(ctx._pendingAuthRequests.has(TEST_OPERATOR_ID)).toBe(false);
            expect(card.classList.contains('pending-device-auth')).toBe(false);
        });

        it('logs when operator_id is present', () => {
            const ctx = createMixinContext();

            const eventData = { operator_id: TEST_OPERATOR_ID };

            ctx.handleDeviceAuthorized(eventData);

            expect(mockDevLogger.log).toHaveBeenCalledWith(
                '[OPERATOR] Device authorized via SSE:',
                { operator_id: TEST_OPERATOR_ID }
            );
        });

        it('logs warning when operator_id is missing', () => {
            const ctx = createMixinContext();

            const eventData = {};

            ctx.handleDeviceAuthorized(eventData);

            expect(mockDevLogger.warn).toHaveBeenCalledWith(
                '[OPERATOR] handleDeviceAuthorized called without operator_id'
            );
        });

        it('does not clear UI when operator_id is missing', () => {
            const ctx = createMixinContext();
            const card = buildMockOperatorCard(TEST_OPERATOR_ID);
            ctx.drawerList.appendChild(card);

            const data = {
                token: TEST_TOKEN,
                operator_id: TEST_OPERATOR_ID,
                device_info: { hostname: 'test-device' },
                expires_at: TEST_EXPIRES_AT,
            };

            ctx.handleDevicePendingAuthorization(data);

            const eventData = {};

            ctx.handleDeviceAuthorized(eventData);

            expect(ctx._pendingAuthRequests.has(TEST_OPERATOR_ID)).toBe(true);
            expect(card.classList.contains('pending-device-auth')).toBe(true);
        });
    });
});
