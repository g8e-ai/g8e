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

const MOCK_SETTINGS = [
    {
        key: 'app_url',
        section: 'general',
        label: 'App URL',
        description: 'Public URL.',
        type: 'text',
        secret: false,
        placeholder: 'https://localhost',
        default: 'https://localhost',
        value: 'https://localhost',
        dbValue: 'https://localhost',
    },
    {
        key: 'passkey_rp_name',
        section: 'general',
        label: 'RP Name',
        description: 'Passkey name.',
        type: 'text',
        secret: false,
        placeholder: 'g8e',
        default: 'g8e',
        dbValue: 'g8e',
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
        key: 'temperature',
        section: 'llm',
        label: 'Temperature',
        description: 'LLM sampling temperature.',
        type: 'text',
        options: null,
        secret: false,
        placeholder: '1.0',
        value: '1.0',
        dbValue: '1.0',
    },
    {
        key: 'openai_api_key',
        section: 'llm',
        label: 'OpenAI API Key',
        description: 'API key.',
        type: 'password',
        secret: true,
        placeholder: 'sk-...',
        value: 'secret-key',
        dbValue: '',
    },
    {
        key: 'llm_command_gen_enabled',
        section: 'llm',
        label: 'Command Generation Enabled',
        description: 'Enable AI command generation.',
        type: 'select',
        group: 'universal',
        options: [
            { value: true,  label: 'Enabled' },
            { value: false, label: 'Disabled' },
        ],
        secret: false,
        placeholder: '',
        value: true,
        dbValue: true,
    },
    {
        key: 'google_search_enabled',
        section: 'search',
        label: 'Google Search Enabled',
        description: 'Enable search_web AI tool.',
        type: 'select',
        options: [
            { value: false, label: 'Disabled (default)' },
            { value: true, label: 'Enabled' },
        ],
        secret: false,
        placeholder: '',
        value: false,
        dbValue: false,
    },
];

function buildDOM() {
    return new JSDOM(`
        <!DOCTYPE html>
        <html>
        <body>
            <div id="test-container"></div>
        </body>
        </html>
    `, { url: 'https://localhost/settings', runScripts: 'dangerously' });
}

// Inject minimal field building functionality
function injectFieldScript(dom) {
    const { window } = dom;

    const script = `
    (function () {
        let dirty = new Map();

        function markDirty(key, value) {
            dirty.set(key, value);
        }

        function buildField(setting) {
            const wrap = document.createElement('div');
            wrap.className = 'settings-field';

            let inputHtml = '';

            if (setting.type === 'select' && setting.options) {
                const opts = setting.options.map(opt =>
                    '<option value="' + escAttr(String(opt.value)) + '" ' + (setting.value === opt.value ? 'selected' : '') + '>' + escHtml(opt.label) + '</option>'
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
                const resolveValue = () => {
                    const raw = input.value;
                    if (setting.type === 'select' && setting.options) {
                        const match = setting.options.find(o => String(o.value) === raw);
                        return match ? match.value : raw;
                    }
                    return raw;
                };
                input.addEventListener('input', () => markDirty(setting.key, resolveValue()));
                input.addEventListener('change', () => markDirty(setting.key, resolveValue()));
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

        window.__fieldTest = {
            buildField,
            escHtml,
            escAttr,
            getDirty: () => dirty,
        };
    })();
    `;

    window.eval(script);
}

describe('Settings Field Rendering [UNIT - jsdom]', () => {
    let dom;
    let window;
    let document;

    beforeEach(() => {
        dom = buildDOM();
        window = dom.window;
        document = window.document;
        injectFieldScript(dom);
    });

    afterEach(() => {
        dom.window.close();
        vi.clearAllMocks();
    });

    describe('buildField — text input', () => {
        it('renders an input[type=text] with correct data-key', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'app_url');
            const field = window.__fieldTest.buildField(setting);
            const input = field.querySelector('input[data-key="app_url"]');
            expect(input).not.toBeNull();
            expect(input.type).toBe('text');
        });

        it('sets the input value to the setting value', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'app_url');
            const field = window.__fieldTest.buildField(setting);
            const input = field.querySelector('input[data-key="app_url"]');
            expect(input.value).toBe('https://localhost');
        });

        it('renders the setting label', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'app_url');
            const field = window.__fieldTest.buildField(setting);
            expect(field.querySelector('.settings-field-label').textContent).toContain('App URL');
        });

        it('renders the setting description', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'app_url');
            const field = window.__fieldTest.buildField(setting);
            expect(field.querySelector('.settings-field-desc').textContent).toContain('Public URL');
        });

        it('sets placeholder attribute correctly', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'app_url');
            const field = window.__fieldTest.buildField(setting);
            const input = field.querySelector('input');
            expect(input.placeholder).toBe('https://localhost');
        });

        it('has settings-field class', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'app_url');
            const field = window.__fieldTest.buildField(setting);
            expect(field.className).toBe('settings-field');
        });
    });

    describe('buildField — password input', () => {
        it('renders input[type=password] with correct data-key', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'openai_api_key');
            const field = window.__fieldTest.buildField(setting);
            const input = field.querySelector('input[data-key="openai_api_key"]');
            expect(input).not.toBeNull();
            expect(input.type).toBe('password');
        });

        it('renders reveal button with visibility icon', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'openai_api_key');
            const field = window.__fieldTest.buildField(setting);
            const revealBtn = field.querySelector('.settings-reveal-btn');
            expect(revealBtn).not.toBeNull();
            expect(revealBtn.querySelector('.material-symbols-outlined').textContent).toBe('visibility');
        });

        it('has has-toggle class on password input', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'openai_api_key');
            const field = window.__fieldTest.buildField(setting);
            const input = field.querySelector('input');
            expect(input.classList.contains('has-toggle')).toBe(true);
        });

        it('password input has autocomplete="new-password"', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'openai_api_key');
            const field = window.__fieldTest.buildField(setting);
            const input = field.querySelector('input');
            expect(input.getAttribute('autocomplete')).toBe('new-password');
        });

        it('reveal button has correct aria-label', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'openai_api_key');
            const field = window.__fieldTest.buildField(setting);
            const revealBtn = field.querySelector('.settings-reveal-btn');
            expect(revealBtn.getAttribute('aria-label')).toBe('Toggle visibility');
        });
    });

    describe('buildField — select', () => {
        it('renders a select element with data-key', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'provider');
            const field = window.__fieldTest.buildField(setting);
            const sel = field.querySelector('select[data-key="provider"]');
            expect(sel).not.toBeNull();
        });

        it('renders all options from schema', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'provider');
            const field = window.__fieldTest.buildField(setting);
            const opts = field.querySelectorAll('option');
            expect(opts.length).toBe(setting.options.length);
        });

        it('pre-selects the current value', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'provider');
            const field = window.__fieldTest.buildField(setting);
            const sel = field.querySelector('select');
            expect(sel.value).toBe(LLMProvider.GEMINI);
        });

        it('each option has correct value attribute', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'provider');
            const field = window.__fieldTest.buildField(setting);
            const opts = Array.from(field.querySelectorAll('option'));
            const values = opts.map(o => o.value);
            expect(values).toContain(LLMProvider.OPENAI);
            expect(values).toContain(LLMProvider.OLLAMA);
            expect(values).toContain(LLMProvider.GEMINI);
            expect(values).toContain(LLMProvider.ANTHROPIC);
        });

        it('each option has correct label text', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'provider');
            const field = window.__fieldTest.buildField(setting);
            const opts = Array.from(field.querySelectorAll('option'));
            const labels = opts.map(o => o.textContent);
            expect(labels).toContain('OpenAI');
            expect(labels).toContain('Ollama');
            expect(labels).toContain('Gemini (Google)');
            expect(labels).toContain('Anthropic (Claude)');
        });
    });

    describe('field interactions', () => {
        it('input event marks field as dirty', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'app_url');
            const field = window.__fieldTest.buildField(setting);
            document.getElementById('test-container').appendChild(field);

            const input = field.querySelector('input[data-key="app_url"]');
            input.value = 'https://new.example.com';
            input.dispatchEvent(new window.Event('input', { bubbles: true }));

            const dirty = window.__fieldTest.getDirty();
            expect(dirty.get('app_url')).toBe('https://new.example.com');
        });

        it('change event marks field as dirty', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'app_url');
            const field = window.__fieldTest.buildField(setting);
            document.getElementById('test-container').appendChild(field);

            const input = field.querySelector('input[data-key="app_url"]');
            input.value = 'https://changed.example.com';
            input.dispatchEvent(new window.Event('change', { bubbles: true }));

            const dirty = window.__fieldTest.getDirty();
            expect(dirty.get('app_url')).toBe('https://changed.example.com');
        });

        it('select change event marks field as dirty', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'provider');
            const field = window.__fieldTest.buildField(setting);
            document.getElementById('test-container').appendChild(field);

            const select = field.querySelector('select');
            select.value = LLMProvider.OLLAMA;
            select.dispatchEvent(new window.Event('change', { bubbles: true }));

            const dirty = window.__fieldTest.getDirty();
            expect(dirty.get('provider')).toBe(LLMProvider.OLLAMA);
        });
    });

    describe('reveal toggle functionality', () => {
        it('clicking reveal button changes input type from password to text', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'openai_api_key');
            const field = window.__fieldTest.buildField(setting);
            document.getElementById('test-container').appendChild(field);

            const pwInput = field.querySelector('input[data-key="openai_api_key"]');
            const revealBtn = field.querySelector('.settings-reveal-btn');

            expect(pwInput.type).toBe('password');
            revealBtn.click();
            expect(pwInput.type).toBe('text');
        });

        it('clicking reveal button again toggles back to password', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'openai_api_key');
            const field = window.__fieldTest.buildField(setting);
            document.getElementById('test-container').appendChild(field);

            const pwInput = field.querySelector('input[data-key="openai_api_key"]');
            const revealBtn = field.querySelector('.settings-reveal-btn');

            revealBtn.click();
            revealBtn.click();
            expect(pwInput.type).toBe('password');
        });

        it('icon changes to visibility_off when revealed', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'openai_api_key');
            const field = window.__fieldTest.buildField(setting);
            document.getElementById('test-container').appendChild(field);

            const pwInput = field.querySelector('input[data-key="openai_api_key"]');
            const revealBtn = field.querySelector('.settings-reveal-btn');

            revealBtn.click();
            expect(revealBtn.querySelector('.material-symbols-outlined').textContent).toBe('visibility_off');
        });

        it('icon returns to visibility when hidden again', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'openai_api_key');
            const field = window.__fieldTest.buildField(setting);
            document.getElementById('test-container').appendChild(field);

            const pwInput = field.querySelector('input[data-key="openai_api_key"]');
            const revealBtn = field.querySelector('.settings-reveal-btn');

            revealBtn.click();
            revealBtn.click();
            expect(revealBtn.querySelector('.material-symbols-outlined').textContent).toBe('visibility');
        });
    });

    describe('field structure validation', () => {
        it('all fields have label and description', () => {
            MOCK_SETTINGS.forEach(setting => {
                const field = window.__fieldTest.buildField(setting);
                expect(field.querySelector('.settings-field-label')).not.toBeNull();
                expect(field.querySelector('.settings-field-desc')).not.toBeNull();
            });
        });

        it('all input fields have data-key attribute', () => {
            MOCK_SETTINGS.forEach(setting => {
                const field = window.__fieldTest.buildField(setting);
                const input = field.querySelector('[data-key]');
                expect(input).not.toBeNull();
                expect(input.dataset.key).toBe(setting.key);
            });
        });

        it('password fields are wrapped in settings-input-wrap', () => {
            const passwordSetting = MOCK_SETTINGS.find(s => s.key === 'openai_api_key');
            const field = window.__fieldTest.buildField(passwordSetting);
            const wrap = field.querySelector('.settings-input-wrap');
            expect(wrap).not.toBeNull();
            expect(wrap.querySelector('input')).not.toBeNull();
        });

        it('text and select fields are not wrapped', () => {
            const textSetting = MOCK_SETTINGS.find(s => s.key === 'app_url');
            const selectSetting = MOCK_SETTINGS.find(s => s.key === 'provider');

            const textField = window.__fieldTest.buildField(textSetting);
            const selectField = window.__fieldTest.buildField(selectSetting);

            expect(textField.querySelector('.settings-input-wrap')).toBeNull();
            expect(selectField.querySelector('.settings-input-wrap')).toBeNull();
        });
    });

    describe('buildField — boolean-valued select options', () => {
        it('renders without throwing when option values are booleans', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'llm_command_gen_enabled');
            expect(() => window.__fieldTest.buildField(setting)).not.toThrow();
        });

        it('renders without throwing when setting.value is boolean false', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'google_search_enabled');
            expect(() => window.__fieldTest.buildField(setting)).not.toThrow();
        });

        it('serializes boolean option values to string attributes', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'llm_command_gen_enabled');
            const field = window.__fieldTest.buildField(setting);
            const opts = Array.from(field.querySelectorAll('option'));
            const values = opts.map(o => o.value);
            expect(values).toContain('true');
            expect(values).toContain('false');
        });

        it('pre-selects boolean true correctly', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'llm_command_gen_enabled');
            const field = window.__fieldTest.buildField(setting);
            const sel = field.querySelector('select');
            expect(sel.value).toBe('true');
        });

        it('pre-selects boolean false correctly', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'google_search_enabled');
            const field = window.__fieldTest.buildField(setting);
            const sel = field.querySelector('select');
            expect(sel.value).toBe('false');
        });

        it('change handler resolves DOM string back to boolean true', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'google_search_enabled');
            const field = window.__fieldTest.buildField(setting);
            document.getElementById('test-container').appendChild(field);

            const select = field.querySelector('select');
            select.value = 'true';
            select.dispatchEvent(new window.Event('change', { bubbles: true }));

            const dirty = window.__fieldTest.getDirty();
            expect(dirty.get('google_search_enabled')).toBe(true);
        });

        it('change handler resolves DOM string back to boolean false', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'llm_command_gen_enabled');
            const field = window.__fieldTest.buildField(setting);
            document.getElementById('test-container').appendChild(field);

            const select = field.querySelector('select');
            select.value = 'false';
            select.dispatchEvent(new window.Event('change', { bubbles: true }));

            const dirty = window.__fieldTest.getDirty();
            expect(dirty.get('llm_command_gen_enabled')).toBe(false);
        });

        it('renders correct labels for boolean options', () => {
            const setting = MOCK_SETTINGS.find(s => s.key === 'llm_command_gen_enabled');
            const field = window.__fieldTest.buildField(setting);
            const opts = Array.from(field.querySelectorAll('option'));
            const labels = opts.map(o => o.textContent);
            expect(labels).toContain('Enabled');
            expect(labels).toContain('Disabled');
        });
    });

    describe('escHtml — type safety', () => {
        it('handles null by returning empty string', () => {
            expect(window.__fieldTest.escHtml(null)).toBe('');
        });

        it('handles undefined by returning empty string', () => {
            expect(window.__fieldTest.escHtml(undefined)).toBe('');
        });

        it('coerces boolean true to string', () => {
            expect(window.__fieldTest.escHtml(true)).toBe('true');
        });

        it('coerces boolean false to string', () => {
            expect(window.__fieldTest.escHtml(false)).toBe('false');
        });

        it('coerces number to string', () => {
            expect(window.__fieldTest.escHtml(42)).toBe('42');
        });

        it('escapes HTML special characters in strings', () => {
            expect(window.__fieldTest.escHtml('<script>"alert"</script>')).toBe(
                '&lt;script&gt;&quot;alert&quot;&lt;/script&gt;'
            );
        });
    });
});
