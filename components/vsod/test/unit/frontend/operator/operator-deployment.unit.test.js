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

let OperatorDeployment;
let templateLoader;

const TEST_TEMPLATE = `<div class="opdeploy">
    <div class="opdeploy__header">
        <span class="material-symbols-outlined opdeploy__header-icon">rocket_launch</span>
        <span class="opdeploy__header-text">Getting Started</span>
    </div>
    <div class="opdeploy__steps">
        <div class="opdeploy__step">
            <div class="opdeploy__step-number">
                <span class="opdeploy__step-badge">1</span>
            </div>
            <div class="opdeploy__step-content">
                <div class="opdeploy__step-title">Bind an Operator</div>
            </div>
        </div>
    </div>
    <div class="opdeploy__section">
        <div class="opdeploy__section-header">
            <span class="material-symbols-outlined opdeploy__section-icon">key</span>
            <span class="opdeploy__section-title">Operator API Keys</span>
        </div>
        <div class="opdeploy__section-desc">Each operator has a unique API key assigned at creation.</div>
    </div>
    <div class="opdeploy__footer">
        <span class="material-symbols-outlined opdeploy__footer-icon">info</span>
        <span class="opdeploy__footer-text">Your g8e-pod operator is already authenticated.</span>
    </div>
</div>`;

function buildMockContainer() {
    const container = document.createElement('div');
    return container;
}

beforeEach(async () => {
    vi.resetModules();

    vi.doMock('@vsod/public/js/utils/template-loader.js', () => ({
        templateLoader: {
            load: vi.fn(() => Promise.resolve(TEST_TEMPLATE)),
        },
    }));

    const mod = await import('@vsod/public/js/components/operator-deployment.js');
    OperatorDeployment = mod.OperatorDeployment;

    const tlMod = await import('@vsod/public/js/utils/template-loader.js');
    templateLoader = tlMod.templateLoader;
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
});

describe('OperatorDeployment [UNIT - jsdom]', () => {
    describe('constructor', () => {
        it('initializes with default values when no opts provided', () => {
            const deployment = new OperatorDeployment();
            expect(deployment.onClose).toBeNull();
            expect(deployment._container).toBeNull();
        });

        it('initializes with onClose callback when provided', () => {
            const onCloseCallback = vi.fn();
            const deployment = new OperatorDeployment({ onClose: onCloseCallback });
            expect(deployment.onClose).toBe(onCloseCallback);
            expect(deployment._container).toBeNull();
        });

        it('initializes container as null regardless of opts', () => {
            const deployment = new OperatorDeployment({ onClose: vi.fn(), other: 'value' });
            expect(deployment._container).toBeNull();
        });
    });

    describe('mount', () => {
        it('clears container before mounting', async () => {
            const container = buildMockContainer();
            container.innerHTML = '<div>old content</div>';

            const deployment = new OperatorDeployment();
            await deployment.mount(container);

            expect(container.innerHTML).not.toContain('old content');
        });

        it('loads operator-deployment template', async () => {
            const container = buildMockContainer();

            const deployment = new OperatorDeployment();
            await deployment.mount(container);

            expect(templateLoader.load).toHaveBeenCalledWith('operator-deployment');
        });

        it('appends template content to container', async () => {
            const container = buildMockContainer();

            const deployment = new OperatorDeployment();
            await deployment.mount(container);

            expect(container.querySelector('.opdeploy')).not.toBeNull();
            expect(container.querySelector('.opdeploy__header-text').textContent).toBe('Getting Started');
        });

        it('stores container reference', async () => {
            const container = buildMockContainer();

            const deployment = new OperatorDeployment();
            await deployment.mount(container);

            expect(deployment._container).toBe(container);
        });
    });

    describe('destroy', () => {
        it('sets container to null', async () => {
            const container = buildMockContainer();

            const deployment = new OperatorDeployment();
            await deployment.mount(container);

            deployment.destroy();

            expect(deployment._container).toBeNull();
        });

        it('does not throw when container is null', async () => {
            const deployment = new OperatorDeployment();
            deployment._container = null;

            expect(() => deployment.destroy()).not.toThrow();
        });

        it('can be called multiple times safely', async () => {
            const container = buildMockContainer();

            const deployment = new OperatorDeployment();
            await deployment.mount(container);

            expect(() => {
                deployment.destroy();
                deployment.destroy();
                deployment.destroy();
            }).not.toThrow();
        });
    });

    describe('setUser', () => {
        it('is a no-op method', () => {
            const deployment = new OperatorDeployment();
            expect(() => deployment.setUser()).not.toThrow();
        });

        it('accepts any arguments without error', () => {
            const deployment = new OperatorDeployment();
            expect(() => deployment.setUser({})).not.toThrow();
            expect(() => deployment.setUser(null)).not.toThrow();
            expect(() => deployment.setUser('user')).not.toThrow();
        });
    });

    describe('integration with mount and destroy', () => {
        it('full lifecycle: mount, destroy', async () => {
            const container = buildMockContainer();

            const deployment = new OperatorDeployment();
            await deployment.mount(container);

            deployment.destroy();

            expect(deployment._container).toBeNull();
        });

        it('can remount after destroy', async () => {
            const container = buildMockContainer();

            const deployment = new OperatorDeployment();
            await deployment.mount(container);
            deployment.destroy();

            await deployment.mount(container);

            expect(deployment._container).toBe(container);
        });
    });
});
