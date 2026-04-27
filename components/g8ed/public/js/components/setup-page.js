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

import { ApiPaths } from '../constants/api-paths.js';
import { ComponentName } from '../constants/service-client-constants.js';

const LAST_STEP = 2;

/**
 * LLM provider catalog is server-injected as a JSON script tag in setup.ejs
 * (sourced from components/g8ed/constants/ai.js — the single source of truth).
 * Reading it from the DOM avoids duplicating the catalog in a browser-side module.
 */
function _readCatalog() {
    const el = typeof document !== 'undefined' ? document.getElementById('llm-catalog') : null;
    if (!el || !el.textContent) return { providers: {}, providerModels: {}, providerDefaultModels: {} };
    try {
        return JSON.parse(el.textContent);
    } catch {
        return { providers: {}, providerModels: {}, providerDefaultModels: {} };
    }
}

const {
    providers: LLMProvider,
    providerModels: PROVIDER_MODELS,
    providerDefaultModels: PROVIDER_DEFAULT_MODELS,
} = _readCatalog();

// Wire-protocol provider identifiers are stable canonical strings
// (see shared/constants/status.json and components/g8ed/constants/ai.js).
const PROVIDER_KEY_FIELDS = {
    gemini:    'gemini_api_key',
    anthropic: 'anthropic_api_key',
    openai:    'openai_api_key',
    ollama:    'ollama_url',
};

const PROVIDER_LABELS = {
    gemini:    'Gemini',
    anthropic: 'Anthropic',
    openai:    'OpenAI',
    ollama:    'Ollama',
};

function _modelToProvider(modelValue) {
    for (const [provider, config] of Object.entries(PROVIDER_MODELS)) {
        if ((config.all || []).some(m => m.id === modelValue)) return provider;
    }
    return null;
}

function _base64urlToBuffer(b64url) {
    const b64 = b64url.replace(/-/g, '+').replace(/_/g, '/');
    const bin = atob(b64);
    const buf = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) buf[i] = bin.charCodeAt(i);
    return buf.buffer;
}

function _bufferToBase64url(buf) {
    const bytes = new Uint8Array(buf);
    let str = '';
    for (const b of bytes) str += String.fromCharCode(b);
    return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

export const EMPTY_MODEL_PLACEHOLDER = 'Select a Model';

function _prepareCreationOptions(options) {
    return {
        ...options,
        challenge: _base64urlToBuffer(options.challenge),
        user: {
            ...options.user,
            id: _base64urlToBuffer(options.user.id),
        },
        excludeCredentials: (options.excludeCredentials || []).map(c => ({
            ...c,
            id: _base64urlToBuffer(c.id),
        })),
    };
}

function _serializeCredential(cred) {
    return {
        id:    cred.id,
        rawId: _bufferToBase64url(cred.rawId),
        type:  cred.type,
        response: {
            attestationObject: _bufferToBase64url(cred.response.attestationObject),
            clientDataJSON:    _bufferToBase64url(cred.response.clientDataJSON),
        },
        clientExtensionResults: cred.getClientExtensionResults(),
    };
}

export class SetupPage {
    constructor() {
        this._step           = 1;
        this._searchProvider = '';
        this._selectedModels = { primary: '', assistant: '', lite: '' };
        this._lastProviderEdited = null;
    }

    init() {
        this._initNavButtons();
        this._initProviderKeyInputs();
        this._initRevealButtons();
        this._initFinishButton();
        this._initSearchProvider();
        this._initUseGeminiKeyCheckbox();
        this._initModelDropdowns();
        this._updateProviderStates();
        this._updateNav();
    }

    // ---------------------------------------------------------------------------
    // Status bar
    // ---------------------------------------------------------------------------

    _showStatus(type, msg) {
        const bar  = document.getElementById('setup-status');
        const icon = document.getElementById('setup-status-icon');
        const text = document.getElementById('setup-status-msg');
        const icons = { success: 'check_circle', error: 'error', info: 'info', loading: 'sync' };
        bar.className = `setup-status visible ${type}`;
        icon.textContent = icons[type] || 'info';
        if (type === 'loading') icon.classList.add('spin');
        else icon.classList.remove('spin');
        text.textContent = msg;
    }

    _clearStatus() {
        document.getElementById('setup-status').className = 'setup-status';
    }

    // ---------------------------------------------------------------------------
    // Step navigation
    // ---------------------------------------------------------------------------

    _goToStep(step) {
        if (step > this._step && !this._validateStep(this._step)) return;

        document.querySelector(`[data-panel="${this._step}"]`).classList.remove('active');
        const prevStepEl = document.querySelector(`[data-step="${this._step}"]`);
        prevStepEl.classList.remove('active');
        if (step > this._step) prevStepEl.classList.add('done');

        this._step = step;

        document.querySelector(`[data-panel="${step}"]`).classList.add('active');
        document.querySelector(`[data-step="${step}"]`).classList.add('active');

        this._updateNav();
        this._clearStatus();

        if (step === LAST_STEP) {
            this._renderSummary();
        }

        window.scrollTo({ top: 0, behavior: 'smooth' });
    }

    _updateNav() {
        const nav     = document.getElementById('wizard-nav');
        const backBtn = document.getElementById('wizard-back-btn');
        const nextBtn = document.getElementById('wizard-next-btn');

        const showNav = this._step < LAST_STEP;
        nav.style.display = showNav ? '' : 'none';

        if (backBtn) backBtn.style.display = this._step > 1 ? '' : 'none';
        if (nextBtn) {
            const show = this._step === 1 && this._isProviderStepReady();
            nextBtn.style.display = show ? '' : 'none';
        }
    }

    _validateStep(step) {
        if (step !== 1) return true;

        const active = this._getActiveProviders();
        if (active.length === 0) {
            this._showStatus('error', 'Configure at least one provider (API key or Ollama endpoint)');
            return false;
        }
        if (!this._selectedModels.primary) {
            this._showStatus('error', 'Select a primary model');
            return false;
        }
        if (!this._selectedModels.assistant) {
            this._showStatus('error', 'Select an assistant model');
            return false;
        }
        if (!this._selectedModels.lite) {
            this._showStatus('error', 'Select a lite model');
            return false;
        }

        const ollamaUrl = document.getElementById('ollama_url')?.value.trim();
        if (ollamaUrl) {
            const focusField = () => document.getElementById('ollama_url').focus();
            if (ollamaUrl.startsWith('https://')) {
                this._showStatus('error', 'Ollama only supports HTTP, not HTTPS');
                focusField();
                return false;
            }
            const stripped = ollamaUrl.replace(/^http:\/\//i, '');
            if (stripped.includes('/')) {
                this._showStatus('error', 'Enter Ollama host as "host:port" (no path, no /v1)');
                focusField();
                return false;
            }
            if (!/^[A-Za-z0-9._-]+:\d{1,5}$/.test(stripped)) {
                this._showStatus('error', 'Ollama host must be "host:port" (e.g. 192.168.1.100:11434)');
                focusField();
                return false;
            }
        }

        if (this._searchProvider === 'google') {
            const searchKey = document.getElementById('search_api_key')?.value.trim();
            const projectId = document.getElementById('google_project_id')?.value.trim();
            const appId = document.getElementById('vertex_ai_search_app_id')?.value.trim();
            if (!searchKey || !projectId || !appId) {
                this._showStatus('error', 'Complete Google search configuration or select "None"');
                return false;
            }
        }

        return true;
    }

    // ---------------------------------------------------------------------------
    // Back / Next navigation
    // ---------------------------------------------------------------------------

    _initNavButtons() {
        document.getElementById('wizard-back-btn').addEventListener('click', () => {
            if (this._step > 1) this._goToStep(this._step - 1);
        });
        document.getElementById('wizard-next-btn').addEventListener('click', () => {
            if (this._step < LAST_STEP) this._goToStep(this._step + 1);
        });

        document.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && this._step === 1 && this._isProviderStepReady()) {
                const activeEl = document.activeElement;
                if (activeEl && (activeEl.tagName === 'INPUT' || activeEl.tagName === 'SELECT')) {
                    e.preventDefault();
                    this._goToStep(this._step + 1);
                }
            }
        });
    }

    // ---------------------------------------------------------------------------
    // Provider key inputs and model dropdowns
    // ---------------------------------------------------------------------------

    _initProviderKeyInputs() {
        Object.entries(PROVIDER_KEY_FIELDS).forEach(([provider, id]) => {
            const el = document.getElementById(id);
            if (el) el.addEventListener('input', () => this._onProviderKeyChange(provider));
        });

        const searchKeyEl = document.getElementById('search_api_key');
        if (searchKeyEl) searchKeyEl.addEventListener('input', () => this._updateNav());

        ['google_project_id', 'vertex_ai_search_app_id'].forEach(id => {
            const el = document.getElementById(id);
            if (el) el.addEventListener('input', () => this._updateNav());
        });
    }

    _onProviderKeyChange(provider) {
        this._lastProviderEdited = provider;
        this._validateApiKey(provider);
        this._updateProviderStates();
        this._updateModelDropdowns();
        this._updateNav();
    }

    _validateApiKey(provider) {
        const fieldId = PROVIDER_KEY_FIELDS[provider];
        if (!fieldId) return;
        
        const el = document.getElementById(fieldId);
        if (!el) return;
        
        const val = el.value.trim();
        const row = el.closest('.wizard-provider-key-row');
        if (!row) return;

        let hint = '';
        if (val) {
            if (provider === 'gemini' && !val.startsWith('AIza')) {
                hint = 'Key usually starts with "AIza"';
            } else if (provider === 'openai' && !val.startsWith('sk-')) {
                hint = 'Key usually starts with "sk-"';
            } else if (provider === 'anthropic' && !val.startsWith('sk-ant-')) {
                hint = 'Key usually starts with "sk-ant-"';
            }
        }

        let hintEl = row.querySelector('.wizard-provider-key-hint');
        if (!hintEl && hint) {
            hintEl = document.createElement('div');
            hintEl.className = 'wizard-provider-key-hint';
            hintEl.style.fontSize = '12px';
            hintEl.style.color = 'var(--accent-blue)';
            hintEl.style.marginTop = '4px';
            el.parentNode.parentNode.appendChild(hintEl);
        }
        
        if (hintEl) {
            hintEl.textContent = hint;
            hintEl.style.display = hint ? 'block' : 'none';
        }
    }

    _getActiveProviders() {
        const active = [];
        for (const [provider, fieldId] of Object.entries(PROVIDER_KEY_FIELDS)) {
            if (!fieldId) continue;
            const el = document.getElementById(fieldId);
            if (el && el.value.trim()) active.push(provider);
        }
        return active;
    }

    _updateProviderStates() {
        const active = this._getActiveProviders();

        document.querySelectorAll('.wizard-provider-key-row').forEach(row => {
            const provider = row.getAttribute('data-provider');
            const hasValue = active.includes(provider);
            row.classList.toggle('has-value', hasValue);
            const statusEl = row.querySelector('.wizard-provider-key-status');
            if (statusEl) statusEl.textContent = hasValue ? 'Configured' : '';
        });

        const modelSection = document.getElementById('wizard-model-selection');
        if (modelSection) modelSection.classList.toggle('active', active.length > 0);
    }

    _initModelDropdowns() {
        const roles = ['primary', 'assistant', 'lite'];
        roles.forEach(role => {
            const dropdown = document.getElementById(`${role}_model`);
            if (!dropdown) return;

            dropdown.addEventListener('click', (e) => {
                e.stopPropagation();
                this._toggleDropdown(role);
            });

            dropdown.addEventListener('keydown', (e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    this._toggleDropdown(role);
                } else if (e.key === 'Escape') {
                    this._closeAllDropdowns();
                }
            });
        });

        document.addEventListener('click', () => {
            this._closeAllDropdowns();
        });
    }

    _toggleDropdown(role) {
        const dropdown = document.getElementById(`${role}_model`);
        if (!dropdown || dropdown.classList.contains('disabled')) return;
        
        const isOpen = dropdown.classList.contains('open');
        this._closeAllDropdowns();
        
        if (!isOpen) {
            dropdown.classList.add('open');
            dropdown.setAttribute('aria-expanded', 'true');
        }
    }

    _closeAllDropdowns() {
        const roles = ['primary', 'assistant', 'lite'];
        roles.forEach(role => {
            const dropdown = document.getElementById(`${role}_model`);
            if (dropdown) {
                dropdown.classList.remove('open');
                dropdown.setAttribute('aria-expanded', 'false');
            }
        });
    }

    _updateModelDropdowns() {
        const active = this._getActiveProviders();
        const roles = ['primary', 'assistant', 'lite'];

        if (active.length === 0) {
            roles.forEach(role => {
                const dropdown = document.getElementById(`${role}_model`);
                const menu = document.getElementById(`${role}_model-menu`);
                const text = dropdown?.querySelector('.llm-model-dropdown__text');
                const badge = dropdown?.querySelector('.llm-model-dropdown__recommended-badge');
                
                if (dropdown) dropdown.classList.add('disabled');
                if (menu) menu.innerHTML = '';
                if (text) text.textContent = 'Enter at least one API key';
                if (badge) badge.style.display = 'none';
                this._selectedModels[role] = '';
            });
            return;
        }

        roles.forEach(role => {
            const dropdown = document.getElementById(`${role}_model`);
            const text = dropdown?.querySelector('.llm-model-dropdown__text');
            const badge = dropdown?.querySelector('.llm-model-dropdown__recommended-badge');
            
            if (!dropdown) return;
            
            dropdown.classList.remove('disabled');

            let prevValue = this._selectedModels[role];
            if (prevValue && prevValue !== 'custom') {
                const stillAvailable = active.some(p => {
                    const cfg = PROVIDER_MODELS[p];
                    return cfg && (cfg.all || []).some(m => m.id === prevValue);
                });
                if (!stillAvailable) {
                    prevValue = '';
                    this._selectedModels[role] = '';
                }
            }
            if (!prevValue) {
                // If a provider key was just entered and it has sensible defaults, use those
                if (this._lastProviderEdited && active.includes(this._lastProviderEdited)) {
                    const defaults = PROVIDER_DEFAULT_MODELS[this._lastProviderEdited];
                    if (defaults && defaults[role]) {
                        const defaultModel = PROVIDER_MODELS[this._lastProviderEdited]?.all?.find(m => m.id === defaults[role]);
                        if (defaultModel) {
                            prevValue = defaultModel.id;
                            this._selectedModels[role] = prevValue;
                            if (text) text.textContent = defaultModel.label;
                        }
                    }
                }
            }

            if (badge) badge.style.display = 'none';
            // Show recommended badge if the selected model matches the default for its provider
            const provider = _modelToProvider(prevValue);
            if (provider && PROVIDER_DEFAULT_MODELS[provider]?.[role] === prevValue) {
                if (badge) badge.style.display = '';
            }

            if (text && !prevValue) text.textContent = EMPTY_MODEL_PLACEHOLDER;
            
            this._renderModelDropdownMenu(role, active, prevValue);
        });
    }

    _renderModelDropdownMenu(role, activeProviders, selectedValue) {
        const menu = document.getElementById(`${role}_model-menu`);
        if (!menu) return;
        
        menu.innerHTML = '';

        for (const provider of activeProviders) {
            const config = PROVIDER_MODELS[provider];
            if (!config) continue;

            const providerLabel = PROVIDER_LABELS[provider] || provider;
            const models = config.all || [];
            
            if (models.length === 0) continue;

            const category = document.createElement('div');
            category.className = 'llm-model-dropdown__category';
            category.textContent = providerLabel;
            menu.appendChild(category);

            for (const model of models) {
                const option = document.createElement('div');
                option.className = 'llm-model-dropdown__option';
                option.textContent = model.label;
                option.dataset.value = model.id;
                option.dataset.provider = provider;

                if (model.id === selectedValue) {
                    option.classList.add('selected');
                }

                option.addEventListener('click', (e) => {
                    e.stopPropagation();
                    this._selectModel(role, model.id, model.label);
                });

                menu.appendChild(option);
            }
        }

        const customOption = document.createElement('div');
        customOption.className = 'llm-model-dropdown__option';
        customOption.textContent = 'Custom...';
        customOption.dataset.value = 'custom';
        customOption.dataset.custom = 'true';

        if (selectedValue === 'custom') {
            customOption.classList.add('selected');
        }

        customOption.addEventListener('click', (e) => {
            e.stopPropagation();
            this._showCustomModelInput(role);
        });

        menu.appendChild(customOption);

        if (menu.children.length === 0) {
            const dropdown = document.getElementById(`${role}_model`);
            const text = dropdown?.querySelector('.llm-model-dropdown__text');
            if (text) text.textContent = 'No models available';
            this._selectedModels[role] = '';
        }
    }

    _selectModel(role, modelId, label) {
        this._selectedModels[role] = modelId;

        const dropdown = document.getElementById(`${role}_model`);
        const menu = document.getElementById(`${role}_model-menu`);
        const text = dropdown?.querySelector('.llm-model-dropdown__text');

        if (text) text.textContent = label;

        const options = menu?.querySelectorAll('.llm-model-dropdown__option') || [];
        options.forEach(option => {
            if (option.dataset.value === modelId) {
                option.classList.add('selected');
            } else {
                option.classList.remove('selected');
            }
        });

        this._closeAllDropdowns();
        this._updateNav();
    }

    _showCustomModelInput(role) {
        const customLabel = prompt('Enter custom model name:');
        if (customLabel && customLabel.trim()) {
            this._selectedModels[role] = 'custom';
            this._selectedModels[`${role}CustomLabel`] = customLabel.trim();

            const dropdown = document.getElementById(`${role}_model`);
            const menu = document.getElementById(`${role}_model-menu`);
            const text = dropdown?.querySelector('.llm-model-dropdown__text');

            if (text) text.textContent = customLabel.trim();

            const options = menu?.querySelectorAll('.llm-model-dropdown__option') || [];
            options.forEach(option => {
                if (option.dataset.value === 'custom') {
                    option.classList.add('selected');
                } else {
                    option.classList.remove('selected');
                }
            });

            this._closeAllDropdowns();
            this._updateNav();
        }
    }

    _isProviderStepReady() {
        const active = this._getActiveProviders();
        if (active.length === 0) return false;
        const primary = this._selectedModels.primary;
        const assistant = this._selectedModels.assistant;
        const lite = this._selectedModels.lite;
        return !!(primary && assistant && lite);
    }

    // ---------------------------------------------------------------------------
    // Search provider
    // ---------------------------------------------------------------------------

    _initSearchProvider() {
        const select = document.getElementById('search_provider');
        if (!select) return;
        select.addEventListener('change', () => {
            this._searchProvider = select.value;
            const googleConfig = document.getElementById('search-config-google');
            if (googleConfig) {
                googleConfig.classList.toggle('setup-field-hidden', select.value !== 'google');
            }
            this._updateUseGeminiKeyVisibility();
        });
    }

    // ---------------------------------------------------------------------------
    // "Use Gemini API Key" checkbox
    // ---------------------------------------------------------------------------

    _initUseGeminiKeyCheckbox() {
        const checkbox = document.getElementById('use_gemini_api_key');
        if (!checkbox) return;
        checkbox.addEventListener('change', () => {
            const searchKeyEl = document.getElementById('search_api_key');
            const geminiKeyEl = document.getElementById('gemini_api_key');
            if (!searchKeyEl) return;
            if (checkbox.checked) {
                searchKeyEl.value = geminiKeyEl?.value.trim() || '';
            } else {
                searchKeyEl.value = '';
            }
            searchKeyEl.dispatchEvent(new Event('input', { bubbles: true }));
        });
    }

    _updateUseGeminiKeyVisibility() {
        const wrap = document.getElementById('use-gemini-key-wrap');
        const checkbox = document.getElementById('use_gemini_api_key');
        const geminiKeyEl = document.getElementById('gemini_api_key');
        if (!wrap || !checkbox) return;

        const hasGeminiKey = !!(geminiKeyEl && geminiKeyEl.value.trim());
        const showCheckbox = hasGeminiKey && this._searchProvider === 'google';
        wrap.classList.toggle('setup-field-hidden', !showCheckbox);
        if (!showCheckbox && checkbox.checked) {
            checkbox.checked = false;
        }
    }

    // ---------------------------------------------------------------------------
    // Reveal buttons (password fields)
    // ---------------------------------------------------------------------------

    _initRevealButtons() {
        document.querySelectorAll('.setup-reveal-btn').forEach(btn => {
            btn.addEventListener('click', function () {
                const inp = document.getElementById(this.getAttribute('data-for'));
                if (!inp) return;
                const hidden = inp.type === 'password';
                inp.type = hidden ? 'text' : 'password';
                this.querySelector('.material-symbols-outlined').textContent = hidden ? 'visibility_off' : 'visibility';
            });
        });
    }

    // ---------------------------------------------------------------------------
    // Summary
    // ---------------------------------------------------------------------------

    _renderSummary() {
        const primaryModel = this._selectedModels.primary || '';
        const assistantModel = this._selectedModels.assistant || '';
        const liteModel = this._selectedModels.lite || '';

        const primaryModelDisplay = primaryModel === 'custom' ? (this._selectedModels.primaryCustomLabel || 'Custom') : primaryModel;
        const assistantModelDisplay = assistantModel === 'custom' ? (this._selectedModels.assistantCustomLabel || 'Custom') : assistantModel;
        const liteModelDisplay = liteModel === 'custom' ? (this._selectedModels.liteCustomLabel || 'Custom') : liteModel;

        const activeProviders = this._getActiveProviders();
        const providerLabels = activeProviders.map(p => PROVIDER_LABELS[p] || p).join(', ') || 'None';

        const searchProviderLabel = {
            google: 'Google',
        }[this._searchProvider] || 'None';

        const rows = [
            { icon: 'psychology',     label: 'Providers',       value: providerLabels },
            { icon: 'model_training', label: 'Primary Model',   value: primaryModelDisplay },
            { icon: 'assistant',      label: 'Assistant Model', value: assistantModelDisplay },
            { icon: 'bolt',           label: 'Lite Model',      value: liteModelDisplay },
            { icon: 'travel_explore', label: 'Web Search',      value: searchProviderLabel },
        ];

        const container = document.getElementById('wizard-summary');
        container.replaceChildren(...rows.map(r => {
            const row = document.createElement('div');
            row.className = 'wizard-summary-row';

            const icon = document.createElement('span');
            icon.className = 'material-symbols-outlined wizard-summary-icon';
            icon.textContent = r.icon;

            const label = document.createElement('span');
            label.className = 'wizard-summary-label';
            label.textContent = r.label;

            const value = document.createElement('span');
            value.className = 'wizard-summary-value';
            value.textContent = r.value;

            row.append(icon, label, value);
            return row;
        }));
    }

    // ---------------------------------------------------------------------------
    // User settings collection (no platform settings -- derived server-side)
    // ---------------------------------------------------------------------------

    _collectUserSettings() {
        const userSettings = {};

        const primaryModel = this._selectedModels.primary || '';
        const assistantModel = this._selectedModels.assistant || '';
        const liteModel = this._selectedModels.lite || '';

        const primaryModelValue = primaryModel === 'custom' ? this._selectedModels.primaryCustomLabel : primaryModel;
        const assistantModelValue = assistantModel === 'custom' ? this._selectedModels.assistantCustomLabel : assistantModel;
        const liteModelValue = liteModel === 'custom' ? this._selectedModels.liteCustomLabel : liteModel;

        const primaryProvider = primaryModel === 'custom' ? this._getActiveProviders()[0] : _modelToProvider(primaryModel);
        const assistantProvider = assistantModel === 'custom' ? this._getActiveProviders()[0] : _modelToProvider(assistantModel);
        const liteProvider = liteModel === 'custom' ? this._getActiveProviders()[0] : _modelToProvider(liteModel);

        if (primaryProvider) {
            userSettings.llm_primary_provider = primaryProvider;
        }
        if (assistantProvider) {
            userSettings.llm_assistant_provider = assistantProvider;
        }
        if (liteProvider) {
            userSettings.llm_lite_provider = liteProvider;
        }
        if (primaryModelValue) userSettings.llm_model = primaryModelValue;
        if (assistantModelValue) userSettings.llm_assistant_model = assistantModelValue;
        if (liteModelValue) userSettings.llm_lite_model = liteModelValue;

        const geminiKey = document.getElementById('gemini_api_key')?.value.trim();
        if (geminiKey) userSettings.gemini_api_key = geminiKey;

        const anthropicKey = document.getElementById('anthropic_api_key')?.value.trim();
        if (anthropicKey) userSettings.anthropic_api_key = anthropicKey;

        const openaiKey = document.getElementById('openai_api_key')?.value.trim();
        if (openaiKey) {
            userSettings.openai_api_key = openaiKey;
        }

        const ollamaHost = document.getElementById('ollama_url')?.value.trim();
        if (ollamaHost) {
            userSettings.ollama_endpoint = ollamaHost;
        }

        if (this._searchProvider === 'google') {
            const searchKey = document.getElementById('search_api_key')?.value.trim();
            if (searchKey) userSettings.vertex_search_api_key = searchKey;

            const projectId = document.getElementById('google_project_id')?.value.trim();
            if (projectId) userSettings.vertex_search_project_id = projectId;

            const appId = document.getElementById('vertex_ai_search_app_id')?.value.trim();
            if (appId) userSettings.vertex_search_engine_id = appId;

            userSettings.vertex_search_enabled = true;
        }

        return userSettings;
    }

    // ---------------------------------------------------------------------------
    // API calls
    // ---------------------------------------------------------------------------

    async _registerUser(data) {
        const res = await window.serviceClient.post(ComponentName.G8ED, ApiPaths.auth.register(), data);
        return res.json();
    }

    // ---------------------------------------------------------------------------
    // Finish button
    // ---------------------------------------------------------------------------

    _initFinishButton() {
        document.getElementById('finish-btn').addEventListener('click', async () => {
            const btn   = document.getElementById('finish-btn');
            btn.disabled = true;

            try {
                this._showStatus('loading', 'Creating account and saving configuration...');
                const userSettings = this._collectUserSettings();

                const userJson = await this._registerUser({
                    settings: userSettings
                });

                const userId = userJson.user_id;
                const challengeOptions = userJson.challenge_options;

                if (!challengeOptions) {
                    throw new Error('Server did not return passkey challenge');
                }

                this._showStatus('loading', 'Follow your browser prompt to register a passkey...');
                const options = _prepareCreationOptions(challengeOptions);
                const cred    = await navigator.credentials.create({ publicKey: options });

                this._showStatus('loading', 'Finalizing setup...');
                const verifyRes  = await window.serviceClient.post(ComponentName.G8ED, ApiPaths.auth.passkey.registerVerifySetup(), {
                    user_id: userId,
                    attestation_response: _serializeCredential(cred)
                });
                const verifyJson = await verifyRes.json();
                if (!verifyJson.session) {
                    throw new Error('Passkey registration failed — no session returned');
                }

                this._showStatus('success', 'Account created! Redirecting...');
                setTimeout(() => { window.location.href = '/chat'; }, 1000);

            } catch (err) {
                this._showStatus('error', err.message);
                btn.disabled = false;
            }
        });
    }
}

const page = new SetupPage();
page.init();
