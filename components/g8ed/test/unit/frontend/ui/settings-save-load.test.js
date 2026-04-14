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
import { LLMProvider } from '@g8ed/constants/ai.js';

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
        </body>
        </html>
    `, { url: 'https://localhost/settings', runScripts: 'dangerously' });
}

// Inject the settings page IIFE with save/load functionality
function injectScript(dom, fetchMock) {
    const { window } = dom;
    window.fetch = fetchMock;

    const script = `
    (function () {
        const API = '/api/settings';

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

            return wrap;
        }

        async function loadSettings() {
            document.getElementById('settings-loading').style.display = 'flex';
            document.getElementById('settings-body').style.display = 'none';

            try {
                const res = await fetch(API, { credentials: 'include' });
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

        async function saveSettings() {
            if (dirty.size === 0) return;

            const btn = document.getElementById('save-btn');
            btn.disabled = true;
            btn.innerHTML = '<span class="material-symbols-outlined spin">sync</span> Saving...';

            const updates = {};
            dirty.forEach((val, key) => { updates[key] = val; });

            try {
                const res = await fetch(API, {
                    method: 'PUT',
                    credentials: 'include',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ settings: updates }),
                });

                const json = await res.json();

                if (!res.ok || !json.success) {
                    throw new Error(json.error || 'HTTP ' + res.status);
                }

                dirty.clear();

                const skippedNote = json.skipped && json.skipped.length
                    ? ' (' + json.skipped.length + ' write-once key(s) skipped)'
                    : '';

                showStatus('success', 'Settings saved successfully.' + skippedNote + ' Restart the platform to apply changes.');
            } catch (err) {
                showStatus('error', 'Save failed: ' + err.message);
                btn.disabled = false;
            } finally {
                btn.innerHTML = '<span class="material-symbols-outlined">save</span> Save Changes';
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

        document.getElementById('save-btn').addEventListener('click', saveSettings);

        window.__settingsTest = {
            loadSettings,
            saveSettings,
            markDirty,
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

describe('Settings Save/Load Operations [UNIT - jsdom]', () => {
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

    describe('dirty tracking', () => {
        it('save button starts disabled', () => {
            const btn = document.getElementById('save-btn');
            expect(btn.disabled).toBe(true);
        });

        it('input event on text field enables save button', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'https://new.example.com';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            expect(document.getElementById('save-btn').disabled).toBe(false);
        });

        it('change event on select field enables save button', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const sel = document.querySelector('select[data-key="provider"]');
            sel.value = LLMProvider.OLLAMA;
            sel.dispatchEvent(new window.Event('change', { bubbles: true }));

            expect(document.getElementById('save-btn').disabled).toBe(false);
        });

        it('dirty map contains changed key with new value after input', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'https://updated.example.com';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            const dirty = window.__settingsTest.getDirty();
            expect(dirty.get('app_url')).toBe('https://updated.example.com');
        });

        it('multiple changes tracked independently in dirty map', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const appUrlInput = document.querySelector('input[data-key="app_url"]');
            appUrlInput.value = 'https://first.example.com';
            appUrlInput.dispatchEvent(new window.Event('input', { bubbles: true }));

            const sel = document.querySelector('select[data-key="provider"]');
            sel.value = LLMProvider.OLLAMA;
            sel.dispatchEvent(new window.Event('change', { bubbles: true }));

            const dirty = window.__settingsTest.getDirty();
            expect(dirty.size).toBe(2);
            expect(dirty.get('app_url')).toBe('https://first.example.com');
            expect(dirty.get('provider')).toBe(LLMProvider.OLLAMA);
        });

        it('changing a field clears the status bar', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            // First show a status
            const bar = document.getElementById('status-bar');
            bar.className = 'settings-status visible success';

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'https://new.example.com';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            expect(bar.className).toBe('settings-status');
        });
    });

    describe('saveSettings — success', () => {
        it('does nothing when dirty map is empty', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();
            fetchMock.mockClear();

            await window.__settingsTest.saveSettings();
            // fetch should NOT have been called for the PUT
            expect(fetchMock).not.toHaveBeenCalled();
        });

        it('sends PUT to /api/settings with dirty keys as body', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'https://saved.example.com';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            fetchMock.mockResolvedValue({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ success: true, saved: ['app_url'], skipped: [] }),
            });

            await window.__settingsTest.saveSettings();

            const putCall = fetchMock.mock.calls.find(c => c[1]?.method === 'PUT');
            expect(putCall).toBeDefined();
            const body = JSON.parse(putCall[1].body);
            expect(body.settings.app_url).toBe('https://saved.example.com');
        });

        it('clears dirty map on successful save', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'https://saved.example.com';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            fetchMock.mockResolvedValue({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ success: true, saved: ['app_url'], skipped: [] }),
            });

            await window.__settingsTest.saveSettings();
            expect(window.__settingsTest.getDirty().size).toBe(0);
        });

        it('shows success status after save', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'https://saved.example.com';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            fetchMock.mockResolvedValue({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ success: true, saved: ['app_url'], skipped: [] }),
            });

            await window.__settingsTest.saveSettings();
            const bar = document.getElementById('status-bar');
            expect(bar.className).toContain('success');
            expect(document.getElementById('status-msg').textContent).toContain('Settings saved successfully');
        });

        it('notes skipped write-once keys in success message', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'test';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            fetchMock.mockResolvedValue({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ success: true, saved: [], skipped: ['app_url'] }),
            });

            await window.__settingsTest.saveSettings();
            expect(document.getElementById('status-msg').textContent).toContain('write-once key(s) skipped');
        });

        it('sends Content-Type: application/json header', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'test';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            fetchMock.mockResolvedValue({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ success: true, saved: ['app_url'], skipped: [] }),
            });

            await window.__settingsTest.saveSettings();

            const putCall = fetchMock.mock.calls.find(c => c[1]?.method === 'PUT');
            expect(putCall[1].headers['Content-Type']).toBe('application/json');
        });

        it('sends credentials:include on PUT', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'test';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            fetchMock.mockResolvedValue({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ success: true, saved: ['app_url'], skipped: [] }),
            });

            await window.__settingsTest.saveSettings();

            const putCall = fetchMock.mock.calls.find(c => c[1]?.method === 'PUT');
            expect(putCall[1].credentials).toBe('include');
        });

        it('button shows loading state during save', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'test';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            // Mock a delayed response
            fetchMock.mockImplementation(() => new Promise(resolve => 
                setTimeout(() => resolve({
                    ok: true,
                    status: 200,
                    json: () => Promise.resolve({ success: true, saved: ['app_url'], skipped: [] }),
                }), 100)
            ));

            const savePromise = window.__settingsTest.saveSettings();
            
            // Button should be in loading state
            const btn = document.getElementById('save-btn');
            expect(btn.disabled).toBe(true);
            expect(btn.innerHTML).toContain('Saving...');
            expect(btn.innerHTML).toContain('sync');

            await savePromise;
        });

        it('button returns to normal state after successful save', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'test';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            fetchMock.mockResolvedValue({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ success: true, saved: ['app_url'], skipped: [] }),
            });

            await window.__settingsTest.saveSettings();

            const btn = document.getElementById('save-btn');
            expect(btn.innerHTML).toContain('Save Changes');
            expect(btn.innerHTML).toContain('save');
        });
    });

    describe('saveSettings — error handling', () => {
        async function setupDirtyAndSave(putResponse) {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'test';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            fetchMock.mockResolvedValue(putResponse);
            await window.__settingsTest.saveSettings();
        }

        it('shows error status on non-ok HTTP response', async () => {
            await setupDirtyAndSave({
                ok: false,
                status: 500,
                json: () => Promise.resolve({ success: false, error: 'Internal error' }),
            });
            expect(document.getElementById('status-bar').className).toContain('error');
            expect(document.getElementById('status-msg').textContent).toContain('Save failed');
        });

        it('re-enables save button on error', async () => {
            await setupDirtyAndSave({
                ok: false,
                status: 500,
                json: () => Promise.resolve({ success: false, error: 'Internal error' }),
            });
            expect(document.getElementById('save-btn').disabled).toBe(false);
        });

        it('shows error status on json.success false', async () => {
            await setupDirtyAndSave({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ success: false, error: 'Validation failed' }),
            });
            expect(document.getElementById('status-bar').className).toContain('error');
        });

        it('shows error status on network failure', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'test';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            fetchMock.mockRejectedValue(new Error('Network failure'));
            await window.__settingsTest.saveSettings();

            expect(document.getElementById('status-bar').className).toContain('error');
            expect(document.getElementById('status-msg').textContent).toContain('Save failed');
        });

        it('does not clear dirty map on error', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'test';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            fetchMock.mockResolvedValue({
                ok: false,
                status: 500,
                json: () => Promise.resolve({ success: false, error: 'Error' }),
            });
            await window.__settingsTest.saveSettings();

            expect(window.__settingsTest.getDirty().size).toBeGreaterThan(0);
        });

        it('restores Save Changes button label after error', async () => {
            await setupDirtyAndSave({
                ok: false,
                status: 500,
                json: () => Promise.resolve({ success: false, error: 'Error' }),
            });
            expect(document.getElementById('save-btn').innerHTML).toContain('Save Changes');
        });
    });

    describe('save button click handler', () => {
        beforeEach(() => { vi.useFakeTimers(); });
        afterEach(() => { vi.useRealTimers(); });

        it('clicking save button when dirty triggers PUT', async () => {
            fetchMock.mockResolvedValue(makeSuccessResponse());
            await window.__settingsTest.loadSettings();

            const input = document.querySelector('input[data-key="app_url"]');
            input.value = 'https://click-save.example.com';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            fetchMock.mockResolvedValue({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ success: true, saved: ['app_url'], skipped: [] }),
            });

            document.getElementById('save-btn').click();
            await vi.runAllTimersAsync();

            const putCall = fetchMock.mock.calls.find(c => c[1]?.method === 'PUT');
            expect(putCall).toBeDefined();
        });
    });
});
