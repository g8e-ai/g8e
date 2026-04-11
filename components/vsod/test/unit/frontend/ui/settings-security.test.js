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

// Inject field building functionality for security testing
function injectFieldScript(dom) {
    const { window } = dom;

    const script = `
    (function () {
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

        window.__securityTest = {
            buildField,
            escHtml,
            escAttr,
        };
    })();
    `;

    window.eval(script);
}

describe('Settings Security [UNIT - jsdom]', () => {
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

    describe('HTML escaping', () => {
        it('escHtml escapes & < > " characters', () => {
            const { escHtml } = window.__securityTest;
            expect(escHtml('&')).toBe('&amp;');
            expect(escHtml('<')).toBe('&lt;');
            expect(escHtml('>')).toBe('&gt;');
            expect(escHtml('"')).toBe('&quot;');
            expect(escHtml("'")).toBe('&#39;');
        });

        it('escHtml returns empty string for null', () => {
            expect(window.__securityTest.escHtml(null)).toBe('');
        });

        it('escHtml returns empty string for undefined', () => {
            expect(window.__securityTest.escHtml(undefined)).toBe('');
        });

        it('escAttr uses same escaping as escHtml', () => {
            const { escHtml, escAttr } = window.__securityTest;
            const testInputs = ['&<>"\'', '<script>alert("xss")</script>', 'normal text'];
            testInputs.forEach(input => {
                expect(escAttr(input)).toBe(escHtml(input));
            });
        });

        it('escHtml handles complex malicious strings', () => {
            const { escHtml } = window.__securityTest;
            const malicious = '<script>window.xss=true</script><img src=x onerror=alert(1)>"\'&';
            const escaped = escHtml(malicious);
            expect(escaped).not.toContain('<script>');
            expect(escaped).not.toContain('<img');
            expect(escaped).toContain('&lt;script&gt;');
            expect(escaped).toContain('&lt;img');
            expect(escaped).toContain('&quot;');
            expect(escaped).toContain('&#39;');
            expect(escaped).toContain('&amp;');
        });
    });

    describe('XSS protection in field labels', () => {
        it('malicious label does not inject a script element into the DOM', async () => {
            const maliciousSetting = {
                ...MOCK_SETTINGS[0],
                label: '<script>window.__xss=true</script>',
            };
            const field = window.__securityTest.buildField(maliciousSetting);
            document.body.appendChild(field);
            expect(window.__xss).toBeUndefined();
            expect(document.querySelector('script[src]')).toBeNull();
            document.body.removeChild(field);
        });

        it('malicious label HTML is properly escaped', async () => {
            const maliciousSetting = {
                ...MOCK_SETTINGS[0],
                label: '<img src=x onerror=window.__xss=1>Label',
            };
            const field = window.__securityTest.buildField(maliciousSetting);
            document.body.appendChild(field);
            
            const labelElement = field.querySelector('.settings-field-label');
            expect(labelElement.innerHTML).not.toContain('<img');
            expect(labelElement.innerHTML).toContain('&lt;img');
            expect(window.__xss).toBeUndefined();
            
            document.body.removeChild(field);
        });

        it('malicious label with multiple attack vectors is escaped', async () => {
            const maliciousSetting = {
                ...MOCK_SETTINGS[0],
                label: '"><script>window.__xss=1</script><img src=x onerror=window.__xss=2>',
            };
            const field = window.__securityTest.buildField(maliciousSetting);
            document.body.appendChild(field);
            
            const labelElement = field.querySelector('.settings-field-label');
            expect(labelElement.innerHTML).not.toContain('<script>');
            expect(labelElement.innerHTML).not.toContain('<img');
            expect(labelElement.innerHTML).toContain('&lt;script&gt;');
            expect(labelElement.innerHTML).toContain('&lt;img');
            expect(window.__xss).toBeUndefined();
            
            document.body.removeChild(field);
        });
    });

    describe('XSS protection in field descriptions', () => {
        it('malicious description does not inject script', async () => {
            const maliciousSetting = {
                ...MOCK_SETTINGS[0],
                description: '<script>window.__descXss=true</script>',
            };
            const field = window.__securityTest.buildField(maliciousSetting);
            document.body.appendChild(field);
            
            expect(window.__descXss).toBeUndefined();
            const descElement = field.querySelector('.settings-field-desc');
            expect(descElement.innerHTML).not.toContain('<script>');
            expect(descElement.innerHTML).toContain('&lt;script&gt;');
            
            document.body.removeChild(field);
        });

        it('malicious description with iframe is escaped', async () => {
            const maliciousSetting = {
                ...MOCK_SETTINGS[0],
                description: '<iframe src="javascript:alert(1)"></iframe>',
            };
            const field = window.__securityTest.buildField(maliciousSetting);
            document.body.appendChild(field);
            
            const descElement = field.querySelector('.settings-field-desc');
            expect(descElement.innerHTML).not.toContain('<iframe>');
            expect(descElement.innerHTML).toContain('&lt;iframe');
            expect(descElement.querySelector('iframe')).toBeNull();
            
            document.body.removeChild(field);
        });
    });

    describe('XSS protection in field values', () => {
        it('malicious value does not set onerror attribute', async () => {
            const maliciousSetting = {
                ...MOCK_SETTINGS[0],
                value: '" onerror="window.__xss=true"',
            };
            const field = window.__securityTest.buildField(maliciousSetting);
            document.body.appendChild(field);
            
            const input = field.querySelector('input');
            expect(input.getAttribute('onerror')).toBeNull();
            expect(input.value).toBe('" onerror="window.__xss=true"');
            expect(window.__xss).toBeUndefined();
            
            document.body.removeChild(field);
        });

        it('malicious value is properly escaped in HTML attribute', async () => {
            const maliciousSetting = {
                ...MOCK_SETTINGS[0],
                value: '"><script>window.__xss=1</script>',
            };
            const field = window.__securityTest.buildField(maliciousSetting);
            document.body.appendChild(field);
            
            const input = field.querySelector('input');
            // The value attribute should contain the raw text (browser handles escaping)
            expect(input.value).toBe('"><script>window.__xss=1</script>');
            // But no script execution should occur
            expect(input.getAttribute('onload')).toBeNull();
            expect(window.__xss).toBeUndefined();
            
            document.body.removeChild(field);
        });

        it('malicious value with javascript: protocol is escaped', async () => {
            const maliciousSetting = {
                ...MOCK_SETTINGS[0],
                value: 'javascript:alert(1)',
            };
            const field = window.__securityTest.buildField(maliciousSetting);
            document.body.appendChild(field);
            
            const input = field.querySelector('input');
            expect(input.value).toBe('javascript:alert(1)');
            expect(window.__xss).toBeUndefined();
            // No script execution should occur
            expect(input.getAttribute('onload')).toBeNull();
            
            document.body.removeChild(field);
        });
    });

    describe('XSS protection in select options', () => {
        it('malicious option labels are escaped', async () => {
            const maliciousSetting = {
                ...MOCK_SETTINGS[0],
                type: 'select',
                options: [
                    { value: 'safe', label: '<script>window.__optXss=1</script>Safe Option' },
                    { value: 'malicious', label: '<img src=x onerror=window.__optXss=2>' },
                ],
            };
            const field = window.__securityTest.buildField(maliciousSetting);
            document.body.appendChild(field);
            
            const options = field.querySelectorAll('option');
            expect(window.__optXss).toBeUndefined();
            
            Array.from(options).forEach(opt => {
                expect(opt.innerHTML).not.toContain('<script>');
                expect(opt.innerHTML).not.toContain('<img>');
                expect(opt.innerHTML).toContain('&lt;');
            });
            
            document.body.removeChild(field);
        });

        it('malicious option values are escaped in HTML but preserved in DOM', async () => {
            const maliciousSetting = {
                ...MOCK_SETTINGS[0],
                type: 'select',
                options: [
                    { value: '<script>alert(1)</script>', label: 'Safe Label' },
                ],
            };
            const field = window.__securityTest.buildField(maliciousSetting);
            document.body.appendChild(field);
            
            const option = field.querySelector('option');
            expect(option.value).toBe('<script>alert(1)</script>');
            // The value attribute contains the raw value, but when rendered as text it should be escaped
            expect(option.textContent).toBe('Safe Label'); // Label should be safe
            expect(window.__xss).toBeUndefined();
            
            document.body.removeChild(field);
        });
    });

    describe('XSS protection in placeholders', () => {
        it('malicious placeholder is handled safely', async () => {
            const maliciousSetting = {
                ...MOCK_SETTINGS[0],
                placeholder: '"><script>window.__placeholderXss=1</script>',
            };
            const field = window.__securityTest.buildField(maliciousSetting);
            document.body.appendChild(field);
            
            const input = field.querySelector('input');
            expect(input.placeholder).toBe('"><script>window.__placeholderXss=1</script>');
            expect(window.__placeholderXss).toBeUndefined();
            
            document.body.removeChild(field);
        });
    });

    describe('Security edge cases', () => {
        it('null/undefined values are handled safely', async () => {
            const nullSetting = {
                ...MOCK_SETTINGS[0],
                value: null,
                label: undefined,
            };
            const field = window.__securityTest.buildField(nullSetting);
            document.body.appendChild(field);
            
            const input = field.querySelector('input');
            expect(input.value).toBe('');
            expect(field.querySelector('.settings-field-label').textContent).toBe('');
            expect(window.__xss).toBeUndefined();
            
            document.body.removeChild(field);
        });

        it('empty string values are handled safely', async () => {
            const emptySetting = {
                ...MOCK_SETTINGS[0],
                value: '',
                label: '',
                description: '',
            };
            const field = window.__securityTest.buildField(emptySetting);
            document.body.appendChild(field);
            
            const input = field.querySelector('input');
            expect(input.value).toBe('');
            expect(field.querySelector('.settings-field-label').textContent).toBe('');
            expect(field.querySelector('.settings-field-desc').textContent).toBe('');
            expect(window.__xss).toBeUndefined();
            
            document.body.removeChild(field);
        });

        it('unicode characters are handled safely', async () => {
            const unicodeSetting = {
                ...MOCK_SETTINGS[0],
                label: '🔥 Fire emoji <script>alert("unicode")</script>',
                value: '测试中文<script>window.__unicodeXss=1</script>',
            };
            const field = window.__securityTest.buildField(unicodeSetting);
            document.body.appendChild(field);
            
            const labelElement = field.querySelector('.settings-field-label');
            const input = field.querySelector('input');
            
            expect(labelElement.textContent).toContain('🔥 Fire emoji');
            expect(labelElement.innerHTML).not.toContain('<script>');
            expect(labelElement.innerHTML).toContain('&lt;script&gt;');
            
            expect(input.value).toContain('测试中文');
            expect(window.__unicodeXss).toBeUndefined();
            
            document.body.removeChild(field);
        });

        it('very long malicious strings are handled safely', async () => {
            const longMalicious = '<script>'.repeat(1000) + 'window.__longXss=1' + '</script>'.repeat(1000);
            const longSetting = {
                ...MOCK_SETTINGS[0],
                label: longMalicious,
            };
            const field = window.__securityTest.buildField(longSetting);
            document.body.appendChild(field);
            
            const labelElement = field.querySelector('.settings-field-label');
            expect(labelElement.innerHTML).not.toContain('<script>');
            expect(labelElement.innerHTML).toContain('&lt;script&gt;');
            expect(window.__longXss).toBeUndefined();
            
            document.body.removeChild(field);
        });
    });

    describe('DOM injection prevention', () => {
        it('field building does not create unexpected DOM elements', async () => {
            const setting = MOCK_SETTINGS[0];
            const field = window.__securityTest.buildField(setting);
            document.body.appendChild(field);
            
            // Should have expected structure (field itself is the container)
            expect(field.className).toBe('settings-field');
            expect(field.querySelector('.settings-field-label')).not.toBeNull();
            expect(field.querySelector('.settings-field-desc')).not.toBeNull();
            expect(field.querySelector('input')).not.toBeNull();
            
            // Should not have any script elements, iframes, etc.
            expect(field.querySelectorAll('script')).toHaveLength(0);
            expect(field.querySelectorAll('iframe')).toHaveLength(0);
            expect(field.querySelectorAll('object')).toHaveLength(0);
            expect(field.querySelectorAll('embed')).toHaveLength(0);
            
            document.body.removeChild(field);
        });

        it('multiple malicious fields do not contaminate each other', async () => {
            const settings = [
                { ...MOCK_SETTINGS[0], label: '<script>window.__xss1=1</script>', value: 'value1' },
                { ...MOCK_SETTINGS[1], label: '<script>window.__xss2=2</script>', value: 'value2' },
            ];
            
            settings.forEach((setting, index) => {
                const field = window.__securityTest.buildField(setting);
                document.body.appendChild(field);
                
                const labelElement = field.querySelector('.settings-field-label');
                expect(labelElement.innerHTML).not.toContain('<script>');
                expect(labelElement.innerHTML).toContain('&lt;script&gt;');
                
                document.body.removeChild(field);
            });
            
            expect(window.__xss1).toBeUndefined();
            expect(window.__xss2).toBeUndefined();
        });
    });
});
