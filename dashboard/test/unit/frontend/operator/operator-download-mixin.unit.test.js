// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

let OperatorDownloadMixin;
let operatorPanelService;

const TEST_API_KEY = 'dak_abcdefghijklmnopqrstuvwxyz1234567890';

function buildMockContainer() {
    const container = document.createElement('div');
    container.innerHTML = `
        <div id="operator-binary-downloads">
            <div class="operator-download-links-row">
                <a class="operator-download-link" data-os="linux" data-arch="amd64" href="#">Linux x64</a>
                <a class="operator-download-link" data-os="linux" data-arch="arm64" href="#">Linux ARM64</a>
                <a class="operator-download-link" data-os="linux" data-arch="386" href="#">Linux x86</a>
            </div>
            <div class="operator-deploy-section">
                <div class="operator-deploy-row">
                    <span class="operator-deploy-label">DropKey</span>
                    <div class="operator-deploy-api-key-row">
                        <div class="operator-deploy-api-key-value obfuscated" id="deploy-api-key-value"></div>
                        <button class="operator-deploy-icon-btn" id="deploy-api-key-toggle" type="button" title="Show/Hide">
                            <span class="material-symbols-outlined">visibility</span>
                        </button>
                        <button class="operator-deploy-icon-btn" id="deploy-api-key-copy" type="button" title="Copy">
                            <span class="material-symbols-outlined">content_copy</span>
                        </button>
                    </div>
                </div>
                <div class="operator-deploy-row">
                    <span class="operator-deploy-label">Device Link</span>
                    <div class="operator-device-link-generate-row">
                        <div class="operator-device-link-param">
                            <label class="operator-device-link-param-label" for="device-link-cmd-count">Count</label>
                            <div class="operator-counter">
                                <button class="operator-counter-btn" id="device-link-cmd-count-dec" type="button">-</button>
                                <input class="operator-counter-input" id="device-link-cmd-count" type="number" min="1" max="10000" value="1">
                                <button class="operator-counter-btn" id="device-link-cmd-count-inc" type="button">+</button>
                            </div>
                        </div>
                        <div class="operator-device-link-param">
                            <label class="operator-device-link-param-label" for="device-link-cmd-ttl">TTL (hours)</label>
                            <div class="operator-counter">
                                <button class="operator-counter-btn" id="device-link-cmd-ttl-dec" type="button">-</button>
                                <input class="operator-counter-input" id="device-link-cmd-ttl" type="number" min="1" max="8760" value="24">
                                <button class="operator-counter-btn" id="device-link-cmd-ttl-inc" type="button">+</button>
                            </div>
                        </div>
                        <button class="operator-device-link-generate-btn" id="device-link-generate-btn" type="button">
                            <span class="material-symbols-outlined">add_link</span>
                            Generate
                        </button>
                    </div>
                    <div class="operator-device-link-result initially-hidden" id="device-link-result">
                        <span class="operator-deploy-sublabel">Curl Command</span>
                        <div class="operator-deploy-cmd-row">
                            <div class="operator-deploy-cmd" id="device-link-curl-cmd"></div>
                            <button class="operator-deploy-icon-btn" id="device-link-copy-curl" type="button" title="Copy">
                                <span class="material-symbols-outlined">content_copy</span>
                            </button>
                        </div>
                        <span class="operator-deploy-sublabel">Device Link Token</span>
                        <div class="operator-deploy-cmd-row">
                            <div class="operator-deploy-cmd" id="device-link-token"></div>
                            <button class="operator-deploy-icon-btn" id="device-link-copy-token" type="button" title="Copy">
                                <span class="material-symbols-outlined">content_copy</span>
                            </button>
                        </div>
                    </div>
                    <div class="operator-device-link-error initially-hidden" id="device-link-generate-error"></div>
                </div>
            </div>
        </div>
    `;
    return container;
}

function createMixinContext(overrides = {}) {
    const ctx = Object.create(null);
    Object.assign(ctx, OperatorDownloadMixin);
    ctx.copyCurlCommand = vi.fn();
    ctx.handleOperatorDownload = vi.fn();
    ctx.collapseDownloadSection = vi.fn();
    Object.assign(ctx, overrides);
    return ctx;
}

beforeEach(async () => {
    vi.resetModules();

    vi.doMock('@g8ed/public/js/utils/web-session-service.js', () => ({
        webSessionService: {
            getApiKey: vi.fn(() => TEST_API_KEY),
        },
    }));

    vi.doMock('@g8ed/public/js/utils/operator-panel-service.js', () => ({
        operatorPanelService: {
            createDeviceLink: vi.fn(),
        },
    }));

    vi.doMock('@g8ed/public/js/utils/dev-logger.js', () => ({
        devLogger: { log: vi.fn(), error: vi.fn(), warn: vi.fn() },
    }));

    vi.doMock('@g8ed/public/js/utils/template-loader.js', () => ({
        templateLoader: { load: vi.fn(), render: vi.fn() },
    }));

    vi.doMock('@g8ed/public/js/constants/service-client-constants.js', () => ({
        BEARER_PREFIX: 'Bearer ',
    }));

    const mod = await import('@g8ed/public/js/components/operator-download-mixin.js');
    OperatorDownloadMixin = mod.OperatorDownloadMixin;

    const opsMod = await import('@g8ed/public/js/utils/operator-panel-service.js');
    operatorPanelService = opsMod.operatorPanelService;
});

afterEach(() => {
    vi.restoreAllMocks();
});

describe('OperatorDownloadMixin [UNIT - jsdom]', () => {

    describe('_obfuscateApiKey', () => {
        it('returns placeholder dots for null api key', () => {
            const ctx = createMixinContext();
            expect(ctx._obfuscateApiKey(null)).toBe('••••••••••••••••');
        });

        it('returns placeholder dots for empty string', () => {
            const ctx = createMixinContext();
            expect(ctx._obfuscateApiKey('')).toBe('••••••••••••••••');
        });

        it('returns placeholder dots for short keys (< 20 chars)', () => {
            const ctx = createMixinContext();
            expect(ctx._obfuscateApiKey('short_key_123')).toBe('••••••••••••••••');
        });

        it('shows first 12 and last 4 characters with dots in between for valid keys', () => {
            const ctx = createMixinContext();
            const result = ctx._obfuscateApiKey(TEST_API_KEY);
            expect(result).toBe(TEST_API_KEY.substring(0, 12) + '••••••••••••' + TEST_API_KEY.substring(TEST_API_KEY.length - 4));
        });

        it('preserves the exact prefix and suffix of the key', () => {
            const ctx = createMixinContext();
            const result = ctx._obfuscateApiKey(TEST_API_KEY);
            expect(result.startsWith('dak_abcdefgh')).toBe(true);
            expect(result.endsWith('7890')).toBe(true);
        });
    });

    describe('_bindDeployApiKey', () => {
        it('populates the api key element with obfuscated text', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._bindDeployApiKey(container, TEST_API_KEY);

            const apiKeyEl = container.querySelector('#deploy-api-key-value');
            expect(apiKeyEl.textContent).toBe(ctx._obfuscateApiKey(TEST_API_KEY));
        });

        it('stores the raw api key in dataset', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._bindDeployApiKey(container, TEST_API_KEY);

            const apiKeyEl = container.querySelector('#deploy-api-key-value');
            expect(apiKeyEl.dataset.apiKey).toBe(TEST_API_KEY);
        });

        it('starts with obfuscated class', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._bindDeployApiKey(container, TEST_API_KEY);

            const apiKeyEl = container.querySelector('#deploy-api-key-value');
            expect(apiKeyEl.classList.contains('obfuscated')).toBe(true);
        });

        it('toggles visibility on toggle button click', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._bindDeployApiKey(container, TEST_API_KEY);

            const apiKeyEl = container.querySelector('#deploy-api-key-value');
            const toggleBtn = container.querySelector('#deploy-api-key-toggle');

            toggleBtn.click();
            expect(apiKeyEl.classList.contains('obfuscated')).toBe(false);
            expect(apiKeyEl.textContent).toBe(TEST_API_KEY);

            const icon = toggleBtn.querySelector('.material-symbols-outlined');
            expect(icon.textContent).toBe('visibility_off');
        });

        it('re-obfuscates on second toggle click', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._bindDeployApiKey(container, TEST_API_KEY);

            const apiKeyEl = container.querySelector('#deploy-api-key-value');
            const toggleBtn = container.querySelector('#deploy-api-key-toggle');

            toggleBtn.click();
            toggleBtn.click();

            expect(apiKeyEl.classList.contains('obfuscated')).toBe(true);
            expect(apiKeyEl.textContent).toBe(ctx._obfuscateApiKey(TEST_API_KEY));
        });

        it('copies the raw api key on copy button click', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._bindDeployApiKey(container, TEST_API_KEY);

            const copyBtn = container.querySelector('#deploy-api-key-copy');
            copyBtn.click();

            expect(ctx.copyCurlCommand).toHaveBeenCalledWith(TEST_API_KEY, copyBtn);
        });

        it('does nothing if api key element is missing', () => {
            const ctx = createMixinContext();
            const container = document.createElement('div');
            ctx._bindDeployApiKey(container, TEST_API_KEY);
            expect(ctx.copyCurlCommand).not.toHaveBeenCalled();
        });
    });

    describe('_bindDeviceLinkGeneration', () => {
        it('binds counter increment and decrement for count input', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._bindDeviceLinkGeneration(container, TEST_API_KEY);

            const countInput = container.querySelector('#device-link-cmd-count');
            const incBtn = container.querySelector('#device-link-cmd-count-inc');
            const decBtn = container.querySelector('#device-link-cmd-count-dec');

            expect(countInput.value).toBe('1');

            incBtn.click();
            expect(countInput.value).toBe('2');

            decBtn.click();
            expect(countInput.value).toBe('1');
        });

        it('does not decrement count below 1', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._bindDeviceLinkGeneration(container, TEST_API_KEY);

            const countInput = container.querySelector('#device-link-cmd-count');
            const decBtn = container.querySelector('#device-link-cmd-count-dec');

            decBtn.click();
            expect(countInput.value).toBe('1');
        });

        it('does not increment count above 10000', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._bindDeviceLinkGeneration(container, TEST_API_KEY);

            const countInput = container.querySelector('#device-link-cmd-count');
            countInput.value = '10000';
            const incBtn = container.querySelector('#device-link-cmd-count-inc');

            incBtn.click();
            expect(countInput.value).toBe('10000');
        });

        it('binds counter increment and decrement for TTL input', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._bindDeviceLinkGeneration(container, TEST_API_KEY);

            const ttlInput = container.querySelector('#device-link-cmd-ttl');
            const incBtn = container.querySelector('#device-link-cmd-ttl-inc');
            const decBtn = container.querySelector('#device-link-cmd-ttl-dec');

            expect(ttlInput.value).toBe('24');

            incBtn.click();
            expect(ttlInput.value).toBe('25');

            decBtn.click();
            expect(ttlInput.value).toBe('24');
        });

        it('populates curl command and token on successful generate', async () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            const testToken = 'dl_test_token_abc123';
            operatorPanelService.createDeviceLink.mockResolvedValue({
                ok: true,
                json: async () => ({ success: true, token: testToken }),
            });

            ctx._bindDeviceLinkGeneration(container, TEST_API_KEY);

            const generateBtn = container.querySelector('#device-link-generate-btn');
            generateBtn.click();

            await vi.waitFor(() => {
                const curlCmd = container.querySelector('#device-link-curl-cmd');
                expect(curlCmd.textContent).toContain('curl -fsSL');
                expect(curlCmd.textContent).toContain(testToken);
                expect(curlCmd.textContent).toContain('sh -s --');
            });

            const tokenDiv = container.querySelector('#device-link-token');
            expect(tokenDiv.textContent).toBe(testToken);

            const resultDiv = container.querySelector('#device-link-result');
            expect(resultDiv.classList.contains('initially-hidden')).toBe(false);
        });

        it('passes count and TTL to createDeviceLink', async () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            operatorPanelService.createDeviceLink.mockResolvedValue({
                ok: true,
                json: async () => ({ success: true, token: 'tok' }),
            });

            const countInput = container.querySelector('#device-link-cmd-count');
            const ttlInput = container.querySelector('#device-link-cmd-ttl');
            countInput.value = '5';
            ttlInput.value = '48';

            ctx._bindDeviceLinkGeneration(container, TEST_API_KEY);

            const generateBtn = container.querySelector('#device-link-generate-btn');
            generateBtn.click();

            await vi.waitFor(() => {
                expect(operatorPanelService.createDeviceLink).toHaveBeenCalledWith({
                    maxUses: 5,
                    expiresInHours: 48,
                });
            });
        });

        it('shows error message on failed generate', async () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            operatorPanelService.createDeviceLink.mockResolvedValue({
                ok: false,
                json: async () => ({ success: false, error: 'Rate limit exceeded' }),
            });

            ctx._bindDeviceLinkGeneration(container, TEST_API_KEY);

            const generateBtn = container.querySelector('#device-link-generate-btn');
            generateBtn.click();

            await vi.waitFor(() => {
                const errorDiv = container.querySelector('#device-link-generate-error');
                expect(errorDiv.textContent).toBe('Rate limit exceeded');
                expect(errorDiv.classList.contains('initially-hidden')).toBe(false);
            });

            const resultDiv = container.querySelector('#device-link-result');
            expect(resultDiv.classList.contains('initially-hidden')).toBe(true);
        });

        it('shows error on network failure', async () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            operatorPanelService.createDeviceLink.mockRejectedValue(new Error('Network error'));

            ctx._bindDeviceLinkGeneration(container, TEST_API_KEY);

            const generateBtn = container.querySelector('#device-link-generate-btn');
            generateBtn.click();

            await vi.waitFor(() => {
                const errorDiv = container.querySelector('#device-link-generate-error');
                expect(errorDiv.textContent).toBe('Network error');
                expect(errorDiv.classList.contains('initially-hidden')).toBe(false);
            });
        });

        it('re-enables the generate button after completion', async () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            operatorPanelService.createDeviceLink.mockResolvedValue({
                ok: true,
                json: async () => ({ success: true, token: 'tok' }),
            });

            ctx._bindDeviceLinkGeneration(container, TEST_API_KEY);

            const generateBtn = container.querySelector('#device-link-generate-btn');
            generateBtn.click();

            await vi.waitFor(() => {
                expect(generateBtn.disabled).toBe(false);
                expect(generateBtn.innerHTML).toContain('Generate');
                expect(generateBtn.innerHTML).toContain('add_link');
            });
        });

        it('binds copy handlers on the curl and token copy buttons after generate', async () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            const testToken = 'dl_copy_test_token';
            operatorPanelService.createDeviceLink.mockResolvedValue({
                ok: true,
                json: async () => ({ success: true, token: testToken }),
            });

            ctx._bindDeviceLinkGeneration(container, TEST_API_KEY);

            const generateBtn = container.querySelector('#device-link-generate-btn');
            generateBtn.click();

            await vi.waitFor(() => {
                expect(container.querySelector('#device-link-curl-cmd').textContent).toContain(testToken);
            });

            const copyCurlBtn = container.querySelector('#device-link-copy-curl');
            copyCurlBtn.click();
            expect(ctx.copyCurlCommand).toHaveBeenCalledWith(
                expect.stringContaining('curl -fsSL'),
                copyCurlBtn
            );

            const copyTokenBtn = container.querySelector('#device-link-copy-token');
            copyTokenBtn.click();
            expect(ctx.copyCurlCommand).toHaveBeenCalledWith(testToken, copyTokenBtn);
        });

        it('clears previous error on new generate attempt', async () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            operatorPanelService.createDeviceLink
                .mockResolvedValueOnce({
                    ok: false,
                    json: async () => ({ success: false, error: 'First error' }),
                })
                .mockResolvedValueOnce({
                    ok: true,
                    json: async () => ({ success: true, token: 'tok' }),
                });

            ctx._bindDeviceLinkGeneration(container, TEST_API_KEY);

            const generateBtn = container.querySelector('#device-link-generate-btn');
            generateBtn.click();

            await vi.waitFor(() => {
                const errorDiv = container.querySelector('#device-link-generate-error');
                expect(errorDiv.textContent).toBe('First error');
            });

            generateBtn.click();

            await vi.waitFor(() => {
                const errorDiv = container.querySelector('#device-link-generate-error');
                expect(errorDiv.classList.contains('initially-hidden')).toBe(true);
                const resultDiv = container.querySelector('#device-link-result');
                expect(resultDiv.classList.contains('initially-hidden')).toBe(false);
            });
        });
    });

    describe('_populateBinaryDownloadLinks', () => {
        it('attaches click handlers to all three binary links', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._populateBinaryDownloadLinks(container);

            const links = container.querySelectorAll('.operator-download-link');
            expect(links).toHaveLength(3);

            links[0].click();
            expect(ctx.handleOperatorDownload).toHaveBeenCalledWith('linux/amd64', TEST_API_KEY);
            expect(ctx.collapseDownloadSection).toHaveBeenCalled();
        });

        it('calls _bindDeployApiKey and _bindDeviceLinkGeneration', () => {
            const ctx = createMixinContext();
            const bindApiKeySpy = vi.spyOn(ctx, '_bindDeployApiKey');
            const bindDeviceLinkSpy = vi.spyOn(ctx, '_bindDeviceLinkGeneration');
            const container = buildMockContainer();

            ctx._populateBinaryDownloadLinks(container);

            expect(bindApiKeySpy).toHaveBeenCalledWith(container, TEST_API_KEY);
            expect(bindDeviceLinkSpy).toHaveBeenCalledWith(container, TEST_API_KEY);
        });
    });
});
