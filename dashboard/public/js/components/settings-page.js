// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

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
            btn.dataset.section = sec.id;
            btn.innerHTML = `<span class="material-symbols-outlined">${escHtml(sec.icon)}</span>${escHtml(sec.label)}`;
            btn.addEventListener('click', () => this._switchSection(sec.id));
            nav.appendChild(btn);
        });
    }

    _switchSection(sectionId) {
        this.activeSection = sectionId;
        document.querySelectorAll('.settings-nav-item').forEach(btn => {
            btn.classList.toggle('active', btn.dataset.section === sectionId);
        });
        document.querySelectorAll('.settings-section').forEach(el => {
            el.classList.toggle('active', el.dataset.section === sectionId);
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
            panel.dataset.section = sec.id;

            panel.innerHTML = `
                <div class="settings-section-header">
                    <span class="material-symbols-outlined settings-section-icon">${escHtml(sec.icon)}</span>
                    <h2 class="settings-section-title">${escHtml(sec.label)}</h2>
                </div>
            `;

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
            field.dataset.provider = s.provider;
            
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
        divider.innerHTML = `
            <div class="settings-divider-text">
                <span class="material-symbols-outlined">api</span>
                LLM Controls & Safeguards
            </div>
        `;
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
            const fieldProvider = field.dataset.provider;
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

        const envBadge = '';

        const envHint = '';

        let inputHtml;

        if (setting.type === 'select' && setting.options) {
            const opts = setting.options.map(opt =>
                `<option value="${escAttr(String(opt.value))}" ${setting.value === opt.value ? 'selected' : ''}>${escHtml(opt.label)}</option>`
            ).join('');
            inputHtml = `<select class="settings-select" data-key="${escAttr(setting.key)}">${opts}</select>`;
        } else if (setting.type === 'password') {
            inputHtml = `
                <div class="settings-input-wrap">
                    <input type="password"
                        class="settings-input has-toggle"
                        data-key="${escAttr(setting.key)}"
                        placeholder="${escAttr(setting.placeholder)}"
                        value="${escAttr(setting.value)}"
                        autocomplete="new-password">
                    <button class="settings-reveal-btn" type="button" aria-label="Toggle visibility" data-for="${escAttr(setting.key)}">
                        <span class="material-symbols-outlined">visibility</span>
                    </button>
                </div>`;
        } else {
            inputHtml = `
                <input type="text"
                    class="settings-input"
                    data-key="${escAttr(setting.key)}"
                    placeholder="${escAttr(setting.placeholder)}"
                    value="${escAttr(setting.value)}">`;
        }

        wrap.innerHTML = `
            <div class="settings-field-label">
                ${escHtml(setting.label)}
                ${envBadge}
            </div>
            <div class="settings-field-desc">${escHtml(setting.description)}</div>
            ${envHint}
            ${inputHtml}
        `;

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
        btn.innerHTML = '<span class="material-symbols-outlined spin">sync</span> Saving...';

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
            btn.innerHTML = '<span class="material-symbols-outlined">save</span> Save Changes';
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

const page = new SettingsPage();
page.init();
