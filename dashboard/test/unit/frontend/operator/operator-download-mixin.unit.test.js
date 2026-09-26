// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

let OperatorDownloadMixin;

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

        it('calls _bindDeployApiKey', () => {
            const ctx = createMixinContext();
            const bindApiKeySpy = vi.spyOn(ctx, '_bindDeployApiKey');
            const container = buildMockContainer();

            ctx._populateBinaryDownloadLinks(container);

            expect(bindApiKeySpy).toHaveBeenCalledWith(container, TEST_API_KEY);
        });
    });
});
