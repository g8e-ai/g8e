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

/**
 * LLM provider catalog is server-injected as a JSON script tag in settings.ejs
 * (sourced from components/g8ed/constants/ai.js — the single source of truth).
 * Reading it from the DOM avoids duplicating the catalog in a browser-side module.
 */
export function _readCatalog() {
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

const PROVIDER_LABELS = {
    gemini:    'Gemini',
    anthropic: 'Anthropic',
    openai:    'OpenAI',
    ollama:    'Ollama',
};

function _modelToProvider(modelValue, providerModels = PROVIDER_MODELS) {
    for (const [provider, config] of Object.entries(providerModels)) {
        if ((config.all || []).some(m => m.id === modelValue)) return provider;
    }
    return null;
}

function _getModelKey(role) {
    const keyMap = { primary: 'llm_model', assistant: 'llm_assistant_model', lite: 'llm_lite_model' };
    return keyMap[role];
}

function escHtml(str) {
    if (str == null) return '';
    if (typeof str !== 'string') str = String(str);
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

function escAttr(str) {
    return escHtml(str);
}

export const EMPTY_MODEL_PLACEHOLDER = 'Select a Model';

export class SettingsPage {
    constructor(options = {}) {
        this.allSettings = [];
        this.sections = [];
        this.dirty = new Map();
        this.activeSection = null;
        this.selectedModels = { primary: '', assistant: '', lite: '' };
        this.lastProviderEdited = null;

        // Allow injecting catalog for tests
        this.PROVIDER_MODELS = options.providerModels || PROVIDER_MODELS;
        this.PROVIDER_DEFAULT_MODELS = options.providerDefaultModels || PROVIDER_DEFAULT_MODELS;
        this.LLMProvider = options.providers || LLMProvider;
    }

    init() {
        document.getElementById('save-btn').addEventListener('click', () => this._saveSettings());
        this._loadSettings();
    }

    _initModelDropdowns() {
        const roles = ['primary', 'assistant', 'lite'];
        roles.forEach(role => {
            const dropdown = document.getElementById(`${role}_model`);
            if (!dropdown) {
                console.error(`[SettingsPage] Dropdown element not found: ${role}_model`);
                return;
            }
            console.log(`[SettingsPage] Initializing dropdown: ${role}_model`, dropdown);

            dropdown.addEventListener('click', (e) => {
                console.log(`[SettingsPage] Dropdown clicked: ${role}`, e);
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
        console.log(`[SettingsPage] _toggleDropdown called for: ${role}`, dropdown);
        if (!dropdown) {
            console.error(`[SettingsPage] Dropdown not found in _toggleDropdown: ${role}_model`);
            return;
        }
        if (dropdown.classList.contains('disabled')) {
            console.log(`[SettingsPage] Dropdown is disabled: ${role}`);
            return;
        }

        const isOpen = dropdown.classList.contains('open');
        console.log(`[SettingsPage] Dropdown ${role} isOpen: ${isOpen}`);
        this._closeAllDropdowns();

        if (!isOpen) {
            dropdown.classList.add('open');
            dropdown.setAttribute('aria-expanded', 'true');
            console.log(`[SettingsPage] Opened dropdown: ${role}`);
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
        const roles = ['primary', 'assistant', 'lite'];

        roles.forEach(role => {
            const dropdown = document.getElementById(`${role}_model`);
            const menu = document.getElementById(`${role}_model-menu`);
            const text = dropdown?.querySelector('.llm-model-dropdown__text');

            if (!dropdown || !menu) return;

            menu.innerHTML = '';

            let prevValue = this.selectedModels[role] || '';
            if (text && !prevValue) text.textContent = EMPTY_MODEL_PLACEHOLDER;

            // Show all providers regardless of API key configuration
            const activeProviders = Object.keys(this.PROVIDER_MODELS);

            dropdown.classList.remove('disabled');

            // Auto-select sensible defaults if a provider key was just entered and no model is selected
            if (!prevValue && this.lastProviderEdited && activeProviders.includes(this.lastProviderEdited)) {
                const defaults = this.PROVIDER_DEFAULT_MODELS[this.lastProviderEdited];
                if (defaults && defaults[role]) {
                    const defaultModel = this.PROVIDER_MODELS[this.lastProviderEdited]?.all?.find(m => m.id === defaults[role]);
                    if (defaultModel) {
                        prevValue = defaultModel.id;
                        this.selectedModels[role] = prevValue;
                        if (text) text.textContent = defaultModel.label;
                        this._markDirty(_getModelKey(role), prevValue);
                    }
                }
            }

            // Try to find the selected model in ALL providers (not just active) to get its label
            let foundLabel = null;
            for (const provider of Object.keys(this.PROVIDER_MODELS)) {
                const config = this.PROVIDER_MODELS[provider];
                if (!config) continue;
                const allModels = config.all || [];
                const match = allModels.find(m => m.id === prevValue);
                if (match) {
                    foundLabel = match.label;
                    break;
                }
            }

            // If found in catalog but not in active providers, still set the text
            if (foundLabel && !activeProviders.some(p => {
                const config = this.PROVIDER_MODELS[p];
                return config?.all?.some(m => m.id === prevValue);
            })) {
                if (text) text.textContent = foundLabel;
            }

            // Handle custom model label
            if (!foundLabel && prevValue && this.selectedModels[`${role}CustomLabel`]) {
                if (text) text.textContent = this.selectedModels[`${role}CustomLabel`];
            }

            for (const provider of activeProviders) {
                const config = this.PROVIDER_MODELS[provider];
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

                    if (model.id === prevValue) {
                        option.classList.add('selected');
                        if (text) text.textContent = model.label;
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

            if (prevValue === 'custom') {
                customOption.classList.add('selected');
                if (text) text.textContent = this.selectedModels[`${role}CustomLabel`] || 'Custom';
            }

            customOption.addEventListener('click', (e) => {
                e.stopPropagation();
                this._showCustomModelInput(role);
            });

            menu.appendChild(customOption);
        });
    }

    _selectModel(role, modelId, label) {
        this.selectedModels[role] = modelId;

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

        // Mark dirty with the appropriate key
        const keyMap = { primary: 'llm_model', assistant: 'llm_assistant_model', lite: 'llm_lite_model' };
        const key = keyMap[role];
        if (key) {
            this._markDirty(key, modelId);
        }
    }

    _showCustomModelInput(role) {
        const customLabel = prompt('Enter custom model name:');
        if (customLabel && customLabel.trim()) {
            this.selectedModels[role] = 'custom';
            this.selectedModels[`${role}CustomLabel`] = customLabel.trim();

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

            const keyMap = { primary: 'llm_model', assistant: 'llm_assistant_model', lite: 'llm_lite_model' };
            const key = keyMap[role];
            if (key) {
                this._markDirty(key, customLabel.trim());
            }
        }
    }

    _showStatus(type, msg) {
        const bar  = document.getElementById('status-bar');
        const icon = document.getElementById('status-icon');
        const text = document.getElementById('status-msg');
        const icons = { success: 'check_circle', error: 'error', info: 'info' };
        bar.className = `settings-status visible ${type}`;
        icon.textContent = icons[type] || 'info';
        text.textContent = msg;
    }

    _hideStatus() {
        document.getElementById('status-bar').className = 'settings-status';
    }

    _markDirty(key, value) {
        this.dirty.set(key, value);
        document.getElementById('save-btn').disabled = false;
        this._hideStatus();
    }

    _validateApiKey(provider, input) {
        if (!input) return;
        const val = input.value.trim();
        const row = input.closest('.settings-field');
        if (!row) return;

        let hint = '';
        if (val && !val.includes('*')) { // Only validate if user typed something new
            if (provider === 'gemini' && !val.startsWith('AIza')) {
                hint = 'Key usually starts with "AIza"';
            } else if (provider === 'openai' && !val.startsWith('sk-')) {
                hint = 'Key usually starts with "sk-"';
            } else if (provider === 'anthropic' && !val.startsWith('sk-ant-')) {
                hint = 'Key usually starts with "sk-ant-"';
            }
        }

        let hintEl = row.querySelector('.settings-field-hint');
        if (!hintEl && hint) {
            hintEl = document.createElement('div');
            hintEl.className = 'settings-field-hint';
            hintEl.style.fontSize = '12px';
            hintEl.style.color = 'var(--accent-blue)';
            hintEl.style.marginTop = '4px';
            row.appendChild(hintEl);
        }
        
        if (hintEl) {
            hintEl.textContent = hint;
            hintEl.style.display = hint ? 'block' : 'none';
        }
    }

    _buildNav() {
        const nav = document.getElementById('settings-nav');
        nav.innerHTML = '';
        this.sections.forEach((sec, idx) => {
            const btn = document.createElement('button');
            btn.className = 'settings-nav-item' + (idx === 0 ? ' active' : '');
            btn.setAttribute('data-section', sec.id);
            const icon = document.createElement('span');
            icon.className = 'material-symbols-outlined';
            icon.textContent = sec.icon;
            btn.appendChild(icon);
            btn.appendChild(document.createTextNode(sec.label));
            btn.addEventListener('click', () => this._switchSection(sec.id));
            nav.appendChild(btn);
        });
    }

    _switchSection(sectionId) {
        this.activeSection = sectionId;
        document.querySelectorAll('.settings-nav-item').forEach(btn => {
            btn.classList.toggle('active', btn.getAttribute('data-section') === sectionId);
        });
        document.querySelectorAll('.settings-section').forEach(el => {
            el.classList.toggle('active', el.getAttribute('data-section') === sectionId);
        });
    }

    _buildSections() {
        const container = document.getElementById('settings-sections');
        container.innerHTML = '';

        this.sections.forEach((sec, idx) => {
            const secSettings = this.allSettings.filter(s => s.section === sec.id);
            if (!secSettings.length && sec.id !== 'advanced') return;

            const panel = document.createElement('div');
            panel.className = 'settings-section' + (idx === 0 ? ' active' : '');
            panel.setAttribute('data-section', sec.id);

            const header = document.createElement('div');
            header.className = 'settings-section-header';
            const icon = document.createElement('span');
            icon.className = 'material-symbols-outlined settings-section-icon';
            icon.textContent = sec.icon;
            const title = document.createElement('h2');
            title.className = 'settings-section-title';
            title.textContent = sec.label;
            header.appendChild(icon);
            header.appendChild(title);
            panel.appendChild(header);

            if (sec.id === 'llm') {
                this._buildLlmSection(panel, secSettings);
            } else if (sec.id === 'search') {
                this._buildSearchSection(panel, secSettings);
            } else if (sec.id === 'advanced') {
                this._buildAdvancedSection(panel);
            } else {
                secSettings.forEach(setting => {
                    panel.appendChild(this._buildField(setting));
                });
            }

            container.appendChild(panel);
        });
    }

    _buildLlmSection(panel, settings) {
        const providerSpecificSettings = settings.filter(s => s.provider);

        // Create custom model dropdown section (3 dropdowns in a row, categorized)
        const modelSelectionContainer = document.createElement('div');
        modelSelectionContainer.className = 'setup-fields-model-selection';

        const wizardModelSelection = document.createElement('div');
        wizardModelSelection.className = 'wizard-model-selection';
        wizardModelSelection.id = 'wizard-model-selection';

        const modelFields = document.createElement('div');
        modelFields.className = 'wizard-model-fields';

        // Primary model dropdown
        const primaryModelField = this._buildModelDropdownField('primary', 'Primary Model', 'model_training');
        modelFields.appendChild(primaryModelField);

        // Assistant model dropdown
        const assistantModelField = this._buildModelDropdownField('assistant', 'Assistant Model', 'psychology');
        modelFields.appendChild(assistantModelField);

        // Lite model dropdown
        const liteModelField = this._buildModelDropdownField('lite', 'Lite Model', 'bolt');
        modelFields.appendChild(liteModelField);

        wizardModelSelection.appendChild(modelFields);
        modelSelectionContainer.appendChild(wizardModelSelection);
        panel.appendChild(modelSelectionContainer);

        // API keys section
        const apiKeysContainer = document.createElement('div');
        apiKeysContainer.className = 'setup-fields-api-keys';

        // Group settings by provider
        const providerGroups = {};
        providerSpecificSettings.forEach(s => {
            // Skip model fields since they're now handled by custom dropdowns
            if (s.key === 'llm_model' || s.key === 'llm_assistant_model' || s.key === 'llm_lite_model') {
                // Initialize selectedModels from current values
                const role = s.key === 'llm_model' ? 'primary' : s.key === 'llm_assistant_model' ? 'assistant' : 'lite';
                const value = this.dirty.get(s.key) || s.value;
                if (value) {
                    this.selectedModels[role] = value;
                    // If it's a custom model (not in any provider catalog), store as custom label
                    if (!_modelToProvider(value, this.PROVIDER_MODELS)) {
                        this.selectedModels[`${role}CustomLabel`] = value;
                    }
                }
                return;
            }
            if (!providerGroups[s.provider]) {
                providerGroups[s.provider] = [];
            }
            providerGroups[s.provider].push(s);
        });

        // Build provider rows matching setup.ejs structure
        const providerConfig = {
            gemini: {
                icon: 'auto_awesome',
                name: 'Gemini',
                sub: 'Google'
            },
            anthropic: {
                icon: 'smart_toy',
                name: 'Claude',
                sub: 'Anthropic'
            },
            openai: {
                icon: 'hub',
                name: 'GPT',
                sub: 'OpenAI'
            },
            ollama: {
                icon: 'lan',
                name: 'Ollama',
                sub: 'Self-Hosted'
            }
        };

        for (const [provider, config] of Object.entries(providerConfig)) {
            const providerSettings = providerGroups[provider] || [];
            if (providerSettings.length === 0) continue;

            const providerRow = document.createElement('div');
            providerRow.className = 'wizard-provider-key-row';
            providerRow.setAttribute('data-provider', provider);

            const providerHeader = document.createElement('div');
            providerHeader.className = 'wizard-provider-key-header';

            const headerIcon = document.createElement('span');
            headerIcon.className = 'material-symbols-outlined wizard-provider-key-icon';
            headerIcon.textContent = config.icon;

            const nameStrong = document.createElement('strong');
            nameStrong.textContent = config.name;

            const subSpan = document.createElement('span');
            subSpan.className = 'wizard-provider-key-sub';
            subSpan.textContent = config.sub;

            const statusSpan = document.createElement('span');
            statusSpan.className = 'wizard-provider-key-status';
            statusSpan.id = `status-${provider}`;

            providerHeader.appendChild(headerIcon);
            providerHeader.appendChild(nameStrong);
            providerHeader.appendChild(subSpan);
            providerHeader.appendChild(statusSpan);

            providerRow.appendChild(providerHeader);

            // Add the field(s) for this provider
            providerSettings.forEach(s => {
                const field = this._buildProviderField(s);
                providerRow.appendChild(field);
            });

            apiKeysContainer.appendChild(providerRow);
        }

        panel.appendChild(apiKeysContainer);

        // Use requestAnimationFrame to ensure DOM is fully rendered before initializing dropdowns
        requestAnimationFrame(() => {
            this._initModelDropdowns();
            this._updateModelDropdowns();
        });
    }

    _buildSearchSection(panel, settings) {
        // Create wizard-panel-header matching setup.ejs
        const panelHeader = document.createElement('div');
        panelHeader.className = 'wizard-panel-header';

        const headerIcon = document.createElement('span');
        headerIcon.className = 'material-symbols-outlined wizard-panel-icon';
        headerIcon.textContent = 'travel_explore';

        const headerDiv = document.createElement('div');
        const headerTitle = document.createElement('h2');
        headerTitle.className = 'wizard-panel-title';
        headerTitle.textContent = 'Vertex Search';
        headerDiv.appendChild(headerTitle);

        panelHeader.appendChild(headerIcon);
        panelHeader.appendChild(headerDiv);
        panel.appendChild(panelHeader);

        // Create setup-fields container
        const setupFields = document.createElement('div');
        setupFields.className = 'setup-fields';

        // Create setup-fields-row
        const fieldsRow = document.createElement('div');
        fieldsRow.className = 'setup-fields-row';

        // Build each search field
        settings.forEach(setting => {
            if (setting.key === 'vertex_search_enabled') {
                // Add the enabled dropdown at the top before the row
                const field = this._buildSearchField(setting);
                fieldsRow.insertBefore(field, fieldsRow.firstChild);
            } else if (setting.key === 'vertex_search_location') {
                // Skip location field as it's not in setup.ejs
            } else {
                const field = this._buildSearchField(setting);
                fieldsRow.appendChild(field);
            }
        });

        setupFields.appendChild(fieldsRow);
        panel.appendChild(setupFields);
    }

    _buildSearchField(setting) {
        const field = document.createElement('div');
        field.className = 'setup-field';

        const label = document.createElement('label');
        label.className = 'setup-label';
        label.setAttribute('for', setting.key);
        label.textContent = setting.label;
        field.appendChild(label);

        if (setting.type === 'password') {
            const inputWrap = document.createElement('div');
            inputWrap.className = 'setup-input-wrap';

            const input = document.createElement('input');
            input.type = 'password';
            input.id = setting.key;
            input.name = setting.key;
            input.className = 'setup-input has-toggle';
            input.placeholder = setting.placeholder || '';
            input.autocomplete = 'new-password';
            input.spellcheck = false;
            input.setAttribute('data-key', setting.key);

            if (setting.value) {
                input.setAttribute('data-real-value', setting.value);
                input.value = '*'.repeat(Math.min(setting.value.length, 32));
            } else {
                input.value = '';
            }

            inputWrap.appendChild(input);

            const revealBtn = document.createElement('button');
            revealBtn.type = 'button';
            revealBtn.className = 'setup-reveal-btn';
            revealBtn.ariaLabel = 'Toggle visibility';
            revealBtn.setAttribute('data-for', setting.key);

            const revealIcon = document.createElement('span');
            revealIcon.className = 'material-symbols-outlined';
            revealIcon.textContent = 'visibility';
            revealBtn.appendChild(revealIcon);

            inputWrap.appendChild(revealBtn);
            field.appendChild(inputWrap);

            const handleInputChange = () => {
                this._markDirty(setting.key, input.value);
            };

            input.addEventListener('input', handleInputChange);
            input.addEventListener('change', handleInputChange);

            revealBtn.addEventListener('click', () => {
                const isHidden = input.type === 'password';
                input.type = isHidden ? 'text' : 'password';

                if (isHidden) {
                    const realValue = input.getAttribute('data-real-value');
                    if (realValue) {
                        input.setAttribute('data-obfuscated-value', input.value);
                        input.value = realValue;
                    }
                } else {
                    const obfuscatedValue = input.getAttribute('data-obfuscated-value');
                    if (obfuscatedValue) {
                        input.value = obfuscatedValue;
                    }
                }

                revealIcon.textContent = isHidden ? 'visibility_off' : 'visibility';
            });
        } else if (setting.type === 'select' && setting.options) {
            const select = document.createElement('select');
            select.id = setting.key;
            select.name = setting.key;
            select.className = 'setup-input';
            select.setAttribute('data-key', setting.key);

            setting.options.forEach(opt => {
                const option = document.createElement('option');
                option.value = String(opt.value);
                option.textContent = opt.label;
                if (setting.value === opt.value) {
                    option.selected = true;
                }
                select.appendChild(option);
            });

            field.appendChild(select);

            const handleInputChange = () => {
                const selectedOption = select.options[select.selectedIndex];
                this._markDirty(setting.key, selectedOption.value);
            };

            select.addEventListener('change', handleInputChange);
        } else {
            const input = document.createElement('input');
            input.type = 'text';
            input.id = setting.key;
            input.name = setting.key;
            input.className = 'setup-input';
            input.placeholder = setting.placeholder || '';
            input.spellcheck = false;
            input.autocomplete = 'off';
            input.value = setting.value || '';
            input.setAttribute('data-key', setting.key);

            field.appendChild(input);

            const handleInputChange = () => {
                this._markDirty(setting.key, input.value);
            };

            input.addEventListener('input', handleInputChange);
            input.addEventListener('change', handleInputChange);
        }

        return field;
    }

    _buildProviderField(setting) {
        const field = document.createElement('div');
        field.className = 'setup-field';

        if (setting.type === 'password') {
            const inputWrap = document.createElement('div');
            inputWrap.className = 'setup-input-wrap';

            const input = document.createElement('input');
            input.type = 'password';
            input.id = setting.key;
            input.name = setting.key;
            input.className = 'setup-input has-toggle';
            input.placeholder = setting.placeholder || '';
            input.autocomplete = 'new-password';
            input.spellcheck = false;
            input.setAttribute('data-key', setting.key);

            if (setting.value) {
                input.setAttribute('data-real-value', setting.value);
                input.value = '*'.repeat(Math.min(setting.value.length, 32));
            } else {
                input.value = '';
            }

            inputWrap.appendChild(input);

            const revealBtn = document.createElement('button');
            revealBtn.type = 'button';
            revealBtn.className = 'setup-reveal-btn';
            revealBtn.ariaLabel = 'Toggle visibility';
            revealBtn.setAttribute('data-for', setting.key);

            const revealIcon = document.createElement('span');
            revealIcon.className = 'material-symbols-outlined';
            revealIcon.textContent = 'visibility';
            revealBtn.appendChild(revealIcon);

            inputWrap.appendChild(revealBtn);
            field.appendChild(inputWrap);

            // Event listeners
            const handleInputChange = () => {
                this._markDirty(setting.key, input.value);
                if (setting.provider) {
                    this.lastProviderEdited = setting.provider;
                    this._validateApiKey(setting.provider, input);
                    this._updateModelDropdowns();
                }
            };

            input.addEventListener('input', handleInputChange);
            input.addEventListener('change', handleInputChange);

            revealBtn.addEventListener('click', () => {
                const isHidden = input.type === 'password';
                input.type = isHidden ? 'text' : 'password';

                if (isHidden) {
                    const realValue = input.getAttribute('data-real-value');
                    if (realValue) {
                        input.setAttribute('data-obfuscated-value', input.value);
                        input.value = realValue;
                    }
                } else {
                    const obfuscatedValue = input.getAttribute('data-obfuscated-value');
                    if (obfuscatedValue) {
                        input.value = obfuscatedValue;
                    }
                }

                revealIcon.textContent = isHidden ? 'visibility_off' : 'visibility';
            });
        } else {
            const input = document.createElement('input');
            input.type = 'text';
            input.id = setting.key;
            input.name = setting.key;
            input.className = 'setup-input';
            input.placeholder = setting.placeholder || '';
            input.spellcheck = false;
            input.autocomplete = 'off';
            input.value = setting.value || '';
            input.setAttribute('data-key', setting.key);

            field.appendChild(input);

            const handleInputChange = () => {
                this._markDirty(setting.key, input.value);
                if (setting.provider) {
                    this.lastProviderEdited = setting.provider;
                    this._updateModelDropdowns();
                }
            };

            input.addEventListener('input', handleInputChange);
            input.addEventListener('change', handleInputChange);
        }

        return field;
    }

    _buildModelDropdownField(role, label, iconName) {
        const field = document.createElement('div');
        field.className = 'setup-field';

        const labelEl = document.createElement('label');
        labelEl.className = 'setup-label';
        labelEl.setAttribute('for', `${role}_model`);
        labelEl.textContent = label;
        field.appendChild(labelEl);

        const dropdown = document.createElement('div');
        dropdown.id = `${role}_model`;
        dropdown.className = 'llm-model-dropdown custom-dropdown setup-model-dropdown';
        dropdown.tabIndex = 0;
        dropdown.setAttribute('role', 'combobox');
        dropdown.setAttribute('aria-expanded', 'false');

        const selected = document.createElement('div');
        selected.className = 'llm-model-dropdown__selected';

        const icon = document.createElement('span');
        icon.className = 'llm-model-icon material-symbols-outlined';
        icon.textContent = iconName;

        const text = document.createElement('span');
        text.className = 'llm-model-dropdown__text';
        text.textContent = EMPTY_MODEL_PLACEHOLDER;

        const arrow = document.createElement('span');
        arrow.className = 'llm-model-dropdown__arrow material-symbols-outlined';
        arrow.textContent = 'expand_more';

        selected.appendChild(icon);
        selected.appendChild(text);
        selected.appendChild(arrow);

        const menu = document.createElement('div');
        menu.className = 'llm-model-dropdown__menu';
        menu.id = `${role}_model-menu`;

        dropdown.appendChild(selected);
        dropdown.appendChild(menu);
        field.appendChild(dropdown);

        return field;
    }

    _buildAdvancedSection(panel) {
        const template = document.getElementById('advanced-section-template');
        if (!template) return;
        const content = template.content.cloneNode(true);
        panel.appendChild(content);
        this._initDevLogsToggle();
    }

    _buildField(setting) {
        const wrap = document.createElement('div');
        wrap.className = 'settings-field';

        const label = document.createElement('div');
        label.className = 'settings-field-label';
        label.textContent = setting.label;
        wrap.appendChild(label);

        const desc = document.createElement('div');
        desc.className = 'settings-field-desc';
        desc.textContent = setting.description;
        wrap.appendChild(desc);

        let inputEl;

        if (setting.type === 'toggle') {
            const label = document.createElement('label');
            label.className = 'settings-toggle';

            inputEl = document.createElement('input');
            inputEl.type = 'checkbox';
            inputEl.setAttribute('data-key', setting.key);
            inputEl.checked = setting.value === true || setting.value === 'true';

            const track = document.createElement('span');
            track.className = 'settings-toggle-track';
            const thumb = document.createElement('span');
            thumb.className = 'settings-toggle-thumb';
            track.appendChild(thumb);

            const toggleLabel = document.createElement('span');
            toggleLabel.className = 'settings-toggle-label';
            toggleLabel.textContent = inputEl.checked ? 'Enabled' : 'Disabled';

            label.appendChild(inputEl);
            label.appendChild(track);
            label.appendChild(toggleLabel);
            wrap.appendChild(label);

            inputEl.addEventListener('change', () => {
                toggleLabel.textContent = inputEl.checked ? 'Enabled' : 'Disabled';
                this._markDirty(setting.key, inputEl.checked);
            });
        } else if (setting.type === 'select' && setting.options) {
            inputEl = document.createElement('select');
            inputEl.className = 'settings-select';
            inputEl.setAttribute('data-key', setting.key);
            setting.options.forEach(opt => {
                const option = document.createElement('option');
                option.value = String(opt.value);
                option.textContent = opt.label;
                if (setting.value === opt.value) {
                    option.selected = true;
                }
                inputEl.appendChild(option);
            });
            wrap.appendChild(inputEl);
        } else if (setting.type === 'password') {
            const inputWrap = document.createElement('div');
            inputWrap.className = 'settings-input-wrap';

            inputEl = document.createElement('input');
            inputEl.type = 'password';
            inputEl.className = 'settings-input has-toggle';
            inputEl.setAttribute('data-key', setting.key);
            inputEl.placeholder = setting.placeholder || '';
            inputEl.autocomplete = 'new-password';
            
            if (setting.value) {
                inputEl.setAttribute('data-real-value', setting.value);
                inputEl.value = '*'.repeat(Math.min(setting.value.length, 32));
            } else {
                inputEl.value = '';
            }
            
            inputWrap.appendChild(inputEl);

            const revealBtn = document.createElement('button');
            revealBtn.className = 'settings-reveal-btn';
            revealBtn.type = 'button';
            revealBtn.ariaLabel = 'Toggle visibility';
            revealBtn.setAttribute('data-for', setting.key);
            const revealIcon = document.createElement('span');
            revealIcon.className = 'material-symbols-outlined';
            revealIcon.textContent = 'visibility';
            revealBtn.appendChild(revealIcon);
            inputWrap.appendChild(revealBtn);

            wrap.appendChild(inputWrap);
        } else {
            inputEl = document.createElement('input');
            inputEl.type = 'text';
            inputEl.className = 'settings-input';
            inputEl.setAttribute('data-key', setting.key);
            inputEl.placeholder = setting.placeholder || '';
            inputEl.value = setting.value || '';
            wrap.appendChild(inputEl);
        }

        if (true) {
            const input = wrap.querySelector('[data-key]');
            if (input) {
                const resolveValue = () => {
                    const raw = input.value;
                    if (setting.type === 'select' && setting.options) {
                        const match = setting.options.find(o => String(o.value) === raw);
                        return match ? match.value : raw;
                    }
                    if (setting.type === 'password') {
                        input.setAttribute('data-real-value', raw);
                    }
                    return raw;
                };
                
                // Track provider key changes for auto-selection of default models
                const handleInputChange = () => {
                    this._markDirty(setting.key, resolveValue());
                    
                    // Check if this is a provider API key field
                    if (setting.provider && (setting.key.includes('api_key') || setting.key.includes('endpoint'))) {
                        this.lastProviderEdited = setting.provider;
                        this._validateApiKey(setting.provider, input);
                        this._updateModelDropdowns();
                    }
                };
                
                input.addEventListener('input', handleInputChange);
                input.addEventListener('change', handleInputChange);
            }

            const revealBtn = wrap.querySelector('.settings-reveal-btn');
            if (revealBtn) {
                revealBtn.addEventListener('click', () => {
                    const inp = wrap.querySelector('.settings-input');
                    if (!inp) return;
                    const isHidden = inp.type === 'password';
                    inp.type = isHidden ? 'text' : 'password';
                    
                    if (isHidden) {
                        const realValue = inp.getAttribute('data-real-value');
                        if (realValue) {
                            inp.setAttribute('data-obfuscated-value', inp.value);
                            inp.value = realValue;
                        }
                    } else {
                        const obfuscatedValue = inp.getAttribute('data-obfuscated-value');
                        if (obfuscatedValue) {
                            inp.value = obfuscatedValue;
                        }
                    }
                    
                    revealBtn.querySelector('.material-symbols-outlined').textContent =
                        isHidden ? 'visibility_off' : 'visibility';
                });
            }
        }

        return wrap;
    }

    async _loadSettings() {
        document.getElementById('settings-loading').style.display = 'flex';
        document.getElementById('settings-body').style.display = 'none';

        try {
            const res = await fetch(ApiPaths.settings.list(), { credentials: 'include' });
            if (res.status === 401 || res.status === 403) {
                this._showStatus('error', 'Access denied. Admin role required.');
                document.getElementById('settings-loading').style.display = 'none';
                return;
            }
            if (!res.ok) throw new Error(`HTTP ${res.status}`);

            const json = await res.json();
            if (!json.success) throw new Error(json.error || 'Load failed');

            this.allSettings = json.settings;
            this.sections    = json.sections;

            this._buildNav();
            this._buildSections();

            if (this.sections.length > 0) {
                this.activeSection = this.sections[0].id;
            }

            document.getElementById('settings-loading').style.display = 'none';
            document.getElementById('settings-body').style.display = 'block';
        } catch (err) {
            document.getElementById('settings-loading').style.display = 'none';
            this._showStatus('error', 'Failed to load settings: ' + err.message);
        }
    }

    async _saveSettings() {
        if (this.dirty.size === 0) return;

        const btn = document.getElementById('save-btn');
        btn.disabled = true;
        btn.textContent = '';
        const icon = document.createElement('span');
        icon.className = 'material-symbols-outlined spin';
        icon.textContent = 'sync';
        btn.appendChild(icon);
        btn.appendChild(document.createTextNode(' Saving...'));

        const updates = {};
        this.dirty.forEach((val, key) => { updates[key] = val; });

        // Derive providers from selected models
        const roleModelMap = {
            'llm_model': 'llm_primary_provider',
            'llm_assistant_model': 'llm_assistant_provider',
            'llm_lite_model': 'llm_lite_provider'
        };

        for (const [modelKey, providerKey] of Object.entries(roleModelMap)) {
            if (updates[modelKey]) {
                const provider = _modelToProvider(updates[modelKey], this.PROVIDER_MODELS);
                if (provider) {
                    updates[providerKey] = provider;
                }
            }
        }

        try {
            const res = await fetch(ApiPaths.settings.save(), {
                method: 'PUT',
                credentials: 'include',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ settings: updates }),
            });

            const json = await res.json();

            if (!res.ok || !json.success) {
                throw new Error(json.error || `HTTP ${res.status}`);
            }

            this.dirty.clear();

            const skippedNote = json.skipped?.length
                ? ` (${json.skipped.length} env-locked key(s) skipped)`
                : '';

            this._showStatus('success', `Settings saved successfully.${skippedNote} Changes are applied immediately.`);
        } catch (err) {
            this._showStatus('error', 'Save failed: ' + err.message);
            btn.disabled = false;
        } finally {
            btn.textContent = '';
            const icon = document.createElement('span');
            icon.className = 'material-symbols-outlined';
            icon.textContent = 'save';
            btn.appendChild(icon);
            btn.appendChild(document.createTextNode(' Save Changes'));
        }
    }

    _initDevLogsToggle() {
        const toggle = document.getElementById('dev-logs-toggle');
        const label  = document.getElementById('dev-logs-label');
        if (!toggle) return;

        toggle.checked = window.__DEV_LOGS_ENABLED === true;
        label.textContent = window.__DEV_LOGS_ENABLED === true ? 'Enabled' : 'Disabled';

        toggle.addEventListener('change', async () => {
            const enabled = toggle.checked;
            toggle.disabled = true;
            try {
                const res = await fetch(ApiPaths.user.devLogs(), {
                    method: 'PATCH',
                    credentials: 'include',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ enabled }),
                });
                const json = await res.json();
                if (!res.ok || !json.success) {
                    toggle.checked = !enabled;
                    this._showStatus('error', 'Failed to update dev logging: ' + (json.error || `HTTP ${res.status}`));
                } else {
                    label.textContent = enabled ? 'Enabled' : 'Disabled';
                    this._showStatus('success', 'Dev logging ' + (enabled ? 'enabled' : 'disabled') + '. Reload any open page to apply.');
                }
            } catch (err) {
                toggle.checked = !enabled;
                this._showStatus('error', 'Failed to update dev logging: ' + err.message);
            } finally {
                toggle.disabled = false;
            }
        });
    }
}

// Auto-initialization removed to support unit testing
// Consumer should call init() explicitly when DOM is ready:
// const page = new SettingsPage();
// page.init();
