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
import { 
    DeviceLinkStatus,
    DEFAULT_DEVICE_LINK_MAX_USES,
    DEVICE_LINK_MAX_USES_MIN,
    DEVICE_LINK_MAX_USES_MAX
} from '@vsod/public/js/constants/auth-constants.js';

let OperatorDeviceLinkMixin;
let operatorPanelService;
let devLogger;
let notificationService;
let showConfirmationModal;

const TEST_TOKEN = 'test-token-123';
const TEST_OVERLAY_HTML = `
    <div id="device-link-available-slots"></div>
    <input id="device-link-max-uses" type="number">
    <button id="device-link-create-btn"></button>
    <input id="device-link-token-name">
    <select id="device-link-expiry">
        <option value="1">1 Hour</option>
        <option value="24">24 Hours</option>
        <option value="48" selected>48 Hours</option>
    </select>
    <div id="device-link-create-error" class="initially-hidden">
        <span id="device-link-create-error-text"></span>
    </div>
    <div id="device-link-create-section"></div>
    <div id="device-link-token-created" class="initially-hidden"></div>
    <button id="device-link-create-another-btn"></button>
    <div id="device-link-tokens-list"></div>
    <div id="device-link-tokens-loading" class="initially-hidden"></div>
    <div id="device-link-tokens-empty" class="initially-hidden"></div>
`;

function createMixinContext(overrides = {}) {
    const ctx = Object.create(null);
    Object.assign(ctx, OperatorDeviceLinkMixin);
    ctx.maxSlots = 10;
    ctx.usedSlots = 2;
    ctx.copyCurlCommand = vi.fn();
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
    document.body.innerHTML = TEST_OVERLAY_HTML;

    vi.doMock('@vsod/public/js/utils/dev-logger.js', () => ({
        devLogger: { log: vi.fn(), error: vi.fn(), warn: vi.fn() },
    }));

    vi.doMock('@vsod/public/js/utils/operator-panel-service.js', () => ({
        operatorPanelService: {
            createDeviceLink: vi.fn(),
            listDeviceLinks: vi.fn(),
            revokeDeviceLink: vi.fn(),
            deleteDeviceLink: vi.fn(),
        },
    }));

    vi.doMock('@vsod/public/js/utils/html.js', () => ({
        escapeHtml: (text) => {
            if (!text) return '';
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        },
    }));

    vi.doMock('@vsod/public/js/utils/notification-service.js', () => ({
        notificationService: {
            error: vi.fn(),
            info: vi.fn(),
            success: vi.fn(),
            warning: vi.fn(),
        },
    }));

    vi.doMock('@vsod/public/js/utils/ui-utils.js', () => ({
        showConfirmationModal: vi.fn(),
    }));

    ({ OperatorDeviceLinkMixin } = await import('@vsod/public/js/components/operator-device-link-mixin.js'));
    ({ operatorPanelService } = await import('@vsod/public/js/utils/operator-panel-service.js'));
    ({ devLogger } = await import('@vsod/public/js/utils/dev-logger.js'));
    const nsMod = await import('@vsod/public/js/utils/notification-service.js');
    notificationService = nsMod.notificationService;
    const uiUtilsMod = await import('@vsod/public/js/utils/ui-utils.js');
    showConfirmationModal = uiUtilsMod.showConfirmationModal;

    // Mock confirm and alert (legacy, should be replaced with notificationService)
    vi.stubGlobal('confirm', vi.fn(() => true));
    vi.stubGlobal('alert', vi.fn());
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
});

describe('OperatorDeviceLinkMixin [UNIT - jsdom]', () => {

    describe('_initDeviceLinkDeploymentSection', () => {
        it('calls _loadDeviceLinkAvailableSlots and _loadDeviceLinks', async () => {
            const ctx = createMixinContext();
            ctx._loadDeviceLinkAvailableSlots = vi.fn();
            ctx._loadDeviceLinks = vi.fn();
            const overlay = document.body;

            await ctx._initDeviceLinkDeploymentSection(overlay);

            expect(ctx._loadDeviceLinkAvailableSlots).toHaveBeenCalledWith(overlay);
            expect(ctx._loadDeviceLinks).toHaveBeenCalledWith(overlay);
        });
    });

    describe('_loadDeviceLinkAvailableSlots', () => {
        it('updates available slots span and max uses input', async () => {
            const ctx = createMixinContext({ maxSlots: 5, usedSlots: 2 });
            const overlay = document.body;
            const span = overlay.querySelector('#device-link-available-slots');
            const input = overlay.querySelector('#device-link-max-uses');

            await ctx._loadDeviceLinkAvailableSlots(overlay);

            expect(span.textContent).toBe('3');
            expect(input.max).toBe('3');
            expect(input.value).toBe('3');
        });

        it('handles Infinity maxSlots', async () => {
            const ctx = createMixinContext({ maxSlots: Infinity, usedSlots: 10 });
            const overlay = document.body;
            const span = overlay.querySelector('#device-link-available-slots');
            const input = overlay.querySelector('#device-link-max-uses');

            await ctx._loadDeviceLinkAvailableSlots(overlay);

            expect(span.textContent).toBe('Unlimited');
            expect(input.max).toBe(''); // No max for Infinity
        });

        it('defaults to 0 available slots if maxSlots - usedSlots < 0', async () => {
            const ctx = createMixinContext({ maxSlots: 2, usedSlots: 5 });
            const overlay = document.body;
            const span = overlay.querySelector('#device-link-available-slots');

            await ctx._loadDeviceLinkAvailableSlots(overlay);

            expect(span.textContent).toBe('0');
        });

        it('sets span to "-" on error', async () => {
            const ctx = createMixinContext();
            // Stub the property with a getter that throws
            Object.defineProperty(ctx, 'maxSlots', {
                get() { throw new Error('fail'); }
            });
            const overlay = document.body;
            const span = overlay.querySelector('#device-link-available-slots');

            await ctx._loadDeviceLinkAvailableSlots(overlay);

            expect(span.textContent).toBe('-');
            expect(devLogger.error).toHaveBeenCalled();
        });
    });

    describe('_bindDeviceLinkCreateForm', () => {
        it('binds click events to create and create-another buttons', () => {
            const ctx = createMixinContext();
            ctx._createDeviceLink = vi.fn();
            const overlay = document.body;
            ctx._bindDeviceLinkCreateForm(overlay);

            overlay.querySelector('#device-link-create-btn').click();
            expect(ctx._createDeviceLink).toHaveBeenCalledWith(overlay);

            const section = overlay.querySelector('#device-link-create-section');
            section.classList.add('initially-hidden');
            overlay.querySelector('#device-link-create-another-btn').click();
            expect(section.classList.contains('initially-hidden')).toBe(false);
        });
    });

    describe('_createDeviceLink', () => {
        let overlay, elements;

        beforeEach(() => {
            elements = {
                '#device-link-create-btn': document.createElement('button'),
                '#device-link-token-name': document.createElement('input'),
                '#device-link-max-uses': document.createElement('input'),
                '#device-link-expiry': document.createElement('select'),
                '#device-link-create-error': document.createElement('div'),
                '#device-link-create-error-text': document.createElement('span'),
                '#device-link-create-section': document.createElement('div'),
                '#device-link-token-created': document.createElement('div'),
                '#device-link-create-another-btn': document.createElement('button'),
            };
            overlay = {
                querySelector: vi.fn(selector => elements[selector] || null)
            };
            
            // Set some default values
            elements['#device-link-token-name'].value = 'Test Token';
            elements['#device-link-max-uses'].value = '5';
            const expiry = elements['#device-link-expiry'];
            const opt = document.createElement('option');
            opt.value = '24';
            expiry.appendChild(opt);
            expiry.value = '24';
        });

        it('successfully creates a device link and reloads lists', async () => {
            const ctx = createMixinContext();
            
            operatorPanelService.createDeviceLink.mockResolvedValue({
                ok: true,
                json: async () => ({ success: true, token: TEST_TOKEN })
            });

            ctx._loadDeviceLinks = vi.fn();
            ctx._loadDeviceLinkAvailableSlots = vi.fn();

            await ctx._createDeviceLink(overlay);

            expect(operatorPanelService.createDeviceLink).toHaveBeenCalledWith({
                name: 'Test Token',
                maxUses: 5,
                expiresInHours: 24
            });
            expect(ctx._loadDeviceLinks).toHaveBeenCalled();
            expect(ctx._loadDeviceLinkAvailableSlots).toHaveBeenCalled();
            expect(elements['#device-link-token-name'].value).toBe('');
        });

        it('shows error if max uses is out of range (above max)', async () => {
            const ctx = createMixinContext();
            elements['#device-link-max-uses'].value = String(DEVICE_LINK_MAX_USES_MAX + 1);

            const spy = vi.spyOn(ctx, '_showDeviceLinkError');

            await ctx._createDeviceLink(overlay);

            const expectedMsg = `Max uses must be between ${DEVICE_LINK_MAX_USES_MIN.toLocaleString()} and ${DEVICE_LINK_MAX_USES_MAX.toLocaleString()}`;
            expect(spy).toHaveBeenCalledWith(
                elements['#device-link-create-error'],
                elements['#device-link-create-error-text'],
                expectedMsg
            );
            
            expect(elements['#device-link-create-error-text'].textContent).toBe(expectedMsg);
            expect(operatorPanelService.createDeviceLink).not.toHaveBeenCalled();
        });

        it('shows error if max uses is 0 (below min)', async () => {
            const ctx = createMixinContext();
            elements['#device-link-max-uses'].value = '0';

            const spy = vi.spyOn(ctx, '_showDeviceLinkError');

            await ctx._createDeviceLink(overlay);

            const expectedMsg = `Max uses must be between ${DEVICE_LINK_MAX_USES_MIN.toLocaleString()} and ${DEVICE_LINK_MAX_USES_MAX.toLocaleString()}`;
            expect(spy).toHaveBeenCalledWith(
                elements['#device-link-create-error'],
                elements['#device-link-create-error-text'],
                expectedMsg
            );
            expect(operatorPanelService.createDeviceLink).not.toHaveBeenCalled();
        });

        it('defaults to DEFAULT_DEVICE_LINK_MAX_USES when input is empty/NaN', async () => {
            const ctx = createMixinContext();
            elements['#device-link-max-uses'].value = '';
            elements['#device-link-token-name'].value = '';

            operatorPanelService.createDeviceLink.mockResolvedValue({
                ok: true,
                json: async () => ({ success: true, token: TEST_TOKEN })
            });

            ctx._loadDeviceLinks = vi.fn();
            ctx._loadDeviceLinkAvailableSlots = vi.fn();

            await ctx._createDeviceLink(overlay);

            expect(operatorPanelService.createDeviceLink).toHaveBeenCalledWith({
                name: '',
                maxUses: DEFAULT_DEVICE_LINK_MAX_USES,
                expiresInHours: 24
            });
        });

        it('handles API errors gracefully', async () => {
            const ctx = createMixinContext();
            
            operatorPanelService.createDeviceLink.mockResolvedValue({
                ok: false,
                json: async () => ({ success: false, error: 'Limit reached' })
            });

            await ctx._createDeviceLink(overlay);

            expect(elements['#device-link-create-error-text'].textContent).toBe('Limit reached');
            expect(devLogger.error).toHaveBeenCalled();
        });

        it('disables create button during request', async () => {
            const ctx = createMixinContext();
            const createBtn = elements['#device-link-create-btn'];

            let resolveRequest;
            operatorPanelService.createDeviceLink.mockReturnValue(new Promise(resolve => {
                resolveRequest = resolve;
            }));

            const createPromise = ctx._createDeviceLink(overlay);
            expect(createBtn.disabled).toBe(true);
            expect(createBtn.innerHTML).toContain('rotating');

            resolveRequest({
                ok: true,
                json: async () => ({ success: true })
            });
            await createPromise;

            expect(createBtn.disabled).toBe(false);
            expect(createBtn.innerHTML).toContain('play_arrow');
        });
    });

    describe('_loadDeviceLinks', () => {
        it('renders device link elements for each token', async () => {
            const ctx = createMixinContext();
            const overlay = document.body;
            const tokensList = overlay.querySelector('#device-link-tokens-list');

            const mockLinks = [
                { token: 't1', name: 'Link 1', status: DeviceLinkStatus.ACTIVE, created_at: Date.now(), expires_at: Date.now() + 3600000, uses: 0, max_uses: 10 },
                { token: 't2', name: 'Link 2', status: DeviceLinkStatus.EXHAUSTED, created_at: Date.now(), expires_at: Date.now() + 3600000, uses: 10, max_uses: 10 }
            ];

            operatorPanelService.listDeviceLinks.mockResolvedValue({
                ok: true,
                json: async () => ({ links: mockLinks })
            });

            await ctx._loadDeviceLinks(overlay);

            expect(tokensList.children.length).toBe(2);
            expect(tokensList.innerHTML).toContain('Link 1');
            expect(tokensList.innerHTML).toContain('Link 2');
        });

        it('shows empty state if no tokens found', async () => {
            const ctx = createMixinContext();
            const overlay = document.body;
            const tokensEmpty = overlay.querySelector('#device-link-tokens-empty');

            operatorPanelService.listDeviceLinks.mockResolvedValue({
                ok: true,
                json: async () => ({ links: [] })
            });

            await ctx._loadDeviceLinks(overlay);

            expect(tokensEmpty.classList.contains('initially-hidden')).toBe(false);
        });

        it('shows error state on failure', async () => {
            const ctx = createMixinContext();
            const overlay = document.body;
            const tokensEmpty = overlay.querySelector('#device-link-tokens-empty');

            operatorPanelService.listDeviceLinks.mockResolvedValue({ ok: false });

            await ctx._loadDeviceLinks(overlay);

            expect(tokensEmpty.innerHTML).toContain('Failed to load tokens');
            expect(tokensEmpty.classList.contains('initially-hidden')).toBe(false);
        });
    });

    describe('_createDeviceLinkElement', () => {
        it('creates element with correct status classes for ACTIVE token', () => {
            const ctx = createMixinContext();
            const token = {
                token: 't-active',
                name: 'Active Link',
                status: DeviceLinkStatus.ACTIVE,
                created_at: new Date('2026-01-01').getTime(),
                expires_at: new Date('2026-12-31').getTime(),
                uses: 1,
                max_uses: 10
            };

            const el = ctx._createDeviceLinkElement(token, document.body);

            expect(el.querySelector('.device-link-status-active')).toBeTruthy();
            expect(el.querySelector('.device-link-copy-btn')).toBeTruthy();
            expect(el.querySelector('.device-link-revoke-btn')).toBeTruthy();
            expect(el.querySelector('.device-link-delete-btn')).toBeFalsy();
        });

        it('creates element with EXPIRED status if date passed', () => {
            const ctx = createMixinContext();
            const token = {
                token: 't-expired',
                name: 'Expired Link',
                status: DeviceLinkStatus.ACTIVE,
                created_at: new Date('2026-01-01').getTime(),
                expires_at: new Date('2026-01-02').getTime(),
                uses: 0,
                max_uses: 10
            };

            const el = ctx._createDeviceLinkElement(token, document.body);

            expect(el.textContent).toContain(DeviceLinkStatus.EXPIRED);
            expect(el.querySelector('.device-link-delete-btn')).toBeTruthy();
            expect(el.querySelector('.device-link-revoke-btn')).toBeFalsy();
        });

        it('binds copy actions', () => {
            const ctx = createMixinContext();
            const token = {
                token: 't1',
                operator_command: 'g8e exec t1',
                status: DeviceLinkStatus.ACTIVE,
                expires_at: Date.now() + 86400000
            };

            const el = ctx._createDeviceLinkElement(token, document.body);
            
            el.querySelector('.device-link-copy-btn').click();
            expect(ctx.copyCurlCommand).toHaveBeenCalledWith('t1', expect.anything());

            el.querySelector('.device-link-copy-cmd-btn').click();
            expect(ctx.copyCurlCommand).toHaveBeenCalledWith('g8e exec t1', expect.anything());
        });

        it('falls back to default operator command if missing', () => {
            const ctx = createMixinContext();
            const token = {
                token: 't1',
                status: DeviceLinkStatus.ACTIVE,
                expires_at: Date.now() + 86400000
            };

            const el = ctx._createDeviceLinkElement(token, document.body);
            
            el.querySelector('.device-link-copy-cmd-btn').click();
            expect(ctx.copyCurlCommand).toHaveBeenCalledWith('g8e.operator --device-token t1', expect.anything());
        });

        it('shows EXHAUSTED status', () => {
            const ctx = createMixinContext();
            const token = {
                token: 't-exhausted',
                status: DeviceLinkStatus.EXHAUSTED,
                created_at: Date.now(),
                expires_at: Date.now() + 3600000,
                uses: 10,
                max_uses: 10
            };

            const el = ctx._createDeviceLinkElement(token, document.body);
            expect(el.querySelector('.device-link-status-exhausted')).toBeTruthy();
            expect(el.querySelector('.device-link-delete-btn')).toBeTruthy();
        });

        it('shows REVOKED status', () => {
            const ctx = createMixinContext();
            const token = {
                token: 't-revoked',
                status: DeviceLinkStatus.REVOKED,
                created_at: Date.now(),
                expires_at: Date.now() + 3600000,
                uses: 0,
                max_uses: 10
            };

            const el = ctx._createDeviceLinkElement(token, document.body);
            expect(el.querySelector('.device-link-status-revoked')).toBeTruthy();
            expect(el.querySelector('.device-link-delete-btn')).toBeTruthy();
        });
    });

    describe('_revokeDeviceLink', () => {
        it('shows confirmation modal and revokes the link', async () => {
            const ctx = createMixinContext();
            const overlay = document.body;
            ctx._loadDeviceLinks = vi.fn();
            
            showConfirmationModal.mockResolvedValue(true);
            operatorPanelService.revokeDeviceLink.mockResolvedValue({ ok: true });

            await ctx._revokeDeviceLink('token-to-revoke', overlay);

            expect(operatorPanelService.revokeDeviceLink).toHaveBeenCalledWith('token-to-revoke');
            expect(ctx._loadDeviceLinks).toHaveBeenCalled();
        });

        it('aborts if not confirmed', async () => {
            const ctx = createMixinContext();
            showConfirmationModal.mockResolvedValue(false);

            await ctx._revokeDeviceLink('token-to-revoke', document.body);

            expect(operatorPanelService.revokeDeviceLink).not.toHaveBeenCalled();
        });

        it('shows notificationService error on error', async () => {
            const ctx = createMixinContext();
            showConfirmationModal.mockResolvedValue(true);
            operatorPanelService.revokeDeviceLink.mockResolvedValue({
                ok: false,
                json: async () => ({ error: 'Forbidden' })
            });

            await ctx._revokeDeviceLink('token-to-revoke', document.body);

            expect(notificationService.error).toHaveBeenCalledWith(expect.stringContaining('Forbidden'));
        });
    });

    describe('_deleteDeviceLink', () => {
        it('deletes the link without confirmation', async () => {
            const ctx = createMixinContext();
            const overlay = document.body;
            ctx._loadDeviceLinks = vi.fn();
            
            operatorPanelService.deleteDeviceLink.mockResolvedValue({ ok: true });

            await ctx._deleteDeviceLink('token-to-delete', overlay);

            expect(operatorPanelService.deleteDeviceLink).toHaveBeenCalledWith('token-to-delete');
            expect(ctx._loadDeviceLinks).toHaveBeenCalled();
        });

        it('shows notificationService error on error', async () => {
            const ctx = createMixinContext();
            operatorPanelService.deleteDeviceLink.mockResolvedValue({
                ok: false,
                json: async () => ({ error: 'Not found' })
            });

            await ctx._deleteDeviceLink('token-to-delete', document.body);

            expect(notificationService.error).toHaveBeenCalledWith(expect.stringContaining('Not found'));
        });
    });

    describe('_showDeviceLinkError', () => {
        it('updates error text and shows div', () => {
            const ctx = createMixinContext();
            const div = document.createElement('div');
            div.classList.add('initially-hidden');
            const text = document.createElement('span');
            
            ctx._showDeviceLinkError(div, text, 'Something went wrong');

            expect(text.textContent).toBe('Something went wrong');
            expect(div.classList.contains('initially-hidden')).toBe(false);
        });
    });
});
