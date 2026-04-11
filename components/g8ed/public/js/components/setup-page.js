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

import { LLMProvider } from '../constants/ai-constants.js';
import { ApiPaths } from '../constants/api-paths.js';
import { ServiceName } from '../constants/service-client-constants.js';

const LAST_STEP = 4;

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
        this._provider       = null;
        this._searchProvider = '';
    }

    init() {
        this._initNavButtons();
        this._initProviderCards();
        this._initApiKeyListeners();
        this._initRevealButtons();
        this._initFinishButton();
        this._initSearchProvider();
        const checkedProvider = document.querySelector('input[name="ai_provider"]:checked');
        if (checkedProvider) this._selectProvider(checkedProvider.value);
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
            if (!this._provider) {
                this._showStatus('error', 'Select an AI provider');
                return false;
            }
            if (this._provider === LLMProvider.GEMINI) {
                const key = document.getElementById('gemini_api_key').value.trim();
                if (!key) {
                    this._showStatus('error', 'Gemini API key is required');
                    document.getElementById('gemini_api_key').focus();
                    return false;
                }
            } else if (this._provider === LLMProvider.ANTHROPIC) {
                const key = document.getElementById('anthropic_api_key').value.trim();
                if (!key) {
                    this._showStatus('error', 'Anthropic API key is required');
                    document.getElementById('anthropic_api_key').focus();
                    return false;
                }
            } else if (this._provider === LLMProvider.OPENAI) {
                const key = document.getElementById('openai_api_key').value.trim();
                if (!key) {
                    this._showStatus('error', 'OpenAI API key is required');
                    document.getElementById('openai_api_key').focus();
                    return false;
                }
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
    // Provider cards
    // ---------------------------------------------------------------------------

    _initProviderCards() {
        document.querySelectorAll('.wizard-provider-card input[type="radio"]').forEach(input => {
            input.addEventListener('change', () => {
                this._selectProvider(input.value);
            });
        });
    }

    _isProviderStepReady() {
        if (!this._provider) return false;
        if (this._provider === LLMProvider.OLLAMA) {
            const urlEl = document.getElementById('ollama_url');
            return !!(urlEl && urlEl.value.trim());
        }
        const keyId = { [LLMProvider.GEMINI]: 'gemini_api_key', [LLMProvider.ANTHROPIC]: 'anthropic_api_key', [LLMProvider.OPENAI]: 'openai_api_key' }[this._provider];
        const el = keyId && document.getElementById(keyId);
        return !!(el && el.value.trim());
    }

    _selectProvider(provider) {
        this._provider = provider;

        document.querySelectorAll('.wizard-provider-card').forEach(c => {
            c.classList.toggle('active', c.getAttribute('data-provider') === provider);
        });

        document.querySelectorAll('.wizard-provider-config').forEach(cfg => {
            cfg.style.display = 'none';
        });
        const configEl = document.getElementById(`config-${provider}`);
        if (configEl) configEl.style.display = 'flex';

        this._clearStatus();
        this._updateNav();
    }

    // ---------------------------------------------------------------------------
    // Reveal buttons (password fields)
    // ---------------------------------------------------------------------------

    _initApiKeyListeners() {
        ['gemini_api_key', 'anthropic_api_key', 'openai_api_key', 'ollama_url'].forEach(id => {
            const el = document.getElementById(id);
            if (el) el.addEventListener('input', () => this._updateNav());
        });

        const searchKeyEl = document.getElementById('search_api_key');
        if (searchKeyEl) searchKeyEl.addEventListener('input', () => this._updateNav());

        ['google_project_id', 'vertex_ai_search_app_id'].forEach(id => {
            const el = document.getElementById(id);
            if (el) el.addEventListener('input', () => this._updateNav());
        });
    }

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
        const provider  = this._provider || 'not set';
        const model     = this._getSelectedModel();
        const email     = document.getElementById('account_email').value.trim();
        const name      = document.getElementById('account_name').value.trim();

        const providerLabel = {
            [LLMProvider.GEMINI]:    'Gemini',
            [LLMProvider.ANTHROPIC]: 'Anthropic',
            [LLMProvider.OPENAI]:    'OpenAI',
            [LLMProvider.OLLAMA]:    'Ollama',
        }[provider] || provider;

        const assistantModel = this._getSelectedAssistantModel();

        const searchProviderLabel = {
            google: 'Google',
        }[this._searchProvider] || 'None';

        const rows = [
            { icon: 'person',         label: 'Account',         value: name ? `${name} (${email})` : email },
            { icon: 'psychology',     label: 'AI Provider',     value: providerLabel },
            { icon: 'model_training', label: 'Primary Model',   value: model },
            { icon: 'assistant',      label: 'Assistant Model', value: assistantModel },
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

    _getSelectedModel() {
        if (!this._provider) return '';
        const el = document.getElementById(`${this._provider}_primary_model`);
        return el ? el.value : '';
    }

    _getSelectedAssistantModel() {
        if (!this._provider) return '';
        const el = document.getElementById(`${this._provider}_assistant_model`);
        return el ? el.value : '';
    }

    // ---------------------------------------------------------------------------
    // User settings collection (no platform settings -- derived server-side)
    // ---------------------------------------------------------------------------

    _normalizeOllamaUrl(url) {
        if (!url) return url;
        let normalized = url.trim();

        try {
            const urlObj = new URL(normalized);
            if (!urlObj.port) {
                urlObj.port = '11434';
            }
            if (!urlObj.pathname.endsWith('/v1')) {
                urlObj.pathname = urlObj.pathname.replace(/\/$/, '') + '/v1';
            }
            return urlObj.toString();
        } catch (e) {
            return normalized;
        }
    }

    _collectUserSettings() {
        const userSettings = {};

        if (this._provider === LLMProvider.GEMINI) {
            userSettings.llm_provider = LLMProvider.GEMINI;
            userSettings.llm_model = this._getSelectedModel();
            userSettings.llm_assistant_model = this._getSelectedAssistantModel();
            const key = document.getElementById('gemini_api_key').value.trim();
            if (key) userSettings.gemini_api_key = key;

        } else if (this._provider === LLMProvider.ANTHROPIC) {
            userSettings.llm_provider = LLMProvider.ANTHROPIC;
            userSettings.llm_model = this._getSelectedModel();
            userSettings.llm_assistant_model = this._getSelectedAssistantModel();
            const key = document.getElementById('anthropic_api_key').value.trim();
            if (key) userSettings.anthropic_api_key = key;

        } else if (this._provider === LLMProvider.OPENAI) {
            userSettings.llm_provider = LLMProvider.OPENAI;
            userSettings.openai_endpoint = 'https://api.openai.com/v1';
            userSettings.llm_model = this._getSelectedModel();
            userSettings.llm_assistant_model = this._getSelectedAssistantModel();
            const key = document.getElementById('openai_api_key').value.trim();
            if (key) userSettings.openai_api_key = key;

        } else if (this._provider === LLMProvider.OLLAMA) {
            userSettings.llm_provider = LLMProvider.OLLAMA;
            const rawUrl = document.getElementById('ollama_url').value.trim();
            userSettings.ollama_endpoint = this._normalizeOllamaUrl(rawUrl);
            userSettings.llm_model = this._getSelectedModel();
            userSettings.llm_assistant_model = this._getSelectedAssistantModel();
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
