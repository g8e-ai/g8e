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

import { LLMProvider, PROVIDER_MODELS } from '../constants/ai-constants.js';
import { ApiPaths } from '../constants/api-paths.js';
import { ComponentName } from '../constants/service-client-constants.js';

const LAST_STEP = 4;

const PROVIDER_KEY_FIELDS = {
    [LLMProvider.GEMINI]:    'gemini_api_key',
    [LLMProvider.ANTHROPIC]: 'anthropic_api_key',
    [LLMProvider.OPENAI]:    'openai_api_key',
    [LLMProvider.OLLAMA]:    'ollama_url',
};

const PROVIDER_LABELS = {
    [LLMProvider.GEMINI]:    'Gemini',
    [LLMProvider.ANTHROPIC]: 'Anthropic',
    [LLMProvider.OPENAI]:    'OpenAI',
    [LLMProvider.OLLAMA]:    'Ollama',
};

function _modelToProvider(modelValue) {
    for (const [provider, config] of Object.entries(PROVIDER_MODELS)) {
        const allModels = [...config.primary, ...config.assistant];
        if (allModels.some(m => m.id === modelValue)) return provider;
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
    }

    init() {
        this._initNavButtons();
        this._initProviderKeyInputs();
        this._initRevealButtons();
        this._initFinishButton();
        this._initSearchProvider();
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
            const show = this._step < 2 || (this._step === 2 && this._isProviderStepReady()) || this._step === 3;
            nextBtn.style.display = show ? '' : 'none';
        }
    }

    _validateStep(step) {
        if (step === 1) {
            const email = document.getElementById('account_email').value.trim();
            if (!email) {
                this._showStatus('error', 'Email address is required');
                document.getElementById('account_email').focus();
                return false;
            }
            const emailRe = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
            if (!emailRe.test(email)) {
                this._showStatus('error', 'Enter a valid email address');
                document.getElementById('account_email').focus();
                return false;
            }
        }

        if (step === 2) {
            const active = this._getActiveProviders();
            if (active.length === 0) {
                this._showStatus('error', 'Enter at least one provider API key');
                return false;
            }
            const primary = document.getElementById('primary_model')?.value;
            if (!primary) {
                this._showStatus('error', 'Select a primary model');
                return false;
            }
            const assistant = document.getElementById('assistant_model')?.value;
            if (!assistant) {
                this._showStatus('error', 'Select an assistant model');
                return false;
            }
            const lite = document.getElementById('lite_model')?.value;
            if (!lite) {
                this._showStatus('error', 'Select a lite model');
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
            if (e.key === 'Enter' && this._step >= 2 && this._step <= LAST_STEP - 1) {
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
        Object.values(PROVIDER_KEY_FIELDS).forEach(id => {
            const el = document.getElementById(id);
            if (el) el.addEventListener('input', () => this._onProviderKeyChange());
        });

        const searchKeyEl = document.getElementById('search_api_key');
        if (searchKeyEl) searchKeyEl.addEventListener('input', () => this._updateNav());

        ['google_project_id', 'vertex_ai_search_app_id'].forEach(id => {
            const el = document.getElementById(id);
            if (el) el.addEventListener('input', () => this._updateNav());
        });
    }

    _onProviderKeyChange() {
        this._updateProviderStates();
        this._updateModelDropdowns();
        this._updateNav();
    }

    _getActiveProviders() {
        const active = [];
        for (const [provider, fieldId] of Object.entries(PROVIDER_KEY_FIELDS)) {
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

    _updateModelDropdowns() {
        const active = this._getActiveProviders();
        const primarySelect = document.getElementById('primary_model');
        const assistantSelect = document.getElementById('assistant_model');
        const liteSelect = document.getElementById('lite_model');
        if (!primarySelect || !assistantSelect || !liteSelect) return;

        const prevPrimary = primarySelect.value;
        const prevAssistant = assistantSelect.value;
        const prevLite = liteSelect.value;

        primarySelect.innerHTML = '';
        assistantSelect.innerHTML = '';
        liteSelect.innerHTML = '';

        if (active.length === 0) {
            primarySelect.disabled = true;
            assistantSelect.disabled = true;
            liteSelect.disabled = true;
            const placeholder = document.createElement('option');
            placeholder.value = '';
            placeholder.textContent = 'Enter at least one API key above';
            primarySelect.appendChild(placeholder);
            assistantSelect.appendChild(placeholder.cloneNode(true));
            liteSelect.appendChild(placeholder.cloneNode(true));
            return;
        }

        primarySelect.disabled = false;
        assistantSelect.disabled = false;
        liteSelect.disabled = false;

        let firstPrimaryDefault = null;
        let firstAssistantDefault = null;
        let firstLiteDefault = null;

        for (const provider of active) {
            const config = PROVIDER_MODELS[provider];
            if (!config) continue;

            const providerLabel = PROVIDER_LABELS[provider] || provider;

            const pGroup = document.createElement('optgroup');
            pGroup.label = providerLabel;
            for (const m of config.primary) {
                const opt = document.createElement('option');
                opt.value = m.id;
                opt.textContent = m.label;
                pGroup.appendChild(opt);
            }
            primarySelect.appendChild(pGroup);

            const aGroup = document.createElement('optgroup');
            aGroup.label = providerLabel;
            for (const m of config.assistant) {
                const opt = document.createElement('option');
                opt.value = m.id;
                opt.textContent = m.label;
                aGroup.appendChild(opt);
            }
            assistantSelect.appendChild(aGroup);

            const lGroup = document.createElement('optgroup');
            lGroup.label = providerLabel;
            for (const m of config.lite) {
                const opt = document.createElement('option');
                opt.value = m.id;
                opt.textContent = m.label;
                lGroup.appendChild(opt);
            }
            liteSelect.appendChild(lGroup);

            if (!firstPrimaryDefault) firstPrimaryDefault = config.defaultPrimary;
            if (!firstAssistantDefault) firstAssistantDefault = config.defaultAssistant;
            if (!firstLiteDefault) firstLiteDefault = config.defaultLite;
        }

        if (prevPrimary && this._selectHasValue(primarySelect, prevPrimary)) {
            primarySelect.value = prevPrimary;
        } else if (firstPrimaryDefault) {
            primarySelect.value = firstPrimaryDefault;
        }

        if (prevAssistant && this._selectHasValue(assistantSelect, prevAssistant)) {
            assistantSelect.value = prevAssistant;
        } else if (firstAssistantDefault) {
            assistantSelect.value = firstAssistantDefault;
        }

        if (prevLite && this._selectHasValue(liteSelect, prevLite)) {
            liteSelect.value = prevLite;
        } else if (firstLiteDefault) {
            liteSelect.value = firstLiteDefault;
        }
    }

    _selectHasValue(selectEl, value) {
        return Array.from(selectEl.options).some(o => o.value === value);
    }

    _isProviderStepReady() {
        const active = this._getActiveProviders();
        if (active.length === 0) return false;
        const primary = document.getElementById('primary_model')?.value;
        const assistant = document.getElementById('assistant_model')?.value;
        const lite = document.getElementById('lite_model')?.value;
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
            if (googleConfig) googleConfig.classList.toggle('setup-field-hidden', select.value !== 'google');
        });
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
        const primaryModel = document.getElementById('primary_model')?.value || '';
        const assistantModel = document.getElementById('assistant_model')?.value || '';
        const liteModel = document.getElementById('lite_model')?.value || '';
        const primaryProvider = _modelToProvider(primaryModel);
        const email = document.getElementById('account_email').value.trim();
        const name = document.getElementById('account_name').value.trim();

        const activeProviders = this._getActiveProviders();
        const providerLabels = activeProviders.map(p => PROVIDER_LABELS[p] || p).join(', ') || 'None';

        const searchProviderLabel = {
            google: 'Google',
        }[this._searchProvider] || 'None';

        const rows = [
            { icon: 'person',         label: 'Account',         value: name ? `${name} (${email})` : email },
            { icon: 'psychology',     label: 'Providers',       value: providerLabels },
            { icon: 'model_training', label: 'Primary Model',   value: primaryModel },
            { icon: 'assistant',      label: 'Assistant Model', value: assistantModel },
            { icon: 'bolt',           label: 'Lite Model',      value: liteModel },
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

        const primaryModel = document.getElementById('primary_model')?.value || '';
        const assistantModel = document.getElementById('assistant_model')?.value || '';
        const liteModel = document.getElementById('lite_model')?.value || '';
        const primaryProvider = _modelToProvider(primaryModel);
        const assistantProvider = _modelToProvider(assistantModel);
        const liteProvider = _modelToProvider(liteModel);

        if (primaryProvider) {
            userSettings.llm_primary_provider = primaryProvider;
        }
        if (assistantProvider) {
            userSettings.llm_assistant_provider = assistantProvider;
        }
        if (liteProvider) {
            userSettings.llm_lite_provider = liteProvider;
        }
        if (primaryModel) userSettings.llm_model = primaryModel;
        if (assistantModel) userSettings.llm_assistant_model = assistantModel;
        if (liteModel) userSettings.llm_lite_model = liteModel;

        const geminiKey = document.getElementById('gemini_api_key')?.value.trim();
        if (geminiKey) userSettings.gemini_api_key = geminiKey;

        const anthropicKey = document.getElementById('anthropic_api_key')?.value.trim();
        if (anthropicKey) userSettings.anthropic_api_key = anthropicKey;

        const openaiKey = document.getElementById('openai_api_key')?.value.trim();
        if (openaiKey) {
            userSettings.openai_api_key = openaiKey;
            userSettings.openai_endpoint = 'https://api.openai.com/v1';
        }

        const ollamaUrl = document.getElementById('ollama_url')?.value.trim();
        if (ollamaUrl) {
            userSettings.ollama_endpoint = ollamaUrl.endsWith('/') ? ollamaUrl + 'v1' : ollamaUrl + '/v1';
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

            const email = document.getElementById('account_email').value.trim();
            const name  = document.getElementById('account_name').value.trim();

            try {
                this._showStatus('loading', 'Creating account and saving configuration...');
                const userSettings = this._collectUserSettings();

                const userJson = await this._registerUser({
                    email,
                    name,
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
                const verifyRes  = await window.serviceClient.post(ComponentName.G8ED, ApiPaths.auth.passkey.registerVerify(), {
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
