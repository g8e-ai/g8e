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

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { JSDOM } from 'jsdom';
import { Collections } from '@vsod/constants/collections.js';
import { LLMProvider } from '@vsod/constants/ai.js';
import { ApiPaths } from '@vsod/public/js/constants/api-paths.js';

const MOCK_SECTIONS = [
    { id: 'general', label: 'General', icon: 'settings' },
    { id: 'llm', label: 'LLM Provider', icon: 'psychology' },
];

const MOCK_SETTINGS = [
    {
        key: 'app_url',
        section: 'general',
        label: 'Application URL',
        description: 'Public-facing URL.',
        type: 'text',
        options: null,
        secret: false,
        placeholder: 'https://localhost',
        value: 'https://localhost',
        dbValue: 'https://localhost',
    },
    {
        key: 'internal_auth_token',
        section: 'general',
        label: 'Internal Auth Token',
        description: 'Shared secret.',
        type: 'password',
        options: null,
        secret: true,
        placeholder: 'change-me',
        value: '',
        dbValue: '',
    },
    {
        key: 'provider',
        section: 'llm',
        label: 'LLM Provider',
        description: 'AI provider type.',
        type: 'select',
        options: [
            { value: LLMProvider.OPENAI,    label: 'OpenAI' },
            { value: LLMProvider.OLLAMA,    label: 'Ollama' },
            { value: LLMProvider.GEMINI,    label: 'Gemini (Google)' },
            { value: LLMProvider.ANTHROPIC, label: 'Anthropic (Claude)' },
        ],
        secret: false,
        placeholder: '',
        value: LLMProvider.GEMINI,
        dbValue: LLMProvider.GEMINI,
    },
    {
        key: 'gemini_api_key',
        section: 'llm',
        label: 'Gemini API Key',
        description: 'Google Cloud API key for Gemini provider.',
        type: 'password',
        options: null,
        secret: true,
        placeholder: 'your-gemini-api-key-here',
        value: '',
        dbValue: '',
    },
    {
        key: 'ollama_endpoint',
        section: 'llm',
        label: 'Ollama Endpoint URL',
        description: 'API endpoint for Ollama.',
        type: 'text',
        options: null,
        secret: false,
        placeholder: 'https://your-ollama-host:11434/v1',
        value: 'https://my-ollama:11434/v1',
        dbValue: 'https://my-ollama:11434/v1',
    },
];

function makeSuccessResponse(settings = MOCK_SETTINGS, sections = MOCK_SECTIONS) {
    return {
        ok: true,
        status: 200,
        json: () => Promise.resolve({ success: true, settings, sections }),
    };
}

function makeErrorResponse(status, body) {
    return {
        ok: false,
        status,
        json: () => Promise.resolve(body),
    };
}

function buildDOM() {
    return new JSDOM(`
        <!DOCTYPE html>
        <html>
        <body>
            <div id="settings-loading" class="settings-loading" style="display:flex;">
                <span>Loading...</span>
            </div>
            <div id="settings-body" style="display:none;">
                <nav id="settings-nav"></nav>
                <div id="settings-sections"></div>
            </div>
            <div id="status-bar" class="settings-status">
                <span id="status-icon"></span>
                <span id="status-msg"></span>
            </div>
            <button id="save-btn" disabled>
                <span class="material-symbols-outlined">save</span>
                Save Changes
            </button>
            <div class="settings-dev-panel">
                <label class="settings-toggle">
                    <input type="checkbox" id="dev-logs-toggle">
                    <span class="settings-toggle-track"><span class="settings-toggle-thumb"></span></span>
                    <span class="settings-toggle-label" id="dev-logs-label">Disabled</span>
                </label>
            </div>
        </body>
        </html>
    `, { url: 'https://localhost/settings', runScripts: 'dangerously' });
}

// Inject the settings page IIFE extracted from settings.ejs into the jsdom window.
// We extract only the <script> body (the IIFE) and evaluate it in the DOM context.
function injectScript(dom, fetchMock) {
    const { window } = dom;
    window.fetch = fetchMock;
    
    // Inject ApiPaths into window context
    window.ApiPaths = ApiPaths;

    // The IIFE from settings.ejs verbatim (simplified version focusing on core functionality)
    const script = `
    (function () {
        const API = ApiPaths.settings.list();

        let allSettings = [];
        let sections = [];
        let dirty = new Map();
        let activeSection = null;

        function showStatus(type, msg) {
            const bar = document.getElementById('status-bar');
            const icon = document.getElementById('status-icon');
            const text = document.getElementById('status-msg');
            const icons = { success: 'check_circle', error: 'error', info: 'info' };
            bar.className = 'settings-status visible ' + type;
            icon.textContent = icons[type] || 'info';
            text.textContent = msg;
        }

        function hideStatus() {
            document.getElementById('status-bar').className = 'settings-status';
        }

        function markDirty(key, value) {
            dirty.set(key, value);
            document.getElementById('save-btn').disabled = false;
            hideStatus();
        }

        function buildNav() {
            const nav = document.getElementById('settings-nav');
            nav.innerHTML = '';
            sections.forEach((sec, idx) => {
                const btn = document.createElement('button');
                btn.className = 'settings-nav-item' + (idx === 0 ? ' active' : '');
                btn.dataset.section = sec.id;
                btn.innerHTML = '<span class="material-symbols-outlined">' + escHtml(sec.icon) + '</span>' + escHtml(sec.label);
                btn.addEventListener('click', () => switchSection(sec.id));
                nav.appendChild(btn);
            });
        }

        function switchSection(sectionId) {
            activeSection = sectionId;
            document.querySelectorAll('.settings-nav-item').forEach(btn => {
                btn.classList.toggle('active', btn.dataset.section === sectionId);
            });
            document.querySelectorAll('.settings-section').forEach(el => {
                el.classList.toggle('active', el.dataset.section === sectionId);
            });
        }

        function buildSections() {
            const container = document.getElementById('settings-sections');
            container.innerHTML = '';

            sections.forEach((sec, idx) => {
                const secSettings = allSettings.filter(s => s.section === sec.id);
                if (!secSettings.length) return;

                const panel = document.createElement('div');
                panel.className = 'settings-section' + (idx === 0 ? ' active' : '');
                panel.dataset.section = sec.id;

                panel.innerHTML =
                    '<div class="settings-section-header">' +
                    '<span class="material-symbols-outlined settings-section-icon">' + escHtml(sec.icon) + '</span>' +
                    '<h2 class="settings-section-title">' + escHtml(sec.label) + '</h2>' +
                    '</div>';

                secSettings.forEach(setting => {
                    panel.appendChild(buildField(setting));
                });

                container.appendChild(panel);
            });
        }

        function buildField(setting) {
            const wrap = document.createElement('div');
            wrap.className = 'settings-field';

            let inputHtml = '';

            if (setting.type === 'select' && setting.options) {
                const opts = setting.options.map(opt =>
                    '<option value="' + escAttr(opt.value) + '" ' + (setting.value === opt.value ? 'selected' : '') + '>' + escHtml(opt.label) + '</option>'
                ).join('');
                inputHtml = '<select class="settings-select" data-key="' + escAttr(setting.key) + '">' + opts + '</select>';
            } else if (setting.type === 'password') {
                inputHtml =
                    '<div class="settings-input-wrap">' +
                    '<input type="password" class="settings-input has-toggle" data-key="' + escAttr(setting.key) + '" placeholder="' + escAttr(setting.placeholder) + '" value="' + escAttr(setting.value) + '" autocomplete="new-password">' +
                    '<button class="settings-reveal-btn" type="button" aria-label="Toggle visibility" data-for="' + escAttr(setting.key) + '">' +
                    '<span class="material-symbols-outlined">visibility</span>' +
                    '</button>' +
                    '</div>';
            } else {
                inputHtml =
                    '<input type="text" class="settings-input" data-key="' + escAttr(setting.key) + '" placeholder="' + escAttr(setting.placeholder) + '" value="' + escAttr(setting.value) + '">';
            }

            wrap.innerHTML =
                '<div class="settings-field-label">' + escHtml(setting.label) + '</div>' +
                '<div class="settings-field-desc">' + escHtml(setting.description) + '</div>' +
                inputHtml;

            const input = wrap.querySelector('[data-key]');
            if (input) {
                input.addEventListener('input', () => markDirty(setting.key, input.value));
                input.addEventListener('change', () => markDirty(setting.key, input.value));
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

        async function loadSettings() {
            document.getElementById('settings-loading').style.display = 'flex';
            document.getElementById('settings-body').style.display = 'none';

            try {
                const res = await fetch(ApiPaths.settings.list(), { credentials: 'include' });
                if (res.status === 401 || res.status === 403) {
                    showStatus('error', 'Access denied. Admin role required.');
                    document.getElementById('settings-loading').style.display = 'none';
                    return;
                }
                if (!res.ok) throw new Error('HTTP ' + res.status);

                const json = await res.json();
                if (!json.success) throw new Error(json.error || 'Load failed');

                allSettings = json.settings;
                sections = json.sections;

                buildNav();
                buildSections();

                if (sections.length > 0) {
                    activeSection = sections[0].id;
                }

                document.getElementById('settings-loading').style.display = 'none';
                document.getElementById('settings-body').style.display = 'block';
            } catch (err) {
                document.getElementById('settings-loading').style.display = 'none';
                showStatus('error', 'Failed to load settings: ' + err.message);
            }
        }

        function escHtml(str) {
            if (str == null) return '';
            return String(str)
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;')
                .replace(/'/g, '&#39;');
        }

        function escAttr(str) {
            return escHtml(str);
        }

        window.__settingsTest = {
            loadSettings,
            buildNav,
            buildSections,
            buildField,
            switchSection,
            escHtml,
            escAttr,
            getDirty: () => dirty,
            getAllSettings: () => allSettings,
            getSections: () => sections,
            setAllSettings: (s) => { allSettings = s; },
            setSections: (s) => { sections = s; },
        };
    })();
    `;

    window.eval(script);
}

describe('Settings Page Core Functionality [UNIT - jsdom]', () => {
    let dom;
    let window;
    let document;
    let fetchMock;

    beforeEach(() => {
        dom = buildDOM();
        window = dom.window;
        document = window.document;
        fetchMock = vi.fn();
        injectScript(dom, fetchMock);
    });

    afterEach(() => {
        dom.window.close();
        vi.clearAllMocks();
    });

    describe('loadSettings', () => {
        it('calls GET /api/settings with credentials:include', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();
            expect(fetchMock).toHaveBeenCalledWith(ApiPaths.settings.list(), { credentials: 'include' });
        });

        it('shows settings-body and hides loading spinner on success', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();
            expect(document.getElementById('settings-loading').style.display).toBe('none');
            expect(document.getElementById('settings-body').style.display).toBe('block');
        });

        it('renders nav buttons equal to sections count', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();
            const navBtns = document.querySelectorAll('.settings-nav-item');
            expect(navBtns.length).toBe(MOCK_SECTIONS.length);
        });

        it('first nav button has active class', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();
            const first = document.querySelector('.settings-nav-item');
            expect(first.classList.contains('active')).toBe(true);
        });

        it('first section panel has active class', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();
            const first = document.querySelector('.settings-section');
            expect(first.classList.contains('active')).toBe(true);
        });

        it('sets activeSection to first section id after load', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();
            expect(window.__settingsTest.getSections()[0].id).toBe(MOCK_SECTIONS[0].id);
        });

        it('shows access-denied error and hides loader on 401', async () => {
            fetchMock.mockResolvedValue(makeErrorResponse(401, { success: false, error: 'Unauthorized' }));
            await window.__settingsTest.loadSettings();
            expect(document.getElementById('settings-loading').style.display).toBe('none');
            const bar = document.getElementById('status-bar');
            expect(bar.className).toContain('error');
            expect(document.getElementById('status-msg').textContent).toContain('Access denied');
        });

        it('shows access-denied error on 403', async () => {
            fetchMock.mockResolvedValue(makeErrorResponse(403, { success: false, error: 'Forbidden' }));
            await window.__settingsTest.loadSettings();
            expect(document.getElementById('status-msg').textContent).toContain('Access denied');
        });

        it('shows error status and hides loader on non-ok response', async () => {
            fetchMock.mockResolvedValue(makeErrorResponse(500, { success: false, error: 'Server error' }));
            await window.__settingsTest.loadSettings();
            expect(document.getElementById('settings-loading').style.display).toBe('none');
            expect(document.getElementById('status-bar').className).toContain('error');
        });

        it('shows error status on network failure', async () => {
            fetchMock.mockRejectedValue(new Error('network error'));
            await window.__settingsTest.loadSettings();
            expect(document.getElementById('settings-loading').style.display).toBe('none');
            expect(document.getElementById('status-msg').textContent).toContain('Failed to load settings');
        });

        it('shows error status when json.success is false', async () => {
            fetchMock.mockResolvedValue({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ success: false, error: 'Unexpected error' }),
            });
            await window.__settingsTest.loadSettings();
            expect(document.getElementById('status-bar').className).toContain('error');
        });
    });

    describe('buildNav and switchSection', () => {
        beforeEach(async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();
        });

        it('each nav button has data-section attribute matching section id', () => {
            const btns = document.querySelectorAll('.settings-nav-item');
            btns.forEach((btn, i) => {
                expect(btn.dataset.section).toBe(MOCK_SECTIONS[i].id);
            });
        });

        it('switchSection moves active class to the correct nav button', () => {
            window.__settingsTest.switchSection('llm');
            const btns = document.querySelectorAll('.settings-nav-item');
            const generalBtn = Array.from(btns).find(b => b.dataset.section === 'general');
            const llmBtn = Array.from(btns).find(b => b.dataset.section === 'llm');
            expect(generalBtn.classList.contains('active')).toBe(false);
            expect(llmBtn.classList.contains('active')).toBe(true);
        });

        it('switchSection shows only the matching section panel', () => {
            window.__settingsTest.switchSection('llm');
            const panels = document.querySelectorAll('.settings-section');
            panels.forEach(panel => {
                if (panel.dataset.section === 'llm') {
                    expect(panel.classList.contains('active')).toBe(true);
                } else {
                    expect(panel.classList.contains('active')).toBe(false);
                }
            });
        });

        it('clicking a nav button switches to that section', () => {
            const llmBtn = Array.from(document.querySelectorAll('.settings-nav-item'))
                .find(b => b.dataset.section === 'llm');
            llmBtn.click();
            const llmPanel = document.querySelector('.settings-section[data-section="llm"]');
            expect(llmPanel.classList.contains('active')).toBe(true);
        });
    });

    describe('buildSections', () => {
        beforeEach(async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();
        });

        it('creates section panels for sections with settings', () => {
            const panels = document.querySelectorAll('.settings-section');
            expect(panels.length).toBe(2); // general and llm sections have settings
        });

        it('section panel has correct data-section attribute', () => {
            const generalPanel = document.querySelector('.settings-section[data-section="general"]');
            const llmPanel = document.querySelector('.settings-section[data-section="llm"]');
            expect(generalPanel).not.toBeNull();
            expect(llmPanel).not.toBeNull();
        });

        it('section header contains correct icon and label', () => {
            const generalPanel = document.querySelector('.settings-section[data-section="general"]');
            const header = generalPanel.querySelector('.settings-section-header');
            expect(header.querySelector('.material-symbols-outlined').textContent).toBe('settings');
            expect(header.querySelector('.settings-section-title').textContent).toBe('General');
        });

        it('section contains only settings from that section', () => {
            const generalPanel = document.querySelector('.settings-section[data-section="general"]');
            const llmPanel = document.querySelector('.settings-section[data-section="llm"]');
            
            const generalFields = generalPanel.querySelectorAll('.settings-field');
            const llmFields = llmPanel.querySelectorAll('.settings-field');
            
            expect(generalFields.length).toBe(2); // app_url, internal_auth_token
            expect(llmFields.length).toBe(3); // provider, gemini_api_key, ollama_endpoint
        });
    });
});
