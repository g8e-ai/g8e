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

let OperatorDownloadMixin;
let operatorPanelService;
let webSessionService;
let notificationService;
let bindCounter;
let obfuscateApiKey;
let copyToClipboardWithFeedback;

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
                    <span class="operator-deploy-label">G8eKey</span>
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
                            <div class="operator-deploy-cmd obfuscated" id="device-link-token"></div>
                            <button class="operator-deploy-icon-btn" id="device-link-token-toggle" type="button" title="Show/Hide">
                                <span class="material-symbols-outlined">visibility</span>
                            </button>
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
    
    // Don't mock methods by default, let the test decide what to mock
    Object.assign(ctx, overrides);
    return ctx;
}

beforeEach(async () => {
    vi.resetModules();

    // Mock global functions
    global.fetch = vi.fn();
    window.alert = vi.fn();
    global.URL.createObjectURL = vi.fn(() => 'blob:url');
    global.URL.revokeObjectURL = vi.fn();

    vi.doMock('@g8ed/public/js/utils/web-session-service.js', () => ({
        webSessionService: {
            getApiKey: vi.fn(() => TEST_API_KEY),
            setApiKey: vi.fn(),
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
        templateLoader: { 
            load: vi.fn(), 
            render: vi.fn(),
            cache: {
                get: vi.fn((name) => {
                    if (name === 'operator-platform-selection') {
                        return '<div class="download-menu-back">Back</div><div class="platform-options">{{optionsHtml}}</div>';
                    }
                    if (name === 'operator-download-layer') {
                        return '<div class="download-menu-back">Back</div><div class="curl-copy-btn">Copy</div><div class="api-key-text">key</div><div class="api-key-toggle-btn"><span class="toggle-text">Show</span><span class="toggle-icon"></span></div><div class="api-key-copy-btn">Copy</div><div class="download-direct-btn">Download</div>';
                    }
                    return `Template for ${name}`;
                })
            },
            replace: vi.fn((template, data) => {
                let result = template;
                for (const [key, value] of Object.entries(data)) {
                    result = result.replace(`{{${key}}}`, value);
                }
                return result;
            })
        },
    }));

    vi.doMock('@g8ed/public/js/constants/service-client-constants.js', () => ({
        BEARER_PREFIX: 'Bearer ',
    }));

    vi.doMock('@g8ed/public/js/utils/notification-service.js', () => ({
        notificationService: {
            error: vi.fn(),
            info: vi.fn(),
            success: vi.fn(),
            warning: vi.fn(),
        },
    }));

    vi.doMock('@g8ed/public/js/utils/ui-utils.js', () => ({
        bindCounter: vi.fn(),
        obfuscateApiKey: vi.fn((key) => {
            if (!key || key.length < 20) return '••••••••••••••••';
            return key.substring(0, 12) + '••••••••••••' + key.substring(key.length - 4);
        }),
        copyToClipboardWithFeedback: vi.fn(),
    }));

    const mod = await import('@g8ed/public/js/components/operator-download-mixin.js');
    OperatorDownloadMixin = mod.OperatorDownloadMixin;

    const opsMod = await import('@g8ed/public/js/utils/operator-panel-service.js');
    operatorPanelService = opsMod.operatorPanelService;

    const wssMod = await import('@g8ed/public/js/utils/web-session-service.js');
    webSessionService = wssMod.webSessionService;

    const nsMod = await import('@g8ed/public/js/utils/notification-service.js');
    notificationService = nsMod.notificationService;

    const uiUtilsMod = await import('@g8ed/public/js/utils/ui-utils.js');
    ({ bindCounter, obfuscateApiKey, copyToClipboardWithFeedback } = uiUtilsMod);

    // Make bindCounter actually work in tests
    bindCounter.mockImplementation((decId, incId, input, min, max, container) => {
        const decBtn = container.querySelector(decId);
        const incBtn = container.querySelector(incId);
        
        if (decBtn) {
            decBtn.addEventListener('click', () => {
                const val = parseInt(input.value, 10) || min;
                input.value = Math.max(min, val - 1);
            });
        }
        
        if (incBtn) {
            incBtn.addEventListener('click', () => {
                const val = parseInt(input.value, 10) || min;
                input.value = Math.min(max, val + 1);
            });
        }
    });

    // Reset global mocks for each test
    global.fetch.mockClear();
    window.alert.mockClear();
    global.URL.createObjectURL.mockClear();
    global.URL.revokeObjectURL.mockClear();

    if (!global.navigator.clipboard) {
        global.navigator.clipboard = {
            writeText: vi.fn().mockResolvedValue(undefined)
        };
    }
});

afterEach(() => {
    vi.restoreAllMocks();
});

describe('OperatorDownloadMixin [UNIT - jsdom]', () => {

    describe('_bindDeployApiKey', () => {
        it('populates the api key element with obfuscated text', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._bindDeployApiKey(container, TEST_API_KEY);

            const apiKeyEl = container.querySelector('#deploy-api-key-value');
            expect(obfuscateApiKey).toHaveBeenCalledWith(TEST_API_KEY);
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
            expect(obfuscateApiKey).toHaveBeenCalledWith(TEST_API_KEY);
        });

        it('copies the raw api key on copy button click', () => {
            const ctx = createMixinContext();
            const container = buildMockContainer();

            ctx._bindDeployApiKey(container, TEST_API_KEY);

            const copyBtn = container.querySelector('#deploy-api-key-copy');
            copyBtn.click();

            expect(copyToClipboardWithFeedback).toHaveBeenCalledWith(
                TEST_API_KEY,
                copyBtn,
                expect.any(Function),
                expect.any(Function)
            );
        });

        it('does nothing if api key element is missing', () => {
            const ctx = createMixinContext();
            const container = document.createElement('div');
            ctx._bindDeployApiKey(container, TEST_API_KEY);
            expect(copyToClipboardWithFeedback).not.toHaveBeenCalled();
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
            expect(obfuscateApiKey).toHaveBeenCalledWith(testToken);
            expect(tokenDiv.dataset.token).toBe(testToken);

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
            expect(copyToClipboardWithFeedback).toHaveBeenCalledWith(
                expect.stringContaining('curl -fsSL'),
                copyCurlBtn,
                expect.any(Function),
                expect.any(Function)
            );

            const copyTokenBtn = container.querySelector('#device-link-copy-token');
            copyTokenBtn.click();
            expect(copyToClipboardWithFeedback).toHaveBeenCalledWith(
                testToken,
                copyTokenBtn,
                expect.any(Function),
                expect.any(Function)
            );
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

        it('starts with obfuscated class on device link token', async () => {
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
                const tokenDiv = container.querySelector('#device-link-token');
                expect(tokenDiv.classList.contains('obfuscated')).toBe(true);
            });
        });

        it('toggles visibility on device link token toggle button click', async () => {
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
                const tokenDiv = container.querySelector('#device-link-token');
                expect(tokenDiv.dataset.token).toBe(testToken);
            });

            const toggleBtn = container.querySelector('#device-link-token-toggle');
            const tokenDiv = container.querySelector('#device-link-token');

            toggleBtn.click();
            expect(tokenDiv.classList.contains('obfuscated')).toBe(false);
            expect(tokenDiv.textContent).toBe(testToken);

            const icon = toggleBtn.querySelector('.material-symbols-outlined');
            expect(icon.textContent).toBe('visibility_off');
        });

        it('re-obfuscates on second toggle click for device link token', async () => {
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
                const tokenDiv = container.querySelector('#device-link-token');
                expect(tokenDiv.dataset.token).toBe(testToken);
            });

            const toggleBtn = container.querySelector('#device-link-token-toggle');
            const tokenDiv = container.querySelector('#device-link-token');

            toggleBtn.click();
            toggleBtn.click();

            expect(tokenDiv.classList.contains('obfuscated')).toBe(true);
            expect(obfuscateApiKey).toHaveBeenCalledWith(testToken);
        });
    });

    describe('_populateBinaryDownloadLinks', () => {
        it('attaches click handlers to all three binary links', () => {
            const ctx = createMixinContext({
                handleOperatorDownload: vi.fn(),
                collapseDownloadSection: vi.fn()
            });
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

    describe('State Management', () => {
        it('toggleDownloadSection expands if collapsed', () => {
            const ctx = createMixinContext();
            ctx.downloadSectionExpanded = false;
            ctx.expandDownloadSection = vi.fn();
            ctx.toggleDownloadSection();
            expect(ctx.expandDownloadSection).toHaveBeenCalled();
        });

        it('toggleDownloadSection collapses if expanded', () => {
            const ctx = createMixinContext();
            ctx.downloadSectionExpanded = true;
            ctx.collapseDownloadSection = vi.fn();
            ctx.toggleDownloadSection();
            expect(ctx.collapseDownloadSection).toHaveBeenCalled();
        });

        it('expandDownloadSection populates and adds expanded class', () => {
            const collapsible = document.createElement('div');
            const ctx = createMixinContext({
                downloadCollapsible: collapsible,
                downloadSectionPopulated: false
            });
            ctx.populateDownloadSection = vi.fn();
            ctx.expandDownloadSection();
            expect(ctx.populateDownloadSection).toHaveBeenCalled();
            expect(collapsible.classList.contains('expanded')).toBe(true);
            expect(ctx.downloadSectionExpanded).toBe(true);
        });

        it('collapseDownloadSection removes expanded class', () => {
            const collapsible = document.createElement('div');
            collapsible.classList.add('expanded');
            const ctx = createMixinContext({
                downloadCollapsible: collapsible
            });
            ctx.collapseDownloadSection();
            expect(collapsible.classList.contains('expanded')).toBe(false);
            expect(ctx.downloadSectionExpanded).toBe(false);
        });
    });

    describe('populateDownloadSection', () => {
        it('renders template and populates links', () => {
            const content = document.createElement('div');
            const ctx = createMixinContext({
                downloadCollapsibleContent: content,
                _populateBinaryDownloadLinks: vi.fn()
            });
            
            const templateLoaderMod = vi.mocked(import.meta.glob('@g8ed/public/js/utils/template-loader.js')['template-loader.js']);
            
            ctx.populateDownloadSection();
            
            expect(content.innerHTML).toContain('Template for operator-initial-download-overlay');
            expect(ctx._populateBinaryDownloadLinks).toHaveBeenCalledWith(content);
            expect(ctx.downloadSectionPopulated).toBe(true);
        });
    });

    describe('handleRefreshG8eKey', () => {
        it('refreshes the key successfully', async () => {
            const button = document.createElement('button');
            button.innerHTML = 'refresh';
            const apiKeyValue = document.createElement('div');
            const ctx = createMixinContext();
            
            const newKey = 'dak_new_key_1234567890abcdef';
            global.fetch.mockResolvedValue({
                ok: true,
                json: async () => ({ success: true, g8e_key: newKey })
            });

            await ctx.handleRefreshG8eKey(button, apiKeyValue);

            expect(apiKeyValue.dataset.apiKey).toBe(newKey);
            expect(webSessionService.setApiKey).toHaveBeenCalledWith(newKey);
            expect(button.disabled).toBe(false);
            expect(button.innerHTML).toBe('refresh');
        });

        it('shows alert on error', async () => {
            const button = document.createElement('button');
            const apiKeyValue = document.createElement('div');
            const ctx = createMixinContext();
            
            global.fetch.mockResolvedValue({
                ok: false,
                json: async () => ({ success: false, error: 'Server error' })
            });

            await ctx.handleRefreshG8eKey(button, apiKeyValue);

            expect(notificationService.error).toHaveBeenCalledWith(expect.stringContaining('Server error'));
            expect(button.disabled).toBe(false);
        });
    });

    describe('handleOperatorDownload', () => {
        it('shows notificationService warning if apiKey is missing', async () => {
            const ctx = createMixinContext();
            await ctx.handleOperatorDownload('linux/amd64', null);
            expect(notificationService.warning).toHaveBeenCalledWith(expect.stringContaining('No API key found'));
        });

        it('shows notificationService error if fetch fails', async () => {
            const ctx = createMixinContext();
            global.fetch.mockResolvedValue({ ok: false, status: 500 });
            await ctx.handleOperatorDownload('linux/amd64', TEST_API_KEY);
            expect(notificationService.error).toHaveBeenCalledWith(expect.stringContaining('status: 500'));
        });

        it('skips anchor click in test environment', async () => {
            const ctx = createMixinContext();
            ctx._setTestEnvironment(true);
            
            global.fetch.mockResolvedValue({
                ok: true,
                blob: async () => new Blob(['data'], { type: 'application/octet-stream' })
            });

            await ctx.handleOperatorDownload('linux/amd64', TEST_API_KEY);

            expect(global.fetch).toHaveBeenCalledWith('/operator/download/linux/amd64', expect.objectContaining({
                method: 'GET',
                headers: { 'Authorization': `Bearer ${TEST_API_KEY}` }
            }));
            expect(global.URL.createObjectURL).not.toHaveBeenCalled();
            expect(global.URL.revokeObjectURL).not.toHaveBeenCalled();
        });
    });

    describe('populateDownloadDetails', () => {
        it('sets up all UI elements correctly', () => {
            const ctx = createMixinContext();
            const downloadSpy = vi.spyOn(ctx, 'handleOperatorDownload');
            copyToClipboardWithFeedback.mockClear();
            ctx.collapseDownloadSection = vi.fn();
            ctx.updatePlatformIcons = vi.fn();
            
            const overlay = document.createElement('div');
            overlay.innerHTML = `
                <div id="secure-download-command"></div>
                <div id="verify-checksum-command"></div>
                <div id="curl-command"></div>
                <input type="checkbox" id="curl-sudo-checkbox">
                <div id="api-key-display"></div>
                <div id="direct-platform-icon"></div>
                <div id="direct-platform-name"></div>
                <div id="direct-platform-file"></div>
                <div id="download-final-text"></div>
                <div class="download-method-tab" data-method="direct"></div>
                <div class="download-method-tab" data-method="secure"></div>
                <div class="download-method-panel" data-method="direct"></div>
                <div class="download-method-panel" data-method="secure"></div>
                <div id="secure-api-key-display"></div>
                <button id="secure-api-key-toggle"><span class="material-symbols-outlined">visibility</span></button>
                <button id="secure-api-key-copy"></button>
                <button id="secure-env-copy"></button>
                <button id="secure-download-copy"></button>
                <button id="verify-checksum-copy"></button>
                <button id="secure-run-copy"></button>
                <button id="curl-env-copy"></button>
                <button id="curl-copy"></button>
                <button id="api-key-copy"></button>
                <button id="api-key-toggle"><span class="material-symbols-outlined">visibility</span></button>
                <button id="download-final-btn"></button>
            `;

            ctx.populateDownloadDetails(overlay, 'linux', 'amd64');

            expect(overlay.querySelector('#direct-platform-name').textContent).toBe('Linux x64');
            expect(overlay.querySelector('#direct-platform-file').textContent).toBe('g8e.operator');
            
            // Check copy buttons
            overlay.querySelector('#secure-api-key-copy').click();
            expect(copyToClipboardWithFeedback).toHaveBeenCalledWith(
                TEST_API_KEY,
                expect.any(HTMLElement),
                expect.any(Function),
                expect.any(Function)
            );

            // Check final download button
            overlay.querySelector('#download-final-btn').click();
            expect(downloadSpy).toHaveBeenCalledWith('linux/amd64', TEST_API_KEY);
            expect(ctx.collapseDownloadSection).toHaveBeenCalled();
        });
    });

    describe('Overlay Management', () => {
        it('showPlatformSelection creates and adds overlay', () => {
            const drawer = document.createElement('div');
            drawer.className = 'operator-drawer-content';
            document.body.appendChild(drawer);

            const ctx = createMixinContext({
                platformOptions: { linux: [{ arch: 'amd64', label: 'x64' }] },
                downloadMenuStack: []
            });

            ctx.showPlatformSelection('linux');

            const overlay = drawer.querySelector('.download-menu-overlay');
            expect(overlay).not.toBeNull();
            expect(overlay.dataset.layer).toBe('platform-selection');
            expect(ctx.downloadMenuStack).toHaveLength(1);

            document.body.removeChild(drawer);
        });

        it('closeCurrentOverlay removes top overlay', () => {
            vi.useFakeTimers();
            const overlay = document.createElement('div');
            overlay.classList.add('active');
            const ctx = createMixinContext({
                downloadMenuStack: [overlay]
            });

            ctx.closeCurrentOverlay();
            expect(overlay.classList.contains('active')).toBe(false);
            
            vi.advanceTimersByTime(300);
            expect(ctx.downloadMenuStack).toHaveLength(0);
            vi.useRealTimers();
        });

        it('closeAllOverlays clears stack', () => {
            vi.useFakeTimers();
            const o1 = document.createElement('div');
            const o2 = document.createElement('div');
            const ctx = createMixinContext({
                downloadMenuStack: [o1, o2]
            });

            ctx.closeAllOverlays();
            expect(ctx.downloadMenuStack).toHaveLength(0);
            expect(ctx.currentOS).toBeNull();
            vi.useRealTimers();
        });
    });

    describe('_isTestEnvironment', () => {
        it('returns true when explicitly set via _setTestEnvironment', () => {
            const ctx = createMixinContext();
            ctx._setTestEnvironment(true);
            expect(ctx._isTestEnvironment()).toBe(true);
        });

        it('returns false when explicitly set via _setTestEnvironment', () => {
            const ctx = createMixinContext();
            ctx._setTestEnvironment(false);
            expect(ctx._isTestEnvironment()).toBe(false);
        });

        it('falls back to window.__vitest__ detection when not explicitly set', () => {
            const ctx = createMixinContext();
            window.__vitest__ = true;
            expect(ctx._isTestEnvironment()).toBe(true);
            window.__vitest__ = undefined;
        });

        it('falls back to jsdom userAgent detection when not explicitly set', () => {
            const ctx = createMixinContext();
            window.__vitest__ = undefined;
            Object.defineProperty(navigator, 'userAgent', {
                value: 'jsdom/16.4.0',
                configurable: true
            });
            expect(ctx._isTestEnvironment()).toBe(true);
            Object.defineProperty(navigator, 'userAgent', {
                value: 'Mozilla/5.0',
                configurable: true
            });
        });

        it('returns false when not in test environment and not explicitly set', () => {
            const ctx = createMixinContext();
            window.__vitest__ = undefined;
            Object.defineProperty(navigator, 'userAgent', {
                value: 'Mozilla/5.0',
                configurable: true
            });
            expect(ctx._isTestEnvironment()).toBe(false);
        });
    });
});
