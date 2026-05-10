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
let tutorialManager;

const TEST_TEMPLATE = `<div class="opdeploy">
    <div class="opdeploy__tutorial-section">
        <button id="start-tutorial-btn">Start Tutorial</button>
    </div>
    <div class="opdeploy__header">
        <span class="opdeploy__header-text">Getting Started</span>
    </div>
    <div class="opdeploy__steps">
        <div class="opdeploy__step">
            <div class="opdeploy__step-number">
                <span class="opdeploy__step-badge">1</span>
                <span class="material-symbols-outlined opdeploy__step-arrow" data-direction="down">arrow_downward</span>
            </div>
            <div class="opdeploy__step-content">
                <div class="opdeploy__step-title">Operator Download & Authentication</div>
                <div class="opdeploy__step-desc">
                    Authenticate and deploy the operator binary to your target host using one of the following methods:
                    <ul class="opdeploy__methods">
                        <li><strong>Manual:</strong> Download the binary to your desktop, copy it to the remote system, and run it using its unique API key:
                            <pre><code>./g8e.operator -k &lt;api_key&gt;</code></pre>
                        </li>
                        <li><strong>Device Link:</strong> Generate a short-lived token to authenticate. Once generated, you can:
                            <ul>
                                <li>Click the <span class="material-symbols-outlined opdeploy__step-inline-icon">terminal</span> icon to copy and run the <strong>one-line deployment script</strong> via curl/wget.
                                    <pre><code>curl -fsSL http://localhost/g8e | sh -s -- &lt;device link token&gt;</code></pre>
                                </li>
                                <li>Or, manually run: <pre><code>./g8e.operator -D &lt;token&gt;</code></pre></li>
                            </ul>
                        </li>
                        <li><strong>Fleet:</strong> Use the <code>g8e</code> script for fleet-scale injection:
                            <pre><code>./g8e operator stream --hosts hosts.txt --device-token &lt;token&gt;</code></pre>
                        </li>
                    </ul>
                </div>
            </div>
        </div>
        <div class="opdeploy__step">
            <div class="opdeploy__step-number">
                <span class="opdeploy__step-badge">2</span>
                <span class="material-symbols-outlined opdeploy__step-arrow" data-direction="left">arrow_back</span>
            </div>
            <div class="opdeploy__step-content">
                <div class="opdeploy__step-title">Bind to Web Session</div>
                <div class="opdeploy__step-desc">Manually <strong>bind</strong> Active operators to your session to enable co-validation. The AI has no standing authority; it can only propose actions on bound operators, which <strong>require your explicit approval</strong> to execute.</div>
                <div class="opdeploy__step-note">g8ep is ready. Click the <span class="material-symbols-outlined opdeploy__step-inline-icon">link</span> icon to begin.</div>
            </div>
        </div>
    </div>
</div>`;

function buildMockContainer() {
    const container = document.createElement('div');
    return container;
}

beforeEach(async () => {
    vi.resetModules();

    vi.doMock('@g8ed/public/js/utils/template-loader.js', () => ({
        templateLoader: {
            load: vi.fn(() => Promise.resolve(TEST_TEMPLATE)),
        },
    }));

    vi.doMock('@g8ed/public/js/utils/tutorial-manager.js', () => ({
        tutorialManager: {
            start: vi.fn(),
        },
    }));

    const mod = await import('@g8ed/public/js/components/operator-deployment.js');
    OperatorDeployment = mod.OperatorDeployment;

    const tlMod = await import('@g8ed/public/js/utils/template-loader.js');
    templateLoader = tlMod.templateLoader;

    const tutMod = await import('@g8ed/public/js/utils/tutorial-manager.js');
    tutorialManager = tutMod.tutorialManager;
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

        it('binds start tutorial button to tutorialManager.start', async () => {
            const container = buildMockContainer();
            const deployment = new OperatorDeployment();
            await deployment.mount(container);

            const tutorialBtn = container.querySelector('#start-tutorial-btn');
            expect(tutorialBtn).not.toBeNull();

            tutorialBtn.click();
            expect(tutorialManager.start).toHaveBeenCalled();
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
