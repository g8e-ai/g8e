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

export class SettingsPage {
    constructor() {
        this.allSettings = [];
        this.sections = [];
        this.dirty = new Map();
        this.activeSection = null;
    }

    init() {
        document.getElementById('save-btn').addEventListener('click', () => this._saveSettings());
        this._loadSettings();
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
            if (!secSettings.length) return;

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
        const providerSetting = settings.find(s => s.key === 'llm_provider');
        const universalSettings = settings.filter(s => s.group === 'universal');
        const providerSpecificSettings = settings.filter(s => s.provider);

        if (providerSetting) {
            const field = this._buildField(providerSetting);
            panel.appendChild(field);

            const select = field.querySelector('select');
            if (select) {
                select.addEventListener('change', () => {
                    this._updateLlmVisibility(panel, select.value);
                });
            }
        }

        const specificContainer = document.createElement('div');
        specificContainer.className = 'settings-llm-specific';
        providerSpecificSettings.forEach(s => {
            const field = this._buildField(s);
            field.setAttribute('data-provider', s.provider);
            
            // For unified keys, ensure inputs only update their specific provider context in the dirty map
            // though actually we want one key to rule them all now.
            if (s.key === 'llm_model' || s.key === 'llm_assistant_model') {
                field.classList.add('llm-model-field');
            }
            specificContainer.appendChild(field);
        });
        panel.appendChild(specificContainer);

        const divider = document.createElement('div');
        divider.className = 'settings-section-divider';
        const dividerText = document.createElement('div');
        dividerText.className = 'settings-divider-text';
        const icon = document.createElement('span');
        icon.className = 'material-symbols-outlined';
        icon.textContent = 'api';
        dividerText.appendChild(icon);
        dividerText.appendChild(document.createTextNode('LLM Controls & Safeguards'));
        divider.appendChild(dividerText);
        panel.appendChild(divider);

        universalSettings.forEach(s => {
            panel.appendChild(this._buildField(s));
        });

        const currentProvider = providerSetting ? (this.dirty.get('llm_provider') || providerSetting.value) : '';
        this._updateLlmVisibility(panel, currentProvider);
    }

    _updateLlmVisibility(panel, provider) {
        const specificFields = panel.querySelectorAll('.settings-llm-specific .settings-field');
        specificFields.forEach(field => {
            const fieldProvider = field.getAttribute('data-provider');
            field.style.display = (fieldProvider === provider) ? 'block' : 'none';
        });
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

        if (setting.type === 'select' && setting.options) {
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
            inputEl.value = setting.value || '';
            inputEl.autocomplete = 'new-password';
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
                    return raw;
                };
                input.addEventListener('input',  () => this._markDirty(setting.key, resolveValue()));
                input.addEventListener('change', () => this._markDirty(setting.key, resolveValue()));
            }

            const revealBtn = wrap.querySelector('.settings-reveal-btn');
            if (revealBtn) {
                revealBtn.addEventListener('click', () => {
                    const inp = wrap.querySelector('.settings-input');
                    if (!inp) return;
                    const isHidden = inp.type === 'password';
                    inp.type = isHidden ? 'text' : 'password';
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

            this._showStatus('success', `Settings saved successfully.${skippedNote} Restart the platform to apply changes.`);
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
