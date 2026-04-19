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
import { MockServiceClient } from '@test/mocks/mock-browser-env.js';
import {
    LLMProvider,
    AnthropicModel,
    OllamaModel,
    PROVIDER_MODELS,
} from '@g8ed/constants/ai.js';
import { ApiPaths } from '@g8ed/public/js/constants/api-paths.js';
import { ComponentName } from '@g8ed/public/js/constants/service-client-constants.js';

// Import SetupPage class dynamically to prevent auto-execution
let SetupPage;

describe('SetupPage [FRONTEND - jsdom]', () => {
    let serviceClient;
    let setupPage;

    beforeEach(async () => {
        // Set up DOM first
        serviceClient = new MockServiceClient();
        window.serviceClient = serviceClient;
        window.scrollTo = vi.fn();
        
        // Mock window.location.href to prevent jsdom navigation error
        delete window.location;
        window.location = {
            href: '/setup',
            pathname: '/setup',
            search: '',
            origin: 'http://localhost'
        };
        
        // Mock navigator.credentials for WebAuthn
        navigator.credentials = {
            create: vi.fn()
        };

        // Set up DOM structure for the setup wizard (multi-provider layout).
        // The llm-catalog script tag mirrors what setup.ejs injects in production,
        // providing the canonical LLM provider catalog to setup-page.js.
        const catalogJson = JSON.stringify({
            providers: LLMProvider,
            providerModels: PROVIDER_MODELS,
        }).replace(/</g, '\\u003c');
        document.body.innerHTML = `
            <script id="llm-catalog" type="application/json">${catalogJson}</script>
            <div id="setup-status" class="setup-status">
                <span id="setup-status-icon" class="material-symbols-outlined"></span>
                <span id="setup-status-msg"></span>
            </div>
            <div id="wizard-nav" style="display:none">
                <button id="wizard-back-btn" style="display:none">Back</button>
                <button id="wizard-next-btn" style="display:none">Next</button>
            </div>
            <div data-panel="1" class="active"></div>
            <div data-panel="2"></div>
            <div data-panel="3"></div>
            <div data-panel="4"></div>
            <div data-step="1" class="active"></div>
            <div data-step="2"></div>
            <div data-step="3"></div>
            <div data-step="4"></div>
            <input id="account_email" type="email" />
            <input id="account_name" type="text" />
            <div class="wizard-provider-key-row" data-provider="gemini">
                <span class="wizard-provider-key-status" id="status-gemini"></span>
                <input id="gemini_api_key" type="password" />
                <button class="setup-reveal-btn" data-for="gemini_api_key">
                    <span class="material-symbols-outlined">visibility</span>
                </button>
            </div>
            <div class="wizard-provider-key-row" data-provider="anthropic">
                <span class="wizard-provider-key-status" id="status-anthropic"></span>
                <input id="anthropic_api_key" type="password" />
                <button class="setup-reveal-btn" data-for="anthropic_api_key">
                    <span class="material-symbols-outlined">visibility</span>
                </button>
            </div>
            <div class="wizard-provider-key-row" data-provider="openai">
                <span class="wizard-provider-key-status" id="status-openai"></span>
                <input id="openai_api_key" type="password" />
                <button class="setup-reveal-btn" data-for="openai_api_key">
                    <span class="material-symbols-outlined">visibility</span>
                </button>
            </div>
            <div class="wizard-provider-key-row" data-provider="ollama">
                <span class="wizard-provider-key-status" id="status-ollama"></span>
                <input id="ollama_url" type="text" />
            </div>
            <div id="wizard-model-selection">
                <div id="primary_model" class="llm-model-dropdown disabled" aria-expanded="false">
                    <span class="llm-model-dropdown__text">Enter at least one API key</span>
                </div>
                <div id="primary_model-menu" class="llm-model-dropdown__menu"></div>
                <div id="assistant_model" class="llm-model-dropdown disabled" aria-expanded="false">
                    <span class="llm-model-dropdown__text">Enter at least one API key</span>
                </div>
                <div id="assistant_model-menu" class="llm-model-dropdown__menu"></div>
                <div id="lite_model" class="llm-model-dropdown disabled" aria-expanded="false">
                    <span class="llm-model-dropdown__text">Enter at least one API key</span>
                </div>
                <div id="lite_model-menu" class="llm-model-dropdown__menu"></div>
            </div>
            <select id="search_provider">
                <option value="">None</option>
                <option value="google">Google</option>
            </select>
            <div id="search-config-google" class="setup-field-hidden">
                <input id="search_api_key" type="password" />
                <input id="google_project_id" type="text" />
                <input id="vertex_ai_search_app_id" type="text" />
                <label id="use-gemini-key-wrap" class="setup-field-hidden" for="use_gemini_api_key">
                    <input id="use_gemini_api_key" type="checkbox" />
                    <span>Use Gemini API Key</span>
                </label>
            </div>
            <div id="wizard-summary"></div>
            <button id="finish-btn">Finish</button>
        `;

        // Dynamically import after DOM is ready
        const module = await import('@g8ed/public/js/components/setup-page.js');
        SetupPage = module.SetupPage;
    });

    afterEach(() => {
        vi.clearAllMocks();
        delete window.serviceClient;
        delete window.scrollTo;
        delete navigator.credentials;
    });

    describe('constructor', () => {
        it('initializes step to 1', () => {
            setupPage = new SetupPage();
            expect(setupPage._step).toBe(1);
        });

        it('initializes searchProvider to empty string', () => {
            setupPage = new SetupPage();
            expect(setupPage._searchProvider).toBe('');
        });
    });

    describe('init', () => {
        it('initializes nav button listeners', () => {
            setupPage = new SetupPage();
            const backBtn = document.getElementById('wizard-back-btn');
            const nextBtn = document.getElementById('wizard-next-btn');
            const backSpy = vi.spyOn(backBtn, 'addEventListener');
            const nextSpy = vi.spyOn(nextBtn, 'addEventListener');
            setupPage.init();
            expect(backSpy).toHaveBeenCalledWith('click', expect.any(Function));
            expect(nextSpy).toHaveBeenCalledWith('click', expect.any(Function));
            backSpy.mockRestore();
            nextSpy.mockRestore();
        });

        it('initializes provider key input listeners', () => {
            setupPage = new SetupPage();
            const geminiKey = document.getElementById('gemini_api_key');
            const spy = vi.spyOn(geminiKey, 'addEventListener');
            setupPage.init();
            expect(spy).toHaveBeenCalledWith('input', expect.any(Function));
            spy.mockRestore();
        });

        it('initializes reveal button listeners', () => {
            setupPage = new SetupPage();
            const revealBtn = document.querySelector('.setup-reveal-btn');
            const spy = vi.spyOn(revealBtn, 'addEventListener');
            setupPage.init();
            expect(spy).toHaveBeenCalledWith('click', expect.any(Function));
            spy.mockRestore();
        });

        it('initializes finish button listener', () => {
            setupPage = new SetupPage();
            const finishBtn = document.getElementById('finish-btn');
            const spy = vi.spyOn(finishBtn, 'addEventListener');
            setupPage.init();
            expect(spy).toHaveBeenCalledWith('click', expect.any(Function));
            spy.mockRestore();
        });

        it('initializes search provider listener', () => {
            setupPage = new SetupPage();
            const searchSelect = document.getElementById('search_provider');
            const spy = vi.spyOn(searchSelect, 'addEventListener');
            setupPage.init();
            expect(spy).toHaveBeenCalledWith('change', expect.any(Function));
            spy.mockRestore();
        });

        it('initializes keyboard navigation listener', () => {
            setupPage = new SetupPage();
            const spy = vi.spyOn(document, 'addEventListener');
            setupPage.init();
            expect(spy).toHaveBeenCalledWith('keydown', expect.any(Function));
            spy.mockRestore();
        });

        it('calls _updateNav on init', () => {
            setupPage = new SetupPage();
            const updateNavSpy = vi.spyOn(setupPage, '_updateNav');
            setupPage.init();
            expect(updateNavSpy).toHaveBeenCalled();
        });
    });

    describe('_showStatus', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
        });

        it('shows success status', () => {
            setupPage._showStatus('success', 'Test message');
            const bar = document.getElementById('setup-status');
            const icon = document.getElementById('setup-status-icon');
            const text = document.getElementById('setup-status-msg');
            expect(bar.className).toContain('success');
            expect(bar.className).toContain('visible');
            expect(icon.textContent).toBe('check_circle');
            expect(icon.classList.contains('spin')).toBe(false);
            expect(text.textContent).toBe('Test message');
        });

        it('shows error status', () => {
            setupPage._showStatus('error', 'Error message');
            const bar = document.getElementById('setup-status');
            const icon = document.getElementById('setup-status-icon');
            expect(bar.className).toContain('error');
            expect(icon.textContent).toBe('error');
            expect(icon.classList.contains('spin')).toBe(false);
        });

        it('shows info status', () => {
            setupPage._showStatus('info', 'Info message');
            const icon = document.getElementById('setup-status-icon');
            expect(icon.textContent).toBe('info');
        });

        it('shows loading status with spin', () => {
            setupPage._showStatus('loading', 'Loading...');
            const icon = document.getElementById('setup-status-icon');
            expect(icon.textContent).toBe('sync');
            expect(icon.classList.contains('spin')).toBe(true);
        });

        it('defaults to info icon for unknown type', () => {
            setupPage._showStatus('unknown', 'Message');
            const icon = document.getElementById('setup-status-icon');
            expect(icon.textContent).toBe('info');
        });
    });

    describe('_clearStatus', () => {
        it('removes visible class from status bar', () => {
            setupPage = new SetupPage();
            setupPage._showStatus('success', 'Test');
            setupPage._clearStatus();
            const bar = document.getElementById('setup-status');
            expect(bar.className).toBe('setup-status');
        });
    });

    describe('_goToStep', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
            setupPage.init();
        });

        it('moves to step 2 from step 1', () => {
            document.getElementById('account_email').value = 'test@example.com';
            setupPage._goToStep(2);
            expect(setupPage._step).toBe(2);
            expect(document.querySelector('[data-panel="1"]').classList.contains('active')).toBe(false);
            expect(document.querySelector('[data-panel="2"]').classList.contains('active')).toBe(true);
            expect(document.querySelector('[data-step="1"]').classList.contains('done')).toBe(true);
        });

        it('does not move forward if validation fails', () => {
            setupPage._step = 1;
            document.getElementById('account_email').value = '';
            setupPage._goToStep(2);
            expect(setupPage._step).toBe(1);
        });

        it('moves backward without validation', () => {
            setupPage._step = 3;
            setupPage._goToStep(2);
            expect(setupPage._step).toBe(2);
        });

        it('calls _updateNav', () => {
            const updateNavSpy = vi.spyOn(setupPage, '_updateNav');
            document.getElementById('account_email').value = 'test@example.com';
            setupPage._goToStep(2);
            expect(updateNavSpy).toHaveBeenCalled();
        });

        it('calls _clearStatus', () => {
            const clearStatusSpy = vi.spyOn(setupPage, '_clearStatus');
            document.getElementById('account_email').value = 'test@example.com';
            setupPage._goToStep(2);
            expect(clearStatusSpy).toHaveBeenCalled();
        });

        it('calls _renderSummary on last step', () => {
            const renderSummarySpy = vi.spyOn(setupPage, '_renderSummary');
            setupPage._step = 3;
            setupPage._goToStep(4);
            expect(renderSummarySpy).toHaveBeenCalled();
        });

        it('scrolls to top', () => {
            document.getElementById('account_email').value = 'test@example.com';
            setupPage._goToStep(2);
            expect(window.scrollTo).toHaveBeenCalledWith({ top: 0, behavior: 'smooth' });
        });
    });

    describe('_updateNav', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
            setupPage.init();
        });

        it('shows nav on step 1', () => {
            setupPage._step = 1;
            setupPage._updateNav();
            const nav = document.getElementById('wizard-nav');
            expect(nav.style.display).toBe('');
        });

        it('shows nav on step 2', () => {
            setupPage._step = 2;
            setupPage._updateNav();
            const nav = document.getElementById('wizard-nav');
            expect(nav.style.display).toBe('');
        });

        it('hides nav on step 4 (last step)', () => {
            setupPage._step = 4;
            setupPage._updateNav();
            const nav = document.getElementById('wizard-nav');
            expect(nav.style.display).toBe('none');
        });

        it('hides back button on step 1', () => {
            setupPage._step = 1;
            setupPage._updateNav();
            const backBtn = document.getElementById('wizard-back-btn');
            expect(backBtn.style.display).toBe('none');
        });

        it('shows back button on step 2', () => {
            setupPage._step = 2;
            setupPage._updateNav();
            const backBtn = document.getElementById('wizard-back-btn');
            expect(backBtn.style.display).toBe('');
        });

        it('hides next button on step 2 when no providers configured', () => {
            setupPage._step = 2;
            setupPage._updateNav();
            const nextBtn = document.getElementById('wizard-next-btn');
            expect(nextBtn.style.display).toBe('none');
        });

        it('shows next button on step 2 when provider key entered and models selected', () => {
            setupPage._step = 2;
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._onProviderKeyChange();
            // Models are auto-selected by _updateModelDropdowns
            setupPage._updateNav();
            const nextBtn = document.getElementById('wizard-next-btn');
            expect(nextBtn.style.display).toBe('');
        });

        it('shows next button on step 3', () => {
            setupPage._step = 3;
            setupPage._updateNav();
            const nextBtn = document.getElementById('wizard-next-btn');
            expect(nextBtn.style.display).toBe('');
        });
    });

    describe('_validateStep', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
        });

        it('returns false for step 1 with empty email', () => {
            document.getElementById('account_email').value = '';
            expect(setupPage._validateStep(1)).toBe(false);
        });

        it('shows error for empty email', () => {
            document.getElementById('account_email').value = '';
            setupPage._validateStep(1);
            const text = document.getElementById('setup-status-msg');
            expect(text.textContent).toBe('Email address is required');
        });

        it('returns false for step 1 with invalid email', () => {
            document.getElementById('account_email').value = 'invalid-email';
            expect(setupPage._validateStep(1)).toBe(false);
        });

        it('shows error for invalid email', () => {
            document.getElementById('account_email').value = 'invalid-email';
            setupPage._validateStep(1);
            const text = document.getElementById('setup-status-msg');
            expect(text.textContent).toBe('Enter a valid email address');
        });

        it('returns true for step 1 with valid email', () => {
            document.getElementById('account_email').value = 'test@example.com';
            expect(setupPage._validateStep(1)).toBe(true);
        });

        it('returns false for step 2 with no provider keys entered', () => {
            expect(setupPage._validateStep(2)).toBe(false);
        });

        it('shows error for no provider keys entered', () => {
            setupPage._validateStep(2);
            const text = document.getElementById('setup-status-msg');
            expect(text.textContent).toBe('Enter at least one provider API key');
        });

        it('returns true for step 2 with Gemini key and models selected', () => {
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._onProviderKeyChange();
            // Models are auto-selected by _updateModelDropdowns
            expect(setupPage._validateStep(2)).toBe(true);
        });

        it('returns true for step 2 with OpenAI key and models selected', () => {
            document.getElementById('openai_api_key').value = 'test-key';
            setupPage._onProviderKeyChange();
            // Models are auto-selected by _updateModelDropdowns
            expect(setupPage._validateStep(2)).toBe(true);
        });

        it('returns true for step 2 with Anthropic key and models selected', () => {
            document.getElementById('anthropic_api_key').value = 'test-key';
            setupPage._onProviderKeyChange();
            // Models are auto-selected by _updateModelDropdowns
            expect(setupPage._validateStep(2)).toBe(true);
        });

        it('returns true for step 2 with Ollama URL and models selected', () => {
            document.getElementById('ollama_url').value = 'http://localhost:11434';
            setupPage._onProviderKeyChange();
            // Models are auto-selected by _updateModelDropdowns
            expect(setupPage._validateStep(2)).toBe(true);
        });

        it('returns true for step 2 with bare Ollama host:port', () => {
            document.getElementById('ollama_url').value = '192.168.1.100:11434';
            setupPage._onProviderKeyChange();
            expect(setupPage._validateStep(2)).toBe(true);
        });

        it('rejects Ollama host with /v1 path', () => {
            document.getElementById('ollama_url').value = 'http://localhost:11434/v1';
            setupPage._onProviderKeyChange();
            expect(setupPage._validateStep(2)).toBe(false);
            const text = document.getElementById('setup-status-msg');
            expect(text.textContent).toContain('host:port');
        });

        it('rejects Ollama host missing port', () => {
            document.getElementById('ollama_url').value = 'localhost';
            setupPage._onProviderKeyChange();
            expect(setupPage._validateStep(2)).toBe(false);
        });

        it('rejects https Ollama URL', () => {
            document.getElementById('ollama_url').value = 'https://localhost:11434';
            setupPage._onProviderKeyChange();
            expect(setupPage._validateStep(2)).toBe(false);
            const text = document.getElementById('setup-status-msg');
            expect(text.textContent).toBe('Ollama only supports HTTP, not HTTPS');
        });

        it('returns true for step 3', () => {
            expect(setupPage._validateStep(3)).toBe(true);
        });

        it('returns true for step 4', () => {
            expect(setupPage._validateStep(4)).toBe(true);
        });
    });

    describe('_getActiveProviders', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
        });

        it('returns empty array when no keys entered', () => {
            expect(setupPage._getActiveProviders()).toEqual([]);
        });

        it('returns gemini when gemini key entered', () => {
            document.getElementById('gemini_api_key').value = 'test-key';
            expect(setupPage._getActiveProviders()).toContain(LLMProvider.GEMINI);
        });

        it('returns anthropic when anthropic key entered', () => {
            document.getElementById('anthropic_api_key').value = 'test-key';
            expect(setupPage._getActiveProviders()).toContain(LLMProvider.ANTHROPIC);
        });

        it('returns openai when openai key entered', () => {
            document.getElementById('openai_api_key').value = 'test-key';
            expect(setupPage._getActiveProviders()).toContain(LLMProvider.OPENAI);
        });

        it('returns ollama when ollama url entered', () => {
            document.getElementById('ollama_url').value = 'http://localhost:11434';
            expect(setupPage._getActiveProviders()).toContain(LLMProvider.OLLAMA);
        });

        it('returns multiple providers when multiple keys entered', () => {
            document.getElementById('gemini_api_key').value = 'key1';
            document.getElementById('openai_api_key').value = 'key2';
            const active = setupPage._getActiveProviders();
            expect(active).toContain(LLMProvider.GEMINI);
            expect(active).toContain(LLMProvider.OPENAI);
            expect(active).toHaveLength(2);
        });
    });

    describe('_updateProviderStates', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
        });

        it('adds has-value class when key entered', () => {
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._updateProviderStates();
            const row = document.querySelector('[data-provider="gemini"]');
            expect(row.classList.contains('has-value')).toBe(true);
        });

        it('removes has-value class when key empty', () => {
            document.getElementById('gemini_api_key').value = '';
            setupPage._updateProviderStates();
            const row = document.querySelector('[data-provider="gemini"]');
            expect(row.classList.contains('has-value')).toBe(false);
        });

        it('sets status text to Configured when key entered', () => {
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._updateProviderStates();
            const status = document.getElementById('status-gemini');
            expect(status.textContent).toBe('Configured');
        });

        it('activates model selection when providers active', () => {
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._updateProviderStates();
            const section = document.getElementById('wizard-model-selection');
            expect(section.classList.contains('active')).toBe(true);
        });
    });

    describe('_updateModelDropdowns', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
        });

        it('disables dropdowns when no providers active', () => {
            setupPage._updateModelDropdowns();
            expect(document.getElementById('primary_model').classList.contains('disabled')).toBe(true);
            expect(document.getElementById('assistant_model').classList.contains('disabled')).toBe(true);
        });

        it('enables dropdowns when a provider is active', () => {
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._updateModelDropdowns();
            expect(document.getElementById('primary_model').classList.contains('disabled')).toBe(false);
            expect(document.getElementById('assistant_model').classList.contains('disabled')).toBe(false);
        });

        it('populates models for active provider', () => {
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._updateModelDropdowns();
            const primaryMenu = document.getElementById('primary_model-menu');
            const primaryText = document.getElementById('primary_model').querySelector('.llm-model-dropdown__text');
            expect(primaryMenu.children.length).toBeGreaterThan(0);
            expect(primaryText.textContent).not.toBe('Enter at least one API key');
        });

        it('auto-selects the first available model when providers active but nothing selected', () => {
            document.getElementById('primary_model').querySelector('.llm-model-dropdown__text').textContent = 'API Key Required';
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._selectedModels.primary = '';
            setupPage._updateModelDropdowns();
            const primaryText = document.getElementById('primary_model').querySelector('.llm-model-dropdown__text');
            expect(primaryText.textContent).not.toBe('Enter at least one API key');
            expect(primaryText.textContent).not.toBe('Select a Model');
            expect(setupPage._selectedModels.primary).toBeTruthy();
        });

        it('populates models from multiple active providers', () => {
            document.getElementById('gemini_api_key').value = 'key1';
            document.getElementById('openai_api_key').value = 'key2';
            setupPage._updateModelDropdowns();
            const primaryMenu = document.getElementById('primary_model-menu');
            const categories = primaryMenu.querySelectorAll('.llm-model-dropdown__category');
            expect(categories.length).toBe(2);
        });

        it('preserves previous selection if still available', () => {
            document.getElementById('gemini_api_key').value = 'key1';
            setupPage._updateModelDropdowns();
            const firstSelected = setupPage._selectedModels.primary;
            document.getElementById('openai_api_key').value = 'key2';
            setupPage._updateModelDropdowns();
            expect(setupPage._selectedModels.primary).toBe(firstSelected);
        });

        it('auto-selects sensible defaults when Gemini API key is entered', () => {
            setupPage._selectedModels = { primary: '', assistant: '', lite: '' };
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._onProviderKeyChange('gemini');
            
            const primaryText = document.getElementById('primary_model').querySelector('.llm-model-dropdown__text');
            const assistantText = document.getElementById('assistant_model').querySelector('.llm-model-dropdown__text');
            const liteText = document.getElementById('lite_model').querySelector('.llm-model-dropdown__text');
            
            expect(primaryText.textContent).toBe('Gemini 3.1 Pro');
            expect(assistantText.textContent).toBe('Gemini 3 Flash');
            expect(liteText.textContent).toBe('Gemini 3.1 Flash Lite');
        });

        it('auto-selects sensible defaults when Anthropic API key is entered', () => {
            setupPage._selectedModels = { primary: '', assistant: '', lite: '' };
            document.getElementById('anthropic_api_key').value = 'test-key';
            setupPage._onProviderKeyChange('anthropic');
            
            const primaryText = document.getElementById('primary_model').querySelector('.llm-model-dropdown__text');
            const assistantText = document.getElementById('assistant_model').querySelector('.llm-model-dropdown__text');
            const liteText = document.getElementById('lite_model').querySelector('.llm-model-dropdown__text');
            
            expect(primaryText.textContent).toBe('Claude Opus 4.6');
            expect(assistantText.textContent).toBe('Claude Sonnet 4.6');
            expect(liteText.textContent).toBe('Claude Haiku 4.5');
        });

        it('auto-selects sensible defaults when OpenAI API key is entered', () => {
            setupPage._selectedModels = { primary: '', assistant: '', lite: '' };
            document.getElementById('openai_api_key').value = 'test-key';
            setupPage._onProviderKeyChange('openai');
            
            const primaryText = document.getElementById('primary_model').querySelector('.llm-model-dropdown__text');
            const assistantText = document.getElementById('assistant_model').querySelector('.llm-model-dropdown__text');
            const liteText = document.getElementById('lite_model').querySelector('.llm-model-dropdown__text');
            
            expect(primaryText.textContent).toBe('GPT-5.4 Pro');
            expect(assistantText.textContent).toBe('GPT-5.4');
            expect(liteText.textContent).toBe('GPT-5.4 Mini');
        });

        it('auto-selects sensible defaults when Ollama URL is entered', () => {
            setupPage._selectedModels = { primary: '', assistant: '', lite: '' };
            document.getElementById('ollama_url').value = '192.168.1.100:11434';
            setupPage._onProviderKeyChange('ollama');
            
            const primaryText = document.getElementById('primary_model').querySelector('.llm-model-dropdown__text');
            const assistantText = document.getElementById('assistant_model').querySelector('.llm-model-dropdown__text');
            const liteText = document.getElementById('lite_model').querySelector('.llm-model-dropdown__text');
            
            expect(primaryText.textContent).toBe('Qwen 3.5 122B');
            expect(assistantText.textContent).toBe('GLM 5.1');
            expect(liteText.textContent).toBe('Llama 3.2 3B');
        });

        it('does not override existing model selection when provider key is entered', () => {
            document.getElementById('gemini_api_key').value = 'key1';
            setupPage._onProviderKeyChange('gemini');
            const firstSelected = setupPage._selectedModels.primary;
            
            // Enter a different provider key
            document.getElementById('openai_api_key').value = 'key2';
            setupPage._onProviderKeyChange('openai');
            
            // Should preserve the first selection since it's still available
            expect(setupPage._selectedModels.primary).toBe(firstSelected);
        });
    });

    describe('_isProviderStepReady', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
        });

        it('returns false when no keys entered', () => {
            expect(setupPage._isProviderStepReady()).toBe(false);
        });

        it('returns true when key entered and models populated', () => {
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._onProviderKeyChange();
            // Models are auto-selected by _updateModelDropdowns
            expect(setupPage._isProviderStepReady()).toBe(true);
        });
    });

    describe('_initRevealButtons', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
            setupPage.init();
        });

        it('toggles password field type on click', () => {
            const revealBtn = document.querySelector('.setup-reveal-btn');
            const geminiKey = document.getElementById('gemini_api_key');
            expect(geminiKey.type).toBe('password');
            revealBtn.click();
            expect(geminiKey.type).toBe('text');
            revealBtn.click();
            expect(geminiKey.type).toBe('password');
        });

        it('toggles icon text', () => {
            const revealBtn = document.querySelector('.setup-reveal-btn');
            const icon = revealBtn.querySelector('.material-symbols-outlined');
            expect(icon.textContent).toBe('visibility');
            revealBtn.click();
            expect(icon.textContent).toBe('visibility_off');
            revealBtn.click();
            expect(icon.textContent).toBe('visibility');
        });

        it('handles missing input element gracefully', () => {
            const revealBtn = document.querySelector('.setup-reveal-btn');
            revealBtn.dataset.for = 'nonexistent';
            expect(() => revealBtn.click()).not.toThrow();
        });
    });

    describe('_initProviderKeyInputs', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
        });

        it('adds input listener to gemini_api_key', () => {
            const geminiKey = document.getElementById('gemini_api_key');
            const spy = vi.spyOn(geminiKey, 'addEventListener');
            setupPage._initProviderKeyInputs();
            expect(spy).toHaveBeenCalledWith('input', expect.any(Function));
            spy.mockRestore();
        });

        it('adds input listener to anthropic_api_key', () => {
            const anthropicKey = document.getElementById('anthropic_api_key');
            const spy = vi.spyOn(anthropicKey, 'addEventListener');
            setupPage._initProviderKeyInputs();
            expect(spy).toHaveBeenCalledWith('input', expect.any(Function));
            spy.mockRestore();
        });

        it('adds input listener to openai_api_key', () => {
            const openaiKey = document.getElementById('openai_api_key');
            const spy = vi.spyOn(openaiKey, 'addEventListener');
            setupPage._initProviderKeyInputs();
            expect(spy).toHaveBeenCalledWith('input', expect.any(Function));
            spy.mockRestore();
        });

        it('adds input listener to ollama_url', () => {
            const ollamaUrl = document.getElementById('ollama_url');
            const spy = vi.spyOn(ollamaUrl, 'addEventListener');
            setupPage._initProviderKeyInputs();
            expect(spy).toHaveBeenCalledWith('input', expect.any(Function));
            spy.mockRestore();
        });

        it('adds input listener to search_api_key', () => {
            const searchKey = document.getElementById('search_api_key');
            const spy = vi.spyOn(searchKey, 'addEventListener');
            setupPage._initProviderKeyInputs();
            expect(spy).toHaveBeenCalledWith('input', expect.any(Function));
            spy.mockRestore();
        });

        it('calls _onProviderKeyChange on key input', () => {
            setupPage._initProviderKeyInputs();
            const onChangeSpy = vi.spyOn(setupPage, '_onProviderKeyChange');
            const geminiKey = document.getElementById('gemini_api_key');
            geminiKey.dispatchEvent(new Event('input'));
            expect(onChangeSpy).toHaveBeenCalled();
        });

        it('handles missing elements gracefully', () => {
            document.getElementById('gemini_api_key').remove();
            expect(() => setupPage._initProviderKeyInputs()).not.toThrow();
        });
    });

    describe('_initSearchProvider', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
        });

        it('sets searchProvider on change', () => {
            setupPage._initSearchProvider();
            const searchSelect = document.getElementById('search_provider');
            searchSelect.value = 'google';
            searchSelect.dispatchEvent(new Event('change'));
            expect(setupPage._searchProvider).toBe('google');
        });

        it('shows google config when google selected', () => {
            setupPage._initSearchProvider();
            const searchSelect = document.getElementById('search_provider');
            const googleConfig = document.getElementById('search-config-google');
            searchSelect.value = 'google';
            searchSelect.dispatchEvent(new Event('change'));
            expect(googleConfig.classList.contains('setup-field-hidden')).toBe(false);
        });

        it('hides google config when not google selected', () => {
            setupPage._initSearchProvider();
            const searchSelect = document.getElementById('search_provider');
            const googleConfig = document.getElementById('search-config-google');
            searchSelect.value = '';
            searchSelect.dispatchEvent(new Event('change'));
            expect(googleConfig.classList.contains('setup-field-hidden')).toBe(true);
        });

        it('handles missing search provider select gracefully', () => {
            document.getElementById('search_provider').remove();
            expect(() => setupPage._initSearchProvider()).not.toThrow();
        });
    });

    describe('Use Gemini API Key checkbox', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
            setupPage._initSearchProvider();
            setupPage._initUseGeminiKeyCheckbox();
        });

        it('is hidden when no gemini key is entered', () => {
            document.getElementById('search_provider').value = 'google';
            document.getElementById('search_provider').dispatchEvent(new Event('change'));
            const wrap = document.getElementById('use-gemini-key-wrap');
            expect(wrap.classList.contains('setup-field-hidden')).toBe(true);
        });

        it('is hidden when search provider is not google', () => {
            document.getElementById('gemini_api_key').value = 'AIzaTESTKEY';
            setupPage._updateUseGeminiKeyVisibility();
            const wrap = document.getElementById('use-gemini-key-wrap');
            expect(wrap.classList.contains('setup-field-hidden')).toBe(true);
        });

        it('is shown when gemini key is entered and google is selected', () => {
            document.getElementById('gemini_api_key').value = 'AIzaTESTKEY';
            document.getElementById('search_provider').value = 'google';
            document.getElementById('search_provider').dispatchEvent(new Event('change'));
            const wrap = document.getElementById('use-gemini-key-wrap');
            expect(wrap.classList.contains('setup-field-hidden')).toBe(false);
        });

        it('populates search_api_key with gemini key when checked', () => {
            document.getElementById('gemini_api_key').value = 'AIzaTESTKEY';
            document.getElementById('search_provider').value = 'google';
            document.getElementById('search_provider').dispatchEvent(new Event('change'));

            const checkbox = document.getElementById('use_gemini_api_key');
            checkbox.checked = true;
            checkbox.dispatchEvent(new Event('change'));

            expect(document.getElementById('search_api_key').value).toBe('AIzaTESTKEY');
        });

        it('clears search_api_key when unchecked', () => {
            document.getElementById('gemini_api_key').value = 'AIzaTESTKEY';
            document.getElementById('search_provider').value = 'google';
            document.getElementById('search_provider').dispatchEvent(new Event('change'));

            const checkbox = document.getElementById('use_gemini_api_key');
            checkbox.checked = true;
            checkbox.dispatchEvent(new Event('change'));
            checkbox.checked = false;
            checkbox.dispatchEvent(new Event('change'));

            expect(document.getElementById('search_api_key').value).toBe('');
        });

        it('unchecks the checkbox when it becomes hidden', () => {
            document.getElementById('gemini_api_key').value = 'AIzaTESTKEY';
            document.getElementById('search_provider').value = 'google';
            document.getElementById('search_provider').dispatchEvent(new Event('change'));

            const checkbox = document.getElementById('use_gemini_api_key');
            checkbox.checked = true;
            checkbox.dispatchEvent(new Event('change'));

            document.getElementById('search_provider').value = '';
            document.getElementById('search_provider').dispatchEvent(new Event('change'));

            expect(checkbox.checked).toBe(false);
        });

        it('handles missing checkbox gracefully', () => {
            document.getElementById('use_gemini_api_key').remove();
            expect(() => setupPage._initUseGeminiKeyCheckbox()).not.toThrow();
            expect(() => setupPage._updateUseGeminiKeyVisibility()).not.toThrow();
        });
    });

    describe('_renderSummary', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
        });

        it('renders account with name and email', () => {
            document.getElementById('account_email').value = 'test@example.com';
            document.getElementById('account_name').value = 'Test User';
            setupPage._renderSummary();
            const container = document.getElementById('wizard-summary');
            const rows = container.querySelectorAll('.wizard-summary-row');
            expect(rows[0].querySelector('.wizard-summary-value').textContent).toBe('Test User (test@example.com)');
        });

        it('renders account with email only when name empty', () => {
            document.getElementById('account_email').value = 'test@example.com';
            document.getElementById('account_name').value = '';
            setupPage._renderSummary();
            const container = document.getElementById('wizard-summary');
            const rows = container.querySelectorAll('.wizard-summary-row');
            expect(rows[0].querySelector('.wizard-summary-value').textContent).toBe('test@example.com');
        });

        it('renders configured providers', () => {
            document.getElementById('gemini_api_key').value = 'key1';
            document.getElementById('openai_api_key').value = 'key2';
            setupPage._renderSummary();
            const container = document.getElementById('wizard-summary');
            const rows = container.querySelectorAll('.wizard-summary-row');
            expect(rows[1].querySelector('.wizard-summary-value').textContent).toContain('Gemini');
            expect(rows[1].querySelector('.wizard-summary-value').textContent).toContain('OpenAI');
        });

        it('renders None for providers when no keys entered', () => {
            setupPage._renderSummary();
            const container = document.getElementById('wizard-summary');
            const rows = container.querySelectorAll('.wizard-summary-row');
            expect(rows[1].querySelector('.wizard-summary-value').textContent).toBe('None');
        });

        it('renders primary model from unified dropdown', () => {
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._onProviderKeyChange();
            setupPage._renderSummary();
            const container = document.getElementById('wizard-summary');
            const rows = container.querySelectorAll('.wizard-summary-row');
            expect(rows[2].querySelector('.wizard-summary-value').textContent).not.toBe('');
        });

        it('renders assistant model from unified dropdown', () => {
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._onProviderKeyChange();
            setupPage._renderSummary();
            const container = document.getElementById('wizard-summary');
            const rows = container.querySelectorAll('.wizard-summary-row');
            expect(rows[3].querySelector('.wizard-summary-value').textContent).not.toBe('');
        });

        it('renders Google search provider', () => {
            setupPage._searchProvider = 'google';
            setupPage._renderSummary();
            const container = document.getElementById('wizard-summary');
            const rows = container.querySelectorAll('.wizard-summary-row');
            expect(rows[5].querySelector('.wizard-summary-value').textContent).toBe('Google');
        });

        it('renders None for search provider when not set', () => {
            setupPage._searchProvider = '';
            setupPage._renderSummary();
            const container = document.getElementById('wizard-summary');
            const rows = container.querySelectorAll('.wizard-summary-row');
            expect(rows[5].querySelector('.wizard-summary-value').textContent).toBe('None');
        });
    });

    describe('_collectUserSettings', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
        });

        it('collects all entered API keys', () => {
            document.getElementById('gemini_api_key').value = 'test-gemini-key';
            document.getElementById('anthropic_api_key').value = 'test-anthropic-key';
            document.getElementById('openai_api_key').value = 'test-openai-key';
            setupPage._updateModelDropdowns();
            const settings = setupPage._collectUserSettings();
            expect(settings.gemini_api_key).toBe('test-gemini-key');
            expect(settings.anthropic_api_key).toBe('test-anthropic-key');
            expect(settings.openai_api_key).toBe('test-openai-key');
            expect(settings.openai_endpoint).toBe('https://api.openai.com/v1');
        });

        it('derives llm_primary_provider and llm_assistant_provider from selected models', () => {
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._onProviderKeyChange();
            const settings = setupPage._collectUserSettings();
            expect(settings.llm_primary_provider).toBe(LLMProvider.GEMINI);
            expect(settings.llm_assistant_provider).toBe(LLMProvider.GEMINI);
            expect(settings.llm_model).toBeDefined();
            expect(settings.llm_assistant_model).toBeDefined();
        });

        it('derives correct provider for Anthropic models', () => {
            document.getElementById('anthropic_api_key').value = 'test';
            setupPage._onProviderKeyChange();
            // Manually set selected models to test provider derivation
            setupPage._selectedModels.primary = AnthropicModel.ANTHROPIC_CLAUDE_OPUS_4_6;
            setupPage._selectedModels.assistant = AnthropicModel.ANTHROPIC_CLAUDE_SONNET_4_6;
            const settings = setupPage._collectUserSettings();
            expect(settings.llm_primary_provider).toBe(LLMProvider.ANTHROPIC);
            expect(settings.llm_assistant_provider).toBe(LLMProvider.ANTHROPIC);
        });

        it('derives correct provider for Ollama models', () => {
            document.getElementById('ollama_url').value = 'test';
            setupPage._onProviderKeyChange();
            // Manually set selected models to test provider derivation
            setupPage._selectedModels.primary = OllamaModel.QWEN3_5_122B;
            setupPage._selectedModels.assistant = OllamaModel.GEMMA4_26B;
            const settings = setupPage._collectUserSettings();
            expect(settings.llm_primary_provider).toBe(LLMProvider.OLLAMA);
            expect(settings.llm_assistant_provider).toBe(LLMProvider.OLLAMA);
        });

        it('collects Ollama host as-is (backend normalizes to a URL)', () => {
            document.getElementById('ollama_url').value = '192.168.1.100:11434';
            setupPage._updateModelDropdowns();
            const settings = setupPage._collectUserSettings();
            expect(settings.ollama_endpoint).toBe('192.168.1.100:11434');
        });

        it('collects Google search settings', () => {
            setupPage._searchProvider = 'google';
            document.getElementById('search_api_key').value = 'test-search-key';
            document.getElementById('google_project_id').value = 'test-project';
            document.getElementById('vertex_ai_search_app_id').value = 'test-app-id';
            const settings = setupPage._collectUserSettings();
            expect(settings.vertex_search_api_key).toBe('test-search-key');
            expect(settings.vertex_search_project_id).toBe('test-project');
            expect(settings.vertex_search_engine_id).toBe('test-app-id');
            expect(settings.vertex_search_enabled).toBe(true);
        });

        it('does not include search settings when provider not google', () => {
            setupPage._searchProvider = '';
            const settings = setupPage._collectUserSettings();
            expect(settings.vertex_search_enabled).toBeUndefined();
        });

        it('returns empty object when no keys entered', () => {
            const settings = setupPage._collectUserSettings();
            expect(Object.keys(settings)).toHaveLength(0);
        });
    });

    describe('_registerUser', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
        });

        it('calls serviceClient.post with correct path', async () => {
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.register(), {
                ok: true,
                json: async () => ({
                    user_id: 'user_123',
                    challenge_options: { challenge: 'test-challenge', user: { id: 'user-id' } }
                })
            });
            await setupPage._registerUser({ email: 'test@example.com', name: 'Test User' });
            const log = serviceClient.getRequestLog();
            expect(log[0].path).toBe(ApiPaths.auth.register());
            expect(log[0].service).toBe(ComponentName.G8ED);
        });

        it('sends user data in request body', async () => {
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.register(), {
                ok: true,
                json: async () => ({
                    user_id: 'user_123',
                    challenge_options: { challenge: 'test-challenge', user: { id: 'user-id' } }
                })
            });
            const userData = { email: 'test@example.com', name: 'Test User' };
            await setupPage._registerUser(userData);
            const log = serviceClient.getRequestLog();
            expect(log[0].body).toEqual(userData);
        });

        it('returns parsed JSON response', async () => {
            const mockResponse = {
                user_id: 'user_123',
                challenge_options: { challenge: 'test-challenge', user: { id: 'user-id' } }
            };
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.register(), {
                ok: true,
                json: async () => mockResponse
            });
            const result = await setupPage._registerUser({ email: 'test@example.com', name: 'Test User' });
            expect(result).toEqual(mockResponse);
        });
    });

    describe('_initFinishButton', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
            setupPage.init();
        });

        it('disables button on click', async () => {
            const finishBtn = document.getElementById('finish-btn');
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.register(), {
                ok: true,
                json: async () => ({
                    user_id: 'user_123',
                    challenge_options: { challenge: 'test-challenge', user: { id: 'user-id' } }
                })
            });
            navigator.credentials.create.mockResolvedValue({
                id: 'cred-id',
                rawId: new Uint8Array([1, 2, 3]),
                type: 'public-key',
                response: {
                    attestationObject: new Uint8Array([4, 5, 6]),
                    clientDataJSON: new Uint8Array([7, 8, 9])
                },
                getClientExtensionResults: () => ({})
            });
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.passkey.registerVerifySetup(), {
                ok: true,
                json: async () => ({ session: 'session_123' })
            });
            
            finishBtn.click();
            await new Promise(resolve => setTimeout(resolve, 0));
            expect(finishBtn.disabled).toBe(true);
        });

        it('shows loading status on click', async () => {
            const finishBtn = document.getElementById('finish-btn');
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.register(), {
                ok: true,
                json: async () => ({
                    user_id: 'user_123',
                    challenge_options: { challenge: 'test-challenge', user: { id: 'user-id' } }
                })
            });
            navigator.credentials.create.mockResolvedValue({
                id: 'cred-id',
                rawId: new Uint8Array([1, 2, 3]),
                type: 'public-key',
                response: {
                    attestationObject: new Uint8Array([4, 5, 6]),
                    clientDataJSON: new Uint8Array([7, 8, 9])
                },
                getClientExtensionResults: () => ({})
            });
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.passkey.registerVerifySetup(), {
                ok: true,
                json: async () => ({ session: 'session_123' })
            });
            
            finishBtn.click();
            const text = document.getElementById('setup-status-msg');
            expect(text.textContent).toBe('Creating account and saving configuration...');
        });

        it('calls _registerUser with user data', async () => {
            const finishBtn = document.getElementById('finish-btn');
            document.getElementById('account_email').value = 'test@example.com';
            document.getElementById('account_name').value = 'Test User';
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._updateModelDropdowns();
            
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.register(), {
                ok: true,
                json: async () => ({
                    user_id: 'user_123',
                    challenge_options: { challenge: 'test-challenge', user: { id: 'user-id' } }
                })
            });
            navigator.credentials.create.mockResolvedValue({
                id: 'cred-id',
                rawId: new Uint8Array([1, 2, 3]),
                type: 'public-key',
                response: {
                    attestationObject: new Uint8Array([4, 5, 6]),
                    clientDataJSON: new Uint8Array([7, 8, 9])
                },
                getClientExtensionResults: () => ({})
            });
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.passkey.registerVerifySetup(), {
                ok: true,
                json: async () => ({ session: 'session_123' })
            });
            
            finishBtn.click();
            await new Promise(resolve => setTimeout(resolve, 0));
            
            const log = serviceClient.getRequestLog();
            const registerCall = log.find(call => call.path === ApiPaths.auth.register());
            expect(registerCall.body.email).toBe('test@example.com');
            expect(registerCall.body.name).toBe('Test User');
            expect(registerCall.body.settings).toBeDefined();
        });

        it('calls navigator.credentials.create with prepared options', async () => {
            const finishBtn = document.getElementById('finish-btn');
            document.getElementById('account_email').value = 'test@example.com';
            document.getElementById('account_name').value = 'Test User';
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._updateModelDropdowns();
            
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.register(), {
                ok: true,
                json: async () => ({
                    user_id: 'user_123',
                    challenge_options: {
                        challenge: 'dGVzdC1jaGFsbGVuZ2U',
                        user: { id: 'dXNlci1pZA', name: 'Test User', displayName: 'Test User' }
                    }
                })
            });
            navigator.credentials.create.mockResolvedValue({
                id: 'cred-id',
                rawId: new Uint8Array([1, 2, 3]),
                type: 'public-key',
                response: {
                    attestationObject: new Uint8Array([4, 5, 6]),
                    clientDataJSON: new Uint8Array([7, 8, 9])
                },
                getClientExtensionResults: () => ({})
            });
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.passkey.registerVerifySetup(), {
                ok: true,
                json: async () => ({ session: 'session_123' })
            });
            
            finishBtn.click();
            await new Promise(resolve => setTimeout(resolve, 100));
            
            expect(navigator.credentials.create).toHaveBeenCalledWith({
                publicKey: expect.objectContaining({
                    challenge: expect.any(ArrayBuffer),
                    user: expect.objectContaining({
                        id: expect.any(ArrayBuffer)
                    })
                })
            });
        });

        it('calls passkey verify endpoint', async () => {
            const finishBtn = document.getElementById('finish-btn');
            document.getElementById('account_email').value = 'test@example.com';
            document.getElementById('account_name').value = 'Test User';
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._updateModelDropdowns();
            
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.register(), {
                ok: true,
                json: async () => ({
                    user_id: 'user_123',
                    challenge_options: {
                        challenge: 'dGVzdC1jaGFsbGVuZ2U',
                        user: { id: 'dXNlci1pZA', name: 'Test User', displayName: 'Test User' }
                    }
                })
            });
            navigator.credentials.create.mockResolvedValue({
                id: 'cred-id',
                rawId: new Uint8Array([1, 2, 3]),
                type: 'public-key',
                response: {
                    attestationObject: new Uint8Array([4, 5, 6]),
                    clientDataJSON: new Uint8Array([7, 8, 9])
                },
                getClientExtensionResults: () => ({})
            });
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.passkey.registerVerifySetup(), {
                ok: true,
                json: async () => ({ session: 'session_123' })
            });
            
            finishBtn.click();
            await new Promise(resolve => setTimeout(resolve, 100));
            
            const log = serviceClient.getRequestLog();
            const verifyCall = log.find(call => call.path === ApiPaths.auth.passkey.registerVerifySetup());
            expect(verifyCall).toBeDefined();
            expect(verifyCall.body.user_id).toBe('user_123');
            expect(verifyCall.body.attestation_response).toBeDefined();
        });

        it('shows success status and redirects on success', async () => {
            const finishBtn = document.getElementById('finish-btn');
            document.getElementById('account_email').value = 'test@example.com';
            document.getElementById('account_name').value = 'Test User';
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._updateModelDropdowns();
            
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.register(), {
                ok: true,
                json: async () => ({
                    user_id: 'user_123',
                    challenge_options: {
                        challenge: 'dGVzdC1jaGFsbGVuZ2U',
                        user: { id: 'dXNlci1pZA', name: 'Test User', displayName: 'Test User' }
                    }
                })
            });
            navigator.credentials.create.mockResolvedValue({
                id: 'cred-id',
                rawId: new Uint8Array([1, 2, 3]),
                type: 'public-key',
                response: {
                    attestationObject: new Uint8Array([4, 5, 6]),
                    clientDataJSON: new Uint8Array([7, 8, 9])
                },
                getClientExtensionResults: () => ({})
            });
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.passkey.registerVerifySetup(), {
                ok: true,
                json: async () => ({ session: 'session_123' })
            });
            
            finishBtn.click();
            await new Promise(resolve => setTimeout(resolve, 1100));
            
            const text = document.getElementById('setup-status-msg');
            expect(text.textContent).toBe('Account created! Redirecting...');
            expect(window.location.href).toBe('/chat');
        });

        it('throws error when challenge_options missing', async () => {
            const finishBtn = document.getElementById('finish-btn');
            document.getElementById('account_email').value = 'test@example.com';
            document.getElementById('account_name').value = 'Test User';
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._updateModelDropdowns();
            
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.register(), {
                ok: true,
                json: async () => ({ user_id: 'user_123' })
            });
            
            finishBtn.click();
            await new Promise(resolve => setTimeout(resolve, 0));
            
            const text = document.getElementById('setup-status-msg');
            expect(text.textContent).toBe('Server did not return passkey challenge');
            expect(finishBtn.disabled).toBe(false);
        });

        it('throws error when verify response missing session', async () => {
            const finishBtn = document.getElementById('finish-btn');
            document.getElementById('account_email').value = 'test@example.com';
            document.getElementById('account_name').value = 'Test User';
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._updateModelDropdowns();
            
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.register(), {
                ok: true,
                json: async () => ({
                    user_id: 'user_123',
                    challenge_options: {
                        challenge: 'dGVzdC1jaGFsbGVuZ2U',
                        user: { id: 'dXNlci1pZA', name: 'Test User', displayName: 'Test User' }
                    }
                })
            });
            navigator.credentials.create.mockResolvedValue({
                id: 'cred-id',
                rawId: new Uint8Array([1, 2, 3]),
                type: 'public-key',
                response: {
                    attestationObject: new Uint8Array([4, 5, 6]),
                    clientDataJSON: new Uint8Array([7, 8, 9])
                },
                getClientExtensionResults: () => ({})
            });
            serviceClient.setResponse(ComponentName.G8ED, ApiPaths.auth.passkey.registerVerifySetup(), {
                ok: true,
                json: async () => ({})
            });
            
            finishBtn.click();
            await new Promise(resolve => setTimeout(resolve, 100));
            
            const text = document.getElementById('setup-status-msg');
            expect(text.textContent).toBe('Passkey registration failed — no session returned');
            expect(finishBtn.disabled).toBe(false);
        });
    });

    describe('Keyboard navigation', () => {
        beforeEach(() => {
            setupPage = new SetupPage();
            setupPage.init();
        });

        it('advances step on Enter in input field', () => {
            setupPage._step = 2;
            document.getElementById('gemini_api_key').value = 'test-key';
            setupPage._onProviderKeyChange();
            // Ensure all models are selected (auto-selected by _updateModelDropdowns)
            expect(setupPage._isProviderStepReady()).toBe(true);
            document.getElementById('gemini_api_key').focus();
            
            const event = new KeyboardEvent('keydown', { key: 'Enter', cancelable: true });
            document.dispatchEvent(event);
            
            expect(setupPage._step).toBe(3);
        });

        it('does not advance on Enter when not in input field', () => {
            setupPage._step = 2;
            const event = new KeyboardEvent('keydown', { key: 'Enter', cancelable: true });
            document.dispatchEvent(event);
            expect(setupPage._step).toBe(2);
        });

        it('does not advance on Enter outside step range', () => {
            setupPage._step = 1;
            document.getElementById('account_email').focus();
            const event = new KeyboardEvent('keydown', { key: 'Enter', cancelable: true });
            document.dispatchEvent(event);
            expect(setupPage._step).toBe(1);
        });

        it('prevents default on Enter in input field', () => {
            setupPage._step = 2;
            setupPage._provider = LLMProvider.GEMINI;
            document.getElementById('gemini_api_key').value = 'test-key';
            document.getElementById('gemini_api_key').focus();
            
            const event = new KeyboardEvent('keydown', { key: 'Enter', cancelable: true });
            document.dispatchEvent(event);
            
            expect(event.defaultPrevented).toBe(true);
        });
    });
});
