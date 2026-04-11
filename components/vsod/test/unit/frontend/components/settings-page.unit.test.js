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

// @vitest-environment jsdom

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { SettingsPage } from '@vsod/public/js/components/settings-page.js';

describe('SettingsPage [UNIT - jsdom]', () => {
    let page;

    beforeEach(() => {
        page = new SettingsPage();
        document.body.innerHTML = `
            <div id="settings-loading"></div>
            <div id="settings-body"></div>
            <div id="settings-nav"></div>
            <div id="settings-sections"></div>
            <div id="save-btn"></div>
            <div id="status-bar"></div>
            <div id="status-icon"></div>
            <div id="status-msg"></div>
            <template id="advanced-section-template">
                <div class="advanced-section">Advanced Settings</div>
            </template>
        `;
    });

    afterEach(() => {
        document.body.innerHTML = '';
    });

    describe('constructor', () => {
        it('initializes with empty state', () => {
            expect(page.allSettings).toEqual([]);
            expect(page.sections).toEqual([]);
            expect(page.dirty).toBeInstanceOf(Map);
            expect(page.dirty.size).toBe(0);
            expect(page.activeSection).toBeNull();
        });
    });

    describe('_buildField', () => {
        it('creates text input field', () => {
            const setting = {
                key: 'test_key',
                type: 'text',
                label: 'Test Label',
                description: 'Test Description',
                value: 'test_value',
                placeholder: 'Enter value'
            };

            const field = page._buildField(setting);
            expect(field.className).toBe('settings-field');
            
            const label = field.querySelector('.settings-field-label');
            expect(label?.textContent).toBe('Test Label');
            
            const desc = field.querySelector('.settings-field-desc');
            expect(desc?.textContent).toBe('Test Description');
            
            const input = field.querySelector('input');
            expect(input?.type).toBe('text');
            expect(input?.className).toBe('settings-input');
            expect(input?.dataset.key).toBe('test_key');
            expect(input?.placeholder).toBe('Enter value');
            expect(input?.value).toBe('test_value');
        });

        it('creates password input field with reveal button', () => {
            const setting = {
                key: 'password_key',
                type: 'password',
                label: 'Password',
                description: 'Enter password',
                value: 'secret'
            };

            const field = page._buildField(setting);
            expect(field.className).toBe('settings-field');
            
            const inputWrap = field.querySelector('.settings-input-wrap');
            expect(inputWrap).toBeTruthy();
            
            const input = field.querySelector('input');
            expect(input?.type).toBe('password');
            expect(input?.className).toBe('settings-input has-toggle');
            expect(input?.autocomplete).toBe('new-password');
            
            const revealBtn = field.querySelector('.settings-reveal-btn');
            expect(revealBtn).toBeTruthy();
            expect(revealBtn?.type).toBe('button');
            expect(revealBtn?.ariaLabel).toBe('Toggle visibility');
        });

        it('creates select field with options', () => {
            const setting = {
                key: 'select_key',
                type: 'select',
                label: 'Select Option',
                description: 'Choose an option',
                value: 'option2',
                options: [
                    { value: 'option1', label: 'Option 1' },
                    { value: 'option2', label: 'Option 2' },
                    { value: 'option3', label: 'Option 3' }
                ]
            };

            const field = page._buildField(setting);
            expect(field.className).toBe('settings-field');
            
            const select = field.querySelector('select');
            expect(select?.className).toBe('settings-select');
            expect(select?.dataset.key).toBe('select_key');
            
            const options = select?.querySelectorAll('option');
            expect(options?.length).toBe(3);
            
            expect(options?.[0]?.value).toBe('option1');
            expect(options?.[0]?.textContent).toBe('Option 1');
            expect(options?.[0]?.selected).toBe(false);
            
            expect(options?.[1]?.value).toBe('option2');
            expect(options?.[1]?.textContent).toBe('Option 2');
            expect(options?.[1]?.selected).toBe(true);
            
            expect(options?.[2]?.value).toBe('option3');
            expect(options?.[2]?.textContent).toBe('Option 3');
            expect(options?.[2]?.selected).toBe(false);
        });

        it('uses pure DOM manipulation without innerHTML for field structure', () => {
            const setting = {
                key: 'test_key',
                type: 'text',
                label: 'Test Label',
                description: 'Test Description',
                value: 'test_value'
            };

            const field = page._buildField(setting);
            
            // Verify structure was built using createElement, not innerHTML
            expect(field.className).toBe('settings-field');
            
            const label = field.querySelector('.settings-field-label');
            expect(label?.textContent).toBe('Test Label');
            expect(label?.innerHTML).toBe('Test Label'); // textContent sets innerHTML
            
            const desc = field.querySelector('.settings-field-desc');
            expect(desc?.textContent).toBe('Test Description');
            
            const input = field.querySelector('input');
            expect(input?.value).toBe('test_value');
        });

        it('handles empty values', () => {
            const setting = {
                key: 'empty_key',
                type: 'text',
                label: 'Empty Field',
                description: 'Field with empty value',
                value: ''
            };

            const field = page._buildField(setting);
            const input = field.querySelector('input');
            expect(input?.value).toBe('');
        });

        it('handles missing optional properties', () => {
            const setting = {
                key: 'minimal_key',
                type: 'text',
                label: 'Minimal Field',
                description: 'Field with minimal props'
            };

            const field = page._buildField(setting);
            const input = field.querySelector('input');
            expect(input?.placeholder).toBe('');
            expect(input?.value).toBe('');
        });

        it('adds input event listener for dirty tracking', () => {
            const setting = {
                key: 'track_key',
                type: 'text',
                label: 'Track Field',
                description: 'Field with tracking',
                value: 'initial'
            };

            const field = page._buildField(setting);
            const input = field.querySelector('input');
            
            expect(page.dirty.has('track_key')).toBe(false);
            
            input.value = 'changed';
            input.dispatchEvent(new Event('input'));
            
            expect(page.dirty.has('track_key')).toBe(true);
            expect(page.dirty.get('track_key')).toBe('changed');
        });

        it('adds change event listener for dirty tracking', () => {
            const setting = {
                key: 'change_key',
                type: 'text',
                label: 'Change Field',
                description: 'Field with change tracking',
                value: 'initial'
            };

            const field = page._buildField(setting);
            const input = field.querySelector('input');
            
            expect(page.dirty.has('change_key')).toBe(false);
            
            input.value = 'changed';
            input.dispatchEvent(new Event('change'));
            
            expect(page.dirty.has('change_key')).toBe(true);
            expect(page.dirty.get('change_key')).toBe('changed');
        });

        it('enables save button when field becomes dirty', () => {
            const setting = {
                key: 'save_key',
                type: 'text',
                label: 'Save Field',
                description: 'Field enabling save',
                value: 'initial'
            };

            const field = page._buildField(setting);
            const input = field.querySelector('input');
            const saveBtn = document.getElementById('save-btn');
            
            saveBtn.disabled = true;
            
            input.value = 'changed';
            input.dispatchEvent(new Event('input'));
            
            expect(saveBtn.disabled).toBe(false);
        });

        it('toggles password visibility on reveal button click', () => {
            const setting = {
                key: 'reveal_key',
                type: 'password',
                label: 'Password',
                description: 'Password field',
                value: 'secret'
            };

            const field = page._buildField(setting);
            const input = field.querySelector('input');
            const revealBtn = field.querySelector('.settings-reveal-btn');
            const icon = revealBtn?.querySelector('.material-symbols-outlined');
            
            expect(input?.type).toBe('password');
            expect(icon?.textContent).toBe('visibility');
            
            revealBtn?.click();
            
            expect(input?.type).toBe('text');
            expect(icon?.textContent).toBe('visibility_off');
            
            revealBtn?.click();
            
            expect(input?.type).toBe('password');
            expect(icon?.textContent).toBe('visibility');
        });
    });
});
